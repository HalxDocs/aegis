package sim

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestEachHeartbeatCitesTheSendThatScheduledIt(t *testing.T) {
	e, rec := newTestEngine(t, EngineConfig{})
	if err := e.AddNode(NodeConfig{ID: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := e.AddNode(NodeConfig{ID: "B", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"A"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.AddLink(LinkConfig{From: "B", To: "A", Latency: tickAt(20)}); err != nil {
		t.Fatal(err)
	}
	if err := e.AddReceiver(&stubReceiver{}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(350)); err != nil {
		t.Fatal(err)
	}

	sent := rec.ofKind(domain.KindHeartbeatSent)
	if len(sent) != 3 {
		t.Fatalf("sends = %d, want three rounds", len(sent))
	}
	var started domain.Event
	for _, ev := range rec.ofKind(domain.KindNodeStarted) {
		if ev.Node == "B" {
			started = ev
		}
	}
	// The first round is caused by the node starting; every later round by the
	// send before it, because the timer is re-armed from the moment of sending.
	want := []domain.Ref{rec.refOf(started)}
	if !equalRefs(sent[0].Causes, want) {
		t.Fatalf("round 1 causes = %v, want %v", sent[0].Causes, want)
	}
	for i := 1; i < len(sent); i++ {
		if want := []domain.Ref{rec.refOf(sent[i-1])}; !equalRefs(sent[i].Causes, want) {
			t.Fatalf("round %d causes = %v, want the send before it %v", i+1, sent[i].Causes, want)
		}
		if sent[i].Causes[0] <= sent[i-1].Causes[0]-1 {
			// The reference also has to move forwards, which the recorder checks
			// for the real log; this asserts the chain is not accidentally cyclic.
			t.Fatalf("round %d cites %v, which does not follow round %d", i+1, sent[i].Causes, i)
		}
	}
}

// refOf finds the reference the stub recorder would have given an event. The
// stub does not assign sequence numbers, so the position in the slice is the
// reference, exactly as the real recorder would number it.
func (r *stubRecorder) refOf(ev domain.Event) domain.Ref {
	for i, other := range r.events {
		if other.At == ev.At && other.Kind == ev.Kind && other.Node == ev.Node && other.Peer == ev.Peer && other.Round == ev.Round {
			return domain.Ref(i + 1)
		}
	}
	return 0
}

// TestEachDirectionHasItsOwnChain pins the scope of the send chain. A node with
// two peers sends two messages per round; one chain for the node would claim the
// second link's round one caused the first link's round two.
func TestEachDirectionHasItsOwnChain(t *testing.T) {
	e, rec := newTestEngine(t, EngineConfig{})
	for _, id := range []domain.NodeID{"A", "C"} {
		if err := e.AddNode(NodeConfig{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.AddNode(NodeConfig{ID: "B", HeartbeatEvery: tickAt(100), Peers: []domain.NodeID{"A", "C"}}); err != nil {
		t.Fatal(err)
	}
	for _, peer := range []domain.NodeID{"A", "C"} {
		if err := e.AddLink(LinkConfig{From: "B", To: peer, Latency: tickAt(20)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.AddReceiver(&stubReceiver{}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(250)); err != nil {
		t.Fatal(err)
	}

	byPeer := map[domain.NodeID][]domain.Event{}
	for _, ev := range rec.ofKind(domain.KindHeartbeatSent) {
		byPeer[ev.Peer] = append(byPeer[ev.Peer], ev)
	}
	if len(byPeer["A"]) != 2 || len(byPeer["C"]) != 2 {
		t.Fatalf("sends per direction = %d and %d, want two rounds each", len(byPeer["A"]), len(byPeer["C"]))
	}
	for peer, sends := range byPeer {
		if want := []domain.Ref{rec.refOf(sends[0])}; !equalRefs(sends[1].Causes, want) {
			t.Fatalf("B->%s round 2 cites %v, want its own round 1 %v", peer, sends[1].Causes, want)
		}
	}
	// The two round-one sends cite the same start, not each other.
	if byPeer["A"][0].Causes[0] != byPeer["C"][0].Causes[0] {
		t.Fatalf("round one cites %v for A and %v for C, want the same node start",
			byPeer["A"][0].Causes, byPeer["C"][0].Causes)
	}
}
