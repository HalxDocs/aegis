package sim

import (
	"errors"
	"fmt"
	"slices"

	"github.com/HalxDocs/aegis/internal/domain"
)

var (
	// ErrNoRecorder is returned when a simulator is created without a history.
	ErrNoRecorder = errors.New("simulator requires an event recorder")

	// ErrNoReceiver is returned when messages arrive with no observer attached.
	ErrNoReceiver = errors.New("simulator requires a message receiver")

	// ErrUnknownNode is returned for an identifier the simulator never learned.
	ErrUnknownNode = errors.New("unknown node")

	// ErrDuplicateNode is returned when a node is configured twice.
	ErrDuplicateNode = errors.New("node is already configured")

	// ErrAlreadyStarted is returned when a run is started twice.
	ErrAlreadyStarted = errors.New("simulator has already started")
)

// EventRecorder is the append-only history the simulator writes to. It is
// declared here, at the consumer, so this package depends on the behaviour it
// needs rather than on a concrete recorder implementation.
//
// Record returns the reference the recorder assigned, because a later event may
// need to cite the one just written.
type EventRecorder interface {
	Record(domain.Event) (domain.Ref, error)
}

// Receiver is told about messages that actually arrived. A detector is the only
// implementation; the simulator never decides what an observer believes.
//
// Every receiver sees every delivery and decides for itself whether the message
// concerns it, because the simulator knows nothing about who is watching whom.
type Receiver interface {
	HeartbeatReceived(at domain.Tick, from, to domain.NodeID, round uint64) error
}

// EngineConfig configures a simulator run.
//
// Seed drives every random decision. StepLimit bounds executed tasks; zero
// selects DefaultStepLimit.
type EngineConfig struct {
	Recorder  EventRecorder
	Seed      uint64
	StepLimit int
}

// Engine owns virtual time, the event queue, the network and ground truth.
type Engine struct {
	rec       EventRecorder
	sched     *scheduler
	net       *network
	nodes     map[domain.NodeID]*node
	order     []domain.NodeID
	receivers []Receiver
	err       error
	started   bool
}

// Engine is usable anywhere a clock or a scheduler is expected. The detector
// receives exactly these two capabilities and nothing else.
var (
	_ domain.Clock     = (*Engine)(nil)
	_ domain.Scheduler = (*Engine)(nil)
)

// NewEngine validates the configuration and returns an idle simulator.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	if cfg.Recorder == nil {
		return nil, ErrNoRecorder
	}
	e := &Engine{
		rec:   cfg.Recorder,
		sched: newScheduler(cfg.StepLimit),
		nodes: make(map[domain.NodeID]*node),
	}
	e.net = newNetwork(e.sched, cfg.Recorder, newRNG(cfg.Seed))
	e.net.deliver = e.deliver
	e.net.fault = e.fail
	return e, nil
}

// AddNode validates and registers a node. Nodes are kept in sorted order so
// start-up events never depend on configuration order.
func (e *Engine) AddNode(cfg NodeConfig) error {
	if e.started {
		return ErrAlreadyStarted
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	if _, exists := e.nodes[cfg.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateNode, cfg.ID)
	}
	e.nodes[cfg.ID] = newNode(cfg, e)
	e.order = append(e.order, cfg.ID)
	slices.Sort(e.order)
	return nil
}

// AddLink configures one directed network path.
func (e *Engine) AddLink(cfg LinkConfig) error {
	if e.started {
		return ErrAlreadyStarted
	}
	return e.net.addLink(cfg)
}

// AddReceiver registers a component that observes delivered messages.
//
// Receivers are called in registration order for every delivery. Two observers
// watching the same sender therefore write their receipts in a fixed sequence,
// which keeps the log reproducible no matter how many observers exist.
func (e *Engine) AddReceiver(r Receiver) error {
	if r == nil {
		return ErrNoReceiver
	}
	e.receivers = append(e.receivers, r)
	return nil
}

// Nodes returns the configured node identifiers in sorted order.
func (e *Engine) Nodes() []domain.NodeID { return slices.Clone(e.order) }

// Now returns the current virtual time, in nanoseconds since the run started.
func (e *Engine) Now() domain.Tick { return e.sched.now }

// At schedules work at a virtual time, rejecting the past.
func (e *Engine) At(at domain.Tick, fn func()) (domain.Timer, error) { return e.sched.At(at, fn) }

// AtEnd schedules work after all ordinary work at the same virtual instant.
func (e *Engine) AtEnd(at domain.Tick, fn func()) (domain.Timer, error) { return e.sched.AtEnd(at, fn) }

// Truth reports a node's actual execution state. Tests and the CLI use it to
// compare ground truth with observer belief; the detector has no access to it.
func (e *Engine) Truth(id domain.NodeID) (domain.NodeState, bool) {
	n, ok := e.nodes[id]
	if !ok {
		return "", false
	}
	return n.state, true
}

// Start records every node as running and arms its heartbeat timer.
//
// Peer references are resolved here rather than in AddNode so a scenario may
// configure nodes in any order; a peer that never appears is a configuration
// error and stops the run before any event is recorded.
func (e *Engine) Start() error {
	if e.started {
		return ErrAlreadyStarted
	}
	for _, id := range e.order {
		n := e.nodes[id]
		for _, peer := range n.peers {
			if _, ok := e.nodes[peer]; !ok {
				return fmt.Errorf("%w: node %s heartbeats unknown peer %s", ErrUnknownNode, n.id, peer)
			}
		}
	}
	e.started = true
	at := e.sched.now
	for _, id := range e.order {
		if err := e.nodes[id].start(at); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) record(ev domain.Event) (domain.Ref, error) { return e.rec.Record(ev) }

// Fail records err as the run's first fault. Components that schedule their own
// work report failures here, because a scheduled callback has no caller to
// return an error to and silently losing a write would make the log lie.
func (e *Engine) Fail(err error) { e.fail(err) }

// fail keeps the first error raised inside a scheduled task. Tasks return
// nothing, so the run loop is the only place an error can be reported without
// stopping the queue mid-step and losing ordering information.
func (e *Engine) fail(err error) {
	if err != nil && e.err == nil {
		e.err = err
	}
}

func (e *Engine) deliver(at domain.Tick, msg Message) {
	if len(e.receivers) == 0 {
		e.fail(ErrNoReceiver)
		return
	}
	for _, r := range e.receivers {
		if err := r.HeartbeatReceived(at, msg.From, msg.To, msg.Round); err != nil {
			e.fail(fmt.Errorf("deliver heartbeat %s->%s round %d: %w", msg.From, msg.To, msg.Round, err))
			return
		}
	}
}
