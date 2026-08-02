package kv

import (
	"fmt"
	"math"
	"testing"
)

// TestSignatureCoversEveryElement checks that every element's bit really lands
// in the signature. A bitMask wider than the signature would silently drop the
// out-of-range offsets in set, leaving most elements unrepresented.
func TestSignatureCoversEveryElement(t *testing.T) {
	next := randMaps(4)
	for i := 0; i < 1000; i++ {
		m := next()
		s := fromMap(m)
		for _, e := range s.kvs {
			var kbit, kvbit fixedSignature
			kbit.set(e.kbit())
			kvbit.set(e.kvbit())
			if (kbit == fixedSignature{}) || (kvbit == fixedSignature{}) {
				t.Fatalf("%s=%s: set produced an empty signature", e.k, e.v)
			}
			if !s.ksig.contains(kbit) {
				t.Fatalf("%v: ksig %#x is missing the bit for key %q", m, s.ksig, e.k)
			}
			if !s.kvsig.contains(kvbit) {
				t.Fatalf("%v: kvsig %#x is missing the bit for %s=%s", m, s.kvsig, e.k, e.v)
			}
		}
	}
}

// TestSignatureBitsAreReachable pins bitMask to the signature's width: every
// offset it can produce must set a bit, and between them they must cover the
// whole signature.
func TestSignatureBitsAreReachable(t *testing.T) {
	var all, want fixedSignature
	for i := range want {
		want[i] = ^uint64(0)
	}
	for bit := uint(0); bit <= uint(bitMask); bit++ {
		var s fixedSignature
		s.set(bit)
		if (s == fixedSignature{}) {
			t.Fatalf("set(%d) set no bit: bitMask is wider than the signature", bit)
		}
		for i := range all {
			all[i] |= s[i]
		}
	}
	if all != want {
		t.Errorf("bits reachable through bitMask = %#x, want %#x: bitMask is narrower than the signature", all, want)
	}
}

// TestSignatureRejectionRate reports the share of absent keys that ksig rejects
// outright, which is what BenchmarkContainsKeysSaturation is timing the
// consequences of. It asserts only the direction, as the rate depends on the
// hash; the numbers it logs are the useful part.
func TestSignatureRejectionRate(t *testing.T) {
	probes := make([]string, 4096)
	for i := range probes {
		probes[i] = fmt.Sprintf("absent-%06d", i)
	}

	var prev float64 = 1
	for _, n := range []int{8, 16, 32, 64, 128, 256, 1024} {
		set := benchSet(n)
		rejected := 0
		for _, p := range probes {
			var probeSig fixedSignature
			probeSig.set(makeKV(p, "").kbit())
			if !set.ksig.contains(probeSig) {
				rejected++
			}
		}
		rate := float64(rejected) / float64(len(probes))
		t.Logf("n=%4d: ksig rejects %5.1f%% of absent keys (expected %5.1f%%)",
			n, 100*rate, 100*math.Pow(1-1/float64(64*sigLen), float64(n)))
		if rate > prev {
			t.Errorf("n=%d: rejection rate %.3f rose above %.3f; the gate should only decay as the set fills", n, rate, prev)
		}
		prev = rate
	}
}
