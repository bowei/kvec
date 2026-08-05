package sig

import (
	"fmt"
	"testing"
)

// hex renders a fixed vector as its words in hex. It takes any because the
// tests below print values of their type parameter, which vet's printf check
// cannot see through to a formattable type.
func hex(f any) string { return fmt.Sprintf("%#x", f) }

// fixedPtr is the operations every fixed vector shares. The methods sit on the
// pointer receiver, so the tests below are written over the pointer type PT and
// its element type T, which is the value the methods take and compare.
type fixedPtr[T any] interface {
	*T
	Equal(T) bool
	Contains(T) bool
	Set(uint)
}

// fixedOf returns a fixed vector with the given bits set.
func fixedOf[T any, PT fixedPtr[T]](bits ...uint) T {
	var f T
	for _, b := range bits {
		PT(&f).Set(b)
	}
	return f
}

func TestFixed64(t *testing.T) {
	testFixedSetCoversEveryBit(t, Fixed64Bits, func(f *Fixed64) []uint64 { return f[:] })
	testFixedContains[Fixed64](t, Fixed64Bits)
	testFixedEqual[Fixed64](t, Fixed64Bits)
	testFixedNoFalseNegatives[Fixed64](t, []uint{0, 1, 2, 31, 62, Fixed64Bits - 1})
}

func TestFixed128(t *testing.T) {
	testFixedSetCoversEveryBit(t, Fixed128Bits, func(f *Fixed128) []uint64 { return f[:] })
	testFixedContains[Fixed128](t, Fixed128Bits)
	testFixedEqual[Fixed128](t, Fixed128Bits)
	testFixedNoFalseNegatives[Fixed128](t, []uint{0, 1, 63, 64, 65, Fixed128Bits - 1})
}

func TestFixed256(t *testing.T) {
	testFixedSetCoversEveryBit(t, Fixed256Bits, func(f *Fixed256) []uint64 { return f[:] })
	testFixedContains[Fixed256](t, Fixed256Bits)
	testFixedEqual[Fixed256](t, Fixed256Bits)
	testFixedNoFalseNegatives[Fixed256](t, []uint{0, 63, 64, 127, 128, Fixed256Bits - 1})
}

// testFixedSetCoversEveryBit pins the unrolled Set to the width of the vector:
// every offset below nbits must set a bit of its own, and between them they must
// cover the whole vector. Offsets at or above nbits must set nothing at all.
//
// words hands back the underlying vector, which the shared methods do not
// expose; it is what lets this check reach past the API to the storage.
func testFixedSetCoversEveryBit[T comparable, PT fixedPtr[T]](t *testing.T, nbits uint, words func(*T) []uint64) {
	t.Helper()

	var all, want, zero T
	for i := range words(&want) {
		words(&want)[i] = ^uint64(0)
	}
	for bit := uint(0); bit < nbits; bit++ {
		f := fixedOf[T, PT](bit)
		if f == zero {
			t.Fatalf("Set(%d) set no bit", bit)
		}
		if PT(&all).Contains(f) {
			t.Errorf("Set(%d) reused a bit already set by a lower offset", bit)
		}
		fw := words(&f)
		for i, w := range words(&all) {
			words(&all)[i] = w | fw[i]
		}
	}
	if all != want {
		t.Errorf("bits reachable through Set = %s, want %s", hex(all), hex(want))
	}

	// Out of range offsets are documented to drop rather than wrap onto a bit
	// that a real element could own.
	for _, bit := range []uint{nbits, nbits + 1, 2*nbits - 1, 2 * nbits} {
		if f := fixedOf[T, PT](bit); f != zero {
			t.Errorf("Set(%d) = %s, want no bit set for an offset at or above %d", bit, hex(f), nbits)
		}
	}
}

