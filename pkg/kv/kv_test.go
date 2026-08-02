package kv

import (
	"fmt"
	"maps"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

// newKV builds a FastKV from alternating key, value arguments.
func newKV(kvs ...string) *KV {
	return New(func(yield func(k, v string) bool) {
		for i := 0; i < len(kvs); i += 2 {
			if !yield(kvs[i], kvs[i+1]) {
				return
			}
		}
	}, len(kvs)/2)
}

// fromMap builds a FastKV from m. Map iteration order is unspecified, so this
// also exercises the constructor's independence from insertion order.
func fromMap(m map[string]string) *KV {
	return New(func(yield func(k, v string) bool) {
		for k, v := range m {
			if !yield(k, v) {
				return
			}
		}
	}, len(m))
}

// dump renders a FastKV as key=value pairs in stored order, for failure
// messages.
func dump(s *KV) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, e := range s.kvs {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%s=%s", e.k, e.v)
	}
	b.WriteByte('}')
	return b.String()
}

func TestNew(t *testing.T) {
	s := newKV("c", "3", "a", "1", "b", "2")
	if got, want := s.Len(), 3; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	if !slices.IsSortedFunc(s.kvs, kvCmp) {
		t.Errorf("kvs = %v, want sorted by kvCmp", s.kvs)
	}
	if (s.ksig == fixedSignature{}) {
		t.Errorf("ksig = 0, want non-zero")
	}
	if (s.kvsig == fixedSignature{}) {
		t.Errorf("kvsig = 0, want non-zero")
	}
}

func TestNewIgnoresOrderAndCap(t *testing.T) {
	want := newKV("a", "1", "b", "2", "c", "3")
	kvs := []string{"c", "3", "a", "1", "b", "2"}
	// The cap argument is only a preallocation hint: too small, exact and too
	// large must all produce the same set.
	for _, cap := range []int{0, 1, 3, 64} {
		got := New(func(yield func(k, v string) bool) {
			for i := 0; i < len(kvs); i += 2 {
				if !yield(kvs[i], kvs[i+1]) {
					return
				}
			}
		}, cap)
		if !got.Equal(want) {
			t.Errorf("cap %d: NewFastKV(...) = %s, want %s", cap, dump(got), dump(want))
		}
	}
}

func TestEmpty(t *testing.T) {
	var zero KV
	empty := newKV()
	for _, s := range []*KV{&zero, empty} {
		if got := s.Len(); got != 0 {
			t.Errorf("%s.Len() = %d, want 0", dump(s), got)
		}
		if (s.ksig != fixedSignature{}) || (s.kvsig != fixedSignature{}) {
			t.Errorf("%s has non-zero signatures %#x %#x", dump(s), s.ksig, s.kvsig)
		}
		if !s.Contains(&zero) || !s.Contains(empty) {
			t.Errorf("%s does not contain the empty set", dump(s))
		}
		if s.Contains(newKV("a", "1")) {
			t.Errorf("%s contains a non-empty set", dump(s))
		}
		if !s.Equal(&zero) || !s.Equal(empty) {
			t.Errorf("%s is not equal to the empty set", dump(s))
		}
		if s.ContainsKeys("a") || s.ContainsKeys("") || s.ContainsKeys("a", "") {
			t.Errorf("%s contains a key", dump(s))
		}
		if !s.ContainsKeys() {
			t.Errorf("%s does not contain the empty key list", dump(s))
		}
		// Every non-empty set contains the empty one.
		if !newKV("a", "1").Contains(s) {
			t.Errorf("{a=1} does not contain %s", dump(s))
		}
	}
}

func TestEqual(t *testing.T) {
	a := newKV("a", "1", "b", "2")
	for _, tc := range []struct {
		name string
		b    *KV
		want bool
	}{
		{"same order", newKV("a", "1", "b", "2"), true},
		{"other order", newKV("b", "2", "a", "1"), true},
		{"different value", newKV("a", "1", "b", "3"), false},
		{"different key", newKV("a", "1", "c", "2"), false},
		{"swapped values", newKV("a", "2", "b", "1"), false},
		{"key and value swapped", newKV("1", "a", "2", "b"), false},
		{"subset", newKV("a", "1"), false},
		{"superset", newKV("a", "1", "b", "2", "c", "3"), false},
		{"empty", newKV(), false},
	} {
		if got := a.Equal(tc.b); got != tc.want {
			t.Errorf("%s: %s.Equal(%s) = %t, want %t", tc.name, dump(a), dump(tc.b), got, tc.want)
		}
		// Equal is symmetric.
		if got := tc.b.Equal(a); got != tc.want {
			t.Errorf("%s: %s.Equal(%s) = %t, want %t", tc.name, dump(tc.b), dump(a), got, tc.want)
		}
	}
}

