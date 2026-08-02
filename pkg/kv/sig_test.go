package kv

import (
	"fmt"
	"testing"
)

func TestSigBits(t *testing.T) {
	for _, tc := range []struct {
		in   uint
		want uint
	}{
		{0, 64},
		{1, 64},
		{63, 64},
		{64, 64},
		{65, 128},
		{128, 128},
		{129, 256},
		{1000, 1024},
		{maxSigBits, maxSigBits},
	} {
		if got := sigBits(tc.in); got != tc.want {
			t.Errorf("sigBits(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestNewSignatureWidth(t *testing.T) {
	for _, bitWidth := range []uint{0, 1, 64, 65, 512, 1000} {
		sig := NewSignature(bitWidth)
		want := sigBits(bitWidth)
		if got := sig.BitWidth(); got != want {
			t.Errorf("NewSignature(%d).BitWidth() = %d, want %d", bitWidth, got, want)
		}
		if got := uint(len(sig.words)) * 64; got != want {
			t.Errorf("NewSignature(%d) allocated %d bits, want %d", bitWidth, got, want)
		}
	}
}

func TestNewSignatureTooWide(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("NewSignature(%d) did not panic", maxSigBits+1)
		}
	}()
	NewSignature(maxSigBits + 1)
}

func TestSignatureContainsMismatchedWidthPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("Contains across widths 64 and 128 did not panic")
		}
	}()
	NewSignature(64, "a").Contains(NewSignature(128, "a"))
}

func TestSignatureEmpty(t *testing.T) {
	empty := NewSignature(128)
	full := NewSignature(128, "a", "b", "c")

	if !full.Contains(empty) {
		t.Errorf("a non-empty signature does not contain the empty one")
	}
	if !empty.Contains(empty) || !empty.Equal(NewSignature(128)) {
		t.Errorf("the empty signature is not equal to itself")
	}
	if empty.Contains(full) {
		t.Errorf("the empty signature contains a non-empty one")
	}
	// The zero value is a valid, if unusable, width-zero signature.
	var zero Signature
	if !zero.Contains(&Signature{}) || zero.BitWidth() != 0 {
		t.Errorf("the zero Signature does not contain itself")
	}
}

func TestSignatureOrderAndDuplicates(t *testing.T) {
	a := NewSignature(256, "a", "b", "c")
	b := NewSignature(256, "c", "a", "b")
	c := NewSignature(256, "a", "a", "b", "c", "c", "c")
	if !a.Equal(b) {
		t.Errorf("signatures differ by key order")
	}
	if !a.Equal(c) {
		t.Errorf("signatures differ by duplicate keys")
	}
	// Width is part of equality, and a wider signature is not comparable at all.
	if a.Equal(NewSignature(512, "a", "b", "c")) {
		t.Errorf("signatures of different widths compare equal")
	}
}

// TestSignatureCoversEveryKey checks that every key's bit really lands in the
// vector: a signature must contain the single-key signature of each of its
// keys.
func TestSignatureCoversEveryKey(t *testing.T) {
	keys := []string{"", "a", "b", "carrot", "a-longer-key", "\x00\xff"}
	for _, bitWidth := range []uint{64, 65, 128, 1000} {
		sig := NewSignature(bitWidth, keys...)
		for _, k := range keys {
			if !sig.Contains(NewSignature(bitWidth, k)) {
				t.Errorf("width %d: signature of %v is missing key %q", bitWidth, keys, k)
			}
		}
	}
}

// TestSignatureNoFalseNegatives is the property that makes a Signature usable
// as a filter: whenever a really contains b, the signatures must agree. The
// converse is allowed to be wrong, and at width 64 with this key pool it
// regularly is, so it is only counted.
func TestSignatureNoFalseNegatives(t *testing.T) {
	next := randMaps(5)
	for _, bitWidth := range []uint{64, 1024} {
		falsePositives := 0
		for i := 0; i < 10000; i++ {
			am, bm := next(), next()
			a, b := sigOfKeys(bitWidth, am), sigOfKeys(bitWidth, bm)
			got, want := a.Contains(b), containsAllKeys(am, bm)
			if want && !got {
				t.Fatalf("width %d: signature of %v does not contain signature of %v, but the key set does",
					bitWidth, keysOf(am), keysOf(bm))
			}
			if got && !want {
				falsePositives++
			}
		}
		t.Logf("width %d: %d/10000 false positives", bitWidth, falsePositives)
	}
}

// sigOfKeys builds a Signature over the keys of m.
func sigOfKeys(bitWidth uint, m map[string]string) *Signature {
	return NewSignature(bitWidth, keysOf(m)...)
}

func keysOf(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// containsAllKeys is the obvious reference implementation of key containment.
func containsAllKeys(a, b map[string]string) bool {
	for k := range b {
		if _, ok := a[k]; !ok {
			return false
		}
	}
	return true
}

func benchSignature(bitWidth uint, n int) *Signature {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%04d", i)
	}
	return NewSignature(bitWidth, keys...)
}

func BenchmarkSignatureContains(b *testing.B) {
	for _, bitWidth := range []uint{64, 1024, 65536} {
		big := benchSignature(bitWidth, 64)
		hit := NewSignature(bitWidth, "key-0016", "key-0017")
		miss := NewSignature(bitWidth, "key-1000", "key-1001")
		if !big.Contains(hit) {
			b.Fatalf("width %d: benchmark inputs are wrong", bitWidth)
		}
		b.Run(fmt.Sprintf("bits=%d/hit", bitWidth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if !big.Contains(hit) {
					b.Fatal("want contains")
				}
			}
		})
		b.Run(fmt.Sprintf("bits=%d/miss", bitWidth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				big.Contains(miss)
			}
		})
	}
}
