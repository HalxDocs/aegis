package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestActionIDZeroNamesNothing(t *testing.T) {
	var zero ActionID
	if zero.Valid() {
		t.Fatal("the zero action identifier is valid, but it names no action")
	}
	if ActionID(1).Valid() != true {
		t.Fatal("action 1 is not valid")
	}
}

func TestActionIDStringIsReadable(t *testing.T) {
	if got := ActionID(7).String(); got != "action 7" {
		t.Fatalf("String() = %q, want %q", got, "action 7")
	}
}

func TestActionKindIsAClosedVocabulary(t *testing.T) {
	if !ActionRestartNode.Valid() {
		t.Fatal("restart is not a valid action kind")
	}
	for _, kind := range []ActionKind{"", "stop_node", "Restart_Node", "restart node"} {
		if kind.Valid() {
			t.Errorf("%q is accepted as an action kind", kind)
		}
	}
}

func TestActionValidate(t *testing.T) {
	valid := Action{ID: 1, Kind: ActionRestartNode, Target: "B", At: 625}

	if err := valid.Validate(); err != nil {
		t.Fatalf("valid action rejected: %v", err)
	}

	cases := map[string]struct {
		action Action
		want   string
	}{
		"no identifier": {Action{Kind: ActionRestartNode, Target: "B"}, "identifier is zero"},
		"unknown kind":  {Action{ID: 1, Kind: "stop_node", Target: "B"}, "unknown kind"},
		"bad target":    {Action{ID: 1, Kind: ActionRestartNode, Target: "bad node"}, "target"},
		"past time":     {Action{ID: 1, Kind: ActionRestartNode, Target: "B", At: -1}, "negative virtual time"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.action.Validate()
			if err == nil {
				t.Fatalf("%s was accepted", name)
			}
			if !errors.Is(err, ErrInvalidAction) {
				t.Fatalf("error = %v, want ErrInvalidAction", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestActionStringNamesKindAndTarget(t *testing.T) {
	action := Action{ID: 2, Kind: ActionRestartNode, Target: "B"}
	if got := action.String(); got != "restart_node B" {
		t.Fatalf("String() = %q, want %q", got, "restart_node B")
	}
}
