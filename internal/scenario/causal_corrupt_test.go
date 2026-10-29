package scenario

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/HalxDocs/aegis/internal/causal"
	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/observability"
)

// fixtureEvents loads the committed artifact exactly as the CLI does.
func fixtureEvents(t *testing.T) []domain.Event {
	t.Helper()
	f, err := os.Open("testdata/one-way-partition.jsonl")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	events, err := observability.ReadJSONL(f)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return events
}

// writeEvents puts a history on disk in the same shape the recorder writes, so
// the corrupt copy is damaged in content only and stays a well-formed file.
func writeEvents(t *testing.T, name string, events []domain.Event) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			t.Fatalf("encode event %d: %v", ev.Seq, err)
		}
	}
	return path
}

// readEvents reads a history back through the public reader.
func readEvents(t *testing.T, path string) []domain.Event {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	events, err := observability.ReadJSONL(f)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return events
}

// corrupt rewrites one event's cause list, leaving every other field and every
// other event untouched. It works by sequence number rather than by slice index
// so a change to the fixture's earlier events cannot silently retarget the edit.
func corrupt(t *testing.T, events []domain.Event, seq domain.Ref, causes ...domain.Ref) []domain.Event {
	t.Helper()
	out := make([]domain.Event, len(events))
	copy(out, events)
	for i, ev := range out {
		if domain.Ref(ev.Seq) != seq {
			continue
		}
		out[i].Causes = append([]domain.Ref(nil), causes...)
		return out
	}
	t.Fatalf("no event %s in the fixture", seq)
	return nil
}

// TestTheReaderCannotSeeAFutureCause is the control for the two tests below.
//
// A log corrupted on disk is still valid JSONL with dense sequence numbers, so
// ReadJSONL accepts it and the damage only becomes visible when the graph is
// built. That division of labour is what makes this test end to end: without it,
// a failure in the tests below could just mean the rewrite was broken.
func TestTheReaderCannotSeeAFutureCause(t *testing.T) {
	events := fixtureEvents(t)
	causes := corrupt(t, events, 61, 63)
	path := writeEvents(t, "future-cause.jsonl", causes)

	reread := readEvents(t, path)
	if len(reread) != len(events) {
		t.Fatalf("reread %d events, wrote %d", len(reread), len(events))
	}
	if got := reread[60].Causes; len(got) != 1 || got[0] != 63 {
		t.Fatalf("event 61 cites %v after the round trip, want [63]", got)
	}
}

// TestACorruptedFixtureCannotCiteALaterEvent takes the artifact in the
// repository, points the recovery at an event that had not happened yet, and
// requires the index to refuse it.
//
// The corruption is the one a hand-edited log produces, and it is the reason the
// rules live in New rather than in a renderer: a forward edge still yields an
// explanation that prints cleanly, and it is wrong.
func TestACorruptedFixtureCannotCiteALaterEvent(t *testing.T) {
	events := fixtureEvents(t)
	// The honest file is the control: the same write path, unmutated, indexes.
	honest := writeEvents(t, "honest.jsonl", events)
	if _, err := causal.New(readEvents(t, honest)); err != nil {
		t.Fatalf("the unmutated copy no longer indexes: %v", err)
	}

	causes := corrupt(t, events, 61, 63)
	idx, err := causal.New(readEvents(t, writeEvents(t, "future-cause.jsonl", causes)))
	if !errors.Is(err, causal.ErrCauseOrder) {
		t.Fatalf("error = %v, want ErrCauseOrder", err)
	}
	if idx != nil {
		t.Fatal("a rejected history must not produce an index")
	}
}

// TestACorruptedFixtureCannotCiteAMissingEvent covers the other edit a damaged
// file makes: a reference into a history that is not there.
func TestACorruptedFixtureCannotCiteAMissingEvent(t *testing.T) {
	causes := corrupt(t, fixtureEvents(t), 61, 900)
	if _, err := causal.New(readEvents(t, writeEvents(t, "dangling.jsonl", causes))); !errors.Is(err, causal.ErrDanglingCause) {
		t.Fatalf("error = %v, want ErrDanglingCause", err)
	}
}
