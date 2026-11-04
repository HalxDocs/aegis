// Command aegis runs one scenario and prints its event history, explains why one
// event in it happened, or lists the scenarios it can run.
//
// The table view and the explanation are both derived from the recorded events
// after the run; neither is a second source of truth. JSONL is the canonical
// output, so the default format is the one a later milestone reads.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/HalxDocs/aegis/internal/causal"
	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/scenario"
)

const usage = `usage: aegis run [flags]
       aegis explain -ref <seq> [flags]
       aegis scenarios

run flags:
  -scenario string  scenario to run (default "node-crash")
                    node-crash, one-way-partition, recovered-crash, lone-witness
  -seed uint        seed for deterministic decisions (default 0)
  -format string    output format: jsonl or table (default "jsonl")

explain flags:
  -ref uint         sequence number of the event to explain (required)
  -log string       a JSONL history to read instead of running a scenario
  -scenario string  scenario to run (default "node-crash")
  -seed uint        seed for deterministic decisions (default 0)
  -depth int        how far back to walk; 0 prints the whole chain

-log explains a recorded history, so -scenario and -seed do not apply with it.
`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "aegis:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing command\n\n" + usage)
	}
	switch args[0] {
	case "run":
		return runScenario(args[1:], stdout, stderr)
	case "explain":
		return runExplain(args[1:], stdout, stderr)
	case "scenarios":
		return runScenarios(args[1:], stdout)
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

func runScenario(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("aegis run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	name := flags.String("scenario", "node-crash", "scenario to run")
	seed := flags.Uint64("seed", 0, "seed for deterministic decisions")
	format := flags.String("format", "jsonl", "output format: jsonl or table")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	spec, err := lookup(*name)
	if err != nil {
		return err
	}
	result, err := scenario.Run(spec, *seed)
	if err != nil {
		return err
	}

	switch *format {
	case "jsonl":
		return result.Log.WriteJSONL(stdout)
	case "table":
		return writeTable(stdout, result)
	default:
		return fmt.Errorf("unknown format %q (available: jsonl, table)", *format)
	}
}

// runScenarios prints the scenarios this build can run, one per line.
//
// It exists so that the set is part of the command's interface rather than a
// detail of its source: a consumer that needs to know what exists asks the
// binary, instead of parsing the table above out of main.go and quietly holding
// an answer that a rename there would make wrong.
func runScenarios(args []string, stdout io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("scenarios takes no arguments, got %q", args[0])
	}
	for _, name := range scenarioNames() {
		if _, err := fmt.Fprintln(stdout, name); err != nil {
			return err
		}
	}
	return nil
}

// runExplain runs a scenario and prints why one of its events happened.
//
// The explanation is built from the finished log, not from anything the run kept
// in memory, so it says exactly what the artifact says — a reader with the JSONL
// and this command would get the same answer.
func runExplain(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("aegis explain", flag.ContinueOnError)
	flags.SetOutput(stderr)
	name := flags.String("scenario", "node-crash", "scenario to run")
	seed := flags.Uint64("seed", 0, "seed for deterministic decisions")
	ref := flags.Uint64("ref", 0, "sequence number of the event to explain")
	depth := flags.Int("depth", 0, "how far back to walk; 0 prints the whole chain")
	path := flags.String("log", "", "a JSONL history to read instead of running a scenario")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *ref == 0 {
		return errors.New("explain requires -ref: the sequence number of an event")
	}
	if *depth < 0 {
		return fmt.Errorf("depth %d is negative", *depth)
	}
	if *path != "" {
		// A flag that only means something while a scenario runs is not silently
		// ignored: a reader who passed it believed it did something, and being
		// told otherwise is cheaper than a wrong conclusion about the chain.
		var inapplicable []string
		flags.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "scenario":
				inapplicable = append(inapplicable, "-scenario")
			case "seed":
				inapplicable = append(inapplicable, "-seed")
			}
		})
		if len(inapplicable) > 0 {
			return fmt.Errorf("explain -log reads a recorded history, so %s cannot apply",
				strings.Join(inapplicable, " and "))
		}
	}

	index, err := explainIndex(*path, *name, *seed)
	if err != nil {
		return err
	}
	exp, err := index.Explain(domain.Ref(*ref))
	if err != nil {
		return fmt.Errorf("%s: %w", source(*path, *name), err)
	}
	return writeExplanation(stdout, exp, *depth)
}

// explainIndex returns the graph to explain from: either a history read from
// disk or the log of a scenario run now.
func explainIndex(path, name string, seed uint64) (*causal.Index, error) {
	if path != "" {
		return loadHistory(path)
	}
	spec, err := lookup(name)
	if err != nil {
		return nil, err
	}
	result, err := scenario.Run(spec, seed)
	if err != nil {
		return nil, err
	}
	index, err := result.Causal()
	if err != nil {
		return nil, fmt.Errorf("scenario %s: %w", name, err)
	}
	return index, nil
}

// source names where the history came from, so an error says which one.
func source(path, name string) string {
	if path != "" {
		return "log " + path
	}
	return "scenario " + name
}

// runnable is the set of scenarios this command offers, in the order they are
// announced. It is the only list: lookup dispatches from it and the
// unknown-scenario error quotes it, so a scenario cannot be run without being
// named, and a name cannot be offered without something to run.
//
// Reading scenarios from files is a later milestone; until then an unknown name
// is an error rather than an empty run.
var runnable = []struct {
	name  string
	build func() scenario.Spec
}{
	{name: "node-crash", build: scenario.NodeCrash},
	{name: "one-way-partition", build: scenario.OneWayPartition},
	{name: "recovered-crash", build: scenario.RecoveredCrash},
	{name: "lone-witness", build: scenario.LoneWitness},
}

// scenarioNames lists the runnable scenarios as a reader would type them.
func scenarioNames() []string {
	out := make([]string, 0, len(runnable))
	for _, s := range runnable {
		out = append(out, s.name)
	}
	return out
}

// lookup returns the spec for a named scenario.
func lookup(name string) (scenario.Spec, error) {
	for _, s := range runnable {
		if s.name == name {
			return s.build(), nil
		}
	}
	return scenario.Spec{}, fmt.Errorf("unknown scenario %q (available: %s)", name, strings.Join(scenarioNames(), ", "))
}
