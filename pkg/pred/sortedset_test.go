package pred

import (
	"fmt"
	"slices"
	"testing"
)

func TestNewSortedSet(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want sortedset
	}{
		{"nil", nil, nil},
		{"empty", []string{}, nil},
		{"one", []string{"a"}, sortedset{"a"}},
		{"already sorted", []string{"a", "b", "c"}, sortedset{"a", "b", "c"}},
		{"unsorted", []string{"c", "a", "b"}, sortedset{"a", "b", "c"}},
		{"duplicates", []string{"b", "a", "b", "a"}, sortedset{"a", "b"}},
		{"all one value", []string{"a", "a", "a"}, sortedset{"a"}},
		{"empty string is a value", []string{"b", "", "a"}, sortedset{"", "a", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := newSortedSet(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("newSortedSet(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNewSortedSetClones checks that the caller keeps its slice: newSortedSet
// must neither sort it in place nor hand back an alias of it.
func TestNewSortedSetClones(t *testing.T) {
	in := []string{"c", "a", "b"}
	s := newSortedSet(in)

	if want := []string{"c", "a", "b"}; !slices.Equal(in, want) {
		t.Errorf("input = %q after newSortedSet, want %q", in, want)
	}
	s[0] = "z"
	if want := []string{"c", "a", "b"}; !slices.Equal(in, want) {
		t.Errorf("input = %q after writing to the set, want %q", in, want)
	}
}

// TestContains covers both branches of contains: the linear scan at or below
// linearScanMax, and the binary search past it.
func TestContains(t *testing.T) {
	for _, size := range []int{0, 1, linearScanMax - 1, linearScanMax, linearScanMax + 1, 4 * linearScanMax} {
		t.Run(fmt.Sprintf("size %d", size), func(t *testing.T) {
			values := make([]string, 0, size)
			for i := range size {
				values = append(values, fmt.Sprintf("v%02d", i))
			}
			s := newSortedSet(values)

			for _, v := range values {
				if !s.contains(v) {
					t.Errorf("contains(%q) = false, want true", v)
				}
			}
			for _, v := range []string{"absent", "", "v", "v99"} {
				if s.contains(v) {
					t.Errorf("contains(%q) = true, want false", v)
				}
			}
		})
	}
}

func TestIntersect(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b sortedset
		want sortedset
	}{
		{"both empty", nil, nil, nil},
		{"empty receiver", nil, sortedset{"a"}, nil},
		{"empty argument", sortedset{"a"}, nil, nil},
		{"disjoint", sortedset{"a", "c"}, sortedset{"b", "d"}, nil},
		{"equal", sortedset{"a", "b"}, sortedset{"a", "b"}, sortedset{"a", "b"}},
		{"overlap", sortedset{"a", "b", "c"}, sortedset{"b", "c", "d"}, sortedset{"b", "c"}},
		{"receiver is a subset", sortedset{"b"}, sortedset{"a", "b", "c"}, sortedset{"b"}},
		{"argument is a subset", sortedset{"a", "b", "c"}, sortedset{"b"}, sortedset{"b"}},
		{"interleaved", sortedset{"a", "c", "e"}, sortedset{"a", "b", "e", "f"}, sortedset{"a", "e"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.intersect(tc.b); !slices.Equal(got, tc.want) {
				t.Errorf("%q.intersect(%q) = %q, want %q", tc.a, tc.b, got, tc.want)
			}
			// Intersection is symmetric.
			if got := tc.b.intersect(tc.a); !slices.Equal(got, tc.want) {
				t.Errorf("%q.intersect(%q) = %q, want %q", tc.b, tc.a, got, tc.want)
			}
		})
	}
}

func TestUnion(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b sortedset
		want sortedset
	}{
		{"both empty", nil, nil, nil},
		{"empty receiver", nil, sortedset{"a", "b"}, sortedset{"a", "b"}},
		{"empty argument", sortedset{"a", "b"}, nil, sortedset{"a", "b"}},
		{"disjoint", sortedset{"a", "c"}, sortedset{"b", "d"}, sortedset{"a", "b", "c", "d"}},
		{"equal", sortedset{"a", "b"}, sortedset{"a", "b"}, sortedset{"a", "b"}},
		{"overlap", sortedset{"a", "b", "c"}, sortedset{"b", "c", "d"}, sortedset{"a", "b", "c", "d"}},
		{"receiver runs out first", sortedset{"a"}, sortedset{"b", "c", "d"}, sortedset{"a", "b", "c", "d"}},
		{"argument runs out first", sortedset{"a", "b", "c"}, sortedset{"a"}, sortedset{"a", "b", "c"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.union(tc.b); !slices.Equal(got, tc.want) {
				t.Errorf("%q.union(%q) = %q, want %q", tc.a, tc.b, got, tc.want)
			}
			// Union is symmetric.
			if got := tc.b.union(tc.a); !slices.Equal(got, tc.want) {
				t.Errorf("%q.union(%q) = %q, want %q", tc.b, tc.a, got, tc.want)
			}
		})
	}
}

func TestSubtract(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b sortedset
		want sortedset
	}{
		{"both empty", nil, nil, nil},
		{"empty receiver", nil, sortedset{"a"}, nil},
		{"empty argument", sortedset{"a", "b"}, nil, sortedset{"a", "b"}},
		{"disjoint", sortedset{"a", "c"}, sortedset{"b", "d"}, sortedset{"a", "c"}},
		{"equal", sortedset{"a", "b"}, sortedset{"a", "b"}, nil},
		{"overlap", sortedset{"a", "b", "c"}, sortedset{"b", "c", "d"}, sortedset{"a"}},
		{"argument is a superset", sortedset{"b"}, sortedset{"a", "b", "c"}, nil},
		{"removes the first", sortedset{"a", "b", "c"}, sortedset{"a"}, sortedset{"b", "c"}},
		{"removes the last", sortedset{"a", "b", "c"}, sortedset{"c"}, sortedset{"a", "b"}},
		{"argument trails past the receiver", sortedset{"a"}, sortedset{"b", "c"}, sortedset{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.subtract(tc.b); !slices.Equal(got, tc.want) {
				t.Errorf("%q.subtract(%q) = %q, want %q", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestOperandsUnchanged checks that the set operations neither write to their
// operands nor alias one in the result, since Build holds on to sets it has
// already combined. The empty operands matter most: they are what tempts an
// operation into handing the other one straight back.
func TestOperandsUnchanged(t *testing.T) {
	for _, op := range []struct {
		name string
		fn   func(a, b sortedset) sortedset
	}{
		{"intersect", sortedset.intersect},
		{"union", sortedset.union},
		{"subtract", sortedset.subtract},
	} {
		for _, operands := range []struct {
			name string
			a, b sortedset
		}{
			{"overlapping", sortedset{"a", "b", "c"}, sortedset{"b", "c", "d"}},
			{"disjoint", sortedset{"a", "c"}, sortedset{"b", "d"}},
			{"empty argument", sortedset{"a", "b", "c"}, sortedset{}},
			{"empty receiver", sortedset{}, sortedset{"b", "c", "d"}},
		} {
			t.Run(op.name+"/"+operands.name, func(t *testing.T) {
				a, b := slices.Clone(operands.a), slices.Clone(operands.b)
				out := op.fn(a, b)
				for i := range out {
					out[i] = "z"
				}
				if !slices.Equal(a, operands.a) {
					t.Errorf("receiver = %q, want %q", a, operands.a)
				}
				if !slices.Equal(b, operands.b) {
					t.Errorf("argument = %q, want %q", b, operands.b)
				}
			})
		}
	}
}

// TestResultsAreSets checks the invariant the type carries: whatever the
// operations return is itself sorted and deduplicated, so it can be fed back in.
func TestResultsAreSets(t *testing.T) {
	a := sortedset{"a", "b", "c", "e"}
	b := sortedset{"b", "c", "d"}

	for _, tc := range []struct {
		name string
		got  sortedset
	}{
		{"intersect", a.intersect(b)},
		{"union", a.union(b)},
		{"subtract", a.subtract(b)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !slices.IsSorted(tc.got) {
				t.Errorf("%q is not sorted", tc.got)
			}
			if compacted := slices.Compact(slices.Clone(tc.got)); len(compacted) != len(tc.got) {
				t.Errorf("%q holds duplicates", tc.got)
			}
		})
	}
}
