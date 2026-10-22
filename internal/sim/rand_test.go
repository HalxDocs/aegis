package sim

import "testing"

// These vectors were produced by an independent splitmix64 implementation, not
// by this code, so the test checks the algorithm rather than restating it.
func TestRNGMatchesReferenceVectors(t *testing.T) {
	cases := []struct {
		name string
		seed uint64
		want [5]uint64
	}{
		{
			name: "zero seed",
			seed: 0,
			want: [5]uint64{0xe220a8397b1dcdaf, 0x6e789e6aa1b965f4, 0x06c45d188009454f, 0xf88bb8a8724c81ec, 0x1b39896a51a8749b},
		},
		{
			name: "one",
			seed: 1,
			want: [5]uint64{0x910a2dec89025cc1, 0xbeeb8da1658eec67, 0xf893a2eefb32555e, 0x71c18690ee42c90b, 0x71bb54d8d101b5b9},
		},
		{
			name: "deadbeef",
			seed: 0xDEADBEEF,
			want: [5]uint64{0x4adfb90f68c9eb9b, 0xde586a3141a10922, 0x021fbc2f8e1cfc1d, 0x7466ce737be16790, 0x3bfa8764f685bd1c},
		},
		{
			name: "wide",
			seed: 0x0123456789ABCDEF,
			want: [5]uint64{0x157a3807a48faa9d, 0xd573529b34a1d093, 0x2f90b72e996dccbe, 0xa2d419334c4667ec, 0x01404ce914938008},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRNG(tc.seed)
			for i, want := range tc.want {
				if got := r.next(); got != want {
					t.Fatalf("draw %d = %#016x, want %#016x", i, got, want)
				}
			}
		})
	}
}

func TestRNGHitBoundaries(t *testing.T) {
	r := newRNG(7)
	for i := 0; i < 100; i++ {
		if r.hit(0) {
			t.Fatal("hit(0) must never fire")
		}
		if !r.hit(ppmScale) {
			t.Fatal("hit(1_000_000) must always fire")
		}
		if !r.hit(ppmScale + 1) {
			t.Fatal("hit above full scale must always fire")
		}
	}
}

func TestRNGHitDoesNotDrawAtBoundaries(t *testing.T) {
	untouched := newRNG(99)
	touched := newRNG(99)
	for i := 0; i < 10; i++ {
		touched.hit(0)
		touched.hit(ppmScale)
	}
	if a, b := untouched.next(), touched.next(); a != b {
		t.Fatalf("boundary decisions consumed randomness: %#x != %#x", a, b)
	}
}

func TestRNGHitIsRoughlyProportional(t *testing.T) {
	r := newRNG(0xC0FFEE)
	const draws = 100_000
	hits := 0
	for i := 0; i < draws; i++ {
		if r.hit(250_000) {
			hits++
		}
	}
	ratio := float64(hits) / draws
	if ratio < 0.24 || ratio > 0.26 {
		t.Fatalf("hit rate = %.4f, want near 0.25", ratio)
	}
}
