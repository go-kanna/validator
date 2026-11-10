package validator_test

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-kanna/validator"
)

// email stands in for a value type of kanna-value: its UnmarshalJSON rejects a
// value with a Coded error and leaves the field as it was.
type email struct{ v string }

func (e *email) UnmarshalJSON(data []byte) error {
	var s string
	if err := jsonv1.Unmarshal(data, &s); err != nil {
		return err //nolint:wrapcheck // the test expects encoding/json's own error back
	}
	if !strings.Contains(s, "@") {
		return &typeError{code: "email"}
	}
	e.v = s
	return nil
}

// role stands in for an enum that checks itself in UnmarshalText.
type role string

func (r *role) UnmarshalText(text []byte) error {
	if s := role(text); s == "admin" || s == "member" {
		*r = s
		return nil
	}
	return validator.Inclusion()
}

// Validate finds nothing: only decoding is under test where a role is decoded
// alone.
func (role) Validate() error { return nil }

// period stands in for a type written for encoding/json/v2, which decodes
// itself from the decoder. It rejects a period that ends before it starts.
type period struct {
	From int `json:"from"`
	To   int `json:"to"`
}

func (p *period) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	type plain period
	var v plain
	if err := json.UnmarshalDecode(dec, &v); err != nil {
		return err //nolint:wrapcheck // encoding/json/v2 reports it with its position
	}
	if v.To < v.From {
		return validator.Fail("backwards")
	}
	*p = period(v)
	return nil
}

// Validate stands in for `validate:"required"` on both ends.
func (p period) Validate() error {
	var errs validator.Errors
	if p.From == 0 {
		errs.Add(validator.Field("from"), validator.Blank())
	}
	if p.To == 0 {
		errs.Add(validator.Field("to"), validator.Blank())
	}
	return errs.Err()
}

type address struct {
	Zip email `json:"zip"`
}

type signUp struct {
	Email   email               `json:"email"`
	Name    string              `json:"name"`
	Role    role                `json:"role"`
	Stay    period              `json:"stay"`
	Address address             `json:"address"`
	Backups []email             `json:"backups"`
	Labels  map[string]email    `json:"labels"`
	Groups  map[string]*address `json:"groups"`
}

