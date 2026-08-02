package kv

import (
	"fmt"
	"slices"
	"testing"
)

func benchSet(n int) *KV {
	return New(func(yield func(k, v string) bool) {
		for i := 0; i < n; i++ {
			if !yield(fmt.Sprintf("key-%04d", i), fmt.Sprintf("value-%04d", i*7)) {
				return
			}
		}
	}, n)
}

func BenchmarkContains(b *testing.B) {
	big := benchSet(64)
	hit := newKV("key-0016", "value-0112", "key-0017", "value-0119")
	miss := newKV("key-1000", "value-0007", "key-1001", "value-0014")
	if !big.Contains(hit) || big.Contains(miss) {
		b.Fatal("benchmark inputs are wrong")
	}

	b.Run("hit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !big.Contains(hit) {
				b.Fatal("want contains")
			}
		}
	})
	b.Run("miss", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if big.Contains(miss) {
				b.Fatal("want !contains")
			}
		}
	})
}

// benchSizes are the element counts used by the pairwise benchmarks.
var benchSizes = []struct {
	name string
	n    int
}{
	{"1k", 1_000},
	{"10k", 10_000},
	{"100k", 100_000},
	{"1M", 1_000_000},
}

// BenchmarkContainsPairwise times Contains for every pair of sizes. benchSet(m)
// is a prefix of benchSet(n) for m <= n, so those calls are genuine hits that
// run the merge to completion; the calls where other is the larger set are
// rejected on the length check and measure the early exit.
//
// At these sizes both signatures are saturated on both sides, so they never
// reject: this is the case where the merge has to do the work.
func BenchmarkContainsPairwise(b *testing.B) {
	sets := make(map[int]*KV, len(benchSizes))
	for _, s := range benchSizes {
		sets[s.n] = benchSet(s.n)
	}

	for _, s := range benchSizes {
		for _, o := range benchSizes {
			big, other := sets[s.n], sets[o.n]
			want := o.n <= s.n
			if got := big.Contains(other); got != want {
				b.Fatalf("%s.Contains(%s) = %t, want %t", s.name, o.name, got, want)
			}
			b.Run(fmt.Sprintf("s=%s/other=%s", s.name, o.name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if big.Contains(other) != want {
						b.Fatal("wrong result")
					}
				}
			})
		}
	}
}

func BenchmarkContainsKeys(b *testing.B) {
	big := benchSet(64)
	b.Run("hit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !big.ContainsKeys("key-0033") {
				b.Fatal("want contains")
			}
		}
	})
	b.Run("miss", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if big.ContainsKeys("key-1000") {
				b.Fatal("want !contains")
			}
		}
	})
	// Several keys at once: all present, and the same list with one miss
	// appended, which pays for the whole scan before rejecting.
	hits := []string{"key-0000", "key-0011", "key-0033", "key-0063"}
	misses := append(slices.Clone(hits), "key-1000")
	b.Run("multi-hit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !big.ContainsKeys(hits...) {
				b.Fatal("want contains")
			}
		}
	})
	b.Run("multi-miss", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if big.ContainsKeys(misses...) {
				b.Fatal("want !contains")
			}
		}
	})
}

// BenchmarkContainsKeysSaturation measures the signature gate as the set fills
// it. A KV carries 64*sigLen bits, so a set of n keys leaves about
// (1-1/(64*sigLen))^n of them clear, and only a probe landing on a clear bit is
// rejected without a search. The gate therefore decays as n grows, and the cost
// of a missing key rises from a bit test towards the binary search it was meant
// to save.
//
// The probes rotate through keys the set does not hold, so each figure averages
// over both outcomes. Timing one fixed key instead would measure only whether
// that key happens to collide, which is a property of the key and not of the
// signature.
func BenchmarkContainsKeysSaturation(b *testing.B) {
	probes := make([]string, 64)
	for i := range probes {
		probes[i] = fmt.Sprintf("absent-%04d", i)
	}

	for _, n := range []int{8, 16, 32, 64, 128, 256, 1024} {
		set := benchSet(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			j := 0
			for i := 0; i < b.N; i++ {
				if set.ContainsKeys(probes[j]) {
					b.Fatal("want !contains")
				}
				if j++; j == len(probes) {
					j = 0
				}
			}
		})
	}
}