func TestContains(t *testing.T) {
	a := newKV("a", "1", "b", "2", "c", "3")
	for _, tc := range []struct {
		name string
		b    *KV
		want bool
	}{
		{"empty", newKV(), true},
		{"self", a, true},
		{"one", newKV("b", "2"), true},
		{"two", newKV("a", "1", "c", "3"), true},
		{"reordered subset", newKV("c", "3", "a", "1"), true},
		{"missing key", newKV("d", "4"), false},
		{"missing key among present ones", newKV("a", "1", "d", "4", "c", "3"), false},
		{"wrong value", newKV("b", "9"), false},
		{"right keys, one wrong value", newKV("a", "1", "b", "9"), false},
		{"superset", newKV("a", "1", "b", "2", "c", "3", "d", "4"), false},
		{"empty key", newKV("", ""), false},
	} {
		if got := a.Contains(tc.b); got != tc.want {
			t.Errorf("%s: %s.Contains(%s) = %t, want %t", tc.name, dump(a), dump(tc.b), got, tc.want)
		}
	}
}

func TestContainsKeys(t *testing.T) {
	s := newKV("a", "1", "b", "2", "carrot", "3", "", "empty")
	for _, tc := range []struct {
		keys []string
		want bool
	}{
		{nil, true},
		{[]string{"a"}, true},
		{[]string{"b"}, true},
		{[]string{"carrot"}, true},
		{[]string{""}, true},
		{[]string{"c"}, false},
		{[]string{"car"}, false},
		{[]string{"carrots"}, false},
		{[]string{"A"}, false},
		{[]string{"1"}, false},
		{[]string{"z"}, false},
		// Only all-present lists hold.
		{[]string{"a", "b"}, true},
		{[]string{"", "a", "b", "carrot"}, true},
		{[]string{"carrot", "a"}, true},
		{[]string{"a", "a", "a"}, true},
		{[]string{"a", "z"}, false},
		{[]string{"z", "a"}, false},
		{[]string{"", "a", "b", "carrot", "d"}, false},
		{[]string{"y", "z"}, false},
	} {
		if got := s.ContainsKeys(tc.keys...); got != tc.want {
			t.Errorf("%s.ContainsKeys(%q) = %t, want %t", dump(s), tc.keys, got, tc.want)
		}
	}
}

// containsSlow is the obvious reference implementation.
func containsSlow(a, b map[string]string) bool {
	for k, v := range b {
		if av, ok := a[k]; !ok || av != v {
			return false
		}
	}
	return true
}

// randMaps returns a generator of small random key-value maps drawn from a
// deliberately narrow pool, so that subsets and near-misses come up often.
func randMaps(seed int64) func() map[string]string {
	rng := rand.New(rand.NewSource(seed))
	keys := []string{"", "a", "b", "c", "ab", "ba", "key", "kex", "a-longer-key", "a-longer-kez"}
	vals := []string{"", "0", "1", "value"}
	return func() map[string]string {
		m := map[string]string{}
		for _, k := range keys {
			if rng.Intn(3) == 0 {
				m[k] = vals[rng.Intn(len(vals))]
			}
		}
		return m
	}
}

func TestContainsRandom(t *testing.T) {
	next := randMaps(1)
	for i := 0; i < 10000; i++ {
		am, bm := next(), next()
		a, b := fromMap(am), fromMap(bm)
		if got, want := a.Contains(b), containsSlow(am, bm); got != want {
			t.Fatalf("%v.Contains(%v) = %t, want %t", am, bm, got, want)
		}
		// A set always contains itself, however it was built.
		if !a.Contains(fromMap(am)) {
			t.Fatalf("%v does not contain itself", am)
		}
	}
}

