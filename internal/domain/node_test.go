package domain_test

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestNodeStateTransitions(t *testing.T) {
	cases := []struct {
		name    string
		from    domain.NodeState
		to      domain.NodeState
		wantErr bool
	}{
		{"running to crashed", domain.StateRunning, domain.StateCrashed, false},
		{"running to running", domain.StateRunning, domain.StateRunning, true},
		{"crashed to crashed", domain.StateCrashed, domain.StateCrashed, true},
		{"crashed to running", domain.StateCrashed, domain.StateRunning, true},
		{"unknown to running", domain.NodeState("exploded"), domain.StateRunning, true},
		{"running to unknown", domain.StateRunning, domain.NodeState("exploded"), true},
		{"zero value to crashed", domain.NodeState(""), domain.StateCrashed, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.from.TransitionTo(tc.to)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrInvalidTransition) {
					t.Fatalf("TransitionTo(%q) error = %v, want ErrInvalidTransition", tc.to, err)
				}
				if got != tc.from {
					t.Fatalf("rejected transition changed state to %q", got)
				}
				var transition *domain.TransitionError
				if !errors.As(err, &transition) {
					t.Fatalf("error %v is not a *TransitionError", err)
				}
				if transition.Subject != "node" || transition.From != string(tc.from) || transition.To != string(tc.to) {
					t.Fatalf("error fields = %+v", transition)
				}
				return
			}
			if err != nil {
				t.Fatalf("TransitionTo(%q) = %v, want nil", tc.to, err)
			}
			if got != tc.to {
				t.Fatalf("TransitionTo(%q) = %q", tc.to, got)
			}
		})
	}
}

func TestNodeStateValidity(t *testing.T) {
	if !domain.StateRunning.Valid() || !domain.StateCrashed.Valid() {
		t.Fatal("declared states must be valid")
	}
	if domain.NodeState("").Valid() || domain.NodeState("stopped").Valid() {
		t.Fatal("undeclared states must be invalid")
	}
}
