package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

// TestVersionNamesBothTheBuildAndTheFormat pins the pair a reader needs before
// trusting a file: which build they are holding, and whether it reads the format
// in front of it.
func TestVersionNamesBothTheBuildAndTheFormat(t *testing.T) {
	stdout, stderr, err := exec(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("output = %q, want two lines", stdout)
	}
	if !strings.HasPrefix(lines[0], "aegis ") || strings.TrimSpace(lines[0]) == "aegis" {
		t.Fatalf("first line = %q, want the build", lines[0])
	}
	if want := "schema " + domain.EventLogSchema; lines[1] != want {
		t.Fatalf("second line = %q, want %q", lines[1], want)
	}

	// The same two lines behind the flag form, which is what a script reaches
	// for first.
	for _, form := range []string{"-version", "--version"} {
		out, _, err := exec(t, form)
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if out != stdout {
			t.Fatalf("%s printed %q, want %q", form, out, stdout)
		}
	}
}

// TestVersionTakesNoArguments keeps the command a question rather than a
// filter: `aegis version something` has no meaning, and inventing one would
// make the next version flag ambiguous.
func TestVersionTakesNoArguments(t *testing.T) {
	if _, _, err := exec(t, "version", "extra"); err == nil {
		t.Fatal("version accepted an argument")
	}
}

// TestRunFromAFileIsTheSameRunAsTheScenario proves the file path end to end at
// the command line: the same world described in a file prints the same bytes as
// the built-in scenario it was written from.
func TestRunFromAFileIsTheSameRunAsTheScenario(t *testing.T) {
	builtin, _, err := exec(t, "run", "--scenario", "recovered-crash", "--format", "jsonl")
	if err != nil {
		t.Fatalf("run --scenario: %v", err)
	}

	path := filepath.Join(t.TempDir(), "recovered-crash.json")
	file := `{
		"name": "recovered-crash",
		"horizon": "1200ms",
		"nodes": [
			{"id": "A"},
			{"id": "B", "heartbeat_every": "100ms", "peers": ["A", "C"]},
			{"id": "C"}
		],
		"links": [
			{"from": "B", "to": "A", "latency": "20ms"},
			{"from": "B", "to": "C", "latency": "25ms"}
		],
		"observers": [
			{"observer": "A", "target": "B", "timeout": "350ms"},
			{"observer": "C", "target": "B", "timeout": "350ms"}
		],
		"crash": {"node": "B", "at": "215ms"},
		"recovery": {"kind": "restart_node", "min_witnesses": 2, "patience": "50ms", "delay": "100ms"}
	}`
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatalf("write scenario file: %v", err)
	}

	fromFile, _, err := exec(t, "run", "--file", path, "--format", "jsonl")
	if err != nil {
		t.Fatalf("run --file: %v", err)
	}
	if fromFile != builtin {
		t.Fatalf("the file ran a different world:\nfile:\n%s\nbuilt-in:\n%s", fromFile, builtin)
	}
}

// TestNamingBothWorldsIsRefused: a command line that names two worlds is a
// question about precedence, and the answer here is that there is no precedence.
func TestNamingBothWorldsIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minimal.json")
	if err := os.WriteFile(path, []byte(`{"name":"w","horizon":"1s","nodes":[{"id":"A"}]}`), 0o600); err != nil {
		t.Fatalf("write scenario file: %v", err)
	}
	_, _, err := exec(t, "run", "--file", path, "--scenario", "node-crash")
	if err == nil {
		t.Fatal("run accepted -file and -scenario together")
	}
	if !strings.Contains(err.Error(), "-file and -scenario") {
		t.Fatalf("error = %q, want it to name both flags", err)
	}
}

// TestAFileThatCannotRunIsNamedByPath keeps a broken scenario file from looking
// like a broken program.
func TestAFileThatCannotRunIsNamedByPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte(`{"name":"w","horizon":"1s","nodes":[{"id":"A","peerz":[]}]}`), 0o600); err != nil {
		t.Fatalf("write scenario file: %v", err)
	}
	_, _, err := exec(t, "run", "--file", path)
	if err == nil {
		t.Fatal("run accepted a file with an unknown field")
	}
	if !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("error = %q, want it to name the file", err)
	}
}
