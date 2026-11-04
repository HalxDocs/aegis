package scenario

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/recovery"
	"github.com/HalxDocs/aegis/internal/sim"
)

// ErrInvalidFile identifies a scenario file that cannot describe a world.
var ErrInvalidFile = errors.New("invalid scenario file")

// duration is a length of virtual time written as a Go duration string.
//
// The file says "350ms" rather than 350000000 because a scenario is read by
// people before it is run. The tick count is still the stored form, so a file
// gains no precision it cannot express and the conversion stays exact.
type duration domain.Tick

// UnmarshalJSON reads a duration such as "20ms".
func (d *duration) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("a time must be a string such as \"20ms\", not %s", string(data))
	}
	parsed, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("%q is not a duration: %w", text, err)
	}
	if parsed < 0 {
		return fmt.Errorf("%q is negative; virtual time starts at zero", text)
	}
	*d = duration(domain.TickFrom(parsed))
	return nil
}

// MarshalJSON writes the duration back as the string it was read as.
func (d duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(domain.Tick(d).Duration().String())
}

// fileSpec is the on-disk form of a Spec.
//
// It is a separate type on purpose. Spec holds simulator configuration types and
// tick counts; this holds the names and strings a file may contain. Keeping them
// apart means the file format can be checked, described and refused on its own
// terms, and a change to either one has to be made deliberately in both.
type fileSpec struct {
	Name       string          `json:"name"`
	Horizon    duration        `json:"horizon"`
	Nodes      []fileNode      `json:"nodes,omitempty"`
	Links      []fileLink      `json:"links,omitempty"`
	Observers  []fileObserver  `json:"observers,omitempty"`
	Crash      *fileCrash      `json:"crash"`
	Partitions []filePartition `json:"partitions,omitempty"`
	Recovery   *fileRecovery   `json:"recovery"`
}

type fileNode struct {
	ID             domain.NodeID   `json:"id"`
	HeartbeatEvery duration        `json:"heartbeat_every"`
	Peers          []domain.NodeID `json:"peers"`
}

type fileLink struct {
	From    domain.NodeID `json:"from"`
	To      domain.NodeID `json:"to"`
	Latency duration      `json:"latency"`
	DropPPM uint32        `json:"drop_ppm"`
}

type fileObserver struct {
	Observer domain.NodeID `json:"observer"`
	Target   domain.NodeID `json:"target"`
	Timeout  duration      `json:"timeout"`
}

type fileCrash struct {
	Node domain.NodeID `json:"node"`
	At   duration      `json:"at"`
}

type filePartition struct {
	From  domain.NodeID `json:"from"`
	To    domain.NodeID `json:"to"`
	At    duration      `json:"at"`
	Until duration      `json:"until"`
}

type fileRecovery struct {
	Kind         domain.ActionKind `json:"kind"`
	MinWitnesses int               `json:"min_witnesses"`
	Patience     duration          `json:"patience"`
	Delay        duration          `json:"delay"`
}

// LoadFile reads and checks a scenario file.
func LoadFile(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, fmt.Errorf("scenario file %s: %w", path, err)
	}
	spec, err := Load(bytes.NewReader(data))
	if err != nil {
		return Spec{}, fmt.Errorf("scenario file %s: %w", path, err)
	}
	return spec, nil
}

