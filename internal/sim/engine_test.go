package sim

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestNewEngineRequiresRecorder(t *testing.T) {
	if _, err := NewEngine(EngineConfig{}); !errors.Is(err, ErrNoRecorder) {
		t.Fatalf("error = %v, want ErrNoRecorder", err)
	}
}

func TestAddNodeValidatesConfiguration(t *testing.T) {
	cases := map[string]NodeConfig{
		"empty id":               {},
		"invalid id":             {ID: "bad node"},
		"negative interval":      {ID: "A", HeartbeatEvery: -1},
		"peers without interval": {ID: "A", Peers: []domain.NodeID{"B"}},
		"interval without peers": {ID: "A", HeartbeatEvery: tickAt(100)},
		"self peer":              {ID: "A", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"A"}},
		"duplicate peer":         {ID: "A", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"B", "B"}},
		"invalid peer":           {ID: "A", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"bad peer"}},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			e, _ := newTestEngine(t, EngineConfig{})
			if err := e.AddNode(cfg); err == nil {
				t.Fatalf("AddNode(%+v) accepted an invalid configuration", cfg)
			}
		})
	}
}

func TestAddNodeRejectsDuplicates(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if err := e.AddNode(NodeConfig{ID: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := e.AddNode(NodeConfig{ID: "A"}); !errors.Is(err, ErrDuplicateNode) {
		t.Fatalf("error = %v, want ErrDuplicateNode", err)
	}
}

func TestNodesAreSortedRegardlessOfConfigurationOrder(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	for _, id := range []domain.NodeID{"C", "A", "B"} {
		if err := e.AddNode(NodeConfig{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	got := e.Nodes()
	want := []domain.NodeID{"A", "B", "C"}
	if len(got) != len(want) {
		t.Fatalf("Nodes() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Nodes() = %v, want %v", got, want)
		}
	}
}

func TestStartRejectsUnknownPeer(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if err := e.AddNode(NodeConfig{ID: "B", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"A"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("Start error = %v, want ErrUnknownNode", err)
	}
}

func TestStartIsOnceOnly(t *testing.T) {
	e, _, _, _ := canonicalEngine(t)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Start error = %v, want ErrAlreadyStarted", err)
	}
	if err := e.AddNode(NodeConfig{ID: "D"}); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("AddNode after Start error = %v, want ErrAlreadyStarted", err)
	}
	if err := e.AddLink(LinkConfig{From: "A", To: "B"}); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("AddLink after Start error = %v, want ErrAlreadyStarted", err)
	}
}

func TestAddReceiverRejectsNil(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if err := e.AddReceiver(nil); !errors.Is(err, ErrNoReceiver) {
		t.Fatalf("error = %v, want ErrNoReceiver", err)
	}
}

func TestEveryReceiverSeesEveryDeliveryInRegistrationOrder(t *testing.T) {
	e, _, _, _ := canonicalEngine(t)
	second := &stubReceiver{}
	if err := e.AddReceiver(second); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(250)); err != nil {
		t.Fatal(err)
	}

	first := e.receivers[0].(*stubReceiver)
	if len(first.got) != 4 || len(second.got) != 4 {
		t.Fatalf("deliveries = %d and %d, want 4 each", len(first.got), len(second.got))
	}
	for i := range first.got {
		if first.got[i] != second.got[i] {
			t.Fatalf("delivery %d differs between receivers: %+v and %+v", i, first.got[i], second.got[i])
		}
	}
}

func TestOnCrashRejectsNil(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if err := e.OnCrash(nil); !errors.Is(err, ErrNilListener) {
		t.Fatalf("error = %v, want ErrNilListener", err)
	}
}

func TestTruthReportsUnknownNodes(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if _, ok := e.Truth("nobody"); ok {
		t.Fatal("unknown node must not report ground truth")
	}
	if state, ok := e.Truth("A"); ok || state != "" {
		t.Fatalf("Truth(A) = %q, %v; want empty and false for an unconfigured node", state, ok)
	}
}

func TestAddNodeThenTruthStartsRunning(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if err := e.AddNode(NodeConfig{ID: "A"}); err != nil {
		t.Fatal(err)
	}
	state, ok := e.Truth("A")
	if !ok || state != domain.StateRunning {
		t.Fatalf("Truth(A) = %q, %v; want running", state, ok)
	}
}

func TestEngineAtSchedulesWork(t *testing.T) {
	e, _ := newTestEngine(t, EngineConfig{})
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	ran := false
	if _, err := e.At(tickAt(50), func() { ran = true }); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(100)); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("scheduled work did not run")
	}
	if now := e.Now(); now != tickAt(100) {
		t.Fatalf("Now() = %d, want %d", now, tickAt(100))
	}
}
