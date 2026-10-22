package sim

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestRunAdvancesClockToHorizon(t *testing.T) {
	e, _, _, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(1200)); err != nil {
		t.Fatal(err)
	}
	if now := e.Now(); now != tickAt(1200) {
		t.Fatalf("Now() = %d, want %d", now, tickAt(1200))
	}
}

func TestRunRequiresStart(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if err := e.Run(tickAt(10)); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("error = %v, want ErrNotStarted", err)
	}
}

func TestRunRejectsHorizonInThePast(t *testing.T) {
	e, _, _, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(200)); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(100)); !errors.Is(err, ErrHorizonBeforeNow) {
		t.Fatalf("error = %v, want ErrHorizonBeforeNow", err)
	}
}

func TestRunPropagatesReceiverErrors(t *testing.T) {
	boom := errors.New("observer exploded")
	e, _, rx, _ := canonicalEngine(t)
	rx.err = boom
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(500)); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
}

func TestRunPropagatesCrashListenerErrors(t *testing.T) {
	boom := errors.New("listener exploded")
	e, _, _, listener := canonicalEngine(t)
	listener.err = boom
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.CrashAt(tickAt(150), "B"); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(500)); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
}

func TestRunFailsWhenMessagesArriveWithNoReceiver(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	for _, cfg := range []NodeConfig{
		{ID: "A"},
		{ID: "B", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"A"}},
	} {
		if err := e.AddNode(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.AddLink(LinkConfig{From: "B", To: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(500)); !errors.Is(err, ErrNoReceiver) {
		t.Fatalf("error = %v, want ErrNoReceiver", err)
	}
}

func TestRunEnforcesStepLimit(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{StepLimit: 3})
	for _, cfg := range []NodeConfig{
		{ID: "A"},
		{ID: "B", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"A"}},
	} {
		if err := e.AddNode(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.AddLink(LinkConfig{From: "B", To: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := e.SetReceiver(&stubReceiver{}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(1200)); !errors.Is(err, ErrStepLimit) {
		t.Fatalf("error = %v, want ErrStepLimit", err)
	}
}

func TestCrashAtRejectsUnknownNode(t *testing.T) {
	e, _, _, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.CrashAt(tickAt(100), "nobody"); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("error = %v, want ErrUnknownNode", err)
	}
}

func TestCrashAtRejectsThePast(t *testing.T) {
	e, _, _, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(200)); err != nil {
		t.Fatal(err)
	}
	if err := e.CrashAt(tickAt(150), "B"); !errors.Is(err, ErrScheduleInPast) {
		t.Fatalf("error = %v, want ErrScheduleInPast", err)
	}
}

func TestCrashFiresAtTheScheduledInstant(t *testing.T) {
	e, rec, _, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.CrashAt(tickAt(215), "B"); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(216)); err != nil {
		t.Fatal(err)
	}

	crashes := rec.ofKind(domain.KindNodeCrashed)
	if len(crashes) != 1 || crashes[0].At != tickAt(215) {
		t.Fatalf("crashes = %+v, want one at 215", crashes)
	}
	state, ok := e.Truth("B")
	if !ok || state != domain.StateCrashed {
		t.Fatalf("Truth(B) = %q, %v; want crashed", state, ok)
	}
}
