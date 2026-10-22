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
	"slices"
	"text/tabwriter"

	"github.com/HalxDocs/aegis/internal/domain"
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
	default:
		return scenario.Spec{}, fmt.Errorf("unknown scenario %q (available: node-crash)", name)
	}
}

func writeTable(w io.Writer, res scenario.Result) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SEQ\tAT\tEVENT\tDETAIL")
	for _, ev := range res.Log.Events() {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", ev.Seq, ev.At.Duration(), ev.Kind, detail(ev))
	}

	fmt.Fprintln(tw)
	fmt.Fprintln(tw, "TRUTH\tSTATE\tBELIEFS")
	for _, id := range idsOf(res) {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", id, res.Truth[id], beliefsAbout(res, id))
	}
	fmt.Fprintf(tw, "\nreports\t%d\tevents\t%d\n", len(res.Reports), res.Log.Len())
	return tw.Flush()
}

func detail(ev domain.Event) string {
	switch ev.Kind {
	case domain.KindScenarioStarted, domain.KindScenarioFinished:
		return ev.Scenario
	case domain.KindNodeStarted:
		return fmt.Sprintf("%s started", ev.Node)
	case domain.KindNodeCrashed:
		return fmt.Sprintf("%s crashed (ground truth)", ev.Node)
	case domain.KindHeartbeatSent, domain.KindHeartbeatReceived:
		return fmt.Sprintf("%s->%s round %d", ev.Node, ev.Peer, ev.Round)
	case domain.KindMessageDropped:
		return fmt.Sprintf("%s->%s round %d (%s)", ev.Node, ev.Peer, ev.Round, ev.Reason)
	case domain.KindSuspicionRaised:
		return fmt.Sprintf("%s distrusted by %s (%s)", ev.Node, ev.Peer, ev.Reason)
	case domain.KindSuspicionCleared:
		return fmt.Sprintf("%s trusted again by %s", ev.Node, ev.Peer)
	default:
		return ""
	}
}

// idsOf returns the configured nodes in sorted order, so the summary never
// depends on the order a scenario happened to list them.
func idsOf(res scenario.Result) []domain.NodeID {
	ids := make([]domain.NodeID, 0, len(res.Truth))
	for id := range res.Truth {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func beliefsAbout(res scenario.Result, target domain.NodeID) string {
	out := ""
	for _, o := range res.Spec.Observers {
		if o.Target != target {
			continue
		}
		obs, ok := res.Belief(o.Observer, target)
		if !ok {
			continue
		}
		if out != "" {
			out += "; "
		}
		out += fmt.Sprintf("%s: %s", o.Observer, describe(obs))
	}
	if out == "" {
		return "-"
	}
	return out
}

func describe(obs domain.Observation) string {
	if obs.Belief == domain.BeliefSuspected {
		return fmt.Sprintf("suspected since %s (silent %s, last seen %s)",
			obs.SuspectedAt.Duration(), (obs.SuspectedAt - obs.LastSeen).Duration(), obs.LastSeen.Duration())
	}
	return fmt.Sprintf("healthy (last seen %s)", obs.LastSeen.Duration())
}
