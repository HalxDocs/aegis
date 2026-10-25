package scenario

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/sim"
)

// TestOneWayPartitionBreaksNobodyAndStillFoolsSomebody is the milestone's claim:
// a whole run in which every node is running, nobody is unreachable by everyone,
// and one observer is still sure something is wrong.
func TestOneWayPartitionBreaksNobodyAndStillFoolsSomebody(t *testing.T) {
	res := mustRun(t, OneWayPartition(), 1)

	for _, id := range []domain.NodeID{"A", "B", "C"} {
		if got := res.Truth[id]; got != domain.StateRunning {
			t.Fatalf("truth for %s = %s, want running: nothing in this world failed", id, got)
		}
	}

	reports := res.ReportsFor("A")
	if len(reports) != 1 {
		t.Fatalf("reports from A = %d, want exactly one false suspicion", len(reports))
	}
	want := domain.FailureReport{
		Observer:    "A",
		Target:      "B",
		SuspectedAt: tick(570),
		LastSeenAt:  tick(220),
		Timeout:     tick(350),
		Silence:     tick(350),
	}
	if reports[0] != want {
		t.Fatalf("report = %+v, want %+v", reports[0], want)
	}
	if len(res.Reports) != 1 {
		t.Fatalf("reports = %d, want only A's: B and C see a working path", len(res.Reports))
	}

	// The cut runs B->A only, so the far ends of it disagree: B keeps hearing A
	// and never doubts it, and C keeps hearing B on its own link.
	for _, observer := range []domain.NodeID{"B", "C"} {
		if n := len(res.ReportsFor(observer)); n != 0 {
			t.Fatalf("%s produced %d reports, want none", observer, n)
		}
	}
	if obs, ok := res.Belief("B", "A"); !ok || obs.Belief != domain.BeliefHealthy {
		t.Fatalf("B's belief about A = %+v, want healthy: that direction is never cut", obs)
	}
	if obs, ok := res.Belief("C", "B"); !ok || obs.Belief != domain.BeliefHealthy || obs.Heard != 11 {
		t.Fatalf("C's belief about B = %+v, want healthy after every round", obs)
	}

	// A ends the run trusting B again. The report is history, not a verdict.
	obs, ok := res.Belief("A", "B")
	if !ok || obs.Belief != domain.BeliefHealthy {
		t.Fatalf("A's final belief = %+v, want healthy after the repair", obs)
	}
	if obs.LastSeen != tick(1120) {
		t.Fatalf("A last saw B at %d, want the final round", obs.LastSeen)
	}
}

// TestRepairingThePathDoesNotRepairTheBelief is the second claim: topology and
// trust are different clocks. A only changes its mind when evidence arrives.
func TestRepairingThePathDoesNotRepairTheBelief(t *testing.T) {
	res := mustRun(t, OneWayPartition(), 1)

	var restored, cleared domain.Event
	var arrivals []domain.Tick
	for _, ev := range res.Log.Events() {
		switch {
		case ev.Kind == domain.KindLinkRestored:
			restored = ev
		case ev.Kind == domain.KindSuspicionCleared:
			cleared = ev
		case ev.Kind == domain.KindHeartbeatReceived && ev.Peer == "A":
			arrivals = append(arrivals, ev.At)
		}
	}

	if restored.Kind == "" || restored.At != tick(900) {
		t.Fatalf("restore event = %+v, want B->A restored at 900", restored)
	}
	if cleared.Kind == "" || cleared.At != tick(920) {
		t.Fatalf("cleared event = %+v, want A trusting B again at 920", cleared)
	}
	if got := cleared.At - restored.At; got != tick(20) {
		t.Fatalf("belief followed the repair after %d, want one link latency (%d)", got, tick(20))
	}

	// A heard nothing for 700 ms, and the repair alone did not change that: the
	// only thing between the repair and the cleared suspicion is a heartbeat.
	want := []domain.Tick{tick(120), tick(220), tick(920), tick(1020), tick(1120)}
	if !slices.Equal(arrivals, want) {
		t.Fatalf("A received at %v, want %v", arrivals, want)
	}
}

