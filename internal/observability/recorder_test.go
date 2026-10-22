package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func event(kind domain.Kind, at domain.Tick) domain.Event {
	return domain.Event{At: at, Kind: kind, Node: "B", Peer: "A", Round: 1}
}

func TestRecordAssignsSequenceFromOne(t *testing.T) {
	r := NewRecorder()
	for i, at := range []domain.Tick{0, 10, 10, 20} {
		ev := domain.Event{At: at, Kind: domain.KindHeartbeatSent, Node: "B", Peer: "A", Round: uint64(i + 1)}
		if err := r.Record(ev); err != nil {
			t.Fatalf("Record(%d): %v", i, err)
		}
	}
	events := r.Events()
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4", len(events))
	}
	for i, ev := range events {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("event %d has seq %d, want %d", i, ev.Seq, i+1)
		}
	}
	if r.Len() != 4 {
		t.Fatalf("Len() = %d, want 4", r.Len())
	}
}

func TestRecordRejectsInvalidEventWithoutAdvancingSequence(t *testing.T) {
	r := NewRecorder()
	if err := r.Record(domain.Event{At: 1, Kind: domain.KindHeartbeatSent, Node: "B"}); err == nil {
		t.Fatal("event without a peer must be rejected")
	}
	if r.Len() != 0 {
		t.Fatalf("Len() = %d, want 0 after a rejected event", r.Len())
	}
	if err := r.Record(event(domain.KindHeartbeatSent, 1)); err != nil {
		t.Fatalf("Record after rejection: %v", err)
	}
	if got := r.Events()[0].Seq; got != 1 {
		t.Fatalf("first accepted event has seq %d, want 1", got)
	}
}

func TestRecordRejectsEventThatMovesBackwards(t *testing.T) {
	r := NewRecorder()
	if err := r.Record(event(domain.KindHeartbeatSent, 20)); err != nil {
		t.Fatal(err)
	}
	err := r.Record(event(domain.KindHeartbeatSent, 19))
	if !errors.Is(err, ErrOutOfOrder) {
		t.Fatalf("error = %v, want ErrOutOfOrder", err)
	}
	if r.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", r.Len())
	}
}

func TestRecordAllowsRepeatedTimestamps(t *testing.T) {
	r := NewRecorder()
	for i := 0; i < 2; i++ {
		if err := r.Record(event(domain.KindHeartbeatSent, 100)); err != nil {
			t.Fatalf("record at the same instant: %v", err)
		}
	}
	if r.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", r.Len())
	}
}

func TestRecordRejectsPreassignedSequence(t *testing.T) {
	r := NewRecorder()
	ev := event(domain.KindHeartbeatSent, 1)
	ev.Seq = 7
	err := r.Record(ev)
	if !errors.Is(err, ErrPreassignedSeq) {
		t.Fatalf("error = %v, want ErrPreassignedSeq", err)
	}
}

func TestEventsReturnsACopy(t *testing.T) {
	r := NewRecorder()
	if err := r.Record(event(domain.KindHeartbeatSent, 1)); err != nil {
		t.Fatal(err)
	}
	events := r.Events()
	events[0].Node = "Z"
	if got := r.Events()[0].Node; got != "B" {
		t.Fatalf("internal history mutated through a copy: node = %s", got)
	}
}

func TestWriteJSONLIsStableAndComplete(t *testing.T) {
	r := NewRecorder()
	for _, ev := range []domain.Event{
		{At: 0, Kind: domain.KindScenarioStarted, Scenario: "node-crash"},
		{At: 0, Kind: domain.KindNodeStarted, Node: "A"},
		{At: 100, Kind: domain.KindHeartbeatSent, Node: "B", Peer: "A", Round: 1},
		{At: 120, Kind: domain.KindHeartbeatReceived, Node: "B", Peer: "A", Round: 1},
		{At: 215, Kind: domain.KindNodeCrashed, Node: "B"},
	} {
		if err := r.Record(ev); err != nil {
			t.Fatalf("Record(%+v): %v", ev, err)
		}
	}

	var first bytes.Buffer
	if err := r.WriteJSONL(&first); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}
	var second bytes.Buffer
	if err := r.WriteJSONL(&second); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("two encodings of the same history differ")
	}

	lines := strings.Split(strings.TrimSuffix(first.String(), "\n"), "\n")
	if len(lines) != r.Len() {
		t.Fatalf("lines = %d, want %d", len(lines), r.Len())
	}
	for i, line := range lines {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("line %d is not JSON: %v", i, err)
		}
		if decoded["seq"] != float64(i+1) {
			t.Fatalf("line %d seq = %v, want %d", i, decoded["seq"], i+1)
		}
		if _, ok := decoded["at"]; !ok {
			t.Fatalf("line %d has no virtual timestamp", i)
		}
		for key := range decoded {
			switch key {
			case "time", "timestamp", "wall", "wallclock":
				t.Fatalf("line %d carries wall-clock field %q", i, key)
			}
		}
	}
}

func TestWriteJSONLOfAnEmptyHistoryIsEmpty(t *testing.T) {
	var out bytes.Buffer
	if err := NewRecorder().WriteJSONL(&out); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want empty", out.String())
	}
}

// failWriter refuses every write, which is how a closed pipe behaves.
type failWriter struct{ err error }

func (w failWriter) Write([]byte) (int, error) { return 0, w.err }

func TestWriteJSONLReportsEncoderFailures(t *testing.T) {
	r := NewRecorder()
	if err := r.Record(event(domain.KindHeartbeatSent, 1)); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("write end closed")
	err := r.WriteJSONL(failWriter{err: boom})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
	if !strings.Contains(err.Error(), "event 1") {
		t.Fatalf("error %q does not name the event that failed", err)
	}
}
