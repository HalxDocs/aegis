package recovery_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/recovery"
)

func TestDecisionValidateAcceptsBothOutcomes(t *testing.T) {
	granted := recovery.Decision{Granted: true, Target: "B", Witnesses: []domain.Ref{14, 15}}
	if err := granted.Validate(); err != nil {
		t.Fatalf("grant rejected: %v", err)
	}
	refused := recovery.Decision{
		Target:    "B",
		Witnesses: []domain.Ref{14},
		Reason:    domain.ReasonInsufficientWitnesses,
	}
	if err := refused.Validate(); err != nil {
		t.Fatalf("refusal rejected: %v", err)
	}
}

// The asymmetry is the point: a grant needs no excuse, a refusal needs one.
func TestDecisionRequiresAReasonExactlyWhenItRefuses(t *testing.T) {
	cases := map[string]struct {
		decision recovery.Decision
		want     string
	}{
		"refusal without a reason": {
			decision: recovery.Decision{Target: "B", Witnesses: []domain.Ref{14}},
			want:     "must say why",
		},
		"grant with a reason": {
			decision: recovery.Decision{Granted: true, Target: "B", Witnesses: []domain.Ref{14}, Reason: "because"},
			want:     "grant carries no reason",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.decision.Validate()
			if !errors.Is(err, recovery.ErrInvalidDecision) {
				t.Fatalf("error = %v, want ErrInvalidDecision", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestDecisionRequiresSortedDistinctEvidence(t *testing.T) {
	cases := map[string]recovery.Decision{
		"no evidence":      {Granted: true, Target: "B"},
		"zero reference":   {Granted: true, Target: "B", Witnesses: []domain.Ref{0}},
		"out of order":     {Granted: true, Target: "B", Witnesses: []domain.Ref{15, 14}},
		"repeated witness": {Granted: true, Target: "B", Witnesses: []domain.Ref{14, 14}},
		"bad target":       {Granted: true, Target: "bad node", Witnesses: []domain.Ref{14}},
	}
	for name, decision := range cases {
		t.Run(name, func(t *testing.T) {
			if err := decision.Validate(); !errors.Is(err, recovery.ErrInvalidDecision) {
				t.Fatalf("error = %v, want ErrInvalidDecision", err)
			}
		})
	}
}

func TestDecisionStringReadsAsAnOutcome(t *testing.T) {
	granted := recovery.Decision{Granted: true, Target: "B", Witnesses: []domain.Ref{14, 15}}
	if got := granted.String(); got != "granted for B on 2 witnesses" {
		t.Fatalf("String() = %q", got)
	}
	refused := recovery.Decision{Target: "B", Witnesses: []domain.Ref{14}, Reason: domain.ReasonInsufficientWitnesses}
	if got := refused.String(); !strings.Contains(got, "refused for B") || !strings.Contains(got, domain.ReasonInsufficientWitnesses) {
		t.Fatalf("String() = %q", got)
	}
}
