package sim

import (
	"errors"
	"testing"

	"github.com/HalxDocs/aegis/internal/domain"
)

func mustLink(t *testing.T, from, to domain.NodeID) domain.Link {
	t.Helper()
	link, err := domain.NewLink(from, to)
	if err != nil {
		t.Fatal(err)
	}
	return link
}

// TestCutCrossesOnlyWhenTheTransitMeetsIt pins the boundary rule that decides
// whether a message survived a cut. Every case is an instant a reader of the log
// could reasonably disagree about, so the answer is written down rather than
// inferred from whichever task happened to run first.
func TestCutCrossesOnlyWhenTheTransitMeetsIt(t *testing.T) {
	c := cut{from: tickAt(300), until: tickAt(900)}
	cases := map[string]struct {
		start, end domain.Tick
		want       bool
	}{
		"well inside":               {start: tickAt(350), end: tickAt(370), want: true},
		"departs as the cut starts": {start: tickAt(300), end: tickAt(320), want: true},
		"arrives as the cut starts": {start: tickAt(280), end: tickAt(300), want: false},
		"in flight when it heals":   {start: tickAt(890), end: tickAt(910), want: true},
		"departs as it heals":       {start: tickAt(900), end: tickAt(920), want: false},
		"before the cut":            {start: tickAt(100), end: tickAt(120), want: false},
		"after the cut":             {start: tickAt(1000), end: tickAt(1020), want: false},
		"instant inside":            {start: tickAt(500), end: tickAt(500), want: true},
		"instant at the start":      {start: tickAt(300), end: tickAt(300), want: false},
		"instant at the heal":       {start: tickAt(900), end: tickAt(900), want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := c.crosses(tc.start, tc.end); got != tc.want {
				t.Fatalf("crosses(%d, %d) = %v, want %v", tc.start, tc.end, got, tc.want)
			}
		})
	}
}

// TestPartitionLosesOnlyMessagesThatCrossIt drives the whole send path: the
// canonical cluster, one cut on B->A, and the rounds that must and must not
// arrive. The B->C link keeps its traffic, so a lost delivery cannot be blamed
// on the receiver or the sender being broken.
func TestPartitionLosesOnlyMessagesThatCrossIt(t *testing.T) {
	cases := map[string]struct {
		from, until domain.Tick
		lost        []uint64
	}{
		// Rounds leave B at 100, 200, 300, 400, 500 and arrive 20 ms later.
		"before any traffic":      {from: tickAt(50), until: tickAt(90)},
		"nothing in the window":   {from: tickAt(600), until: tickAt(700)},
		"loss in flight":          {from: tickAt(210), until: tickAt(400), lost: []uint64{2, 3}},
		"boundary at the send":    {from: tickAt(300), until: tickAt(400), lost: []uint64{3}},
		"boundary at the arrival": {from: tickAt(220), until: tickAt(400), lost: []uint64{3}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e, rec, rx := canonicalEngine(t)
			if err := e.PartitionAt("B", "A", tc.from, tc.until); err != nil {
				t.Fatalf("PartitionAt: %v", err)
			}
			if err := e.Start(); err != nil {
				t.Fatalf("Start: %v", err)
			}
			if err := e.Run(tickAt(550)); err != nil {
				t.Fatalf("Run: %v", err)
			}

			lost := make(map[uint64]bool, len(tc.lost))
			for _, round := range tc.lost {
				lost[round] = true
			}

			var toA []uint64
			for _, hb := range rx.got {
				if hb.To == "A" {
					toA = append(toA, hb.Round)
					if lost[hb.Round] {
						t.Fatalf("round %d crossed the cut at %d..%d but was delivered at %d",
							hb.Round, tc.from, tc.until, hb.At)
					}
				}
			}
			if want := 5 - len(tc.lost); len(toA) != want {
				t.Fatalf("A received rounds %v, want %d of 5", toA, want)
			}

			dropped := rec.ofKind(domain.KindMessageDropped)
			if len(dropped) != len(tc.lost) {
				t.Fatalf("drops = %d, want %d", len(dropped), len(tc.lost))
			}
			for i, ev := range dropped {
				if ev.Peer != "A" || ev.Node != "B" {
					t.Fatalf("drop %d = %s->%s, want B->A", i, ev.Node, ev.Peer)
				}
				if ev.Reason != domain.ReasonPartition {
					t.Fatalf("drop %d reason = %q, want %q", i, ev.Reason, domain.ReasonPartition)
				}
				if !lost[ev.Round] {
					t.Fatalf("round %d was dropped but is not expected", ev.Round)
				}
				// The loss is reported when the message fails to arrive, not
				// when it leaves: the sender already believes it was sent.
				if want := tickAt(int64(ev.Round)*100 + 20); ev.At != want {
					t.Fatalf("round %d dropped at %d, want its arrival instant %d", ev.Round, ev.At, want)
				}
			}
			rec.requireClean(t)
		})
	}
}

