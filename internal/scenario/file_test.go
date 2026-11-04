package scenario

import (
	"errors"
	"strings"
	"testing"
)

// builtins is every scenario the binary can run. It is spelled out rather than
// imported from cmd/aegis because this test is about the file format, and the
// package that owns the format should not need the command to tell it what it
// has to be able to express.
func builtins() map[string]Spec {
	return map[string]Spec{
		"node-crash":        NodeCrash(),
		"one-way-partition": OneWayPartition(),
		"recovered-crash":   RecoveredCrash(),
		"lone-witness":      LoneWitness(),
		// Not a built-in scenario, but the only world that sets a drop rate: a
		// format that forgot `drop_ppm` would round-trip every built-in and
		// still describe a different network than the one asked for.
		"node-crash-lossy": lossy(),
	}
}

// TestEveryBuiltinSurvivesTheFileFormat is the guard that keeps this format
// honest.
//
// Each built-in scenario is written out, read back, and run. The logs have to be
// identical, which means the file can express every setting the simulator takes
// and a reader who ports a scenario to a file is not quietly given a different
// world. A field the format forgot to carry fails here as a difference in the
// log, not as a mystery about why a run changed.
func TestEveryBuiltinSurvivesTheFileFormat(t *testing.T) {
	for name, spec := range builtins() {
		t.Run(name, func(t *testing.T) {
			written, err := Marshal(spec)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			read, err := Load(strings.NewReader(string(written)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got, want := jsonl(t, mustRun(t, read, 0)), jsonl(t, mustRun(t, spec, 0)); got != want {
				t.Fatalf("the file describes a different run:\nwritten:\n%s", written)
			}
		})
	}
}

// TestTheShippedExampleIsTheScenarioItClaims keeps the file the README points at
// from becoming a description of a world that used to exist.
//
// A reader who copies it should get the run it names, and the day the built-in
// scenario changes the example has to change with it — which is a failing test
// rather than a paragraph nobody re-reads.
func TestTheShippedExampleIsTheScenarioItClaims(t *testing.T) {
	spec, err := LoadFile("../../docs/scenario-example.json")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if spec.Name != "recovered-crash" {
		t.Fatalf("name = %q, want recovered-crash", spec.Name)
	}
	if got, want := jsonl(t, mustRun(t, spec, 0)), jsonl(t, mustRun(t, RecoveredCrash(), 0)); got != want {
		t.Fatal("the shipped example is no longer the scenario it names")
	}
}

// TestLoadRejectsWhatItCannotHonour covers the mistakes a file can make. Each
// one is refused at load, before a run exists to explain it.
func TestLoadRejectsWhatItCannotHonour(t *testing.T) {
	valid := `{"name":"w","horizon":"1s","nodes":[{"id":"A"}]}`
	if _, err := Load(strings.NewReader(valid)); err != nil {
		t.Fatalf("the minimal file was rejected: %v", err)
	}

	cases := map[string]struct {
		file string
		want string
	}{
		"unknown field": {
			file: `{"name":"w","horizon":"1s","nodes":[{"id":"A"}],"observor":[]}`,
			want: "unknown field",
		},
		"misspelled node field": {
			file: `{"name":"w","horizon":"1s","nodes":[{"id":"A","hearbeat_every":"100ms"}]}`,
			want: "unknown field",
		},
		"a time without units": {
			file: `{"name":"w","horizon":"1000","nodes":[{"id":"A"}]}`,
			want: "is not a duration",
		},
		"a negative time": {
			file: `{"name":"w","horizon":"-1s","nodes":[{"id":"A"}]}`,
			want: "negative",
		},
		"a number instead of a duration": {
			file: `{"name":"w","horizon":1000,"nodes":[{"id":"A"}]}`,
			want: "must be a string",
		},
		"two scenarios in one file": {
			file: valid + "\n" + valid,
			want: "more than one scenario",
		},
		"no name": {
			file: `{"horizon":"1s","nodes":[{"id":"A"}]}`,
			want: "name is required",
		},
		"no horizon": {
			file: `{"name":"w","nodes":[{"id":"A"}]}`,
			want: "horizon 0 is not positive",
		},
		"an observer of nobody": {
			file: `{"name":"w","horizon":"1s","nodes":[{"id":"A"}],
				"observers":[{"observer":"A","target":"B","timeout":"350ms"}]}`,
			want: "target B is not a configured node",
		},
		"a recovery policy nobody states": {
			file: `{"name":"w","horizon":"1s","nodes":[{"id":"A"}],
				"recovery":{"min_witnesses":2,"patience":"50ms","delay":"100ms"}}`,
			want: "unknown action kind",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tc.file))
			if err == nil {
				t.Fatalf("Load accepted a file it cannot honour")
			}
			if !errors.Is(err, ErrInvalidFile) {
				t.Fatalf("error = %v, want ErrInvalidFile", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestLoadFileNamesTheFile keeps the error usable when the failure is a path
// rather than a field: a message that says only "invalid scenario" leaves the
// reader to work out which of several files they were looking at.
func TestLoadFileNamesTheFile(t *testing.T) {
	_, err := LoadFile("testdata/does-not-exist.json")
	if err == nil {
		t.Fatal("LoadFile accepted a missing file")
	}
	if !strings.Contains(err.Error(), "does-not-exist.json") {
		t.Fatalf("error = %q, want it to name the file", err)
	}
}

// TestAMinimalFileRuns is the end-to-end case: a file with no crash and one
// observer is a world, and it runs.
func TestAMinimalFileRuns(t *testing.T) {
	file := `{
		"name": "watched-heartbeat",
		"horizon": "400ms",
		"nodes": [
			{"id": "A"},
			{"id": "B", "heartbeat_every": "100ms", "peers": ["A"]}
		],
		"links": [{"from": "B", "to": "A", "latency": "20ms"}],
		"observers": [{"observer": "A", "target": "B", "timeout": "350ms"}]
	}`
	spec, err := Load(strings.NewReader(file))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if spec.Name != "watched-heartbeat" {
		t.Fatalf("name = %q", spec.Name)
	}
	res := mustRun(t, spec, 0)
	if res.Log.Len() == 0 {
		t.Fatal("the run recorded nothing")
	}
}
