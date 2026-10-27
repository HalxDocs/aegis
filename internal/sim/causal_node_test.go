package sim

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

// TestNodeStartedCitesTheScenario is the bottom of every walk backwards: a node
// begins because the scenario that opened the history started it.
func TestNodeStartedCitesTheScenario(t *testing.T) {
	e, rec := newTestEngine(t, EngineConfig{})
	if err := e.AddNode(NodeConfig{ID: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	started := rec.ofKind(domain.KindNodeStarted)
	if len(started) != 1 {
		t.Fatalf("node_started events = %d, want 1", len(started))
	}
	if want := []domain.Ref{e.Root()}; !equalRefs(started[0].Causes, want) {
		t.Fatalf("node_started causes = %v, want %v", started[0].Causes, want)
	}
}

func equalRefs(got, want []domain.Ref) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
