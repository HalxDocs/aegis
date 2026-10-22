package detector

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestCancelledDeadlineNeverFires(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")

	if err := h.det.HeartbeatReceived(tickAt(120), "B", "A", 1); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	h.sched.run(tickAt(400), true)

	if n := h.rec.count(domain.KindSuspicionRaised); n != 0 {
		t.Fatalf("suspicion events = %d, want 0: the 350 deadline was replaced", n)
	}
	if obs := h.believe(t, "B"); obs.Belief != domain.BeliefHealthy {
		t.Fatalf("belief at 400 = %s, want healthy", obs.Belief)
	}
}

func TestPhantomDeadlineIsIgnoredByTheEpochGuard(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")
	if err := h.det.HeartbeatReceived(tickAt(120), "B", "A", 1); err != nil {
		t.Fatalf("receipt: %v", err)
	}

	// Deliberately run cancelled work, which is what a scheduler that lost a
	// cancellation would do. The stale callback must change nothing.
	h.sched.run(tickAt(400), false)

	if n := h.rec.count(domain.KindSuspicionRaised); n != 0 {
		t.Fatalf("suspicion events after a stale deadline = %d, want 0", n)
	}
	if n := len(h.det.Reports()); n != 0 {
		t.Fatalf("reports after a stale deadline = %d, want 0", n)
	}
	if obs := h.believe(t, "B"); obs.Belief != domain.BeliefHealthy {
		t.Fatalf("belief = %s, want healthy", obs.Belief)
	}

	// The live deadline still works, so the guard did not disable the monitor.
	h.sched.run(tickAt(470), true)
	if obs := h.believe(t, "B"); obs.Belief != domain.BeliefSuspected || obs.SuspectedAt != tickAt(470) {
		t.Fatalf("belief at 470 = %+v, want suspected", obs)
	}
	h.rec.requireClean(t)
}

func TestMessageAtTheDeadlineInstantsWinsTheTie(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")

	// The delivery is ordinary work and the deadline is end-of-instant work, so
	// the message is observed first even though it was scheduled later.
	if _, err := h.sched.At(tickAt(350), func() {
		if err := h.det.HeartbeatReceived(tickAt(350), "B", "A", 4); err != nil {
			t.Errorf("receipt: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	h.sched.run(tickAt(350), true)

	obs := h.believe(t, "B")
	if obs.Belief != domain.BeliefHealthy || obs.LastSeen != tickAt(350) || obs.Heard != 1 {
		t.Fatalf("observation = %+v, want healthy with last seen 350", obs)
	}
	if n := h.rec.count(domain.KindSuspicionRaised); n != 0 {
		t.Fatalf("suspicion events = %d, want 0: evidence at the deadline is evidence", n)
	}
	if n := len(h.det.Reports()); n != 0 {
		t.Fatalf("reports = %d, want 0", n)
	}
	if got := h.sched.live(); got != 1 {
		t.Fatalf("live timers = %d, want the replacement deadline only", got)
	}
	h.rec.requireClean(t)
}

func TestRecorderFailureInsideADeadlineAbortsTheRun(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")

	boom := errors.New("disk full")
	h.rec.err = boom
	h.sched.run(tickAt(350), true)

	if len(h.faults.errs) != 1 {
		t.Fatalf("faults = %v, want exactly one", h.faults.errs)
	}
	if !errors.Is(h.faults.errs[0], boom) {
		t.Fatalf("fault = %v, want %v", h.faults.errs[0], boom)
	}
	if obs := h.believe(t, "B"); obs.Belief != domain.BeliefHealthy {
		t.Fatalf("belief = %s, want unchanged after a failed write", obs.Belief)
	}
	if n := len(h.det.Reports()); n != 0 {
		t.Fatalf("reports = %d, want none: an unrecorded report must not exist", n)
	}
}
