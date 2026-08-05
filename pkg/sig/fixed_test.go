package sig

import (
	"testing"
)

// fixedOf returns a Fixed with the given bits set.
func fixedOf(bits ...uint) Fixed {
	var f Fixed
	for _, b := range bits {
		f.Set(b)
	}
	return f
}

// TestFixedSetCoversEveryBit pins the unrolled Set to the width of the vector:
// every offset below FixedBits must set a bit of its own, and between them they
// must cover the whole vector.
func TestFixedSetCoversEveryBit(t *testing.T) {
	var all, want Fixed
	for i := range want {
		want[i] = ^uint64(0)
	}
	for bit := uint(0); bit < FixedBits; bit++ {
		f := fixedOf(bit)
		if (f == Fixed{}) {
			t.Fatalf("Set(%d) set no bit", bit)
		}
		if all.Contains(f) {
			t.Errorf("Set(%d) reused a bit already set by a lower offset", bit)
		}
		for i := range all {
			all[i] |= f[i]
		}
	}
	if all != want {
		t.Errorf("bits reachable through Set = %#x, want %#x", all, want)
	}
}

func TestFixedContains(t *testing.T) {
	var empty Fixed
	// One bit in each word, so that a difference in either is exercised.
	both := fixedOf(1, 70)
	for _, tc := range []struct {
		name  string
		s     Fixed
		other Fixed
		want  bool
	}{
		{"empty contains empty", empty, empty, true},
		{"any contains empty", both, empty, true},
		{"empty contains none", empty, both, false},
		{"self", both, both, true},
		{"subset in the first word", both, fixedOf(1), true},
		{"subset in the last word", both, fixedOf(70), true},
		{"missing bit in the first word", both, fixedOf(1, 2), false},
		{"missing bit in the last word", both, fixedOf(70, 71), false},
		{"disjoint", fixedOf(1), fixedOf(70), false},
	} {
		if got := tc.s.Contains(tc.other); got != tc.want {
			t.Errorf("%s: %#x.Contains(%#x) = %t, want %t", tc.name, tc.s, tc.other, got, tc.want)
		}
	}
}

func TestFixedEqual(t *testing.T) {
	var empty Fixed
	both := fixedOf(1, 70)
	for _, tc := range []struct {
		name  string
		s     Fixed
		other Fixed
		want  bool
	}{
		{"empty", empty, empty, true},
		{"self", both, both, true},
		{"order of the offsets", fixedOf(1, 70), fixedOf(70, 1), true},
		{"repeated offsets", both, fixedOf(1, 1, 70), true},
		{"differs in the first word", both, fixedOf(2, 70), false},
		{"differs in the last word", both, fixedOf(1, 71), false},
		{"subset", both, fixedOf(1), false},
		{"empty against non-empty", empty, both, false},
	} {
		if got := tc.s.Equal(tc.other); got != tc.want {
			t.Errorf("%s: %#x.Equal(%#x) = %t, want %t", tc.name, tc.s, tc.other, got, tc.want)
		}
		// Equal is symmetric.
		if got := tc.other.Equal(tc.s); got != tc.want {
			t.Errorf("%s: %#x.Equal(%#x) = %t, want %t", tc.name, tc.other, tc.s, got, tc.want)
		}
	}
}

// TestFixedNoFalseNegatives is the property that makes a Fixed usable as a
// filter: whenever the offsets of one set contain the offsets of another, the
// signatures must agree. Offsets are used directly here rather than hashed
// keys, as folding onto a bit is the caller's business.
func TestFixedNoFalseNegatives(t *testing.T) {
	// Every subset of a handful of offsets, spanning both words and including
	// the boundary between them.
	offsets := []uint{0, 1, 63, 64, 65, FixedBits - 1}
	for a := 0; a < 1<<len(offsets); a++ {
		for b := 0; b < 1<<len(offsets); b++ {
			var sa, sb Fixed
			for i, off := range offsets {
				if a&(1<<i) != 0 {
					sa.Set(off)
				}
				if b&(1<<i) != 0 {
					sb.Set(off)
				}
			}
			// The offsets are distinct, so containment of the bits is exactly
			// containment of the sets and there are no false positives either.
			want := a&b == b
			if got := sa.Contains(sb); got != want {
				t.Fatalf("%#x.Contains(%#x) = %t, want %t (offsets %06b and %06b)", sa, sb, got, want, a, b)
			}
			if got := sa.Equal(sb); got != (a == b) {
				t.Fatalf("%#x.Equal(%#x) = %t, want %t (offsets %06b and %06b)", sa, sb, got, a == b, a, b)
			}
		}
	}
}
