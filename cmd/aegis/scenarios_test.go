package main

import (
	"slices"
	"strings"
	"testing"
)

// TestUsageAnnouncesEveryRunnableScenario keeps the usage text and the dispatch
// in step. They are two statements of one list, and a name in only one of them is
// a lie told by whichever the reader happened to use: offered but unsupported, or
// supported but invisible.
func TestUsageAnnouncesEveryRunnableScenario(t *testing.T) {
	announced := announcedScenarios(t)
	runnable := scenarioNames()

	for _, name := range announced {
		if !slices.Contains(runnable, name) {
			t.Errorf("usage offers %q but lookup cannot run it (%v)", name, runnable)
		}
	}
	for _, name := range runnable {
		if !slices.Contains(announced, name) {
			t.Errorf("lookup runs %q but usage does not announce it (%v)", name, announced)
		}
	}
}

// TestUnknownScenarioNamesEveryRunnableScenario checks the error a reader sees
// after a typo: that it offers the scenarios that exist, once each, in order,
// and that every name it offers is one this binary accepts.
//
// The names in the message are derived from the same table as scenarioNames, so
// this cannot catch a stale copy the way the usage test does — there is no copy
// left to go stale. What it holds is the part a derivation can still get wrong:
// that the list stays parseable in the documented form, and that the advice is
// executable, since a reader's next move is to run one of these names.
func TestUnknownScenarioNamesEveryRunnableScenario(t *testing.T) {
	_, err := lookup("no-such-scenario")
	if err == nil {
		t.Fatal("lookup accepted a scenario that does not exist")
	}
	const prefix = "available: "
	at := strings.Index(err.Error(), prefix)
	if at < 0 {
		t.Fatalf("error = %q, want it to offer %q", err, prefix)
	}
	listed := strings.TrimSpace(err.Error()[at+len(prefix):])
	listed = strings.TrimSuffix(listed, ")")
	offered := strings.Split(listed, ", ")
	want := scenarioNames()
	if !slices.Equal(offered, want) {
		t.Fatalf("error offers %q, want %q", offered, want)
	}
	for _, name := range offered {
		if _, err := lookup(name); err != nil {
			t.Fatalf("the error offered %q, which lookup rejects: %v", name, err)
		}
	}
}

// TestScenariosCommandPrintsEveryRunnableScenario keeps the list a command
// rather than a source detail. The page asks the binary which scenarios exist
// instead of parsing the table out of main.go, so this is the interface that
// change depends on: one name per line, every one of them runnable.
func TestScenariosCommandPrintsEveryRunnableScenario(t *testing.T) {
	stdout, stderr, err := exec(t, "scenarios")
	if err != nil {
		t.Fatalf("scenarios: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	want := strings.Join(scenarioNames(), "\n") + "\n"
	if stdout != want {
		t.Fatalf("scenarios printed %q, want %q", stdout, want)
	}
	for _, name := range strings.Fields(stdout) {
		if _, err := lookup(name); err != nil {
			t.Fatalf("the command printed %q, which cannot run: %v", name, err)
		}
	}
	if _, _, err := exec(t, "scenarios", "extra"); err == nil {
		t.Fatal("scenarios accepted an argument it does not have")
	}
}

// announcedScenarios reads the names the usage text offers, which are written on
// the line under the run command's -scenario flag. The explain block repeats that
// flag, and its next line is another flag rather than a list.
func announcedScenarios(t *testing.T) []string {
	t.Helper()
	lines := strings.Split(usage, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "-scenario string") || i+1 >= len(lines) {
			continue
		}
		listed := strings.TrimSpace(lines[i+1])
		if listed == "" || strings.HasPrefix(listed, "-") {
			continue
		}
		names := strings.Split(listed, ",")
		for j := range names {
			names[j] = strings.TrimSpace(names[j])
		}
		return names
	}
	t.Fatalf("usage names no scenarios under -scenario:\n%s", usage)
	return nil
}
