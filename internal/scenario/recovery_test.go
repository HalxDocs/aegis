package scenario

import (
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/recovery"
)

func TestRecoveredCrashGrantsTheRestartAndTheNodeResumes(t *testing.T) {
	res, err := Run(RecoveredCrash(), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := res.Truth["B"]; got != domain.StateRunning {
		t.Fatalf("B is %s at the horizon, want running", got)
	}

	proposed := eventsOf(res, domain.KindRecoveryProposed)
	if len(proposed) != 1 {
		t.Fatalf("recovery_proposed events = %d, want 1", len(proposed))
	}
	if proposed[0].At != tickMs(620) {
		t.Fatalf("proposal at %s, want 620ms", proposed[0].At.Duration())
	}
	if proposed[0].Action != 1 || proposed[0].Node != "B" {
		t.Fatalf("proposal = %+v, want action 1 for B", proposed[0])
	}
	if len(proposed[0].Causes) != 2 {
		t.Fatalf("proposal cites %v, want the two reports", proposed[0].Causes)
	}

	started := eventsOf(res, domain.KindRecoveryStarted)
	if len(started) != 1 || started[0].At != tickMs(720) {
		t.Fatalf("recovery_started = %+v, want one event at 720ms", started)
	}
	restarted := eventsOf(res, domain.KindNodeRestarted)
	if len(restarted) != 1 || restarted[0].At != tickMs(720) {
		t.Fatalf("node_restarted = %+v, want one event at 720ms", restarted)
	}
	if len(restarted[0].Causes) != 1 || restarted[0].Causes[0] != domain.Ref(started[0].Seq) {
		t.Fatalf("node_restarted cites %v, want the action #%d", restarted[0].Causes, started[0].Seq)
	}

	// The resumed round is round three: the counter did not start over.
	var resumed domain.Event
	for _, ev := range eventsOf(res, domain.KindHeartbeatSent) {
		if ev.At > tickMs(720) {
			resumed = ev
			break
		}
	}
	if resumed.Round != 3 {
		t.Fatalf("first round after the restart is %d, want 3", resumed.Round)
	}
	if len(resumed.Causes) != 1 || resumed.Causes[0] != domain.Ref(restarted[0].Seq) {
		t.Fatalf("resumed round cites %v, want the restart #%d", resumed.Causes, restarted[0].Seq)
	}

	if len(res.Decisions) != 1 || !res.Decisions[0].Granted {
		t.Fatalf("decisions = %v, want one grant", res.Decisions)
	}
	if len(res.Actions) != 1 || res.Actions[0].At != tickMs(720) {
		t.Fatalf("actions = %v, want one at 720ms", res.Actions)
	}
}

func TestRecoveredCrashClearsTheBeliefOnlyWhenEvidenceArrives(t *testing.T) {
	res, err := Run(RecoveredCrash(), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	cleared := eventsOf(res, domain.KindSuspicionCleared)
	if len(cleared) != 2 {
		t.Fatalf("suspicion_cleared events = %d, want 2", len(cleared))
	}
	if cleared[0].At != tickMs(840) || cleared[0].Peer != "A" {
		t.Fatalf("A clears at %s, want 840ms", cleared[0].At.Duration())
	}
	if cleared[0].At <= tickMs(720) {
		t.Fatal("a belief was cleared before the node was restarted")
	}
}

func TestRecoveryIsAbsentUnlessTheScenarioAsksForIt(t *testing.T) {
	res, err := Run(NodeCrash(), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, kind := range []domain.Kind{
		domain.KindRecoveryProposed, domain.KindRecoveryRefused,
		domain.KindRecoveryStarted, domain.KindNodeRestarted,
	} {
		if n := len(eventsOf(res, kind)); n != 0 {
			t.Fatalf("%s events = %d in a scenario with no policy", kind, n)
		}
	}
	if len(res.Decisions) != 0 || len(res.Actions) != 0 {
		t.Fatalf("run reported decisions %v and actions %v without a policy", res.Decisions, res.Actions)
	}
}

func TestRecoveryRejectsAnInvalidPolicy(t *testing.T) {
	spec := RecoveredCrash()
	spec.Recovery = &RecoverySpec{Policy: recovery.Policy{Kind: domain.ActionRestartNode, MinWitnesses: 0}}
	if _, err := Run(spec, 0); err == nil {
		t.Fatal("Run accepted a policy with no threshold")
	}
}

func TestRecoveredCrashIsDeterministic(t *testing.T) {
	first, err := Run(RecoveredCrash(), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	second, err := Run(RecoveredCrash(), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if first.Log.Len() != second.Log.Len() {
		t.Fatalf("lengths differ: %d and %d", first.Log.Len(), second.Log.Len())
	}
	for i, ev := range first.Log.Events() {
		other := second.Log.Events()[i]
		if ev.Kind != other.Kind || ev.At != other.At || ev.Node != other.Node || ev.Round != other.Round {
			t.Fatalf("event %d differs: %+v and %+v", i+1, ev, other)
		}
	}
}

// tickMs is the virtual instant n milliseconds into a run.
func tickMs(n int64) domain.Tick {
	return domain.TickFrom(time.Duration(n) * time.Millisecond)
}

// eventsOf returns the recorded events of one kind, in sequence order.
func eventsOf(res Result, kind domain.Kind) []domain.Event {
	var out []domain.Event
	for _, ev := range res.Log.Events() {
		if ev.Kind == kind {
			out = append(out, ev)
		}
	}
	return out
}