// TestPartitionAnnouncesBothEnds checks the log explains itself: a dropped
// message carries a reason, and the reason has a cause written down at the
// instant it happened.
func TestPartitionAnnouncesBothEnds(t *testing.T) {
	e, rec, _ := canonicalEngine(t)
	if err := e.PartitionAt("B", "A", tickAt(250), tickAt(450)); err != nil {
		t.Fatalf("PartitionAt: %v", err)
	}
	if err := e.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.Run(tickAt(600)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	partitioned := rec.ofKind(domain.KindLinkPartitioned)
	if len(partitioned) != 1 {
		t.Fatalf("partitioned events = %d, want 1", len(partitioned))
	}
	if got := partitioned[0]; got.At != tickAt(250) || got.Node != "B" || got.Peer != "A" {
		t.Fatalf("partitioned event = %+v, want B->A at 250", got)
	}
	restored := rec.ofKind(domain.KindLinkRestored)
	if len(restored) != 1 {
		t.Fatalf("restored events = %d, want 1", len(restored))
	}
	if got := restored[0]; got.At != tickAt(450) || got.Node != "B" || got.Peer != "A" {
		t.Fatalf("restored event = %+v, want B->A at 450", got)
	}
	rec.requireClean(t)
}

// TestPartitionBeyondTheHorizonIsNeverRepaired covers a cut that outlives the
// run: the log must not claim a repair that did not happen.
func TestPartitionBeyondTheHorizonIsNeverRepaired(t *testing.T) {
	e, rec, _ := canonicalEngine(t)
	if err := e.PartitionAt("B", "A", tickAt(250), tickAt(5000)); err != nil {
		t.Fatalf("PartitionAt: %v", err)
	}
	if err := e.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.Run(tickAt(600)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n := rec.count(domain.KindLinkPartitioned); n != 1 {
		t.Fatalf("partitioned events = %d, want 1", n)
	}
	if n := rec.count(domain.KindLinkRestored); n != 0 {
		t.Fatalf("restored events = %d, want none before the horizon", n)
	}
}

func TestPartitionRejectsIncoherentIntervals(t *testing.T) {
	link := mustLink(t, "B", "A")
	cases := map[string]struct {
		from, until domain.Tick
		seed        *cut
		want        error
	}{
		"ends before it begins": {from: tickAt(450), until: tickAt(250), want: ErrCutOrder},
		"zero length":           {from: tickAt(300), until: tickAt(300), want: ErrCutOrder},
		"overlaps": {
			from: tickAt(400), until: tickAt(600),
			seed: &cut{from: tickAt(300), until: tickAt(500)}, want: ErrCutOverlap,
		},
		"touches": {
			from: tickAt(400), until: tickAt(500),
			seed: &cut{from: tickAt(300), until: tickAt(400)}, want: ErrCutOverlap,
		},
		"swallows": {
			from: tickAt(200), until: tickAt(900),
			seed: &cut{from: tickAt(300), until: tickAt(400)}, want: ErrCutOverlap,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			net, _, _, _ := newTestNetwork(t, 1)
			if err := net.addLink(LinkConfig{From: "B", To: "A", Latency: tickAt(20)}); err != nil {
				t.Fatal(err)
			}
			if tc.seed != nil {
				net.addCut(link, tc.seed.from, tc.seed.until)
			}
			if err := net.validateCut(link, tc.from, tc.until); !errors.Is(err, tc.want) {
				t.Fatalf("validateCut() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPartitionRequiresADeclaredLink(t *testing.T) {
	e, _, _ := canonicalEngine(t)
	err := e.PartitionAt("A", "C", tickAt(300), tickAt(400))
	if !errors.Is(err, ErrUnknownLink) {
		t.Fatalf("error = %v, want ErrUnknownLink", err)
	}
	err = e.PartitionAt("bad node", "A", tickAt(300), tickAt(400))
	if err == nil {
		t.Fatal("an invalid node id was accepted as a cut endpoint")
	}
}
