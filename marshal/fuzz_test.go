package marshal

import (
	"errors"
	"testing"

	"qi-json/scanner"
)

// fuzzAny decodes any JSON value at the document root. It runs the whole
// semantic layer — strings, numbers, booleans, null, objects and arrays — over
// arbitrary input without a Schema, so the fuzz targets exercise the same code
// paths a real decoder would.
type fuzzAny struct{ V any }

func (a *fuzzAny) DecodeJSON(r *Reader) error {
	v, err := decodeAny(r)
	if err != nil {
		return err
	}
	a.V = v
	return nil
}

// fuzzSeeds reach every branch: one value of each kind, composites, escapes, a
// surrogate pair, an overflowing number, and a spread of malformed documents.
var fuzzSeeds = []string{
	`null`, `true`, `false`, `0`, `-0`, `-1.5e10`, `1e400`, `"hi"`, `""`,
	`{"a":1,"b":[true,null,"x"],"c":{"d":2.5}}`,
	`[]`, `{}`, `[1,2,3]`, `{"":[]}`,
	`"\u0041\uD83D\uDE00"`, `"\n\t\"\\\/"`, `"\uD800"`,
	``, ` `, `{`, `[1,]`, `tru`, `01`, `"\u12`, `{}x`, `1.2.3`, `"a` + "\n" + `b"`,
	`{"a":}`, `[1 2]`, `nul`, `-`, `1e`, `"unterminated`, `[`, `{"a"`, `1 2`,
	// A number that overflows float64 fails as a type error before the trailing
	// garbage is reached, so this is invalid JSON that is not a syntax error.
	`1e700A`, `1e999x`,
	// Invalid UTF-8: the decoder rejects it as a syntax error, but the
	// syntax-only scanner accepts the same bytes.
	"\"a\xff\"", "{\"k\xff\":1}", "\"\xc0\xaf\"", "\"\xed\xa0\x80\"",
}

// FuzzUnmarshal checks the entry point over arbitrary bytes: it must never
// panic, every failure must be an *Error with a JSON path and an in-range
// offset, and a successful decode must be deterministic.
func FuzzUnmarshal(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := Unmarshal[fuzzAny](data)
		if err != nil {
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("Unmarshal(%q) error %v is not an *Error", data, err)
			}
			if e.Path == "" {
				t.Fatalf("Unmarshal(%q) error has an empty path: %v", data, err)
			}
			if e.Offset < 0 || e.Offset > int64(len(data)) {
				t.Fatalf("Unmarshal(%q) offset %d out of range 0..%d", data, e.Offset, len(data))
			}
			return
		}

		// A successful decode is deterministic.
		again, err := Unmarshal[fuzzAny](data)
		if err != nil {
			t.Fatalf("Unmarshal(%q) succeeded then failed: %v", data, err)
		}
		if !anyEqual(v.V, again.V) {
			t.Fatalf("Unmarshal(%q) is not deterministic: %v then %v", data, v.V, again.V)
		}
	})
}

// FuzzUnmarshalMatchesValid ties the decoder's classification to the scanner in
// the two directions that must hold: a successful decode proves the document is
// valid, and a syntax error proves it is not — except for invalid UTF-8, which
// the decoder rejects but the syntax-only scanner accepts.
//
// The converse does not hold and is deliberately not asserted: a document can be
// invalid yet fail earlier as a type error. For example 1e700A is invalid JSON,
// but the number 1e700 overflows float64 and is rejected as a type error before
// the trailing A is ever reached.
func FuzzUnmarshalMatchesValid(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		valid := scanner.Valid(data)
		_, err := Unmarshal[fuzzAny](data)

		// A successful decode proves the document is valid.
		if err == nil {
			if !valid {
				t.Fatalf("Unmarshal(%q) succeeded but Valid(%q) = false", data, data)
			}
			return
		}

		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("Unmarshal(%q) error %v is not an *Error", data, err)
		}
		// A syntax error proves the document is invalid, unless it is the
		// decoder's stricter UTF-8 rejection.
		if e.Kind == KindSyntax && valid && !errors.Is(err, errInvalidUTF8) {
			t.Fatalf("Unmarshal(%q) reported a syntax error at offset %d but Valid(%q) = true", data, e.Offset, data)
		}
	})
}

// anyEqual reports whether two values produced by decodeAny are deeply equal.
// It does the job of reflect.DeepEqual without reflection, which the decoder is
// forbidden to use, and it exists only to check determinism in the fuzz targets.
func anyEqual(a, b any) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !anyEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			w, ok := bv[k]
			if !ok || !anyEqual(v, w) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