// TestPartitionLeavesTheReturnPathWorking checks the shape of the loss: only the
// cut direction loses messages, and only inside the cut.
func TestPartitionLeavesTheReturnPathWorking(t *testing.T) {
	res := mustRun(t, OneWayPartition(), 1)

	sentByA, receivedByB, drops := 0, 0, 0
	for _, ev := range res.Log.Events() {
		switch ev.Kind {
		case domain.KindHeartbeatSent:
			if ev.Node == "A" {
				sentByA++
			}
		case domain.KindHeartbeatReceived:
			if ev.Node == "A" {
				receivedByB++
			}
		case domain.KindMessageDropped:
			drops++
			if ev.Node != "B" || ev.Peer != "A" {
				t.Fatalf("%s->%s was dropped; only B->A is cut", ev.Node, ev.Peer)
			}
			if ev.Reason != domain.ReasonPartition {
				t.Fatalf("drop reason = %q, want %q", ev.Reason, domain.ReasonPartition)
			}
			if ev.At < tick(300) || ev.At >= tick(900) {
				t.Fatalf("message dropped at %d, outside the cut", ev.At)
			}
		}
	}
	if sentByA != receivedByB {
		t.Fatalf("A sent %d rounds and B received %d: the return path was cut too", sentByA, receivedByB)
	}
	if sentByA != 11 {
		t.Fatalf("A sent %d rounds in 1.15s at 100ms, want 11", sentByA)
	}
	if drops != 6 {
		t.Fatalf("drops = %d, want the six rounds sent during the cut", drops)
	}
}

func TestPartitionNamesMustResolve(t *testing.T) {
	cases := map[string]func(*Spec){
		"unknown sender": func(s *Spec) { s.Partitions[0].From = "D" },
		"unknown target": func(s *Spec) { s.Partitions[0].To = "D" },
		"invalid sender": func(s *Spec) { s.Partitions[0].From = "bad node" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := OneWayPartition()
			mutate(&spec)
			if _, err := Run(spec, 1); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("Run() = %v, want ErrInvalidSpec", err)
			}
		})
	}
}

func TestCutIntervalRulesStayWithTheSimulator(t *testing.T) {
	// The scenario resolves names; the interval itself is the network's rule, so
	// the error has to come from there rather than from a second copy of it.
	cases := map[string]struct {
		mutate func(*Spec)
		want   error
	}{
		"empty interval": {
			mutate: func(s *Spec) { s.Partitions[0].Until = s.Partitions[0].At },
			want:   sim.ErrCutOrder,
		},
		"reversed interval": {
			mutate: func(s *Spec) { s.Partitions[0].Until = tick(100) },
			want:   sim.ErrCutOrder,
		},
		"overlapping cuts": {
			mutate: func(s *Spec) {
				s.Partitions = append(s.Partitions, PartitionSpec{
					From: "B", To: "A", At: tick(800), Until: tick(1000),
				})
			},
			want: sim.ErrCutOverlap,
		},
		"undeclared link": {
			mutate: func(s *Spec) { s.Partitions[0].From, s.Partitions[0].To = "C", "A" },
			want:   sim.ErrUnknownLink,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			spec := OneWayPartition()
			tc.mutate(&spec)
			_, err := Run(spec, 1)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Run() = %v, want %v", err, tc.want)
			}
			if errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("error = %v, want the simulator's verdict, not the spec's", err)
			}
		})
	}
}

// TestOneWayPartitionMatchesTheCommittedLog pins the canonical history as bytes,
// which is what makes the fixture a proof rather than an example.
func TestOneWayPartitionMatchesTheCommittedLog(t *testing.T) {
	committed, err := os.ReadFile("testdata/one-way-partition.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got := jsonl(t, mustRun(t, OneWayPartition(), 0))
	if got != string(committed) {
		t.Fatalf("run does not match testdata/one-way-partition.jsonl:\n%s", got)
	}

	// No drop rate means no random draw, so the seed cannot leak into the log.
	if other := jsonl(t, mustRun(t, OneWayPartition(), 99)); other != got {
		t.Fatal("a scenario without random decisions depended on the seed")
	}
}
