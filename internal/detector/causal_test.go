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

func TestARecoveryCitesTheMessageThatClearedIt(t *testing.T) {
	h := rootedHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")
	h.sched.run(tickAt(120), true)
	if err := h.det.HeartbeatReceived(tickAt(120), "B", "A", 1, 9); err != nil {
		t.Fatal(err)
	}
	h.sched.run(tickAt(480), true)
	if n := h.rec.count(domain.KindSuspicionRaised); n != 1 {
		t.Fatalf("suspicion events = %d, want 1 before recovery", n)
	}
	h.sched.run(tickAt(600), true)
	if err := h.det.HeartbeatReceived(tickAt(600), "B", "A", 2, 10); err != nil {
		t.Fatal(err)
	}

	cleared := h.rec.ofKind(domain.KindSuspicionCleared)
	if len(cleared) != 1 {
		t.Fatalf("recoveries = %d, want 1", len(cleared))
	}
	receipts := h.rec.ofKind(domain.KindHeartbeatReceived)
	last := receipts[len(receipts)-1]
	if want := h.rec.refOf(last); len(cleared[0].Causes) != 1 || cleared[0].Causes[0] != want {
		t.Fatalf("recovery cites %v, want the receipt at 600 ms (%d)", cleared[0].Causes, want)
	}
	raised := h.rec.ofKind(domain.KindSuspicionRaised)[0]
	if cleared[0].Causes[0] <= raised.Causes[0] {
		t.Fatalf("the recovery cites %v, which is not after the evidence the suspicion cited %v",
			cleared[0].Causes, raised.Causes)
	}
}

func TestAnObserverThatNeverHeardAnythingCitesTheScenario(t *testing.T) {
	h := rootedHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")
	h.sched.run(tickAt(400), true)

	raised := h.rec.ofKind(domain.KindSuspicionRaised)
	if len(raised) != 1 {
		t.Fatalf("suspicion events = %d, want 1", len(raised))
	}
	if want := domain.Ref(1); len(raised[0].Causes) != 1 || raised[0].Causes[0] != want {
		t.Fatalf("suspicion cites %v, want the scenario %d: no message armed this deadline", raised[0].Causes, want)
	}
	if h.rec.count(domain.KindHeartbeatReceived) != 0 {
		t.Fatal("the harness received a message it did not expect")
	}
}

func TestAReceiptCitesTheSendItAnswers(t *testing.T) {
	h := rootedHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")
	h.sched.run(tickAt(120), true)
	if err := h.det.HeartbeatReceived(tickAt(120), "B", "A", 1, 42); err != nil {
		t.Fatal(err)
	}
	receipt := h.rec.ofKind(domain.KindHeartbeatReceived)[0]
	if len(receipt.Causes) != 1 || receipt.Causes[0] != domain.Ref(42) {
		t.Fatalf("receipt cites %v, want the send that was handed to it", receipt.Causes)
	}
}
