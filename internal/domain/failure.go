package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidReport identifies an inconsistent failure report.
var ErrInvalidReport = errors.New("invalid failure report")

// FailureReport is the evidence an observer has when it stops trusting a
// target. It is a record of belief, not a verdict about the target: the node it
// names may be running perfectly well behind a broken link.
//
// The fields are typed rather than prose so later stages can reason over them
// without parsing text, and so the report can be compared in tests.
type FailureReport struct {
	Observer    NodeID
	Target      NodeID
	SuspectedAt Tick
	LastSeenAt  Tick
	Timeout     Tick
	Silence     Tick
}

// NewFailureReport derives a report from a suspected observation.
//
// It rejects an observation that is not suspected, a non-positive timeout, and
// a silence shorter than the timeout. The last rule is the "never suspect early"
// invariant: a report that claims suspicion before the deadline is a bug in the
// detector, and it must not become evidence.
func NewFailureReport(obs Observation, timeout Tick) (FailureReport, error) {
	if timeout <= 0 {
		return FailureReport{}, fmt.Errorf("%w: timeout %d is not positive", ErrInvalidReport, timeout)
	}
	if err := obs.Validate(); err != nil {
		return FailureReport{}, fmt.Errorf("%w: %w", ErrInvalidReport, err)
	}
	if obs.Belief != BeliefSuspected {
		return FailureReport{}, fmt.Errorf("%w: %s believes %s is %q, not %q",
			ErrInvalidReport, obs.Observer, obs.Target, obs.Belief, BeliefSuspected)
	}
	silence := obs.SuspectedAt - obs.LastSeen
	if silence < timeout {
		return FailureReport{}, fmt.Errorf("%w: %s suspected %s after %d of silence, before its %d timeout",
			ErrInvalidReport, obs.Observer, obs.Target, silence, timeout)
	}
	return FailureReport{
		Observer:    obs.Observer,
		Target:      obs.Target,
		SuspectedAt: obs.SuspectedAt,
		LastSeenAt:  obs.LastSeen,
		Timeout:     timeout,
		Silence:     silence,
	}, nil
}

// Validate checks that the report is internally consistent.
func (r FailureReport) Validate() error {
	if err := r.Observer.Validate(); err != nil {
		return fmt.Errorf("%w: observer: %w", ErrInvalidReport, err)
	}
	if err := r.Target.Validate(); err != nil {
		return fmt.Errorf("%w: target: %w", ErrInvalidReport, err)
	}
	if r.Observer == r.Target {
		return fmt.Errorf("%w: %s cannot report on itself", ErrInvalidReport, r.Observer)
	}
	if r.Timeout <= 0 {
		return fmt.Errorf("%w: timeout %d is not positive", ErrInvalidReport, r.Timeout)
	}
	if r.LastSeenAt < 0 || r.SuspectedAt < 0 {
		return fmt.Errorf("%w: negative virtual time", ErrInvalidReport)
	}
	if r.SuspectedAt-r.LastSeenAt != r.Silence {
		return fmt.Errorf("%w: silence %d does not match the gap between last seen %d and suspicion %d",
			ErrInvalidReport, r.Silence, r.LastSeenAt, r.SuspectedAt)
	}
	if r.Silence < r.Timeout {
		return fmt.Errorf("%w: silence %d is shorter than timeout %d", ErrInvalidReport, r.Silence, r.Timeout)
	}
	return nil
}
