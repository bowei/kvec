package pred

import (
	"fmt"
	"maps"
	"testing"

	"github.com/bowei/kvec/pkg/kv"
)

// benchBuilder names one term of every kind a Predicate supports, each on a key
// that no other term settles, so that all of them survive Build and Match has
// to check every one.
func benchBuilder() *PredicateBuilder {
	return NewPredicateBuilder().
		Key("owner").
		NoKey("deprecated").
		Has(map[string]string{"app": "web", "tier": "frontend"}).
		ValueIn("env", []string{"prod", "staging"}).
		ValueNotIn("region", []string{"eu"})
}

// benchBuilderRedundant names the same number of terms, but two of them are
// settled by the equality on app: the Key is implied by it and the exclusion is
// decided by its value. Build drops both, so this compiles to three terms where
// benchBuilder compiles to five.
func benchBuilderRedundant() *PredicateBuilder {
	return NewPredicateBuilder().
		Key("app").
		NoKey("deprecated").
		Has(map[string]string{"app": "web", "tier": "frontend"}).
		ValueIn("env", []string{"prod", "staging"}).
		ValueNotIn("app", []string{"api"})
}

// TestBenchPredicates pins what the two builders compile to. The benchmarks
// below are only meaningful if every term of benchBuilder survives Build, and
// if the redundant one really is reduced; a change to the subsumption rules
// that broke either would otherwise leave the benchmarks quietly measuring
// fewer terms than they name.
func TestBenchPredicates(t *testing.T) {
	p := benchBuilder().Build()
	switch {
	case p.unsatisfiable:
		t.Errorf("benchBuilder is unsatisfiable")
	case p.eq == nil || p.eq.Len() != 2:
		t.Errorf("benchBuilder eq = %v, want 2 pairs", p.eq)
	case len(p.keyExists) != 1:
		t.Errorf("benchBuilder keyExists = %v, want 1 key", p.keyExists)
	case len(p.keyNExists) != 1:
		t.Errorf("benchBuilder keyNExists = %v, want 1 key", p.keyNExists)
	case len(p.in) != 1:
		t.Errorf("benchBuilder in = %v, want 1 key", p.in)
	case len(p.notIn) != 1:
		t.Errorf("benchBuilder notIn = %v, want 1 key", p.notIn)
	}

	r := benchBuilderRedundant().Build()
	switch {
	case r.unsatisfiable:
		t.Errorf("benchBuilderRedundant is unsatisfiable")
	case r.eq == nil || r.eq.Len() != 2:
		t.Errorf("benchBuilderRedundant eq = %v, want 2 pairs", r.eq)
	case len(r.keyExists) != 0:
		t.Errorf("benchBuilderRedundant keyExists = %v, want none", r.keyExists)
	case len(r.notIn) != 0:
		t.Errorf("benchBuilderRedundant notIn = %v, want none", r.notIn)
	}
}

// matchingPairs satisfies every term of benchBuilder.
var matchingPairs = map[string]string{
	"app":    "web",
	"tier":   "frontend",
	"env":    "prod",
	"owner":  "alice",
	"region": "us",
}

// benchKV builds a KV of n pairs holding the keys benchBuilder names, as
// mutate leaves them, with filler making up the rest.
func benchKV(n int, mutate func(map[string]string)) *kv.KV {
	pairs := maps.Clone(matchingPairs)
	if mutate != nil {
		mutate(pairs)
	}
	return kv.New(func(yield func(k, v string) bool) {
		for k, v := range pairs {
			if !yield(k, v) {
				return
			}
		}
		for i := range n - len(pairs) {
			if !yield(fmt.Sprintf("k%03d", i), fmt.Sprintf("v%03d", i)) {
				return
			}
		}
	}, n)
}

