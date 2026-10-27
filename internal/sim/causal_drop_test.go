package sim

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

// dropped runs a world with a link that always loses its messages and returns
// the recorded drop.
func dropped(t *testing.T, spec func(*Engine) error) (domain.Event, *stubRecorder, *Engine) {
	t.Helper()
	e, rec := newTestEngine(t, EngineConfig{})
	if err := e.AddNode(NodeConfig{ID: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := e.AddNode(NodeConfig{ID: "B", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"A"}}); err != nil {
		t.Fatal(err)
	}
	if err := spec(e); err != nil {
		t.Fatal(err)
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
	drops := rec.ofKind(domain.KindMessageDropped)
	if len(drops) != 1 {
		t.Fatalf("drops = %d, want 1", len(drops))
	}
	return drops[0], rec, e
}

func TestAPolicyDropCitesTheMessageThatWasLost(t *testing.T) {
	cause, rec, e := dropped(t, func(e *Engine) error {
		return e.AddLink(LinkConfig{From: "B", To: "A", Latency: tickAt(20), DropPPM: 1_000_000})
	})
	sent := rec.ofKind(domain.KindHeartbeatSent)[0]
	if want := []domain.Ref{rec.refOf(sent)}; !equalRefs(cause.Causes, want) {
		t.Fatalf("drop cites %v, want the send %v", cause.Causes, want)
	}
	if cause.Reason != domain.ReasonDropPolicy {
		t.Fatalf("reason = %q, want %q", cause.Reason, domain.ReasonDropPolicy)
	}
	if len(cause.Causes) > 1 {
		t.Fatalf("a policy drop cites %v; only the send caused it", cause.Causes)
	}
	_ = e
}

func TestADropInsideACutAlsoCitesTheCut(t *testing.T) {
	cause, rec, e := dropped(t, func(e *Engine) error {
		if err := e.AddLink(LinkConfig{From: "B", To: "A", Latency: tickAt(20)}); err != nil {
			return err
		}
		return e.PartitionAt("B", "A", tickAt(50), tickAt(400))
	})
	cut := rec.ofKind(domain.KindLinkPartitioned)[0]
	sent := rec.ofKind(domain.KindHeartbeatSent)[0]
	if cause.Reason != domain.ReasonPartition {
		t.Fatalf("reason = %q, want %q", cause.Reason, domain.ReasonPartition)
	}
	want := []domain.Ref{rec.refOf(cut), rec.refOf(sent)}
	if !equalRefs(cause.Causes, want) {
		t.Fatalf("drop cites %v, want the cut and the send %v", cause.Causes, want)
	}
	if got, _ := e.Truth("B"); got != domain.StateRunning {
		t.Fatalf("truth for B = %s: a cut must not change ground truth", got)
	}
}

// TestADropIsNotBlamedOnARepairThatCameLater is the boundary the two-parent
// edge exists for: the message died in the cut, so the cut is cited even though
// the repair is the more recent event when the drop is written down.
func TestADropIsNotBlamedOnARepairThatCameLater(t *testing.T) {
	e, rec := newTestEngine(t, EngineConfig{})
	if err := e.AddNode(NodeConfig{ID: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := e.AddNode(NodeConfig{ID: "B", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"A"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.AddLink(LinkConfig{From: "B", To: "A", Latency: tickAt(200)}); err != nil {
		t.Fatal(err)
	}
	if err := e.PartitionAt("B", "A", tickAt(50), tickAt(150)); err != nil {
		t.Fatal(err)
	}
	if err := e.AddReceiver(&stubReceiver{}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(400)); err != nil {
		t.Fatal(err)
	}

	cut := rec.ofKind(domain.KindLinkPartitioned)[0]
	repair := rec.ofKind(domain.KindLinkRestored)[0]
	// The send at 100 ms is due at 300 ms, long after the repair at 150 ms.
	drops := rec.ofKind(domain.KindMessageDropped)
	var late domain.Event
	for _, d := range drops {
		if d.At == tickAt(300) {
			late = d
		}
	}
	if late.Kind == "" {
		t.Fatalf("no drop at 300 ms; drops were %+v", drops)
	}
	if want := []domain.Ref{rec.refOf(cut), rec.refOf(rec.ofKind(domain.KindHeartbeatSent)[0])}; !equalRefs(late.Causes, want) {
		t.Fatalf("the late drop cites %v, want %v", late.Causes, want)
	}
	for _, cause := range late.Causes {
		if cause == rec.refOf(repair) {
			t.Fatalf("the drop cites the repair %s, which did not kill it", cause)
		}
	}
}
