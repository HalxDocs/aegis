package main

import (
	"fmt"
	"io"
	"slices"
	"text/tabwriter"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/scenario"
	"github.com/HalxDocs/aegis/internal/verify"
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
	fmt.Fprintln(tw)
	fmt.Fprintln(tw, "DECISION\tAT\tOUTCOME\tEVIDENCE\tPROOF")
	if err := writeDecisions(tw, res); err != nil {
		return err
	}
	fmt.Fprintf(tw, "\nreports\t%d\tevents\t%d\n", len(res.Reports), res.Log.Len())
	return tw.Flush()
}

// writeDecisions lists every decision the history holds and, for the granted
// ones, the chain of events that proves the action happened.
//
// The proof column is not read from what the run claimed: it comes from walking
// the causal edges of the written log, so a table that says "applied #18" is
// saying the restart cites the action that cites the evidence.
func writeDecisions(w io.Writer, res scenario.Result) error {
	idx, err := res.Causal()
	if err != nil {
		return fmt.Errorf("table: %w", err)
	}
	for _, ev := range res.Log.Events() {
		if ev.Kind != domain.KindRecoveryProposed && ev.Kind != domain.KindRecoveryRefused {
			continue
		}
		outcome := "granted"
		if ev.Kind == domain.KindRecoveryRefused {
			outcome = "refused (" + ev.Reason + ")"
		}
		proof, ok, err := verify.Action(idx, ev.Action)
		if err != nil {
			return fmt.Errorf("table: %w", err)
		}
		fmt.Fprintf(w, "%s\t%s\t%s for %s\t%s\t%s\n",
			ev.Action, ev.At.Duration(), outcome, ev.Node, refs(ev.Causes), describeProof(proof, ok))
	}
	return nil
}

// describeProof renders the chain, or says plainly that the log does not connect
// one end to the other.
func describeProof(p verify.Proof, ok bool) string {
	if !ok {
		return "no complete chain"
	}
	return fmt.Sprintf("proposed %s, started %s, applied %s", p.Proposed, p.Started, p.Applied)
}

func refs(causes []domain.Ref) string {
	if len(causes) == 0 {
		return "-"
	}
	out := ""
	for i, cause := range causes {
		if i > 0 {
			out += " "
		}
		out += cause.String()
	}
	return out
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
	case domain.KindLinkPartitioned:
		return fmt.Sprintf("%s->%s path cut (one way)", ev.Node, ev.Peer)
	case domain.KindLinkRestored:
		return fmt.Sprintf("%s->%s path restored", ev.Node, ev.Peer)
	case domain.KindSuspicionRaised:
		return fmt.Sprintf("%s distrusted by %s (%s)", ev.Node, ev.Peer, ev.Reason)
	case domain.KindSuspicionCleared:
		return fmt.Sprintf("%s trusted again by %s", ev.Node, ev.Peer)
	case domain.KindRecoveryProposed:
		return fmt.Sprintf("%s granted for %s", ev.Action, ev.Node)
	case domain.KindRecoveryRefused:
		return fmt.Sprintf("%s refused for %s (%s)", ev.Action, ev.Node, ev.Reason)
	case domain.KindRecoveryStarted:
		return fmt.Sprintf("%s carried out on %s", ev.Action, ev.Node)
	case domain.KindNodeRestarted:
		return fmt.Sprintf("%s restarted (ground truth)", ev.Node)
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
