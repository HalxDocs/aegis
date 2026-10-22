package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidObservation identifies an inconsistent observation snapshot.
var ErrInvalidObservation = errors.New("invalid observation")

// Belief is what one observer currently believes about one target. It is not a
// node state: a crashed node can be believed healthy, and a running node can be
// suspected. Keeping the two dimensions in separate types is what stops the
// detector from ever reading ground truth.
type Belief string

const (
	// BeliefHealthy means the observer has evidence recent enough to trust.
	BeliefHealthy Belief = "healthy"

	// BeliefSuspected means the observer has waited past its timeout without
	// evidence. It does not mean the target has failed.
	BeliefSuspected Belief = "suspected"
)

// String returns the belief as written in event logs.
func (b Belief) String() string { return string(b) }

// Valid reports whether b is a known belief.
func (b Belief) Valid() bool {
	return b == BeliefHealthy || b == BeliefSuspected
}

// CanTransitionTo reports whether an observation may move from b to next.
//
// Both directions are allowed: silence creates suspicion, and fresh evidence
// clears it. Repeating the current belief is rejected so a no-op cannot appear
// in the event history as if something happened.
func (b Belief) CanTransitionTo(next Belief) bool {
	if b == next {
		return false
	}
	return (b == BeliefHealthy && next == BeliefSuspected) ||
		(b == BeliefSuspected && next == BeliefHealthy)
}

// TransitionTo validates a belief change.
func (b Belief) TransitionTo(next Belief) (Belief, error) {
	if !b.Valid() || !b.CanTransitionTo(next) {
		return b, &TransitionError{Subject: "belief", From: string(b), To: string(next)}
	}
	return next, nil
}

// Observation is everything one observer knows about one target at a virtual
// instant. It contains no ground truth: LastSeen is when a message was actually
// received, and Belief is the conclusion the observer drew from that.
//
// Observations are immutable values. Observed and Suspect return updated copies
// so a scheduler callback cannot mutate state an in-flight report still refers
// to.
type Observation struct {
	Observer NodeID
	Target   NodeID
	Belief   Belief

	// LastSeen is the virtual time of the most recent message accepted from the
	// target, or zero if none has arrived yet.
	LastSeen Tick

	// SuspectedAt is the virtual time suspicion was raised, or zero while the
	// observer is healthy.
	SuspectedAt Tick

	// Heard counts messages accepted from the target for the whole run. It is
	// audit evidence: it shows whether an observer has ever heard from a node.
	Heard uint64
}

// NewObservation returns a healthy observation with no evidence.
func NewObservation(observer, target NodeID) Observation {
	return Observation{Observer: observer, Target: target, Belief: BeliefHealthy}
}

// Observed records that a message from the target arrived at the given virtual
// time and returns the updated observation.
//
// A suspected observer returns to healthy on the first message it receives,
// because suspicion is a statement about silence, not about the target.
func (o Observation) Observed(at Tick) (Observation, error) {
	if err := o.validatePeers(); err != nil {
		return o, err
	}
	if at < o.LastSeen {
		return o, fmt.Errorf("%w: message at %d precedes last seen %d", ErrInvalidObservation, at, o.LastSeen)
	}
	belief := o.Belief
	if belief == BeliefSuspected {
		next, err := belief.TransitionTo(BeliefHealthy)
		if err != nil {
			return o, err
		}
		belief = next
	}
	if !belief.Valid() {
		return o, fmt.Errorf("%w: unknown belief %q", ErrInvalidObservation, o.Belief)
	}
	return Observation{
		Observer: o.Observer,
		Target:   o.Target,
		Belief:   belief,
		LastSeen: at,
		Heard:    o.Heard + 1,
	}, nil
}

// Suspect records that the observer stopped trusting the target at the given
// virtual time and returns the updated observation.
func (o Observation) Suspect(at Tick) (Observation, error) {
	if err := o.validatePeers(); err != nil {
		return o, err
	}
	next, err := o.Belief.TransitionTo(BeliefSuspected)
	if err != nil {
		return o, err
	}
	if at <= o.LastSeen {
		return o, fmt.Errorf("%w: suspicion at %d is not after last seen %d", ErrInvalidObservation, at, o.LastSeen)
	}
	o.Belief = next
	o.SuspectedAt = at
	return o, nil
}

// SilentFor returns how long the observer has been without evidence at the
// given virtual time. It reports zero rather than a negative duration when the
// time precedes the last sighting.
func (o Observation) SilentFor(at Tick) Tick {
	if at < o.LastSeen {
		return 0
	}
	return at - o.LastSeen
}

// Validate checks the internal consistency of the snapshot.
func (o Observation) Validate() error {
	if err := o.validatePeers(); err != nil {
		return err
	}
	if !o.Belief.Valid() {
		return fmt.Errorf("%w: unknown belief %q", ErrInvalidObservation, o.Belief)
	}
	if o.LastSeen < 0 {
		return fmt.Errorf("%w: last seen %d is negative", ErrInvalidObservation, o.LastSeen)
	}
	switch o.Belief {
	case BeliefSuspected:
		if o.SuspectedAt <= o.LastSeen {
			return fmt.Errorf("%w: suspicion at %d is not after last seen %d", ErrInvalidObservation, o.SuspectedAt, o.LastSeen)
		}
	case BeliefHealthy:
		if o.SuspectedAt != 0 {
			return fmt.Errorf("%w: healthy observation carries suspicion time %d", ErrInvalidObservation, o.SuspectedAt)
		}
	}
	return nil
}

func (o Observation) validatePeers() error {
	if err := o.Observer.Validate(); err != nil {
		return fmt.Errorf("%w: observer: %w", ErrInvalidObservation, err)
	}
	if err := o.Target.Validate(); err != nil {
		return fmt.Errorf("%w: target: %w", ErrInvalidObservation, err)
	}
	if o.Observer == o.Target {
		return fmt.Errorf("%w: %s cannot observe itself", ErrInvalidObservation, o.Observer)
	}
	return nil
}
