package recovery_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/recovery"
)

func gate(t *testing.T, minWitnesses int) recovery.Gate {
	t.Helper()
	g, err := recovery.New(recovery.Policy{
		Kind:         domain.ActionRestartNode,
		MinWitnesses: minWitnesses,
		Patience:     domain.TickFrom(100 * time.Millisecond),
		Delay:        domain.TickFrom(50 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

func TestNewRejectsAnUnusablePolicy(t *testing.T) {
	_, err := recovery.New(recovery.Policy{Kind: domain.ActionRestartNode})
	if !errors.Is(err, recovery.ErrInvalidPolicy) {
		t.Fatalf("error = %v, want ErrInvalidPolicy", err)
	}
}

func TestPolicyIsReadableFromTheGate(t *testing.T) {
	if got := gate(t, 2).Policy().MinWitnesses; got != 2 {
		t.Fatalf("Policy().MinWitnesses = %d", got)
	}
}

// Two observers on two links are the whole point: each has only its own
// silence, and together they have enough.
func TestTwoWitnessesGrantAnAction(t *testing.T) {
	decision, err := gate(t, 2).Decide([]recovery.Witness{
		witness("A", 14),
		{Report: report("C", 575*time.Millisecond), Ref: 15},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if !decision.Granted {
		t.Fatalf("decision = %s, want a grant", decision)
	}
	if decision.Reason != "" {
		t.Fatalf("a grant carries reason %q", decision.Reason)
	}
	if want := []domain.Ref{14, 15}; !slices.Equal(decision.Witnesses, want) {
		t.Fatalf("witnesses = %v, want %v", decision.Witnesses, want)
	}
}

func TestOneWitnessIsRefusedByAThresholdOfTwo(t *testing.T) {
	decision, err := gate(t, 2).Decide([]recovery.Witness{witness("A", 14)})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if decision.Granted {
		t.Fatal("one witness granted an action that asked for two")
	}
	if decision.Reason != domain.ReasonInsufficientWitnesses {
		t.Fatalf("reason = %q", decision.Reason)
	}
	if want := []domain.Ref{14}; !slices.Equal(decision.Witnesses, want) {
		t.Fatalf("refusal cites %v, want %v", decision.Witnesses, want)
	}
}

func TestOneWitnessIsEnoughForAThresholdOfOne(t *testing.T) {
	decision, err := gate(t, 1).Decide([]recovery.Witness{witness("A", 14)})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if !decision.Granted {
		t.Fatalf("decision = %s, want a grant", decision)
	}
}

// One observer, one vote. Counting events instead of observers would let a node
// that suspected twice act alone on its own repetition.
func TestOneObserverCannotWitnessTwice(t *testing.T) {
	second := recovery.Witness{Report: report("A", 620*time.Millisecond), Ref: 21}
	_, err := gate(t, 2).Decide([]recovery.Witness{witness("A", 14), second})
	if !errors.Is(err, recovery.ErrRepeatedObserver) {
		t.Fatalf("error = %v, want ErrRepeatedObserver", err)
	}
}

func TestWitnessesMustAgreeOnATarget(t *testing.T) {
	other := recovery.Witness{Report: report("C", 575*time.Millisecond), Ref: 15}
	other.Report.Target = "D"
	// A report about D is still a valid report; only the gate can see that the
	// two witnesses answer different questions.
	_, err := gate(t, 2).Decide([]recovery.Witness{witness("A", 14), other})
	if !errors.Is(err, recovery.ErrMixedTargets) {
		t.Fatalf("error = %v, want ErrMixedTargets", err)
	}
}

func TestNoEvidenceIsAnErrorRatherThanARefusal(t *testing.T) {
	_, err := gate(t, 2).Decide(nil)
	if !errors.Is(err, recovery.ErrNoWitnesses) {
		t.Fatalf("error = %v, want ErrNoWitnesses", err)
	}
}

func TestInvalidWitnessIsRejected(t *testing.T) {
	_, err := gate(t, 2).Decide([]recovery.Witness{{Report: report("A", 570*time.Millisecond)}})
	if !errors.Is(err, recovery.ErrInvalidWitness) {
		t.Fatalf("error = %v, want ErrInvalidWitness", err)
	}
}

// The gate hands back a canonical cause list so no caller has to sort one.
func TestDecisionCitesEvidenceInIncreasingOrder(t *testing.T) {
	decision, err := gate(t, 2).Decide([]recovery.Witness{
		{Report: report("C", 575*time.Millisecond), Ref: 15},
		witness("A", 14),
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if !slices.IsSorted(decision.Witnesses) {
		t.Fatalf("witnesses = %v are not sorted", decision.Witnesses)
	}
}
