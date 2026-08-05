package sig

import (
	"fmt"
	"math/bits"
	"testing"

	"github.com/bowei/kvec/pkg/fnv"
)

// sigBenchWidths span one word, sixteen words and a kilo-word, so that the
// per-word cost of the loops can be separated from the fixed cost of the call.
var sigBenchWidths = []uint{64, 1024, 65536}

// benchSigKeys returns n distinct keys.
func benchSigKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%04d", i)
	}
	return keys
}

func benchSignature(bitWidth uint, n int) *Signature {
	return NewSignature(bitWidth, benchSigKeys(n)...)
}

// sigWithBit returns a signature of the given width with exactly one bit set.
// It writes the word directly rather than hashing a key so that the position of
// the difference, and so the word on which Contains exits, is exact.
func sigWithBit(bitWidth, bit uint) *Signature {
	sig := NewSignature(bitWidth)
	sig.words[bit/64] |= 1 << (bit % 64)
	return sig
}

// clearBit returns the index of a bit that sig does not have set, within the
// given word. A signature that leaves no room for one would make the miss
// benchmarks measure something other than what they claim, so this fails loudly.
func clearBit(b *testing.B, sig *Signature, word int) uint {
	b.Helper()
	free := ^sig.words[word]
	if free == 0 {
		b.Fatalf("word %d of the signature is saturated, cannot place a miss", word)
	}
	return uint(word)*64 + uint(bits.TrailingZeros64(free))
}

// BenchmarkNewSignature times construction: one allocation for the bit vector,
// which grows with the width, plus a hash and a store per key, which does not.
// The two costs are separated by sweeping each independently.
func BenchmarkNewSignature(b *testing.B) {
	for _, bitWidth := range sigBenchWidths {
		for _, n := range []int{0, 8, 64, 1024} {
			keys := benchSigKeys(n)
			b.Run(fmt.Sprintf("bits=%d/keys=%d", bitWidth, n), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					sigSink = NewSignature(bitWidth, keys...)
				}
			})
		}
	}
}

// BenchmarkSignatureContains separates the three shapes the loop can take. A
// hit always walks every word, so it is the full cost of the width. A miss
// exits on the first word that fails, so placing the difference in the first
// and in the last word brackets what a rejection can cost: the gap between
// them is what the early exit is worth.
func BenchmarkSignatureContains(b *testing.B) {
	for _, bitWidth := range sigBenchWidths {
		big := benchSignature(bitWidth, 64)
		last := len(big.words) - 1

		cases := []struct {
			name  string
			other *Signature
			want  bool
		}{
			{"hit/subset", NewSignature(bitWidth, "key-0016", "key-0017"), true},
			{"hit/self", big, true},
			{"miss/first-word", sigWithBit(bitWidth, clearBit(b, big, 0)), false},
		}
		// At one word wide the two miss cases are the same benchmark.
		if last > 0 {
			cases = append(cases, struct {
				name  string
				other *Signature
				want  bool
			}{"miss/last-word", sigWithBit(bitWidth, clearBit(b, big, last)), false})
		}

		for _, tc := range cases {
			if got := big.Contains(tc.other); got != tc.want {
				b.Fatalf("bits=%d/%s: Contains = %t, want %t", bitWidth, tc.name, got, tc.want)
			}
			b.Run(fmt.Sprintf("bits=%d/%s", bitWidth, tc.name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if big.Contains(tc.other) != tc.want {
						b.Fatal("wrong result")
					}
				}
			})
		}
	}
}

