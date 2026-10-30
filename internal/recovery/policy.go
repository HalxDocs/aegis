package recovery

import (
	"errors"
	"fmt"

	"github.com/HalxDocs/aegis/internal/domain"
)

// ErrInvalidPolicy identifies a policy that cannot be applied to a world.
var ErrInvalidPolicy = errors.New("invalid recovery policy")

// Policy is the bar an action has to clear before it happens.
//
// The three numbers answer three different questions, and separating them is the
// whole point of the type:
//
//   - MinWitnesses: how much evidence is enough. A single observer that has
//     stopped hearing from a node has proved only that it stopped hearing from
//     it, and acting on that alone is how one wrong observer takes down a
//     healthy cluster.
//   - Patience: how long to wait for corroboration before deciding with what it
//     has. Without it, a policy whose threshold is never reached waits forever
//     and the run ends with no decision in the log at all.
//   - Delay: how long between deciding and acting. It is not decoration: it is
//     the window in which the log shows the decision before the world changes,
//     and the reason the two are not the same event.
//
// Patience and Delay may be zero, which means "decide now" and "act now". The
// gate still records the decision and the action as separate instants; a world
// cannot tell them apart, but a reader can.
type Policy struct {
	Kind         domain.ActionKind
	MinWitnesses int
	Patience     domain.Tick
	Delay        domain.Tick
}

// Validate checks that the policy could be applied to some world.
func (p Policy) Validate() error {
	if !p.Kind.Valid() {
		return fmt.Errorf("%w: unknown action kind %q", ErrInvalidPolicy, p.Kind)
	}
	if p.MinWitnesses < 1 {
		return fmt.Errorf("%w: %d witnesses is not a threshold", ErrInvalidPolicy, p.MinWitnesses)
	}
	if p.Patience < 0 {
		return fmt.Errorf("%w: patience %d is negative", ErrInvalidPolicy, p.Patience)
	}
	if p.Delay < 0 {
		return fmt.Errorf("%w: delay %d is negative", ErrInvalidPolicy, p.Delay)
	}
	return nil
}

// String renders the policy for logs and test failures.
func (p Policy) String() string {
	return fmt.Sprintf("%s after %d witnesses (patience %s, delay %s)",
		p.Kind, p.MinWitnesses, p.Patience.Duration(), p.Delay.Duration())
}
