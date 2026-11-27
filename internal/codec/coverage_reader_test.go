package codec

import (
	"errors"
	"testing"

	"github.com/qi-lore/qi-json/internal/scanner"
)

// TestCovReaderTypedMismatches drives ReadNull, ReadBool and ReadString when the
// next value is the wrong kind, so each mismatch branch is exercised.
func TestCovReaderTypedMismatches(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		call    func(*Reader) error
		want    string
		wantGot string
	}{
		{"ReadNull", `1`, func(r *Reader) error { return r.ReadNull() }, "null", "number"},
		{"ReadBool", `1`, func(r *Reader) error { _, err := r.ReadBool(); return err }, "boolean", "number"},
		{"ReadString", `1`, func(r *Reader) error { _, err := r.ReadString(); return err }, "string", "number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(newTestReader(tt.doc))
			var te *TypeError
			if !errors.As(err, &te) {
				t.Fatalf("errors.As(*TypeError) = false for %T", err)
			}
			if te.Want != tt.want || te.Got != tt.wantGot {
				t.Errorf("TypeError = %+v, want want=%s got=%s", te, tt.want, tt.wantGot)
			}
		})
	}
}

// TestCovReaderNextKindErrors drives ReadNull, ReadBool and ReadString when the
// first peek itself fails, so each nextKind error branch is exercised.
func TestCovReaderNextKindErrors(t *testing.T) {
	if err := newTestReader(``).ReadNull(); err == nil {
		t.Error("ReadNull on empty input: want an error")
	}
	if _, err := newTestReader(``).ReadBool(); err == nil {
		t.Error("ReadBool on empty input: want an error")
	}
	if _, err := newTestReader(``).ReadString(); err == nil {
		t.Error("ReadString on empty input: want an error")
	}
}

// TestCovReaderPeekAndReadErrors checks the entry points that peek or read
// against invalid or empty input report a syntax error rather than panicking.
func TestCovReaderPeekAndReadErrors(t *testing.T) {
	if _, _, err := newTestReader(``).readNumber("integer"); err == nil {
		t.Error("readNumber on empty input: want an error")
	}
	if err := newTestReader(``).Skip(); err == nil {
		t.Error("Skip on empty input: want an error")
	}
	if _, err := newTestReader(``).raw(); err == nil {
		t.Error("raw on empty input: want an error")
	}
}

// TestCovReaderBeginMismatches checks Object and Array report a type error when
// the next value is not the matching container.
func TestCovReaderBeginMismatches(t *testing.T) {
	if err := newTestReader(`1`).Object(func(string) error { return nil }); err == nil {
		t.Error("Object on a number: want a type error")
	}
	if err := newTestReader(`1`).Array(func(int) error { return nil }); err == nil {
		t.Error("Array on a number: want a type error")
	}
	if err := newTestReader(``).Object(func(string) error { return nil }); err == nil {
		t.Error("Object on empty input: want a syntax error")
	}
	if err := newTestReader(``).Array(func(int) error { return nil }); err == nil {
		t.Error("Array on empty input: want a syntax error")
	}
}

// TestCovReaderInvalidUTF8Key checks a key that is not valid UTF-8 is rejected by
// both the path-tracking Object and the raw objectRaw iterator.
func TestCovReaderInvalidUTF8Key(t *testing.T) {
	data := append([]byte(`{"`), 0xff, '"', ':', '1', '}')

	r := newReader(scanner.NewTokenizer(data), string(data))
	if err := r.Object(func(string) error { return nil }); err == nil {
		t.Error("Object with an invalid UTF-8 key: want a syntax error")
	}

	r = newReader(scanner.NewTokenizer(data), string(data))
	if err := r.objectRaw(func([]byte) error { return nil }); err == nil {
		t.Error("objectRaw with an invalid UTF-8 key: want a syntax error")
	}
}

