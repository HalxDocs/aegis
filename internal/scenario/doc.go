// Package scenario wires one simulated world together: nodes, links, observers
// and a horizon.
//
// This is the only package that knows about the simulator, the detector and the
// recorder at the same time. Keeping that wiring here means neither the
// simulator nor the detector can reach for the other's state, and a scenario is
// a plain Go value that a test can copy and change.
package scenario
