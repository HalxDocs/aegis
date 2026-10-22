package scenario

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

// messageKey identifies one heartbeat by its sender, receiver and round.
type messageKey struct {
	Round uint64
	From  domain.NodeID
	To    domain.NodeID
}

func TestEverySentMessageHasExactlyOneOutcome(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		res := mustRun(t, lossy(), seed)

		sent := make(map[messageKey]bool)
		outcome := make(map[messageKey]domain.Kind)
		for _, ev := range res.Log.Events() {
			if ev.Kind != domain.KindHeartbeatSent &&
				ev.Kind != domain.KindHeartbeatReceived &&
				ev.Kind != domain.KindMessageDropped {
				continue
			}
			key := messageKey{Round: ev.Round, From: ev.Node, To: ev.Peer}
			switch ev.Kind {
			case domain.KindHeartbeatSent:
				if sent[key] {
					t.Fatalf("seed %d: message %+v was sent twice", seed, key)
				}
				sent[key] = true
			default:
				if !sent[key] {
					t.Fatalf("seed %d: %s recorded for a message never sent: %+v", seed, ev.Kind, key)
				}
				if previous, ok := outcome[key]; ok {
					t.Fatalf("seed %d: message %+v both %s and %s", seed, key, previous, ev.Kind)
				}
				outcome[key] = ev.Kind
			}
		}

		// A message with no outcome yet is still on the wire. Only the last
		// round can be, and only one per link, so anything larger means a
		// message vanished without a record.
		pending := 0
		for key := range sent {
			if _, ok := outcome[key]; !ok {
				pending++
			}
		}
		if pending > len(res.Spec.Links) {
			t.Fatalf("seed %d: %d messages are unaccounted for, at most one per link can be in flight", seed, pending)
		}
	}
}

func TestSuspicionAlwaysWaitsForTheFullTimeout(t *testing.T) {
	crashAt := tick(215)
	interval := tick(100)
	for _, timeout := range []domain.Tick{tick(350), tick(500)} {
		for _, latency := range []domain.Tick{tick(5), tick(20), tick(60)} {
			spec := NodeCrash()
			spec.Observers[0].Timeout = timeout
			spec.Observers[1].Timeout = timeout
			spec.Links[0].Latency = latency
			spec.Links[1].Latency = latency
			res := mustRun(t, spec, 1)

			if got := res.Truth["B"]; got != domain.StateCrashed {
				t.Fatalf("timeout %d, latency %d: truth = %s, want crashed", timeout, latency, got)
			}
			if len(res.Reports) != 2 {
				t.Fatalf("timeout %d, latency %d: reports = %d, want 2", timeout, latency, len(res.Reports))
			}
			for _, report := range res.Reports {
				if report.Timeout != timeout {
					t.Fatalf("report carries timeout %d, want %d", report.Timeout, timeout)
				}
				// The deadline is anchored to evidence, not to the crash: it is
				// always exactly one timeout after the last arrival.
				if want := report.LastSeenAt + timeout; report.SuspectedAt != want {
					t.Fatalf("suspected at %d, want last seen %d + timeout %d", report.SuspectedAt, report.LastSeenAt, timeout)
				}
				// Relative to the crash this can be early or late by at most one
				// heartbeat interval plus one link latency.
				if low := crashAt + timeout - interval; report.SuspectedAt < low {
					t.Fatalf("suspected at %d, before the earliest possible deadline %d", report.SuspectedAt, low)
				}
				if high := crashAt + timeout + latency; report.SuspectedAt > high {
					t.Fatalf("suspected at %d, after the latest possible deadline %d", report.SuspectedAt, high)
				}
				if err := report.Validate(); err != nil {
					t.Fatalf("report invalid: %v", err)
				}
			}
		}
	}
}
