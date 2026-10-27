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
	events[4].Causes = []domain.Ref{4, 3}
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

func TestExplainReturnsTheChainOfReasons(t *testing.T) {
	idx, err := New(sample())
	if err != nil {
		t.Fatal(err)
	}
	exp, err := idx.Explain(5)
	if err != nil {
		t.Fatal(err)
	}
	if exp.Event.Kind != domain.KindSuspicionRaised {
		t.Fatalf("explanation is about %s, want the suspicion", exp.Event.Kind)
	}
	if exp.Depth() != 4 {
		t.Fatalf("depth = %d, want 4 edges back to the root", exp.Depth())
	}
	if exp.Count() != 5 {
		t.Fatalf("count = %d, want the five events on the chain", exp.Count())
	}
	// Every step of the way down has exactly one cause, in order.
	want := []domain.Kind{
		domain.KindSuspicionRaised, domain.KindHeartbeatReceived,
		domain.KindHeartbeatSent, domain.KindNodeStarted, domain.KindScenarioStarted,
	}
	for i, kind := range want {
		if exp.Event.Kind != kind {
			t.Fatalf("step %d is %s, want %s", i, exp.Event.Kind, kind)
		}
		if i == len(want)-1 {
			break
		}
		if len(exp.Causes) != 1 {
			t.Fatalf("step %d (%s) has %d causes, want 1", i, exp.Event.Kind, len(exp.Causes))
		}
		exp = exp.Causes[0]
	}
}

func TestExplainRejectsAReferenceThatNamesNothing(t *testing.T) {
	idx, err := New(sample())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Explain(9); !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("error = %v, want ErrUnknownRef", err)
	}
}

func TestAnExplanationCanHaveMoreThanOneParent(t *testing.T) {
	idx, err := New([]domain.Event{
		{Seq: 1, At: 0, Kind: domain.KindScenarioStarted, Scenario: "cut"},
		{Seq: 2, At: 0, Kind: domain.KindNodeStarted, Node: "A", Causes: []domain.Ref{1}},
		{Seq: 3, At: 300, Kind: domain.KindHeartbeatSent, Node: "A", Peer: "B", Round: 3, Causes: []domain.Ref{2}},
		{Seq: 4, At: 300, Kind: domain.KindLinkPartitioned, Node: "B", Peer: "A", Causes: []domain.Ref{1}},
		{Seq: 5, At: 320, Kind: domain.KindMessageDropped, Node: "A", Peer: "B", Round: 3, Reason: domain.ReasonPartition, Causes: []domain.Ref{3, 4}},
		{Seq: 6, At: 600, Kind: domain.KindScenarioFinished, Scenario: "cut", Causes: []domain.Ref{1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	exp, err := idx.Explain(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(exp.Causes) != 2 {
		t.Fatalf("the drop has %d causes, want the send and the cut", len(exp.Causes))
	}
	if exp.Causes[0].Event.Seq != 3 || exp.Causes[1].Event.Seq != 4 {
		t.Fatalf("causes are %d and %d, want 3 then 4",
			exp.Causes[0].Event.Seq, exp.Causes[1].Event.Seq)
	}
	// 5 -> {3 -> 2 -> 1, 4 -> 1}: six nodes, the root reached along two paths.
	if exp.Count() != 6 {
		t.Fatalf("count = %d, want 6 nodes counting the shared root once per path", exp.Count())
	}
	if exp.Depth() != 3 {
		t.Fatalf("depth = %d, want 3", exp.Depth())
	}
}
