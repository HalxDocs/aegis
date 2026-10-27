package detector

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

// rootedHarness records a scenario_started event first, so the root a detector
// cites is a real event rather than a reference to the suspicion itself.
func rootedHarness(t *testing.T, observer domain.NodeID, timeout domain.Tick) *harness {
	t.Helper()
	h := &harness{
		sched:  newFakeScheduler(),
		rec:    &stubRecorder{},
		faults: &stubFaults{},
	}
	root, err := h.rec.Record(domain.Event{At: 0, Kind: domain.KindScenarioStarted, Scenario: "test"})
	if err != nil {
		t.Fatalf("record root: %v", err)
	}
	det, err := New(Config{
		Observer:  observer,
		Timeout:   timeout,
		Clock:     h.sched,
		Scheduler: h.sched,
		Recorder:  h.rec,
		Faults:    h.faults,
		Root:      root,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.det = det
	return h
}

// refOf finds the reference the stub would have given an event: the stub does
// not assign sequence numbers, so a position in the slice is the reference.
func (r *stubRecorder) refOf(ev domain.Event) domain.Ref {
	for i, other := range r.events {
		if other.At == ev.At && other.Kind == ev.Kind && other.Node == ev.Node && other.Peer == ev.Peer && other.Round == ev.Round {
			return domain.Ref(i + 1)
		}
	}
	return 0
}

func TestASuspicionCitesTheEvidenceItsDeadlineMeasured(t *testing.T) {
	h := rootedHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")
	h.sched.run(tickAt(120), true)
	if err := h.det.HeartbeatReceived(tickAt(120), "B", "A", 1, 9); err != nil {
		t.Fatal(err)
	}
	h.sched.run(tickAt(600), true)

	raised := h.rec.ofKind(domain.KindSuspicionRaised)
	if len(raised) != 1 {
		t.Fatalf("suspicion events = %d, want 1", len(raised))
	}
	receipt := h.rec.ofKind(domain.KindHeartbeatReceived)[0]
	if want := h.rec.refOf(receipt); len(raised[0].Causes) != 1 || raised[0].Causes[0] != want {
		t.Fatalf("suspicion cites %v, want the last receipt %d", raised[0].Causes, want)
	}
}
