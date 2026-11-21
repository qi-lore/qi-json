package scanner

import (
	"strconv"
	"strings"
	"testing"
)

// requireParseError asserts that in is rejected at the exact position with a
// message containing wantProblem.
func requireParseError(t *testing.T, in string, wantAt int64, wantProblem string) {
	t.Helper()

	err, failed := validate([]byte(in))
	if !failed {
		t.Fatalf("validate(%q) = no error, want %q at position %d", in, wantProblem, wantAt)
	}
	if err.at != wantAt {
		t.Errorf("error position = %d, want %d", err.at, wantAt)
	}
	if !strings.Contains(err.problem, wantProblem) {
		t.Errorf("problem = %q, want it to contain %q", err.problem, wantProblem)
	}
}

func TestValidateAcceptsValidDocuments(t *testing.T) {
	for _, in := range validDocuments {
		t.Run(strconv.Quote(in), func(t *testing.T) {
			if _, failed := validate([]byte(in)); failed {
				t.Fatalf("validate(%q) failed, want success", in)
			}
		})
	}
}

func TestInvalidDocuments(t *testing.T) {
	tests := []struct {
		in      string
		at      int64
		problem string
	}{
		// Empty input and truncated documents.
		{``, 0, "unexpected end of JSON input"},
		{` `, 1, "unexpected end of JSON input"},
		{`{`, 1, "unexpected end of JSON input"},
		{`[`, 1, "unexpected end of JSON input"},
		{`{"a"`, 4, "unexpected end of JSON input"},
		{`{"a":`, 5, "unexpected end of JSON input"},
		{`[1`, 2, "unexpected end of JSON input"},
		{`"`, 1, "unexpected end of JSON input"},
		{`"abc`, 4, "unexpected end of JSON input"},

		// Truncated or mistyped literals.
		{`tru`, 3, "in literal true (expecting 'e')"},
		{`trux`, 4, "in literal true (expecting 'e')"},
		{`tx`, 2, "in literal true (expecting 'r')"},
		{`trx`, 3, "in literal true (expecting 'u')"},
		{`fals`, 4, "in literal false (expecting 'e')"},
		{`falsy`, 5, "in literal false (expecting 'e')"},
		{`fx`, 2, "in literal false (expecting 'a')"},
		{`falx`, 4, "in literal false (expecting 's')"},
		{`nul`, 3, "in literal null (expecting 'l')"},
		{`nulx`, 4, "in literal null (expecting 'l')"},
		{`nx`, 2, "in literal null (expecting 'u')"},
		{`nuX`, 3, "in literal null (expecting 'l')"},

		// Invalid numbers.
		{`-`, 1, "in numeric literal"},
		{`--1`, 2, "in numeric literal"},
		{`01`, 2, "in numeric literal"},
		{`-01`, 3, "in numeric literal"},
		{`1.`, 2, "after decimal point in numeric literal"},
		{`1.e1`, 3, "after decimal point in numeric literal"},
		{`1e`, 2, "in exponent of numeric literal"},
		{`1e+`, 3, "in exponent of numeric literal"},
		{`1e++1`, 4, "in exponent of numeric literal"},
		{`1e+-1`, 4, "in exponent of numeric literal"},

		// Numbers may contain at most one fraction and one exponent.
		{`1.2.3`, 4, "after top-level value"},
		{`1e1e2`, 4, "after top-level value"},
		{`1e1.5`, 4, "after top-level value"},
		{`0e1e2`, 4, "after top-level value"},
		{`1e+1e2`, 5, "after top-level value"},
		{`-1.5E2.3`, 7, "after top-level value"},
		{`[1.2.3]`, 5, "after array value"},
		{`{"a":1.5.6}`, 9, "after object value"},

		// Bytes that cannot begin a value.
		{`+1`, 1, "looking for beginning of value"},
		{`.5`, 1, "looking for beginning of value"},
		{`NaN`, 1, "looking for beginning of value"},
		{`'a'`, 1, "looking for beginning of value"},

		// Object structure.
		{`{a:1}`, 2, "looking for beginning of object key string"},
		{`{,}`, 2, "looking for beginning of object key string"},
		{`{]`, 2, "looking for beginning of object key string"},
		{`{"a" 1}`, 6, "after object key"},
		{`{"a":1 "b":2}`, 8, "after object value"},
		{`{"a":1,}`, 8, "looking for beginning of object key string"},
		{`{"a":1,,"b":2}`, 8, "looking for beginning of object key string"},
		{`{"a":1]`, 7, "after object value"},
		{`{"a":1;}`, 7, "after object value"},
		{`{"a":}`, 6, "looking for beginning of value"},

		// Array structure.
		{`[1 2]`, 4, "after array value"},
		{`[1,]`, 4, "looking for beginning of value"},
		{`[1,,2]`, 4, "looking for beginning of value"},
		{`[,]`, 2, "looking for beginning of value"},
		{`[1}`, 3, "after array value"},
		{`[}`, 2, "looking for beginning of value"},

		// Bytes after a complete top-level value.
		{`{}x`, 3, "after top-level value"},
		{`{} {}`, 4, "after top-level value"},
		{`1 2`, 3, "after top-level value"},
		{`"a""b"`, 4, "after top-level value"},
		{`[1]]`, 4, "after top-level value"},

		// Strings and escapes.
		{`"\q"`, 3, "in string escape code"},
		{`"\u"`, 4, `in \u hexadecimal character escape`},
		{`"\u00"`, 6, `in \u hexadecimal character escape`},
		{`"\u12g"`, 6, `in \u hexadecimal character escape`},
		{`"\u12`, 5, `in \u hexadecimal character escape`},
		{`"\u0g00"`, 5, `in \u hexadecimal character escape`},
		{`"\u000g"`, 7, `in \u hexadecimal character escape`},
		{"\"a\nb\"", 3, "in string literal"},
		{"\"a\tb\"", 3, "in string literal"},
		{"\"a\x00b\"", 3, `invalid character '\x00' in string literal`},

		// Terminators past the eight-byte warm-up exercise the word scan.
		{"\"aaaaaaaaaaaa\x00b\"", 14, "in string literal"},
		{"\"aaaaaaaaaaaa\x1fb\"", 14, "in string literal"},
		{`"aaaaaaaaaaaaaaaaaaaa`, 21, "unexpected end of JSON input"},

		// Incomplete key position after a comma.
		{"{\"a\":1, ", 8, "unexpected end of JSON input"},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			requireParseError(t, tt.in, tt.at, tt.problem)
		})
	}
}