// TestContainsRandomLopsided covers the search Contains falls back to once it
// has skipped gallopAfter pairs in a row. randMaps draws from too small a pool
// to produce a run that long, so the sets here are wide, and the subset is
// drawn thinly enough to leave long stretches of s between its pairs.
func TestContainsRandomLopsided(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	// Long enough to hold runs many times gallopAfter.
	keys := make([]string, 200)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%03d", i)
	}
	vals := []string{"", "0", "1", "value"}

	for i := 0; i < 2000; i++ {
		// s is most of the pool; other is a sparse sample of it, so the pairs
		// other holds sit far apart in s.
		sm := map[string]string{}
		for _, k := range keys {
			if rng.Intn(8) != 0 {
				sm[k] = vals[rng.Intn(len(vals))]
			}
		}
		om := map[string]string{}
		for _, k := range keys {
			if rng.Intn(20) == 0 {
				// Usually copy the value across, so that most cases turn on a
				// missing key rather than a mismatched value.
				v := vals[rng.Intn(len(vals))]
				if sv, ok := sm[k]; ok && rng.Intn(4) != 0 {
					v = sv
				}
				om[k] = v
			}
		}

		s, other := fromMap(sm), fromMap(om)
		if got, want := s.Contains(other), containsSlow(sm, om); got != want {
			t.Fatalf("Contains() = %t, want %t\ns     = %s\nother = %s", got, want, dump(s), dump(other))
		}
		if !s.Contains(fromMap(sm)) {
			t.Fatalf("%s does not contain itself", dump(s))
		}
	}
}

func TestEqualRandom(t *testing.T) {
	next := randMaps(2)
	for i := 0; i < 10000; i++ {
		am, bm := next(), next()
		a, b := fromMap(am), fromMap(bm)
		want := containsSlow(am, bm) && containsSlow(bm, am)
		if got := a.Equal(b); got != want {
			t.Fatalf("%v.Equal(%v) = %t, want %t", am, bm, got, want)
		}
	}
}

func TestContainsKeysRandom(t *testing.T) {
	next := randMaps(3)
	probes := []string{"", "a", "b", "c", "ab", "ba", "key", "kex", "a-longer-key", "a-longer-kez", "zzz", "A", "0"}
	for i := 0; i < 10000; i++ {
		m := next()
		s := fromMap(m)
		for _, k := range probes {
			_, want := m[k]
			if got := s.ContainsKeys(k); got != want {
				t.Fatalf("%v.ContainsKeys(%q) = %t, want %t", m, k, got, want)
			}
			// Pairs hold only when both keys do.
			for _, k2 := range probes {
				_, want2 := m[k2]
				want := want && want2
				if got := s.ContainsKeys(k, k2); got != want {
					t.Fatalf("%v.ContainsKeys(%q, %q) = %t, want %t", m, k, k2, got, want)
				}
			}
		}
		// The whole key set is present, and stays so under duplication.
		keys := slices.Collect(maps.Keys(m))
		if !s.ContainsKeys(keys...) {
			t.Fatalf("%v.ContainsKeys(%q) = false, want true", m, keys)
		}
		if !s.ContainsKeys(append(slices.Clone(keys), keys...)...) {
			t.Fatalf("%v.ContainsKeys(%q twice over) = false, want true", m, keys)
		}
	}
}

func TestSignatureIsOrderIndependent(t *testing.T) {
	a := newKV("a", "1", "b", "2", "c", "3")
	b := newKV("c", "3", "b", "2", "a", "1")
	if a.ksig != b.ksig {
		t.Errorf("ksig = %#x and %#x, want equal", a.ksig, b.ksig)
	}
	if a.kvsig != b.kvsig {
		t.Errorf("kvsig = %#x and %#x, want equal", a.kvsig, b.kvsig)
	}
}

