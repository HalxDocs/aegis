package observability

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/HalxDocs/aegis/internal/domain"
)

var (
	// ErrOutOfOrder is returned for an event that would appear earlier in the
	// history than the event before it. Accepting it would make the log a
	// description of arrival order rather than of virtual time.
	ErrOutOfOrder = errors.New("event precedes the previous event")

	// ErrPreassignedSeq is returned when a caller supplies its own sequence
	// number. Sequence is the recorder's to assign; a second numbering scheme
	// is how two histories quietly diverge.
	ErrPreassignedSeq = errors.New("event already has a sequence number")
)

// Recorder is an in-memory append-only history.
//
// It is not safe for concurrent use: a run is single-goroutine, and isolation
// between runs comes from giving each run its own Recorder.
type Recorder struct {
	events []domain.Event
}

// NewRecorder returns an empty history.
func NewRecorder() *Recorder { return &Recorder{} }

// Record validates an event and appends it with the next sequence number.
//
// The event is appended only after validation succeeds, so a rejected event
// never leaves a hole in the sequence.
func (r *Recorder) Record(ev domain.Event) error {
	if ev.Seq != 0 {
		return fmt.Errorf("%w: seq %d", ErrPreassignedSeq, ev.Seq)
	}
	if err := ev.Validate(); err != nil {
		return err
	}
	if n := len(r.events); n > 0 {
		if prev := r.events[n-1]; ev.At < prev.At {
			return fmt.Errorf("%w: %s at %d follows %s at %d",
				ErrOutOfOrder, ev.Kind, ev.At, prev.Kind, prev.At)
		}
	}

	ev.Seq = uint64(len(r.events) + 1)
	r.events = append(r.events, ev)
	return nil
}

// Events returns a copy of the history in sequence order.
func (r *Recorder) Events() []domain.Event { return slices.Clone(r.events) }

// Len returns the number of recorded events.
func (r *Recorder) Len() int { return len(r.events) }

// WriteJSONL writes one JSON object per line in sequence order.
//
// Field order comes from the event struct, never from a map, so two runs of the
// same scenario produce identical bytes and a diff is a real difference.
func (r *Recorder) WriteJSONL(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, ev := range r.events {
		if err := enc.Encode(ev); err != nil {
			return fmt.Errorf("encode event %d: %w", ev.Seq, err)
		}
	}
	return nil
}
