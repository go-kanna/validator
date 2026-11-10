package validator

import (
	"cmp"
	"encoding"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DecodeJSON decodes the JSON value r holds into v, which must be a non-nil
// pointer, and reports everything wrong with it at once. v has to validate
// itself, as a struct marked //kanna:validate does, so that a struct left
// unmarked does not pass unchecked.
//
// A value whose type rejects it with a Coded error, as the value types of
// kanna-value do, does not stop the decoding: the error is recorded at the
// value's path and decoding goes on. The rejection may come from any method
// encoding/json/v2 decodes a type with: UnmarshalJSONFrom, UnmarshalJSON, or
// UnmarshalText. Then what the Validate method of v finds is added, except at
// and under the paths decoding rejected. Those values were never set, and
// checking them would only report the absence of a value that was in fact
// sent.
//
// The result is the Errors of both, or nil. A request that cannot be read,
// rather than one that breaks a rule, fails with a *DecodeError instead: JSON
// that is malformed, holds more than one value, or does not fit the Go types, a
// rejection that is not Coded or that falls on a map key, and a failure to read
// r.
//
// Decoding follows encoding/json/v2, which, unlike encoding/json, matches names
// case-sensitively, rejects duplicate names, and skips unknown names. r is read
// to its end, so a limit on its size, such as http.MaxBytesReader sets, is the
// caller's to set.
func DecodeJSON(r io.Reader, v interface{ Validate() error }) error {
	var d decoding
	dec := jsontext.NewDecoder(r)
	err := json.UnmarshalDecode(dec, v, json.WithUnmarshalers(json.JoinUnmarshalers(
		// In the order encoding/json/v2 prefers the methods.
		json.UnmarshalFromFunc(d.unmarshalJSONFrom),
		json.UnmarshalFromFunc(d.unmarshalJSON),
		json.UnmarshalFromFunc(d.unmarshalText),
	)))
	if errors.Is(err, io.EOF) {
		// There was no value at all.
		return &DecodeError{Reason: ReasonSyntax, err: io.ErrUnexpectedEOF}
	}
	if err != nil {
		return decodeError(err)
	}
	switch _, err := dec.ReadToken(); {
	case errors.Is(err, io.EOF):
	case err == nil || errors.As(err, new(*jsontext.SyntacticError)):
		if err == nil {
			err = errTrailing
		}
		return &DecodeError{Reason: ReasonTrailing, err: err}
	default:
		return decodeError(err)
	}

	var found Errors
	found.Nest("", v.Validate())
	for _, fe := range found {
		if !slices.ContainsFunc(d.rejected, func(path string) bool { return within(fe.Field, path) }) {
			d.errs = append(d.errs, fe)
		}
	}
	return d.errs.Err()
}

// DecodeError is why DecodeJSON could not read a request. It says where and
// why, but never what was sent or what Go type it was meant for, so that it can
// answer the request as it is, in its message or encoded as JSON, unlike the
// error it wraps.
type DecodeError struct {
	// Pointer is the JSON pointer to where reading failed, such as
	// "/address/zip", or "" for the top-level value.
	Pointer jsontext.Pointer `json:"pointer"`

	Reason DecodeReason `json:"reason"`

	// err is the error of encoding/json/v2, or of reading, behind it.
	err error
}

// DecodeReason is what kind of request DecodeJSON could not read.
type DecodeReason string

// The reasons DecodeJSON could not read a request.
const (
	// ReasonSyntax is JSON that is malformed or cut short.
	ReasonSyntax DecodeReason = "syntax"
	// ReasonTrailing is anything after the JSON value but white space.
	ReasonTrailing DecodeReason = "trailing"
	// ReasonDuplicateName is a name an object already has.
	ReasonDuplicateName DecodeReason = "duplicate_name"
	// ReasonValue is a JSON value, or a name for a map key, that the Go type
	// does not take: one of the wrong kind, such as a string for a struct, a
	// number out of range, a string that is no time, or one a type rejects
	// without a Coded error.
	ReasonValue DecodeReason = "value"
	// ReasonRead is a failure to read the request, such as one
	// http.MaxBytesReader cuts off.
	ReasonRead DecodeReason = "read"
)

// Error says why and where, such as
// `validator: cannot decode JSON at "/address": value`. The pointer is quoted,
// as the names in it come from the request.
func (e *DecodeError) Error() string {
	if e.Pointer == "" {
		return "validator: cannot decode JSON: " + string(e.Reason)
	}
	return "validator: cannot decode JSON at " + strconv.Quote(string(e.Pointer)) + ": " + string(e.Reason)
}

// Unwrap returns the error behind e, which errors.As can tell apart, such as a
// *json.SemanticError or an *http.MaxBytesError.
func (e *DecodeError) Unwrap() error {
	return e.err
}

var errTrailing = errors.New("JSON value after the top-level value")

// decodeError tells why encoding/json/v2 failed with err. A SemanticError may
// wrap a SyntacticError a type reported, but not the other way around.
func decodeError(err error) *DecodeError {
	if sem, ok := errors.AsType[*json.SemanticError](err); ok {
		return &DecodeError{Pointer: sem.JSONPointer, Reason: ReasonValue, err: err}
	}
	if syn, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		reason := ReasonSyntax
		if errors.Is(syn.Err, jsontext.ErrDuplicateName) {
			reason = ReasonDuplicateName
		}
		return &DecodeError{Pointer: syn.JSONPointer, Reason: reason, err: err}
	}
	return &DecodeError{Reason: ReasonRead, err: err}
}

