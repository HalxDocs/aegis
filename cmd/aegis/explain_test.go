package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
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
		"<- 12  220ms  heartbeat_received  B->A round 2",
		"    <- 9  200ms  heartbeat_sent  B->A round 2",
		"        <- 5  100ms  heartbeat_sent  B->A round 1",
		"            <- 3  0s  node_started  B started",
		"                <- 1  0s  scenario_started  node-crash",
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

func TestExplainingADropShowsBothReasons(t *testing.T) {
	out, err := runCLI(t, "explain", "-scenario", "one-way-partition", "-ref", "22")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.Contains(lines[0], "message_dropped") {
		t.Fatalf("first line = %q, want the drop", lines[0])
	}
	// Both causes are one level down: the cut, then the send. Each branch is
	// expanded in turn, so the send is not the second line.
	branches := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "<- ") {
			branches++
			if !strings.Contains(line, "link_partitioned") && !strings.Contains(line, "heartbeat_sent") {
				t.Fatalf("top-level branch = %q, want the cut or the send", line)
			}
		}
	}
	if branches != 2 {
		t.Fatalf("top-level branches = %d, want two:\n%s", branches, out)
	}
}

func TestExplainingTheRecoveryShowsTheArrivalThatCausedIt(t *testing.T) {
	out, err := runCLI(t, "explain", "-scenario", "one-way-partition", "-ref", "61")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.Contains(lines[0], "suspicion_cleared") {
		t.Fatalf("first line = %q, want the recovery", lines[0])
	}
	if !strings.Contains(lines[1], "920ms") || !strings.Contains(lines[2], "900ms") {
		t.Fatalf("recovery chain = %q then %q, want 920 ms over 900 ms", lines[1], lines[2])
	}
	if strings.Contains(out, "link_restored") {
		t.Fatal("the repair appears in the recovery's reasoning; the belief changed because a message arrived")
	}
}

// loggedRun runs a scenario through the CLI and returns its history as bytes,
// so the tests below explain exactly the artifact a user would have on disk.
func loggedRun(t *testing.T, scenarioName string) []byte {
	t.Helper()
	out, err := runCLI(t, "run", "-scenario", scenarioName, "-format", "jsonl")
	if err != nil {
		t.Fatalf("run %s: %v", scenarioName, err)
	}
	return []byte(out)
}

// writeLog decodes a history, lets mutate edit it, and writes the result to a
// temporary file the CLI can be pointed at.
func writeLog(t *testing.T, name string, raw []byte, mutate func(events []domain.Event)) string {
	t.Helper()
	var events []domain.Event
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var ev domain.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		events = append(events, ev)
	}
	if mutate != nil {
		mutate(events)
	}

	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			t.Fatalf("encode event %d: %v", ev.Seq, err)
		}
	}
	return path
}

// setCause replaces one event's cause list in a decoded history.
func setCause(events []domain.Event, seq uint64, causes ...domain.Ref) {
	for i := range events {
		if events[i].Seq == seq {
			events[i].Causes = causes
			return
		}
	}
}

// TestExplainReadsAPinnedLog proves -log explains the file rather than a fresh
// run: the same reference must produce the same chain either way.
func TestExplainReadsAPinnedLog(t *testing.T) {
	path := writeLog(t, "node-crash.jsonl", loggedRun(t, "node-crash"), nil)

	fromFile, err := runCLI(t, "explain", "-log", path, "-ref", "14")
	if err != nil {
		t.Fatalf("explain -log: %v", err)
	}
	fromRun, err := runCLI(t, "explain", "-scenario", "node-crash", "-ref", "14")
	if err != nil {
		t.Fatalf("explain -scenario: %v", err)
	}
	if fromFile != fromRun {
		t.Fatalf("the file and the run disagree:\nfile:\n%s\nrun:\n%s", fromFile, fromRun)
	}
}

// TestExplainRefusesALogWithAFutureCause is the end-to-end version of the rule
// the index enforces: a hand-edited log is refused with a sentence about the
// file, not a chain that looks plausible.
func TestExplainRefusesALogWithAFutureCause(t *testing.T) {
	path := writeLog(t, "future-cause.jsonl", loggedRun(t, "one-way-partition"), func(events []domain.Event) {
		setCause(events, 61, 63)
	})

	out, err := runCLI(t, "explain", "-log", path, "-ref", "61")
	if err == nil {
		t.Fatal("a log whose recovery cites a later event was explained anyway")
	}
	if out != "" {
		t.Fatalf("a refused log still produced output:\n%s", out)
	}
	msg := err.Error()
	for _, want := range []string{"is not a causal history", "cause is not recorded before its effect"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q is missing %q", msg, want)
		}
	}
	if !strings.Contains(msg, path) {
		t.Fatalf("error %q does not name the file it refused", msg)
	}
}

// TestExplainRefusesALogWithAMissingCause covers the other way an edited log
// breaks: a reference to an event that was never written down.
func TestExplainRefusesALogWithAMissingCause(t *testing.T) {
	path := writeLog(t, "dangling.jsonl", loggedRun(t, "node-crash"), func(events []domain.Event) {
		setCause(events, 14, 900)
	})

	_, err := runCLI(t, "explain", "-log", path, "-ref", "14")
	if err == nil {
		t.Fatal("a log citing an event outside the history was explained anyway")
	}
	if !strings.Contains(err.Error(), "cause names an event outside the history") {
		t.Fatalf("error = %q, want the dangling-cause rule", err)
	}
}

// TestExplainRefusesALogItCannotParse keeps the two refusals apart: damage the
// reader can see is reported as unreadable, not as a graph violation.
func TestExplainRefusesALogItCannotParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "truncated.jsonl")
	if err := os.WriteFile(path, []byte("{\"seq\":1,\"at\":0,"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	_, err := runCLI(t, "explain", "-log", path, "-ref", "1")
	if err == nil {
		t.Fatal("a truncated log was read as a history")
	}
	if !strings.Contains(err.Error(), "is not a readable history") {
		t.Fatalf("error = %q, want the unreadable-history verdict", err)
	}
}

func TestExplainRefusesLogAndScenarioTogether(t *testing.T) {
	path := writeLog(t, "node-crash.jsonl", loggedRun(t, "node-crash"), nil)
	if _, err := runCLI(t, "explain", "-log", path, "-scenario", "node-crash", "-ref", "14"); err == nil {
		t.Fatal("explain accepted two sources for one explanation")
	}
}

func TestExplainingTheSuspicionInThePartitionRunStopsAtTheEvidence(t *testing.T) {
	out, err := runCLI(t, "explain", "-scenario", "one-way-partition", "-ref", "36")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.Contains(lines[0], "suspicion_raised") {
		t.Fatalf("first line = %q, want the suspicion", lines[0])
	}
	if !strings.Contains(out, "220ms") {
		t.Fatalf("the chain does not mention the evidence at 220 ms:\n%s", out)
	}
	if strings.Contains(out, "link_partitioned") {
		t.Fatalf("the cut appears in the belief's reasoning:\n%s", out)
	}
}
