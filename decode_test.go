package validator_test

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
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
		errs.Add("from", validator.Blank())
	}
	if p.To == 0 {
		errs.Add("to", validator.Blank())
	}
	return errs.Err()
}

type address struct {
	Zip email `json:"zip"`
}

type signUp struct {
	Email   email            `json:"email"`
	Name    string           `json:"name"`
	Role    role             `json:"role"`
	Stay    period           `json:"stay"`
	Address address          `json:"address"`
	Backups []email          `json:"backups"`
	Labels  map[string]email `json:"labels"`
}

// Validate stands in for what kanna-validator generates for
// `validate:"required"` on Email, Name, and Role, and for the nested Stay.
func (s signUp) Validate() error {
	var errs validator.Errors
	if s.Email.v == "" {
		errs.Add("email", validator.Blank())
	}
	if s.Name == "" {
		errs.Add("name", validator.Blank())
	}
	if s.Role == "" {
		errs.Add("role", validator.Blank())
	}
	errs.Nest("stay", s.Stay.Validate())
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
		errs.Add("email", validator.Blank())
	}
	if m.EmailBackup.v == "" {
		errs.Add("email_backup", validator.Blank())
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
			want: validator.Errors{{Field: validator.Base, Err: validator.Inclusion()}},
		},
		{
			// Nothing was set, so whatever Validate finds is left out.
			name: "with Validate",
			v:    new(period),
			in:   `{"from":2,"to":1}`,
			want: validator.Errors{{Field: validator.Base, Err: validator.Fail("backwards")}},
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

func TestDecodeJSON_Unreadable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    interface{ Validate() error }
		in   string
		as   func(error) bool
	}{
		{
			name: "malformed",
			v:    new(signUp),
			in:   `{"email":`,
			as:   isError[*jsontext.SyntacticError],
		},
		{
			name: "more than one value",
			v:    new(signUp),
			in:   `{"name":"kanna"} {}`,
			as:   isError[*jsontext.SyntacticError],
		},
		{
			name: "does not fit the Go type",
			v:    new(signUp),
			in:   `{"name":1}`,
			as:   isError[*json.SemanticError],
		},
		{
			name: "does not fit the type behind an UnmarshalJSON",
			v:    new(signUp),
			in:   `{"email":1}`,
			as:   isError[*jsonv1.UnmarshalTypeError],
		},
		{
			name: "not a string for an UnmarshalText",
			v:    new(signUp),
			in:   `{"role":1}`,
			as:   isError[*json.SemanticError],
		},
		{
			name: "a rejected map key",
			v:    new(roleCounts),
			in:   `{"root":1}`,
			as:   isError[*json.SemanticError],
		},
		{
			name: "a rejection partway through a value",
			v:    new(partway),
			in:   `{"X":[1,2]}`,
			as:   isError[*json.SemanticError],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validator.DecodeJSON(strings.NewReader(tt.in), tt.v)
			if _, ok := errors.AsType[validator.Errors](err); ok || !tt.as(err) {
				t.Errorf("DecodeJSON() error = %v (%T), want encoding/json's own error", err, err)
			}
		})
	}
}

func isError[E error](err error) bool {
	_, ok := errors.AsType[E](err)
	return ok
}
