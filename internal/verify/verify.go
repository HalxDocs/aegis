// Package verify answers questions about a finished history.
//
// Verification here is a query, not a stage. Nothing is written when an action
// succeeds, because a "verified" event would be a claim by the same run that
// performed the action, and a claim like that is worth exactly what the reader's
// trust in the claimant is worth. Instead the log already contains the proof:
// the action was proposed citing its evidence, started citing the proposal, and
// followed by a restart citing the start. This package walks those edges.
//
// It reads a causal.Index, so it can only see events that were recorded and
// only follow causes that point backwards. An action that the log does not
// connect end to end is reported as unproven, however confidently the run
// described it.
package verify

import (
	"fmt"

	"github.com/HalxDocs/aegis/internal/causal"
	"github.com/HalxDocs/aegis/internal/domain"
)

// Proof is the chain of events that shows one action was carried out.
type Proof struct {
	Action domain.ActionID

	// Proposed cites the evidence; Started carries the action out; Applied is
	// the change in ground truth that followed.
	Proposed domain.Ref
	Started  domain.Ref
	Applied  domain.Ref
}

// Action looks for the recorded chain behind one action.
//
// The search is by identifier and by edge, never by timestamp: three events in
// the same millisecond are normal, and an action is proved by which event cites
// which, not by the order they happen to share.
func Action(idx *causal.Index, id domain.ActionID) (Proof, bool, error) {
	if idx == nil {
		return Proof{}, false, fmt.Errorf("verify: no history")
	}
	if !id.Valid() {
		return Proof{}, false, fmt.Errorf("verify: %s is not an identifier", id)
	}

	proposed, ok := find(idx, func(ev domain.Event) bool {
		return ev.Kind == domain.KindRecoveryProposed && ev.Action == id
	})
	if !ok {
		return Proof{}, false, nil
	}
	started, ok := find(idx, func(ev domain.Event) bool {
		return ev.Kind == domain.KindRecoveryStarted && ev.Action == id && hasCause(ev, proposed)
	})
	if !ok {
		return Proof{Action: id, Proposed: proposed}, false, nil
	}
	applied, ok := find(idx, func(ev domain.Event) bool {
		return ev.Kind == domain.KindNodeRestarted && hasCause(ev, started)
	})
	if !ok {
		return Proof{Action: id, Proposed: proposed, Started: started}, false, nil
	}
	return Proof{Action: id, Proposed: proposed, Started: started, Applied: applied}, true, nil
}

// Actions verifies every recovery action named in the history, in the order the
// proposals were recorded.
func Actions(idx *causal.Index) ([]Proof, error) {
	if idx == nil {
		return nil, fmt.Errorf("verify: no history")
	}
	var out []Proof
	seen := make(map[domain.ActionID]struct{})
	for seq := 1; seq <= idx.Len(); seq++ {
		ev, ok := idx.Event(domain.Ref(seq))
		if !ok {
			return nil, fmt.Errorf("verify: history has no event %d", seq)
		}
		if ev.Kind != domain.KindRecoveryProposed {
			continue
		}
		if _, duplicate := seen[ev.Action]; duplicate {
			continue
		}
		seen[ev.Action] = struct{}{}
		proof, _, err := Action(idx, ev.Action)
		if err != nil {
			return nil, err
		}
		out = append(out, proof)
	}
	return out, nil
}

// find returns the first event in sequence order that matches.
func find(idx *causal.Index, match func(domain.Event) bool) (domain.Ref, bool) {
	for seq := 1; seq <= idx.Len(); seq++ {
		ref := domain.Ref(seq)
		ev, ok := idx.Event(ref)
		if !ok {
			return 0, false
		}
		if match(ev) {
			return ref, true
		}
	}
	return 0, false
}

// hasCause reports whether ev cites ref.
func hasCause(ev domain.Event, ref domain.Ref) bool {
	for _, cause := range ev.Causes {
		if cause == ref {
			return true
		}
	}
	return false
}
