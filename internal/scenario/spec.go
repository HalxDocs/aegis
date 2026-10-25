package scenario

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/sim"
)

// ErrInvalidSpec identifies a scenario that cannot describe a world.
var ErrInvalidSpec = errors.New("invalid scenario")

// ObserverSpec is one node watching another with its own timeout.
//
// The timeout lives per observer, not per target: two observers on different
// links legitimately disagree about how long silence is suspicious, and a
// shared timeout would erase exactly the difference this milestone measures.
type ObserverSpec struct {
	Observer domain.NodeID
	Target   domain.NodeID
	Timeout  domain.Tick
}

// CrashSpec is a scheduled change in ground truth.
type CrashSpec struct {
	Node domain.NodeID
	At   domain.Tick
}

// PartitionSpec is a scheduled one-way outage of a declared link.
//
// The interval is half-open: the path is unusable from At up to, but not
// including, Until. Only the From->To direction is affected, which is what makes
// the two ends of one link disagree about who is alive.
//
// Everything about the interval itself — that it ends after it begins, that it
// does not overlap another cut, that the link exists — is checked by the
// simulator. A scenario only resolves names.
type PartitionSpec struct {
	From  domain.NodeID
	To    domain.NodeID
	At    domain.Tick
	Until domain.Tick
}

// Spec describes one run.
//
// Nodes and links reuse the simulator's configuration types so a scenario
// cannot express a setting the simulator does not understand. Crash is optional:
// a scenario with no crash is how the baseline "nothing goes wrong" run is
// expressed.
type Spec struct {
	Name       string
	Nodes      []sim.NodeConfig
	Links      []sim.LinkConfig
	Observers  []ObserverSpec
	Crash      *CrashSpec
	Partitions []PartitionSpec
	Horizon    domain.Tick
}

// clone returns a snapshot that shares no mutable state with the receiver.
//
// A run keeps the spec it executed so the result can explain itself, and a
// caller that reuses and edits its Spec afterwards must not be able to rewrite
// the history of a run that already happened.
func (s Spec) clone() Spec {
	out := s
	out.Nodes = make([]sim.NodeConfig, len(s.Nodes))
	for i, cfg := range s.Nodes {
		cfg.Peers = slices.Clone(cfg.Peers)
		out.Nodes[i] = cfg
	}
	out.Links = slices.Clone(s.Links)
	out.Observers = slices.Clone(s.Observers)
	out.Partitions = slices.Clone(s.Partitions)
	if s.Crash != nil {
		crash := *s.Crash
		out.Crash = &crash
	}
	return out
}

