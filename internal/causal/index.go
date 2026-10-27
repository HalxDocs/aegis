// Package causal reads a recorded history as a graph of reasons.
//
// It imports internal/domain and the standard library, and nothing else. That is
// the point: an explainer that could reach the simulator would be able to claim
// knowledge no observer had, which is exactly the confusion the project exists to
// remove. Here the log is the only input, so every explanation is made of events
// that were written down.
//
// The graph is a tree in all but name: references only point backwards, so
// following causes always terminates, and reachability of every event from the
// root is a consequence of the rules rather than a property to be checked —
// an event numbered n can only cite events numbered below n, and event one cites
// nothing, so every walk ends there.
package causal

import (
	"errors"
	"fmt"
	"slices"

	"github.com/HalxDocs/aegis/internal/domain"
)

var (
	// ErrEmptyHistory is returned for a log with no events in it.
	ErrEmptyHistory = errors.New("history is empty")

	// ErrBadSequence is returned when the events are not numbered from one
	// without gaps or reordering.
	ErrBadSequence = errors.New("history is not numbered from one without gaps")

	// ErrNoRoot is returned when the first event is not the opening event.
	ErrNoRoot = errors.New("history does not open with scenario_started")

	// ErrUncaused is returned for an event after the root that cites nothing.
	ErrUncaused = errors.New("event cites no cause")

	// ErrDanglingCause is returned when a cause names an event the history does
	// not contain.
	ErrDanglingCause = errors.New("cause names an event outside the history")

	// ErrCauseOrder is returned when a cause is recorded after the event that
	// cites it.
	ErrCauseOrder = errors.New("cause is not recorded before its effect")

	// ErrUnsortedCauses is returned when a cause list is not strictly
	// increasing.
	ErrUnsortedCauses = errors.New("causes are not in increasing order")

	// ErrFutureCause is returned when a cause carries a later timestamp than
	// the event it explains.
	ErrFutureCause = errors.New("cause happens after its effect")

	// ErrUnknownRef is returned when a reference names no event.
	ErrUnknownRef = errors.New("no such event in the history")
)

// Index is a history addressed by reference, with the edges pointing backwards.
type Index struct {
	events   map[domain.Ref]domain.Event
	order    []domain.Ref
	children map[domain.Ref][]domain.Ref
	root     domain.Ref
}

// New validates a history and returns an index over it.
//
// Validation is not defensive decoration: a log can arrive from a file, and a
// graph with a forward edge or a missing cause produces an explanation that
// looks plausible and is wrong. Every rule here is one that would let such an
// explanation be built.
func New(events []domain.Event) (*Index, error) {
	if len(events) == 0 {
		return nil, ErrEmptyHistory
	}
	idx := &Index{
		events:   make(map[domain.Ref]domain.Event, len(events)),
		order:    make([]domain.Ref, 0, len(events)),
		children: make(map[domain.Ref][]domain.Ref, len(events)),
	}
	for i, ev := range events {
		if ev.Seq != uint64(i+1) {
			return nil, fmt.Errorf("%w: position %d holds sequence %d", ErrBadSequence, i+1, ev.Seq)
		}
		ref := domain.Ref(ev.Seq)
		idx.events[ref] = ev
		idx.order = append(idx.order, ref)
	}
	idx.root = idx.order[0]
	if got := idx.events[idx.root].Kind; got != domain.KindScenarioStarted {
		return nil, fmt.Errorf("%w: the first event is %s", ErrNoRoot, got)
	}

	for _, ref := range idx.order[1:] {
		ev := idx.events[ref]
		if len(ev.Causes) == 0 {
			return nil, fmt.Errorf("%w: %s at %d", ErrUncaused, ev.Kind, ev.At)
		}
		for i, cause := range ev.Causes {
			if i > 0 && cause <= ev.Causes[i-1] {
				return nil, fmt.Errorf("%w: %s cites %v", ErrUnsortedCauses, ev.Kind, ev.Causes)
			}
			target, ok := idx.events[cause]
			if !ok {
				return nil, fmt.Errorf("%w: %s cites %s", ErrDanglingCause, ev.Kind, cause)
			}
			if cause >= ref {
				return nil, fmt.Errorf("%w: %s cites %s", ErrCauseOrder, ev.Kind, cause)
			}
			if target.At > ev.At {
				return nil, fmt.Errorf("%w: %s at %d cites %s at %d",
					ErrFutureCause, ev.Kind, ev.At, target.Kind, target.At)
			}
			idx.children[cause] = append(idx.children[cause], ref)
		}
	}
	return idx, nil
}

// Root returns the reference of the event that opened the history.
func (i *Index) Root() domain.Ref { return i.root }

// Len returns the number of events in the history.
func (i *Index) Len() int { return len(i.order) }

// Event returns the event with the given reference.
func (i *Index) Event(ref domain.Ref) (domain.Event, bool) {
	ev, ok := i.events[ref]
	return ev, ok
}

// Ancestors returns every event ref is descended from, in increasing order.
//
// The event itself is not included: an ancestry is the set of reasons, not the
// thing they justify.
func (i *Index) Ancestors(ref domain.Ref) ([]domain.Ref, error) {
	if _, ok := i.events[ref]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownRef, ref)
	}
	seen := make(map[domain.Ref]struct{})
	var walk func(domain.Ref)
	walk = func(at domain.Ref) {
		for _, cause := range i.events[at].Causes {
			if _, ok := seen[cause]; ok {
				continue
			}
			seen[cause] = struct{}{}
			walk(cause)
		}
	}
	walk(ref)
	out := make([]domain.Ref, 0, len(seen))
	for cause := range seen {
		out = append(out, cause)
	}
	slices.Sort(out)
	return out, nil
}
