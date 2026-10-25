package sim

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

type delivery struct {
	at  domain.Tick
	msg Message
}

func newTestNetwork(t *testing.T, seed uint64) (*network, *scheduler, *stubRecorder, *[]delivery) {
	t.Helper()
	sched := newScheduler(0)
	rec := &stubRecorder{}
	net := newNetwork(sched, rec, newRNG(seed))
	got := &[]delivery{}
	net.deliver = func(at domain.Tick, msg Message) {
		*got = append(*got, delivery{at: at, msg: msg})
	}
	// The engine supplies this in a real run; without it a failed write inside a
	// scheduled task would be lost, which is exactly what the hook exists for.
	net.fault = func(err error) { t.Errorf("scheduled write failed: %v", err) }
	return net, sched, rec, got
}

func runScheduler(t *testing.T, s *scheduler) {
	t.Helper()
	for s.next() != nil {
		if err := s.step(); err != nil {
			t.Fatalf("step: %v", err)
		}
	}
}

func TestNetworkDeliversAfterConfiguredLatency(t *testing.T) {
	net, sched, rec, got := newTestNetwork(t, 1)
	if err := net.addLink(LinkConfig{From: "B", To: "A", Latency: tickAt(250)}); err != nil {
		t.Fatalf("addLink: %v", err)
	}
	if err := net.send(tickAt(100), Message{From: "B", To: "A", Round: 1}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(*got) != 0 {
		t.Fatal("message delivered before time advanced")
	}

	runScheduler(t, sched)

	if len(*got) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(*got))
	}
	if (*got)[0].at != tickAt(350) {
		t.Fatalf("delivered at %d, want %d", (*got)[0].at, tickAt(350))
	}
	if (*got)[0].msg != (Message{From: "B", To: "A", Round: 1}) {
		t.Fatalf("delivered message = %+v", (*got)[0].msg)
	}
	rec.requireClean(t)
}

func TestNetworkZeroLatencyDeliversAtSendTime(t *testing.T) {
	net, sched, _, got := newTestNetwork(t, 1)
	if err := net.addLink(LinkConfig{From: "B", To: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := net.send(tickAt(100), Message{From: "B", To: "A", Round: 3}); err != nil {
		t.Fatal(err)
	}
	runScheduler(t, sched)
	if len(*got) != 1 || (*got)[0].at != tickAt(100) {
		t.Fatalf("deliveries = %+v, want one at 100", *got)
	}
}

func TestNetworkDropIsDecidedAtSendTime(t *testing.T) {
	net, sched, rec, got := newTestNetwork(t, 1)
	if err := net.addLink(LinkConfig{From: "B", To: "A", Latency: tickAt(20), DropPPM: ppmScale}); err != nil {
		t.Fatal(err)
	}
	if err := net.send(tickAt(100), Message{From: "B", To: "A", Round: 2}); err != nil {
		t.Fatal(err)
	}

	if sched.next() != nil {
		t.Fatal("a dropped message must not be scheduled for delivery")
	}
	runScheduler(t, sched)
	if len(*got) != 0 {
		t.Fatalf("dropped message was delivered: %+v", *got)
	}
	if n := rec.count(domain.KindHeartbeatSent); n != 1 {
		t.Fatalf("sent events = %d, want 1", n)
	}
	dropped := rec.ofKind(domain.KindMessageDropped)
	if len(dropped) != 1 {
		t.Fatalf("dropped events = %d, want 1", len(dropped))
	}
	if dropped[0].Reason != domain.ReasonDropPolicy {
		t.Fatalf("drop reason = %q, want %q", dropped[0].Reason, domain.ReasonDropPolicy)
	}
	if dropped[0].Round != 2 {
		t.Fatalf("drop round = %d, want 2", dropped[0].Round)
	}
	rec.requireClean(t)
}

func TestNetworkSendRejectsUnknownLink(t *testing.T) {
	net, _, _, _ := newTestNetwork(t, 1)
	if err := net.addLink(LinkConfig{From: "B", To: "A"}); err != nil {
		t.Fatal(err)
	}
	err := net.send(tickAt(1), Message{From: "C", To: "A", Round: 1})
	if !errors.Is(err, ErrUnknownLink) {
		t.Fatalf("error = %v, want ErrUnknownLink", err)
	}
}

func TestNetworkSendRejectsInvalidMessage(t *testing.T) {
	net, _, _, _ := newTestNetwork(t, 1)
	if err := net.addLink(LinkConfig{From: "B", To: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := net.send(tickAt(1), Message{From: "B", To: "A"}); err == nil {
		t.Fatal("round zero must be rejected")
	}
	if err := net.send(tickAt(1), Message{From: "B", To: "B", Round: 1}); !errors.Is(err, domain.ErrInvalidLink) {
		t.Fatalf("self message error = %v, want ErrInvalidLink", err)
	}
}

func TestNetworkRejectsBadLinkConfiguration(t *testing.T) {
	cases := map[string]LinkConfig{
		"negative latency": {From: "B", To: "A", Latency: -1},
		"drop above scale": {From: "B", To: "A", DropPPM: ppmScale + 1},
		"self link":        {From: "A", To: "A"},
		"invalid endpoint": {From: "B", To: "bad node"},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			net, _, _, _ := newTestNetwork(t, 1)
			if err := net.addLink(cfg); err == nil {
				t.Fatal("addLink accepted an invalid configuration")
			}
		})
	}
}

func TestNetworkRejectsDuplicateLink(t *testing.T) {
	net, _, _, _ := newTestNetwork(t, 1)
	if err := net.addLink(LinkConfig{From: "B", To: "A", Latency: tickAt(20)}); err != nil {
		t.Fatal(err)
	}
	if err := net.addLink(LinkConfig{From: "B", To: "A", Latency: tickAt(30)}); err == nil {
		t.Fatal("duplicate link must be rejected")
	}
}

func TestHeartbeatToAPeerWithoutALinkStopsTheRun(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	for _, cfg := range []NodeConfig{
		{ID: "A"},
		{ID: "B", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"A"}},
	} {
		if err := e.AddNode(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.AddReceiver(&stubReceiver{}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(200)); !errors.Is(err, ErrUnknownLink) {
		t.Fatalf("Run error = %v, want ErrUnknownLink: an undeclared path must not be guessed", err)
	}
}
