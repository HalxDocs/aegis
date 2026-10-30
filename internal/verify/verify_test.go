package verify

import (
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/causal"
	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/observability"
)

// history records the events in order and returns the log read as a graph.
//
// The recorder assigns the sequence numbers, so the literals in decided below
// are a claim about those numbers and not a second numbering scheme.
func history(t *testing.T, events ...domain.Event) *causal.Index {
	t.Helper()
	rec := observability.NewRecorder()
	for i, ev := range events {
		if _, err := rec.Record(ev); err != nil {
			t.Fatalf("record event %d: %v", i+1, err)
		}
	}
	idx, err := causal.New(rec.Events())
	if err != nil {
		t.Fatalf("causal.New: %v", err)
	}
	return idx
}

// ms is the virtual instant n milliseconds into a run.
func ms(n int64) domain.Tick {
	return domain.TickFrom(time.Duration(n) * time.Millisecond)
}

// decided is the prefix every history here shares: a run opens, records a node,
// and hears two observers call that node dead.
func decided(_ *testing.T) []domain.Event {
	return []domain.Event{
		{At: ms(0), Kind: domain.KindScenarioStarted, Scenario: "recovered-crash"},
		{At: ms(0), Kind: domain.KindNodeStarted, Node: "B", Causes: []domain.Ref{1}},
		{At: ms(570), Kind: domain.KindSuspicionRaised, Node: "B", Peer: "A",
			Reason: domain.ReasonHeartbeatTimeout, Causes: []domain.Ref{2}},
		{At: ms(575), Kind: domain.KindSuspicionRaised, Node: "B", Peer: "C",
			Reason: domain.ReasonHeartbeatTimeout, Causes: []domain.Ref{2}},
	}
}

// proposal is the granted decision the two reports justified, citing both.
func proposal() domain.Event {
	return domain.Event{At: ms(620), Kind: domain.KindRecoveryProposed, Node: "B",
		Action: 1, Causes: []domain.Ref{3, 4}}
}

// started is the action being carried out, citing the proposal.
func started() domain.Event {
	return domain.Event{At: ms(720), Kind: domain.KindRecoveryStarted, Node: "B",
		Action: 1, Causes: []domain.Ref{5}}
}

// restarted is the change in ground truth, citing the action that caused it.
func restarted() domain.Event {
	return domain.Event{At: ms(720), Kind: domain.KindNodeRestarted, Node: "B",
		Causes: []domain.Ref{6}}
}

func TestActionProvesTheWholeChain(t *testing.T) {
	idx := history(t, append(decided(t), proposal(), started(), restarted())...)

	proof, ok, err := Action(idx, 1)
	if err != nil {
		t.Fatalf("Action: %v", err)
	}
	if !ok {
		t.Fatal("Action reported no proof for a chain the log connects end to end")
	}
	want := Proof{Action: 1, Proposed: 5, Started: 6, Applied: 7}
	if proof != want {
		t.Fatalf("proof = %+v, want %+v", proof, want)
	}
}

func TestActionStopsAtTheMissingEdge(t *testing.T) {
	// The restart is recorded immediately after the action, at the same instant,
	// but it does not cite it. Adjacency in the file is not an explanation.
	stray := domain.Event{At: ms(720), Kind: domain.KindNodeRestarted, Node: "B",
		Causes: []domain.Ref{1}}
	idx := history(t, append(decided(t), append([]domain.Event{proposal(), started()}, stray)...)...)

	proof, ok, err := Action(idx, 1)
	if err != nil {
		t.Fatalf("Action: %v", err)
	}
	if ok {
		t.Fatal("Action accepted a restart that never cites the action")
	}
	if proof.Proposed != 5 || proof.Started != 6 {
		t.Fatalf("proof = %+v, want the proposal and its start", proof)
	}
	if proof.Applied != 0 {
		t.Fatalf("proof cites a change in ground truth %s it did not find", proof.Applied)
	}
}

func TestActionReportsAGrantThatWasNeverCarriedOut(t *testing.T) {
	idx := history(t, append(decided(t), proposal())...)

	proof, ok, err := Action(idx, 1)
	if err != nil {
		t.Fatalf("Action: %v", err)
	}
	if ok {
		t.Fatal("Action proved an action that was only proposed")
	}
	if proof.Proposed != 5 || proof.Started != 0 {
		t.Fatalf("proof = %+v, want the proposal and nothing after it", proof)
	}
}

