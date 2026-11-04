package domain_test

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestEventValidateAcceptsEachKind(t *testing.T) {
	events := map[string]domain.Event{
		"scenario started": {
			Kind: domain.KindScenarioStarted, Scenario: "node-crash", Schema: domain.EventLogSchema,
		},
		"scenario finished": {Kind: domain.KindScenarioFinished, At: tick(1200), Scenario: "node-crash"},
		"node started":      {Kind: domain.KindNodeStarted, Node: "A"},
		"node crashed":      {Kind: domain.KindNodeCrashed, At: tick(215), Node: "B"},
		"heartbeat sent":    {Kind: domain.KindHeartbeatSent, At: tick(100), Node: "B", Peer: "A", Round: 1},
		"message dropped": {
			Kind: domain.KindMessageDropped, At: tick(100), Node: "B", Peer: "A", Round: 1, Reason: domain.ReasonDropPolicy,
		},
		"heartbeat received": {Kind: domain.KindHeartbeatReceived, At: tick(120), Node: "A", Peer: "B", Round: 1},
		"suspicion raised": {
			Kind: domain.KindSuspicionRaised, At: tick(570), Node: "A", Peer: "B", Reason: domain.ReasonHeartbeatTimeout,
		},
		"suspicion cleared": {Kind: domain.KindSuspicionCleared, At: tick(600), Node: "A", Peer: "B"},
		"link partitioned":  {Kind: domain.KindLinkPartitioned, At: tick(300), Node: "B", Peer: "A"},
		"link restored":     {Kind: domain.KindLinkRestored, At: tick(900), Node: "B", Peer: "A"},
		"recovery proposed": {
			Kind: domain.KindRecoveryProposed, At: tick(575), Node: "B", Action: 1, Causes: []domain.Ref{14, 15},
		},
		"recovery refused": {
			Kind: domain.KindRecoveryRefused, At: tick(870), Node: "B", Action: 1,
			Reason: domain.ReasonInsufficientWitnesses, Causes: []domain.Ref{14},
		},
		"recovery started": {
			Kind: domain.KindRecoveryStarted, At: tick(625), Node: "B", Action: 1, Causes: []domain.Ref{17},
		},
		"node restarted": {Kind: domain.KindNodeRestarted, At: tick(625), Node: "B"},
	}
	for name, event := range events {
		t.Run(name, func(t *testing.T) {
			if err := event.Validate(); err != nil {
				t.Fatalf("Validate() = %v", err)
			}
		})
	}
}

func TestEventValidateRejectsIncompleteEvents(t *testing.T) {
	cases := map[string]domain.Event{
		"unknown kind":               {Kind: domain.Kind("node_vanished"), Node: "A"},
		"negative time":              {Kind: domain.KindNodeStarted, At: -1, Node: "A"},
		"scenario without a name":    {Kind: domain.KindScenarioStarted, Schema: domain.EventLogSchema},
		"scenario naming a node":     {Kind: domain.KindScenarioStarted, Scenario: "node-crash", Node: "A"},
		"node event without node":    {Kind: domain.KindNodeCrashed, At: tick(215)},
		"node event with a peer":     {Kind: domain.KindNodeCrashed, At: tick(215), Node: "B", Peer: "A"},
		"sent without peer":          {Kind: domain.KindHeartbeatSent, Node: "B", Round: 1},
		"sent with round zero":       {Kind: domain.KindHeartbeatSent, Node: "B", Peer: "A"},
		"dropped without a reason":   {Kind: domain.KindMessageDropped, Node: "B", Peer: "A", Round: 1},
		"suspicion without a reason": {Kind: domain.KindSuspicionRaised, Node: "A", Peer: "B"},
		"cleared without a peer":     {Kind: domain.KindSuspicionCleared, Node: "A"},
		"cut without a peer":         {Kind: domain.KindLinkPartitioned, Node: "B"},
		"cut with a round":           {Kind: domain.KindLinkPartitioned, Node: "B", Peer: "A", Round: 1},
		"cut with a reason":          {Kind: domain.KindLinkRestored, Node: "B", Peer: "A", Reason: domain.ReasonPartition},
		"invalid node id":            {Kind: domain.KindNodeStarted, Node: "bad node"},

		"proposal without an action": {Kind: domain.KindRecoveryProposed, Node: "B", Causes: []domain.Ref{14}},
		"proposal without evidence": {
			Kind: domain.KindRecoveryProposed, Node: "B", Action: 1,
		},
		"proposal with a peer": {
			Kind: domain.KindRecoveryProposed, Node: "B", Peer: "A", Action: 1, Causes: []domain.Ref{14},
		},
		"proposal with a reason": {
			Kind: domain.KindRecoveryProposed, Node: "B", Action: 1, Reason: "because", Causes: []domain.Ref{14},
		},
		"refusal without a reason": {
			Kind: domain.KindRecoveryRefused, Node: "B", Action: 1, Causes: []domain.Ref{14},
		},
		"refusal without evidence": {
			Kind: domain.KindRecoveryRefused, Node: "B", Action: 1, Reason: domain.ReasonInsufficientWitnesses,
		},
		"restart of no node":     {Kind: domain.KindNodeRestarted, At: tick(625), Action: 1},
		"restart with an action": {Kind: domain.KindNodeRestarted, At: tick(625), Node: "B", Action: 1},
		"schema on another kind": {
			Kind: domain.KindNodeStarted, Node: "A", Schema: domain.EventLogSchema,
		},
	}
	for name, event := range cases {
		t.Run(name, func(t *testing.T) {
			if err := event.Validate(); !errors.Is(err, domain.ErrInvalidEvent) {
				t.Fatalf("Validate() = %v, want ErrInvalidEvent", err)
			}
		})
	}
}

