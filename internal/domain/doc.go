// Package domain defines the vocabulary Aegis reasons with: virtual time, node
// identifiers, the difference between a node's execution state and an
// observer's belief about it, and the events and reports that record both.
//
// The package imports nothing from the rest of the module. That is deliberate.
// Every other package may depend on these types; none of them may leak
// simulator internals back into them.
package domain
