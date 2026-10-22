package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/HalxDocs/aegis/internal/scenario"
)

func exec(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = run(args, &out, &errOut)
	return out.String(), errOut.String(), err
}

func TestRunJSONLIsTheRecorderOutput(t *testing.T) {
	stdout, stderr, err := exec(t, "run", "--scenario", "node-crash", "--format", "jsonl")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	res, err := scenario.Run(scenario.NodeCrash(), 0)
	if err != nil {
		t.Fatalf("scenario.Run: %v", err)
	}
	var want bytes.Buffer
	if err := res.Log.WriteJSONL(&want); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}
	if stdout != want.String() {
		t.Fatalf("CLI output differs from the recorded history:\n%s", stdout)
	}
	if lines := strings.Count(stdout, "\n"); lines != 16 {
		t.Fatalf("lines = %d, want the 16-event fixture", lines)
	}
}

func TestRunDefaultsToJSONL(t *testing.T) {
	withDefault, _, err := exec(t, "run")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	explicit, _, err := exec(t, "run", "--scenario", "node-crash", "--format", "jsonl", "--seed", "0")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if withDefault != explicit {
		t.Fatal("default flags did not select the canonical JSONL run")
	}
}

func TestRunTableShowsTheFailure(t *testing.T) {
	stdout, _, err := exec(t, "run", "--format", "table")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		"SEQ",
		"215ms  node_crashed        B crashed (ground truth)",
		"570ms  suspicion_raised    B distrusted by A (heartbeat_timeout)",
		"575ms  suspicion_raised    B distrusted by C (heartbeat_timeout)",
		"B      crashed",
		"A: suspected since 570ms (silent 350ms, last seen 220ms)",
		"C: suspected since 575ms (silent 350ms, last seen 225ms)",
		"reports  2",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("table output is missing %q:\n%s", want, stdout)
		}
	}
}

func TestRunRejectsUnknownInputs(t *testing.T) {
	cases := map[string][]string{
		"no command":       {},
		"unknown command":  {"simulate"},
		"unknown scenario": {"run", "--scenario", "partition"},
		"unknown format":   {"run", "--format", "yaml"},
		"unexpected arg":   {"run", "extra"},
		"unknown flag":     {"run", "--turbo"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := exec(t, args...); err == nil {
				t.Fatalf("%v was accepted", args)
			}
		})
	}
}

func TestHelpSucceeds(t *testing.T) {
	stdout, _, err := exec(t, "help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(stdout, "usage: aegis run") || !strings.Contains(stdout, "-format") {
		t.Fatalf("help output = %q", stdout)
	}
	if _, _, err := exec(t, "run", "-h"); err != nil {
		t.Fatalf("run -h: %v", err)
	}
}