// TestSignatureCollision covers the case the signature cannot decide: the
// values "v7" and "v30" have FNV hashes that agree in the low bits kvbit keeps,
// so these two sets have identical ksig and kvsig despite differing. Equal and
// Contains must fall through to comparing the values themselves.
func TestSignatureCollision(t *testing.T) {
	a := newKV("k", "v7")
	b := newKV("k", "v30")

	if a.ksig != b.ksig || a.kvsig != b.kvsig {
		t.Fatalf("signatures %#x/%#x and %#x/%#x differ, want equal: the collision no longer holds, pick new values",
			a.ksig, a.kvsig, b.ksig, b.kvsig)
	}
	if a.Equal(b) || b.Equal(a) {
		t.Errorf("%s.Equal(%s) = true, want false: the values differ", dump(a), dump(b))
	}
	if a.Contains(b) || b.Contains(a) {
		t.Errorf("%s and %s Contain each other, want not: the values differ", dump(a), dump(b))
	}
	// The key is shared, so ContainsKeys agrees on both.
	if !a.ContainsKeys("k") || !b.ContainsKeys("k") {
		t.Errorf("%s or %s does not contain key %q", dump(a), dump(b), "k")
	}
}

// forced builds a FastKV from entries whose khash the caller chooses, mirroring
// NewFastKV. It exists to fabricate hash collisions, which cannot be produced
// by hashing real keys.
func forced(entries ...kv) *KV {
	var s KV
	s.kvs = slices.Clone(entries)
	slices.SortFunc(s.kvs, kvCmp)
	for _, e := range s.kvs {
		s.ksig.set(e.kbit())
		s.kvsig.set(e.kvbit())
	}
	return &s
}

// TestHashCollision covers distinct keys that share a khash. FNV-64 collisions
// are not findable by hand, so the entries are synthesized. The values are
// equal on purpose: that makes both signatures identical too, so nothing can
// tell the keys apart except the tie-break in kvCmp.
//
// ContainsKeys is absent here because it hashes the keys it is given, so it
// cannot be reached with a fabricated khash.
func TestHashCollision(t *testing.T) {
	const h = 0x0123456789abcdef
	a := forced(kv{khash: h, k: "a", v: "1"})
	b := forced(kv{khash: h, k: "b", v: "1"})
	both := forced(kv{khash: h, k: "a", v: "1"}, kv{khash: h, k: "b", v: "1"})

	if a.ksig != b.ksig || a.kvsig != b.kvsig {
		t.Fatalf("signatures %#x/%#x and %#x/%#x differ, want equal: the test is not exercising the tie-break",
			a.ksig, a.kvsig, b.ksig, b.kvsig)
	}
	if !slices.IsSortedFunc(both.kvs, kvCmp) {
		t.Errorf("kvs = %v, want sorted by kvCmp", both.kvs)
	}
	if a.Contains(b) || b.Contains(a) {
		t.Errorf("%s and %s Contain each other, want not: the keys differ", dump(a), dump(b))
	}
	if a.Equal(b) {
		t.Errorf("%s.Equal(%s) = true, want false: the keys differ", dump(a), dump(b))
	}
	if !both.Contains(a) {
		t.Errorf("%s does not contain %s", dump(both), dump(a))
	}
	if !both.Contains(b) {
		t.Errorf("%s does not contain %s", dump(both), dump(b))
	}
	if !both.Contains(both) {
		t.Errorf("%s does not contain itself", dump(both))
	}
	if a.Contains(both) {
		t.Errorf("%s contains the larger %s", dump(a), dump(both))
	}
	// Same colliding keys, one wrong value.
	wrong := forced(kv{khash: h, k: "b", v: "9"})
	if both.Contains(wrong) {
		t.Errorf("%s contains %s, want not: the value differs", dump(both), dump(wrong))
	}
}

func TestKVCmpIsATotalOrder(t *testing.T) {
	// Equal hashes must not compare equal unless the keys match, otherwise the
	// sort order is unspecified and the merge in Contains cannot rely on it.
	const h = 0x0123456789abcdef
	x := kv{khash: h, k: "a", v: "1"}
	y := kv{khash: h, k: "b", v: "1"}
	if got := kvCmp(x, y); got == 0 {
		t.Errorf("kvCmp(%v, %v) = 0, want non-zero", x, y)
	}
	if kvCmp(x, y) != -kvCmp(y, x) {
		t.Errorf("kvCmp(%v, %v) = %d and kvCmp(%v, %v) = %d, want opposites", x, y, kvCmp(x, y), y, x, kvCmp(y, x))
	}
	// Same key and hash: the value is not part of the ordering.
	z := kv{khash: h, k: "a", v: "2"}
	if got := kvCmp(x, z); got != 0 {
		t.Errorf("kvCmp(%v, %v) = %d, want 0", x, z, got)
	}
}
