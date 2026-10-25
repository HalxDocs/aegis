package sim

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func mustLink(t *testing.T, from, to domain.NodeID) domain.Link {
	t.Helper()
	link, err := domain.NewLink(from, to)
	if err != nil {
		t.Fatal(err)
	}
	return link
}
