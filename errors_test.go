package validator_test

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-kanna/validator"
)

// typeError stands in for a Coded error from another package, such as
// *rule.Error of go-kanna/value.
type typeError struct{ code string }

func (e *typeError) Error() string          { return "type: " + e.code }
func (e *typeError) Code() string           { return e.code }
func (e *typeError) Params() map[string]any { return nil }

// equalErrors compares by field and by the text of each error, which is enough
// to tell apart the errors these tests build.
func equalErrors(a, b validator.Errors) bool {
	return slices.EqualFunc(a, b, func(x, y validator.FieldError) bool {
		return x.Field == y.Field && x.Err.Error() == y.Err.Error()
	})
}

func TestErrors_Add(t *testing.T) {
	t.Parallel()

	var errs validator.Errors
	errs.Add(validator.Field("name"), validator.Blank())
	errs.Add(validator.Field("age"), nil)
	errs.Add(validator.Field("tags").Index(1), validator.TooLong(20))

	want := validator.Errors{{Field: "name", Err: validator.Blank()}, {Field: "tags[1]", Err: validator.TooLong(20)}}
	if !equalErrors(errs, want) {
		t.Errorf("Errors = %v, want %v", errs, want)
	}
}

// Errors added rather than nested still move under the field, every one of
// them.
func TestErrors_Add_Errors(t *testing.T) {
	t.Parallel()

	var inner validator.Errors
	inner.Add(validator.Field("zip"), validator.Blank())
	inner.Add(validator.Field("city"), validator.TooLong(3))

	var errs validator.Errors
	errs.Add(validator.Field("address"), inner.Err())

	want := validator.Errors{
		{Field: "address.zip", Err: validator.Blank()},
		{Field: "address.city", Err: validator.TooLong(3)},
	}
	if !equalErrors(errs, want) {
		t.Errorf("Errors = %v, want %v", errs, want)
	}
}

func TestErrors_Nest(t *testing.T) {
	t.Parallel()

	inner := validator.Errors{{Field: "zip", Err: validator.Blank()}, {Field: "city", Err: validator.TooLong(30)}}

	tests := []struct {
		name  string
		field validator.Path
		err   error
		want  validator.Errors
	}{
		{name: "nil", field: validator.Field("address"), err: nil, want: nil},
		{
			name:  "Errors move under the field",
			field: validator.Field("address"),
			err:   inner,
			want: validator.Errors{
				{Field: "address.zip", Err: validator.Blank()},
				{Field: "address.city", Err: validator.TooLong(30)},
			},
		},
		{
			// An embedded struct without a JSON name keeps its fields at the
			// level of the struct embedding it.
			name:  "an empty field keeps the paths",
			field: validator.Path{},
			err:   inner,
			want:  inner,
		},
		{
			name:  "element of a slice",
			field: validator.Field("items").Index(2),
			err:   validator.Errors{{Field: "sku", Err: validator.Blank()}},
			want:  validator.Errors{{Field: "items[2].sku", Err: validator.Blank()}},
		},
		{
			name:  "a Coded error lands on the field",
			field: validator.Field("email"),
			err:   &typeError{code: "email"},
			want:  validator.Errors{{Field: "email", Err: &typeError{code: "email"}}},
		},
		{
			name:  "a plain error lands on the field",
			field: validator.Field("address"),
			err:   errors.New("lookup failed"),
			want:  validator.Errors{{Field: "address", Err: errors.New("lookup failed")}},
		},
		{
			name:  "an error without a field lands on base",
			field: validator.Path{},
			err:   errors.New("lookup failed"),
			want:  validator.Errors{{Field: "base", Err: errors.New("lookup failed")}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got validator.Errors
			got.Nest(tt.field, tt.err)
			if !equalErrors(got, tt.want) {
				t.Errorf("Nest(%s) = %v, want %v", tt.field, got, tt.want)
			}
		})
	}
}

// level has a String method, which encoding/json does not name a key by.
type level int

func (l level) String() string { return "level " + strconv.Itoa(int(l)) }

// span names a key by its text, as encoding/json does.
type span struct{ from, to int }

func (s span) MarshalText() ([]byte, error) {
	return []byte(strconv.Itoa(s.from) + "-" + strconv.Itoa(s.to)), nil
}

