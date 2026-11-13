package impl_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/go-kanna/validator/impl"
)

type span struct{ from, to int }

func (s span) MarshalText() ([]byte, error) {
	return []byte(strconv.Itoa(s.from) + "-" + strconv.Itoa(s.to)), nil
}

func TestSortedKeys(t *testing.T) {
	t.Parallel()

	m := map[span]int{{10, 20}: 0, {2, 3}: 0, {1, 5}: 0}
	want := []span{{1, 5}, {10, 20}, {2, 3}}
	for range 20 {
		if got := impl.SortedKeys(m); !slices.Equal(got, want) {
			t.Fatalf("SortedKeys() = %v, want %v", got, want)
		}
	}
}