// Load reads a scenario file and checks what the run would otherwise discover
// part-way through.
//
// The decode is strict. An unknown field is an error, not something ignored:
// "observor" would otherwise produce a run that looks correct and watches
// nothing, and the file would be the only record of what was meant.
func Load(r io.Reader) (Spec, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var f fileSpec
	if err := dec.Decode(&f); err != nil {
		return Spec{}, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	if err := rejectTrailing(dec); err != nil {
		return Spec{}, err
	}

	spec := f.spec()
	if err := spec.validate(); err != nil {
		return Spec{}, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	return spec, nil
}

// rejectTrailing refuses anything after the first object.
//
// A file holding two scenarios is not a scenario with more in it: it is one run
// and a puzzle about the rest. Naming that here is cheaper than a reader
// discovering later which half the log belongs to.
func rejectTrailing(dec *json.Decoder) error {
	var extra json.RawMessage
	if err := dec.Decode(&extra); err == nil {
		return fmt.Errorf("%w: the file holds more than one scenario", ErrInvalidFile)
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	return nil
}

// spec converts the file's names and strings into the types a run uses.
func (f fileSpec) spec() Spec {
	out := Spec{
		Name:    f.Name,
		Horizon: domain.Tick(f.Horizon),
	}
	for _, n := range f.Nodes {
		out.Nodes = append(out.Nodes, sim.NodeConfig{
			ID:             n.ID,
			HeartbeatEvery: domain.Tick(n.HeartbeatEvery),
			Peers:          n.Peers,
		})
	}
	for _, l := range f.Links {
		out.Links = append(out.Links, sim.LinkConfig{
			From:    l.From,
			To:      l.To,
			Latency: domain.Tick(l.Latency),
			DropPPM: l.DropPPM,
		})
	}
	for _, o := range f.Observers {
		out.Observers = append(out.Observers, ObserverSpec{
			Observer: o.Observer,
			Target:   o.Target,
			Timeout:  domain.Tick(o.Timeout),
		})
	}
	if f.Crash != nil {
		out.Crash = &CrashSpec{Node: f.Crash.Node, At: domain.Tick(f.Crash.At)}
	}
	for _, p := range f.Partitions {
		out.Partitions = append(out.Partitions, PartitionSpec{
			From:  p.From,
			To:    p.To,
			At:    domain.Tick(p.At),
			Until: domain.Tick(p.Until),
		})
	}
	if f.Recovery != nil {
		out.Recovery = &RecoverySpec{Policy: recovery.Policy{
			Kind:         f.Recovery.Kind,
			MinWitnesses: f.Recovery.MinWitnesses,
			Patience:     domain.Tick(f.Recovery.Patience),
			Delay:        domain.Tick(f.Recovery.Delay),
		}}
	}
	return out
}

// Marshal writes a spec in the file format Load reads.
//
// It exists so the format can be held to the built-in scenarios: a scenario the
// simulator can run but the file cannot express is a file format that would
// have been discovered to be incomplete by its first user.
func Marshal(spec Spec) ([]byte, error) {
	f := fileSpec{Name: spec.Name, Horizon: duration(spec.Horizon)}
	for _, n := range spec.Nodes {
		f.Nodes = append(f.Nodes, fileNode{
			ID:             n.ID,
			HeartbeatEvery: duration(n.HeartbeatEvery),
			Peers:          n.Peers,
		})
	}
	for _, l := range spec.Links {
		f.Links = append(f.Links, fileLink{
			From:    l.From,
			To:      l.To,
			Latency: duration(l.Latency),
			DropPPM: l.DropPPM,
		})
	}
	for _, o := range spec.Observers {
		f.Observers = append(f.Observers, fileObserver{
			Observer: o.Observer,
			Target:   o.Target,
			Timeout:  duration(o.Timeout),
		})
	}
	if spec.Crash != nil {
		f.Crash = &fileCrash{Node: spec.Crash.Node, At: duration(spec.Crash.At)}
	}
	for _, p := range spec.Partitions {
		f.Partitions = append(f.Partitions, filePartition{
			From:  p.From,
			To:    p.To,
			At:    duration(p.At),
			Until: duration(p.Until),
		})
	}
	if spec.Recovery != nil {
		f.Recovery = &fileRecovery{
			Kind:         spec.Recovery.Policy.Kind,
			MinWitnesses: spec.Recovery.Policy.MinWitnesses,
			Patience:     duration(spec.Recovery.Policy.Patience),
			Delay:        duration(spec.Recovery.Policy.Delay),
		}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	return append(data, '\n'), nil
}
