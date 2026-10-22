package domain

import "time"

// Tick is a point on the simulation's virtual timeline, measured in
// nanoseconds since the start of the run.
//
// It is deliberately an integer. Boundary decisions such as "did the deadline
// pass before the message arrived" must be exact, and a nested event
// simulation has no business consulting the wall clock.
type Tick int64

// TickFrom converts a duration into virtual time. The conversion is exact
// because time.Duration is itself a count of nanoseconds; no rounding happens
// at any point.
func TickFrom(d time.Duration) Tick { return Tick(d) }

// Duration converts a tick back to a duration, for display and configuration.
func (t Tick) Duration() time.Duration { return time.Duration(t) }

// Add returns the tick that occurs d later.
func (t Tick) Add(d time.Duration) Tick { return t + Tick(d) }

// Sub returns the virtual time elapsed between u and t. The result is negative
// when t precedes u, which lets callers detect a clock inversion explicitly.
func (t Tick) Sub(u Tick) Tick { return t - u }

// Before reports whether t occurs strictly before u.
func (t Tick) Before(u Tick) bool { return t < u }

// IsZero reports whether t is the zero tick. Virtual time starts at zero, so a
// zero tick also means "at the start of the run".
func (t Tick) IsZero() bool { return t == 0 }
