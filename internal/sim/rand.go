package sim

// rng is a splitmix64 generator.
//
// The project does not use math/rand for simulated decisions. math/rand gives
// no cross-release guarantee that a given seed produces the same stream, and a
// milestone whose whole claim is reproducible replay cannot have its replay
// break on a toolchain upgrade. splitmix64 is eight lines, fully specified, and
// verified against an independent implementation in the tests.
type rng struct {
	state uint64
}

func newRNG(seed uint64) *rng {
	return &rng{state: seed}
}

// next returns the next value in the stream.
func (r *rng) next() uint64 {
	r.state += 0x9E3779B97F4A7C15
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// ppmScale is the denominator used for probability decisions: one million.
const ppmScale = 1_000_000

// hit reports whether a draw lands below ppm parts per million.
//
// The empty cases do not consume a draw. A link configured never to drop
// therefore cannot shift the stream that other links draw from, which keeps a
// scenario's randomness attached to the decisions that actually exist.
func (r *rng) hit(ppm uint32) bool {
	switch {
	case ppm == 0:
		return false
	case ppm >= ppmScale:
		return true
	}
	return r.next()%ppmScale < uint64(ppm)
}
