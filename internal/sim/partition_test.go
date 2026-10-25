package sim

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func mustLink(t *testing.T, from, to domain.NodeID) domain.Link {
	t.Helper()
	link, err := domain.NewLink(from, to)
	if err != nil {
		t.Fatal(err)
	}
	return link
}

// TestCutCrossesOnlyWhenTheTransitMeetsIt pins the boundary rule that decides
// whether a message survived a cut. Every case is an instant a reader of the log
// could reasonably disagree about, so the answer is written down rather than
// inferred from whichever task happened to run first.
func TestCutCrossesOnlyWhenTheTransitMeetsIt(t *testing.T) {
	c := cut{from: tickAt(300), until: tickAt(900)}
	cases := map[string]struct {
		start, end domain.Tick
		want       bool
	}{
		"well inside":               {start: tickAt(350), end: tickAt(370), want: true},
		"departs as the cut starts": {start: tickAt(300), end: tickAt(320), want: true},
		"arrives as the cut starts": {start: tickAt(280), end: tickAt(300), want: false},
		"in flight when it heals":   {start: tickAt(890), end: tickAt(910), want: true},
		"departs as it heals":       {start: tickAt(900), end: tickAt(920), want: false},
		"before the cut":            {start: tickAt(100), end: tickAt(120), want: false},
		"after the cut":             {start: tickAt(1000), end: tickAt(1020), want: false},
		"instant inside":            {start: tickAt(500), end: tickAt(500), want: true},
		"instant at the start":      {start: tickAt(300), end: tickAt(300), want: false},
		"instant at the heal":       {start: tickAt(900), end: tickAt(900), want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := c.crosses(tc.start, tc.end); got != tc.want {
				t.Fatalf("crosses(%d, %d) = %v, want %v", tc.start, tc.end, got, tc.want)
			}
		})
	}
}