// Validate stands in for what kanna-validator generates for
// `validate:"required"` on Email, Name, and Role, and for the nested Stay.
func (s signUp) Validate() error {
	var errs validator.Errors
	if s.Email.v == "" {
		errs.Add(validator.Field("email"), validator.Blank())
	}
	if s.Name == "" {
		errs.Add(validator.Field("name"), validator.Blank())
	}
	if s.Role == "" {
		errs.Add(validator.Field("role"), validator.Blank())
	}
	errs.Nest(validator.Field("stay"), s.Stay.Validate())
	return errs.Err()
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want validator.Errors
	}{
		{
			name: "valid",
			in:   `{"email":"a@example.com","name":"kanna","role":"admin","stay":{"from":1,"to":2}}`,
			want: nil,
		},
		{
			name: "every rejection with its path, then what Validate finds",
			in: `{"email":"x","name":"","role":"root","stay":{"from":2,"to":1},"address":{"zip":"y"},` +
				`"backups":["a@example.com","z"],"labels":{"home":"w"}}`,
			want: validator.Errors{
				{Field: "email", Err: &typeError{code: "email"}},
				{Field: "role", Err: validator.Inclusion()},
				{Field: "stay", Err: validator.Fail("backwards")},
				{Field: "address.zip", Err: &typeError{code: "email"}},
				{Field: "backups[1]", Err: &typeError{code: "email"}},
				{Field: "labels.home", Err: &typeError{code: "email"}},
				{Field: "name", Err: validator.Blank()},
			},
		},
		{
			name: "Validate on fields that were never sent",
			in:   `{"name":"kanna","stay":{"to":2}}`,
			want: validator.Errors{
				{Field: "email", Err: validator.Blank()},
				{Field: "role", Err: validator.Blank()},
				{Field: "stay.from", Err: validator.Blank()},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got signUp
			err := validator.DecodeJSON(strings.NewReader(tt.in), &got)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("DecodeJSON() error = %v, want nil", err)
				}
				return
			}

			errs, ok := errors.AsType[validator.Errors](err)
			if !ok || !equalErrors(errs, tt.want) {
				t.Errorf("DecodeJSON() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// The segments of a rejection tell the keys of a map from the fields of a
// struct, which its JSON does not.
func TestDecodeJSON_Segments(t *testing.T) {
	t.Parallel()

	in := `{"email":"a@example.com","name":"kanna","role":"admin","stay":{"from":1,"to":2},"address":{"zip":"x"},` +
		`"backups":["y"],"labels":{"zip":"z"},"groups":{"home.office":{"zip":"w"}}}`
	err := validator.DecodeJSON(strings.NewReader(in), new(signUp))
	errs, ok := errors.AsType[validator.Errors](err)
	if !ok {
		t.Fatalf("DecodeJSON() error = %v, want Errors", err)
	}

	field := func(name string) validator.Segment {
		return validator.Segment{Kind: validator.FieldSegment, Name: name}
	}
	key := func(name string) validator.Segment { return validator.Segment{Kind: validator.KeySegment, Name: name} }
	want := map[string][]validator.Segment{
		"address.zip":            {field("address"), field("zip")},
		"backups[0]":             {field("backups"), {Kind: validator.IndexSegment, Index: 0}},
		"labels.zip":             {field("labels"), key("zip")},
		"groups.home.office.zip": {field("groups"), key("home.office"), field("zip")},
	}
	if len(errs) != len(want) {
		t.Fatalf("Errors = %v, want %d of them", errs, len(want))
	}
	for _, fe := range errs {
		if w, ok := want[fe.Field]; !ok || !slices.Equal(fe.Segments, w) {
			t.Errorf("segments of %s = %v, want %v", fe.Field, fe.Segments, w)
		}
	}
}

func TestDecodeJSON_KeepsDecoding(t *testing.T) {
	t.Parallel()

	got := signUp{Email: email{v: "old@example.com"}}
	if err := validator.DecodeJSON(strings.NewReader(`{"email":"x","name":"kanna"}`), &got); err == nil {
		t.Fatal("DecodeJSON() error = nil, want the rejected email")
	}
	if got.Email.v != "old@example.com" {
		t.Errorf("Email = %q, want it left at %q", got.Email.v, "old@example.com")
	}
	if got.Name != "kanna" {
		t.Errorf("Name = %q, want %q decoded after the rejected email", got.Name, "kanna")
	}
}

type mailbox struct {
	Email       email `json:"email"`
	EmailBackup email `json:"email_backup"`
}

// Validate stands in for `validate:"required"` on both.
func (m mailbox) Validate() error {
	var errs validator.Errors
	if m.Email.v == "" {
		errs.Add(validator.Field("email"), validator.Blank())
	}
	if m.EmailBackup.v == "" {
		errs.Add(validator.Field("email_backup"), validator.Blank())
	}
	return errs.Err()
}

func TestDecodeJSON_KeepsWhatIsBesideARejection(t *testing.T) {
	t.Parallel()

	var got mailbox
	err := validator.DecodeJSON(strings.NewReader(`{"email":"x"}`), &got)

	// email_backup starts like email but does not lie under it.
	want := validator.Errors{
		{Field: "email", Err: &typeError{code: "email"}},
		{Field: "email_backup", Err: validator.Blank()},
	}
	if errs, ok := errors.AsType[validator.Errors](err); !ok || !equalErrors(errs, want) {
		t.Errorf("DecodeJSON() error = %v, want %v", err, want)
	}
}

func TestDecodeJSON_TopLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    interface{ Validate() error }
		in   string
		want validator.Errors
	}{
		{
			name: "rejected while decoding",
			v:    new(role),
			in:   `"root"`,
			want: validator.Errors{{Field: "base", Err: validator.Inclusion()}},
		},
		{
			// Nothing was set, so whatever Validate finds is left out.
			name: "with Validate",
			v:    new(period),
			in:   `{"from":2,"to":1}`,
			want: validator.Errors{{Field: "base", Err: validator.Fail("backwards")}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validator.DecodeJSON(strings.NewReader(tt.in), tt.v)
			if errs, ok := errors.AsType[validator.Errors](err); !ok || !equalErrors(errs, tt.want) {
				t.Errorf("DecodeJSON() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestDecodeJSON_LeavesTimeToEncodingJSON(t *testing.T) {
	t.Parallel()

	// encoding/json/v2 sets a time to zero on null, where Time.UnmarshalJSON
	// would leave it as it was.
	got := stamped{At: time.Unix(1, 0)}
	if err := validator.DecodeJSON(strings.NewReader(`{"at":null}`), &got); err != nil {
		t.Fatalf("DecodeJSON() error = %v, want nil", err)
	}
	if !got.At.IsZero() {
		t.Errorf("At = %v, want the zero time", got.At)
	}
}

// stamped holds a time, with nothing to validate.
type stamped struct {
	At time.Time `json:"at"`
}

func (stamped) Validate() error { return nil }

// roleCounts is a map keyed by roles, with nothing to validate.
type roleCounts map[role]int

func (roleCounts) Validate() error { return nil }

// partway holds a value rejected before it is read whole, with nothing to
// validate.
type partway struct{ X impatient }

func (partway) Validate() error { return nil }

// impatient rejects a value before it has read all of it.
type impatient struct{}

func (*impatient) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if _, err := dec.ReadToken(); err != nil {
		return err //nolint:wrapcheck // encoding/json/v2 reports it with its position
	}
	return validator.Invalid()
}

// failing fails every read with what it holds.
type failing struct{ err error }

func (f failing) Read([]byte) (int, error) { return 0, f.err }

// dated holds a time and a small number, with nothing to validate.
type dated struct {
	At    time.Time `json:"at"`
	Small int8      `json:"small"`
	Stay  struct{}  `json:"stay"`
}

func (dated) Validate() error { return nil }

func TestDecodeJSON_Unreadable(t *testing.T) {
	t.Parallel()

	tooLarge := http.MaxBytesReader(httptest.NewRecorder(), io.NopCloser(strings.NewReader(`{"name":"kanna"}`)), 8)

	tests := []struct {
		name string
		v    interface{ Validate() error }
		in   io.Reader
		want validator.DecodeError
		as   func(error) bool
		msg  string
	}{
		{
			name: "empty",
			v:    new(signUp),
			in:   strings.NewReader(``),
			want: validator.DecodeError{Reason: validator.ReasonSyntax},
			as:   func(err error) bool { return errors.Is(err, io.ErrUnexpectedEOF) },
			msg:  "validator: cannot decode JSON: syntax",
		},
		{
			name: "malformed",
			v:    new(signUp),
			in:   strings.NewReader(`{"email":`),
			want: validator.DecodeError{Pointer: "/email", Reason: validator.ReasonSyntax},
			as:   isError[*jsontext.SyntacticError],
			msg:  `validator: cannot decode JSON at "/email": syntax`,
		},
		{
			name: "more than one value",
			v:    new(signUp),
			in:   strings.NewReader(`{"name":"kanna"} {}`),
			want: validator.DecodeError{Reason: validator.ReasonTrailing},
			as:   func(error) bool { return true },
			msg:  "validator: cannot decode JSON: trailing",
		},
		{
			name: "garbage after the value",
			v:    new(signUp),
			in:   strings.NewReader(`{"name":"kanna"} secret`),
			want: validator.DecodeError{Reason: validator.ReasonTrailing},
			as:   isError[*jsontext.SyntacticError],
			msg:  "validator: cannot decode JSON: trailing",
		},
		{
			name: "a duplicate name",
			v:    new(signUp),
			in:   strings.NewReader(`{"name":"kanna","name":"secret"}`),
			want: validator.DecodeError{Pointer: "/name", Reason: validator.ReasonDuplicateName},
			as:   isError[*jsontext.SyntacticError],
			msg:  `validator: cannot decode JSON at "/name": duplicate_name`,
		},
		{
			name: "does not fit the Go type",
			v:    new(signUp),
			in:   strings.NewReader(`{"name":1}`),
			want: validator.DecodeError{Pointer: "/name", Reason: validator.ReasonValue},
			as:   isError[*json.SemanticError],
			msg:  `validator: cannot decode JSON at "/name": value`,
		},
		{
			name: "a string for a struct",
			v:    new(dated),
			in:   strings.NewReader(`{"stay":"secret"}`),
			want: validator.DecodeError{Pointer: "/stay", Reason: validator.ReasonValue},
			as:   isError[*json.SemanticError],
			msg:  `validator: cannot decode JSON at "/stay": value`,
		},
		{
			name: "not a time",
			v:    new(dated),
			in:   strings.NewReader(`{"at":"1990-13-01T00:00:00Z"}`),
			want: validator.DecodeError{Pointer: "/at", Reason: validator.ReasonValue},
			as:   isError[*time.ParseError],
			msg:  `validator: cannot decode JSON at "/at": value`,
		},
		{
			name: "out of range",
			v:    new(dated),
			in:   strings.NewReader(`{"small":300}`),
			want: validator.DecodeError{Pointer: "/small", Reason: validator.ReasonValue},
			as:   isError[*json.SemanticError],
			msg:  `validator: cannot decode JSON at "/small": value`,
		},
		{
			name: "does not fit the type behind an UnmarshalJSON",
			v:    new(signUp),
			in:   strings.NewReader(`{"email":1}`),
			want: validator.DecodeError{Pointer: "/email", Reason: validator.ReasonValue},
			as:   isError[*jsonv1.UnmarshalTypeError],
			msg:  `validator: cannot decode JSON at "/email": value`,
		},
		{
			name: "not a string for an UnmarshalText",
			v:    new(signUp),
			in:   strings.NewReader(`{"role":1}`),
			want: validator.DecodeError{Pointer: "/role", Reason: validator.ReasonValue},
			as:   isError[*json.SemanticError],
			msg:  `validator: cannot decode JSON at "/role": value`,
		},
		{
			name: "a rejected map key",
			v:    new(roleCounts),
			in:   strings.NewReader(`{"secret":1}`),
			want: validator.DecodeError{Pointer: "/secret", Reason: validator.ReasonValue},
			as:   isError[*validator.Error],
			msg:  `validator: cannot decode JSON at "/secret": value`,
		},
		{
			name: "a rejection partway through a value",
			v:    new(partway),
			in:   strings.NewReader(`{"X":[1,2]}`),
			want: validator.DecodeError{Pointer: "/X", Reason: validator.ReasonValue},
			as:   isError[*json.SemanticError],
			msg:  `validator: cannot decode JSON at "/X": value`,
		},
		{
			name: "a name that needs quoting",
			v:    new(roleCounts),
			in:   strings.NewReader(`{"a\n\"b":1}`),
			want: validator.DecodeError{Pointer: "/a\n\"b", Reason: validator.ReasonValue},
			as:   isError[*json.SemanticError],
			msg:  `validator: cannot decode JSON at "/a\n\"b": value`,
		},
		{
			name: "failing to read",
			v:    new(signUp),
			in:   failing{errors.New("secret")},
			want: validator.DecodeError{Reason: validator.ReasonRead},
			as:   func(err error) bool { return strings.Contains(fmt.Sprint(errors.Unwrap(err)), "secret") },
			msg:  "validator: cannot decode JSON: read",
		},
		{
			name: "too large",
			v:    new(signUp),
			in:   tooLarge,
			want: validator.DecodeError{Reason: validator.ReasonRead},
			as:   isError[*http.MaxBytesError],
			msg:  "validator: cannot decode JSON: read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validator.DecodeJSON(tt.in, tt.v)
			got, ok := errors.AsType[*validator.DecodeError](err)
			if !ok {
				t.Fatalf("DecodeJSON() error = %v (%T), want a *DecodeError", err, err)
			}
			if got.Pointer != tt.want.Pointer || got.Reason != tt.want.Reason {
				t.Errorf("DecodeJSON() error = {%q, %q}, want {%q, %q}", got.Pointer, got.Reason, tt.want.Pointer, tt.want.Reason)
			}
			if !tt.as(err) {
				t.Errorf("DecodeJSON() error wraps %v, want encoding/json's own error", errors.Unwrap(err))
			}
			if msg := err.Error(); msg != tt.msg {
				t.Errorf("Error() = %q, want %q", msg, tt.msg)
			}
		})
	}
}

func TestDecodeError_JSON(t *testing.T) {
	t.Parallel()

	err := validator.DecodeJSON(strings.NewReader(`{"stay":"secret"}`), new(dated))
	want := `{"error":{"pointer":"/stay","reason":"value"}}`
	for name, marshal := range map[string]func(any) ([]byte, error){
		"encoding/json":    jsonv1.Marshal,
		"encoding/json/v2": func(v any) ([]byte, error) { return json.Marshal(v) },
	} {
		got, merr := marshal(map[string]error{"error": err})
		if merr != nil || string(got) != want {
			t.Errorf("%s: Marshal() = %s, %v, want %s", name, got, merr, want)
		}
	}

	top := validator.DecodeJSON(strings.NewReader(``), new(dated))
	if got, merr := jsonv1.Marshal(top); merr != nil || string(got) != `{"pointer":"","reason":"syntax"}` {
		t.Errorf("Marshal() = %s, %v, want the pointer to the top-level value", got, merr)
	}
}

func isError[E error](err error) bool {
	_, ok := errors.AsType[E](err)
	return ok
}
