// Package impl is what the code kanna-validator generates runs on: the order it walks a map in. Only generated code
// imports it. Its API changes with the generator, which ships in the same module.
package impl

import (
	"cmp"
	"iter"
	"slices"
	"strings"

	"github.com/go-kanna/validator"
)

// SortedByKey yields the keys and values of m in the order of the keys, the
// one slices.Sorted puts them in. A generated Validate walks a map whose keys
// have an order in it, so that its errors keep one order.
func SortedByKey[M ~map[K]V, K cmp.Ordered, V any](m M) iter.Seq2[K, V] {
	return sortedBy(m, func(k K) K { return k }, cmp.Compare[K])
}

// SortedByName yields the keys and values of m in the order of the keys' names
// in a path, as validator.Path.Key spells them. A generated Validate walks a
// map whose keys have no order of their own, such as structs, in it, so that
// its errors keep one order.
func SortedByName[M ~map[K]V, K comparable, V any](m M) iter.Seq2[K, V] {
	return sortedBy(m, func(k K) string { return validator.Path{}.Key(k).Segments()[0].Name }, strings.Compare)
}

// sortedBy yields each key of m with its value, rather than looking the value
// up by the key, which a NaN key would miss.
func sortedBy[M ~map[K]V, K comparable, V, O any](m M, order func(K) O, compare func(O, O) int) iter.Seq2[K, V] {
	type entry struct {
		key   K
		value V
		order O
	}
	return func(yield func(K, V) bool) {
		all := make([]entry, 0, len(m))
		for k, v := range m {
			all = append(all, entry{key: k, value: v, order: order(k)})
		}
		slices.SortFunc(all, func(a, b entry) int { return compare(a.order, b.order) })
		for _, e := range all {
			if !yield(e.key, e.value) {
				return
			}
		}
	}
}
