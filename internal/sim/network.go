package sim

import (
	"errors"
	"fmt"

	"github.com/HalxDocs/aegis/internal/domain"
)

// ErrUnknownLink is returned when a message has no configured path. Scenarios
// must declare every link explicitly; an implicit default would make latency
// and drop behaviour depend on who forgot to configure what.
var ErrUnknownLink = errors.New("no link configured")

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
	return n.rec.Record(domain.Event{
		At:     at,
		Kind:   domain.KindMessageDropped,
		Node:   msg.From,
		Peer:   msg.To,
		Round:  msg.Round,
		Reason: domain.ReasonDropPolicy,
	})
}
