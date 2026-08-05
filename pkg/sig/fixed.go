package sig

// The fixed vectors put 64*fixedNWords bits for the signature. Assuming uniform
// hash, we should expect (small) sets of keys and values will map to differing
// offsets, meaning that we only have to do an O(1) bit-op to check
// Contains/Equal.
//
// The width is in the type name, so a caller picks one by how many key/values
// it expects to fold onto it. Fixed128 handles sets of << 1k key/values and is
// the usual choice; Fixed64 halves the storage where the sets are smaller
// still, and Fixed256 buys headroom where they are larger.
const (
	fixed64Words  = 1
	fixed128Words = 2
	fixed256Words = 4
)

// Fixed64Bits, Fixed128Bits and Fixed256Bits are the number of bits addressed
// by the corresponding vector. Each is a power of two, so a caller folding a
// hash onto a bit offset can mask with FixedNBits-1 rather than divide.
const (
	Fixed64Bits  = 64 * fixed64Words
	Fixed128Bits = 64 * fixed128Words
	Fixed256Bits = 64 * fixed256Words
)

// Fixed64 is a fixed length bit-vector of Fixed64Bits bits. See Fixed128 for
// what the fixed vectors are for; this is the narrowest of them, a single word.
//
// The zero Fixed64 is an empty signature, which every other Fixed64 contains.
type Fixed64 [fixed64Words]uint64

// Equal returns whether the two signatures have the same bits set. Equal sets
// have equal signatures, but not the reverse.
func (s *Fixed64) Equal(other Fixed64) bool {
	return s[0] == other[0]
}

// Contains returns whether every bit set in other is also set in s. A missing
// bit proves that other holds an element s cannot hold; the converse says
// nothing, as distinct elements can fold onto the same bit.
func (s *Fixed64) Contains(other Fixed64) bool {
	return other[0]&^s[0] == 0
}

// Set the bit at the given offset in the signature. Offsets at or above
// Fixed64Bits are silently dropped, so callers fold their hashes with
// Fixed64Bits-1.
func (s *Fixed64) Set(bit uint) {
	// A single word, so there is no word to select: a shift of 64 or more is
	// zero in Go, which is the silent drop.
	s[0] |= 1 << bit
}

// Fixed128 is a fixed length bit-vector of Fixed128Bits bits.
//
// This is the compile-time sized counterpart of Signature: the width is a
// constant, so the loops below are unrolled and the vector sits inline in the
// caller's struct. Prefer a fixed vector where the width is known and small,
// and Signature where it has to be chosen per instance.
//
// The zero Fixed128 is an empty signature, which every other Fixed128 contains.
type Fixed128 [fixed128Words]uint64

// Equal returns whether the two signatures have the same bits set. Equal sets
// have equal signatures, but not the reverse.
func (s *Fixed128) Equal(other Fixed128) bool {
	var x [fixed128Words]bool

	// loop-unrolled
	x[0] = s[0] == other[0]
	x[1] = s[1] == other[1]

	return x[0] && x[1]
}

// Contains returns whether every bit set in other is also set in s. A missing
// bit proves that other holds an element s cannot hold; the converse says
// nothing, as distinct elements can fold onto the same bit.
func (s *Fixed128) Contains(other Fixed128) bool {
	var x [fixed128Words]uint64

	// loop-unrolled
	x[0] = other[0] &^ s[0]
	x[1] = other[1] &^ s[1]

	return x[0] == 0 && x[1] == 0
}

// Set the bit at the given offset in the signature. Offsets at or above
// Fixed128Bits are silently dropped, so callers fold their hashes with
// Fixed128Bits-1.
func (s *Fixed128) Set(bit uint) {
	// loop-unrolled
	// 0 .. 63
	if bit <= 63 {
		s[0] |= 1 << bit
	} else {
		s[1] |= 1 << (bit - 64)
	}
}

// Fixed256 is a fixed length bit-vector of Fixed256Bits bits. See Fixed128 for
// what the fixed vectors are for; this is the widest of them, for sets large
// enough that the narrower vectors would fill up and lose their selectivity.
//
// The zero Fixed256 is an empty signature, which every other Fixed256 contains.
type Fixed256 [fixed256Words]uint64

// Equal returns whether the two signatures have the same bits set. Equal sets
// have equal signatures, but not the reverse.
func (s *Fixed256) Equal(other Fixed256) bool {
	var x [fixed256Words]bool

	// loop-unrolled
	x[0] = s[0] == other[0]
	x[1] = s[1] == other[1]
	x[2] = s[2] == other[2]
	x[3] = s[3] == other[3]

	return x[0] && x[1] && x[2] && x[3]
}

// Contains returns whether every bit set in other is also set in s. A missing
// bit proves that other holds an element s cannot hold; the converse says
// nothing, as distinct elements can fold onto the same bit.
func (s *Fixed256) Contains(other Fixed256) bool {
	var x [fixed256Words]uint64

	// loop-unrolled
	x[0] = other[0] &^ s[0]
	x[1] = other[1] &^ s[1]
	x[2] = other[2] &^ s[2]
	x[3] = other[3] &^ s[3]

	return x[0] == 0 && x[1] == 0 && x[2] == 0 && x[3] == 0
}

// Set the bit at the given offset in the signature. Offsets at or above
// Fixed256Bits are silently dropped, so callers fold their hashes with
// Fixed256Bits-1.
func (s *Fixed256) Set(bit uint) {
	// loop-unrolled
	switch {
	case bit <= 63: // 0 .. 63
		s[0] |= 1 << bit
	case bit <= 127:
		s[1] |= 1 << (bit - 64)
	case bit <= 191:
		s[2] |= 1 << (bit - 128)
	default:
		s[3] |= 1 << (bit - 192)
	}
}
