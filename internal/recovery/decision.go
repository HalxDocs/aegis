package recovery

import (
	"errors"
	"fmt"

	"github.com/HalxDocs/aegis/internal/domain"
)

// ErrInvalidDecision identifies a decision that could not be written down.
var ErrInvalidDecision = errors.New("invalid recovery decision")

// Decision is what the gate concluded about one target.
//
// A refusal carries a reason and a grant does not, and both carry the references
// that were weighed. That asymmetry is deliberate: "we acted" needs no excuse,
// while "we did nothing" needs one, and a reader who finds no reason on a
// refusal is looking at a decision that cannot be justified.
type Decision struct {
	Granted   bool
	Target    domain.NodeID
	Witnesses []domain.Ref

	// Reason is the machine-readable cause of a refusal, empty on a grant.
	Reason string
}

// Validate checks that the decision is complete in the way its outcome requires.
func (d Decision) Validate() error {
	if err := d.Target.Validate(); err != nil {
		return fmt.Errorf("%w: target: %w", ErrInvalidDecision, err)
	}
	if len(d.Witnesses) == 0 {
		return fmt.Errorf("%w: no evidence", ErrInvalidDecision)
	}
	for i, ref := range d.Witnesses {
		if !ref.Valid() {
			return fmt.Errorf("%w: witness %d is the zero reference", ErrInvalidDecision, i+1)
		}
		if i > 0 && ref <= d.Witnesses[i-1] {
			return fmt.Errorf("%w: witnesses %v are not strictly increasing", ErrInvalidDecision, d.Witnesses)
		}
	}
	if d.Granted && d.Reason != "" {
		return fmt.Errorf("%w: a grant carries no reason", ErrInvalidDecision)
	}
	if !d.Granted && d.Reason == "" {
		return fmt.Errorf("%w: a refusal must say why", ErrInvalidDecision)
	}
	return nil
}

// String renders the decision for logs and test failures.
func (d Decision) String() string {
	if d.Granted {
		return fmt.Sprintf("granted for %s on %d witnesses", d.Target, len(d.Witnesses))
	}
	return fmt.Sprintf("refused for %s on %d witnesses: %s", d.Target, len(d.Witnesses), d.Reason)
}
