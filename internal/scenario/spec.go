package scenario

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/recovery"
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
//
// Recovery is optional in the same sense: without it a scenario observes and
// records, and the crash it schedules is never repaired. A scenario that wants
// the repair has to state the bar for it, because a default policy would be a
// decision the scenario did not make.
type Spec struct {
	Name       string
	Nodes      []sim.NodeConfig
	Links      []sim.LinkConfig
	Observers  []ObserverSpec
	Crash      *CrashSpec
	Partitions []PartitionSpec
	Recovery   *RecoverySpec
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
	if s.Recovery != nil {
		recov := *s.Recovery
		out.Recovery = &recov
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

	if s.Recovery != nil {
		if err := s.Recovery.validate(); err != nil {
			return err
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

// OneWayPartition is the canonical Milestone 2 scenario.
//
// Nothing crashes. A and B heartbeat each other every 100 ms and C watches B, so
// every node is running for the whole run. At 300 ms the B->A direction is cut
// and at 900 ms it is repaired; the A->B direction is untouched throughout.
//
// The result is the first history in which nobody is broken and somebody is
// wrong:
//
//   - A stops hearing B, waits its full 350 ms and suspects B at 570 ms.
//   - B keeps hearing A the whole time and never doubts it.
//   - C, on its own link, keeps hearing B and never doubts anything.
//
// Repairing the path at 900 ms does not repair the belief. A only changes its
// mind at 920 ms, when the first heartbeat after the repair actually arrives:
// 350 ms of suspicion against a node that was never down. That is the milestone
// in one number — trust is made of evidence, not of topology.
func OneWayPartition() Spec {
	return Spec{
		Name: "one-way-partition",
		Nodes: []sim.NodeConfig{
			{
				ID:             "A",
				HeartbeatEvery: domain.TickFrom(100 * time.Millisecond),
				Peers:          []domain.NodeID{"B"},
			},
			{
				ID:             "B",
				HeartbeatEvery: domain.TickFrom(100 * time.Millisecond),
				Peers:          []domain.NodeID{"A", "C"},
			},
			{ID: "C"},
		},
		Links: []sim.LinkConfig{
			{From: "A", To: "B", Latency: domain.TickFrom(20 * time.Millisecond)},
			{From: "B", To: "A", Latency: domain.TickFrom(20 * time.Millisecond)},
			{From: "B", To: "C", Latency: domain.TickFrom(25 * time.Millisecond)},
		},
		Observers: []ObserverSpec{
			{Observer: "A", Target: "B", Timeout: domain.TickFrom(350 * time.Millisecond)},
			{Observer: "B", Target: "A", Timeout: domain.TickFrom(350 * time.Millisecond)},
			{Observer: "C", Target: "B", Timeout: domain.TickFrom(350 * time.Millisecond)},
		},
		Partitions: []PartitionSpec{
			{From: "B", To: "A", At: domain.TickFrom(300 * time.Millisecond), Until: domain.TickFrom(900 * time.Millisecond)},
		},
		// The horizon sits just past the last arrival, so the run ends with an
		// empty wire: every message in the log has an outcome, and none of them
		// is still on a path when the history closes.
		Horizon: domain.TickFrom(1150 * time.Millisecond),
	}
}

// recoveryPolicy is the bar both Milestone 4 scenarios hold an action to.
//
// Two witnesses, fifty milliseconds of patience and a hundred of delay. The
// numbers are chosen so the timing of the log is readable: A reports at 570 ms
// and C at 575 ms, so the review at 620 ms has both, and the restart lands at
// 720 ms — comfortably inside the run and well after the decision that caused
// it.
func recoveryPolicy() *RecoverySpec {
	return &RecoverySpec{Policy: recovery.Policy{
		Kind:         domain.ActionRestartNode,
		MinWitnesses: 2,
		Patience:     domain.TickFrom(50 * time.Millisecond),
		Delay:        domain.TickFrom(100 * time.Millisecond),
	}}
}

// RecoveredCrash is the canonical Milestone 4 scenario: the same crash as
// Milestone 1, with a recovery loop allowed to act on the reports.
//
// The run tells the whole story in one log:
//
//   - B crashes at 215 ms, after its round two messages have left.
//   - A reports at 570 ms and C at 575 ms, exactly as in node-crash.
//   - The gate reviews at 620 ms, two observers are enough, and the action is
//     proposed and then carried out at 720 ms.
//   - B resumes at round three — not round one — at 820 ms, and A's belief is
//     cleared at 840 ms when the message actually arrives.
//
// The gap between 720 and 840 is the milestone: a restored node is not a
// restored belief. A only stops suspecting B when evidence reaches it, which is
// the same rule that made it suspect B in the first place.
func RecoveredCrash() Spec {
	spec := NodeCrash()
	spec.Name = "recovered-crash"
	spec.Horizon = domain.TickFrom(1200 * time.Millisecond)
	spec.Recovery = recoveryPolicy()
	return spec
}

// LoneWitness is the refusal scenario.
//
// The crash is the same and the policy is the same; only the world is smaller.
// With C gone, A is the single node that ever reports, and the policy wants two.
// At 620 ms the gate refuses with insufficient_witnesses, B stays down for the
// rest of the run, and the log ends with a written decision not to act.
//
// C is removed from the world rather than left in it watching nothing. A receipt
// is recorded by the observer that accepts it, so traffic to a node nobody
// watches would appear in the log as a send with no outcome — indistinguishable
// from a message still on the wire. A scenario about a single piece of evidence
// must not manufacture evidence nobody keeps.
//
// A refusal is the reason this scenario exists. Without it, "two witnesses are
// required" would be an untested claim, and a single observer's silence would
// look indistinguishable from a dead node.
func LoneWitness() Spec {
	return Spec{
		Name: "lone-witness",
		Nodes: []sim.NodeConfig{
			{ID: "A"},
			{
				ID:             "B",
				HeartbeatEvery: domain.TickFrom(100 * time.Millisecond),
				Peers:          []domain.NodeID{"A"},
			},
		},
		Links: []sim.LinkConfig{
			{From: "B", To: "A", Latency: domain.TickFrom(20 * time.Millisecond)},
		},
		Observers: []ObserverSpec{
			{Observer: "A", Target: "B", Timeout: domain.TickFrom(350 * time.Millisecond)},
		},
		Crash:    &CrashSpec{Node: "B", At: domain.TickFrom(215 * time.Millisecond)},
		Horizon:  domain.TickFrom(1200 * time.Millisecond),
		Recovery: recoveryPolicy(),
	}
}
