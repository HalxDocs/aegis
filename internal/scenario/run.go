package scenario

import (
	"fmt"

	"github.com/HalxDocs/aegis/internal/detector"
	"github.com/HalxDocs/aegis/internal/domain"
	"github.com/HalxDocs/aegis/internal/observability"
	"github.com/HalxDocs/aegis/internal/sim"
)

// Run builds the world described by spec, executes it to the horizon with the
// given seed, and returns the history together with the final state of both
// layers.
//
// Every run owns its engine, detector and recorder, so concurrent runs share no
// mutable state. The only cross-run input is the spec, which is treated as
// read-only and copied where the engine could otherwise alias it.
func Run(spec Spec, seed uint64) (Result, error) {
	spec = spec.clone()
	if err := spec.validate(); err != nil {
		return Result{}, err
	}

	log := observability.NewRecorder()
	engine, err := sim.NewEngine(sim.EngineConfig{Recorder: log, Seed: seed})
	if err != nil {
		return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
	}

	for _, cfg := range spec.Nodes {
		if err := engine.AddNode(cfg); err != nil {
			return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
		}
	}
	for _, cfg := range spec.Links {
		if err := engine.AddLink(cfg); err != nil {
			return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
		}
	}

	// Cuts are scheduled before the history opens, so a rejected cut cannot
	// leave a half-written log behind.
	for _, cut := range spec.Partitions {
		if err := engine.PartitionAt(cut.From, cut.To, cut.At, cut.Until); err != nil {
			return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
		}
	}

	observers := make([]*detector.Detector, 0, len(spec.Observers))
	for _, o := range spec.Observers {
		det, err := detector.New(detector.Config{
			Observer:  o.Observer,
			Timeout:   o.Timeout,
			Clock:     engine,
			Scheduler: engine,
			Recorder:  log,
			Faults:    engine,
		})
		if err != nil {
			return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
		}
		if err := det.Watch(o.Target); err != nil {
			return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
		}
		if err := engine.AddReceiver(det); err != nil {
			return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
		}
		observers = append(observers, det)
	}

	// Everything is configured, so the history can open. Nothing above this
	// point writes an event, which keeps a configuration error from leaving a
	// half-written log behind.
	if _, err := log.Record(domain.Event{At: engine.Now(), Kind: domain.KindScenarioStarted, Scenario: spec.Name}); err != nil {
		return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
	}
	if err := engine.Start(); err != nil {
		return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
	}
	if spec.Crash != nil {
		if err := engine.CrashAt(spec.Crash.At, spec.Crash.Node); err != nil {
			return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
		}
	}
	if err := engine.Run(spec.Horizon); err != nil {
		return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
	}
	if _, err := log.Record(domain.Event{At: engine.Now(), Kind: domain.KindScenarioFinished, Scenario: spec.Name}); err != nil {
		return Result{}, fmt.Errorf("scenario %s: %w", spec.Name, err)
	}

	return collect(spec, engine, log, observers), nil
}

func collect(spec Spec, engine *sim.Engine, log *observability.Recorder, observers []*detector.Detector) Result {
	res := Result{
		Spec:    spec,
		Log:     log,
		Truth:   make(map[domain.NodeID]domain.NodeState, len(spec.Nodes)),
		Beliefs: make(map[ObservationKey]domain.Observation, len(spec.Observers)),
	}
	for _, cfg := range spec.Nodes {
		if state, ok := engine.Truth(cfg.ID); ok {
			res.Truth[cfg.ID] = state
		}
	}
	for _, det := range observers {
		for _, target := range det.Targets() {
			if obs, ok := det.Observation(target); ok {
				res.Beliefs[ObservationKey{Observer: det.Observer(), Target: target}] = obs
			}
		}
		res.Reports = append(res.Reports, det.Reports()...)
	}
	return res
}
