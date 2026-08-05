// Package sig provides bit-vector signatures: one-sided approximations of the
// Contains operation on a set of strings. A signature answers "other cannot be
// a subset" exactly and "other may be a subset" approximately, which makes it a
// cheap gate in front of an exact check.
//
// Fixed64, Fixed128 and Fixed256 are sized at compile time and live inline in
// the caller's struct. Signature is sized per instance, at the cost of an
// allocation and a loop the compiler cannot unroll.
package sig

import (
	"fmt"
	"math/bits"

	"github.com/bowei/kvec/pkg/fnv"
)

const (
	// minSigBits is the smallest signature, one machine word. A narrower
	// signature would still cost a full word of storage, so there is nothing to
	// gain by allowing one.
	minSigBits = 64

	// maxSigBits caps the requested width at 2MiB of bit vector. Anything
	// larger is a caller error rather than a tuning choice, and the rounding in
	// sigBits would overflow near the top of the uint range.
	maxSigBits = 1 << 24
)

// NewSignature returns a Signature for a given set of keys. Only Signatures
// with equal bitWidth can be compared for Containment.
//
// bitWidth is rounded up to a power of two, at minimum 64, so that a hash can
// be folded into a bit offset with a mask instead of a division. Use BitWidth
// to recover the width that was actually allocated. Duplicate keys are
// harmless: they set the same bit.
//
// Panics if bitWidth exceeds maxSigBits.
func NewSignature(bitWidth uint, keys ...string) *Signature {
	sig := Signature{bitWidth: sigBits(bitWidth)}
	sig.words = make([]uint64, sig.bitWidth/64)
	for _, k := range keys {
		sig.set(fnv.Hash64([]byte(k)))
	}
	return &sig
}

// sigBits rounds bitWidth up to the power of two the signature will use.
func sigBits(bitWidth uint) uint {
	switch {
	case bitWidth > maxSigBits:
		panic(fmt.Sprintf("signature bitWidth %d exceeds the maximum of %d", bitWidth, maxSigBits))
	case bitWidth <= minSigBits:
		return minSigBits
	case bitWidth&(bitWidth-1) == 0:
		// Already a power of two.
		return bitWidth
	default:
		return 1 << uint(bits.Len(bitWidth))
	}
}

// Signature is a approximation for the Contains operation on a set of
// Strings. For two sets of strings A and B and associated Signatures
// SA and SB:
//
// - A contains B implies SA.Contains(SB).
// - SA.Contains(SB) does not necessarily mean A contains B.
//
// Signature.Contains can only be peformed on equal bitwidths.
//
// This is the dynamically sized counterpart of Fixed64/Fixed128/Fixed256: the
// width is chosen per instance rather than at compile time, at the cost of an
// allocation and a loop that the compiler cannot unroll. Prefer a fixed vector
// where the width is known and small.
//
// The zero Signature is an empty signature of width zero. It contains only
// other zero Signatures, and comparing it against a constructed one panics, so
// it is of little use beyond being a valid zero value.
type Signature struct {
	// bitWidth is the number of bits addressed by words, always 64*len(words).
	// It is kept explicitly so that the equal-width precondition reads as the
	// caller stated it.
	bitWidth uint

	// words is the bit vector, least significant bit of words[0] first.
	words []uint64
}

// BitWidth is the number of bits in the signature, after the rounding applied
// by NewSignature. Contains requires both sides to agree on this value.
func (sig *Signature) BitWidth() uint { return sig.bitWidth }

// Contains returns whether every bit set in other is also set in sig. A missing
// bit proves that other holds a key sig cannot hold; the converse says nothing,
// as distinct keys can fold onto the same bit.
//
// Panics if the two signatures have different widths, as there is no answer
// that preserves the one-sided guarantee above.
func (sig *Signature) Contains(other *Signature) bool {
	if sig.bitWidth != other.bitWidth {
		panic(fmt.Sprintf("cannot compare signatures of bitWidth %d and %d", sig.bitWidth, other.bitWidth))
	}
	// Equal widths mean equal lengths; the reslice states that to the bounds
	// check eliminator so the loop body carries no checks.
	otherWords := other.words[:len(sig.words)]
	for i, w := range sig.words {
		if otherWords[i]&^w != 0 {
			return false
		}
	}
	return true
}

// Equal returns whether the two signatures have the same width and the same
// bits set. Equal sets have equal signatures, but not the reverse.
func (sig *Signature) Equal(other *Signature) bool {
	if sig.bitWidth != other.bitWidth {
		return false
	}
	otherWords := other.words[:len(sig.words)]
	for i, w := range sig.words {
		if otherWords[i] != w {
			return false
		}
	}
	return true
}

// set the bit that hash folds onto. bitWidth is a power of two, so the low bits
// of the hash address the vector exactly.
func (sig *Signature) set(hash uint64) {
	bit := uint(hash) & (sig.bitWidth - 1)
	sig.words[bit/64] |= 1 << (bit % 64)
}
