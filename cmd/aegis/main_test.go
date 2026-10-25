package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
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

func TestRunPartitionScenario(t *testing.T) {
	stdout, stderr, err := exec(t, "run", "--scenario", "one-way-partition", "--format", "table")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{
		"B->A path cut (one way)",
		"B->A path restored",
		"B distrusted by A (heartbeat_timeout)",
		"B trusted again by A",
		"message_dropped",
		"A      running",
		"reports  1",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("table output is missing %q:\n%s", want, stdout)
		}
	}

	// The table is a rendering; the JSONL is the artifact. They must agree.
	jsonl, _, err := exec(t, "run", "--scenario", "one-way-partition", "--format", "jsonl")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	res, err := scenario.Run(scenario.OneWayPartition(), 0)
	if err != nil {
		t.Fatalf("scenario.Run: %v", err)
	}
	var want bytes.Buffer
	if err := res.Log.WriteJSONL(&want); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}
	if jsonl != want.String() {
		t.Fatalf("CLI output differs from the recorded history:\n%s", jsonl)
	}
	if lines := strings.Count(jsonl, "\n"); lines != 75 {
		t.Fatalf("lines = %d, want the 75-event partition fixture", lines)
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

func TestDetailCoversEveryEventKind(t *testing.T) {
	cases := map[string]struct {
		event domain.Event
		want  string
	}{
		"scenario":        {domain.Event{Kind: domain.KindScenarioStarted, Scenario: "node-crash"}, "node-crash"},
		"started":         {domain.Event{Kind: domain.KindNodeStarted, Node: "B"}, "B started"},
		"crashed":         {domain.Event{Kind: domain.KindNodeCrashed, Node: "B"}, "B crashed (ground truth)"},
		"sent":            {domain.Event{Kind: domain.KindHeartbeatSent, Node: "B", Peer: "A", Round: 3}, "B->A round 3"},
		"received":        {domain.Event{Kind: domain.KindHeartbeatReceived, Node: "B", Peer: "A", Round: 3}, "B->A round 3"},
		"dropped":         {domain.Event{Kind: domain.KindMessageDropped, Node: "B", Peer: "A", Round: 3, Reason: domain.ReasonDropPolicy}, "B->A round 3 (drop_policy)"},
		"suspicion":       {domain.Event{Kind: domain.KindSuspicionRaised, Node: "B", Peer: "A", Reason: domain.ReasonHeartbeatTimeout}, "B distrusted by A (heartbeat_timeout)"},
		"suspicion clear": {domain.Event{Kind: domain.KindSuspicionCleared, Node: "B", Peer: "A"}, "B trusted again by A"},
		"partitioned":     {domain.Event{Kind: domain.KindLinkPartitioned, Node: "B", Peer: "A"}, "B->A path cut (one way)"},
		"restored":        {domain.Event{Kind: domain.KindLinkRestored, Node: "B", Peer: "A"}, "B->A path restored"},
		"unknown":         {domain.Event{Kind: domain.Kind("invented")}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := detail(tc.event); got != tc.want {
				t.Fatalf("detail() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDescribeBothBeliefs(t *testing.T) {
	suspected := domain.Observation{
		Observer: "A", Target: "B", Belief: domain.BeliefSuspected,
		LastSeen: domain.TickFrom(220 * time.Millisecond), SuspectedAt: domain.TickFrom(570 * time.Millisecond),
	}
	if got, want := describe(suspected), "suspected since 570ms (silent 350ms, last seen 220ms)"; got != want {
		t.Fatalf("describe(suspected) = %q, want %q", got, want)
	}
	healthy := domain.Observation{
		Observer: "A", Target: "B", Belief: domain.BeliefHealthy,
		LastSeen: domain.TickFrom(900 * time.Millisecond),
	}
	if got, want := describe(healthy), "healthy (last seen 900ms)"; got != want {
		t.Fatalf("describe(healthy) = %q, want %q", got, want)
	}
}