// TestCovSyntaxErrorNilCause checks syntaxError(nil) is a no-op.
func TestCovSyntaxErrorNilCause(t *testing.T) {
	if err := newTestReader(`1`).syntaxError(nil); err != nil {
		t.Errorf("syntaxError(nil) = %v, want nil", err)
	}
}

// TestCovContainerLenHintBounds checks an out-of-range start yields no hint.
func TestCovContainerLenHintBounds(t *testing.T) {
	if got := containerLenHint("[]", 5, '[', ']'); got != 0 {
		t.Errorf("containerLenHint(out of range) = %d, want 0", got)
	}
	if got := containerLenHint("[]", -1, '[', ']'); got != 0 {
		t.Errorf("containerLenHint(negative) = %d, want 0", got)
	}
}

// TestCovIsPlainKey checks the identifier test used to decide whether a path key
// can follow a dot.
func TestCovIsPlainKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"", false},
		{"a", true},
		{"A1_$", true},
		{"a.b", false},
		{"a b", false},
	}
	for _, tt := range tests {
		if got := isPlainKey(tt.key); got != tt.want {
			t.Errorf("isPlainKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

// TestCovAppendQuotedTo checks a path key with a quote or a backslash is escaped.
func TestCovAppendQuotedTo(t *testing.T) {
	got := string(appendQuotedTo(nil, `a"b\c`))
	if want := `"a\"b\\c"`; got != want {
		t.Errorf("appendQuotedTo = %s, want %s", got, want)
	}
}

// TestCovUnescapeErrors drives the malformed-escape branches of unescape. The
// scanner rejects these before the decoder sees them, so this guards the
// invariant.
func TestCovUnescapeErrors(t *testing.T) {
	for _, raw := range []string{`\`, `\x`, `\uZZZZ`, `\u12`} {
		if _, err := unescape([]byte(raw)); !errors.Is(err, errBadEscape) {
			t.Errorf("unescape(%q) error = %v, want errBadEscape", raw, err)
		}
	}
}

// TestCovUnescapeUnicode checks the well-formed short/hex checks and the lone
// surrogate fallback.
func TestCovUnescapeUnicode(t *testing.T) {
	tests := []struct {
		raw  string
		ok   bool
		want rune
	}{
		{`\u`, false, 0},
		{`\uZZZZ`, false, 0},
		{`\u0041`, true, 'A'},
		{`\uD834\uDD1E`, true, '\U0001D11E'},
		{`\uD834`, true, '\uFFFD'},
	}
	for _, tt := range tests {
		r, _, ok := unescapeUnicode([]byte(tt.raw))
		if ok != tt.ok {
			t.Errorf("unescapeUnicode(%q) ok = %v, want %v", tt.raw, ok, tt.ok)
			continue
		}
		if ok && r != tt.want {
			t.Errorf("unescapeUnicode(%q) = %U, want %U", tt.raw, r, tt.want)
		}
	}
}

// TestCovHexHelpers checks hex4 and hexVal reject short and non-hex input.
func TestCovHexHelpers(t *testing.T) {
	if _, ok := hex4([]byte(`ab`)); ok {
		t.Error("hex4(short) = ok, want false")
	}
	if _, ok := hex4([]byte(`zzzz`)); ok {
		t.Error("hex4(non-hex) = ok, want false")
	}
	if v, ok := hex4([]byte(`00FF`)); !ok || v != 0xFF {
		t.Errorf("hex4(00FF) = %d, %v, want 255, true", v, ok)
	}

	if _, ok := hexVal('z'); ok {
		t.Error("hexVal('z') = ok, want false")
	}
	for c, want := range map[byte]byte{'0': 0, '9': 9, 'a': 10, 'f': 15, 'A': 10, 'F': 15} {
		v, ok := hexVal(c)
		if !ok || v != want {
			t.Errorf("hexVal(%q) = %d, %v, want %d, true", c, v, ok, want)
		}
	}
}
