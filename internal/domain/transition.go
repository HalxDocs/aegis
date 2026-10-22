package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidTransition identifies any rejected state change. Callers test for
// it with errors.Is and inspect the concrete error for the offending states.
var ErrInvalidTransition = errors.New("invalid transition")

// TransitionError reports a rejected change between two states of one
// dimension. Subject distinguishes a node's execution state from an observer's
// belief, so both dimensions share one error type instead of forcing callers to
// match on message strings.
type TransitionError struct {
	Subject string
	From    string
	To      string
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("%s: cannot move from %q to %q", e.Subject, e.From, e.To)
}

// Unwrap lets errors.Is(err, ErrInvalidTransition) succeed.
func (e *TransitionError) Unwrap() error { return ErrInvalidTransition }
