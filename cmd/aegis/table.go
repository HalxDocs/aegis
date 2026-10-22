package main

import (
	"fmt"
	"io"
	"slices"
	"text/tabwriter"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/scenario"
)

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
