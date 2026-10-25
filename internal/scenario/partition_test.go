package scenario

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

// TestOneWayPartitionBreaksNobodyAndStillFoolsSomebody is the milestone's claim:
// a whole run in which every node is running, nobody is unreachable by everyone,
// and one observer is still sure something is wrong.
func TestOneWayPartitionBreaksNobodyAndStillFoolsSomebody(t *testing.T) {
	res := mustRun(t, OneWayPartition(), 1)

	for _, id := range []domain.NodeID{"A", "B", "C"} {
		if got := res.Truth[id]; got != domain.StateRunning {
			t.Fatalf("truth for %s = %s, want running: nothing in this world failed", id, got)
		}
	}

	reports := res.ReportsFor("A")
	if len(reports) != 1 {
		t.Fatalf("reports from A = %d, want exactly one false suspicion", len(reports))
	}
	want := domain.FailureReport{
		Observer:    "A",
		Target:      "B",
		SuspectedAt: tick(570),
		LastSeenAt:  tick(220),
		Timeout:     tick(350),
		Silence:     tick(350),
	}
	if reports[0] != want {
		t.Fatalf("report = %+v, want %+v", reports[0], want)
	}
	if len(res.Reports) != 1 {
		t.Fatalf("reports = %d, want only A's: B and C see a working path", len(res.Reports))
	}

	// The cut runs B->A only, so the far ends of it disagree: B keeps hearing A
	// and never doubts it, and C keeps hearing B on its own link.
	for _, observer := range []domain.NodeID{"B", "C"} {
		if n := len(res.ReportsFor(observer)); n != 0 {
			t.Fatalf("%s produced %d reports, want none", observer, n)
		}
	}
	if obs, ok := res.Belief("B", "A"); !ok || obs.Belief != domain.BeliefHealthy {
		t.Fatalf("B's belief about A = %+v, want healthy: that direction is never cut", obs)
	}
	if obs, ok := res.Belief("C", "B"); !ok || obs.Belief != domain.BeliefHealthy || obs.Heard != 11 {
		t.Fatalf("C's belief about B = %+v, want healthy after every round", obs)
	}

	// A ends the run trusting B again. The report is history, not a verdict.
	obs, ok := res.Belief("A", "B")
	if !ok || obs.Belief != domain.BeliefHealthy {
		t.Fatalf("A's final belief = %+v, want healthy after the repair", obs)
	}
	if obs.LastSeen != tick(1120) {
		t.Fatalf("A last saw B at %d, want the final round", obs.LastSeen)
	}
}
