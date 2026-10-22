package domain_test

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestTransitionErrorIsDiscoverable(t *testing.T) {
	err := &domain.TransitionError{Subject: "node", From: "running", To: "running"}

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatal("TransitionError must wrap ErrInvalidTransition")
	}
	const want = `node: cannot move from "running" to "running"`
	if got := err.Error(); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestTransitionErrorKeepsSubject(t *testing.T) {
	err := &domain.TransitionError{Subject: "belief", From: "healthy", To: "healthy"}
	if err.Subject != "belief" || err.From != "healthy" || err.To != "healthy" {
		t.Fatalf("fields lost: %+v", err)
	}
}