func TestErrorMessage(t *testing.T) {
	err, failed := validate([]byte(`{"a":1]`))
	if !failed {
		t.Fatal("got no error")
	}

	const want = `invalid character ']' after object value`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestSkipSpace(t *testing.T) {
	tests := []struct {
		in   string
		from int
		want int
	}{
		{"", 0, 0},
		{"   ", 0, 3},
		{" \t\r\nx", 0, 4},
		{"abc", 0, 0},
		{"abc", 2, 2},
		{"abc", 3, 3},
		{" \t\r\n", 1, 4},
	}

	for _, tt := range tests {
		if got := skipSpace([]byte(tt.in), tt.from); got != tt.want {
			t.Errorf("skipSpace(%q, %d) = %d, want %d", tt.in, tt.from, got, tt.want)
		}
	}
}

func TestIsDigit(t *testing.T) {
	for c := 0; c < 256; c++ {
		want := '0' <= c && c <= '9'
		if got := isDigit(byte(c)); got != want {
			t.Errorf("isDigit(%q) = %v, want %v", byte(c), got, want)
		}
	}
}

func TestIsHexDigit(t *testing.T) {
	for c := 0; c < 256; c++ {
		b := byte(c)
		want := ('0' <= b && b <= '9') || ('a' <= b && b <= 'f') || ('A' <= b && b <= 'F')
		if got := isHexDigit(b); got != want {
			t.Errorf("isHexDigit(%q) = %v, want %v", b, got, want)
		}
	}
}

func TestGrowStack(t *testing.T) {
	// One level deeper than the inline stack, so grow runs at least once.
	depth := nestingInline + 1

	in := strings.Repeat("[", depth) + strings.Repeat("]", depth)
	if _, failed := validate([]byte(in)); failed {
		t.Fatalf("validate(%d nested arrays) failed, want success", depth)
	}

	// An unbalanced document at the same depth must still fail cleanly.
	unbalanced := strings.Repeat("[", depth) + strings.Repeat("]", depth-1)
	requireParseError(t, unbalanced, int64(len(unbalanced)), "unexpected end of JSON input")
}

func TestValidatorEmptyInput(t *testing.T) {
	v := validator{stack: make([]container, nestingInline)}
	if v.run() {
		t.Fatal("run on empty input = true, want false")
	}
	if v.failure == nil {
		t.Fatal("failure = nil, want an error")
	}
	if v.failure.at != 0 || !strings.Contains(v.failure.problem, "unexpected end of JSON input") {
		t.Fatalf("failure = %+v, want an end-of-input error at position 0", v.failure)
	}
}