// decoding calls the methods types decode themselves with, as encoding/json/v2
// would, but records a Coded rejection and goes on instead of stopping.
type decoding struct {
	errs Errors

	// rejected holds the path of each of errs, with "" rather than Base for
	// the top-level value.
	rejected []string
}

func (d *decoding) unmarshalJSONFrom(dec *jsontext.Decoder, u json.UnmarshalerFrom) error {
	if !handles(dec, u) {
		return errors.ErrUnsupported
	}
	depth := dec.StackDepth()
	_, before := dec.StackIndex(depth)
	err := u.UnmarshalJSONFrom(dec)
	if dec.StackDepth() == depth {
		if _, after := dec.StackIndex(depth); after == before+1 {
			return d.record(dec, err)
		}
	}
	// It stopped partway through the value, so decoding cannot go on.
	return err //nolint:wrapcheck // encoding/json/v2 reports it with the position it occurred at
}

func (d *decoding) unmarshalJSON(dec *jsontext.Decoder, u json.Unmarshaler) error {
	if !handles(dec, u) {
		return errors.ErrUnsupported
	}
	raw, err := dec.ReadValue()
	if err != nil {
		return err //nolint:wrapcheck // encoding/json/v2 reports it with the offset it occurred at
	}
	return d.record(dec, u.UnmarshalJSON(raw))
}

func (d *decoding) unmarshalText(dec *jsontext.Decoder, u encoding.TextUnmarshaler) error {
	// encoding/json/v2 calls UnmarshalText only for a string. It decodes null
	// and rejects anything else on its own.
	if dec.PeekKind() != jsontext.KindString || !handles(dec, u) {
		return errors.ErrUnsupported
	}
	tok, err := dec.ReadToken()
	if err != nil {
		return err //nolint:wrapcheck // encoding/json/v2 reports it with the offset it occurred at
	}
	return d.record(dec, u.UnmarshalText([]byte(tok.String())))
}

// record keeps err at the path of the value dec has just read if it is Coded,
// so that decoding goes on, and hands back anything else.
func (d *decoding) record(dec *jsontext.Decoder, err error) error {
	if _, ok := errors.AsType[Coded](err); !ok {
		return err // nil, or for encoding/json/v2 to report with the value's position
	}
	path := pathOf(dec)
	d.errs.Add(cmp.Or(path, Base), err)
	d.rejected = append(d.rejected, path)
	return nil
}

// handles reports whether DecodeJSON calls the method of v itself, rather than
// leaving the value to encoding/json/v2. It leaves alone a map key, which
// cannot be skipped like a value, and time.Time, which encoding/json/v2 decodes
// on its own rather than through its methods.
func handles(dec *jsontext.Decoder, v any) bool {
	if _, ok := v.(*time.Time); ok {
		return false
	}
	// An object counts its names and values alike, so a name comes after an
	// even count.
	kind, n := dec.StackIndex(dec.StackDepth())
	return kind != jsontext.KindBeginObject || n%2 == 1
}

// pathOf spells the path to the value dec has just read with JSON names, such
// as "address.zip" or "tags[1]", or returns "" for the top-level value.
//
// A token of a JSON pointer does not tell an array index from an object name,
// so the kind of each level comes from the decoder's stack, which has one level
// per token.
func pathOf(dec *jsontext.Decoder) string {
	var path string
	level := 1
	for token := range dec.StackPointer().Tokens() {
		switch kind, _ := dec.StackIndex(level); {
		case kind == jsontext.KindBeginArray:
			path += "[" + token + "]"
		case path == "":
			path = token
		default:
			path += "." + token
		}
		level++
	}
	return path
}

// within reports whether field is path or lies under it. Every field lies
// under "", the top-level value.
func within(field, path string) bool {
	rest, ok := strings.CutPrefix(field, path)
	return ok && (path == "" || rest == "" || rest[0] == '.' || rest[0] == '[')
}