// BenchmarkSignatureEqual mirrors BenchmarkSignatureContains. Equal runs the
// same shape of loop over the same data, so a difference between the two is a
// difference in the comparison, not in the traversal.
func BenchmarkSignatureEqual(b *testing.B) {
	for _, bitWidth := range sigBenchWidths {
		big := benchSignature(bitWidth, 64)
		same := benchSignature(bitWidth, 64)
		last := len(big.words) - 1

		differs := func(word int) *Signature {
			other := benchSignature(bitWidth, 64)
			other.words[word] ^= 1 << (clearBit(b, big, word) % 64)
			return other
		}

		cases := []struct {
			name  string
			other *Signature
			want  bool
		}{
			{"equal", same, true},
			{"differ/first-word", differs(0), false},
			{"width-mismatch", NewSignature(bitWidth*2, "key-0000"), false},
		}
		if last > 0 {
			cases = append(cases, struct {
				name  string
				other *Signature
				want  bool
			}{"differ/last-word", differs(last), false})
		}

		for _, tc := range cases {
			if got := big.Equal(tc.other); got != tc.want {
				b.Fatalf("bits=%d/%s: Equal = %t, want %t", bitWidth, tc.name, got, tc.want)
			}
			b.Run(fmt.Sprintf("bits=%d/%s", bitWidth, tc.name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if big.Equal(tc.other) != tc.want {
						b.Fatal("wrong result")
					}
				}
			})
		}
	}
}

// BenchmarkSignatureSet times the per-key half of construction, with the
// allocation held outside the loop. The hashed variant is what NewSignature
// actually pays; the bare one isolates the mask and the store, and the
// difference between them is the hash.
func BenchmarkSignatureSet(b *testing.B) {
	keys := benchSigKeys(64)
	hashes := make([]uint64, len(keys))
	for i, k := range keys {
		hashes[i] = fnv.Hash64([]byte(k))
	}

	for _, bitWidth := range sigBenchWidths {
		b.Run(fmt.Sprintf("bits=%d/hash+set", bitWidth), func(b *testing.B) {
			sig := NewSignature(bitWidth)
			for i := 0; i < b.N; i++ {
				sig.set(fnv.Hash64([]byte(keys[i%len(keys)])))
			}
		})
		b.Run(fmt.Sprintf("bits=%d/set", bitWidth), func(b *testing.B) {
			sig := NewSignature(bitWidth)
			for i := 0; i < b.N; i++ {
				sig.set(hashes[i%len(hashes)])
			}
		})
	}
}

// BenchmarkSignatureVsFixed puts Signature against Fixed at the one width they
// share, which is the comparison behind the advice to prefer Fixed where the
// width is known and small. Signature pays an indirection through the slice
// header and a loop the compiler cannot unroll; Fixed pays neither.
func BenchmarkSignatureVsFixed(b *testing.B) {
	const bitWidth = FixedBits

	big := benchSignature(bitWidth, 64)
	sub := NewSignature(bitWidth, "key-0016", "key-0017")

	var fixedBig, fixedSub Fixed
	copy(fixedBig[:], big.words)
	copy(fixedSub[:], sub.words)

	b.Run("dynamic", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !big.Contains(sub) {
				b.Fatal("want contains")
			}
		}
	})
	b.Run("fixed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !fixedBig.Contains(fixedSub) {
				b.Fatal("want contains")
			}
		}
	})
}

// BenchmarkSignatureFalsePositives reports how often a signature of a given
// width still rejects a key it does not hold, once n keys have filled it. The
// timing is nearly flat here: a width of one word is a single word-op whatever
// the answer. What changes with n is the fp/op figure, which is the reason to
// pick one width over another, and which no timing alone would show.
//
// The probes rotate through keys the set does not hold, so each figure averages
// over the pool rather than reporting whether one chosen key happens to collide.
func BenchmarkSignatureFalsePositives(b *testing.B) {
	probes := make([]*Signature, 64)

	for _, bitWidth := range []uint{64, 256, 1024} {
		for i := range probes {
			probes[i] = NewSignature(bitWidth, fmt.Sprintf("absent-%04d", i))
		}
		for _, n := range []int{8, 32, 128, 512} {
			big := benchSignature(bitWidth, n)
			b.Run(fmt.Sprintf("bits=%d/n=%d", bitWidth, n), func(b *testing.B) {
				falsePositives := 0
				j := 0
				for i := 0; i < b.N; i++ {
					if big.Contains(probes[j]) {
						falsePositives++
					}
					if j++; j == len(probes) {
						j = 0
					}
				}
				b.ReportMetric(float64(falsePositives)/float64(b.N), "fp/op")
			})
		}
	}
}

// sigSink keeps the compiler from folding away a benchmarked constructor.
var sigSink *Signature