func TestPaths(t *testing.T) {
	t.Parallel()

	field := func(name string) validator.Segment {
		return validator.Segment{Kind: validator.FieldSegment, Name: name}
	}
	key := func(name string) validator.Segment { return validator.Segment{Kind: validator.KeySegment, Name: name} }
	index := func(i int) validator.Segment { return validator.Segment{Kind: validator.IndexSegment, Index: i} }

	tests := []struct {
		path     validator.Path
		want     string
		segments []validator.Segment
	}{
		{path: validator.Field("name"), want: "name", segments: []validator.Segment{field("name")}},
		{
			path:     validator.Field("address", "zip"),
			want:     "address.zip",
			segments: []validator.Segment{field("address"), field("zip")},
		},
		{path: validator.Base, want: "base", segments: []validator.Segment{field("base")}},
		{path: validator.Path{}, want: "", segments: nil},
		{
			path:     validator.Field("tags").Index(2),
			want:     "tags[2]",
			segments: []validator.Segment{field("tags"), index(2)},
		},
		{
			path:     validator.Field("labels").Key("primary"),
			want:     "labels.primary",
			segments: []validator.Segment{field("labels"), key("primary")},
		},
		{
			path:     validator.Field("scores").Key(7),
			want:     "scores.7",
			segments: []validator.Segment{field("scores"), key("7")},
		},
		{
			path:     validator.Field("groups").Key("admin").Index(0),
			want:     "groups.admin[0]",
			segments: []validator.Segment{field("groups"), key("admin"), index(0)},
		},
		{
			path:     validator.Field("levels").Key(level(1)),
			want:     "levels.1",
			segments: []validator.Segment{field("levels"), key("1")},
		},
		{
			path:     validator.Field("ranges").Key(span{1, 3}),
			want:     "ranges.1-3",
			segments: []validator.Segment{field("ranges"), key("1-3")},
		},
		{
			// Key takes any, and names a key by its own type as a map of that
			// type of keys does.
			path:     validator.Field("ranges").Key(any(span{1, 3})),
			want:     "ranges.1-3",
			segments: []validator.Segment{field("ranges"), key("1-3")},
		},
		{
			path:     validator.Field("flags").Key(true),
			want:     "flags.true",
			segments: []validator.Segment{field("flags"), key("true")},
		},
		{path: validator.Path{}.Index(1), want: "[1]", segments: []validator.Segment{index(1)}},
		{
			path:     validator.Field("items").Index(2).Field("sku"),
			want:     "items[2].sku",
			segments: []validator.Segment{field("items"), index(2), field("sku")},
		},
		{
			path:     validator.Field("groups").Key("admin").Field("owner", "name"),
			want:     "groups.admin.owner.name",
			segments: []validator.Segment{field("groups"), key("admin"), field("owner"), field("name")},
		},
		{
			path:     validator.Field("address").Field("zip"),
			want:     "address.zip",
			segments: []validator.Segment{field("address"), field("zip")},
		},
		{path: validator.Path{}.Field("name"), want: "name", segments: []validator.Segment{field("name")}},
	}
	for _, tt := range tests {
		if got := tt.path.String(); got != tt.want {
			t.Errorf("path = %q, want %q", got, tt.want)
		}
		if got := tt.path.Segments(); !slices.Equal(got, tt.segments) {
			t.Errorf("segments of %s = %v, want %v", tt.want, got, tt.segments)
		}
	}
}

// Paths that share a start do not share what follows it.
func TestPaths_Independent(t *testing.T) {
	t.Parallel()

	tags := validator.Field("tags").Index(0)
	a, b := tags.Index(1), tags.Index(2)
	if a.String() != "tags[0][1]" || b.String() != "tags[0][2]" {
		t.Errorf("paths = %s and %s, want tags[0][1] and tags[0][2]", a, b)
	}
	item := validator.Field("items").Index(0)
	c, d := item.Field("sku"), item.Field("name")
	if c.String() != "items[0].sku" || d.String() != "items[0].name" {
		t.Errorf("paths = %s and %s, want items[0].sku and items[0].name", c, d)
	}
}

// The segments of an error tell a map key from a field, which its path spelled
// the same, and Nest keeps them as it moves the error.
func TestErrors_Segments(t *testing.T) {
	t.Parallel()

	var item validator.Errors
	item.Add(validator.Field("sku"), validator.Blank())
	item.Add(validator.Field("notes").Key("first.note"), validator.TooLong(3))

	var errs validator.Errors
	errs.Nest(validator.Field("items").Index(2), item.Err())
	errs.Nest(validator.Field("tags"), validator.Errors{{
		Field:    "[1]",
		Segments: []validator.Segment{{Kind: validator.IndexSegment, Index: 1}},
		Err:      validator.Blank(),
	}})

	want := []struct {
		field    string
		segments []validator.Segment
	}{
		{field: "items[2].sku", segments: []validator.Segment{
			{Kind: validator.FieldSegment, Name: "items"},
			{Kind: validator.IndexSegment, Index: 2},
			{Kind: validator.FieldSegment, Name: "sku"},
		}},
		{field: "items[2].notes.first.note", segments: []validator.Segment{
			{Kind: validator.FieldSegment, Name: "items"},
			{Kind: validator.IndexSegment, Index: 2},
			{Kind: validator.FieldSegment, Name: "notes"},
			{Kind: validator.KeySegment, Name: "first.note"},
		}},
		{field: "tags[1]", segments: []validator.Segment{
			{Kind: validator.FieldSegment, Name: "tags"},
			{Kind: validator.IndexSegment, Index: 1},
		}},
	}
	if len(errs) != len(want) {
		t.Fatalf("Errors = %v, want %d of them", errs, len(want))
	}
	for i, w := range want {
		if errs[i].Field != w.field || !slices.Equal(errs[i].Segments, w.segments) {
			t.Errorf("error %d = %q %v, want %q %v", i, errs[i].Field, errs[i].Segments, w.field, w.segments)
		}
	}
}

