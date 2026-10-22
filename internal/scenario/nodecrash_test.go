package scenario

import (
	"bytes"
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
)

func tick(ms int64) domain.Tick { return domain.TickFrom(time.Duration(ms) * time.Millisecond) }

// step is an event with the fields this milestone's fixture cares about.
type step struct {
	At     domain.Tick
	Kind   domain.Kind
	Node   domain.NodeID
	Peer   domain.NodeID
	Round  uint64
	Reason string
}

func steps(t *testing.T, res Result) []step {
	t.Helper()
	out := make([]step, 0, res.Log.Len())
	for _, ev := range res.Log.Events() {
		out = append(out, step{At: ev.At, Kind: ev.Kind, Node: ev.Node, Peer: ev.Peer, Round: ev.Round, Reason: ev.Reason})
	}
	return out
}

func assertSteps(t *testing.T, got, want []step) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event count = %d, want %d\ngot:  %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestCanonicalScenarioReproducesTheExpectedTimeline(t *testing.T) {
	res, err := Run(NodeCrash(), 1)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []step{
		{At: 0, Kind: domain.KindScenarioStarted},
		{At: 0, Kind: domain.KindNodeStarted, Node: "A"},
		{At: 0, Kind: domain.KindNodeStarted, Node: "B"},
		{At: 0, Kind: domain.KindNodeStarted, Node: "C"},
		{At: tick(100), Kind: domain.KindHeartbeatSent, Node: "B", Peer: "A", Round: 1},
		{At: tick(100), Kind: domain.KindHeartbeatSent, Node: "B", Peer: "C", Round: 1},
		{At: tick(120), Kind: domain.KindHeartbeatReceived, Node: "B", Peer: "A", Round: 1},
		{At: tick(125), Kind: domain.KindHeartbeatReceived, Node: "B", Peer: "C", Round: 1},
		{At: tick(200), Kind: domain.KindHeartbeatSent, Node: "B", Peer: "A", Round: 2},
		{At: tick(200), Kind: domain.KindHeartbeatSent, Node: "B", Peer: "C", Round: 2},
		{At: tick(215), Kind: domain.KindNodeCrashed, Node: "B"},
		{At: tick(220), Kind: domain.KindHeartbeatReceived, Node: "B", Peer: "A", Round: 2},
		{At: tick(225), Kind: domain.KindHeartbeatReceived, Node: "B", Peer: "C", Round: 2},
		{At: tick(570), Kind: domain.KindSuspicionRaised, Node: "B", Peer: "A", Reason: domain.ReasonHeartbeatTimeout},
		{At: tick(575), Kind: domain.KindSuspicionRaised, Node: "B", Peer: "C", Reason: domain.ReasonHeartbeatTimeout},
		{At: tick(1200), Kind: domain.KindScenarioFinished},
	}
	assertSteps(t, steps(t, res), want)

	// The log's sequence numbers must be dense and increasing: a gap would mean
	// an event was dropped without anyone noticing.
	for i, ev := range res.Log.Events() {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("event %d has seq %d, want %d", i, ev.Seq, i+1)
		}
	}
}

func TestCrashedNodeIsBelievedHealthyUntilEachTimeout(t *testing.T) {
	before := NodeCrash()
	before.Horizon = tick(216)
	res, err := Run(before, 1)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := res.Truth["B"]; got != domain.StateCrashed {
		t.Fatalf("truth for B = %s, want crashed", got)
	}
	for _, observer := range []domain.NodeID{"A", "C"} {
		obs, ok := res.Belief(observer, "B")
		if !ok {
			t.Fatalf("%s holds no belief about B", observer)
		}
		if obs.Belief != domain.BeliefHealthy {
			t.Fatalf("%s believes B is %s at 216, want healthy", observer, obs.Belief)
		}
		// At 216 the round-two messages are still in flight, so the last
		// evidence either observer holds predates the crash by nearly 100 ms.
		if obs.LastSeen != tick(120) && obs.LastSeen != tick(125) {
			t.Fatalf("%s last saw B at %d, want the round-one arrival", observer, obs.LastSeen)
		}
	}
	if n := len(res.Reports); n != 0 {
		t.Fatalf("reports at 216 = %d, want none yet", n)
	}
}

func TestEachObserverSuspendsAtItsOwnDeadline(t *testing.T) {
	res, err := Run(NodeCrash(), 1)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := res.Truth["B"]; got != domain.StateCrashed {
		t.Fatalf("truth for B = %s, want crashed", got)
	}
	if len(res.Reports) != 2 {
		t.Fatalf("reports = %d, want one per observer", len(res.Reports))
	}

	want := []domain.FailureReport{
		{
			Observer:    "A",
			Target:      "B",
			SuspectedAt: tick(570),
			LastSeenAt:  tick(220),
			Timeout:     tick(350),
			Silence:     tick(350),
		},
		{
			Observer:    "C",
			Target:      "B",
			SuspectedAt: tick(575),
			LastSeenAt:  tick(225),
			Timeout:     tick(350),
			Silence:     tick(350),
		},
	}
	for i := range want {
		if res.Reports[i] != want[i] {
			t.Fatalf("report %d = %+v, want %+v", i, res.Reports[i], want[i])
		}
		if err := res.Reports[i].Validate(); err != nil {
			t.Fatalf("report %d is invalid: %v", i, err)
		}
	}

	for _, observer := range []domain.NodeID{"A", "C"} {
		obs, ok := res.Belief(observer, "B")
		if !ok || obs.Belief != domain.BeliefSuspected {
			t.Fatalf("%s final belief = %+v, want suspected", observer, obs)
		}
	}
}

