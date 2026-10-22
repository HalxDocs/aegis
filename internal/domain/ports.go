package domain

// Clock reports the current virtual time. The simulation is the only
// implementation that matters; fakes exist in tests.
type Clock interface {
	Now() Tick
}

// Timer is work scheduled for a virtual time. Cancel is idempotent and never
// fails: cancelling work that already ran, or was cancelled before, is a no-op.
// Callers rely on that when they re-arm a deadline on every message.
type Timer interface {
	Cancel()
}

// Scheduler runs fn at a virtual time.
//
// Scheduling in the past returns an error rather than clamping to the present.
// Clamping would silently reorder events, and reordering is exactly the class of
// bug a deterministic simulator exists to make impossible.
type Scheduler interface {
	// At runs fn at the given virtual time, in scheduling order with other
	// work at the same instant.
	At(at Tick, fn func()) (Timer, error)

	// AtEnd runs fn at the given virtual time, after all work registered with
	// At for that same instant. Deadline expiry uses it so a message that
	// arrives exactly on the deadline still counts as evidence; otherwise the
	// outcome of a tie would depend on which timer happened to be armed first.
	AtEnd(at Tick, fn func()) (Timer, error)
}
