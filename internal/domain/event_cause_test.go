package domain

import (
	"errors"
	"testing"
)

// suspected returns a minimal valid event that carries causes. Suspicion is the
// event the milestone is about, so it is the one the cause rules are tested on.
func suspected(causes ...Ref) Event {
	return Event{
		At:     1,
		Kind:   KindSuspicionRaised,
		Node:   "B",
		Peer:   "A",
		Reason: ReasonHeartbeatTimeout,
		Causes: causes,
	}
}

func TestEventRejectsAZeroCause(t *testing.T) {
	err := suspected(1, 0).Validate()
	if !errors.Is(err, ErrInvalidCause) {
		t.Fatalf("error = %v, want ErrInvalidCause", err)
	}
}
