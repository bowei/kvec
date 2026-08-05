package sig

// fixedWords puts 64*fixedWords bits for the signature. Assumining uniform
// hash, we should expect (small) sets of keys and values will map to differing
// offsets, meaning that we only have to do an O(1) bit-op to check
// Contains/Equal.
//
// The current value handles sets of << 1k key/values. We will need more bits
// if we expect more than this as a common case.
const fixedWords = 2

// FixedBits is the number of bits addressed by a Fixed. It is a power of two,
// so a caller folding a hash onto a bit offset can mask with FixedBits-1 rather
// than divide.
const FixedBits = 64 * fixedWords

// Fixed is a fixed length bit-vector.
//
// This is the compile-time sized counterpart of Signature: the width is a
// constant, so the loops below are unrolled and the vector sits inline in the
// caller's struct. Prefer Fixed where the width is known and small, and
// Signature where it has to be chosen per instance.
//
// The zero Fixed is an empty signature, which every other Fixed contains.
type Fixed [fixedWords]uint64

// Equal returns whether the two signatures have the same bits set. Equal sets
// have equal signatures, but not the reverse.
func (s *Fixed) Equal(other Fixed) bool {
	var x [fixedWords]bool

	// loop-unrolled
	x[0] = s[0] == other[0]
	x[1] = s[1] == other[1]

	return x[0] && x[1]
}

// Contains returns whether every bit set in other is also set in s. A missing
// bit proves that other holds an element s cannot hold; the converse says
// nothing, as distinct elements can fold onto the same bit.
func (s *Fixed) Contains(other Fixed) bool {
	var x [fixedWords]uint64

	// loop-unrolled
	x[0] = other[0] &^ s[0]
	x[1] = other[1] &^ s[1]

	return x[0] == 0 && x[1] == 0
}

// Set the bit at the given offset in the signature. Offsets at or above
// FixedBits are silently dropped, so callers fold their hashes with FixedBits-1.
func (s *Fixed) Set(bit uint) {
	// loop-unrolled
	// 0 .. 63
	if bit <= 63 {
		s[0] |= 1 << bit
	} else {
		s[1] |= 1 << (bit - 64)
	}
}
