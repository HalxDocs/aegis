// Package recovery decides whether evidence justifies acting on a failure.
//
// It imports internal/domain and the standard library, and nothing else. That is
// the second half of the rule the detector lives under: a planner that could
// reach the simulator would be able to propose a restart because of something no
// observer witnessed, and an action taken for a reason nobody could have known
// is indistinguishable from a lucky guess. Here the only input is a list of
// failure reports, so a decision can be replayed from the log by anyone.
//
// The output is a decision, not an effect. Granting an action changes nothing on
// its own: the composition root performs it, because moving a node out of a
// crash is a change in ground truth and ground truth belongs to the simulator.
//
// A refusal is an outcome of the same machinery. The gate can decide that the
// evidence is too thin, and that decision is written to the history with the
// same weight as a grant — a system that only records the actions it took
// cannot be asked why it did nothing.
package recovery
