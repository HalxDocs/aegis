package causal

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

// sample is a small history with the shape the milestone cares about: a node
// starts, sends, is heard, and is then suspected.
func sample() []domain.Event {
	return []domain.Event{
		{Seq: 1, At: 0, Kind: domain.KindScenarioStarted, Scenario: "sample"},
		{Seq: 2, At: 0, Kind: domain.KindNodeStarted, Node: "A", Causes: []domain.Ref{1}},
		{Seq: 3, At: 100, Kind: domain.KindHeartbeatSent, Node: "A", Peer: "B", Round: 1, Causes: []domain.Ref{2}},
		{Seq: 4, At: 120, Kind: domain.KindHeartbeatReceived, Node: "A", Peer: "B", Round: 1, Causes: []domain.Ref{3}},
		{Seq: 5, At: 470, Kind: domain.KindSuspicionRaised, Node: "B", Peer: "A", Reason: domain.ReasonHeartbeatTimeout, Causes: []domain.Ref{4}},
		{Seq: 6, At: 600, Kind: domain.KindScenarioFinished, Scenario: "sample", Causes: []domain.Ref{1}},
	}
}

func TestNewIndexesAWellFormedHistory(t *testing.T) {
	idx, err := New(sample())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if idx.Len() != 6 {
		t.Fatalf("Len() = %d, want 6", idx.Len())
	}
	if idx.Root() != domain.Ref(1) {
		t.Fatalf("Root() = %s, want 1", idx.Root())
	}
	ev, ok := idx.Event(3)
	if !ok || ev.Kind != domain.KindHeartbeatSent {
		t.Fatalf("Event(3) = %+v, %v; want the send", ev, ok)
	}
	if _, ok := idx.Event(7); ok {
		t.Fatal("Event(7) resolved to nothing in a six-event history")
	}
}

func TestNewRejectsAnEmptyHistory(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, ErrEmptyHistory) {
		t.Fatalf("error = %v, want ErrEmptyHistory", err)
	}
}

func TestNewRejectsAHistoryThatIsNotNumberedFromOne(t *testing.T) {
	for name, mutate := range map[string]func([]domain.Event){
		"gap":     func(e []domain.Event) { e[3].Seq = 9 },
		"restart": func(e []domain.Event) { e[3].Seq = 3 },
		"zero":    func(e []domain.Event) { e[0].Seq = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			events := sample()
			mutate(events)
			if _, err := New(events); !errors.Is(err, ErrBadSequence) {
				t.Fatalf("error = %v, want ErrBadSequence", err)
			}
		})
	}
}

func TestNewRejectsAHistoryThatDoesNotOpenWithTheScenario(t *testing.T) {
	events := sample()
	events[0].Kind = domain.KindNodeStarted
	events[0].Node = "A"
	events[0].Scenario = ""
	if _, err := New(events); !errors.Is(err, ErrNoRoot) {
		t.Fatalf("error = %v, want ErrNoRoot", err)
	}
}

func TestNewRejectsAnEventWithNoCause(t *testing.T) {
	events := sample()
	events[2].Causes = nil
	if _, err := New(events); !errors.Is(err, ErrUncaused) {
		t.Fatalf("error = %v, want ErrUncaused", err)
	}
}

func TestNewRejectsCausesOutOfOrder(t *testing.T) {
	events := sample()
	events[4].Causes = []domain.Ref{3, 4}
	if _, err := New(events); !errors.Is(err, ErrUnsortedCauses) {
		t.Fatalf("error = %v, want ErrUnsortedCauses", err)
	}
}

func TestNewRejectsACauseOutsideTheHistory(t *testing.T) {
	events := sample()
	events[2].Causes = []domain.Ref{9}
	if _, err := New(events); !errors.Is(err, ErrDanglingCause) {
		t.Fatalf("error = %v, want ErrDanglingCause", err)
	}
}

func TestNewRejectsAnEventThatCitesItself(t *testing.T) {
	events := sample()
	events[2].Causes = []domain.Ref{3}
	if _, err := New(events); !errors.Is(err, ErrCauseOrder) {
		t.Fatalf("error = %v, want ErrCauseOrder", err)
	}
}

func TestNewRejectsAnEventThatCitesTheFuture(t *testing.T) {
	events := sample()
	events[1].Causes = []domain.Ref{4}
	if _, err := New(events); !errors.Is(err, ErrCauseOrder) {
		t.Fatalf("error = %v, want ErrCauseOrder for a reference ahead of the event", err)
	}
}

func TestNewRejectsACauseDatestampedAfterItsEffect(t *testing.T) {
	events := sample()
	// The reference is legitimately earlier, but the timestamp is not.
	events[1].Causes = []domain.Ref{1}
	events[0].At = 50
	if _, err := New(events); !errors.Is(err, ErrFutureCause) {
		t.Fatalf("error = %v, want ErrFutureCause", err)
	}
}

func TestAncestorsWalkBackToTheRoot(t *testing.T) {
	idx, err := New(sample())
	if err != nil {
		t.Fatal(err)
	}
	got, err := idx.Ancestors(5)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Ref{1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("ancestors = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ancestors = %v, want %v", got, want)
		}
	}
	if got, err := idx.Ancestors(1); err != nil || len(got) != 0 {
		t.Fatalf("the root has ancestors %v (err %v), want none", got, err)
	}
	if _, err := idx.Ancestors(9); !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("error = %v, want ErrUnknownRef", err)
	}
}

func TestDescendantsAreTheEventsOneRefExplains(t *testing.T) {
	idx, err := New(sample())
	if err != nil {
		t.Fatal(err)
	}
	got, err := idx.Descendants(1)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Ref{2, 3, 4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("descendants of the root = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("descendants of the root = %v, want %v", got, want)
		}
	}
	got, err = idx.Descendants(4)
	if err != nil || len(got) != 1 || got[0] != 5 {
		t.Fatalf("descendants of the receipt = %v (err %v), want the suspicion alone", got, err)
	}
	if _, err := idx.Descendants(0); !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("error = %v, want ErrUnknownRef for the zero reference", err)
	}
}
