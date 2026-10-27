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
