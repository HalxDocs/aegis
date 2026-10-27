package sim

import (
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func TestCutIsCausedByTheScenarioAndRepairByTheCut(t *testing.T) {
	e, rec := newTestEngine(t, EngineConfig{})
	for _, id := range []domain.NodeID{"A", "B"} {
		if err := e.AddNode(NodeConfig{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.AddLink(LinkConfig{From: "B", To: "A", Latency: tickAt(20)}); err != nil {
		t.Fatal(err)
	}
	if err := e.PartitionAt("B", "A", tickAt(100), tickAt(300)); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(400)); err != nil {
		t.Fatal(err)
	}

	cut := rec.ofKind(domain.KindLinkPartitioned)
	if len(cut) != 1 {
		t.Fatalf("link_partitioned = %d, want 1", len(cut))
	}
	if want := []domain.Ref{e.Root()}; !equalRefs(cut[0].Causes, want) {
		t.Fatalf("the cut cites %v, want the scenario %v", cut[0].Causes, want)
	}
	restored := rec.ofKind(domain.KindLinkRestored)
	if len(restored) != 1 {
		t.Fatalf("link_restored = %d, want 1", len(restored))
	}
	if want := []domain.Ref{rec.refOf(cut[0])}; !equalRefs(restored[0].Causes, want) {
		t.Fatalf("the repair cites %v, want the cut it ends %v", restored[0].Causes, want)
	}
}

// TestCutAndRepairKeepTheirOrderInTheHistory guards the other direction: a
// repair cited before the cut it ends would be a graph that describes an outage
// starting after it stopped.
func TestCutAndRepairKeepTheirOrderInTheHistory(t *testing.T) {
	e, rec := newTestEngine(t, EngineConfig{})
	for _, id := range []domain.NodeID{"A", "B"} {
		if err := e.AddNode(NodeConfig{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.AddLink(LinkConfig{From: "B", To: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := e.PartitionAt("B", "A", tickAt(100), tickAt(300)); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(tickAt(400)); err != nil {
		t.Fatal(err)
	}
	cut, restored := rec.ofKind(domain.KindLinkPartitioned)[0], rec.ofKind(domain.KindLinkRestored)[0]
	if rec.refOf(cut) >= rec.refOf(restored) {
		t.Fatalf("the cut (%s) is recorded after the repair that ends it (%s)", rec.refOf(cut), rec.refOf(restored))
	}
}
