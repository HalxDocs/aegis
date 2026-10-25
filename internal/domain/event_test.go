package domain_test

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestEventValidateAcceptsEachKind(t *testing.T) {
	events := map[string]domain.Event{
		"scenario started":  {Kind: domain.KindScenarioStarted, Scenario: "node-crash"},
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
		"scenario without a name":    {Kind: domain.KindScenarioStarted},
		"scenario naming a node":     {Kind: domain.KindScenarioStarted, Scenario: "node-crash", Node: "A"},
		"node event without node":    {Kind: domain.KindNodeCrashed, At: tick(215)},
		"node event with a peer":     {Kind: domain.KindNodeCrashed, At: tick(215), Node: "B", Peer: "A"},
		"sent without peer":          {Kind: domain.KindHeartbeatSent, Node: "B", Round: 1},
		"sent with round zero":       {Kind: domain.KindHeartbeatSent, Node: "B", Peer: "A"},
		"dropped without a reason":   {Kind: domain.KindMessageDropped, Node: "B", Peer: "A", Round: 1},
		"suspicion without a reason": {Kind: domain.KindSuspicionRaised, Node: "A", Peer: "B"},
		"cleared without a peer":     {Kind: domain.KindSuspicionCleared, Node: "A"},
		"invalid node id":            {Kind: domain.KindNodeStarted, Node: "bad node"},
	}
	for name, event := range cases {
		t.Run(name, func(t *testing.T) {
			if err := event.Validate(); !errors.Is(err, domain.ErrInvalidEvent) {
				t.Fatalf("Validate() = %v, want ErrInvalidEvent", err)
			}
		})
	}
}

func TestKindValidity(t *testing.T) {
	if domain.Kind("").Valid() {
		t.Fatal("empty kind must not be valid")
	}
	if !domain.KindHeartbeatReceived.Valid() {
		t.Fatal("declared kind must be valid")
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
