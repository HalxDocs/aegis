package scenario

import (
	"bytes"
	"sync"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func jsonl(t *testing.T, res Result) string {
	t.Helper()
	var out bytes.Buffer
	if err := res.Log.WriteJSONL(&out); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}
	return out.String()
}

// lossy returns the canonical world with an unreliable network, which is the
// only part of Milestone 1 that consumes randomness.
func lossy() Spec {
	spec := NodeCrash()
	spec.Name = "node-crash-lossy"
	spec.Crash = nil
	spec.Horizon = tick(2400)
	for i := range spec.Links {
		spec.Links[i].DropPPM = 400_000
	}
	return spec
}

func TestSeedReplaysTheSameHistory(t *testing.T) {
	want := jsonl(t, mustRun(t, lossy(), 7))
	for i := 0; i < 50; i++ {
		if got := jsonl(t, mustRun(t, lossy(), 7)); got != want {
			t.Fatalf("run %d differs from the first run:\n%s", i, got)
		}
	}
}

func TestDifferentSeedsProduceDifferentHistories(t *testing.T) {
	if jsonl(t, mustRun(t, lossy(), 1)) == jsonl(t, mustRun(t, lossy(), 2)) {
		t.Fatal("two seeds produced the same history on a 40% loss network")
	}
}

func TestScenarioWithoutRandomnessIgnoresTheSeed(t *testing.T) {
	// No drop rate means no draw at all, so the seed cannot leak into the log.
	if jsonl(t, mustRun(t, NodeCrash(), 1)) != jsonl(t, mustRun(t, NodeCrash(), 99)) {
		t.Fatal("a scenario without random decisions depended on the seed")
	}
}

func TestDroppedMessagesAreRecordedAndNeverArrive(t *testing.T) {
	res := mustRun(t, lossy(), 3)
	sent := 0
	dropped := 0
	received := 0
	for _, ev := range res.Log.Events() {
		switch ev.Kind {
		case domain.KindHeartbeatSent:
			sent++
		case domain.KindMessageDropped:
			dropped++
		case domain.KindHeartbeatReceived:
			received++
		}
	}
	if dropped == 0 {
		t.Fatal("a 40% loss network produced no losses")
	}
	if sent == 0 || received == 0 {
		t.Fatalf("sent = %d, received = %d; want traffic in both directions", sent, received)
	}
	// Messages still in flight at the horizon are sent but not received, so the
	// receipts can only be bounded from above by what survived the network.
	if received > sent-dropped {
		t.Fatalf("sent %d - dropped %d = %d, but %d receipts were recorded", sent, dropped, sent-dropped, received)
	}
	if received < sent-dropped-len(res.Spec.Nodes) {
		t.Fatalf("sent %d - dropped %d = %d, but only %d receipts were recorded", sent, dropped, sent-dropped, received)
	}
}

func TestConcurrentRunsShareNoState(t *testing.T) {
	want := jsonl(t, mustRun(t, NodeCrash(), 5))

	const runs = 8
	var wg sync.WaitGroup
	results := make([]string, runs)
	errs := make([]error, runs)
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := Run(NodeCrash(), 5)
			if err != nil {
				errs[i] = err
				return
			}
			var out bytes.Buffer
			if err := res.Log.WriteJSONL(&out); err != nil {
				errs[i] = err
				return
			}
			results[i] = out.String()
		}(i)
	}
	wg.Wait()

	for i := 0; i < runs; i++ {
		if errs[i] != nil {
			t.Fatalf("run %d: %v", i, errs[i])
		}
		if results[i] != want {
			t.Fatalf("run %d produced a different history:\n%s", i, results[i])
		}
	}
}

func mustRun(t *testing.T, spec Spec, seed uint64) Result {
	t.Helper()
	res, err := Run(spec, seed)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}
