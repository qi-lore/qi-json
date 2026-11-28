package codec

import (
	"errors"
	"strconv"
)

// ErrorKind classifies a decode error.
type ErrorKind uint8

const (
	// KindSyntax reports invalid JSON in the input.
	KindSyntax ErrorKind = iota

	// KindType reports a JSON value whose type does not match the target.
	KindType

	// KindUnknownField reports an object member that no field matched. It is
	// produced only in strict mode.
	KindUnknownField

	// KindDuplicateField reports a repeated object member. It is produced only
	// in strict mode.
	KindDuplicateField

	// KindDepth reports input that nests deeper than the decoder's fixed
	// nesting limit. The limit matches the Go standard library: encoding/json
	// defines maxNestingDepth = 10000 and rejects deeper input with "exceeded
	// max depth".
	KindDepth
)

// String returns a short name for the kind, used in diagnostics.
func (k ErrorKind) String() string {
	switch k {
	case KindSyntax:
		return "syntax"
	case KindType:
		return "type"
	case KindUnknownField:
		return "unknown field"
	case KindDuplicateField:
		return "duplicate field"
	case KindDepth:
		return "depth"
	default:
		return "unknown"
	}
}

// Error describes a failure to decode JSON into a Go value. It records where in
// the input the failure happened and wraps the underlying cause so callers can
// inspect it with errors.As and errors.Unwrap. A *Error returned by this package
// is immutable: its fields may be read, including from another goroutine, but
// never written.
type Error struct {
	// Path is the JSON path of the value being decoded, such as "$.a.b[2].c".
	Path string

	// Offset is the byte offset into the input where decoding stopped. For a
	// type mismatch it is the start of the offending value; for a syntax error
	// it is the start of the token the scanner rejected.
	Offset int64

	// Kind classifies the failure.
	Kind ErrorKind

	// Err is the underlying cause: a *TypeError for a type mismatch, or the
	// scanner's error for invalid JSON.
	Err error
}

// *Error implements error as part of its contract; this fails to compile if it
// ever stops doing so.
var _ error = (*Error)(nil)

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return "qijson: <nil>"
	}
	var b []byte
	b = append(b, "qijson: "...)
	if e.Path != "" {
		b = append(b, e.Path...)
		b = append(b, ": "...)
	}
	if e.Err != nil {
		b = append(b, e.Err.Error()...)
	} else {
		b = append(b, e.Kind.String()...)
	}
	b = append(b, " (at offset "...)
	b = strconv.AppendInt(b, e.Offset, 10)
	b = append(b, ')')
	return string(b)
}

// Unwrap returns the underlying cause, enabling errors.Is and errors.As to
// inspect it.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// TypeError describes a JSON value whose type does not match the target. A
// returned *TypeError is immutable.
type TypeError struct {
	// Want is the JSON type the target expected, such as "string".
	Want string

	// Got describes the JSON value that was found, such as "number" or
	// "number 1.5 is not an integer".
	Got string
}

// *TypeError implements error as part of its contract.
var _ error = (*TypeError)(nil)

// Error implements the error interface.
func (e *TypeError) Error() string {
	if e == nil {
		return "qijson: type error"
	}
	return "expected " + e.Want + ", found " + e.Got
}

// errBadEscape reports a malformed escape sequence in a JSON string. The scanner
// rejects these before the decoder sees them, so this guards the invariant.
var errBadEscape = errors.New("qijson: invalid string escape")

// errInvalidUTF8 reports a JSON string that is not valid UTF-8. The scanner is
// syntax-only and accepts it; the decoder is stricter and rejects it as a
// syntax error, since RFC 8259 requires a JSON text to be UTF-8.
var errInvalidUTF8 = errors.New("invalid UTF-8 in string")

// errDepthExceeded reports input whose containers nest deeper than the decoder's
// fixed limit. It is the cause of a *Error of Kind KindDepth. It carries no
// "qijson:" prefix because *Error.Error prepends the package prefix and the path;
// the numeric wording tracks maxDepth.
var errDepthExceeded = errors.New("nesting depth exceeds 10000")
