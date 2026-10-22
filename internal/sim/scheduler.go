package sim

import (
	"container/heap"
	"errors"
	"fmt"

	"github.com/HalxDocs/aegis/internal/domain"
)

// DefaultStepLimit bounds the tasks a single run may execute. A timer that
// re-arms itself at the current instant would otherwise spin forever; the limit
// turns that bug into a returned error instead of a hung test.
const DefaultStepLimit = 1_000_000

var (
	// ErrScheduleInPast is returned when work is requested for a time that has
	// already passed. Clamping to the present would silently reorder events.
	ErrScheduleInPast = errors.New("task scheduled in the past")

	// ErrNilTask is returned when a scheduler call carries no work.
	ErrNilTask = errors.New("nil task")

	// ErrStepLimit is returned when a run exceeds its task budget.
	ErrStepLimit = errors.New("scheduler step limit exceeded")
)

// taskClass separates ordinary work from deadline expiry at a single instant.
//
// A deadline is the last moment at which the absence of a message means
// something, so evidence that arrives at exactly that moment has to be read
// first. Class is the middle key of the ordering: (at, class, seq).
type taskClass uint8

const (
	classNormal taskClass = iota
	classEnd
)

// task is one scheduled unit of work.
//
// seq is assigned when the task is scheduled, never when it runs. Tasks at the
// same instant and of the same class therefore execute in scheduling order,
// which is the documented tie-break rule: the timer armed first fires first.
// The rule is arbitrary but explicit, and the tests hold it still.
type task struct {
	at    domain.Tick
	class taskClass
	seq   uint64
	run   func()

	// dead marks a cancelled task. It stays in the queue as a tombstone and is
	// discarded when it reaches the head, so cancelling never has to search the
	// heap and can never corrupt its ordering.
	dead bool
}

// Cancel implements domain.Timer. It is idempotent.
func (t *task) Cancel() { t.dead = true }

var _ domain.Timer = (*task)(nil)

type taskQueue []*task

func (q taskQueue) Len() int { return len(q) }

func (q taskQueue) Less(i, j int) bool {
	if q[i].at != q[j].at {
		return q[i].at < q[j].at
	}
	if q[i].class != q[j].class {
		return q[i].class < q[j].class
	}
	return q[i].seq < q[j].seq
}

func (q taskQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *taskQueue) Push(x any) { *q = append(*q, x.(*task)) }

func (q *taskQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return item
}

// scheduler is a discrete-event queue over virtual time.
type scheduler struct {
	queue    taskQueue
	nextSeq  uint64
	now      domain.Tick
	executed int
	limit    int
}

func newScheduler(limit int) *scheduler {
	if limit <= 0 {
		limit = DefaultStepLimit
	}
	return &scheduler{limit: limit}
}

// At schedules fn to run at the given virtual time with ordinary work.
func (s *scheduler) At(at domain.Tick, fn func()) (domain.Timer, error) {
	return s.schedule(at, classNormal, fn)
}

// AtEnd schedules fn to run after all ordinary work at the given instant.
func (s *scheduler) AtEnd(at domain.Tick, fn func()) (domain.Timer, error) {
	return s.schedule(at, classEnd, fn)
}

func (s *scheduler) schedule(at domain.Tick, class taskClass, fn func()) (domain.Timer, error) {
	if fn == nil {
		return nil, ErrNilTask
	}
	if at < s.now {
		return nil, fmt.Errorf("%w: at %d, now %d", ErrScheduleInPast, at, s.now)
	}
	s.nextSeq++
	t := &task{at: at, class: class, seq: s.nextSeq, run: fn}
	heap.Push(&s.queue, t)
	return t, nil
}

// next returns the earliest live task without removing it, discarding
// cancelled tasks that reached the head. It returns nil when no work remains.
func (s *scheduler) next() *task {
	for len(s.queue) > 0 && s.queue[0].dead {
		heap.Pop(&s.queue)
	}
	if len(s.queue) == 0 {
		return nil
	}
	return s.queue[0]
}

// step executes the earliest live task and advances the clock to its time.
func (s *scheduler) step() error {
	t := s.next()
	if t == nil {
		return nil
	}
	heap.Pop(&s.queue)
	s.now = t.at
	s.executed++
	if s.executed > s.limit {
		return fmt.Errorf("%w: %d tasks executed", ErrStepLimit, s.executed)
	}
	t.run()
	return nil
}
