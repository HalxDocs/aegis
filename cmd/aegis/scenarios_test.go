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

// TestUnknownScenarioNamesEveryRunnableScenario keeps the error a reader sees
// after a typo in step with the same list. That is the moment they need the real
// names most, and a stale list there sends them to a name that does not run.
func TestUnknownScenarioNamesEveryRunnableScenario(t *testing.T) {
	_, err := lookup("no-such-scenario")
	if err == nil {
		t.Fatal("lookup accepted a scenario that does not exist")
	}
	want := "available: " + strings.Join(scenarioNames(), ", ")
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to list %q", err, want)
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
