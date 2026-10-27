package sim

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestOpenRecordsTheRootOfTheGraph(t *testing.T) {
	e, rec := newTestEngine(t, EngineConfig{})
	if err := e.Open("node-crash"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := e.Root(); got != domain.Ref(1) {
		t.Fatalf("Root() = %s, want 1", got)
	}
	if len(rec.events) != 1 {
		t.Fatalf("events = %d, want the opening event alone", len(rec.events))
	}
	got := rec.events[0]
	if got.Kind != domain.KindScenarioStarted || got.Scenario != "node-crash" {
		t.Fatalf("opening event = %+v, want scenario_started node-crash", got)
	}
	if len(got.Causes) != 0 {
		t.Fatalf("the opening event cites %v, but a root has no cause", got.Causes)
	}
}

func TestRootIsZeroBeforeTheHistoryOpens(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if got := e.Root(); got != 0 {
		t.Fatalf("Root() = %s before Open, want zero", got)
	}
}

func TestOpenTwiceIsRefused(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if err := e.Open("node-crash"); err != nil {
		t.Fatal(err)
	}
	if err := e.Open("node-crash"); !errors.Is(err, ErrAlreadyOpen) {
		t.Fatalf("error = %v, want ErrAlreadyOpen", err)
	}
}

func TestOpenAfterStartIsRefused(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Open("node-crash"); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("error = %v, want ErrAlreadyStarted", err)
	}
}

func TestCloseCitesTheOpeningEvent(t *testing.T) {
	e, rec := newTestEngine(t, EngineConfig{})
	if err := e.Open("node-crash"); err != nil {
		t.Fatal(err)
	}
	if err := e.Close("node-crash"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	last := rec.events[len(rec.events)-1]
	if last.Kind != domain.KindScenarioFinished {
		t.Fatalf("last event = %s, want scenario_finished", last.Kind)
	}
	if len(last.Causes) != 1 || last.Causes[0] != domain.Ref(1) {
		t.Fatalf("closing event cites %v, want the opening event", last.Causes)
	}
}

func TestCloseWithoutOpenIsRefused(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if err := e.Close("node-crash"); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("error = %v, want ErrNotStarted", err)
	}
}
