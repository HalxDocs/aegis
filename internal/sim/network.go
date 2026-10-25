package sim

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/HalxDocs/aegis/internal/domain"
)

// ErrUnknownLink is returned when a message has no configured path. Scenarios
// must declare every link explicitly; an implicit default would make latency
// and drop behaviour depend on who forgot to configure what.
var ErrUnknownLink = errors.New("no link configured")

var (
	// ErrCutOrder is returned for a cut that ends at or before it begins.
	ErrCutOrder = errors.New("cut does not end after it begins")

	// ErrCutOverlap is returned when two cuts on one link overlap or touch.
	// Separate cuts that meet at an instant describe one continuous outage
	// while recording two of them, so the ambiguous case is refused rather
	// than resolved by ordering.
	ErrCutOverlap = errors.New("cut overlaps another cut on the same link")
)

// Message is one heartbeat in flight. Round is assigned by the sender and
// increases monotonically, so a reader can count what an observer never heard.
type Message struct {
	From  domain.NodeID
	To    domain.NodeID
	Round uint64
}

// LinkConfig describes one directed path.
//
// Latency is virtual and fixed for the run. DropPPM is a probability in parts
// per million, decided once at send time: a dropped message never enters the
// network, so "lost" and "late" stay different observations.
type LinkConfig struct {
	From    domain.NodeID
	To      domain.NodeID
	Latency domain.Tick
	DropPPM uint32
}

func (c LinkConfig) validate() (domain.Link, error) {
	link, err := domain.NewLink(c.From, c.To)
	if err != nil {
		return domain.Link{}, err
	}
	if c.Latency < 0 {
		return domain.Link{}, fmt.Errorf("link %s: negative latency %d", link, c.Latency)
	}
	if c.DropPPM > ppmScale {
		return domain.Link{}, fmt.Errorf("link %s: drop rate %d exceeds %d ppm", link, c.DropPPM, ppmScale)
	}
	return link, nil
}

// wire is the behaviour configured for one link.
type wire struct {
	latency domain.Tick
	dropPPM uint32

	// cuts are the scheduled outages of this direction, sorted by start. They are
	// configuration, not runtime state: whether a message is lost depends only on
	// the intervals and the message's own transit, never on which task happened
	// to run first at a boundary instant.
	cuts []cut
}

// cut is a one-way outage, half-open in time: the path is unusable from from up
// to, but not including, until.
type cut struct {
	from  domain.Tick
	until domain.Tick
}

// crosses reports whether a message that left at start and arrives at end was
// on the path while it was cut.
//
// The test is the intersection of the closed transit [start, end] with the open
// cut (from, until). Treating the cut as open is what makes the boundary
// decidable without a tie-break: a message that arrives exactly as the path is
// cut still arrives, and one that departs exactly as the path heals is not
// delayed. A zero-latency message is decided by the single instant it occupies.
func (c cut) crosses(start, end domain.Tick) bool {
	return c.from < end && start < c.until
}

// cutDuring reports whether any configured cut is crossed by this transit.
//
// A message sent before a cut and due after it started is lost in flight, which
// is why the whole transit is tested rather than the send instant alone.
func (w wire) cutDuring(start, end domain.Tick) bool {
	for _, c := range w.cuts {
		if c.crosses(start, end) {
			return true
		}
	}
	return false
}

type network struct {
	links   map[domain.Link]wire
	rng     *rng
	sched   domain.Scheduler
	rec     EventRecorder
	deliver func(at domain.Tick, msg Message)
}

func newNetwork(sched domain.Scheduler, rec EventRecorder, rng *rng) *network {
	return &network{
		links:   make(map[domain.Link]wire),
		sched:   sched,
		rec:     rec,
		rng:     rng,
		deliver: func(domain.Tick, Message) {},
	}
}

// addLink configures one directed path. Reconfiguring the same path is an
// error: a scenario that lists a link twice has two possible latencies, and
// picking one silently would make the run depend on ordering.
func (n *network) addLink(cfg LinkConfig) error {
	link, err := cfg.validate()
	if err != nil {
		return err
	}
	if _, exists := n.links[link]; exists {
		return fmt.Errorf("link %s is configured more than once", link)
	}
	n.links[link] = wire{latency: cfg.Latency, dropPPM: cfg.DropPPM}
	return nil
}

// validateCut rejects an outage the network cannot represent, without changing
// anything. It is separate from addCut so a caller can fail before it schedules
// the events that announce the cut.
func (n *network) validateCut(link domain.Link, from, until domain.Tick) error {
	if until <= from {
		return fmt.Errorf("%w: %s cut %d..%d", ErrCutOrder, link, from, until)
	}
	w, ok := n.links[link]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownLink, link)
	}
	for _, existing := range w.cuts {
		if existing.from <= until && from <= existing.until {
			return fmt.Errorf("%w: %s cut %d..%d meets %d..%d",
				ErrCutOverlap, link, from, until, existing.from, existing.until)
		}
	}
	return nil
}

// addCut records a validated outage, keeping the link's cuts ordered by start.
func (n *network) addCut(link domain.Link, from, until domain.Tick) {
	w := n.links[link]
	at := sort.Search(len(w.cuts), func(i int) bool { return w.cuts[i].from > from })
	w.cuts = slices.Insert(w.cuts, at, cut{from: from, until: until})
	n.links[link] = w
}

// send puts a message on the wire at the given virtual time.
//
// The delivery is scheduled before the send is recorded so a scheduling
// failure cannot leave a "sent" event in the history for a message that was
// never accepted. The delivery cannot overtake the record: scheduling only
// queues work, and queued work runs after the current task returns.
func (n *network) send(at domain.Tick, msg Message) error {
	link, err := domain.NewLink(msg.From, msg.To)
	if err != nil {
		return err
	}
	w, ok := n.links[link]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownLink, link)
	}
	if msg.Round == 0 {
		return fmt.Errorf("message %s: round must be positive", link)
	}

	dropped := n.rng.hit(w.dropPPM)
	if !dropped {
		arrives := at + w.latency
		if _, err := n.sched.At(arrives, func() { n.deliver(arrives, msg) }); err != nil {
			return fmt.Errorf("schedule delivery of %s: %w", link, err)
		}
	}
	if err := n.rec.Record(domain.Event{
		At:    at,
		Kind:  domain.KindHeartbeatSent,
		Node:  msg.From,
		Peer:  msg.To,
		Round: msg.Round,
	}); err != nil {
		return err
	}
	if !dropped {
		return nil
	}
	return n.recordDrop(at, msg, domain.ReasonDropPolicy)
}

// recordDrop writes the outcome of a message that never reached its receiver.
func (n *network) recordDrop(at domain.Tick, msg Message, reason string) error {
	return n.rec.Record(domain.Event{
		At:     at,
		Kind:   domain.KindMessageDropped,
		Node:   msg.From,
		Peer:   msg.To,
		Round:  msg.Round,
		Reason: reason,
	})
}
