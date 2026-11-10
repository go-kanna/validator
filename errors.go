package validator

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Base is the path of an error that concerns the struct as a whole rather than
// one of its fields, "base" as in Rails.
var Base = Field("base")

// Path is where in a value an error is: the fields, map keys, and slice
// indexes leading to it. Field starts one, and its methods go on with it. Its
// zero value is the value itself.
type Path struct {
	segments []Segment
}

// Segment is one step of a path.
type Segment struct {
	Kind SegmentKind

	// Name is the field's JSON name, or the key spelled as encoding/json names
	// it in an object, for a FieldSegment and a KeySegment.
	Name string

	// Index is the element's index, for an IndexSegment.
	Index int
}

// SegmentKind is what a segment of a path steps into.
type SegmentKind int

// The kinds of segments.
const (
	// FieldSegment is a field of a struct.
	FieldSegment SegmentKind = iota + 1
	// KeySegment is the value under a key of a map.
	KeySegment
	// IndexSegment is an element of a slice or an array.
	IndexSegment
)

// Field returns the path through the fields named name and then names, each by
// its JSON name, such as Field("address", "zip") for "address.zip". The
// methods of Path go on with it.
func Field(name string, names ...string) Path {
	return Path{}.Field(name, names...)
}

// Field returns the path through the fields named name and then names below p,
// such as Field("items").Index(2).Field("sku") for "items[2].sku".
func (p Path) Field(name string, names ...string) Path {
	segments := make([]Segment, len(p.segments), len(p.segments)+1+len(names))
	copy(segments, p.segments)
	segments = append(segments, Segment{Kind: FieldSegment, Name: name})
	for _, n := range names {
		segments = append(segments, Segment{Kind: FieldSegment, Name: n})
	}
	return Path{segments: segments}
}

// Index returns the path to element i of the slice at p, such as "tags[2]".
func (p Path) Index(i int) Path {
	return p.with(Segment{Kind: IndexSegment, Index: i})
}

// Key returns the path to the value under key in the map at p, such as
// "labels.primary": a map is a JSON object, so its keys are spelled as the
// names of an object are, as DecodeJSON reports them. key is a key of the map,
// of a type a map can have keys of.
func (p Path) Key(key any) Path {
	return p.with(Segment{Kind: KeySegment, Name: keyName(key)})
}

// Segments returns the steps of p, in order.
func (p Path) Segments() []Segment {
	return slices.Clone(p.segments)
}

// String spells p with JSON names, as FieldError.Field does, such as
// "items[2].sku".
func (p Path) String() string {
	return spell(p.segments)
}

func (p Path) with(s Segment) Path {
	return Path{segments: append(slices.Clip(p.segments), s)}
}

func spell(segments []Segment) string {
	if len(segments) == 1 && segments[0].Kind != IndexSegment {
		return segments[0].Name
	}
	var b strings.Builder
	for i, s := range segments {
		switch {
		case s.Kind == IndexSegment:
			b.WriteString("[" + strconv.Itoa(s.Index) + "]")
		case i > 0:
			b.WriteString("." + s.Name)
		default:
			b.WriteString(s.Name)
		}
	}
	return b.String()
}

// FieldError is a broken rule together with the field it broke on.
type FieldError struct {
	// Field is the path to the field, spelled with JSON names, such as
	// "address.zip" or "tags[2]"; or "base".
	Field string

	// Segments are the steps of the path, which tell a field from a map key:
	// "notes.first" may be either.
	Segments []Segment

	// Err is the rule's error. It is usually Coded; one that is not reports the
	// code "invalid", Rails' catch-all.
	Err error
}

// Path returns Field. It is the method go-kanna/i18n renders an error by,
// together with Code and Params, to name the field in a message.
func (e FieldError) Path() string {
	return e.Field
}

// FieldNames returns the names of the field segments of the path, in order,
// leaving out map keys and indexes: "items[2].notes.first" gives items and
// notes when first is a key. It returns nil without Segments. It lets a package
// such as go-kanna/i18n name the field without importing validator.
func (e FieldError) FieldNames() []string {
	var names []string
	for _, s := range e.Segments {
		if s.Kind == FieldSegment {
			names = append(names, s.Name)
		}
	}
	return names
}

// Code returns the code of Err, or "invalid" when Err carries none.
func (e FieldError) Code() string {
	if c, ok := errors.AsType[Coded](e.Err); ok {
		return c.Code()
	}
	return "invalid"
}

// Params returns the params of Err, or nil when Err carries none.
func (e FieldError) Params() map[string]any {
	if c, ok := errors.AsType[Coded](e.Err); ok {
		return c.Params()
	}
	return nil
}

// Error formats the field followed by the code and params, such as
// "tags[2]: too_long (count=20)". An error that is not Coded contributes its own
// text instead.
func (e FieldError) Error() string {
	if _, ok := errors.AsType[Coded](e.Err); !ok {
		return e.Field + ": " + e.Err.Error()
	}
	return e.Field + ": " + describe(e.Code(), e.Params())
}

// Unwrap returns Err.
func (e FieldError) Unwrap() error {
	return e.Err
}

// Errors collects every rule broken while validating a value, in the order they
// were found. A generated Validate method returns it through Err.
type Errors []FieldError

// Add records err as broken at p. A nil err is ignored, so that the result of
// a rule can be passed as it is. Errors, such as what another Validate
// returns, move under p, as Nest moves them.
func (e *Errors) Add(p Path, err error) {
	if _, ok := errors.AsType[Errors](err); ok {
		e.Nest(p, err)
		return
	}
	if err != nil {
		*e = append(*e, FieldError{Field: p.String(), Segments: p.Segments(), Err: err})
	}
}

