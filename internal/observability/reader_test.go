package observability

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestAWrittenHistoryReadsBackUnchanged(t *testing.T) {
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
	if _, err := r.Record(domain.Event{
		At: 120, Kind: domain.KindHeartbeatReceived, Node: "B", Peer: "A", Round: 1,
		Causes: []domain.Ref{2},
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := r.WriteJSONL(&out); err != nil {
		t.Fatal(err)
	}
	back, err := ReadJSONL(&out)
	if err != nil {
		t.Fatalf("ReadJSONL: %v", err)
	}
	want := r.Events()
	if len(back) != len(want) {
		t.Fatalf("read %d events, wrote %d", len(back), len(want))
	}
	for i := range want {
		if back[i].Seq != want[i].Seq || back[i].Kind != want[i].Kind || back[i].At != want[i].At {
			t.Fatalf("event %d = %+v, want %+v", i, back[i], want[i])
		}
		if len(back[i].Causes) != len(want[i].Causes) {
			t.Fatalf("event %d came back with causes %v, want %v", i, back[i].Causes, want[i].Causes)
		}
	}
}

func TestReadJSONLRefusesADamagedLine(t *testing.T) {
	_, err := ReadJSONL(strings.NewReader("{\"seq\":1,\"kind\":\"scenario_started\"}\nnot json\n"))
	if !errors.Is(err, ErrDamagedLog) {
		t.Fatalf("error = %v, want ErrDamagedLog", err)
	}
}

func TestReadJSONLRefusesAnEventThatBreaksTheNumbering(t *testing.T) {
	_, err := ReadJSONL(strings.NewReader("{\"seq\":2,\"at\":0,\"kind\":\"scenario_started\",\"scenario\":\"x\"}\n"))
	if !errors.Is(err, ErrDamagedLog) {
		t.Fatalf("error = %v, want ErrDamagedLog", err)
	}
}
