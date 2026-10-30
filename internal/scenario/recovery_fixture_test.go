package scenario

import (
	"os"
	"testing"
)

// TestRecoveredCrashMatchesTheCommittedLog pins the granted history as bytes.
//
// It is the one artifact a reader can check without running anything: the log
// says the proposal cites both reports, the start cites the proposal, and the
// restart cites the start. A change in any of those edges changes the file.
func TestRecoveredCrashMatchesTheCommittedLog(t *testing.T) {
	committed, err := os.ReadFile("testdata/recovered-crash.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got := jsonl(t, mustRun(t, RecoveredCrash(), 0))
	if got != string(committed) {
		t.Fatalf("run does not match testdata/recovered-crash.jsonl:\n%s", got)
	}
}

// TestLoneWitnessMatchesTheCommittedLog pins the refused history as bytes.
//
// The interesting part of this fixture is what it does not contain: no
// recovery_started and no node_restarted, so the refusal is visible as an
// absence rather than only as a sentence in a table.
func TestLoneWitnessMatchesTheCommittedLog(t *testing.T) {
	committed, err := os.ReadFile("testdata/lone-witness.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got := jsonl(t, mustRun(t, LoneWitness(), 0))
	if got != string(committed) {
		t.Fatalf("run does not match testdata/lone-witness.jsonl:\n%s", got)
	}
}
