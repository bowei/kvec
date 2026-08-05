package fnv

import (
	"encoding/binary"
	"fmt"
	stdfnv "hash/fnv"
	"math/rand"
	"testing"
)

// std is the standard library's FNV-1a over the same bytes, which is what these
// functions have to agree with to be called FNV-1a at all.
func std(data []byte) uint64 {
	h := stdfnv.New64a()
	h.Write(data)
	return h.Sum64()
}

func TestHash64MatchesStdlib(t *testing.T) {
	inputs := [][]byte{
		nil,
		{},
		[]byte("a"),
		[]byte("ab"),
		[]byte("a-longer-key"),
		{0x00},
		{0x00, 0xff, 0x80, 0x7f},
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 64; i++ {
		b := make([]byte, rng.Intn(64))
		rng.Read(b)
		inputs = append(inputs, b)
	}

	for _, in := range inputs {
		if got, want := Hash64(in), std(in); got != want {
			t.Errorf("Hash64(%#x) = %#x, want %#x", in, got, want)
		}
	}
	// The empty hash is the offset basis by definition.
	if got := Hash64(nil); got != OffsetBasis64 {
		t.Errorf("Hash64(nil) = %#x, want the offset basis %#x", got, OffsetBasis64)
	}
}

// TestAdd64Concatenates is the property the callers rely on: folding data in
// pieces is folding the concatenation, which is why they have to delimit their
// own fields.
func TestAdd64Concatenates(t *testing.T) {
	parts := [][]byte{[]byte("a"), []byte(""), []byte("longer"), {0x00, 0xff}}
	var all []byte
	hash := OffsetBasis64
	for _, p := range parts {
		hash = Add64(hash, p)
		all = append(all, p...)
	}
	if want := Hash64(all); hash != want {
		t.Errorf("folding %q in pieces = %#x, want %#x", parts, hash, want)
	}
}

// TestAddWord64IsTheBytesOfTheWord pins the byte order: AddWord64 must be
// Add64 over the eight little-endian bytes of x, or the stream it produces is
// not one a byte-wise FNV-1a could produce.
func TestAddWord64IsTheBytesOfTheWord(t *testing.T) {
	words := []uint64{0, 1, 0xff, 0x100, 0x0123456789abcdef, ^uint64(0)}
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 64; i++ {
		words = append(words, rng.Uint64())
	}

	for _, x := range words {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], x)
		if got, want := AddWord64(OffsetBasis64, x), Hash64(b[:]); got != want {
			t.Errorf("AddWord64(basis, %#x) = %#x, want %#x", x, got, want)
		}
		// Distinct words must not fold alike, which the byte loop gives for
		// free but a shortcut such as mixing in x directly might not.
		if got := AddWord64(OffsetBasis64, x+1); got == AddWord64(OffsetBasis64, x) {
			t.Errorf("AddWord64 folds %#x and %#x alike", x, x+1)
		}
	}
}

func BenchmarkHash64(b *testing.B) {
	for _, n := range []int{0, 8, 64, 1024} {
		data := []byte(fmt.Sprintf("%*s", n, ""))
		b.Run(fmt.Sprintf("bytes=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sink = Hash64(data)
			}
		})
	}
}

// BenchmarkStdlibHash64 is the comparison behind writing the fold out inline.
// It does not show what that argument assumes: at a call site this simple the
// compiler inlines New64a and keeps the hash object on the stack, so the
// standard library allocates nothing either and the two run within noise of
// each other. The allocation is only avoided for certain where the object would
// escape. What this package buys at these sizes is a plain function, not speed.
func BenchmarkStdlibHash64(b *testing.B) {
	data := []byte("a-longer-key")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sink = std(data)
	}
}

// sink keeps the compiler from folding away the benchmarked call.
var sink uint64
