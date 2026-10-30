package recovery

import (
	"errors"
	"fmt"
	"slices"

	"github.com/HalxDocs/aegis/internal/domain"
)

var (
	// ErrNoWitnesses is returned when a decision is asked for with no evidence.
	// It is an error rather than a refusal because a refusal that cites nothing
	// cannot be written into the history, and a decision the log cannot hold is
	// not a decision the system made.
	ErrNoWitnesses = errors.New("a decision requires at least one witness")

	// ErrMixedTargets is returned when the witnesses are not about the same
	// node. Two reports about two nodes are two questions, and answering either
	// with both would let one node's evidence justify acting on another.
	ErrMixedTargets = errors.New("witnesses name different targets")

	// ErrRepeatedObserver is returned when one observer appears twice. One
	// observer, one vote: a single node that has grown suspicious of the same
	// peer twice has still only proved its own silence, and a gate that counted
	// events instead of observers would let one paranoid node act alone.
	ErrRepeatedObserver = errors.New("one observer cannot witness twice")
)

// Gate applies a policy to a set of witnesses.
//
// It holds no state and reads no clock. Given the same witnesses and the same
// policy it always returns the same decision, which is what makes a run
// replayable and a decision reviewable after the fact.
type Gate struct {
	policy Policy
}

// New validates a policy and returns a gate that applies it.
func New(p Policy) (Gate, error) {
	if err := p.Validate(); err != nil {
		return Gate{}, err
	}
	return Gate{policy: p}, nil
}

// Policy returns the policy this gate applies.
func (g Gate) Policy() Policy { return g.policy }

// Decide weighs the witnesses and returns a decision.
//
// The decision is not the action: a granted decision says the evidence is
// sufficient, and something else has to carry it out. That separation is what
// lets a run record the decision before the world changes, so the log shows the
// reasoning and its consequence as two events rather than one.
func (g Gate) Decide(witnesses []Witness) (Decision, error) {
	if len(witnesses) == 0 {
		return Decision{}, ErrNoWitnesses
	}

	refs := make([]domain.Ref, 0, len(witnesses))
	seen := make(map[domain.NodeID]struct{}, len(witnesses))
	var target domain.NodeID
	for i, w := range witnesses {
		if err := w.Validate(); err != nil {
			return Decision{}, fmt.Errorf("witness %d: %w", i+1, err)
		}
		if i == 0 {
			target = w.Target()
		} else if w.Target() != target {
			return Decision{}, fmt.Errorf("%w: %s and %s", ErrMixedTargets, target, w.Target())
		}
		if _, repeat := seen[w.Report.Observer]; repeat {
			return Decision{}, fmt.Errorf("%w: %s reported on %s more than once",
				ErrRepeatedObserver, w.Report.Observer, target)
		}
		seen[w.Report.Observer] = struct{}{}
		refs = append(refs, w.Ref)
	}

	// The recorder refuses a cause list that is not strictly increasing, so the
	// gate hands back a canonical one rather than leaving each caller to sort.
	slices.Sort(refs)

	decision := Decision{
		Target:    target,
		Witnesses: refs,
		Granted:   len(refs) >= g.policy.MinWitnesses,
	}
	if !decision.Granted {
		decision.Reason = domain.ReasonInsufficientWitnesses
	}
	if err := decision.Validate(); err != nil {
		return Decision{}, err
	}
	return decision, nil
}
