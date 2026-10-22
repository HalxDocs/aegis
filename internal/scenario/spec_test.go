package scenario

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/sim"
)

func TestSpecValidationRejectsIncoherentWorlds(t *testing.T) {
	cases := map[string]func(*Spec){
		"no name":            func(s *Spec) { s.Name = "" },
		"zero horizon":       func(s *Spec) { s.Horizon = 0 },
		"negative horizon":   func(s *Spec) { s.Horizon = -1 },
		"no nodes":           func(s *Spec) { s.Nodes = nil },
		"invalid node id":    func(s *Spec) { s.Nodes[0].ID = "bad node" },
		"duplicate node":     func(s *Spec) { s.Nodes[2].ID = "A" },
		"unknown link start": func(s *Spec) { s.Links[0].From = "D" },
		"unknown link end":   func(s *Spec) { s.Links[0].To = "D" },
		"unknown observer":   func(s *Spec) { s.Observers[0].Observer = "D" },
		"unknown target":     func(s *Spec) { s.Observers[0].Target = "D" },
		"self observation":   func(s *Spec) { s.Observers[0].Target = "A" },
		"zero timeout":       func(s *Spec) { s.Observers[0].Timeout = 0 },
		"duplicate watch":    func(s *Spec) { s.Observers[1] = s.Observers[0] },
		"unknown crash node": func(s *Spec) { s.Crash.Node = "D" },
		"negative crash":     func(s *Spec) { s.Crash.At = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := NodeCrash()
			mutate(&spec)
			if err := spec.validate(); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("validate() = %v, want ErrInvalidSpec", err)
			}
			if _, err := Run(spec, 1); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("Run() = %v, want ErrInvalidSpec", err)
			}
		})
	}
}

func TestCanonicalSpecPassesValidation(t *testing.T) {
	if err := NodeCrash().validate(); err != nil {
		t.Fatalf("NodeCrash() is invalid: %v", err)
	}
}

func TestWorldWithoutTrafficNeedsNoObserver(t *testing.T) {
	// A world with nobody watching is simulable as long as nothing is sent: the
	// engine only insists on a receiver when a message actually arrives.
	spec := NodeCrash()
	spec.Observers = nil
	spec.Nodes = []sim.NodeConfig{{ID: "A"}, {ID: "B"}}
	spec.Links = nil
	spec.Crash = nil

	if err := spec.validate(); err != nil {
		t.Fatalf("validate() = %v", err)
	}
	res, err := Run(spec, 1)
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(res.Beliefs) != 0 || len(res.Reports) != 0 {
		t.Fatalf("beliefs = %v, reports = %v; want none", res.Beliefs, res.Reports)
	}
}

func TestTrafficWithNobodyWatchingFailsTheRun(t *testing.T) {
	// Silence would otherwise look like a successful run while every message
	// fell into nowhere, so the engine refuses to deliver to no receiver.
	spec := NodeCrash()
	spec.Observers = nil
	if _, err := Run(spec, 1); !errors.Is(err, sim.ErrNoReceiver) {
		t.Fatalf("Run() = %v, want ErrNoReceiver", err)
	}
}

func TestNodeAndLinkRulesStayWithTheSimulator(t *testing.T) {
	// The scenario package deliberately does not re-check these: the engine is
	// the single authority, and the error must come from there rather than from
	// a second copy of the rule that can drift.
	spec := NodeCrash()
	spec.Nodes[1] = sim.NodeConfig{ID: "B", HeartbeatEvery: tick(100), Peers: []domain.NodeID{"A"}}
	spec.Links[1] = sim.LinkConfig{From: "B", To: "C", Latency: -1}
	if _, err := Run(spec, 1); err == nil {
		t.Fatal("a negative link latency must fail the run")
	} else if errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("error = %v, want the simulator's verdict, not the spec's", err)
	}
}

func TestRunSnapshotsTheSpecItExecuted(t *testing.T) {
	spec := NodeCrash()
	res, err := Run(spec, 1)
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}

	// Editing the caller's value must not rewrite a run that already happened.
	spec.Name = "tampered"
	spec.Nodes[1].Peers[0] = "C"
	spec.Observers[0].Timeout = 1
	if res.Spec.Name != "node-crash" {
		t.Fatalf("result name = %q, want the executed scenario", res.Spec.Name)
	}
	if got := res.Spec.Nodes[1].Peers[0]; got != "A" {
		t.Fatalf("result peers = %v, want the executed peer list", res.Spec.Nodes[1].Peers)
	}
	if got := res.Spec.Observers[0].Timeout; got != tick(350) {
		t.Fatalf("result timeout = %d, want 350ms", got)
	}

	// The sharing must not go the other way either.
	res.Spec.Nodes[0].ID = "Z"
	res.Spec.Crash.At = 0
	if spec.Nodes[0].ID != "A" || spec.Crash.At != tick(215) {
		t.Fatalf("caller's spec was mutated through the result: %+v", spec.Nodes[0])
	}
}
