// Package domain defines the vocabulary Aegis reasons with: virtual time, node
// identifiers, the difference between a node's execution state and an
// observer's belief about it, and the events and reports that record both.
//
// Events are nodes in a causal graph. Causes names the events that produced one,
// as references to earlier entries in the same history, so a reader can walk
// backwards from a belief to the evidence that justified it. Nothing in the
// graph points forwards: a cause is always recorded before its effect, which the
// recorder enforces because it is the only component that assigns sequence
// numbers.
//
// The package imports nothing from the rest of the module. That is deliberate.
// Every other package may depend on these types; none of them may leak
// simulator internals back into them.
package domain
