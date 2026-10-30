package recovery_test

import (
	"errors"
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/recovery"
)

// report returns the canonical suspicion of B by observer, with the evidence
// timings the node-crash world produces.
func report(observer domain.NodeID, suspectedAt time.Duration) domain.FailureReport {
	r := domain.FailureReport{
		Observer:    observer,
		Target:      "B",
		SuspectedAt: domain.TickFrom(suspectedAt),
		LastSeenAt:  domain.TickFrom(220 * time.Millisecond),
		Timeout:     domain.TickFrom(350 * time.Millisecond),
	}
	r.Silence = r.SuspectedAt - r.LastSeenAt
	return r
}

func witness(observer domain.NodeID, ref domain.Ref) recovery.Witness {
	return recovery.Witness{Report: report(observer, 570*time.Millisecond), Ref: ref}
}

func TestWitnessValidateAcceptsARecordedReport(t *testing.T) {
	if err := witness("A", 14).Validate(); err != nil {
		t.Fatalf("valid witness rejected: %v", err)
	}
}

func TestWitnessRequiresAReference(t *testing.T) {
	err := witness("A", 0).Validate()
	if !errors.Is(err, recovery.ErrInvalidWitness) {
		t.Fatalf("error = %v, want ErrInvalidWitness", err)
	}
}

func TestWitnessRejectsAnInconsistentReport(t *testing.T) {
	w := witness("A", 14)
	// A report that claims suspicion before its own timeout is the detector
	// lying about its evidence; a gate must not accept it as a reason to act.
	w.Report.Silence = domain.TickFrom(10 * time.Millisecond)
	if err := w.Validate(); !errors.Is(err, recovery.ErrInvalidWitness) {
		t.Fatalf("error = %v, want ErrInvalidWitness", err)
	}
}

func TestWitnessNamesItsTarget(t *testing.T) {
	if got := witness("A", 14).Target(); got != "B" {
		t.Fatalf("Target() = %q", got)
	}
}

func TestWitnessStringNamesTargetObserverAndLine(t *testing.T) {
	got := witness("A", 14).String()
	if got != "B by A (#14)" {
		t.Fatalf("String() = %q", got)
	}
}
