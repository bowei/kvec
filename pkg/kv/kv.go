package kv

import (
	"slices"
	"strings"
)

// New creates a new KV from an iterator over the keys and values.
//
//   - Keys are assumed to be unique, constructor does not dedupe.
//   - cap is an optional capacity hint to avoid slice reallocations. This should
//     be the number of key,value pairs.
func New(itr func(yield func(k, v string) bool), cap int) *KV {
	var fkv KV

	fkv.kvs = make([]kv, 0, cap)
	for k, v := range itr {
		fkv.kvs = append(fkv.kvs, makeKV(k, v))
	}
	slices.SortFunc(fkv.kvs, kvCmp)

	for _, e := range fkv.kvs {
		fkv.ksig.set(e.kbit())
		fkv.kvsig.set(e.kvbit())
	}
	return &fkv
}

// KV is an optimized key/value structure for Contains and ContainsKeys
// operations.
type KV struct {
	ksig  fixedSignature
	kvsig fixedSignature

	// kvs are sorted by hash order.
	kvs []kv
}

// Len is the number of key/value pairs.
func (s *KV) Len() int { return len(s.kvs) }

func (s *KV) Equal(other *KV) bool {
	if !s.kvsig.equal(other.kvsig) {
		return false
	}
	if len(s.kvs) != len(other.kvs) {
		return false
	}
	for i := range s.kvs {
		if s.kvs[i].khash != other.kvs[i].khash {
			return false
		}
		if s.kvs[i].k != other.kvs[i].k {
			return false
		}
		if s.kvs[i].v != other.kvs[i].v {
			return false
		}
	}
	return true
}

// Contains returns whether every key-value pair in other is also in s.
func (s *KV) Contains(other *KV) bool {
	// Bits of other's signatures that are missing from s's. Any such bit proves
	// that other holds a pair (or a key) that s cannot hold, so other can be
	// rejected without looking at any element. The converse does not hold: the
	// signatures are approximations, so passing proves nothing.
	if !s.kvsig.contains(other.kvsig) {
		return false
	}
	// Not implied by the kvsig check above: kbit and kvbit fold different
	// hashes into the same width, so a key other holds and s lacks can collide
	// in one and not the other. This gate only sees that case (a shared key
	// with a different value leaves ksig alone), but it is a second independent
	// chance to reject it, and both signatures sit in the same cache line as
	// the one already read.
	if !s.ksig.contains(other.ksig) {
		return false
	}
	if len(other.kvs) > len(s.kvs) {
		return false
	}

	// Both sides are sorted by kvCmp, so a single merge pass suffices.
	i, j := 0, 0
	// skipped counts the pairs of s passed over in a row for the pair of other
	// currently being placed. A short run is cheaper to walk than to search, so
	// the merge runs as it stands until a run proves s is much the larger side,
	// at which point searching takes over; that is the case where the walk
	// would otherwise cost a comparison per pair of s.
	skipped := 0
	for j < len(other.kvs) {
		if len(other.kvs)-j > len(s.kvs)-i {
			// More pairs remain in other than in s.
			return false
		}
		switch c := kvCmp(s.kvs[i], other.kvs[j]); {
		case c < 0:
			// s.kvs[i] sorts first, so it is a key other does not have.
			i++
			skipped++
			if skipped == gallopAfter {
				// The run is long enough that searching out the rest of it
				// beats walking it. This is checked here rather than at the top
				// of the loop so that a merge of two similar sets, which never
				// builds a run this long, does not pay for the test at all.
				off, found := slices.BinarySearchFunc(s.kvs[i:], other.kvs[j], kvCmp)
				if !found {
					// other.kvs[j] holds a key that does not appear in s.
					return false
				}
				// s.kvs[i] now matches, and the next pass makes the comparison
				// again to land on the value check below.
				i += off
				skipped = 0
			}
			continue
		case c > 0:
			// other.kvs[j] holds a key that does not appear in s.
			return false
		}
		if s.kvs[i].v != other.kvs[j].v {
			// Same key, different value.
			return false
		}
		i++
		j++
		skipped = 0
	}
	return true
}

// gallopAfter is the run of skipped pairs at which Contains stops walking and
// searches instead. It is the point where the walk has already cost about as
// much as the log2 comparisons a search over the remainder would, so the two
// strategies cost the same in the worst case and the search wins outright on
// anything longer.
const gallopAfter = 4

// ContainsKeys returns whether s holds a pair for every one of the given keys.
// No keys is vacuously true. Duplicate keys are allowed and are checked more
// than once.
func (s *KV) ContainsKeys(keys ...string) bool {
	for _, key := range keys {
		probe := makeKV(key, "")

		// The key's bit is missing from the signature, so no pair in s has it.
		var probeSig fixedSignature
		probeSig.set(probe.kbit())
		if !s.ksig.contains(probeSig) {
			return false
		}

		if _, found := slices.BinarySearchFunc(s.kvs, probe, kvCmp); !found {
			return false
		}
	}
	return true
}

// Get returns the value of the pair with the given key, and whether such a
// pair exists.
func (s *KV) Get(key string) (string, bool) {
	probe := makeKV(key, "")

	// The key's bit is missing from the signature, so no pair in s has it.
	var probeSig fixedSignature
	probeSig.set(probe.kbit())
	if !s.ksig.contains(probeSig) {
		return "", false
	}

	i, found := slices.BinarySearchFunc(s.kvs, probe, kvCmp)
	if !found {
		return "", false
	}
	return s.kvs[i].v, true
}

type kv struct {
	khash uint64

	k string
	v string
}

func makeKV(k, v string) kv {
	return kv{
		khash: fnv1a64([]byte(k)),
		k:     k,
		v:     v,
	}
}

func kvCmp(a, b kv) int {
	switch {
	case a.khash < b.khash:
		return 1
	case a.khash > b.khash:
		return -1
	default:
		// Distinct keys can share a hash. Break the tie on the key itself so
		// that the ordering is total, which Equal, Contains and ContainsKeys all
		// rely on.
		return strings.Compare(b.k, a.k)
	}
}

// bitMask compresses the hash to a bit offset that fits in the signature.
// sigLen must be a power of two for this to cover the signature exactly.
const bitMask uint64 = 64*sigLen - 1

func (x kv) kbit() uint {
	return uint(x.khash & bitMask)
}

func (x kv) kvbit() uint {
	hash := x.khash
	hash ^= fnv1a64([]byte(x.v))
	hash &= bitMask
	return uint(hash)
}

const (
	fnvOffsetBasis64 = 14695981039346656037
	fnvPrime64       = 1099511628211
)

// fnv1a64 inline avoids allocation of a hash object.
func fnv1a64(data []byte) uint64 {
	hash := uint64(fnvOffsetBasis64)
	for i := range data {
		hash ^= uint64(data[i])
		hash *= fnvPrime64
	}
	return hash
}