// validate rejects a scenario before any of it is simulated.
//
// It checks only what the components cannot check for themselves: that names
// refer to configured nodes and that the run has somewhere to stop. Interval,
// link and transition rules stay in the packages that own them, so there is one
// authority per rule rather than two that can drift apart.
func (s Spec) validate() error {
	if s.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidSpec)
	}
	if s.Horizon <= 0 {
		return fmt.Errorf("%w: horizon %d is not positive", ErrInvalidSpec, s.Horizon)
	}
	if len(s.Nodes) == 0 {
		return fmt.Errorf("%w: no nodes configured", ErrInvalidSpec)
	}

	nodes := make(map[domain.NodeID]struct{}, len(s.Nodes))
	for _, cfg := range s.Nodes {
		if err := cfg.ID.Validate(); err != nil {
			return fmt.Errorf("%w: node: %w", ErrInvalidSpec, err)
		}
		if _, duplicate := nodes[cfg.ID]; duplicate {
			return fmt.Errorf("%w: node %s is configured twice", ErrInvalidSpec, cfg.ID)
		}
		nodes[cfg.ID] = struct{}{}
	}

	for _, link := range s.Links {
		if _, ok := nodes[link.From]; !ok {
			return fmt.Errorf("%w: link %s->%s starts at an unconfigured node", ErrInvalidSpec, link.From, link.To)
		}
		if _, ok := nodes[link.To]; !ok {
			return fmt.Errorf("%w: link %s->%s ends at an unconfigured node", ErrInvalidSpec, link.From, link.To)
		}
	}

	watched := make(map[[2]domain.NodeID]struct{}, len(s.Observers))
	for _, o := range s.Observers {
		if err := o.Observer.Validate(); err != nil {
			return fmt.Errorf("%w: observer: %w", ErrInvalidSpec, err)
		}
		if err := o.Target.Validate(); err != nil {
			return fmt.Errorf("%w: observed target: %w", ErrInvalidSpec, err)
		}
		if _, ok := nodes[o.Observer]; !ok {
			return fmt.Errorf("%w: observer %s is not a configured node", ErrInvalidSpec, o.Observer)
		}
		if _, ok := nodes[o.Target]; !ok {
			return fmt.Errorf("%w: target %s is not a configured node", ErrInvalidSpec, o.Target)
		}
		if o.Observer == o.Target {
			return fmt.Errorf("%w: %s cannot watch itself", ErrInvalidSpec, o.Observer)
		}
		if o.Timeout <= 0 {
			return fmt.Errorf("%w: observer %s has timeout %d, which is not positive", ErrInvalidSpec, o.Observer, o.Timeout)
		}
		key := [2]domain.NodeID{o.Observer, o.Target}
		if _, duplicate := watched[key]; duplicate {
			return fmt.Errorf("%w: %s watches %s twice", ErrInvalidSpec, o.Observer, o.Target)
		}
		watched[key] = struct{}{}
	}

	if s.Crash != nil {
		if err := s.Crash.Node.Validate(); err != nil {
			return fmt.Errorf("%w: crash target: %w", ErrInvalidSpec, err)
		}
		if _, ok := nodes[s.Crash.Node]; !ok {
			return fmt.Errorf("%w: crash target %s is not a configured node", ErrInvalidSpec, s.Crash.Node)
		}
		if s.Crash.At < 0 {
			return fmt.Errorf("%w: crash at %d is in the past", ErrInvalidSpec, s.Crash.At)
		}
	}

	for _, p := range s.Partitions {
		if err := p.From.Validate(); err != nil {
			return fmt.Errorf("%w: partition: %w", ErrInvalidSpec, err)
		}
		if err := p.To.Validate(); err != nil {
			return fmt.Errorf("%w: partition target: %w", ErrInvalidSpec, err)
		}
		if _, ok := nodes[p.From]; !ok {
			return fmt.Errorf("%w: partition starts at unconfigured node %s", ErrInvalidSpec, p.From)
		}
		if _, ok := nodes[p.To]; !ok {
			return fmt.Errorf("%w: partition ends at unconfigured node %s", ErrInvalidSpec, p.To)
		}
	}
	return nil
}

// NodeCrash is the canonical Milestone 1 scenario.
//
// Three nodes, one sender, two observers on links of different length, and a
// crash at 215 ms. The crash lands between heartbeat rounds and after the round
// two messages have left the sender, so both observers' last evidence is newer
// than the crash itself — the detail that makes suspicion latency 355 ms rather
// than the configured 350 ms.
func NodeCrash() Spec {
	return Spec{
		Name: "node-crash",
		Nodes: []sim.NodeConfig{
			{ID: "A"},
			{
				ID:             "B",
				HeartbeatEvery: domain.TickFrom(100 * time.Millisecond),
				Peers:          []domain.NodeID{"A", "C"},
			},
			{ID: "C"},
		},
		Links: []sim.LinkConfig{
			{From: "B", To: "A", Latency: domain.TickFrom(20 * time.Millisecond)},
			{From: "B", To: "C", Latency: domain.TickFrom(25 * time.Millisecond)},
		},
		Observers: []ObserverSpec{
			{Observer: "A", Target: "B", Timeout: domain.TickFrom(350 * time.Millisecond)},
			{Observer: "C", Target: "B", Timeout: domain.TickFrom(350 * time.Millisecond)},
		},
		Crash:   &CrashSpec{Node: "B", At: domain.TickFrom(215 * time.Millisecond)},
		Horizon: domain.TickFrom(1200 * time.Millisecond),
	}
}
