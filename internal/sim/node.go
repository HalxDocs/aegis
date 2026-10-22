package sim

import (
	"fmt"
	"slices"

	"github.com/HalxDocs/aegis/internal/domain"
)

// NodeConfig describes a node at configuration time.
//
// HeartbeatEvery is the interval between heartbeat rounds; zero means the node
// never sends. Peers must be empty in that case, because a peer list without an
// interval describes a node that has been configured to do something impossible.
type NodeConfig struct {
	ID             domain.NodeID
	HeartbeatEvery domain.Tick
	Peers          []domain.NodeID
}

func (c NodeConfig) validate() error {
	if err := c.ID.Validate(); err != nil {
		return fmt.Errorf("node: %w", err)
	}
	if c.HeartbeatEvery < 0 {
		return fmt.Errorf("node %s: heartbeat interval %d is negative", c.ID, c.HeartbeatEvery)
	}
	if c.HeartbeatEvery == 0 && len(c.Peers) > 0 {
		return fmt.Errorf("node %s: peers configured without a heartbeat interval", c.ID)
	}
	if c.HeartbeatEvery > 0 && len(c.Peers) == 0 {
		return fmt.Errorf("node %s: heartbeat interval configured without peers", c.ID)
	}
	seen := make(map[domain.NodeID]struct{}, len(c.Peers))
	for _, peer := range c.Peers {
		if err := peer.Validate(); err != nil {
			return fmt.Errorf("node %s: peer: %w", c.ID, err)
		}
		if peer == c.ID {
			return fmt.Errorf("node %s: cannot heartbeat itself", c.ID)
		}
		if _, duplicate := seen[peer]; duplicate {
			return fmt.Errorf("node %s: peer %s is listed twice", c.ID, peer)
		}
		seen[peer] = struct{}{}
	}
	return nil
}

// node is the simulator's private view of one participant. Its state field is
// ground truth; nothing outside this package may read it except through
// Engine.Truth, and the detector is given no access at all.
type node struct {
	id    domain.NodeID
	state domain.NodeState
	every domain.Tick
	peers []domain.NodeID
	round uint64
	timer domain.Timer
	eng   *Engine
}

func newNode(cfg NodeConfig, eng *Engine) *node {
	peers := slices.Clone(cfg.Peers)
	slices.Sort(peers)
	return &node{
		id:    cfg.ID,
		state: domain.StateRunning,
		every: cfg.HeartbeatEvery,
		peers: peers,
		eng:   eng,
	}
}

// start records the node as running and arms its first heartbeat round.
func (n *node) start(at domain.Tick) error {
	if n.state != domain.StateRunning {
		return fmt.Errorf("node %s: cannot start while %s", n.id, n.state)
	}
	if err := n.eng.record(domain.Event{At: at, Kind: domain.KindNodeStarted, Node: n.id}); err != nil {
		return fmt.Errorf("node %s: record start: %w", n.id, err)
	}
	if n.every == 0 {
		return nil
	}
	return n.scheduleHeartbeat(at)
}

func (n *node) scheduleHeartbeat(from domain.Tick) error {
	at := from + n.every
	timer, err := n.eng.sched.At(at, func() { n.eng.fail(n.beat(at)) })
	if err != nil {
		return fmt.Errorf("node %s: schedule heartbeat: %w", n.id, err)
	}
	n.timer = timer
	return nil
}

// beat sends one heartbeat round to every peer.
//
// Peers are visited in sorted order. The same order decides message sequence
// numbers on a zero-latency link, so an unsorted peer list would make the log
// depend on map iteration or configuration order.
func (n *node) beat(at domain.Tick) error {
	if n.state != domain.StateRunning {
		return fmt.Errorf("node %s: heartbeat at %d while %s", n.id, at, n.state)
	}
	n.round++
	for _, peer := range n.peers {
		msg := Message{From: n.id, To: peer, Round: n.round}
		if err := n.eng.net.send(at, msg); err != nil {
			return fmt.Errorf("node %s: heartbeat round %d: %w", n.id, n.round, err)
		}
	}
	return n.scheduleHeartbeat(at)
}

// crash moves the node to its terminal state and stops its own timers.
//
// Messages already inside the network keep travelling: the network has no way
// to retract what it accepted, and pretending otherwise would remove exactly
// the subtle window this milestone exists to show. Nothing outside the
// simulator is told about the crash, because an observer that was told would no
// longer be observing.
func (n *node) crash(at domain.Tick) error {
	next, err := n.state.TransitionTo(domain.StateCrashed)
	if err != nil {
		return fmt.Errorf("node %s: crash: %w", n.id, err)
	}
	n.state = next
	if n.timer != nil {
		n.timer.Cancel()
	}
	if err := n.eng.record(domain.Event{At: at, Kind: domain.KindNodeCrashed, Node: n.id}); err != nil {
		return fmt.Errorf("node %s: record crash: %w", n.id, err)
	}
	return nil
}
