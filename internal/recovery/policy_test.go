package recovery_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/recovery"
)

func validPolicy() recovery.Policy {
	return recovery.Policy{
		Kind:         domain.ActionRestartNode,
		MinWitnesses: 2,
		Patience:     domain.TickFrom(100 * time.Millisecond),
		Delay:        domain.TickFrom(50 * time.Millisecond),
	}
}

func TestPolicyValidateAcceptsAUsablePolicy(t *testing.T) {
	if err := validPolicy().Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
}

// A zero patience and a zero delay are not mistakes: they mean "decide now" and
// "act now", and the decision and the action are still two instants in the log.
func TestPolicyAllowsImmediateDecisions(t *testing.T) {
	p := validPolicy()
	p.Patience = 0
	p.Delay = 0
	if err := p.Validate(); err != nil {
		t.Fatalf("immediate policy rejected: %v", err)
	}
}

func TestPolicyRejectsUnusableThresholds(t *testing.T) {
	cases := map[string]struct {
		mutate func(*recovery.Policy)
		want   string
	}{
		"no kind":            {func(p *recovery.Policy) { p.Kind = "" }, "unknown action kind"},
		"unknown kind":       {func(p *recovery.Policy) { p.Kind = "stop_node" }, "unknown action kind"},
		"no witnesses":       {func(p *recovery.Policy) { p.MinWitnesses = 0 }, "not a threshold"},
		"negative witnesses": {func(p *recovery.Policy) { p.MinWitnesses = -1 }, "not a threshold"},
		"negative patience":  {func(p *recovery.Policy) { p.Patience = -1 }, "patience"},
		"negative delay":     {func(p *recovery.Policy) { p.Delay = -1 }, "delay"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := validPolicy()
			tc.mutate(&p)
			err := p.Validate()
			if !errors.Is(err, recovery.ErrInvalidPolicy) {
				t.Fatalf("Validate() = %v, want ErrInvalidPolicy", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestPolicyStringStatesAllThreeNumbers(t *testing.T) {
	got := validPolicy().String()
	for _, want := range []string{"restart_node", "2 witnesses", "100ms", "50ms"} {
		if !strings.Contains(got, want) {
			t.Fatalf("String() = %q, want it to mention %q", got, want)
		}
	}
}