// matchCases are the points at which Match can reject, one per term of
// benchBuilder, listed in the order Match tries them. Each mutation breaks
// exactly one term, so a case measures the cost of reaching that term and
// failing there.
var matchCases = []struct {
	name   string
	mutate func(map[string]string)
}{
	{"eq-absent", func(m map[string]string) { delete(m, "app") }},
	{"eq-value", func(m map[string]string) { m["app"] = "nginx" }},
	{"keyExists", func(m map[string]string) { delete(m, "owner") }},
	{"keyNExists", func(m map[string]string) { m["deprecated"] = "true" }},
	{"in", func(m map[string]string) { m["env"] = "dev" }},
	{"notIn", func(m map[string]string) { m["region"] = "eu" }},
	{"accept", nil},
}

// BenchmarkMatch times Match at each point it can reject. Splitting the miss
// this way is the whole point of the benchmark: Match checks its terms
// cheapest-first, so a rejection on the equality set is a signature test and
// costs nothing like one on a value set, which has paid for every earlier term
// before it fails. A benchmark that reported one figure for "miss" would only
// be reporting whichever term its input happened to break.
func BenchmarkMatch(b *testing.B) {
	p := benchBuilder().Build()
	for _, size := range []int{8, 16, 64} {
		for _, tc := range matchCases {
			kv := benchKV(size, tc.mutate)
			want := tc.mutate == nil
			if got := p.Match(kv); got != want {
				b.Fatalf("%s/%d: Match = %t, want %t", tc.name, size, got, want)
			}
			b.Run(fmt.Sprintf("%s/%d", tc.name, size), func(b *testing.B) {
				for b.Loop() {
					if p.Match(kv) != want {
						b.Fatal("wrong result")
					}
				}
			})
		}
	}
}

// BenchmarkMatchUnsatisfiable is the floor: terms that contradict each other
// are resolved by Build, so Match returns on a single bool without reading the
// candidate at all.
func BenchmarkMatchUnsatisfiable(b *testing.B) {
	p := NewPredicateBuilder().Key("app").NoKey("app").Build()
	kv := benchKV(64, nil)
	for b.Loop() {
		if p.Match(kv) {
			b.Fatal("want no match")
		}
	}
}

// BenchmarkBuild times the compilation that Match is relieved of. The redundant
// builder is the same size but does more of its work at build time, as two of
// its terms are dropped rather than compiled.
func BenchmarkBuild(b *testing.B) {
	b.Run("all-terms", func(b *testing.B) {
		for b.Loop() {
			benchBuilder().Build()
		}
	})
	b.Run("redundant", func(b *testing.B) {
		for b.Loop() {
			benchBuilderRedundant().Build()
		}
	})
}

// matchEqByLookup is the alternative to the Contains that Match uses for the eq
// set: a lookup per pair, with no signature gate. It is what Predicate did
// before the set was compiled into a KV, and exists here only to keep the
// comparison below honest.
func matchEqByLookup(kv *kv.KV, eq map[string]string) bool {
	for k, v := range eq {
		if got, ok := kv.Get(k); !ok || got != v {
			return false
		}
	}
	return true
}

// BenchmarkEqStrategy is the measurement behind Match testing the eq set with
// Contains. Contains is far ahead on a miss, where the signature rejects the
// whole set in constant time however large the KV, and ahead on a hit too now
// that Contains searches rather than walks a long run.
func BenchmarkEqStrategy(b *testing.B) {
	eq := map[string]string{"tier": "frontend", "app": "web", "env": "prod"}
	p := NewPredicateBuilder().Has(eq).Build()

	for _, size := range []int{8, 16, 64} {
		hit := benchKV(size, nil)
		miss := benchKV(size, func(m map[string]string) { m["env"] = "dev" })
		for _, tc := range []struct {
			name string
			kv   *kv.KV
		}{{"hit", hit}, {"miss", miss}} {
			b.Run(fmt.Sprintf("contains/%s/%d", tc.name, size), func(b *testing.B) {
				for b.Loop() {
					tc.kv.Contains(p.eq)
				}
			})
			b.Run(fmt.Sprintf("lookup/%s/%d", tc.name, size), func(b *testing.B) {
				for b.Loop() {
					matchEqByLookup(tc.kv, eq)
				}
			})
		}
	}
}
