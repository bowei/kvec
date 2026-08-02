package kv

// sigLen puts 64*sigLen bits for the signature. Assumining uniform hash,
// we should expect (small) sets of keys and values will map to differing
// offsets, meaning that we only have to do an O(1) bit-op to check
// Contains/ContainsKeys.
//
// The current value handles sets of << 1k key/values. We will need more bits
// if we expect more than this as a common case.
const sigLen = 2

// fixedSignature is a fixed length bit-vector.
type fixedSignature [sigLen]uint64

func (s *fixedSignature) equal(other fixedSignature) bool {
	var x [sigLen]bool

	// loop-unrolled
	x[0] = s[0] == other[0]
	x[1] = s[1] == other[1]

	return x[0] && x[1]
}

func (s *fixedSignature) contains(other fixedSignature) bool {
	var x [sigLen]uint64

	// loop-unrolled
	x[0] = other[0] &^ s[0]
	x[1] = other[1] &^ s[1]

	return x[0] == 0 && x[1] == 0
}

// set the bit at the given offset in the signature.
func (s *fixedSignature) set(bit uint) {
	// loop-unrolled
	// 0 .. 63
	if bit <= 63 {
		s[0] |= 1 << bit
	} else {
		s[1] |= 1 << (bit - 64)
	}
}
