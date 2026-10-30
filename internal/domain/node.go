package domain

// NodeState is what actually happened to a node in the simulation. It is
// ground truth: only the simulator sets it, and an observer must never read it
// to decide whether a node is healthy. Suspicion lives on Observation instead.
type NodeState string

const (
	// StateRunning means the simulated node is executing and may send work.
	StateRunning NodeState = "running"

	// StateCrashed means the simulator has stopped the node. Nothing in the world
	// brings it back on its own: only a granted recovery action does, and the
	// simulator performs the move on that action's behalf rather than editing the
	// state directly.
	StateCrashed NodeState = "crashed"
)

// String returns the state as written in event logs.
func (s NodeState) String() string { return string(s) }

// Valid reports whether s is a known execution state.
func (s NodeState) Valid() bool {
	return s == StateRunning || s == StateCrashed
}

// CanTransitionTo reports whether the simulator may move from s to next.
//
// Both directions across the crash are permitted, and repeating either state is
// not. Repeating a crash is rejected as an error rather than ignored: a second
// crash event means the simulator has lost track of the node, and silently
// accepting it would hide that. The same rule forbids restarting a node that is
// already running, which is how a recovery action applied twice shows up as an
// error instead of as a harmless duplicate in the history.
//
// The state machine cannot tell a restart from any other way of reviving a
// node, so the authority is elsewhere: only a granted action reaches the
// simulator's restart path, and nothing else in the system can move a node out
// of StateCrashed.
func (s NodeState) CanTransitionTo(next NodeState) bool {
	if s == next {
		return false
	}
	return (s == StateRunning && next == StateCrashed) ||
		(s == StateCrashed && next == StateRunning)
}

// TransitionTo validates a move and returns the resulting state. On rejection
// it returns the unchanged state together with a *TransitionError that wraps
// ErrInvalidTransition.
func (s NodeState) TransitionTo(next NodeState) (NodeState, error) {
	if !s.Valid() {
		return s, &TransitionError{Subject: "node", From: string(s), To: string(next)}
	}
	if !s.CanTransitionTo(next) {
		return s, &TransitionError{Subject: "node", From: string(s), To: string(next)}
	}
	return next, nil
}
