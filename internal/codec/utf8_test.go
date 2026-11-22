package codec

import (
	"errors"
	"testing"

	"github.com/qi-lore/qi-json/scanner"
)

// invalidUTF8Cases pairs a JSON document with the byte offset of the first
// invalid UTF-8 byte the decoder must reject it at.
var invalidUTF8Cases = []struct {
	name   string
	in     string
	offset int64
}{
	{"high byte", "\"a\xffb\"", 2},
	{"lone continuation", "\"\x80\"", 1},
	{"truncated sequence", "\"a\xc3\"", 2},
	{"overlong", "\"\xc0\xaf\"", 1},
}

func TestReadStringInvalidUTF8(t *testing.T) {
	for _, tt := range invalidUTF8Cases {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestReader(tt.in)
			got, err := r.ReadString()
			if err == nil {
				t.Fatalf("ReadString() = %q, nil, want an invalid-UTF-8 error", got)
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("error type = %T, want *Error", err)
			}
			if e.Kind != KindSyntax {
				t.Errorf("Kind = %v, want %v", e.Kind, KindSyntax)
			}
			if e.Offset != tt.offset {
				t.Errorf("Offset = %d, want %d", e.Offset, tt.offset)
			}
			if !errors.Is(err, errInvalidUTF8) {
				t.Errorf("errors.Is(err, errInvalidUTF8) = false: %v", err)
			}
		})
	}
}

func TestReadStringValidUTF8(t *testing.T) {
	tests := []string{
		`""`,
		`"héllo"`,                     // 2-byte sequences
		`"日本語"`,                       // 3-byte sequences
		`"😀"`,                         // 4-byte sequence
		`"a` + "\xc3\xa9" + `\u0041"`, // raw 2-byte plus a JSON \u escape
	}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			r := newTestReader(in)
			if _, err := r.ReadString(); err != nil {
				t.Fatalf("ReadString() error = %v, want nil", err)
			}
		})
	}
}

// TestInvalidUTF8Key checks that an object member name is validated too, both
// through a Schema (objectRaw) and through Any (Object).
func TestInvalidUTF8Key(t *testing.T) {
	t.Run("schema", func(t *testing.T) {
		type config struct{ A int64 }
		schema := Struct(Int[config]("a", func(c *config) *int64 { return &c.A }))
		var c config
		err := schema.UnmarshalInto([]byte("{\"a\xff\":1}"), &c)
		assertInvalidUTF8(t, err, 3)
	})
	t.Run("any", func(t *testing.T) {
		r := newTestReader("{\"k\xff\":1}")
		_, err := decodeAny(r)
		assertInvalidUTF8(t, err, 3)
	})
	t.Run("any value", func(t *testing.T) {
		r := newTestReader("{\"k\":\"v\xff\"}")
		_, err := decodeAny(r)
		assertInvalidUTF8(t, err, 7)
	})
}

// TestInvalidUTF8ScannerAccepts pins the intended divergence: the scanner is
// syntax-only, so it accepts bytes the decoder rejects.
func TestInvalidUTF8ScannerAccepts(t *testing.T) {
	for _, tt := range invalidUTF8Cases {
		if !scanner.Valid([]byte(tt.in)) {
			t.Errorf("Valid(%q) = false, want true (the scanner does not check UTF-8)", tt.in)
		}
	}
}

func assertInvalidUTF8(t *testing.T, err error, offset int64) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want an invalid-UTF-8 error")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != KindSyntax {
		t.Errorf("Kind = %v, want %v", e.Kind, KindSyntax)
	}
	if e.Offset != offset {
		t.Errorf("Offset = %d, want %d", e.Offset, offset)
	}
	if !errors.Is(err, errInvalidUTF8) {
		t.Errorf("errors.Is(err, errInvalidUTF8) = false: %v", err)
	}
}
