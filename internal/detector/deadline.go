package detector

import (
	"fmt"

	"github.com/HalxDocs/aegis/internal/domain"
)

// monitor is one observer's belief about one target, plus the deadline that
// will overturn it if nothing arrives.
type monitor struct {
	obs domain.Observation

	// epoch is incremented whenever the deadline is re-armed or cancelled.
	// A callback carries the epoch it was created under and does nothing if
	// the belief has moved on since.
	//
	// Cancellation alone is enough with a scheduler that never runs cancelled
	// work, which is what this project ships. The epoch is the second lock: it
	// keeps a phantom suspicion impossible even if a timer is delivered after
	// being cancelled, which is the one bug in a heartbeat detector that looks
	// exactly like a real failure.
	epoch uint64

	// evidence is the reference of the event that armed the current deadline:
	// the receipt that re-armed it, or the scenario when it was armed at start.
	evidence domain.Ref

	timer domain.Timer
}

// arm replaces the monitor's deadline with one timeout after from.
//
// The previous timer is cancelled rather than left to fire; with a queue that
// honours cancellation the stale callback never runs at all, and if one ever
// did, its epoch would be behind the monitor's and it would return without
// changing a belief.
func (d *Detector) arm(target domain.NodeID, m *monitor, from domain.Tick) error {
	m.epoch++
	epoch := m.epoch
	if m.timer != nil {
		m.timer.Cancel()
		m.timer = nil
	}

	deadline := from + d.timeout
	timer, err := d.sched.AtEnd(deadline, func() { d.expire(target, epoch, deadline) })
	if err != nil {
		return fmt.Errorf("detector %s: arm deadline for %s at %d: %w", d.observer, target, deadline, err)
	}
	m.timer = timer
	return nil
}

// expire is the deadline callback. It has no caller to return an error to, so
// a failure is handed to the sink that aborts the run.
func (d *Detector) expire(target domain.NodeID, epoch uint64, at domain.Tick) {
	if err := d.evaluate(target, epoch, at); err != nil {
		d.faults.Fail(err)
	}
}

// evaluate raises suspicion when a deadline passes with no fresh evidence.
//
// Suspicion is inclusive of the deadline: waiting exactly the timeout without
// hearing anything is enough. A message that arrives at that same instant is
// still evidence, because the scheduler runs ordinary work before deadline work.
func (d *Detector) evaluate(target domain.NodeID, epoch uint64, at domain.Tick) error {
	m, ok := d.monitors[target]
	if !ok {
		return nil
	}
	if m.epoch != epoch {
		return nil
	}
	if m.obs.Belief == domain.BeliefSuspected {
		return nil
	}

	next, err := m.obs.Suspect(at)
	if err != nil {
		return fmt.Errorf("detector %s: %w", d.observer, err)
	}
	report, err := domain.NewFailureReport(next, d.timeout)
	if err != nil {
		return fmt.Errorf("detector %s: %w", d.observer, err)
	}
	if _, err := d.rec.Record(domain.Event{
		At:     at,
		Kind:   domain.KindSuspicionRaised,
		Node:   target,
		Peer:   d.observer,
		Reason: domain.ReasonHeartbeatTimeout,
		Causes: []domain.Ref{m.evidence},
	}); err != nil {
		return fmt.Errorf("detector %s: record suspicion of %s: %w", d.observer, target, err)
	}

	m.obs = next
	m.timer = nil
	d.reports = append(d.reports, report)
	return nil
}
