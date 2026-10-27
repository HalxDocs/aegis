package observability

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func sent(at domain.Tick, causes ...domain.Ref) domain.Event {
	return domain.Event{At: at, Kind: domain.KindHeartbeatSent, Node: "B", Peer: "A", Round: 1, Causes: causes}
}

func TestRecordRejectsACauseThatHasNotHappenedYet(t *testing.T) {
	r := NewRecorder()
	if _, err := r.Record(sent(10)); err != nil {
		t.Fatal(err)
	}
	// The next event will be number two, so citing three is citing the future.
	_, err := r.Record(sent(20, 3))
	if !errors.Is(err, ErrFutureCause) {
		t.Fatalf("error = %v, want ErrFutureCause", err)
	}
	if r.Len() != 1 {
		t.Fatalf("Len() = %d, want 1: a rejected event must not be appended", r.Len())
	}
}

func TestRecordAcceptsACauseThatAlreadyHappened(t *testing.T) {
	r := NewRecorder()
	first, err := r.Record(sent(10))
	if err != nil {
		t.Fatal(err)
	}
	if first != domain.Ref(1) {
		t.Fatalf("first reference = %s, want 1", first)
	}
	second, err := r.Record(domain.Event{
		At: 20, Kind: domain.KindHeartbeatReceived, Node: "B", Peer: "A", Round: 1,
		Causes: []domain.Ref{first},
	})
	if err != nil {
		t.Fatalf("citing a recorded event was rejected: %v", err)
	}
	if second != domain.Ref(2) {
		t.Fatalf("second reference = %s, want 2", second)
	}
}
