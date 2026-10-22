package scenario

import (
	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/observability"
)

// ObservationKey names one observer's belief about one target.
type ObservationKey struct {
	Observer domain.NodeID
	Target   domain.NodeID
}

// Result is a finished run: the history it produced and what each layer
// believed at the horizon.
//
// Truth and Beliefs are deliberately separate fields rather than one merged
// view, so a test can state the milestone's claim — crashed and believed
// healthy — without deriving either half from the other.
type Result struct {
	Spec    Spec
	Log     *observability.Recorder
	Truth   map[domain.NodeID]domain.NodeState
	Beliefs map[ObservationKey]domain.Observation
	Reports []domain.FailureReport
}

// Belief returns one observer's final belief about one target.
func (r Result) Belief(observer, target domain.NodeID) (domain.Observation, bool) {
	obs, ok := r.Beliefs[ObservationKey{Observer: observer, Target: target}]
	return obs, ok
}

// ReportsFor returns the failure reports one observer produced, in order.
func (r Result) ReportsFor(observer domain.NodeID) []domain.FailureReport {
	var out []domain.FailureReport
	for _, report := range r.Reports {
		if report.Observer == observer {
			out = append(out, report)
		}
	}
	return out
}
