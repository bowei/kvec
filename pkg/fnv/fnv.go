// Package fnv is an allocation-free FNV-1a hash over 64 bits.
//
// The standard library's hash/fnv returns a hash.Hash64, which allocates an
// object per use. The functions here fold bytes into a plain uint64 instead, so
// hashing a key costs nothing beyond the loop. They are used on paths taken
// once per key at construction and once per probe at query time, where that
// allocation would dominate.
//
// A hash is built up by starting from OffsetBasis64 and folding data in with
// Add64 or AddWord64. Hash64 is the whole of that for a single byte slice.
package fnv

const (
	// OffsetBasis64 is the FNV-1a starting value, and so the hash of the empty
	// byte stream. Callers folding data in themselves start here.
	OffsetBasis64 uint64 = 14695981039346656037

	// prime64 is the FNV-1a multiplier.
	prime64 uint64 = 1099511628211
)

// Hash64 is the FNV-1a hash of data.
func Hash64(data []byte) uint64 {
	return Add64(OffsetBasis64, data)
}

// Add64 continues the hash over more data. Folding two slices in turn gives the
// same result as hashing their concatenation, so a caller hashing several
// fields must delimit them itself: see the length prefixes in kv.KV.Hash.
func Add64(hash uint64, data []byte) uint64 {
	for i := range data {
		hash ^= uint64(data[i])
		hash *= prime64
	}
	return hash
}

// AddWord64 continues the hash over the eight bytes of x, least significant
// byte first. Folding the whole word rather than mixing x in directly keeps the
// result a plain FNV-1a over a byte stream, which is what makes delimiting the
// fields around it mean anything.
func AddWord64(hash, x uint64) uint64 {
	for range 8 {
		hash ^= x & 0xff
		hash *= prime64
		x >>= 8
	}
	return hash
}
