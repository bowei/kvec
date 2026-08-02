package pred

import (
	"maps"
	"slices"

	"github.com/bowei/kvec/pkg/kv"
)

// NewPredicateBuilder creates an empty PredicateBuilder. Terms are added with
// the builder methods, each of which returns the receiver so calls can be
// chained, and Build compiles them into the Predicate that does the matching:
//
//	p := NewPredicateBuilder().
//		Key("app").
//		NoKey("deprecated").
//		Has(map[string]string{"tier": "backend"}).
//		ValueIn("env", []string{"prod", "staging"}).
//		Build()
func NewPredicateBuilder() *PredicateBuilder {
	return &PredicateBuilder{}
}

// PredicateBuilder accumulates the terms of a Predicate. It keeps them in the
// form they were added and does no work beyond copying what the caller passed;
// all of the analysis happens once, in Build.
type PredicateBuilder struct {
	keyExists   []string
	keyNExists  []string
	inValues    []multiValue
	notInValues []multiValue

	eq map[string]string
}

type multiValue struct {
	key    string
	values []string
}

// Key requires that each of keys is present, regardless of its value.
func (b *PredicateBuilder) Key(keys ...string) *PredicateBuilder {
	b.keyExists = append(b.keyExists, keys...)
	return b
}

// NoKey requires that none of keys is present.
func (b *PredicateBuilder) NoKey(keys ...string) *PredicateBuilder {
	b.keyNExists = append(b.keyNExists, keys...)
	return b
}

// Has requires that every key in kv is present with the given value. A key
// added by an earlier call is overwritten by a later one.
func (b *PredicateBuilder) Has(kv map[string]string) *PredicateBuilder {
	if len(kv) == 0 {
		return b
	}
	if b.eq == nil {
		b.eq = make(map[string]string, len(kv))
	}
	maps.Copy(b.eq, kv)
	return b
}

// ValueIn requires that key is present with a value in values. An empty values
// makes the predicate unsatisfiable.
func (b *PredicateBuilder) ValueIn(key string, values []string) *PredicateBuilder {
	b.inValues = append(b.inValues, multiValue{key: key, values: slices.Clone(values)})
	return b
}

// ValueNotIn requires that key is absent, or present with a value not in
// values.
func (b *PredicateBuilder) ValueNotIn(key string, values []string) *PredicateBuilder {
	b.notInValues = append(b.notInValues, multiValue{key: key, values: slices.Clone(values)})
	return b
}

// Predicate is a conjunction of terms: a match requires every term to hold. It
// is the compiled form of a PredicateBuilder, produced by Build, and is
// immutable; the same Predicate can be matched against many KVs, from many
// goroutines.
type Predicate struct {
	// unsatisfiable records that the terms contradict each other, so no KV can
	// match and the rest of the struct is unset.
	unsatisfiable bool

	// eq holds every pinned key/value pair as a KV, so that the whole set is
	// tested by one Contains rather than a lookup per pair. Nil if there are
	// none.
	eq *kv.KV

	// keyExists and keyNExists hold no key whose presence some other term
	// already settles.
	keyExists  sortedset
	keyNExists sortedset

	// in and notIn hold at most one entry per key, sorted by key.
	in    []valueSet
	notIn []valueSet
}

// valueSet is a key tested against a set of values.
type valueSet struct {
	key    string
	values sortedset
}

