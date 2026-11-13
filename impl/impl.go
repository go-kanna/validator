// Package impl is what the code kanna-validator generates runs on: the order it walks a map in. Only generated code
// imports it. Its API changes with the generator, which ships in the same module.
package impl

import (
	"slices"
	"strings"

	"github.com/go-kanna/validator"
)

// SortedKeys returns the keys of m in the order of their names in a path, as
// validator.Path.Key spells them. A generated Validate walks a map whose keys
// have no order of their own, such as structs, in it, so that its errors keep
// one order.
func SortedKeys[M ~map[K]V, K comparable, V any](m M) []K {
	type named struct {
		key  K
		name string
	}
	all := make([]named, 0, len(m))
	for k := range m {
		all = append(all, named{key: k, name: validator.Path{}.Key(k).Segments()[0].Name})
	}
	slices.SortFunc(all, func(a, b named) int { return strings.Compare(a.name, b.name) })

	keys := make([]K, len(all))
	for i, n := range all {
		keys[i] = n.key
	}
	return keys
}
