package codec

import (
	"errors"
	"testing"
)

// TestCovErrorKindString pins the short name of every kind and the fallback for
// an out-of-range kind.
func TestCovErrorKindString(t *testing.T) {
	tests := []struct {
		kind ErrorKind
		want string
	}{
		{KindSyntax, "syntax"},
		{KindType, "type"},
		{KindUnknownField, "unknown field"},
		{KindDuplicateField, "duplicate field"},
		{KindDepth, "depth"},
		{ErrorKind(200), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("ErrorKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

// TestCovErrorNilReceiver checks the nil-receiver methods report the sentinel
// text and no cause rather than panicking.
func TestCovErrorNilReceiver(t *testing.T) {
	var e *Error
	if got, want := e.Error(), "qijson: <nil>"; got != want {
		t.Errorf("(*Error)(nil).Error() = %q, want %q", got, want)
	}
	if err := e.Unwrap(); err != nil {
		t.Errorf("(*Error)(nil).Unwrap() = %v, want nil", err)
	}

	var te *TypeError
	if got, want := te.Error(), "qijson: type error"; got != want {
		t.Errorf("(*TypeError)(nil).Error() = %q, want %q", got, want)
	}
}

// TestCovErrorMessages pins the message composition for a nil cause (kind
// fallback) and a wrapped cause, with and without a path.
func TestCovErrorMessages(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "kind fallback no path",
			err:  &Error{Kind: KindSyntax, Offset: 3},
			want: "qijson: syntax (at offset 3)",
		},
		{
			name: "kind fallback with path",
			err:  &Error{Path: "$.a", Kind: KindDepth, Offset: 5},
			want: "qijson: $.a: depth (at offset 5)",
		},
		{
			name: "wrapped cause",
			err:  &Error{Path: "$.b", Err: errors.New("boom"), Offset: 1},
			want: "qijson: $.b: boom (at offset 1)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCovErrorUnwrap checks Unwrap exposes the cause so errors.Is and errors.As
// reach it.
func TestCovErrorUnwrap(t *testing.T) {
	cause := errors.New("cause")
	e := &Error{Err: cause}
	if !errors.Is(e, cause) {
		t.Errorf("errors.Is(%v, cause) = false, want true", e)
	}

	if e.Unwrap() != cause {
		t.Errorf("Unwrap() = %v, want %v", e.Unwrap(), cause)
	}

	bare := &Error{Kind: KindType}
	if bare.Unwrap() != nil {
		t.Errorf("Unwrap() = %v, want nil for a cause-less error", bare.Unwrap())
	}
}

// TestCovTypeErrorMessage pins both the nil-receiver text and the value form.
func TestCovTypeErrorMessage(t *testing.T) {
	e := &TypeError{Want: "integer", Got: "string"}
	if got, want := e.Error(), "expected integer, found string"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
