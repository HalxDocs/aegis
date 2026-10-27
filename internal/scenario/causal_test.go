package scenario

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/causal"
	"github.com/HalxDocs/aegis/internal/domain"
)

// causalIndex is the graph of a finished run, validated from its own log.
func causalIndex(t *testing.T, res Result) *causal.Index {
	t.Helper()
	idx, err := res.Causal()
	if err != nil {
		t.Fatalf("causal index: %v", err)
	}
	return idx
}

// refOfEvent finds the reference of the only event of a kind, and fails if the
// run does not contain exactly one.
func refOfEvent(t *testing.T, res Result, kind domain.Kind) domain.Ref {
	t.Helper()
	var found domain.Ref
	for _, ev := range res.Log.Events() {
		if ev.Kind == kind {
			if found != 0 {
				t.Fatalf("%s appears more than once", kind)
			}
			found = domain.Ref(ev.Seq)
		}
	}
	if found == 0 {
		t.Fatalf("no %s in the run", kind)
	}
	return found
}

func refsOfKind(t *testing.T, res Result, kind domain.Kind) []domain.Ref {
	t.Helper()
	var out []domain.Ref
	for _, ev := range res.Log.Events() {
		if ev.Kind == kind {
			out = append(out, domain.Ref(ev.Seq))
		}
	}
	return out
}
