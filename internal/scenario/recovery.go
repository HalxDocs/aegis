package scenario

import (
	"fmt"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/recovery"
)

// RecoverySpec configures the loop that decides what to do about a failure.
//
// The policy is the whole configuration. There is no switch for "act" versus
// "only decide": a refusal is a decision too, and a run that could be told to
// skip the gate would be a run whose log cannot be compared with one that used
// it.
type RecoverySpec struct {
	Policy recovery.Policy
}

func (r RecoverySpec) validate() error {
	if err := r.Policy.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}
	return nil
}

// eventRecorder is the append-only history the loop writes its decisions to.
type eventRecorder interface {
	Record(domain.Event) (domain.Ref, error)
}

// restarter is the only capability the loop has over the world. It is declared
// here, at the consumer, so the loop can be tested against a world that refuses
// to move without a simulator being built first.
type restarter interface {
	Restart(domain.Action, domain.Ref) error
}

// recoveryCase is the evidence gathered about one target between the first
// report and the review the policy allows.
//
// A case is opened by a report and closed by its review, exactly once. Reports
// that arrive after the review open a new case: evidence cannot be added to a
// decision that has already been made, or the log would show a decision
// supported by events that are later in the file than the decision itself.
type recoveryCase struct {
	target    domain.NodeID
	witnesses []recovery.Witness
	closed    bool
}

// recoveryLoop turns failure reports into decisions and carries out the ones
// that are granted.
//
// It holds the only stateful part of recovery: which cases are open. Everything
// else — what counts as enough evidence, what a refusal is called — belongs to
// the policy and the gate, which are pure. The loop's state is exactly the part
// that a replay has to reproduce, and it is all in the log once the run ends.
type recoveryLoop struct {
	gate   recovery.Gate
	policy recovery.Policy
	sched  domain.Scheduler
	rec    eventRecorder
	world  restarter
	fault  func(error)

	cases     map[domain.NodeID]*recoveryCase
	order     []domain.NodeID
	nextID    domain.ActionID
	decisions []recovery.Decision
	actions   []domain.Action
}

// newRecoveryLoop validates the policy and returns a loop that has decided
// nothing yet.
func newRecoveryLoop(spec RecoverySpec, sched domain.Scheduler, rec eventRecorder, world restarter, fault func(error)) (*recoveryLoop, error) {
	gate, err := recovery.New(spec.Policy)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}
	if sched == nil {
		return nil, fmt.Errorf("%w: recovery requires a scheduler", ErrInvalidSpec)
	}
	if rec == nil {
		return nil, fmt.Errorf("%w: recovery requires a recorder", ErrInvalidSpec)
	}
	if world == nil {
		return nil, fmt.Errorf("%w: recovery requires a world to act on", ErrInvalidSpec)
	}
	if fault == nil {
		return nil, fmt.Errorf("%w: recovery requires a fault sink", ErrInvalidSpec)
	}
	return &recoveryLoop{
		gate:   gate,
		policy: spec.Policy,
		sched:  sched,
		rec:    rec,
		world:  world,
		fault:  fault,
		cases:  make(map[domain.NodeID]*recoveryCase),
	}, nil
}

// ReportFailed implements the detector's report sink.
//
// The first report about a target opens a case and schedules its review one
// patience later; later reports about the same target join the open case. The
// loop never waits for a report it has not been promised, so a policy whose
// threshold is never reached is decided at the patience deadline with what it
// has, rather than leaving the run with no decision at all.
func (l *recoveryLoop) ReportFailed(report domain.FailureReport, ref domain.Ref) error {
	witness := recovery.Witness{Report: report, Ref: ref}

	c, open := l.cases[report.Target]
	if open && c.closed {
		c, open = nil, false
	}
	if !open {
		c = &recoveryCase{target: report.Target}
		l.cases[c.target] = c
		l.order = append(l.order, c.target)
		// The review is armed from the report's own suspicion instant rather than
		// from the clock, so patience is measured against the evidence instead of
		// against whenever the loop happened to run.
		at := report.SuspectedAt + l.policy.Patience
		if _, err := l.sched.At(at, func() { l.review(c, at) }); err != nil {
			return fmt.Errorf("recovery: schedule review of %s at %d: %w", c.target, at, err)
		}
	}
	c.witnesses = append(c.witnesses, witness)
	return nil
}

// review is the scheduled callback. It has no caller to return an error to, so
// a failure goes to the sink that aborts the run.
func (l *recoveryLoop) review(c *recoveryCase, at domain.Tick) {
	if err := l.decide(c, at); err != nil {
		l.fault(err)
	}
}

// decide weighs the case's evidence and writes the outcome.
//
// Both outcomes are written at the instant of the review, before anything
// happens to the world. A refusal is therefore as reviewable as a grant: it
// names the witnesses it declined to act on and the reason it declined, and a
// reader can check whether that reason was true.
func (l *recoveryLoop) decide(c *recoveryCase, at domain.Tick) error {
	c.closed = true

	decision, err := l.gate.Decide(c.witnesses)
	if err != nil {
		return fmt.Errorf("recovery %s: %w", c.target, err)
	}
	l.nextID++
	id := l.nextID
	l.decisions = append(l.decisions, decision)

	if !decision.Granted {
		if _, err := l.rec.Record(domain.Event{
			At:     at,
			Kind:   domain.KindRecoveryRefused,
			Node:   c.target,
			Action: id,
			Reason: decision.Reason,
			Causes: decision.Witnesses,
		}); err != nil {
			return fmt.Errorf("recovery %s: record refusal: %w", c.target, err)
		}
		return nil
	}

	proposed, err := l.rec.Record(domain.Event{
		At:     at,
		Kind:   domain.KindRecoveryProposed,
		Node:   c.target,
		Action: id,
		Causes: decision.Witnesses,
	})
	if err != nil {
		return fmt.Errorf("recovery %s: record proposal: %w", c.target, err)
	}

	action := domain.Action{
		ID:     id,
		Kind:   l.policy.Kind,
		Target: c.target,
		At:     at + l.policy.Delay,
	}
	l.actions = append(l.actions, action)
	if err := l.world.Restart(action, proposed); err != nil {
		// The proposal is already in the log, so a world that cannot carry it out
		// leaves a grant with no consequence. That is a fault in the run, not a
		// detail to be papered over: it means the decision and the world disagree.
		return fmt.Errorf("recovery %s: granted %s could not be carried out: %w", c.target, action, err)
	}
	return nil
}

// Decisions returns the decisions the loop made, in order.
func (l *recoveryLoop) Decisions() []recovery.Decision {
	out := make([]recovery.Decision, len(l.decisions))
	copy(out, l.decisions)
	return out
}

// Actions returns the granted actions the loop handed to the world, in order.
func (l *recoveryLoop) Actions() []domain.Action {
	out := make([]domain.Action, len(l.actions))
	copy(out, l.actions)
	return out
}
