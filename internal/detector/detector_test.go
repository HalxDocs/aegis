package detector

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestNewValidatesConfig(t *testing.T) {
	clock := newFakeScheduler()
	base := Config{
		Observer:  "A",
		Timeout:   tickAt(350),
		Clock:     clock,
		Scheduler: clock,
		Recorder:  &stubRecorder{},
		Faults:    &stubFaults{},
	}

	cases := map[string]struct {
		mutate func(*Config)
		want   error
	}{
		"empty observer":     {func(c *Config) { c.Observer = "" }, domain.ErrInvalidNodeID},
		"invalid observer":   {func(c *Config) { c.Observer = "bad node" }, domain.ErrInvalidNodeID},
		"zero timeout":       {func(c *Config) { c.Timeout = 0 }, ErrInvalidTimeout},
		"negative timeout":   {func(c *Config) { c.Timeout = -1 }, ErrInvalidTimeout},
		"missing clock":      {func(c *Config) { c.Clock = nil }, ErrNoClock},
		"missing scheduler":  {func(c *Config) { c.Scheduler = nil }, ErrNoScheduler},
		"missing recorder":   {func(c *Config) { c.Recorder = nil }, ErrNoRecorder},
		"missing fault sink": {func(c *Config) { c.Faults = nil }, ErrNoFaultSink},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := base
			tc.mutate(&cfg)
			_, err := New(cfg)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	det, err := New(base)
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if det.Observer() != "A" || det.Timeout() != tickAt(350) {
		t.Fatalf("detector = %s/%d, want A/%d", det.Observer(), det.Timeout(), tickAt(350))
	}
}

func TestWatchRejectsInvalidTargets(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	if err := h.det.Watch(""); !errors.Is(err, domain.ErrInvalidNodeID) {
		t.Fatalf("empty target error = %v, want ErrInvalidNodeID", err)
	}
	if err := h.det.Watch("bad node"); !errors.Is(err, domain.ErrInvalidNodeID) {
		t.Fatalf("invalid target error = %v, want ErrInvalidNodeID", err)
	}
	if err := h.det.Watch("A"); err == nil {
		t.Fatal("an observer must not watch itself")
	}
	h.mustWatch(t, "B")
	if err := h.det.Watch("B"); !errors.Is(err, ErrAlreadyWatched) {
		t.Fatalf("duplicate watch error = %v, want ErrAlreadyWatched", err)
	}
}

func TestWatchStartsHealthyWithOneDeadline(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")

	obs := h.believe(t, "B")
	if obs.Belief != domain.BeliefHealthy || obs.LastSeen != 0 || obs.Heard != 0 {
		t.Fatalf("initial observation = %+v, want healthy with no evidence", obs)
	}
	if err := obs.Validate(); err != nil {
		t.Fatalf("initial observation invalid: %v", err)
	}
	if live := h.sched.live(); live != 1 {
		t.Fatalf("live timers = %d, want 1", live)
	}
	if got := h.sched.liveTask(t); got.at != tickAt(350) || got.class != classEnd {
		t.Fatalf("deadline = %d class %d, want %d at end of instant", got.at, got.class, tickAt(350))
	}
	if targets := h.det.Targets(); len(targets) != 1 || targets[0] != "B" {
		t.Fatalf("targets = %v, want [B]", targets)
	}
}

func TestMessagesAddressedToAnotherObserverAreIgnored(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")

	if err := h.det.HeartbeatReceived(tickAt(120), "B", "C", 1, 1); err != nil {
		t.Fatalf("unaddressed message returned %v", err)
	}
	if len(h.rec.events) != 0 {
		t.Fatalf("records = %+v, want none: A never received that message", h.rec.events)
	}
	if obs := h.believe(t, "B"); obs.LastSeen != 0 || obs.Belief != domain.BeliefHealthy {
		t.Fatalf("observation = %+v, want untouched", obs)
	}
	if live := h.sched.live(); live != 1 {
		t.Fatalf("live timers = %d, want the original deadline only", live)
	}
}

func TestReceiptFromUnwatchedSenderIsStillRecorded(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")

	if err := h.det.HeartbeatReceived(tickAt(120), "C", "A", 7, 1); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	received := h.rec.ofKind(domain.KindHeartbeatReceived)
	if len(received) != 1 || received[0].Node != "C" || received[0].Peer != "A" || received[0].Round != 7 {
		t.Fatalf("received events = %+v, want one C->A round 7", received)
	}
	if _, ok := h.det.Observation("C"); ok {
		t.Fatal("a receipt must not create a monitor for an unwatched sender")
	}
	if live := h.sched.live(); live != 1 {
		t.Fatalf("live timers = %d, want the original deadline only", live)
	}
	h.rec.requireClean(t)
}

func TestReceiptRefreshesBeliefAndDeadline(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")

	if err := h.det.HeartbeatReceived(tickAt(120), "B", "A", 1, 1); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	obs := h.believe(t, "B")
	if obs.Belief != domain.BeliefHealthy || obs.LastSeen != tickAt(120) || obs.Heard != 1 {
		t.Fatalf("observation = %+v, want healthy at 120 with one receipt", obs)
	}
	if live := h.sched.live(); live != 1 {
		t.Fatalf("live timers = %d, want the earlier deadline cancelled and one replacement", live)
	}
	if got := h.sched.liveTask(t); got.at != tickAt(470) {
		t.Fatalf("deadline = %d, want 470", got.at)
	}

	h.sched.run(tickAt(469), true)
	if obs := h.believe(t, "B"); obs.Belief != domain.BeliefHealthy {
		t.Fatalf("belief at 469 = %s, want healthy", obs.Belief)
	}
	if n := h.rec.count(domain.KindSuspicionRaised); n != 0 {
		t.Fatalf("suspicion events before the deadline = %d, want 0", n)
	}
	h.rec.requireClean(t)
}

func TestSilenceRaisesSuspicionAtTheExactDeadline(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")

	h.sched.run(tickAt(349), true)
	if obs := h.believe(t, "B"); obs.Belief != domain.BeliefHealthy {
		t.Fatalf("belief at 349 = %s, want healthy", obs.Belief)
	}

	h.sched.run(tickAt(350), true)
	obs := h.believe(t, "B")
	if obs.Belief != domain.BeliefSuspected || obs.SuspectedAt != tickAt(350) {
		t.Fatalf("observation = %+v, want suspected at 350", obs)
	}
	if err := obs.Validate(); err != nil {
		t.Fatalf("suspected observation invalid: %v", err)
	}

	reports := h.det.Reports()
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	want := domain.FailureReport{
		Observer:    "A",
		Target:      "B",
		SuspectedAt: tickAt(350),
		LastSeenAt:  0,
		Timeout:     tickAt(350),
		Silence:     tickAt(350),
	}
	if reports[0] != want {
		t.Fatalf("report = %+v, want %+v", reports[0], want)
	}
	if err := reports[0].Validate(); err != nil {
		t.Fatalf("report invalid: %v", err)
	}

	raised := h.rec.ofKind(domain.KindSuspicionRaised)
	if len(raised) != 1 || raised[0].At != tickAt(350) || raised[0].Node != "B" ||
		raised[0].Peer != "A" || raised[0].Reason != domain.ReasonHeartbeatTimeout {
		t.Fatalf("suspicion event = %+v", raised)
	}
	h.rec.requireClean(t)
}

func TestSuspicionIsRaisedOnlyOnce(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")
	h.sched.run(tickAt(1200), true)

	if n := len(h.det.Reports()); n != 1 {
		t.Fatalf("reports = %d, want 1", n)
	}
	if n := h.rec.count(domain.KindSuspicionRaised); n != 1 {
		t.Fatalf("suspicion events = %d, want 1", n)
	}
	if live := h.sched.live(); live != 0 {
		t.Fatalf("live timers = %d, want none: a suspected monitor arms no deadline", live)
	}
}

func TestFreshEvidenceClearsSuspicion(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")
	h.sched.run(tickAt(350), true)

	if err := h.det.HeartbeatReceived(tickAt(500), "B", "A", 3, 1); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	obs := h.believe(t, "B")
	if obs.Belief != domain.BeliefHealthy || obs.SuspectedAt != 0 {
		t.Fatalf("observation = %+v, want healthy with no suspicion time", obs)
	}
	if obs.LastSeen != tickAt(500) || obs.Heard != 1 {
		t.Fatalf("observation = %+v, want last seen 500", obs)
	}
	cleared := h.rec.ofKind(domain.KindSuspicionCleared)
	if len(cleared) != 1 || cleared[0].At != tickAt(500) || cleared[0].Node != "B" || cleared[0].Peer != "A" {
		t.Fatalf("cleared events = %+v", cleared)
	}
	if got := h.sched.liveTask(t); got.at != tickAt(850) {
		t.Fatalf("deadline = %d, want a live 850", got.at)
	}

	// Suspicion is a statement about silence, so renewed silence raises it
	// again rather than leaving a permanently cleared belief.
	h.sched.run(tickAt(1200), true)
	if obs := h.believe(t, "B"); obs.Belief != domain.BeliefSuspected || obs.SuspectedAt != tickAt(850) {
		t.Fatalf("belief after renewed silence = %+v, want suspected at 850", obs)
	}
	if n := h.rec.count(domain.KindSuspicionCleared); n != 1 {
		t.Fatalf("cleared events = %d, want 1", n)
	}
	h.rec.requireClean(t)
}

func TestLateMessageIsRejected(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")
	if err := h.det.HeartbeatReceived(tickAt(500), "B", "A", 2, 1); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if err := h.det.HeartbeatReceived(tickAt(400), "B", "A", 1, 1); !errors.Is(err, domain.ErrInvalidObservation) {
		t.Fatalf("out-of-order receipt error = %v, want ErrInvalidObservation", err)
	}
	if obs := h.believe(t, "B"); obs.LastSeen != tickAt(500) || obs.Heard != 1 {
		t.Fatalf("observation = %+v, want the later receipt only", obs)
	}
}

func TestReturnedSnapshotsCannotMutateTheDetector(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.mustWatch(t, "B")
	h.sched.run(tickAt(350), true)

	reports := h.det.Reports()
	reports[0].Silence = 1
	if got := h.det.Reports()[0].Silence; got != tickAt(350) {
		t.Fatalf("internal report mutated through a snapshot: silence = %d", got)
	}

	targets := h.det.Targets()
	targets[0] = "Z"
	if got := h.det.Targets()[0]; got != "B" {
		t.Fatalf("internal target list mutated through a snapshot: %v", got)
	}
}

func TestWatchingMidRunStartsTheDeadlineFromNow(t *testing.T) {
	h := newHarness(t, "A", tickAt(350))
	h.sched.run(tickAt(500), true)

	h.mustWatch(t, "B")
	if got := h.sched.liveTask(t); got.at != tickAt(850) || got.class != classEnd {
		t.Fatalf("deadline = %d class %d, want %d at end of instant", got.at, got.class, tickAt(850))
	}
	h.sched.run(tickAt(849), true)
	if obs := h.believe(t, "B"); obs.Belief != domain.BeliefHealthy {
		t.Fatalf("belief at 849 = %s, want healthy", obs.Belief)
	}
	h.sched.run(tickAt(850), true)
	if obs := h.believe(t, "B"); obs.Belief != domain.BeliefSuspected || obs.SuspectedAt != tickAt(850) {
		t.Fatalf("belief = %+v, want suspected at 850", obs)
	}
	// Never having heard from the target is enough to stop trusting it. The
	// report has no earlier evidence to measure from, so its silence runs from
	// the start of the run (850ms), not from the moment watching began (350ms).
	// Milestone 1 watches from tick zero, where the two are the same; a target
	// joined mid-run would need a "watching since" field to be exact.
	report := h.det.Reports()[0]
	if report.LastSeenAt != 0 || report.Silence != tickAt(850) {
		t.Fatalf("report = %+v, want silence measured from the start of the run", report)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("report invalid: %v", err)
	}
}
