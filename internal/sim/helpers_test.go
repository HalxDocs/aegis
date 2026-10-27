package sim

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

// stubRecorder keeps events in memory. It records schema violations rather
// than failing, because it is called from scheduler tasks where there is no
// *testing.T to report through.
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

type receivedHeartbeat struct {
	At    domain.Tick
	From  domain.NodeID
	To    domain.NodeID
	Round uint64
	Cause domain.Ref
}

type stubReceiver struct {
	got []receivedHeartbeat
	err error
}

func (r *stubReceiver) HeartbeatReceived(at domain.Tick, from, to domain.NodeID, round uint64, cause domain.Ref) error {
	if r.err != nil {
		return r.err
	}
	r.got = append(r.got, receivedHeartbeat{At: at, From: from, To: to, Round: round, Cause: cause})
	return nil
}

func newTestEngine(t *testing.T, cfg EngineConfig) (*Engine, *stubRecorder) {
	t.Helper()
	e, rec := unopenedEngine(t, cfg)
	if err := e.Open("test"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	return e, rec
}

// unopenedEngine builds a simulator with no history open, for the tests that
// are about what happens at the boundary of a run.
func unopenedEngine(t *testing.T, cfg EngineConfig) (*Engine, *stubRecorder) {
	t.Helper()
	rec, ok := cfg.Recorder.(*stubRecorder)
	if !ok {
		rec = &stubRecorder{}
		cfg.Recorder = rec
	}
	e, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return e, rec
}

// canonicalEngine builds the Milestone 1 cluster: three nodes, B heartbeating
// to A and C over links with different latencies.
func canonicalEngine(t *testing.T) (*Engine, *stubRecorder, *stubReceiver) {
	t.Helper()
	e, rec := newTestEngine(t, EngineConfig{Seed: 1})

	nodes := []NodeConfig{
		{ID: "A"},
		{ID: "B", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"A", "C"}},
		{ID: "C"},
	}
	for _, cfg := range nodes {
		if err := e.AddNode(cfg); err != nil {
			t.Fatalf("AddNode(%s): %v", cfg.ID, err)
		}
	}
	links := []LinkConfig{
		{From: "B", To: "A", Latency: tickAt(20)},
		{From: "B", To: "C", Latency: tickAt(25)},
	}
	for _, cfg := range links {
		if err := e.AddLink(cfg); err != nil {
			t.Fatalf("AddLink(%+v): %v", cfg, err)
		}
	}

	rx := &stubReceiver{}
	if err := e.AddReceiver(rx); err != nil {
		t.Fatalf("AddReceiver: %v", err)
	}
	return e, rec, rx
}
