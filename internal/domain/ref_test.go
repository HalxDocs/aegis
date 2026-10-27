package domain

import "testing"

func TestZeroRefNamesNoEvent(t *testing.T) {
	if Ref(0).Valid() {
		t.Fatal("zero was accepted as the reference of a recorded event")
	}
	for _, r := range []Ref{1, 2, 99} {
		if !r.Valid() {
			t.Fatalf("Ref(%d) was rejected as a reference", r)
		}
	}
}

func TestRefPrintsAsItsSequenceNumber(t *testing.T) {
	for _, tc := range []struct {
		ref  Ref
		want string
	}{
		{0, "0"},
		{7, "7"},
		{1234, "1234"},
	} {
		if got := tc.ref.String(); got != tc.want {
			t.Fatalf("Ref(%d).String() = %q, want %q", tc.ref, got, tc.want)
		}
	}
}