func TestHealthyRunSuspendsNobody(t *testing.T) {
	spec := NodeCrash()
	spec.Crash = nil
	res, err := Run(spec, 1)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := res.Truth["B"]; got != domain.StateRunning {
		t.Fatalf("truth for B = %s, want running", got)
	}
	if n := len(res.Reports); n != 0 {
		t.Fatalf("reports = %d, want none in a healthy run", n)
	}
	for _, observer := range []domain.NodeID{"A", "C"} {
		obs, ok := res.Belief(observer, "B")
		if !ok || obs.Belief != domain.BeliefHealthy {
			t.Fatalf("%s belief = %+v, want healthy", observer, obs)
		}
		if obs.Heard < 10 {
			t.Fatalf("%s heard %d heartbeats in 1.2s at 100ms, want the run's traffic", observer, obs.Heard)
		}
	}
}

func TestSlowLinkMakesARunningNodeSuspectAndThenRecovers(t *testing.T) {
	// 400 ms of latency against a 350 ms timeout: the observer's evidence is
	// always older than its patience, so a perfectly healthy sender is doubted.
	spec := NodeCrash()
	spec.Links[0].Latency = tick(400)
	spec.Crash = nil
	res, err := Run(spec, 1)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := res.Truth["B"]; got != domain.StateRunning {
		t.Fatalf("truth for B = %s, want running: the node never failed", got)
	}
	reports := res.ReportsFor("A")
	if len(reports) != 1 {
		t.Fatalf("reports from A = %d, want exactly one false suspicion", len(reports))
	}
	if reports[0].SuspectedAt != tick(350) || reports[0].LastSeenAt != 0 {
		t.Fatalf("report = %+v, want suspicion of a node never heard from", reports[0])
	}
	obs, ok := res.Belief("A", "B")
	if !ok || obs.Belief != domain.BeliefHealthy {
		t.Fatalf("A's final belief = %+v, want healthy after the late heartbeat arrived", obs)
	}
	if reports := res.ReportsFor("C"); len(reports) != 0 {
		t.Fatalf("C produced %d reports on an unaffected link, want none", len(reports))
	}
}

func TestHeartbeatOnTheDeadlineTickPreventsSuspicion(t *testing.T) {
	// The first heartbeat leaves at 100 and takes 250 ms, so it lands exactly on
	// the deadline armed when the observer started watching at 0.
	spec := NodeCrash()
	spec.Nodes[1].HeartbeatEvery = tick(100)
	spec.Links[0].Latency = tick(250)
	spec.Crash = nil
	res, err := Run(spec, 1)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if n := len(res.Reports); n != 0 {
		t.Fatalf("reports = %d, want none: evidence at the deadline is evidence", n)
	}
	for _, ev := range res.Log.Events() {
		if ev.Kind == domain.KindSuspicionRaised {
			t.Fatalf("unexpected suspicion at %d", ev.At)
		}
	}
	if obs, _ := res.Belief("A", "B"); obs.LastSeen != tick(1150) {
		t.Fatalf("A last saw B at %d, want the last round to arrive inside the horizon", obs.LastSeen)
	}
}

func TestRunRejectsInvalidSpecBeforeWritingAnything(t *testing.T) {
	spec := NodeCrash()
	spec.Observers[0].Target = "D"
	if _, err := Run(spec, 1); err == nil {
		t.Fatal("a scenario watching an unconfigured node must be rejected")
	}
}

func TestResultDoesNotExposeMutableHistory(t *testing.T) {
	res, err := Run(NodeCrash(), 1)
	if err != nil {
		t.Fatal(err)
	}
	events := res.Log.Events()
	events[0].Scenario = "tampered"
	if got := res.Log.Events()[0].Scenario; got != "node-crash" {
		t.Fatalf("history mutated through Events(): %q", got)
	}
}

func TestCanonicalScenarioIsByteIdenticalAcrossRuns(t *testing.T) {
	first, err := Run(NodeCrash(), 1)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := first.Log.WriteJSONL(&want); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		res, err := Run(NodeCrash(), 1)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		var got bytes.Buffer
		if err := res.Log.WriteJSONL(&got); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if !bytes.Equal(got.Bytes(), want.Bytes()) {
			t.Fatalf("run %d produced a different log:\n%s", i, got.String())
		}
	}
}
