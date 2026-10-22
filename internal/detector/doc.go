// Package detector turns received heartbeats into per-observer beliefs.
//
// It imports internal/domain and the standard library, and nothing else. It has
// no way to read a node's execution state: its inputs are messages that actually
// arrived, a clock, a scheduler and a place to append events. A crashed node is
// therefore indistinguishable from a silent one, which is the point — this
// package reports only what an observer can justify.
//
// Belief is kept per observer. Two detectors watching the same target hold
// separate observations with separate deadlines, so they may disagree, and the
// log records both opinions rather than one averaged verdict.
package detector
