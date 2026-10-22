package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidNodeID identifies a malformed node identifier.
var ErrInvalidNodeID = errors.New("invalid node id")

// ErrInvalidLink identifies a malformed or self-referential link.
var ErrInvalidLink = errors.New("invalid link")

// maxNodeIDLength bounds identifiers so log lines and scheduler keys stay
// readable. It is generous enough to hold a hostname or a UUID.
const maxNodeIDLength = 64

// NodeID names a simulated node. Values are printable, log-friendly and
// validated on construction, so raw strings never travel through the system
// pretending to be identifiers.
type NodeID string

// NewNodeID validates and returns a node identifier. Allowed characters are
// ASCII letters, digits, '-', '_' and '.'. Whitespace and non-ASCII characters
// are rejected rather than silently normalised, because two identifiers that
// differ only by invisible characters produce two nodes that look identical in
// an event log.
func NewNodeID(s string) (NodeID, error) {
	switch {
	case s == "":
		return "", fmt.Errorf("%w: empty", ErrInvalidNodeID)
	case len(s) > maxNodeIDLength:
		return "", fmt.Errorf("%w: %q is longer than %d bytes", ErrInvalidNodeID, s, maxNodeIDLength)
	case strings.TrimSpace(s) != s:
		return "", fmt.Errorf("%w: %q has leading or trailing whitespace", ErrInvalidNodeID, s)
	}
	for _, r := range s {
		if !validNodeIDRune(r) {
			return "", fmt.Errorf("%w: %q contains %q", ErrInvalidNodeID, s, r)
		}
	}
	return NodeID(s), nil
}

func validNodeIDRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '-', r == '_', r == '.':
		return true
	default:
		return false
	}
}

// String returns the identifier as written in scenarios and event logs.
func (n NodeID) String() string { return string(n) }

// Validate reports whether the identifier is well formed.
func (n NodeID) Validate() error {
	_, err := NewNodeID(string(n))
	return err
}

// Link is an ordered pair naming the direction a message travels. Network
// configuration is keyed by it, and a message never chooses its own route.
type Link struct {
	From NodeID
	To   NodeID
}

// NewLink validates and returns a directed link.
func NewLink(from, to NodeID) (Link, error) {
	l := Link{From: from, To: to}
	if err := l.Validate(); err != nil {
		return Link{}, err
	}
	return l, nil
}

// Validate reports whether both endpoints are valid identifiers that differ.
func (l Link) Validate() error {
	if err := l.From.Validate(); err != nil {
		return fmt.Errorf("%w: from: %w", ErrInvalidLink, err)
	}
	if err := l.To.Validate(); err != nil {
		return fmt.Errorf("%w: to: %w", ErrInvalidLink, err)
	}
	if l.From == l.To {
		return fmt.Errorf("%w: %s cannot link to itself", ErrInvalidLink, l.From)
	}
	return nil
}

// String renders the link in its log form, for example "B->A".
func (l Link) String() string {
	return l.From.String() + "->" + l.To.String()
}
