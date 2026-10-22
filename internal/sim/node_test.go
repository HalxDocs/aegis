package sim

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestNodeStartRecordsSortedStartup(t *testing.T) {
	e, rec, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.Run(tickAt(250)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	started := rec.ofKind(domain.KindNodeStarted)
	if len(started) != 3 {
		t.Fatalf("node_started events = %d, want 3", len(started))
	}
	for i, want := range []domain.NodeID{"A", "B", "C"} {
		if started[i].Node != want {
			t.Fatalf("start order = %s, %s, %s; want A, B, C", started[0].Node, started[1].Node, started[2].Node)
		}
		if started[i].At != 0 {
			t.Fatalf("node %s started at %d, want 0", want, started[i].At)
		}
	}
	rec.requireClean(t)
}

func TestNodeHeartbeatRoundsAndDelivery(t *testing.T) {
	e, rec, rx := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(250)); err != nil {
		t.Fatal(err)
	}

	sent := rec.ofKind(domain.KindHeartbeatSent)
	type wantBeat struct {
		peer  domain.NodeID
		round uint64
		at    domain.Tick
	}
	want := []wantBeat{
		{"A", 1, tickAt(100)},
		{"C", 1, tickAt(100)},
		{"A", 2, tickAt(200)},
		{"C", 2, tickAt(200)},
	}
	if len(sent) != len(want) {
		t.Fatalf("heartbeat_sent events = %d, want %d", len(sent), len(want))
	}
	for i := range want {
		if sent[i].Peer != want[i].peer || sent[i].Round != want[i].round || sent[i].At != want[i].at {
			t.Fatalf("sent[%d] = %s -> %s round %d at %d, want %+v",
				i, sent[i].Node, sent[i].Peer, sent[i].Round, sent[i].At, want[i])
		}
	}

	if len(rx.got) != 4 {
		t.Fatalf("deliveries = %d, want 4", len(rx.got))
	}
	if rx.got[0].At != tickAt(120) || rx.got[1].At != tickAt(125) {
		t.Fatalf("first round delivered at %d and %d, want 120 and 125", rx.got[0].At, rx.got[1].At)
	}
	if rx.got[2].At != tickAt(220) || rx.got[3].At != tickAt(225) {
		t.Fatalf("second round delivered at %d and %d, want 220 and 225", rx.got[2].At, rx.got[3].At)
	}
	rec.requireClean(t)
}

func TestNodeVisitsPeersInSortedOrder(t *testing.T) {
	e, rec := newTestEngine(t, EngineConfig{})
	for _, cfg := range []NodeConfig{
		{ID: "A"},
		{ID: "B", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"C", "A"}},
		{ID: "C"},
	} {
		if err := e.AddNode(cfg); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range []LinkConfig{
		{From: "B", To: "A"},
		{From: "B", To: "C"},
	} {
		if err := e.AddLink(link); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.AddReceiver(&stubReceiver{}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(150)); err != nil {
		t.Fatal(err)
	}

	sent := rec.ofKind(domain.KindHeartbeatSent)
	if len(sent) != 2 {
		t.Fatalf("sent events = %d, want 2", len(sent))
	}
	if sent[0].Peer != "A" || sent[1].Peer != "C" {
		t.Fatalf("peer order = %s then %s, want A then C", sent[0].Peer, sent[1].Peer)
	}
}

func TestCrashStopsTheNodesOwnHeartbeats(t *testing.T) {
	e, rec, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.CrashAt(tickAt(150), "B"); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(1200)); err != nil {
		t.Fatal(err)
	}

	if got := rec.count(domain.KindHeartbeatSent); got != 2 {
		t.Fatalf("heartbeats after a crash at 150 = %d, want 2 (round one only)", got)
	}
	crashes := rec.ofKind(domain.KindNodeCrashed)
	if len(crashes) != 1 || crashes[0].Node != "B" || crashes[0].At != tickAt(150) {
		t.Fatalf("crash events = %+v", crashes)
	}
	state, ok := e.Truth("B")
	if !ok || state != domain.StateCrashed {
		t.Fatalf("ground truth for B = %q, %v", state, ok)
	}
	rec.requireClean(t)
}

func TestCrashIsTerminal(t *testing.T) {
	e, _, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.CrashAt(tickAt(150), "B"); err != nil {
		t.Fatal(err)
	}
	if err := e.CrashAt(tickAt(160), "B"); err != nil {
		t.Fatal(err)
	}
	err := e.Run(tickAt(500))
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("second crash error = %v, want ErrInvalidTransition", err)
	}
}

func TestCrashedNodeRejectsFurtherWork(t *testing.T) {
	e, _, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.CrashAt(tickAt(150), "B"); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(200)); err != nil {
		t.Fatal(err)
	}

	crashed := e.nodes["B"]
	if err := crashed.beat(tickAt(300)); err == nil {
		t.Fatal("a crashed node must reject a heartbeat")
	}
	if err := crashed.start(tickAt(300)); err == nil {
		t.Fatal("a crashed node must reject a restart")
	}
}