func testFixedContains[T comparable, PT fixedPtr[T]](t *testing.T, nbits uint) {
	t.Helper()

	var empty T
	// lo sits in the first word and hi in the last, so that a difference in
	// either is exercised. Both leave room for a neighbouring offset.
	lo, hi := uint(1), nbits-2
	both := fixedOf[T, PT](lo, hi)
	for _, tc := range []struct {
		name  string
		s     T
		other T
		want  bool
	}{
		{"empty contains empty", empty, empty, true},
		{"any contains empty", both, empty, true},
		{"empty contains none", empty, both, false},
		{"self", both, both, true},
		{"subset in the first word", both, fixedOf[T, PT](lo), true},
		{"subset in the last word", both, fixedOf[T, PT](hi), true},
		{"missing bit in the first word", both, fixedOf[T, PT](lo, lo+1), false},
		{"missing bit in the last word", both, fixedOf[T, PT](hi, hi+1), false},
		{"disjoint", fixedOf[T, PT](lo), fixedOf[T, PT](hi), false},
	} {
		if got := PT(&tc.s).Contains(tc.other); got != tc.want {
			t.Errorf("%s: %s.Contains(%s) = %t, want %t", tc.name, hex(tc.s), hex(tc.other), got, tc.want)
		}
	}
}

func testFixedEqual[T comparable, PT fixedPtr[T]](t *testing.T, nbits uint) {
	t.Helper()

	var empty T
	lo, hi := uint(1), nbits-2
	both := fixedOf[T, PT](lo, hi)
	for _, tc := range []struct {
		name  string
		s     T
		other T
		want  bool
	}{
		{"empty", empty, empty, true},
		{"self", both, both, true},
		{"order of the offsets", fixedOf[T, PT](lo, hi), fixedOf[T, PT](hi, lo), true},
		{"repeated offsets", both, fixedOf[T, PT](lo, lo, hi), true},
		{"differs in the first word", both, fixedOf[T, PT](lo+1, hi), false},
		{"differs in the last word", both, fixedOf[T, PT](lo, hi+1), false},
		{"subset", both, fixedOf[T, PT](lo), false},
		{"empty against non-empty", empty, both, false},
	} {
		if got := PT(&tc.s).Equal(tc.other); got != tc.want {
			t.Errorf("%s: %s.Equal(%s) = %t, want %t", tc.name, hex(tc.s), hex(tc.other), got, tc.want)
		}
		// Equal is symmetric.
		if got := PT(&tc.other).Equal(tc.s); got != tc.want {
			t.Errorf("%s: %s.Equal(%s) = %t, want %t", tc.name, hex(tc.other), hex(tc.s), got, tc.want)
		}
	}
}

// testFixedNoFalseNegatives is the property that makes a fixed vector usable as
// a filter: whenever the offsets of one set contain the offsets of another, the
// signatures must agree. Offsets are used directly here rather than hashed keys,
// as folding onto a bit is the caller's business. The callers pass offsets that
// span every word and include the boundaries between them.
func testFixedNoFalseNegatives[T comparable, PT fixedPtr[T]](t *testing.T, offsets []uint) {
	t.Helper()

	// Every subset of the offsets, against every other.
	for a := 0; a < 1<<len(offsets); a++ {
		for b := 0; b < 1<<len(offsets); b++ {
			var sa, sb T
			for i, off := range offsets {
				if a&(1<<i) != 0 {
					PT(&sa).Set(off)
				}
				if b&(1<<i) != 0 {
					PT(&sb).Set(off)
				}
			}
			// The offsets are distinct, so containment of the bits is exactly
			// containment of the sets and there are no false positives either.
			want := a&b == b
			if got := PT(&sa).Contains(sb); got != want {
				t.Fatalf("%s.Contains(%s) = %t, want %t (offsets %06b and %06b)", hex(sa), hex(sb), got, want, a, b)
			}
			if got := PT(&sa).Equal(sb); got != (a == b) {
				t.Fatalf("%s.Equal(%s) = %t, want %t (offsets %06b and %06b)", hex(sa), hex(sb), got, a == b, a, b)
			}
		}
	}
}
