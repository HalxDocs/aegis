// Command aegis runs one Milestone 1 scenario and prints its event history.
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

	"github.com/HalxDocs/aegis/internal/scenario"
)

const usage = `usage: aegis run [flags]

flags:
  -scenario string  scenario to run (default "node-crash")
  -seed uint        seed for deterministic decisions (default 0)
  -format string    output format: jsonl or table (default "jsonl")
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
