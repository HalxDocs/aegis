package domain

// NodeState is what actually happened to a node in the simulation. It is
// ground truth: only the simulator sets it, and an observer must never read it
// to decide whether a node is healthy. Suspicion lives on Observation instead.
type NodeState string

const (
	// StateRunning means the simulated node is executing and may send work.
	StateRunning NodeState = "running"

	// StateCrashed means the simulator has stopped the node. It is terminal for
	// the lifetime of a run; restart is a recovery action, not a state edit.
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
// The only permitted move is running -> crashed. Repeating a crash is rejected
// as an error rather than ignored: a second crash event means the simulator has
// lost track of the node, and silently accepting it would hide that.
func (s NodeState) CanTransitionTo(next NodeState) bool {
	return s == StateRunning && next == StateCrashed
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
