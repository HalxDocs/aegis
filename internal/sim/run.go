package sim

import (
	"errors"
	"fmt"

	"github.com/HalxDocs/aegis/internal/domain"
)

var (
	// ErrNotStarted is returned when a run begins before Start.
	ErrNotStarted = errors.New("simulator has not been started")

	// ErrHorizonBeforeNow is returned when a run asks to move backwards.
	ErrHorizonBeforeNow = errors.New("run horizon precedes the current time")

	// ErrNotCrashed is returned when an action asks to restart a node that is
	// still running. It is an error rather than a no-op because an action is
	// only ever granted against a failure report, and restarting a healthy node
	// would mean the evidence and the world disagreed.
	ErrNotCrashed = errors.New("node is not crashed")
)

// Run executes scheduled work until the horizon, then parks the clock there.
//
// Work scheduled exactly at the horizon is included; the horizon is an instant
// the modelled world reaches, not an instant it stops just short of. Errors
// raised inside tasks surface here rather than being swallowed by the queue.
func (e *Engine) Run(horizon domain.Tick) error {
	if !e.started {
		return ErrNotStarted
	}
	if horizon < e.sched.now {
		return fmt.Errorf("%w: horizon %d, now %d", ErrHorizonBeforeNow, horizon, e.sched.now)
	}
	for {
		if e.err != nil {
			return e.err
		}
		next := e.sched.next()
		if next == nil || next.at > horizon {
			break
		}
		if err := e.sched.step(); err != nil {
			return err
		}
	}
	if e.err != nil {
		return e.err
	}
	e.sched.now = horizon
	return nil
}

// CrashAt schedules a node's crash at a virtual time.
//
// The crash is scheduled like any other event, so it lands in the log in
// timestamp order alongside the messages it interrupts. A crash that is already
// in the past is rejected by the scheduler rather than applied retroactively.
func (e *Engine) CrashAt(at domain.Tick, id domain.NodeID) error {
	n, ok := e.nodes[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownNode, id)
	}
	if _, err := e.sched.At(at, func() { e.fail(n.crash(at)) }); err != nil {
		return fmt.Errorf("schedule crash of %s at %d: %w", id, at, err)
	}
	return nil
}

// Restart performs a granted recovery action at the instant the action names.
//
// The simulator is the only component that can move a node out of the crashed
// state, and it only does so on behalf of an action that already carries a
// decision. The decision's evidence is not re-read here: the action was granted
// in the log, and this call is the consequence of that grant rather than a
// second opinion about it.
//
// cause is the reference of the event that proposed the action, which becomes
// the reason the restart happened. A restart with nothing to cite would be a
// change in ground truth that no observer's account explains.
func (e *Engine) Restart(a domain.Action, cause domain.Ref) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.Kind != domain.ActionRestartNode {
		return fmt.Errorf("%w: %s has no path through the simulator", domain.ErrInvalidAction, a.Kind)
	}
	n, ok := e.nodes[a.Target]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownNode, a.Target)
	}
	if n.state != domain.StateCrashed {
		return fmt.Errorf("%w: %s is %s", ErrNotCrashed, a.Target, n.state)
	}
	if !cause.Valid() {
		return fmt.Errorf("%w: %s cites no cause", domain.ErrInvalidAction, a.ID)
	}
	if a.At < e.sched.now {
		return fmt.Errorf("%w: restart at %d, now %d", ErrScheduleInPast, a.At, e.sched.now)
	}
	if _, err := e.sched.At(a.At, func() { e.fail(e.applyRestart(a, cause)) }); err != nil {
		return fmt.Errorf("schedule restart of %s at %d: %w", a.Target, a.At, err)
	}
	return nil
}

// applyRestart writes the action into the history, then lets the node resume.
//
// The action is recorded first so the log shows the decision being carried out
// before the world changes. A reader therefore never finds a node_restarted
// event whose reason appears later in the file than the restart itself.
func (e *Engine) applyRestart(a domain.Action, cause domain.Ref) error {
	started, err := e.record(domain.Event{
		At:     a.At,
		Kind:   domain.KindRecoveryStarted,
		Node:   a.Target,
		Action: a.ID,
		Causes: []domain.Ref{cause},
	})
	if err != nil {
		return fmt.Errorf("record %s of %s: %w", domain.KindRecoveryStarted, a.ID, err)
	}
	return e.nodes[a.Target].restart(a.At, started)
}

// PartitionAt cuts one direction of a link for a fixed interval and records both
// ends of the outage.
//
// Only from->to stops working: the return path keeps carrying messages, so the
// two ends of one link disagree about who is alive. That asymmetry is the point
// of a one-way cut, and it is why a cut is never expressed as "the link is
// down".
//
// Both ends are scheduled like any other event, so they land in the log in
// timestamp order alongside the messages they interrupt. A cut that begins in
// the past is rejected rather than applied retroactively, and the interval is
// validated before anything is scheduled so a rejected cut leaves no trace.
func (e *Engine) PartitionAt(from, to domain.NodeID, at, until domain.Tick) error {
	link, err := domain.NewLink(from, to)
	if err != nil {
		return err
	}
	if err := e.net.validateCut(link, at, until); err != nil {
		return fmt.Errorf("partition %s: %w", link, err)
	}
	if at < e.sched.now {
		return fmt.Errorf("%w: cut at %d, now %d", ErrScheduleInPast, at, e.sched.now)
	}
	outage := e.net.addCut(link, at, until)

	// The announcement runs before any message scheduled at the same instant,
	// because the cut was scheduled when the scenario was configured. That is
	// what lets a message lost at the start of the cut cite it.
	if _, err := e.sched.At(at, func() {
		ref, err := e.record(domain.Event{
			At: at, Kind: domain.KindLinkPartitioned, Node: link.From, Peer: link.To,
			Causes: []domain.Ref{e.root},
		})
		outage.announced = ref
		e.fail(err)
	}); err != nil {
		return fmt.Errorf("schedule %s of %s at %d: %w", domain.KindLinkPartitioned, link, at, err)
	}
	if _, err := e.sched.At(until, func() {
		_, err := e.record(domain.Event{
			At: until, Kind: domain.KindLinkRestored, Node: link.From, Peer: link.To,
			Causes: []domain.Ref{outage.announced},
		})
		e.fail(err)
	}); err != nil {
		return fmt.Errorf("schedule %s of %s at %d: %w", domain.KindLinkRestored, link, until, err)
	}
	return nil
}
