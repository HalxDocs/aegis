package sim

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

// crashedCanonical returns the Milestone 1 cluster with B crashed at 150 and a
// proposing event at 200 that a restart can cite.
func crashedCanonical(t *testing.T) (*Engine, *stubRecorder, *stubReceiver) {
	t.Helper()
	e, rec, rx := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.Run(tickAt(150)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := e.CrashAt(tickAt(150), "B"); err != nil {
		t.Fatalf("CrashAt: %v", err)
	}
	if err := e.Run(tickAt(200)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return e, rec, rx
}

func restartAction(at domain.Tick, target domain.NodeID) domain.Action {
	return domain.Action{ID: 1, Kind: domain.ActionRestartNode, Target: target, At: at}
}

// refOf returns the reference the stub recorder assigned to the first event of
// the given kind. The stub numbers events from one in the order it took them.
func refOf(t *testing.T, rec *stubRecorder, kind domain.Kind) domain.Ref {
	t.Helper()
	for i, ev := range rec.events {
		if ev.Kind == kind {
			return domain.Ref(i + 1)
		}
	}
	t.Fatalf("no %s event recorded", kind)
	return 0
}

func TestRestartResumesTheHeartbeatRound(t *testing.T) {
	e, rec, rx := crashedCanonical(t)
	proposed, err := rec.Record(domain.Event{
		At: tickAt(200), Kind: domain.KindRecoveryProposed, Node: "B", Action: 1,
		Causes: []domain.Ref{1},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := e.Restart(restartAction(tickAt(300), "B"), proposed); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if err := e.Run(tickAt(600)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if state, _ := e.Truth("B"); state != domain.StateRunning {
		t.Fatalf("B is %s after a granted restart, want running", state)
	}
	sent := rec.ofKind(domain.KindHeartbeatSent)
	var rounds []uint64
	for _, ev := range sent {
		rounds = append(rounds, ev.Round)
	}
	// Round 1 at 100 and then nothing: the crash at 150 cancels the round that
	// was armed for 200. The restart at 300 resumes at round 2 rather than
	// starting the count again, so the same round number never appears twice.
	want := []uint64{1, 1, 2, 2, 3, 3, 4, 4}
	if len(rounds) != len(want) {
		t.Fatalf("rounds = %v, want %v", rounds, want)
	}
	for i := range want {
		if rounds[i] != want[i] {
			t.Fatalf("rounds = %v, want %v", rounds, want)
		}
	}

	// The resumed round names the restart rather than the round before the crash.
	restarted := rec.ofKind(domain.KindNodeRestarted)
	if len(restarted) != 1 {
		t.Fatalf("node_restarted events = %d, want 1", len(restarted))
	}
	resumedRef := refOf(t, rec, domain.KindNodeRestarted)
	var resumed []domain.Event
	for _, ev := range sent {
		if ev.Round == 2 {
			resumed = append(resumed, ev)
		}
	}
	if len(resumed) != 2 {
		t.Fatalf("round 2 sent %d times, want 2", len(resumed))
	}
	for _, ev := range resumed {
		if len(ev.Causes) != 1 {
			t.Fatalf("round 2 cites %v, want one cause", ev.Causes)
		}
		if ev.Causes[0] != resumedRef {
			t.Fatalf("round 2 cites #%d, want the restart #%d", ev.Causes[0], resumedRef)
		}
	}
	// The round sent at 600 is still in flight when the run ends, so the last
	// delivery is round 3 at 525.
	if rx.got[len(rx.got)-1].Round != 3 {
		t.Fatalf("last delivery round = %d, want 3", rx.got[len(rx.got)-1].Round)
	}
	rec.requireClean(t)
}

func TestRestartRecordsTheActionBeforeTheWorldChanges(t *testing.T) {
	e, rec, _ := crashedCanonical(t)
	proposed, err := rec.Record(domain.Event{
		At: tickAt(200), Kind: domain.KindRecoveryProposed, Node: "B", Action: 1,
		Causes: []domain.Ref{1},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := e.Restart(restartAction(tickAt(300), "B"), proposed); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if err := e.Run(tickAt(350)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	started := rec.ofKind(domain.KindRecoveryStarted)
	if len(started) != 1 {
		t.Fatalf("recovery_started events = %d, want 1", len(started))
	}
	if started[0].Action != 1 {
		t.Fatalf("recovery_started names %s, want action 1", started[0].Action)
	}
	if len(started[0].Causes) != 1 || started[0].Causes[0] != proposed {
		t.Fatalf("recovery_started cites %v, want the proposal #%d", started[0].Causes, proposed)
	}
	restarted := rec.ofKind(domain.KindNodeRestarted)
	if len(restarted) != 1 {
		t.Fatalf("node_restarted events = %d, want 1", len(restarted))
	}
	if refOf(t, rec, domain.KindNodeRestarted) <= refOf(t, rec, domain.KindRecoveryStarted) {
		t.Fatal("node_restarted precedes the recovery_started event that carries it out")
	}
	rec.requireClean(t)
}

func TestRestartRejectsARunningNode(t *testing.T) {
	e, rec, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	proposed, err := rec.Record(domain.Event{
		At: tickAt(50), Kind: domain.KindRecoveryProposed, Node: "B", Action: 1,
		Causes: []domain.Ref{1},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	err = e.Restart(restartAction(tickAt(100), "B"), proposed)
	if !errors.Is(err, ErrNotCrashed) {
		t.Fatalf("error = %v, want ErrNotCrashed", err)
	}
	if err := e.Run(tickAt(100)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rec.count(domain.KindRecoveryStarted) != 0 {
		t.Fatalf("a refused restart wrote recovery_started")
	}
}

func TestRestartRejectsAnUnknownTarget(t *testing.T) {
	e, rec, _ := crashedCanonical(t)
	proposed, err := rec.Record(domain.Event{
		At: tickAt(200), Kind: domain.KindRecoveryProposed, Node: "B", Action: 1,
		Causes: []domain.Ref{1},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	err = e.Restart(restartAction(tickAt(300), "D"), proposed)
	if !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("error = %v, want ErrUnknownNode", err)
	}
}

func TestRestartRejectsAnActionWithoutACause(t *testing.T) {
	e, _, _ := crashedCanonical(t)
	err := e.Restart(restartAction(tickAt(300), "B"), 0)
	if !errors.Is(err, domain.ErrInvalidAction) {
		t.Fatalf("error = %v, want ErrInvalidAction", err)
	}
	if state, _ := e.Truth("B"); state != domain.StateCrashed {
		t.Fatalf("B is %s, want crashed: a rejected restart changed ground truth", state)
	}
}

func TestRestartRejectsAnActionInThePast(t *testing.T) {
	e, rec, _ := crashedCanonical(t)
	proposed, err := rec.Record(domain.Event{
		At: tickAt(200), Kind: domain.KindRecoveryProposed, Node: "B", Action: 1,
		Causes: []domain.Ref{1},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	err = e.Restart(restartAction(tickAt(100), "B"), proposed)
	if !errors.Is(err, ErrScheduleInPast) {
		t.Fatalf("error = %v, want ErrScheduleInPast", err)
	}
}

func TestRestartRejectsANonRestartActionKind(t *testing.T) {
	e, rec, _ := crashedCanonical(t)
	proposed, err := rec.Record(domain.Event{
		At: tickAt(200), Kind: domain.KindRecoveryProposed, Node: "B", Action: 1,
		Causes: []domain.Ref{1},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	action := domain.Action{ID: 2, Kind: domain.ActionKind("drain_node"), Target: "B", At: tickAt(300)}
	err = e.Restart(action, proposed)
	if !errors.Is(err, domain.ErrInvalidAction) {
		t.Fatalf("error = %v, want ErrInvalidAction", err)
	}
}

func TestRestartArmsTheTimerFromTheRestartInstant(t *testing.T) {
	e, rec, _ := crashedCanonical(t)
	proposed, err := rec.Record(domain.Event{
		At: tickAt(200), Kind: domain.KindRecoveryProposed, Node: "B", Action: 1,
		Causes: []domain.Ref{1},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	// The node was down from 150 for longer than one interval. A timer resumed
	// from the interrupted schedule would backfill the missed round at 300.
	if err := e.Restart(restartAction(tickAt(450), "B"), proposed); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if err := e.Run(tickAt(600)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var after []domain.Event
	for _, ev := range rec.ofKind(domain.KindHeartbeatSent) {
		if ev.At > tickAt(450) {
			after = append(after, ev)
		}
	}
	if len(after) == 0 {
		t.Fatal("no heartbeat after the restart")
	}
	if after[0].At != tickAt(550) {
		t.Fatalf("first beat after the restart at %d, want 550", after[0].At)
	}
}
