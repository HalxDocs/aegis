package detector

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
)

func tickAt(ms int64) domain.Tick {
	return domain.TickFrom(time.Duration(ms) * time.Millisecond)
}

// Classes mirror the simulator's ordering keys. The detector only needs to ask
// for end-of-instant work; the fake reproduces the ordering so a tie can be
// tested without importing the simulator.
const (
	classNormal = iota
	classEnd
)

type fakeTask struct {
	at    domain.Tick
	class int
	seq   int
	fn    func()
	dead  bool
}

func (t *fakeTask) Cancel() { t.dead = true }

// fakeScheduler is a deterministic queue that also serves as the clock.
//
// run executes tasks in (time, class, scheduling order) and can optionally
// ignore cancellation, which is how a phantom timer is simulated.
type fakeScheduler struct {
	now   domain.Tick
	tasks []*fakeTask
	seq   int
}

func newFakeScheduler() *fakeScheduler { return &fakeScheduler{} }

func (s *fakeScheduler) Now() domain.Tick { return s.now }

func (s *fakeScheduler) At(at domain.Tick, fn func()) (domain.Timer, error) {
	return s.add(at, classNormal, fn)
}

func (s *fakeScheduler) AtEnd(at domain.Tick, fn func()) (domain.Timer, error) {
	return s.add(at, classEnd, fn)
}

func (s *fakeScheduler) add(at domain.Tick, class int, fn func()) (domain.Timer, error) {
	if fn == nil {
		return nil, fmt.Errorf("fake scheduler: nil task")
	}
	if at < s.now {
		return nil, fmt.Errorf("fake scheduler: task at %d precedes now %d", at, s.now)
	}
	s.seq++
	t := &fakeTask{at: at, class: class, seq: s.seq, fn: fn}
	s.tasks = append(s.tasks, t)
	return t, nil
}

// run executes live work up to and including the given instant. Passing false
// for honourCancel delivers work that was cancelled, which a correct scheduler
// never does and a defensive detector must survive.
func (s *fakeScheduler) run(t domain.Tick, honourCancel bool) {
	for steps := 0; steps < 10_000; steps++ {
		sort.SliceStable(s.tasks, func(i, j int) bool {
			a, b := s.tasks[i], s.tasks[j]
			if a.at != b.at {
				return a.at < b.at
			}
			if a.class != b.class {
				return a.class < b.class
			}
			return a.seq < b.seq
		})

		var next *fakeTask
		for _, task := range s.tasks {
			if task.at > t {
				break
			}
			if honourCancel && task.dead {
				continue
			}
			next = task
			break
		}
		if next == nil {
			// Like the simulator, the clock ends at the horizon even when the
			// queue is empty, so work scheduled afterwards starts from there.
			s.now = t
			return
		}
		s.remove(next)
		s.now = next.at
		next.fn()
	}
}

func (s *fakeScheduler) remove(target *fakeTask) {
	kept := s.tasks[:0]
	for _, task := range s.tasks {
		if task != target {
			kept = append(kept, task)
		}
	}
	s.tasks = kept
}

// liveTask returns the only task still expected to run.
func (s *fakeScheduler) liveTask(t *testing.T) *fakeTask {
	t.Helper()
	var found []*fakeTask
	for _, task := range s.tasks {
		if !task.dead {
			found = append(found, task)
		}
	}
	if len(found) != 1 {
		t.Fatalf("live tasks = %d, want 1", len(found))
	}
	return found[0]
}

// live counts the tasks that are still expected to run.
func (s *fakeScheduler) live() int {
	n := 0
	for _, task := range s.tasks {
		if !task.dead {
			n++
		}
	}
	return n
}

// stubRecorder keeps events in memory and records schema violations instead of
// failing, because it is called from places with no *testing.T.
type stubRecorder struct {
	events []domain.Event
	bad    []error
	err    error
}

func (r *stubRecorder) Record(ev domain.Event) (domain.Ref, error) {
	if r.err != nil {
		return 0, r.err
	}
	if err := ev.Validate(); err != nil {
		r.bad = append(r.bad, err)
	}
	r.events = append(r.events, ev)
	return domain.Ref(len(r.events)), nil
}

func (r *stubRecorder) ofKind(kind domain.Kind) []domain.Event {
	var out []domain.Event
	for _, ev := range r.events {
		if ev.Kind == kind {
			out = append(out, ev)
		}
	}
	return out
}

func (r *stubRecorder) count(kind domain.Kind) int { return len(r.ofKind(kind)) }

func (r *stubRecorder) requireClean(t *testing.T) {
	t.Helper()
	for _, err := range r.bad {
		t.Errorf("invalid event recorded: %v", err)
	}
}

type stubFaults struct {
	errs []error
}

func (f *stubFaults) Fail(err error) { f.errs = append(f.errs, err) }

type harness struct {
	det    *Detector
	sched  *fakeScheduler
	rec    *stubRecorder
	faults *stubFaults
}

func newHarness(t *testing.T, observer domain.NodeID, timeout domain.Tick) *harness {
	t.Helper()
	h := &harness{
		sched:  newFakeScheduler(),
		rec:    &stubRecorder{},
		faults: &stubFaults{},
	}
	det, err := New(Config{
		Observer:  observer,
		Timeout:   timeout,
		Clock:     h.sched,
		Scheduler: h.sched,
		Recorder:  h.rec,
		Faults:    h.faults,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.det = det
	return h
}

func (h *harness) mustWatch(t *testing.T, target domain.NodeID) {
	t.Helper()
	if err := h.det.Watch(target); err != nil {
		t.Fatalf("Watch(%s): %v", target, err)
	}
}

func (h *harness) believe(t *testing.T, target domain.NodeID) domain.Observation {
	t.Helper()
	obs, ok := h.det.Observation(target)
	if !ok {
		t.Fatalf("%s does not watch %s", h.det.Observer(), target)
	}
	return obs
}
