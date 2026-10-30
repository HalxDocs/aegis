package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidAction identifies a recovery action that cannot be described.
var ErrInvalidAction = errors.New("invalid recovery action")

// ActionID names one recovery action in the recorded history.
//
// Identifiers are assigned once, by the component that decides to act, and are
// never reused. A later verification asks "was action 1 carried out", and that
// question only has an answer if the name refers to one decision for the whole
// run.
type ActionID uint64

// Valid reports whether the identifier names an action. Zero is the absence of
// one, so the zero value of a struct field cannot be mistaken for action zero.
func (a ActionID) Valid() bool { return a > 0 }

// String renders the identifier for logs and error messages.
func (a ActionID) String() string { return fmt.Sprintf("action %d", uint64(a)) }

// ActionKind is the closed vocabulary of things recovery may do.
//
// There is exactly one kind, and that is a decision rather than an unfinished
// list. A cut path heals on its own schedule and a crashed node never does, so a
// restart is the only failure in this model that no part of the world will
// repair by itself. An action is justified where the world cannot recover, not
// where an operator is impatient.
type ActionKind string

const (
	// ActionRestartNode brings a crashed node back into execution.
	ActionRestartNode ActionKind = "restart_node"
)

// String returns the kind as written in event logs.
func (k ActionKind) String() string { return string(k) }

// Valid reports whether k belongs to the closed vocabulary.
func (k ActionKind) Valid() bool { return k == ActionRestartNode }

// Action is one recovery decision, with the node it applies to.
//
// It contains no evidence. What justified the action lives in the event that
// proposed it, which cites the failure reports it rests on, so there is exactly
// one place to read the reasoning and no second copy to drift out of step.
type Action struct {
	ID     ActionID
	Kind   ActionKind
	Target NodeID
	At     Tick
}

// String renders the action in its log form, for example "restart_node B".
func (a Action) String() string { return a.Kind.String() + " " + a.Target.String() }

// Validate checks that the action describes something the system could do.
func (a Action) Validate() error {
	if !a.ID.Valid() {
		return fmt.Errorf("%w: identifier is zero", ErrInvalidAction)
	}
	if !a.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidAction, a.Kind)
	}
	if err := a.Target.Validate(); err != nil {
		return fmt.Errorf("%w: target: %w", ErrInvalidAction, err)
	}
	if a.At < 0 {
		return fmt.Errorf("%w: negative virtual time %d", ErrInvalidAction, a.At)
	}
	return nil
}
