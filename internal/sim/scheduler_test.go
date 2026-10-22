package sim

import (
	"errors"
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
)

func tickAt(ms int64) domain.Tick {
	return domain.TickFrom(time.Duration(ms) * time.Millisecond)
}

func newTestScheduler(t *testing.T) *scheduler {
	t.Helper()
	return newScheduler(0)
}

func TestSchedulerOrdersByTimeThenSchedulingOrder(t *testing.T) {
	s := newTestScheduler(t)
	var order []string
	schedule := func(name string, ms int64) {
		t.Helper()
		if _, err := s.At(tickAt(ms), func() { order = append(order, name) }); err != nil {
			t.Fatalf("At: %v", err)
		}
	}
	schedule("c", 20)
	schedule("a", 10)
	schedule("b", 10)
	schedule("d", 20)

	for s.next() != nil {
		if err := s.step(); err != nil {
			t.Fatalf("step: %v", err)
		}
	}

	want := []string{"a", "b", "c", "d"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestSchedulerAdvancesClockInOrder(t *testing.T) {
	s := newTestScheduler(t)
	var seen []domain.Tick
	for _, ms := range []int64{30, 10, 20} {
		if _, err := s.At(tickAt(ms), func() {
			seen = append(seen, s.now)
		}); err != nil {
			t.Fatal(err)
		}
	}
	for s.next() != nil {
		if err := s.step(); err != nil {
			t.Fatal(err)
		}
	}
	want := []domain.Tick{tickAt(10), tickAt(20), tickAt(30)}
	if len(seen) != len(want) {
		t.Fatalf("clock samples = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("clock samples = %v, want %v", seen, want)
		}
	}
}

func TestSchedulerSkipsCancelledTasks(t *testing.T) {
	s := newTestScheduler(t)
	var order []string
	schedule := func(name string, ms int64) domain.Timer {
		t.Helper()
		timer, err := s.At(tickAt(ms), func() { order = append(order, name) })
		if err != nil {
			t.Fatalf("At: %v", err)
		}
		return timer
	}
	schedule("a", 10)
	cancelled := schedule("b", 10)
	schedule("c", 10)

	cancelled.Cancel()
	cancelled.Cancel() // cancellation is idempotent

	for s.next() != nil {
		if err := s.step(); err != nil {
			t.Fatal(err)
		}
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "c" {
		t.Fatalf("order = %v, want [a c]", order)
	}
}

func TestSchedulerRejectsPastWork(t *testing.T) {
	s := newTestScheduler(t)
	if _, err := s.At(tickAt(10), func() {}); err != nil {
		t.Fatal(err)
	}
	if err := s.step(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.At(tickAt(9), func() {}); !errors.Is(err, ErrScheduleInPast) {
		t.Fatalf("past scheduling error = %v, want ErrScheduleInPast", err)
	}
	if _, err := s.At(tickAt(10), func() {}); err != nil {
		t.Fatalf("work at the current instant must be allowed: %v", err)
	}
}

func TestSchedulerRejectsNilWork(t *testing.T) {
	s := newTestScheduler(t)
	if _, err := s.At(tickAt(1), nil); !errors.Is(err, ErrNilTask) {
		t.Fatalf("error = %v, want ErrNilTask", err)
	}
}

func TestSchedulerEnforcesStepLimit(t *testing.T) {
	s := newScheduler(2)
	for i := int64(1); i <= 3; i++ {
		if _, err := s.At(tickAt(i), func() {}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.step(); err != nil {
		t.Fatalf("step 1: %v", err)
	}
	if err := s.step(); err != nil {
		t.Fatalf("step 2: %v", err)
	}
	if err := s.step(); !errors.Is(err, ErrStepLimit) {
		t.Fatalf("step 3 error = %v, want ErrStepLimit", err)
	}
}

func TestSchedulerRunsOrdinaryWorkBeforeEndOfInstantWork(t *testing.T) {
	s := newTestScheduler(t)
	var order []string

	// The ordinary work is registered first here, and the end-of-instant work
	// first in the next case: class ordering must hold either way, so the
	// result never depends on which timer was armed first.
	if _, err := s.At(tickAt(10), func() { order = append(order, "message") }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AtEnd(tickAt(10), func() { order = append(order, "deadline") }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.At(tickAt(10), func() { order = append(order, "message-2") }); err != nil {
		t.Fatal(err)
	}

	for s.next() != nil {
		if err := s.step(); err != nil {
			t.Fatal(err)
		}
	}

	want := []string{"message", "message-2", "deadline"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestSchedulerEndOfInstantWorkStillFollowsTime(t *testing.T) {
	s := newTestScheduler(t)
	var order []string
	if _, err := s.AtEnd(tickAt(10), func() { order = append(order, "early-deadline") }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.At(tickAt(20), func() { order = append(order, "late-message") }); err != nil {
		t.Fatal(err)
	}
	for s.next() != nil {
		if err := s.step(); err != nil {
			t.Fatal(err)
		}
	}
	if len(order) != 2 || order[0] != "early-deadline" || order[1] != "late-message" {
		t.Fatalf("order = %v, want [early-deadline late-message]", order)
	}
}

func TestSchedulerTieBreakForWorkScheduledDuringExecution(t *testing.T) {
	s := newTestScheduler(t)
	var order []string

	if _, err := s.At(tickAt(10), func() {
		order = append(order, "first")
		if _, err := s.At(tickAt(10), func() { order = append(order, "nested") }); err != nil {
			t.Errorf("nested schedule: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.At(tickAt(10), func() { order = append(order, "queued") }); err != nil {
		t.Fatal(err)
	}

	for s.next() != nil {
		if err := s.step(); err != nil {
			t.Fatal(err)
		}
	}

	// Work added while executing at instant 10 joins the back of that instant's
	// queue, so it runs after work that was already waiting.
	want := []string{"first", "queued", "nested"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}
