package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestNewNodeIDAcceptsLogFriendlyNames(t *testing.T) {
	valid := []string{"A", "b", "node-1", "node_2.three", "worker-0.eu-west-1", strings.Repeat("x", 64)}
	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			id, err := domain.NewNodeID(name)
			if err != nil {
				t.Fatalf("NewNodeID(%q) = %v, want nil", name, err)
			}
			if got := id.String(); got != name {
				t.Fatalf("String() = %q, want %q", got, name)
			}
			if err := id.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestNewNodeIDRejectsAmbiguousNames(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"leading space", " A"},
		{"trailing space", "A "},
		{"inner space", "node A"},
		{"non ascii", "nœud"},
		{"slash", "a/b"},
		{"too long", strings.Repeat("x", 65)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.NewNodeID(tc.input); !errors.Is(err, domain.ErrInvalidNodeID) {
				t.Fatalf("NewNodeID(%q) error = %v, want ErrInvalidNodeID", tc.input, err)
			}
		})
	}
}

func TestZeroNodeIDIsInvalid(t *testing.T) {
	var id domain.NodeID
	if err := id.Validate(); !errors.Is(err, domain.ErrInvalidNodeID) {
		t.Fatalf("zero NodeID Validate() = %v, want ErrInvalidNodeID", err)
	}
}

func TestNewLink(t *testing.T) {
	link, err := domain.NewLink("B", "A")
	if err != nil {
		t.Fatalf("NewLink = %v", err)
	}
	if got, want := link.String(), "B->A"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if err := link.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestNewLinkRejectsSelfReference(t *testing.T) {
	if _, err := domain.NewLink("A", "A"); !errors.Is(err, domain.ErrInvalidLink) {
		t.Fatalf("self link error = %v, want ErrInvalidLink", err)
	}
}

func TestNewLinkRejectsInvalidEndpoints(t *testing.T) {
	if _, err := domain.NewLink("", "A"); !errors.Is(err, domain.ErrInvalidLink) {
		t.Fatalf("empty from error = %v, want ErrInvalidLink", err)
	}
	if _, err := domain.NewLink("A", "bad id"); !errors.Is(err, domain.ErrInvalidLink) {
		t.Fatalf("bad to error = %v, want ErrInvalidLink", err)
	}
}
