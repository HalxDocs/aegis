package domain_test

import (
	"testing"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestTickConversionIsExact(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want domain.Tick
	}{
		{"zero", 0, 0},
		{"one nanosecond", time.Nanosecond, 1},
		{"one microsecond", time.Microsecond, 1_000},
		{"one millisecond", time.Millisecond, 1_000_000},
		{"heartbeat timeout", 350 * time.Millisecond, 350_000_000},
		{"one hour", time.Hour, 3_600_000_000_000},
		{"negative", -2 * time.Second, -2_000_000_000},
		{"large", time.Duration(1) << 62, 4_611_686_018_427_387_904},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.TickFrom(tc.d)
			if got != tc.want {
				t.Fatalf("TickFrom(%d) = %d, want %d", tc.d, got, tc.want)
			}
			if back := got.Duration(); back != tc.d {
				t.Fatalf("%d.Duration() = %d, want %d", got, back, tc.d)
			}
		})
	}
}

func TestTickArithmetic(t *testing.T) {
	start := domain.TickFrom(time.Second)

	if got, want := start.Add(250*time.Millisecond), domain.TickFrom(1_250*time.Millisecond); got != want {
		t.Errorf("Add = %d, want %d", got, want)
	}
	if got, want := start.Sub(domain.TickFrom(400*time.Millisecond)), domain.TickFrom(600*time.Millisecond); got != want {
		t.Errorf("Sub = %d, want %d", got, want)
	}
	if got := start.Sub(domain.TickFrom(2 * time.Second)); got != domain.TickFrom(-time.Second) {
		t.Errorf("negative subtraction = %d, want %d", got, domain.TickFrom(-time.Second))
	}
}

func TestTickOrdering(t *testing.T) {
	early := domain.TickFrom(570 * time.Millisecond)
	late := domain.TickFrom(575 * time.Millisecond)

	if !early.Before(late) {
		t.Error("earlier tick must sort before later tick")
	}
	if late.Before(early) {
		t.Error("later tick must not sort before earlier tick")
	}
	if early.Before(early) {
		t.Error("Before must be strict")
	}
}

func TestTickZeroIsRunStart(t *testing.T) {
	if !domain.Tick(0).IsZero() {
		t.Error("zero tick must report IsZero")
	}
	if domain.TickFrom(time.Nanosecond).IsZero() {
		t.Error("non-zero tick must not report IsZero")
	}
}