// FieldNames keeps the fields of a path and leaves out its keys and indexes.
func TestFieldError_FieldNames(t *testing.T) {
	t.Parallel()

	var item validator.Errors
	item.Add(validator.Field("notes").Key("first_note"), validator.TooLong(3))
	var nested validator.Errors
	nested.Nest(validator.Field("items").Index(2), item.Err())

	tests := []struct {
		name string
		err  validator.FieldError
		want []string
	}{
		{name: "fields", err: fieldError(validator.Field("address", "zip")), want: []string{"address", "zip"}},
		{
			name: "keys and indexes",
			err:  fieldError(validator.Field("groups").Key("admin").Index(0)),
			want: []string{"groups"},
		},
		{name: "nested", err: nested[0], want: []string{"items", "notes"}},
		{name: "no segments", err: validator.FieldError{Field: "name", Err: validator.Blank()}},
	}
	for _, tt := range tests {
		got := tt.err.FieldNames()
		if !slices.Equal(got, tt.want) || (tt.want == nil) != (got == nil) {
			t.Errorf("%s: FieldNames() = %#v, want %#v", tt.name, got, tt.want)
		}
	}
}

// The names FieldNames returns are the caller's to change.
func TestFieldError_FieldNames_Copy(t *testing.T) {
	t.Parallel()

	fe := fieldError(validator.Field("address", "zip"))
	fe.FieldNames()[0] = "changed"
	if got := fe.FieldNames(); !slices.Equal(got, []string{"address", "zip"}) {
		t.Errorf("FieldNames() = %v after changing a returned slice, want [address zip]", got)
	}
}

func fieldError(p validator.Path) validator.FieldError {
	var errs validator.Errors
	errs.Add(p, validator.Blank())
	return errs[0]
}

func TestSortedKeys(t *testing.T) {
	t.Parallel()

	m := map[span]int{{10, 20}: 0, {2, 3}: 0, {1, 5}: 0}
	want := []span{{1, 5}, {10, 20}, {2, 3}}
	for range 20 {
		if got := validator.SortedKeys(m); !slices.Equal(got, want) {
			t.Fatalf("SortedKeys() = %v, want %v", got, want)
		}
	}
}

func TestErrors_Err(t *testing.T) {
	t.Parallel()

	// A nil error and not an Errors holding nothing, which would compare
	// non-nil.
	if err := (validator.Errors{}).Err(); err != nil {
		t.Errorf("Err() of no errors = %v, want nil", err)
	}

	errs := validator.Errors{{Field: "name", Err: validator.Blank()}}
	got, ok := errors.AsType[validator.Errors](errs.Err())
	if !ok || !equalErrors(got, errs) {
		t.Errorf("Err() = %v, want the errors themselves", errs.Err())
	}
}

