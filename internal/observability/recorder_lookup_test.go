package observability

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestEventReturnsWhatWasRecorded(t *testing.T) {
	r := NewRecorder()
	ref, err := r.Record(sent(10))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := r.Event(ref)
	if !ok {
		t.Fatalf("Event(%s) reported no such event", ref)
	}
	if got.Seq != uint64(ref) || got.At != 10 || got.Kind != domain.KindHeartbeatSent {
		t.Fatalf("Event(%s) = %+v, want the recorded send", ref, got)
	}
}

func TestEventRejectsReferencesItNeverIssued(t *testing.T) {
	r := NewRecorder()
	if _, err := r.Record(sent(10)); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []domain.Ref{0, 2, 99} {
		if _, ok := r.Event(ref); ok {
			t.Fatalf("Event(%s) resolved to an event that does not exist", ref)
		}
	}
}
