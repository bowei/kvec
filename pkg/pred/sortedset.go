package pred

import (
	"slices"
	"strings"
)

// sortedset is a set of strings held sorted and deduplicated. Every value of
// this type must satisfy that invariant: the set operations below rely on it,
// and contains binary searches on it.
type sortedset []string

// newSortedSet returns the values of s as a sortedset, in a slice of its own.
func newSortedSet(s []string) sortedset {
	out := sortedset(slices.Clone(s))
	slices.Sort(out)
	return slices.Compact(out)
}

// linearScanMax is the largest set that contains scans. Below it the scan wins
// on locality and branch prediction; above it the log2(n) comparisons of a
// binary search win.
const linearScanMax = 8

// contains reports whether s holds value.
func (s sortedset) contains(value string) bool {
	if len(s) <= linearScanMax {
		return slices.Contains(s, value)
	}
	_, found := slices.BinarySearch(s, value)
	return found
}

// intersect returns the values held by both s and other.
func (s sortedset) intersect(other sortedset) sortedset {
	var out sortedset
	i, j := 0, 0
	for i < len(s) && j < len(other) {
		switch c := strings.Compare(s[i], other[j]); {
		case c < 0:
			i++
		case c > 0:
			j++
		default:
			out = append(out, s[i])
			i++
			j++
		}
	}
	return out
}

// union returns the values held by either s or other.
func (s sortedset) union(other sortedset) sortedset {
	out := make(sortedset, 0, len(s)+len(other))
	i, j := 0, 0
	for i < len(s) && j < len(other) {
		switch c := strings.Compare(s[i], other[j]); {
		case c < 0:
			out = append(out, s[i])
			i++
		case c > 0:
			out = append(out, other[j])
			j++
		default:
			out = append(out, s[i])
			i++
			j++
		}
	}
	out = append(out, s[i:]...)
	return append(out, other[j:]...)
}

// subtract returns the values held by s but not by other.
func (s sortedset) subtract(other sortedset) sortedset {
	var out sortedset
	j := 0
	for _, v := range s {
		for j < len(other) && other[j] < v {
			j++
		}
		if j < len(other) && other[j] == v {
			continue
		}
		out = append(out, v)
	}
	return out
}
