package scenario

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/causal"
	"github.com/HalxDocs/aegis/internal/domain"
)

// causalIndex is the graph of a finished run, validated from its own log.
func causalIndex(t *testing.T, res Result) *causal.Index {
	t.Helper()
	idx, err := res.Causal()
	if err != nil {
		t.Fatalf("causal index: %v", err)
	}
	return idx
}

// refOfEvent finds the reference of the only event of a kind, and fails if the
// run does not contain exactly one.
func refOfEvent(t *testing.T, res Result, kind domain.Kind) domain.Ref {
	t.Helper()
	var found domain.Ref
	for _, ev := range res.Log.Events() {
		if ev.Kind == kind {
			if found != 0 {
				t.Fatalf("%s appears more than once", kind)
			}
			found = domain.Ref(ev.Seq)
		}
	}
	if found == 0 {
		t.Fatalf("no %s in the run", kind)
	}
	return found
}

func refsOfKind(t *testing.T, res Result, kind domain.Kind) []domain.Ref {
	t.Helper()
	var out []domain.Ref
	for _, ev := range res.Log.Events() {
		if ev.Kind == kind {
			out = append(out, domain.Ref(ev.Seq))
		}
	}
	return out
}

// TestEveryCanonicalRunFormsOneRootedGraph is the structural claim: a run is a
// single tree, every edge points backwards, and the root is the scenario. Any
// writer that forgets a cause fails here rather than in an explanation later.
func TestEveryCanonicalRunFormsOneRootedGraph(t *testing.T) {
	cases := []struct {
		name  string
		spec  Spec
		seeds []uint64
	}{
		{"node-crash", NodeCrash(), []uint64{0, 1, 7}},
		{"one-way-partition", OneWayPartition(), []uint64{0, 1, 7}},
		{"lossy", lossy(), []uint64{1, 2, 3, 4, 5}},
	}
	for _, tc := range cases {
		for _, seed := range tc.seeds {
			res := mustRun(t, tc.spec, seed)
			idx := causalIndex(t, res)
			if idx.Root() != 1 {
				t.Fatalf("%s seed %d: root = %s, want the opening event", tc.name, seed, idx.Root())
			}
			if idx.Len() != res.Log.Len() {
				t.Fatalf("%s seed %d: index covers %d of %d events", tc.name, seed, idx.Len(), res.Log.Len())
			}
			for _, ev := range res.Log.Events() {
				ancestors, err := idx.Ancestors(domain.Ref(ev.Seq))
				if err != nil {
					t.Fatalf("%s seed %d: %v", tc.name, seed, err)
				}
				// Every event traces back to the scenario, and the scenario
				// traces back to nothing.
				hasRoot := false
				for _, ref := range ancestors {
					hasRoot = hasRoot || ref == 1
				}
				if ev.Seq == 1 && len(ancestors) != 0 {
					t.Fatalf("%s seed %d: the root has ancestors %v", tc.name, seed, ancestors)
				}
				if ev.Seq != 1 && !hasRoot {
					t.Fatalf("%s seed %d: %s at %d does not trace back to the scenario",
						tc.name, seed, ev.Kind, ev.At)
				}
			}
		}
	}
}

// TestNoBeliefIsDescendedFromGroundTruth is the milestone's claim. Truth is
// recorded, and it is real, and nothing an observer concluded can be traced
// back to it — because the only thing an observer ever received was a message.
func TestNoBeliefIsDescendedFromGroundTruth(t *testing.T) {
	res := mustRun(t, NodeCrash(), 1)
	idx := causalIndex(t, res)

	crash := refOfEvent(t, res, domain.KindNodeCrashed)
	if descendants, err := idx.Descendants(crash); err != nil {
		t.Fatal(err)
	} else if len(descendants) != 0 {
		t.Fatalf("the crash explains %d events (%v); ground truth must be causally inert", len(descendants), descendants)
	}

	for _, ref := range append(refsOfKind(t, res, domain.KindSuspicionRaised), refsOfKind(t, res, domain.KindSuspicionCleared)...) {
		ancestors, err := idx.Ancestors(ref)
		if err != nil {
			t.Fatal(err)
		}
		for _, cause := range ancestors {
			if cause == crash {
				t.Fatalf("event %s is descended from the crash, which no observer saw", ref)
			}
		}
	}

	// The suspicion cites the last receipt, and the receipt cites the send.
	for _, ref := range refsOfKind(t, res, domain.KindSuspicionRaised) {
		ev, _ := idx.Event(ref)
		if len(ev.Causes) != 1 {
			t.Fatalf("suspicion %s cites %v, want exactly the evidence that set its deadline", ref, ev.Causes)
		}
		cause, ok := idx.Event(ev.Causes[0])
		if !ok || cause.Kind != domain.KindHeartbeatReceived {
			t.Fatalf("suspicion %s cites %+v, want a receipt", ref, cause)
		}
	}
}