func TestActionReportsAnActionTheLogNeverMentions(t *testing.T) {
	idx := history(t, decided(t)...)

	proof, ok, err := Action(idx, 9)
	if err != nil {
		t.Fatalf("Action: %v", err)
	}
	if ok || proof != (Proof{}) {
		t.Fatalf("Action returned %+v and %v for an action with no events", proof, ok)
	}
}

func TestActionRejectsTheZeroIdentifier(t *testing.T) {
	idx := history(t, decided(t)...)
	if _, _, err := Action(idx, 0); err == nil {
		t.Fatal("Action accepted the zero identifier, which names no action")
	}
}

func TestActionRejectsAMissingHistory(t *testing.T) {
	if _, _, err := Action(nil, 1); err == nil {
		t.Fatal("Action accepted a nil history")
	}
}

func TestActionsVerifiesEachProposalOnce(t *testing.T) {
	// A second proposal for the same action is a re-decision, not a second
	// action: it names an identifier that has already been accounted for.
	again := domain.Event{At: ms(700), Kind: domain.KindRecoveryProposed, Node: "B",
		Action: 1, Causes: []domain.Ref{3, 4}}
	// The second proposal is a re-decision, so the start still cites the first
	// proposal: the evidence did not change, only that it was reviewed again.
	reapplied := domain.Event{At: ms(720), Kind: domain.KindRecoveryStarted, Node: "B",
		Action: 1, Causes: []domain.Ref{5}}
	restart := domain.Event{At: ms(720), Kind: domain.KindNodeRestarted, Node: "B",
		Causes: []domain.Ref{7}}
	idx := history(t, append(decided(t), proposal(), again, reapplied, restart)...)

	proofs, err := Actions(idx)
	if err != nil {
		t.Fatalf("Actions: %v", err)
	}
	if len(proofs) != 1 {
		t.Fatalf("proofs = %d, want 1", len(proofs))
	}
	if proofs[0].Started != 7 || proofs[0].Applied != 8 {
		t.Fatalf("proof = %+v, want it to reach the restart at 8", proofs[0])
	}
}

func TestActionsOnAHistoryWithNoRecovery(t *testing.T) {
	idx := history(t, decided(t)...)
	proofs, err := Actions(idx)
	if err != nil {
		t.Fatalf("Actions: %v", err)
	}
	if len(proofs) != 0 {
		t.Fatalf("proofs = %+v, want none", proofs)
	}
}

func TestActionsRejectsAMissingHistory(t *testing.T) {
	if _, err := Actions(nil); err == nil {
		t.Fatal("Actions accepted a nil history")
	}
}

func TestActionIgnoresARestartThatCitesAnotherAction(t *testing.T) {
	// Action 2 restarts the same node later. Its restart must not be offered as
	// proof for action 1, because that would prove the wrong repair happened.
	second := domain.Event{At: ms(900), Kind: domain.KindRecoveryProposed, Node: "B",
		Action: 2, Causes: []domain.Ref{3, 4}}
	secondStart := domain.Event{At: ms(1000), Kind: domain.KindRecoveryStarted, Node: "B",
		Action: 2, Causes: []domain.Ref{7}}
	secondRestart := domain.Event{At: ms(1000), Kind: domain.KindNodeRestarted, Node: "B",
		Causes: []domain.Ref{8}}
	idx := history(t, append(decided(t), proposal(), started(), second, secondStart, secondRestart)...)

	proof, ok, err := Action(idx, 1)
	if err != nil {
		t.Fatalf("Action: %v", err)
	}
	if ok {
		t.Fatal("Action 1 was proved by another action's restart")
	}
	if proof.Proposed != 5 || proof.Started != 6 || proof.Applied != 0 {
		t.Fatalf("proof = %+v, want only action 1's own events", proof)
	}

	later, ok, err := Action(idx, 2)
	if err != nil {
		t.Fatalf("Action: %v", err)
	}
	if !ok || later.Applied != 9 {
		t.Fatalf("proof for action 2 = %+v and %v, want applied 9", later, ok)
	}
}
