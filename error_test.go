package validator_test

import (
	"maps"
	"testing"
	"time"

	"github.com/go-kanna/validator"
)

var _ validator.Coded = validator.Fail("x")

func TestBuiltInErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        *validator.Error
		wantCode   string
		wantParams map[string]any
	}{
		{name: "Blank", err: validator.Blank(), wantCode: "blank"},
		{name: "TooShort", err: validator.TooShort(3), wantCode: "too_short", wantParams: map[string]any{"count": 3}},
		{name: "TooLong", err: validator.TooLong(50), wantCode: "too_long", wantParams: map[string]any{"count": 50}},
		{
			name:       "WrongLength",
			err:        validator.WrongLength(7),
			wantCode:   "wrong_length",
			wantParams: map[string]any{"count": 7},
		},
		// A numeric count keeps the type of the field it was checked on.
		{
			name:       "GreaterThan",
			err:        validator.GreaterThan(0),
			wantCode:   "greater_than",
			wantParams: map[string]any{"count": 0},
		},
		{
			name:       "GreaterThanOrEqualTo",
			err:        validator.GreaterThanOrEqualTo(int64(18)),
			wantCode:   "greater_than_or_equal_to",
			wantParams: map[string]any{"count": int64(18)},
		},
		{name: "LessThan", err: validator.LessThan(1.5), wantCode: "less_than", wantParams: map[string]any{"count": 1.5}},
		{
			name:       "LessThanOrEqualTo",
			err:        validator.LessThanOrEqualTo(uint8(130)),
			wantCode:   "less_than_or_equal_to",
			wantParams: map[string]any{"count": uint8(130)},
		},
		{name: "EqualTo", err: validator.EqualTo(1), wantCode: "equal_to", wantParams: map[string]any{"count": 1}},
		// A time.Duration counts as it is written in a tag.
		{
			name:       "a time.Duration",
			err:        validator.GreaterThanOrEqualTo(90 * time.Second),
			wantCode:   "greater_than_or_equal_to",
			wantParams: map[string]any{"count": "1m30s"},
		},
		{name: "OtherThan", err: validator.OtherThan(1), wantCode: "other_than", wantParams: map[string]any{"count": 1}},
		{name: "Inclusion", err: validator.Inclusion(), wantCode: "inclusion"},
		{
			name:       "Confirmation",
			err:        validator.Confirmation("password"),
			wantCode:   "confirmation",
			wantParams: map[string]any{"attribute": "password"},
		},
		{name: "Invalid", err: validator.Invalid(), wantCode: "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.err.Code(); got != tt.wantCode {
				t.Errorf("Code() = %q, want %q", got, tt.wantCode)
			}
			if got := tt.err.Params(); !maps.Equal(got, tt.wantParams) {
				t.Errorf("Params() = %#v, want %#v", got, tt.wantParams)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *validator.Error
		want string
	}{
		{name: "code only", err: validator.Fail("corp_domain"), want: "validator: corp_domain"},
		{name: "a param", err: validator.TooLong(50), want: "validator: too_long (count=50)"},
		{name: "a time.Duration", err: validator.LessThan(time.Second), want: "validator: less_than (count=1s)"},
		{
			name: "params in key order",
			err:  validator.FailWith("between", map[string]any{"min": 1, "max": 9}),
			want: "validator: between (max=9, min=1)",
		},
		{name: "no params given", err: validator.FailWith("odd", map[string]any{}), want: "validator: odd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFailWith_KeepsItsOwnParams(t *testing.T) {
	t.Parallel()

	params := map[string]any{"max": 20}
	err := validator.FailWith("too_many", params)
	params["max"] = 99

	got := err.Params()
	got["max"] = 0

	if got := err.Params()["max"]; got != 20 {
		t.Errorf(`Params()["max"] = %v, want 20 whatever the caller does to either map`, got)
	}
}

func TestError_ParamsOfABuiltInRule(t *testing.T) {
	t.Parallel()

	err := validator.TooLong(50)
	err.Params()["count"] = 0

	if got := err.Params()["count"]; got != 50 {
		t.Errorf(`Params()["count"] = %v, want 50 whatever the caller does to the map`, got)
	}
}

func TestError_ComparesByIdentity(t *testing.T) {
	t.Parallel()

	// Comparing does not panic on the params, and two errors that read alike
	// are still two errors.
	a, b := validator.TooLong(50), validator.TooLong(50)
	if a == b {
		t.Error("two errors compare equal")
	}
}
