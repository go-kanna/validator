package impl_test

import (
	"iter"
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/go-kanna/validator/impl"
)

type span struct{ from, to int }

func (s span) MarshalText() ([]byte, error) {
	return []byte(strconv.Itoa(s.from) + "-" + strconv.Itoa(s.to)), nil
}

type entry[K comparable] struct {
	key   K
	value string
}

func collect[K comparable](seq iter.Seq2[K, string]) []entry[K] {
	var all []entry[K]
	for k, v := range seq {
		all = append(all, entry[K]{k, v})
	}
	return all
}

func TestSortedByKey(t *testing.T) {
	t.Parallel()

	m := map[string]string{"b": "2", "c": "3", "a": "1"}
	want := []entry[string]{{"a", "1"}, {"b", "2"}, {"c", "3"}}
	for range 20 {
		if got := collect(impl.SortedByKey(m)); !slices.Equal(got, want) {
			t.Fatalf("SortedByKey() = %v, want %v", got, want)
		}
	}
}

// A NaN key, which no lookup finds, still comes with its value, first as in
// slices.Sorted.
func TestSortedByKey_NaN(t *testing.T) {
	t.Parallel()

	got := collect(impl.SortedByKey(map[float64]string{1: "one", math.NaN(): "nan", -1: "minus one"}))
	if len(got) != 3 || !math.IsNaN(got[0].key) || got[0].value != "nan" ||
		!slices.Equal(got[1:], []entry[float64]{{-1, "minus one"}, {1, "one"}}) {
		t.Errorf("SortedByKey() = %v, want [{NaN nan} {-1 minus one} {1 one}]", got)
	}
}

func TestSortedByName(t *testing.T) {
	t.Parallel()

	m := map[span]string{{10, 20}: "b", {2, 3}: "c", {1, 5}: "a"}
	want := []entry[span]{{span{1, 5}, "a"}, {span{10, 20}, "b"}, {span{2, 3}, "c"}}
	for range 20 {
		if got := collect(impl.SortedByName(m)); !slices.Equal(got, want) {
			t.Fatalf("SortedByName() = %v, want %v", got, want)
		}
	}
}

func TestSortedByName_NaN(t *testing.T) {
	t.Parallel()

	type point struct{ X float64 }
	got := collect(impl.SortedByName(map[point]string{{math.NaN()}: "nan"}))
	if len(got) != 1 || got[0].value != "nan" {
		t.Errorf("SortedByName() = %v, want the value under the NaN key", got)
	}
}

func TestSortedByKey_StopsWhenAsked(t *testing.T) {
	t.Parallel()

	var seen []string
	for k := range impl.SortedByKey(map[string]int{"a": 1, "b": 2}) {
		seen = append(seen, k)
		break
	}
	if !slices.Equal(seen, []string{"a"}) {
		t.Errorf("SortedByKey() yielded %v after a break, want [a]", seen)
	}
}
