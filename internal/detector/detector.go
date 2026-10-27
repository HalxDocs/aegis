package detector

import (
	"errors"
	"fmt"
	"slices"

	"github.com/HalxDocs/aegis/internal/domain"
)

var (
	// ErrNoClock is returned when a detector is created without a clock.
	ErrNoClock = errors.New("detector requires a clock")

	// ErrNoScheduler is returned when a detector is created without a scheduler.
	ErrNoScheduler = errors.New("detector requires a scheduler")

	// ErrNoRecorder is returned when a detector is created without a recorder.
	ErrNoRecorder = errors.New("detector requires an event recorder")

	// ErrNoFaultSink is returned when scheduled work has nowhere to report a
	// failure. Without a sink, a recorder error raised inside a deadline
	// callback would disappear and the run would claim success.
	ErrNoFaultSink = errors.New("detector requires a fault sink")

	// ErrInvalidTimeout is returned for a timeout that is not positive.
	ErrInvalidTimeout = errors.New("detector timeout must be positive")

	// ErrNoRoot is returned when a detector is created without the reference of
	// the event that opened the history. A deadline armed before any evidence has
	// to cite something, or the suspicion that follows it has no cause.
	ErrNoRoot = errors.New("detector requires the root of the history")

	// ErrAlreadyWatched is returned when a target is watched twice by the same
	// observer, which would leave two deadlines racing for one belief.
	ErrAlreadyWatched = errors.New("target is already watched")
)

// EventRecorder is the append-only history the detector writes to. It is
// declared here, at the consumer, so this package never learns how events are
// stored.
type EventRecorder interface {
	Record(domain.Event) (domain.Ref, error)
}

// FaultSink receives errors raised inside scheduled work, where there is no
// caller to return them to. The simulator is the implementation: it keeps the
// first fault and aborts the run with it.
type FaultSink interface {
	Fail(error)
}

// Config configures one observer's detector.
//
// Timeout is the silence an observer tolerates before it stops trusting a
// target. It is measured from the last message the observer actually received,
// never from the last message a target claims to have sent.
type Config struct {
	Observer  domain.NodeID
	Timeout   domain.Tick
	Clock     domain.Clock
	Scheduler domain.Scheduler
	Recorder  EventRecorder
	Faults    FaultSink

	// Root is the reference of the scenario_started event. It is the cause of a
	// suspicion whose deadline was armed before any message arrived.
	Root domain.Ref
}

// Detector holds one observer's beliefs about the targets it was told to watch.
//
// It is not safe for concurrent use. A run is single-goroutine by design, and
// isolation between concurrent runs comes from giving each run its own
// Detector.
type Detector struct {
	observer domain.NodeID
	timeout  domain.Tick
	clock    domain.Clock
	sched    domain.Scheduler
	rec      EventRecorder
	faults   FaultSink

	monitors map[domain.NodeID]*monitor
	targets  []domain.NodeID
	root     domain.Ref
	reports  []domain.FailureReport
}

// New validates the configuration and returns a detector that watches nothing.
func New(cfg Config) (*Detector, error) {
	if err := cfg.Observer.Validate(); err != nil {
		return nil, fmt.Errorf("detector: observer: %w", err)
	}
	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidTimeout, cfg.Timeout)
	}
	if cfg.Clock == nil {
		return nil, ErrNoClock
	}
	if cfg.Scheduler == nil {
		return nil, ErrNoScheduler
	}
	if cfg.Recorder == nil {
		return nil, ErrNoRecorder
	}
	if cfg.Faults == nil {
		return nil, ErrNoFaultSink
	}
	if !cfg.Root.Valid() {
		return nil, ErrNoRoot
	}
	return &Detector{
		observer: cfg.Observer,
		timeout:  cfg.Timeout,
		clock:    cfg.Clock,
		sched:    cfg.Scheduler,
		rec:      cfg.Recorder,
		faults:   cfg.Faults,
		root:     cfg.Root,
		monitors: make(map[domain.NodeID]*monitor),
	}, nil
}

// Observer returns the node whose beliefs this detector holds.
func (d *Detector) Observer() domain.NodeID { return d.observer }

// Timeout returns the silence this observer tolerates.
func (d *Detector) Timeout() domain.Tick { return d.timeout }

// Watch starts observing target, beginning with a healthy belief and a deadline
// one timeout from now.
//
// The initial deadline matters: an observer that has never heard from a target
// must still stop trusting it, otherwise a node that dies before its first
// heartbeat would be believed healthy forever.
//
// Watching is therefore allowed at any virtual time. A target that is never
// heard from produces a report whose silence is measured from the start of the
// run, because there is no arrival to measure from; the milestone's scenarios
// watch from tick zero, where that is exact.
func (d *Detector) Watch(target domain.NodeID) error {
	if err := target.Validate(); err != nil {
		return fmt.Errorf("detector %s: watch: %w", d.observer, err)
	}
	if target == d.observer {
		return fmt.Errorf("detector %s: cannot watch itself", d.observer)
	}
	if _, exists := d.monitors[target]; exists {
		return fmt.Errorf("%w: %s watches %s", ErrAlreadyWatched, d.observer, target)
	}

	m := &monitor{obs: domain.NewObservation(d.observer, target), evidence: d.root}
	d.monitors[target] = m
	d.targets = append(d.targets, target)
	slices.Sort(d.targets)
	return d.arm(target, m, d.clock.Now())
}

// HeartbeatReceived implements the simulator's Receiver.
//
// Messages addressed to another observer are ignored entirely: this detector
// must not borrow evidence that was never delivered to it. A message from a
// sender that is not watched is still recorded as a receipt, because the
// history is a record of what arrived, not only of what mattered.
func (d *Detector) HeartbeatReceived(at domain.Tick, from, to domain.NodeID, round uint64, cause domain.Ref) error {
	if to != d.observer {
		return nil
	}
	receipt, err := d.rec.Record(domain.Event{
		At:     at,
		Kind:   domain.KindHeartbeatReceived,
		Node:   from,
		Peer:   to,
		Round:  round,
		Causes: []domain.Ref{cause},
	})
	if err != nil {
		return fmt.Errorf("detector %s: record receipt from %s: %w", d.observer, from, err)
	}

	m, ok := d.monitors[from]
	if !ok {
		return nil
	}
	next, err := m.obs.Observed(at)
	if err != nil {
		return fmt.Errorf("detector %s: %w", d.observer, err)
	}
	if m.obs.Belief == domain.BeliefSuspected && next.Belief == domain.BeliefHealthy {
		if _, err := d.rec.Record(domain.Event{
			At:   at,
			Kind: domain.KindSuspicionCleared,
			Node: from,
			Peer: to,
		}); err != nil {
			return fmt.Errorf("detector %s: record recovery of %s: %w", d.observer, from, err)
		}
	}
	m.obs = next
	// This receipt re-arms the deadline, so it is the evidence a later
	// suspicion will cite.
	m.evidence = receipt
	return d.arm(from, m, at)
}

// Observation returns this observer's belief about a target.
func (d *Detector) Observation(target domain.NodeID) (domain.Observation, bool) {
	m, ok := d.monitors[target]
	if !ok {
		return domain.Observation{}, false
	}
	return m.obs, true
}

// Targets returns the watched targets in sorted order.
func (d *Detector) Targets() []domain.NodeID { return slices.Clone(d.targets) }

// Reports returns the failure reports this observer produced, in the order it
// produced them.
func (d *Detector) Reports() []domain.FailureReport { return slices.Clone(d.reports) }
