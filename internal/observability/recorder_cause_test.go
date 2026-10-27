package observability

import (
	"bytes"
	"encoding/json"
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

func TestJSONLKeepsTheCauses(t *testing.T) {
	r := NewRecorder()
	root, err := r.Record(domain.Event{At: 0, Kind: domain.KindScenarioStarted, Scenario: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Record(domain.Event{
		At: 100, Kind: domain.KindHeartbeatSent, Node: "B", Peer: "A", Round: 1,
		Causes: []domain.Ref{root},
	}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := r.WriteJSONL(&out); err != nil {
		t.Fatal(err)
	}
	var decoded domain.Event
	if err := json.Unmarshal(bytes.Split(out.Bytes(), []byte("\n"))[1], &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Causes) != 1 || decoded.Causes[0] != root {
		t.Fatalf("decoded causes = %v, want %v", decoded.Causes, []domain.Ref{root})
	}
	// The opening event must not carry the field at all, so the log reads cleanly.
	var start domain.Event
	if err := json.Unmarshal(bytes.Split(out.Bytes(), []byte("\n"))[0], &start); err != nil {
		t.Fatal(err)
	}
	if len(start.Causes) != 0 {
		t.Fatalf("the root decoded with causes %v", start.Causes)
	}
}
