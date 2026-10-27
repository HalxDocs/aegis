package main

import (
	"bytes"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := run(args, &out, &errOut)
	if err != nil && errOut.Len() > 0 {
		t.Logf("stderr: %s", errOut.String())
	}
	return out.String(), err
}

// TestExplainWalksFromASuspicionToTheEvidence is the milestone in one command:
// the chain is the receipt, the send, the node and the scenario, and the crash
// is not in it.
func TestExplainWalksFromASuspicionToTheEvidence(t *testing.T) {
	out, err := runCLI(t, "explain", "-scenario", "node-crash", "-ref", "14")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{
		"14  570ms  suspicion_raised  B distrusted by A (heartbeat_timeout)",
		"  <- 12  220ms  heartbeat_received  B->A round 2",
		"    <- 9  200ms  heartbeat_sent  B->A round 2",
		"      <- 3  0s  node_started  B started",
		"        <- 1  0s  scenario_started  node-crash",
	}
	if len(lines) != len(want) {
		t.Fatalf("lines = %d, want %d:\n%s", len(lines), len(want), out)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
	if strings.Contains(out, "node_crashed") {
		t.Fatal("the crash appears in an observer's reasoning; it was never delivered to anyone")
	}
}

func TestExplainCanBeLimitedToAFewSteps(t *testing.T) {
	out, err := runCLI(t, "explain", "-scenario", "node-crash", "-ref", "14", "-depth", "2")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want three steps and a marker:\n%s", len(lines), out)
	}
	if lines[2] != "    <- 9  200ms  heartbeat_sent  B->A round 2" {
		t.Fatalf("last step = %q, want the send", lines[2])
	}
	if strings.TrimSpace(lines[3]) != "..." {
		t.Fatalf("marker = %q, want a truncated branch to say so", lines[3])
	}
}

func TestExplainRejectsAReferenceThatNamesNothing(t *testing.T) {
	if _, err := runCLI(t, "explain", "-scenario", "node-crash", "-ref", "99"); err == nil {
		t.Fatal("explaining an event that does not exist was accepted")
	}
}

func TestExplainRequiresAReference(t *testing.T) {
	if _, err := runCLI(t, "explain", "-scenario", "node-crash"); err == nil {
		t.Fatal("explain ran without being told which event to explain")
	}
}

func TestExplainingTheRootShowsOneLine(t *testing.T) {
	out, err := runCLI(t, "explain", "-scenario", "node-crash", "-ref", "1")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want the opening event alone:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "scenario_started") {
		t.Fatalf("line = %q, want the scenario", lines[0])
	}
}