func TestErrors_Error(t *testing.T) {
	t.Parallel()

	errs := validator.Errors{
		{Field: "name", Err: validator.Blank()},
		{Field: "tags[2]", Err: validator.TooLong(20)},
		{Field: "address", Err: errors.New("lookup failed")},
	}

	want := "validator: name: blank; tags[2]: too_long (count=20); address: lookup failed"
	if got := errs.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrors_UnwrapReachesTheRulesError(t *testing.T) {
	t.Parallel()

	errs := validator.Errors{{Field: "name", Err: validator.Blank()}, {Field: "email", Err: &typeError{code: "email"}}}

	if got, ok := errors.AsType[*typeError](errs.Err()); !ok || got.code != "email" {
		t.Errorf("errors.AsType found %v, %v, want the email error", got, ok)
	}
}

// FieldError is what go-kanna/i18n renders, through the methods it asks for.
var _ interface {
	Path() string
	Code() string
	Params() map[string]any
} = validator.FieldError{}

func TestFieldError_Path(t *testing.T) {
	t.Parallel()

	if got := (validator.FieldError{Field: "tags[1]"}).Path(); got != "tags[1]" {
		t.Errorf("Path() = %q, want %q", got, "tags[1]")
	}
}

func TestFieldError_Code(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantCode   string
		wantParams map[string]any
	}{
		{name: "built-in", err: validator.TooLong(20), wantCode: "too_long", wantParams: map[string]any{"count": 20}},
		{name: "Coded from elsewhere", err: &typeError{code: "email"}, wantCode: "email"},
		{name: "not Coded", err: errors.New("lookup failed"), wantCode: "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fe := validator.FieldError{Field: "x", Err: tt.err}
			if fe.Code() != tt.wantCode || !maps.Equal(fe.Params(), tt.wantParams) {
				t.Errorf("Code(), Params() = %q, %v, want %q, %v", fe.Code(), fe.Params(), tt.wantCode, tt.wantParams)
			}
			if !errors.Is(fe, tt.err) {
				t.Error("errors.Is does not reach Err")
			}
		})
	}
}

func TestErrors_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		errs validator.Errors
		want string
	}{
		{name: "nil", errs: nil, want: `{}`},
		{
			name: "fields in the order they first broke",
			errs: validator.Errors{
				{Field: "password", Err: validator.Blank()},
				{Field: "name", Err: validator.Blank()},
				{Field: "password", Err: validator.TooShort(12)},
			},
			want: `{"password":[{"error":"blank"},{"error":"too_short","count":12}],"name":[{"error":"blank"}]}`,
		},
		{
			name: "params in key order beside the code",
			errs: validator.Errors{{Field: "confirm", Err: validator.FailWith("between", map[string]any{"min": 1, "max": 9})}},
			want: `{"confirm":[{"error":"between","max":9,"min":1}]}`,
		},
		{
			// encoding/json/v2 has no default encoding of a time.Duration.
			name: "a time.Duration as it is written in a tag",
			errs: validator.Errors{{Field: "timeout", Err: validator.GreaterThanOrEqualTo(time.Second)}},
			want: `{"timeout":[{"error":"greater_than_or_equal_to","count":"1s"}]}`,
		},
		{
			name: "a param that is a map, in key order too",
			errs: validator.Errors{{
				Field: "x",
				Err:   validator.FailWith("odd", map[string]any{"range": map[string]int{"to": 9, "from": 1}}),
			}},
			want: `{"x":[{"error":"odd","range":{"from":1,"to":9}}]}`,
		},
		{
			name: "a param named error is left out",
			errs: validator.Errors{{Field: "x", Err: validator.FailWith("odd", map[string]any{"error": "shadow"})}},
			want: `{"x":[{"error":"odd"}]}`,
		},
		{
			name: "an error that is not Coded",
			errs: validator.Errors{{Field: "address", Err: errors.New("lookup failed")}},
			want: `{"address":[{"error":"invalid"}]}`,
		},
		{
			// A map key spells part of the path, and may be any bytes.
			name: "invalid UTF-8 in a field",
			errs: validator.Errors{{Field: "labels.\xff", Err: validator.Blank()}},
			want: `{"labels.` + "�" + `":[{"error":"blank"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v1, err := jsonv1.Marshal(tt.errs)
			if err != nil {
				t.Fatalf("encoding/json: %v", err)
			}
			v2, err := json.Marshal(tt.errs)
			if err != nil {
				t.Fatalf("encoding/json/v2: %v", err)
			}
			// What an encoder that knows only json.Marshaler gets.
			direct, err := tt.errs.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if string(v1) != tt.want || string(v2) != tt.want || string(direct) != tt.want {
				t.Errorf("encoding/json gives %s, encoding/json/v2 %s, and MarshalJSON %s, want %s", v1, v2, direct, tt.want)
			}
		})
	}
}

// failingWriter refuses every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestErrors_MarshalJSONTo_ReportsTheWriter(t *testing.T) {
	t.Parallel()

	errs := validator.Errors{{Field: "name", Err: validator.Blank()}}

	err := errs.MarshalJSONTo(jsontext.NewEncoder(failingWriter{}))
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("MarshalJSONTo() error = %v, want the writer's", err)
	}
}

func TestErrors_MarshalJSON_UnencodableParam(t *testing.T) {
	t.Parallel()

	errs := validator.Errors{{Field: "x", Err: validator.FailWith("odd", map[string]any{"f": func() {}})}}

	_, err := json.Marshal(errs)
	if err == nil || !strings.Contains(err.Error(), `param "f" of x`) {
		t.Errorf("Marshal() error = %v, want one naming the param and its field", err)
	}
}