// TestSchemaBelongsToTheOpeningEvent pins where the log's format is written.
//
// The rule is that the first line says what the file is and no other line may
// claim otherwise: a version repeated on every event is a version that can
// disagree with itself, and a reader has no rule for which line wins.
func TestSchemaBelongsToTheOpeningEvent(t *testing.T) {
	missing := domain.Event{Kind: domain.KindScenarioStarted, Scenario: "node-crash"}
	if err := missing.Validate(); !errors.Is(err, domain.ErrUnsupportedSchema) {
		t.Fatalf("an unversioned opener: error = %v, want ErrUnsupportedSchema", err)
	}

	future := domain.Event{Kind: domain.KindScenarioStarted, Scenario: "node-crash", Schema: "aegis.eventlog/v2"}
	if err := future.Validate(); !errors.Is(err, domain.ErrUnsupportedSchema) {
		t.Fatalf("a newer opener: error = %v, want ErrUnsupportedSchema", err)
	}

	mid := domain.Event{Kind: domain.KindSuspicionRaised, Node: "A", Peer: "B", Reason: domain.ReasonHeartbeatTimeout,
		Schema: domain.EventLogSchema}
	if err := mid.Validate(); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("a versioned middle event: error = %v, want ErrInvalidEvent", err)
	}

	// WithSchema is how a recorder fills the format in, so it must accept the
	// omission and refuse the disagreement rather than silently overwriting.
	stamped, err := domain.WithSchema(missing)
	if err != nil {
		t.Fatalf("WithSchema(no schema) = %v", err)
	}
	if stamped.Schema != domain.EventLogSchema {
		t.Fatalf("stamped schema = %q", stamped.Schema)
	}
	if err := stamped.Validate(); err != nil {
		t.Fatalf("the stamped opener was rejected: %v", err)
	}
	if _, err := domain.WithSchema(future); !errors.Is(err, domain.ErrUnsupportedSchema) {
		t.Fatalf("WithSchema(newer) = %v, want ErrUnsupportedSchema", err)
	}
}

func TestKindValidity(t *testing.T) {
	if domain.Kind("").Valid() {
		t.Fatal("empty kind must not be valid")
	}
	if !domain.KindHeartbeatReceived.Valid() {
		t.Fatal("declared kind must be valid")
	}
	if !domain.KindRecoveryRefused.Valid() {
		t.Fatal("a refusal is part of the vocabulary: if it cannot be written down, it did not happen")
	}
	if got, want := domain.KindNodeCrashed.String(), "node_crashed"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestBeliefString(t *testing.T) {
	if got, want := domain.BeliefSuspected.String(), "suspected"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
