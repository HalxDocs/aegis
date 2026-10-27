package domain

import "strconv"

// Ref identifies a recorded event by the sequence number the recorder assigned
// to it.
//
// A reference is an identity, not a position: the recorder allocates sequence
// numbers densely and in virtual-time order, so a reference is also an ordering
// fact. An event that cites Ref 12 happened before every event numbered above
// 12, whatever the earlier one's timestamp says, which is what makes a causal
// graph impossible to invert.
//
// The zero Ref names no event. Nothing recorded is ever number zero, so a zero
// cause is a bug rather than a reference to the opening event.
type Ref uint64

// Valid reports whether r names a recorded event.
func (r Ref) Valid() bool { return r > 0 }

// String returns the reference as it appears in a log.
func (r Ref) String() string { return strconv.FormatUint(uint64(r), 10) }
