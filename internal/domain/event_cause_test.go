package domain

import (
	"errors"
	"testing"
)

// suspected returns a minimal valid event that carries causes. Suspicion is the
// event the milestone is about, so it is the one the cause rules are tested on.
func suspected(causes ...Ref) Event {
	return Event{
		At:     1,
		Kind:   KindSuspicionRaised,
		Node:   "B",
		Peer:   "A",
		Reason: ReasonHeartbeatTimeout,
		Causes: causes,
	}
}

func TestEventRejectsAZeroCause(t *testing.T) {
	err := suspected(1, 0).Validate()
	if !errors.Is(err, ErrInvalidCause) {
		t.Fatalf("error = %v, want ErrInvalidCause", err)
	}
}

func TestCausesMustBeStrictlyIncreasing(t *testing.T) {
	for name, causes := range map[string][]Ref{
		"repeated":   {4, 4},
		"descending": {5, 2},
	} {
		t.Run(name, func(t *testing.T) {
			if err := suspected(causes...).Validate(); !errors.Is(err, ErrInvalidCause) {
				t.Fatalf("error = %v, want ErrInvalidCause", err)
			}
		})
	}
	if err := suspected(2, 5, 9).Validate(); err != nil {
		t.Fatalf("ascending causes were rejected: %v", err)
	}
}

func TestASingleCauseNeedsNoOrdering(t *testing.T) {
	if err := suspected(9).Validate(); err != nil {
		t.Fatalf("one cause was rejected: %v", err)
	}
}

func TestScenarioStartCannotCiteACause(t *testing.T) {
	start := Event{At: 0, Kind: KindScenarioStarted, Scenario: "node-crash", Causes: []Ref{1}}
	if err := start.Validate(); !errors.Is(err, ErrInvalidCause) {
		t.Fatalf("error = %v, want ErrInvalidCause", err)
	}
	ok := Event{At: 0, Kind: KindScenarioStarted, Scenario: "node-crash"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("the opening event was rejected: %v", err)
	}
}

func TestCausesSurviveValidationOnEveryKind(t *testing.T) {
	for _, ev := range []Event{
		{At: 1, Kind: KindNodeStarted, Node: "A", Causes: []Ref{1}},
		{At: 1, Kind: KindNodeCrashed, Node: "A", Causes: []Ref{1}},
		{At: 1, Kind: KindHeartbeatSent, Node: "A", Peer: "B", Round: 1, Causes: []Ref{2}},
		{At: 1, Kind: KindMessageDropped, Node: "A", Peer: "B", Round: 1, Reason: ReasonPartition, Causes: []Ref{2, 3}},
		{At: 1, Kind: KindSuspicionCleared, Node: "A", Peer: "B", Causes: []Ref{4}},
	} {
		if err := ev.Validate(); err != nil {
			t.Fatalf("%s with causes was rejected: %v", ev.Kind, err)
		}
	}
}
