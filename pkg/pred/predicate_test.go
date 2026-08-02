package pred

import (
	"fmt"
	"testing"

	"github.com/bowei/kvec/pkg/kv"
)

// newKV builds a kv.KV from alternating key, value arguments.
func newKV(kvs ...string) *kv.KV {
	return kv.New(func(yield func(k, v string) bool) {
		for i := 0; i < len(kvs); i += 2 {
			if !yield(kvs[i], kvs[i+1]) {
				return
			}
		}
	}, len(kvs)/2)
}

func TestMatch(t *testing.T) {
	pod := newKV("app", "web", "env", "prod", "tier", "frontend")
	empty := newKV()

	for _, tc := range []struct {
		name string
		b    *PredicateBuilder
		kv   *kv.KV
		want bool
	}{
		{"empty predicate", NewPredicateBuilder(), pod, true},
		{"empty predicate, empty kv", NewPredicateBuilder(), empty, true},

		{"key present", NewPredicateBuilder().Key("app"), pod, true},
		{"key absent", NewPredicateBuilder().Key("missing"), pod, false},
		{"all keys present", NewPredicateBuilder().Key("app", "env"), pod, true},
		{"one key absent", NewPredicateBuilder().Key("app", "missing"), pod, false},

		{"nokey absent", NewPredicateBuilder().NoKey("missing"), pod, true},
		{"nokey present", NewPredicateBuilder().NoKey("app"), pod, false},
		{"key and nokey on same key", NewPredicateBuilder().Key("app").NoKey("app"), pod, false},

		{"has match", NewPredicateBuilder().Has(map[string]string{"app": "web"}), pod, true},
		{"has wrong value", NewPredicateBuilder().Has(map[string]string{"app": "api"}), pod, false},
		{"has missing key", NewPredicateBuilder().Has(map[string]string{"missing": "web"}), pod, false},
		{"has multiple", NewPredicateBuilder().Has(map[string]string{"app": "web", "env": "prod"}), pod, true},
		{"has one wrong", NewPredicateBuilder().Has(map[string]string{"app": "web", "env": "dev"}), pod, false},
		{"has empty map", NewPredicateBuilder().Has(nil), pod, true},

		{"in match", NewPredicateBuilder().ValueIn("env", []string{"prod", "staging"}), pod, true},
		{"in no match", NewPredicateBuilder().ValueIn("env", []string{"dev", "staging"}), pod, false},
		{"in missing key", NewPredicateBuilder().ValueIn("missing", []string{"prod"}), pod, false},
		{"in empty values", NewPredicateBuilder().ValueIn("env", nil), pod, false},

		{"notin match", NewPredicateBuilder().ValueNotIn("env", []string{"dev", "staging"}), pod, true},
		{"notin no match", NewPredicateBuilder().ValueNotIn("env", []string{"prod"}), pod, false},
		{"notin missing key", NewPredicateBuilder().ValueNotIn("missing", []string{"prod"}), pod, true},
		{"notin empty values", NewPredicateBuilder().ValueNotIn("env", nil), pod, true},

		{
			"conjunction of every term",
			NewPredicateBuilder().
				Key("app").
				NoKey("deprecated").
				Has(map[string]string{"tier": "frontend"}).
				ValueIn("env", []string{"prod", "staging"}).
				ValueNotIn("app", []string{"api"}),
			pod,
			true,
		},
		{
			"conjunction with one failing term",
			NewPredicateBuilder().
				Key("app").
				NoKey("tier").
				ValueIn("env", []string{"prod"}),
			pod,
			false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.b.Build().Match(tc.kv); got != tc.want {
				t.Errorf("Match() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestValuesCloned checks that a caller mutating the slice it passed cannot
// change the predicate.
func TestValuesCloned(t *testing.T) {
	values := []string{"prod"}
	p := NewPredicateBuilder().ValueIn("env", values).Build()
	values[0] = "dev"

	if !p.Match(newKV("env", "prod")) {
		t.Errorf("Match() = false, want true; ValueIn did not clone values")
	}
}

// TestRepeatedTerms covers the terms Build merges: repeated calls on one key
// must mean the same thing as the conjunction of the calls.
func TestRepeatedTerms(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    *PredicateBuilder
		kv   *kv.KV
		want bool
	}{
		{
			"in twice, value in both",
			NewPredicateBuilder().
				ValueIn("env", []string{"prod", "staging"}).
				ValueIn("env", []string{"staging", "dev"}),
			newKV("env", "staging"), true,
		},
		{
			"in twice, value in only one",
			NewPredicateBuilder().
				ValueIn("env", []string{"prod", "staging"}).
				ValueIn("env", []string{"staging", "dev"}),
			newKV("env", "prod"), false,
		},
		{
			"in twice, disjoint",
			NewPredicateBuilder().
				ValueIn("env", []string{"prod"}).
				ValueIn("env", []string{"dev"}),
			newKV("env", "prod"), false,
		},
		{
			"notin twice, value in the second",
			NewPredicateBuilder().
				ValueNotIn("env", []string{"dev"}).
				ValueNotIn("env", []string{"staging"}),
			newKV("env", "staging"), false,
		},
		{
			"notin twice, value in neither",
			NewPredicateBuilder().
				ValueNotIn("env", []string{"dev"}).
				ValueNotIn("env", []string{"staging"}),
			newKV("env", "prod"), true,
		},
		{
			"in and notin on one key, value survives",
			NewPredicateBuilder().
				ValueIn("env", []string{"prod", "staging"}).
				ValueNotIn("env", []string{"staging"}),
			newKV("env", "prod"), true,
		},
		{
			"in and notin on one key, value excluded",
			NewPredicateBuilder().
				ValueIn("env", []string{"prod", "staging"}).
				ValueNotIn("env", []string{"staging"}),
			newKV("env", "staging"), false,
		},
		{
			"notin empties the in set",
			NewPredicateBuilder().
				ValueIn("env", []string{"prod"}).
				ValueNotIn("env", []string{"prod"}),
			newKV("env", "prod"), false,
		},
		{
			"has agrees with in",
			NewPredicateBuilder().
				Has(map[string]string{"env": "prod"}).
				ValueIn("env", []string{"prod", "staging"}),
			newKV("env", "prod"), true,
		},
		{
			"has disagrees with in",
			NewPredicateBuilder().
				Has(map[string]string{"env": "prod"}).
				ValueIn("env", []string{"staging"}),
			newKV("env", "prod"), false,
		},
		{
			"has disagrees with notin",
			NewPredicateBuilder().
				Has(map[string]string{"env": "prod"}).
				ValueNotIn("env", []string{"prod"}),
			newKV("env", "prod"), false,
		},
		{
			"nokey and notin on one key",
			NewPredicateBuilder().
				NoKey("env").
				ValueNotIn("env", []string{"prod"}),
			newKV("app", "web"), true,
		},
		{
			"key repeated",
			NewPredicateBuilder().Key("app", "app").Key("app"),
			newKV("app", "web"), true,
		},
		{
			"key alongside has on the same key",
			NewPredicateBuilder().Key("env").Has(map[string]string{"env": "prod"}),
			newKV("env", "dev"), false,
		},
		{
			"nokey alongside has on the same key",
			NewPredicateBuilder().NoKey("env").Has(map[string]string{"env": "prod"}),
			newKV("env", "prod"), false,
		},
		{
			"nokey alongside in on the same key",
			NewPredicateBuilder().NoKey("env").ValueIn("env", []string{"prod", "dev"}),
			newKV("env", "prod"), false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.b.Build().Match(tc.kv); got != tc.want {
				t.Errorf("Match() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestLargeValueSet exercises the binary search that contains switches to past
// linearScanMax.
func TestLargeValueSet(t *testing.T) {
	values := make([]string, 0, 4*linearScanMax)
	for i := range cap(values) {
		values = append(values, fmt.Sprintf("v%02d", i))
	}
	in := NewPredicateBuilder().ValueIn("env", values).Build()
	notIn := NewPredicateBuilder().ValueNotIn("env", values).Build()

	for _, v := range values {
		if !in.Match(newKV("env", v)) {
			t.Errorf("in.Match(env=%s) = false, want true", v)
		}
		if notIn.Match(newKV("env", v)) {
			t.Errorf("notIn.Match(env=%s) = true, want false", v)
		}
	}
	if in.Match(newKV("env", "absent")) {
		t.Errorf("in.Match(env=absent) = true, want false")
	}
	if !notIn.Match(newKV("env", "absent")) {
		t.Errorf("notIn.Match(env=absent) = false, want true")
	}
}

// TestBuildIndependent checks that a Predicate does not alias the builder it
// came from, so that adding terms afterwards leaves it alone.
func TestBuildIndependent(t *testing.T) {
	b := NewPredicateBuilder().
		Key("app").
		Has(map[string]string{"env": "prod"}).
		ValueIn("tier", []string{"frontend", "backend"}).
		ValueNotIn("app", []string{"api"})
	first := b.Build()

	b.Key("added").
		NoKey("env").
		Has(map[string]string{"env": "dev"}).
		ValueIn("tier", []string{"cache"}).
		ValueNotIn("app", []string{"web"})

	kv := newKV("app", "web", "env", "prod", "tier", "frontend")
	if !first.Match(kv) {
		t.Errorf("first.Match() = false, want true; Build aliased the builder")
	}
	if second := b.Build(); second.Match(kv) {
		t.Errorf("second.Match() = true, want false")
	}
	if !first.Match(kv) {
		t.Errorf("first.Match() = false after a second Build, want true")
	}
}
