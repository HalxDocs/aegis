package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidEvent identifies an event that cannot be appended to the history.
var ErrInvalidEvent = errors.New("invalid event")

// Kind names a class of event in the recorded history. The vocabulary is
// closed: an unknown string is rejected rather than written, so a typo cannot
// produce a log that silently loses information.
type Kind string

const (
	// KindScenarioStarted opens a run and names the scenario.
	KindScenarioStarted Kind = "scenario_started"

	// KindScenarioFinished closes a run at its horizon.
	KindScenarioFinished Kind = "scenario_finished"

	// KindNodeStarted records that a node began executing.
	KindNodeStarted Kind = "node_started"

	// KindNodeCrashed records a change in ground truth.
	KindNodeCrashed Kind = "node_crashed"

	// KindHeartbeatSent records a message leaving a node.
	KindHeartbeatSent Kind = "heartbeat_sent"

	// KindMessageDropped records a message the network discarded instead of
	// delivering it. It is written at the instant the message failed to arrive,
	// which for a cut path is later than the send.
	KindMessageDropped Kind = "message_dropped"

	// KindHeartbeatReceived records a message that actually reached an observer.
	KindHeartbeatReceived Kind = "heartbeat_received"

	// KindSuspicionRaised records an observer losing trust in a target.
	KindSuspicionRaised Kind = "suspicion_raised"

	// KindSuspicionCleared records an observer regaining trust in a target.
	KindSuspicionCleared Kind = "suspicion_cleared"

	// KindLinkPartitioned records one direction of a link becoming unusable.
	// Node is the sending end and Peer the receiving end, so the event names the
	// half of the path that stopped working.
	KindLinkPartitioned Kind = "link_partitioned"

	// KindLinkRestored records a partitioned direction becoming usable again.
	KindLinkRestored Kind = "link_restored"
)

// String returns the kind as written in event logs.
func (k Kind) String() string { return string(k) }

// Valid reports whether k belongs to the recorded vocabulary.
func (k Kind) Valid() bool {
	switch k {
	case KindScenarioStarted, KindScenarioFinished,
		KindNodeStarted, KindNodeCrashed,
		KindHeartbeatSent, KindMessageDropped, KindHeartbeatReceived,
		KindSuspicionRaised, KindSuspicionCleared,
		KindLinkPartitioned, KindLinkRestored:
		return true
	default:
		return false
	}
}

// Reasons attached to events that need a machine-readable cause.
const (
	// ReasonDropPolicy means the configured drop rate discarded the message.
	ReasonDropPolicy = "drop_policy"

	// ReasonHeartbeatTimeout means a heartbeat deadline expired with no message.
	ReasonHeartbeatTimeout = "heartbeat_timeout"
)

// Event is one entry in the append-only history. It carries virtual time only:
// no wall-clock field exists, because a wall-clock timestamp would make two
// identical runs differ and replay untestable.
//
// Seq is assigned by the recorder. Node is the event's subject, Peer the
// counterpart where one exists, Round the heartbeat round for message events,
// Reason the machine-readable cause, and Scenario the run's name on the opening
// and closing events.
type Event struct {
	Seq      uint64 `json:"seq"`
	At       Tick   `json:"at"`
	Kind     Kind   `json:"kind"`
	Node     NodeID `json:"node,omitempty"`
	Peer     NodeID `json:"peer,omitempty"`
	Round    uint64 `json:"round,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Scenario string `json:"scenario,omitempty"`
}

// Validate checks the fields required by the event's kind.
func (e Event) Validate() error {
	if !e.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidEvent, e.Kind)
	}
	if e.At < 0 {
		return fmt.Errorf("%w: negative virtual time %d", ErrInvalidEvent, e.At)
	}
	if e.Kind == KindScenarioStarted || e.Kind == KindScenarioFinished {
		if e.Scenario == "" {
			return fmt.Errorf("%w: %s requires a scenario name", ErrInvalidEvent, e.Kind)
		}
		if e.Node != "" || e.Peer != "" {
			return fmt.Errorf("%w: %s must not name nodes", ErrInvalidEvent, e.Kind)
		}
		return nil
	}
	if err := e.Node.Validate(); err != nil {
		return fmt.Errorf("%w: %s requires a subject node: %w", ErrInvalidEvent, e.Kind, err)
	}
	switch e.Kind {
	case KindNodeStarted, KindNodeCrashed:
		if e.Peer != "" {
			return fmt.Errorf("%w: %s must not name a peer", ErrInvalidEvent, e.Kind)
		}
	case KindLinkPartitioned, KindLinkRestored:
		if err := e.Peer.Validate(); err != nil {
			return fmt.Errorf("%w: %s requires a peer: %w", ErrInvalidEvent, e.Kind, err)
		}
		if e.Round != 0 {
			return fmt.Errorf("%w: %s must not name a round", ErrInvalidEvent, e.Kind)
		}
		if e.Reason != "" {
			return fmt.Errorf("%w: %s must not carry a reason", ErrInvalidEvent, e.Kind)
		}
	case KindHeartbeatSent, KindMessageDropped, KindHeartbeatReceived:
		if err := e.Peer.Validate(); err != nil {
			return fmt.Errorf("%w: %s requires a peer: %w", ErrInvalidEvent, e.Kind, err)
		}
		if e.Round == 0 {
			return fmt.Errorf("%w: %s requires a positive round", ErrInvalidEvent, e.Kind)
		}
		if e.Kind == KindMessageDropped && e.Reason == "" {
			return fmt.Errorf("%w: %s requires a reason", ErrInvalidEvent, e.Kind)
		}
	case KindSuspicionRaised:
		if err := e.Peer.Validate(); err != nil {
			return fmt.Errorf("%w: %s requires a peer: %w", ErrInvalidEvent, e.Kind, err)
		}
		if e.Reason == "" {
			return fmt.Errorf("%w: %s requires a reason", ErrInvalidEvent, e.Kind)
		}
	case KindSuspicionCleared:
		if err := e.Peer.Validate(); err != nil {
			return fmt.Errorf("%w: %s requires a peer: %w", ErrInvalidEvent, e.Kind, err)
		}
	}
	return nil
}