// Nest records what validating the value at p reported. The errors of Errors
// move under p ("address" and "zip" make "address.zip"), and the zero Path
// leaves them where they are, as for an embedded struct. Any other error is
// recorded at p itself, or at Base for the zero Path. A nil err is ignored.
func (e *Errors) Nest(p Path, err error) {
	if err == nil {
		return
	}
	nested, ok := errors.AsType[Errors](err)
	if !ok {
		if len(p.segments) == 0 {
			p = Base
		}
		e.Add(p, err)
		return
	}
	for _, fe := range nested {
		if _, ok := errors.AsType[Errors](fe.Err); ok {
			e.Nest(Path{segments: slices.Concat(p.segments, fe.Segments)}, fe.Err)
			continue
		}
		*e = append(*e, FieldError{Field: join(p, fe), Segments: slices.Concat(p.segments, fe.Segments), Err: fe.Err})
	}
}

// join spells the path of fe under p.
func join(p Path, fe FieldError) string {
	switch {
	case len(p.segments) == 0:
		return fe.Field
	case fe.Field == "":
		return p.String()
	case len(fe.Segments) > 0 && fe.Segments[0].Kind == IndexSegment:
		return p.String() + fe.Field
	}
	return p.String() + "." + fe.Field
}

// SortedKeys returns the keys of m in the order of their names in a path, as
// Key spells them. A generated Validate walks a map whose keys have no order
// of their own, such as structs, in it, so that its errors keep one order.
func SortedKeys[M ~map[K]V, K comparable, V any](m M) []K {
	type named struct {
		key  K
		name string
	}
	all := make([]named, 0, len(m))
	for k := range m {
		all = append(all, named{key: k, name: keyName(k)})
	}
	slices.SortFunc(all, func(a, b named) int { return strings.Compare(a.name, b.name) })

	keys := make([]K, len(all))
	for i, n := range all {
		keys[i] = n.key
	}
	return keys
}

// keyName spells key as Marshal names it in an object: a string as it is, and
// anything else as the key of a map, which a String method has no say in.
func keyName(key any) string {
	switch k := key.(type) {
	case string:
		return k
	case int:
		return strconv.Itoa(k)
	}
	b, err := json.Marshal(map[any]struct{}{key: {}})
	if err != nil {
		return fmt.Sprint(key)
	}
	dec := jsontext.NewDecoder(bytes.NewReader(b))
	if _, err := dec.ReadToken(); err != nil {
		return fmt.Sprint(key)
	}
	name, err := dec.ReadToken()
	if err != nil {
		return fmt.Sprint(key)
	}
	return name.String()
}

// Err returns e, or nil when nothing broke. Returning e itself would hand the
// caller a non-nil error holding nothing.
func (e Errors) Err() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

// Error lists every broken rule, such as
// "validator: name: blank; tags[2]: too_long (count=20)".
func (e Errors) Error() string {
	parts := make([]string, len(e))
	for i, fe := range e {
		parts[i] = fe.Error()
	}
	return "validator: " + strings.Join(parts, "; ")
}

// Unwrap returns each FieldError, so that errors.As finds the rule's error
// behind any of them.
func (e Errors) Unwrap() []error {
	errs := make([]error, len(e))
	for i, fe := range e {
		errs[i] = fe
	}
	return errs
}

// MarshalJSON is MarshalJSONTo for an encoder that knows only json.Marshaler.
// encoding/json itself calls MarshalJSONTo.
func (e Errors) MarshalJSON() ([]byte, error) {
	return json.Marshal(e) //nolint:wrapcheck // MarshalJSONTo already says what failed
}

// MarshalJSONTo encodes e as Rails encodes errors.details: an object keyed by
// field, in the order the fields first broke, each holding a list with the code
// under "error" and the params beside it, in key order.
//
//	{"name":[{"error":"blank"}],"tags[2]":[{"error":"too_long","count":20}]}
//
// A param named "error" would hide the code, so it is left out.
func (e Errors) MarshalJSONTo(enc *jsontext.Encoder) error {
	var fields []string
	byField := make(map[string][]FieldError)
	for _, fe := range e {
		if _, seen := byField[fe.Field]; !seen {
			fields = append(fields, fe.Field)
		}
		byField[fe.Field] = append(byField[fe.Field], fe)
	}

	w := tokens{enc: enc}
	w.write(jsontext.BeginObject)
	for _, field := range fields {
		w.write(text(field))
		w.write(jsontext.BeginArray)
		for _, fe := range byField[field] {
			w.detail(fe)
		}
		w.write(jsontext.EndArray)
	}
	w.write(jsontext.EndObject)
	return w.err
}

// tokens writes to an encoder until the first failure, which it keeps.
type tokens struct {
	enc *jsontext.Encoder
	err error
}

func (w *tokens) write(t jsontext.Token) {
	if w.err == nil {
		if err := w.enc.WriteToken(t); err != nil {
			w.err = fmt.Errorf("validator: %w", err)
		}
	}
}

// detail writes one entry of a field's list.
func (w *tokens) detail(fe FieldError) {
	w.write(jsontext.BeginObject)
	w.write(text("error"))
	w.write(text(fe.Code()))
	params := fe.Params()
	for _, k := range slices.Sorted(maps.Keys(params)) {
		if k == "error" {
			continue
		}
		w.write(text(k))
		if w.err == nil {
			if err := json.MarshalEncode(w.enc, params[k], json.Deterministic(true)); err != nil {
				w.err = fmt.Errorf("validator: param %q of %s: %w", k, fe.Field, err)
			}
		}
	}
	w.write(jsontext.EndObject)
}

// text is a JSON string holding s, with invalid UTF-8 replaced as
// encoding/json would rather than refused.
func text(s string) jsontext.Token {
	return jsontext.String(strings.ToValidUTF8(s, "\uFFFD"))
}
