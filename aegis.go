// Package aegis is the part of the failure lab that other programs may use.
//
// It is deliberately small and deliberately not the simulator. What is exported
// here is the set of things that can be trusted without trusting a run: the
// event vocabulary a log is written in, the history read as a graph of reasons,
// the gate that weighs evidence, and the verifier that reads a proof back out of
// a finished log. None of it asks the simulator anything, which is why none of it
// has to be told what really happened.
//
// Everything exported here is an alias of the internal type that implements it,
// so there is exactly one definition of each of these things. A copy would be a
// second source of truth, and this project's whole argument is about what
// happens to two descriptions of one world that are allowed to drift.
//
// What is not here: running a scenario. The simulator, the detectors and the
// scenario specifications stay internal because their shapes are still moving
// with each milestone, and a promise not to move them would be a promise the
// project cannot keep yet. A program that needs a log can run the command and
// read its output with ReadLog; a program that needs to decide about evidence it
// has collected itself can build the reports and ask the gate.
package aegis

import (
	"io"
	"time"

	"github.com/HalxDocs/aegis/internal/causal"
	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/observability"
	"github.com/HalxDocs/aegis/internal/recovery"
	"github.com/HalxDocs/aegis/internal/verify"
)

// EventLogSchema names the format ReadLog accepts and the command writes.
//
// A log says which format it is on its opening event, and ReadLog refuses one
// written in any other. Checking it is not optional for a consumer: the fields
// below are a meaning as well as a shape, and a file that means something else
// by them parses perfectly and lies.
const EventLogSchema = domain.EventLogSchema

// The observable vocabulary of a history.
//
// These are the only kinds a log may contain. A consumer can switch on them
// exhaustively; a kind this list does not name cannot appear in a log written by
// this build, so a default branch is a case that should not happen rather than a
// case nobody has got to yet.
const (
	KindScenarioStarted   = domain.KindScenarioStarted
	KindScenarioFinished  = domain.KindScenarioFinished
	KindNodeStarted       = domain.KindNodeStarted
	KindNodeCrashed       = domain.KindNodeCrashed
	KindNodeRestarted     = domain.KindNodeRestarted
	KindHeartbeatSent     = domain.KindHeartbeatSent
	KindMessageDropped    = domain.KindMessageDropped
	KindHeartbeatReceived = domain.KindHeartbeatReceived
	KindSuspicionRaised   = domain.KindSuspicionRaised
	KindSuspicionCleared  = domain.KindSuspicionCleared
	KindLinkPartitioned   = domain.KindLinkPartitioned
	KindLinkRestored      = domain.KindLinkRestored
	KindRecoveryProposed  = domain.KindRecoveryProposed
	KindRecoveryRefused   = domain.KindRecoveryRefused
	KindRecoveryStarted   = domain.KindRecoveryStarted
)

// Reasons carried by events that need a machine-readable cause.
const (
	ReasonDropPolicy            = domain.ReasonDropPolicy
	ReasonHeartbeatTimeout      = domain.ReasonHeartbeatTimeout
	ReasonPartition             = domain.ReasonPartition
	ReasonInsufficientWitnesses = domain.ReasonInsufficientWitnesses
)

// The core vocabulary of a run.
type (
	// Event is one line of a JSONL history.
	Event = domain.Event

	// Kind names a class of event.
	Kind = domain.Kind

	// NodeID names a node in the world.
	NodeID = domain.NodeID

	// Ref is an event's sequence number, and the only name it ever has.
	Ref = domain.Ref

	// Tick is a point on the virtual timeline, in nanoseconds from the start.
	Tick = domain.Tick

	// ActionID names one recovery decision.
	ActionID = domain.ActionID

	// ActionKind is the kind of change an action makes to the world.
	ActionKind = domain.ActionKind

	// Action is one granted decision, with the node it applies to.
	Action = domain.Action

	// Belief is what an observer currently thinks of a peer.
	Belief = domain.Belief

	// Observation is one observer's belief about one peer, at one instant.
	Observation = domain.Observation

	// FailureReport is the evidence one observer's silence produced.
	FailureReport = domain.FailureReport
)

// ActionRestartNode is the only action this build knows how to express.
const ActionRestartNode = domain.ActionRestartNode

// TickFrom converts a duration into virtual time, exactly.
func TickFrom(d time.Duration) Tick { return domain.TickFrom(d) }

// ReadLog decodes a history written by `aegis run --format jsonl`.
//
// Reading is strict about the format and about position: a log that names
// another schema is refused whole, and a line whose sequence does not match its
// place in the file is an error rather than a skipped line. A loader that
// tolerates damage turns a corrupted artifact into a plausible one.
func ReadLog(r io.Reader) ([]Event, error) { return observability.ReadJSONL(r) }

// Index is a history read as a graph: every event, and the edges that cite it.
type Index = causal.Index

// NewIndex validates a history and returns it as a graph.
//
// The history has to be complete and self-consistent before it can be explained:
// a cause that names no event, a reference that points forward, or a history
// without its opening event is refused here rather than producing an
// explanation that looks plausible.
func NewIndex(events []Event) (*Index, error) { return causal.New(events) }

// Recovery decisions.
type (
	// Policy is the bar an action has to clear: how many witnesses, how long to
	// wait for more, and how long between deciding and acting.
	Policy = recovery.Policy

	// Gate applies a policy to a set of witnesses. It holds no state and reads
	// no clock.
	Gate = recovery.Gate

	// Witness is one observer's report together with the event that recorded it.
	Witness = recovery.Witness

	// Decision is what the gate concluded: granted, or refused with a reason.
	Decision = recovery.Decision
)

// Errors a caller may need to tell apart.
var (
	// ErrNoWitnesses means a decision was asked for with no evidence.
	ErrNoWitnesses = recovery.ErrNoWitnesses

	// ErrMixedTargets means the witnesses were about different nodes.
	ErrMixedTargets = recovery.ErrMixedTargets

	// ErrRepeatedObserver means one observer was counted twice. One observer,
	// one vote.
	ErrRepeatedObserver = recovery.ErrRepeatedObserver
)

// NewGate validates a policy and returns a gate that applies it.
func NewGate(p Policy) (Gate, error) { return recovery.New(p) }

// Verification of a finished history.
type (
	// Proof is the chain of events that shows one action was carried out.
	Proof = verify.Proof
)

// VerifyAction looks for the recorded chain behind one action.
//
// The boolean says the chain is complete: proposed, started, and followed by a
// change in ground truth. An incomplete chain is returned with its progress, so
// a caller can say how far the log actually got instead of only that it failed.
func VerifyAction(idx *Index, id ActionID) (Proof, bool, error) { return verify.Action(idx, id) }

// VerifyActions verifies every action the history names, in the order the
// proposals were recorded.
func VerifyActions(idx *Index) ([]Proof, error) { return verify.Actions(idx) }