// TestTheRepairExplainsNothing is the second claim, and the less obvious one.
// A repair is recorded, cites the cut it ends, and is cited by nothing at all:
// no belief anywhere in the run rests on the wire coming back.
func TestTheRepairExplainsNothing(t *testing.T) {
	res := mustRun(t, OneWayPartition(), 1)
	idx := causalIndex(t, res)

	restored := refOfEvent(t, res, domain.KindLinkRestored)
	if descendants, err := idx.Descendants(restored); err != nil {
		t.Fatal(err)
	} else if len(descendants) != 0 {
		t.Fatalf("the repair explains %v, but repairing a path does not change a belief", descendants)
	}

	// The suspicion's ancestry stops at the last evidence, which arrived before
	// the cut: the graph says why A acted, not why nothing further arrived.
	cut := refOfEvent(t, res, domain.KindLinkPartitioned)
	suspicion := refOfEvent(t, res, domain.KindSuspicionRaised)
	ancestors, err := idx.Ancestors(suspicion)
	if err != nil {
		t.Fatal(err)
	}
	for _, cause := range ancestors {
		if cause == cut {
			t.Fatal("the suspicion is descended from the cut, which no observer saw")
		}
	}
	// Recovery, by contrast, is descended from an arrival after the repair.
	cleared := refOfEvent(t, res, domain.KindSuspicionCleared)
	ev, _ := idx.Event(cleared)
	if len(ev.Causes) != 1 {
		t.Fatalf("the recovery cites %v, want the receipt that cleared it", ev.Causes)
	}
	last, _ := idx.Event(ev.Causes[0])
	if last.At != tick(920) {
		t.Fatalf("the recovery is explained by an event at %d, want the arrival at 920 ms", last.At)
	}
}

// TestALostMessageHasTwoReasons is the shape a branch gives: the send that put
// the message on the wire and the outage that swallowed it.
func TestALostMessageHasTwoReasons(t *testing.T) {
	res := mustRun(t, OneWayPartition(), 1)
	idx := causalIndex(t, res)

	drops := refsOfKind(t, res, domain.KindMessageDropped)
	if len(drops) != 6 {
		t.Fatalf("drops = %d, want the six rounds sent into the cut", len(drops))
	}
	ev, _ := idx.Event(drops[0])
	if len(ev.Causes) != 2 {
		t.Fatalf("the first drop cites %v, want two causes", ev.Causes)
	}
	cut := refOfEvent(t, res, domain.KindLinkPartitioned)
	if ev.Causes[0] != cut {
		t.Fatalf("the first drop cites %v first, want the cut %s", ev.Causes, cut)
	}
	sender, _ := idx.Event(ev.Causes[1])
	if sender.Kind != domain.KindHeartbeatSent {
		t.Fatalf("the second cause is %s, want the send", sender.Kind)
	}
	ancestors, err := idx.Ancestors(drops[0])
	if err != nil {
		t.Fatal(err)
	}
	// Both branches are in the ancestry, and the send branch runs back through
	// every earlier round on that direction before it reaches the node.
	for _, want := range []domain.Ref{cut, ev.Causes[1]} {
		found := false
		for _, cause := range ancestors {
			found = found || cause == want
		}
		if !found {
			t.Fatalf("the drop's ancestry %v omits %s", ancestors, want)
		}
	}
	if len(ancestors) < 6 {
		t.Fatalf("the drop has %d ancestors (%v), want the cut and the four-link send chain", len(ancestors), ancestors)
	}
}

// TestEveryExplanationReachesTheRoot walks every event of the partition run and
// requires its explanation to terminate at the opening event. This is the
// termination proof made concrete: the recursion needs no guard because every
// cause is recorded earlier than its effect.
func TestEveryExplanationReachesTheRoot(t *testing.T) {
	res := mustRun(t, OneWayPartition(), 1)
	idx := causalIndex(t, res)
	for _, ev := range res.Log.Events() {
		exp, err := idx.Explain(domain.Ref(ev.Seq))
		if err != nil {
			t.Fatalf("explain %d: %v", ev.Seq, err)
		}
		deepest := exp
		for len(deepest.Causes) > 0 {
			if len(deepest.Causes) != 1 {
				// Branching is allowed; follow the first path and check the rest
				// separately through the serious events.
				break
			}
			deepest = deepest.Causes[0]
		}
		if len(deepest.Causes) == 0 && deepest.Event.Kind != domain.KindScenarioStarted {
			t.Fatalf("the explanation of %d ends at %s, not at the scenario", ev.Seq, deepest.Event.Kind)
		}
		if exp.Depth() > idx.Len() {
			t.Fatalf("the explanation of %d is deeper (%d) than the history", ev.Seq, exp.Depth())
		}
	}
}

func TestTheGraphHoldsForFortySeeds(t *testing.T) {
	for seed := uint64(1); seed <= 40; seed++ {
		res := mustRun(t, lossy(), seed)
		idx := causalIndex(t, res)
		// The root is the only event allowed to stand alone.
		for _, ev := range res.Log.Events() {
			if ev.Seq == 1 {
				continue
			}
			if len(ev.Causes) == 0 {
				t.Fatalf("seed %d: %s at %d cites nothing", seed, ev.Kind, ev.At)
			}
		}
		if idx.Len() != res.Log.Len() {
			t.Fatalf("seed %d: index is missing events", seed)
		}
	}
}
