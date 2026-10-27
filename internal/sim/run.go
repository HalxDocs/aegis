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
	e.net.addCut(link, at, until)
	for _, mark := range []struct {
		at   domain.Tick
		kind domain.Kind
	}{
		{at, domain.KindLinkPartitioned},
		{until, domain.KindLinkRestored},
	} {
		if _, err := e.sched.At(mark.at, func() {
			_, err := e.record(domain.Event{At: mark.at, Kind: mark.kind, Node: link.From, Peer: link.To})
			e.fail(err)
		}); err != nil {
			return fmt.Errorf("schedule %s of %s at %d: %w", mark.kind, link, mark.at, err)
		}
	}
	return nil
}
