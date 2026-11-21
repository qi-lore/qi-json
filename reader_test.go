package qijson

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	"qi-json/scanner"
)

// newTestReader returns a Reader positioned at the start of doc.
func newTestReader(doc string) *Reader {
	return newReader(scanner.NewTokenizer([]byte(doc)))
}

func TestReadStringUnescapes(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`""`, ``},
		{`"abc"`, `abc`},
		{`"a\"b"`, `a"b`},
		{`"back\\slash"`, `back\slash`},
		{`"solidus\/"`, `solidus/`},
		{`"controls\b\f\n\r\t"`, "controls\b\f\n\r\t"},
		{`"\u0041"`, `A`},
		{`"\u00e9"`, `é`},
		{`"h\u00e9llo w\u00f6rld"`, `héllo wörld`},
		{`"\uD834\uDD1E"`, "\U0001D11E"},
		{`"\uD834"`, "\uFFFD"}, // lone high surrogate becomes the replacement rune
		{`"héllo"`, `héllo`},   // raw UTF-8 passes through
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			r := newTestReader(tt.in)
			got, err := r.ReadString()
			if err != nil {
				t.Fatalf("ReadString() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("ReadString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadStringTypeMismatch(t *testing.T) {
	r := newTestReader(`1`)
	got, err := r.ReadString()
	if err == nil {
		t.Fatalf("ReadString() = %q, nil, want a type error", got)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != KindType {
		t.Errorf("Kind = %v, want %v", e.Kind, KindType)
	}
	var te *TypeError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As(%v, *TypeError) = false", err)
	}
	if te.Want != "string" || te.Got != "number" {
		t.Errorf("TypeError = %+v, want want=string got=number", te)
	}
}

func TestReadInt(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{`0`, 0},
		{`-0`, 0},
		{`1`, 1},
		{`42`, 42},
		{`-42`, -42},
		{`9223372036854775807`, math.MaxInt64},
		{`-9223372036854775808`, math.MinInt64},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			r := newTestReader(tt.in)
			got, err := r.ReadInt()
			if err != nil {
				t.Fatalf("ReadInt() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("ReadInt() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReadIntErrors(t *testing.T) {
	tests := []struct {
		in  string
		got string
	}{
		{`9223372036854775808`, "out of range"},  // overflow
		{`-9223372036854775809`, "out of range"}, // underflow
		{`1.0`, "not an integer"},                // fraction
		{`1e3`, "not an integer"},                // exponent
		{`true`, "boolean"},                      // wrong JSON type
		{`"1"`, "string"},                        // wrong JSON type
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			r := newTestReader(tt.in)
			_, err := r.ReadInt()
			if err == nil {
				t.Fatalf("ReadInt() error = nil, want a failure containing %q", tt.got)
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("error type = %T, want *Error", err)
			}
			if e.Kind != KindType {
				t.Errorf("Kind = %v, want %v", e.Kind, KindType)
			}
			if !strings.Contains(err.Error(), tt.got) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.got)
			}
		})
	}
}

func TestReadFloat(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{`0`, 0},
		{`1.5`, 1.5},
		{`-1.5`, -1.5},
		{`1e3`, 1000},
		{`1E+10`, 1e10},
		{`-1.5e-3`, -0.0015},
		{`123.456e-789`, 0}, // underflows to zero, no error
		{`42`, 42},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			r := newTestReader(tt.in)
			got, err := r.ReadFloat()
			if err != nil {
				t.Fatalf("ReadFloat() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("ReadFloat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadFloatNegativeZero(t *testing.T) {
	r := newTestReader(`-0`)
	got, err := r.ReadFloat()
	if err != nil {
		t.Fatalf("ReadFloat() error = %v, want nil", err)
	}
	if !math.Signbit(got) {
		t.Fatalf("ReadFloat(-0) = %v, want a negative zero", got)
	}
}

func TestReadFloatOverflow(t *testing.T) {
	r := newTestReader(`1e400`)
	_, err := r.ReadFloat()
	if err == nil {
		t.Fatal("ReadFloat(1e400) error = nil, want an overflow error")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error = %q, want it to mention out of range", err.Error())
	}
}

func TestReadBool(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want bool
	}{
		{`true`, true},
		{`false`, false},
	} {
		r := newTestReader(tt.in)
		got, err := r.ReadBool()
		if err != nil {
			t.Fatalf("ReadBool() error = %v, want nil", err)
		}
		if got != tt.want {
			t.Fatalf("ReadBool(%s) = %v, want %v", tt.in, got, tt.want)
		}
	}

	for _, in := range []string{`null`, `1`, `"true"`} {
		r := newTestReader(in)
		if _, err := r.ReadBool(); err == nil {
			t.Errorf("ReadBool(%s) error = nil, want a type error", in)
		}
	}
}

func TestReadNull(t *testing.T) {
	r := newTestReader(`null`)
	if err := r.ReadNull(); err != nil {
		t.Fatalf("ReadNull() error = %v, want nil", err)
	}

	for _, in := range []string{`true`, `0`, `""`} {
		r := newTestReader(in)
		if err := r.ReadNull(); err == nil {
			t.Errorf("ReadNull(%s) error = nil, want a type error", in)
		}
	}
}

func TestErrorPath(t *testing.T) {
	const doc = `{"a":{"b":[0,1,{"c":42}]}}`

	r := newTestReader(doc)
	err := r.Object(func(key string) error {
		if key != "a" {
			return r.Skip()
		}
		return r.Object(func(key string) error {
			if key != "b" {
				return r.Skip()
			}
			return r.Array(func(i int) error {
				if i != 2 {
					return r.Skip()
				}
				return r.Object(func(key string) error {
					if key != "c" {
						return r.Skip()
					}
					_, err := r.ReadString() // 42 is a number, not a string
					return err
				})
			})
		})
	})
	if err == nil {
		t.Fatal("decode error = nil, want a type error at $.a.b[2].c")
	}

	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Path != "$.a.b[2].c" {
		t.Errorf("Path = %q, want %q", e.Path, "$.a.b[2].c")
	}
	if e.Offset != 20 {
		t.Errorf("Offset = %d, want 20", e.Offset)
	}
	if e.Kind != KindType {
		t.Errorf("Kind = %v, want %v", e.Kind, KindType)
	}
}

func TestErrorPathQuotedKey(t *testing.T) {
	const doc = `{"a b":1}`

	r := newTestReader(doc)
	var e *Error
	err := r.Object(func(key string) error {
		if key != "a b" {
			return r.Skip()
		}
		_, err := r.ReadString()
		if !errors.As(err, &e) {
			return errors.New("expected a *Error")
		}
		return err
	})
	if err == nil {
		t.Fatal("decode error = nil, want a type error")
	}
	if e == nil || e.Path != `$["a b"]` {
		t.Fatalf("Path = %v, want %q", e, `$["a b"]`)
	}
}

func TestErrorUnwrap(t *testing.T) {
	t.Run("type", func(t *testing.T) {
		r := newTestReader(`1`)
		_, err := r.ReadBool()

		var te *TypeError
		if !errors.As(err, &te) {
			t.Fatalf("errors.As(%v, *TypeError) = false, want true", err)
		}
		if te.Want != "boolean" || te.Got != "number" {
			t.Errorf("TypeError = %+v, want want=boolean got=number", te)
		}
	})

	t.Run("syntax", func(t *testing.T) {
		r := newTestReader(`"abc`) // unterminated string
		_, err := r.ReadString()

		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("errors.As(%v, *Error) = false, want true", err)
		}
		if e.Kind != KindSyntax {
			t.Errorf("Kind = %v, want %v", e.Kind, KindSyntax)
		}

		underlying := errors.Unwrap(err)
		if underlying == nil {
			t.Fatal("errors.Unwrap() = nil, want the scanner error")
		}
		if !strings.Contains(underlying.Error(), "unexpected end of JSON input") {
			t.Errorf("underlying error = %q, want the scanner diagnostic", underlying.Error())
		}
	})
}

func TestReaderSkip(t *testing.T) {
	const doc = `{"a":{"b":[1,{"c":true}]},"d":2}`

	r := newTestReader(doc)
	var keys []string
	err := r.Object(func(key string) error {
		keys = append(keys, key)
		return r.Skip()
	})
	if err != nil {
		t.Fatalf("Object() error = %v, want nil", err)
	}
	if strings.Join(keys, ",") != "a,d" {
		t.Fatalf("keys = %v, want [a d]", keys)
	}
	if got := r.Offset(); got != int64(len(doc)) {
		t.Fatalf("Offset() = %d, want %d", got, len(doc))
	}
}

func TestReaderPathAtRoot(t *testing.T) {
	r := newTestReader(`0`)
	if got := r.Path(); got != "$" {
		t.Fatalf("Path() = %q, want %q", got, "$")
	}
}

func TestObjectTypeMismatch(t *testing.T) {
	r := newTestReader(`1`)
	err := r.Object(func(string) error { return nil })
	if err == nil {
		t.Fatal("Object() error = nil, want a type error")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != KindType {
		t.Errorf("Kind = %v, want %v", e.Kind, KindType)
	}
	var te *TypeError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As(%v, *TypeError) = false", err)
	}
	if te.Want != "object" || te.Got != "number" {
		t.Errorf("TypeError = %+v, want want=object got=number", te)
	}
}

func TestArrayTypeMismatch(t *testing.T) {
	r := newTestReader(`{}`)
	err := r.Array(func(int) error { return nil })
	if err == nil {
		t.Fatal("Array() error = nil, want a type error")
	}
	var te *TypeError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As(%v, *TypeError) = false", err)
	}
	if te.Want != "array" || te.Got != "object" {
		t.Errorf("TypeError = %+v, want want=array got=object", te)
	}
}

func TestPrimitiveNoAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("allocation test is skipped in short mode")
	}

	// Reading numbers, booleans and null must not allocate; only the tokenizer
	// construction does. (Strings allocate once for the decoded result, so they
	// are covered elsewhere.)
	const doc = `[123,1.5e3,true,null]`

	data := []byte(doc)
	r := newReader(scanner.NewTokenizer(data))

	// Baseline: constructing a tokenizer and nothing else.
	base := testing.AllocsPerRun(100, func() {
		r.tok = scanner.NewTokenizer(data)
	})

	full := testing.AllocsPerRun(100, func() {
		r.tok = scanner.NewTokenizer(data)
		err := r.Array(func(i int) error {
			switch i {
			case 0:
				_, err := r.ReadInt()
				return err
			case 1:
				_, err := r.ReadFloat()
				return err
			case 2:
				_, err := r.ReadBool()
				return err
			default:
				return r.ReadNull()
			}
		})
		if err != nil {
			panic(err)
		}
	})

	if full != base {
		t.Errorf("primitive read path allocated %v times per run, baseline %v; want equal (no allocations)", full, base)
	}
}
