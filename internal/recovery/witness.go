package recovery

import (
	"errors"
	"fmt"

	"github.com/HalxDocs/aegis/internal/domain"
)

// ErrInvalidWitness identifies a witness that cannot support a decision.
var ErrInvalidWitness = errors.New("invalid witness")

// Witness is one observer's failure report together with the event that recorded
// it.
//
// The reference is not a convenience. A decision has to cite the events it acted
// on, and a report on its own names no line in the history: two observers with
// identical timings would be indistinguishable, and the log would carry a
// decision with nothing to point at.
type Witness struct {
	Report domain.FailureReport
	Ref    domain.Ref
}

// Validate checks that the witness is a recorded, consistent report.
func (w Witness) Validate() error {
	if !w.Ref.Valid() {
		return fmt.Errorf("%w: reference is zero", ErrInvalidWitness)
	}
	if err := w.Report.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidWitness, err)
	}
	return nil
}

// Target is the node the witness stopped trusting.
func (w Witness) Target() domain.NodeID { return w.Report.Target }

// String renders the witness for logs and test failures.
func (w Witness) String() string {
	return fmt.Sprintf("%s by %s (#%d)", w.Report.Target, w.Report.Observer, uint64(w.Ref))
}