// Build compiles the accumulated terms into a Predicate. The result is
// independent of the builder: further calls on the builder do not change an
// already built Predicate, and Build may be called more than once.
//
// Build does the work that would otherwise be repeated on every Match:
//
//   - Repeated terms on one key are merged. ValueIn intersects, ValueNotIn
//     unions, so a key costs one lookup however many times it was named.
//   - Terms that settle a key's value make the weaker terms on that key
//     redundant, and the redundant ones are dropped. An equality subsumes both
//     value tests and a Key; an in-set absorbs an exclusion on the same key by
//     subtracting it, and proves the key present; a NoKey satisfies an
//     exclusion vacuously.
//   - An in-set left with one value becomes an equality, which moves it behind
//     the signature test that Contains applies to the whole eq set.
//   - Contradictions are found once and recorded, so Match returns on a single
//     bool.
//   - Value sets are sorted and deduplicated, which both shrinks them and lets
//     contains binary search the large ones.
func (b *PredicateBuilder) Build() *Predicate {
	// One in-set per key: a value must satisfy every ValueIn named on it, so
	// the sets intersect.
	in := make(map[string]sortedset, len(b.inValues))
	for _, mv := range b.inValues {
		values := newSortedSet(mv.values)
		if prev, ok := in[mv.key]; ok {
			values = prev.intersect(values)
		}
		in[mv.key] = values
	}
	// One notIn-set per key: any listed value disqualifies, so the sets union.
	notIn := make(map[string]sortedset, len(b.notInValues))
	for _, mv := range b.notInValues {
		values := newSortedSet(mv.values)
		if prev, ok := notIn[mv.key]; ok {
			values = prev.union(values)
		}
		notIn[mv.key] = values
	}

	eq := maps.Clone(b.eq)

	// An equality pins the value, which decides every other value term on that
	// key at build time.
	for key, value := range eq {
		if values, ok := in[key]; ok {
			if !slices.Contains(values, value) {
				return unsatisfiable()
			}
			delete(in, key)
		}
		if values, ok := notIn[key]; ok {
			if slices.Contains(values, value) {
				return unsatisfiable()
			}
			delete(notIn, key)
		}
	}

	// A key with an in-set must take one of its values, so an exclusion on the
	// same key is nothing more than a smaller in-set.
	for key, values := range in {
		if excluded, ok := notIn[key]; ok {
			values = values.subtract(excluded)
			in[key] = values
			delete(notIn, key)
		}
		if len(values) == 0 {
			return unsatisfiable()
		}
	}

	// A one-value in-set is an equality. Restating it as one lets the eq
	// signature test reject on it.
	for key, values := range in {
		if len(values) == 1 {
			if eq == nil {
				eq = make(map[string]string, 1)
			}
			eq[key] = values[0]
			delete(in, key)
		}
	}

	keyNExists := newSortedSet(b.keyNExists)
	keyExists := newSortedSet(b.keyExists)
	for _, key := range keyNExists {
		// Every one of these requires the key to be present.
		if _, ok := eq[key]; ok {
			return unsatisfiable()
		}
		if _, ok := in[key]; ok {
			return unsatisfiable()
		}
		if keyExists.contains(key) {
			return unsatisfiable()
		}
		// An absent key satisfies an exclusion vacuously.
		delete(notIn, key)
	}

	// eq and the in-sets already prove their key is present, so a Key on the
	// same key is a lookup Match would repeat for nothing.
	keyExists = slices.DeleteFunc(keyExists, func(key string) bool {
		if _, ok := eq[key]; ok {
			return true
		}
		_, ok := in[key]
		return ok
	})

	p := &Predicate{
		keyExists:  keyExists,
		keyNExists: keyNExists,
		in:         valueSets(in),
		notIn:      valueSets(notIn),
	}
	if len(eq) > 0 {
		p.eq = kv.New(func(yield func(k, v string) bool) {
			for key, value := range eq {
				if !yield(key, value) {
					return
				}
			}
		}, len(eq))
	}
	return p
}

func unsatisfiable() *Predicate {
	return &Predicate{unsatisfiable: true}
}

// Match reports whether kv satisfies every term of p. A Predicate built from an
// empty builder matches everything. kv must be non-nil.
//
// Terms are checked cheapest-first. The eq set leads because Contains gates on
// two words of signature, which rejects the whole set without a lookup and is
// the only test here that can reject on a wrong value rather than a missing
// key. The existence tests follow, each rejecting on a signature bit alone in
// the common case. The value tests come last, as they always pay for a lookup.
func (p *Predicate) Match(kv *kv.KV) bool {
	if p.unsatisfiable {
		return false
	}
	if p.eq != nil && !kv.Contains(p.eq) {
		return false
	}
	if !kv.ContainsKeys(p.keyExists...) {
		return false
	}
	for _, key := range p.keyNExists {
		if kv.ContainsKeys(key) {
			return false
		}
	}
	for i := range p.in {
		got, ok := kv.Get(p.in[i].key)
		if !ok || !p.in[i].values.contains(got) {
			return false
		}
	}
	for i := range p.notIn {
		if got, ok := kv.Get(p.notIn[i].key); ok && p.notIn[i].values.contains(got) {
			return false
		}
	}
	return true
}

// valueSets flattens the per-key sets into a slice ordered by key, so that a
// Predicate does not depend on map iteration order.
func valueSets(m map[string]sortedset) []valueSet {
	if len(m) == 0 {
		return nil
	}
	out := make([]valueSet, 0, len(m))
	for _, key := range slices.Sorted(maps.Keys(m)) {
		out = append(out, valueSet{key: key, values: m[key]})
	}
	return out
}
