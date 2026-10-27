// Command aegis runs one scenario and prints its event history.
//
// The table view is derived from the recorded events after the run; it is not a
// second source of truth. JSONL is the canonical output, so the default format
// is the one a later milestone reads.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/scenario"
)

const usage = `usage: aegis run [flags]
       aegis explain -ref <seq> [flags]

run flags:
  -scenario string  scenario to run: node-crash or one-way-partition (default "node-crash")
  -seed uint        seed for deterministic decisions (default 0)
  -format string    output format: jsonl or table (default "jsonl")

explain flags:
  -ref uint         sequence number of the event to explain (required)
  -scenario string  scenario to run (default "node-crash")
  -seed uint        seed for deterministic decisions (default 0)
  -depth int        how far back to walk; 0 prints the whole chain
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

	spec, err := lookup(*name)
	if err != nil {
		return err
	}
	result, err := scenario.Run(spec, *seed)
	if err != nil {
		return err
	}
	index, err := result.Causal()
	if err != nil {
		return fmt.Errorf("scenario %s: %w", *name, err)
	}
	exp, err := index.Explain(domain.Ref(*ref))
	if err != nil {
		return fmt.Errorf("scenario %s: %w", *name, err)
	}
	return writeExplanation(stdout, exp, *depth)
}

// lookup keeps the set of runnable scenarios explicit. Reading scenarios from
// files is a later milestone; until then an unknown name is an error rather
// than an empty run.
func lookup(name string) (scenario.Spec, error) {
	switch name {
	case "node-crash":
		return scenario.NodeCrash(), nil
	case "one-way-partition":
		return scenario.OneWayPartition(), nil
	default:
		return scenario.Spec{}, fmt.Errorf("unknown scenario %q (available: node-crash, one-way-partition)", name)
	}
}
