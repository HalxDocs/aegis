package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
)

func tick(ms int64) domain.Tick {
	return domain.TickFrom(time.Duration(ms) * time.Millisecond)
}

func TestBeliefTransitions(t *testing.T) {
	cases := []struct {
		name    string
		from    domain.Belief
		to      domain.Belief
		wantErr bool
	}{
		{"healthy to suspected", domain.BeliefHealthy, domain.BeliefSuspected, false},
		{"suspected to healthy", domain.BeliefSuspected, domain.BeliefHealthy, false},
		{"healthy to healthy", domain.BeliefHealthy, domain.BeliefHealthy, true},
		{"suspected to suspected", domain.BeliefSuspected, domain.BeliefSuspected, true},
		{"unknown to healthy", domain.Belief("guessing"), domain.BeliefHealthy, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.from.TransitionTo(tc.to)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrInvalidTransition) {
					t.Fatalf("error = %v, want ErrInvalidTransition", err)
				}
				var transition *domain.TransitionError
				if !errors.As(err, &transition) || transition.Subject != "belief" {
					t.Fatalf("error %v is not a belief TransitionError", err)
				}
				return
			}
			if err != nil || got != tc.to {
				t.Fatalf("TransitionTo(%q) = %q, %v", tc.to, got, err)
			}
		})
	}
}

func TestNewObservationStartsHealthyAndDeaf(t *testing.T) {
	obs := domain.NewObservation("A", "B")
	if obs.Belief != domain.BeliefHealthy {
		t.Fatalf("belief = %q, want healthy", obs.Belief)
	}
	if obs.SuspectedAt != 0 || obs.LastSeen != 0 || obs.Heard != 0 {
		t.Fatalf("new observation carries evidence: %+v", obs)
	}
	if err := obs.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestObservedRecordsEvidence(t *testing.T) {
	obs := domain.NewObservation("A", "B")

	obs, err := obs.Observed(tick(120))
	if err != nil {
		t.Fatalf("Observed = %v", err)
	}
	if obs.LastSeen != tick(120) || obs.Heard != 1 {
		t.Fatalf("after first message: %+v", obs)
	}

	obs, err = obs.Observed(tick(220))
	if err != nil {
		t.Fatalf("Observed = %v", err)
	}
	if obs.LastSeen != tick(220) || obs.Heard != 2 {
		t.Fatalf("after second message: %+v", obs)
	}
}

func TestObservationClearsSuspicionOnEvidence(t *testing.T) {
	obs := domain.NewObservation("A", "B")
	obs, err := obs.Observed(tick(120))
	if err != nil {
		t.Fatal(err)
	}
	obs, err = obs.Suspect(tick(470))
	if err != nil {
		t.Fatal(err)
	}
	if obs.Belief != domain.BeliefSuspected || obs.SuspectedAt != tick(470) {
		t.Fatalf("not suspected: %+v", obs)
	}

	obs, err = obs.Observed(tick(500))
	if err != nil {
		t.Fatalf("Observed = %v", err)
	}
	if obs.Belief != domain.BeliefHealthy {
		t.Fatalf("belief = %q, want healthy", obs.Belief)
	}
	if obs.SuspectedAt != 0 {
		t.Fatalf("stale suspicion time %d survived recovery", obs.SuspectedAt)
	}
	if obs.Heard != 2 {
		t.Fatalf("Heard = %d, want 2", obs.Heard)
	}
}

func TestSuspectRejectsRepeatAndTimeTravel(t *testing.T) {
	obs := domain.NewObservation("A", "B")
	obs, err := obs.Observed(tick(120))
	if err != nil {
		t.Fatal(err)
	}
	obs, err = obs.Suspect(tick(470))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obs.Suspect(tick(500)); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("second suspicion error = %v, want ErrInvalidTransition", err)
	}

	fresh := domain.NewObservation("A", "B")
	if _, err := fresh.Suspect(0); !errors.Is(err, domain.ErrInvalidObservation) {
		t.Fatalf("suspicion at last-seen time error = %v, want ErrInvalidObservation", err)
	}
}

func TestObservedRejectsBackwardsTime(t *testing.T) {
	obs := domain.NewObservation("A", "B")
	obs, err := obs.Observed(tick(200))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obs.Observed(tick(100)); !errors.Is(err, domain.ErrInvalidObservation) {
		t.Fatalf("backwards message error = %v, want ErrInvalidObservation", err)
	}
}

func TestObservationRejectsSelfObservation(t *testing.T) {
	obs := domain.NewObservation("A", "A")
	if err := obs.Validate(); !errors.Is(err, domain.ErrInvalidObservation) {
		t.Fatalf("Validate() = %v, want ErrInvalidObservation", err)
	}
	if _, err := obs.Observed(tick(1)); err == nil {
		t.Fatal("Observed on self-observation must fail")
	}
}

func TestObservationValidateRejectsInconsistency(t *testing.T) {
	cases := map[string]domain.Observation{
		"unknown belief": {
			Observer: "A", Target: "B", Belief: domain.Belief("guessing"),
		},
		"healthy with suspicion time": {
			Observer: "A", Target: "B", Belief: domain.BeliefHealthy, SuspectedAt: tick(470),
		},
		"suspected before evidence": {
			Observer: "A", Target: "B", Belief: domain.BeliefSuspected, SuspectedAt: 0,
		},
		"negative last seen": {
			Observer: "A", Target: "B", Belief: domain.BeliefHealthy, LastSeen: -1,
		},
	}
	for name, obs := range cases {
		t.Run(name, func(t *testing.T) {
			if err := obs.Validate(); !errors.Is(err, domain.ErrInvalidObservation) {
				t.Fatalf("Validate() = %v, want ErrInvalidObservation", err)
			}
		})
	}
}

func TestSilentFor(t *testing.T) {
	obs := domain.NewObservation("A", "B")
	obs, err := obs.Observed(tick(220))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := obs.SilentFor(tick(570)), tick(350); got != want {
		t.Fatalf("SilentFor = %d, want %d", got, want)
	}
	if got := obs.SilentFor(tick(100)); got != 0 {
		t.Fatalf("SilentFor before evidence = %d, want 0", got)
	}
}
