package scanner

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

var validDocuments = []string{
	// Literals.
	`null`,
	`true`,
	`false`,

	// Numbers.
	`0`,
	`-0`,
	`1`,
	`-1`,
	`1234567890`,
	`-1234567890`,
	`0.0`,
	`-0.0`,
	`1.5`,
	`-1.5`,
	`1e0`,
	`1E0`,
	`1e+0`,
	`1e-0`,
	`-1.5E+10`,
	`123.456e-789`,

	// Strings.
	`""`,
	`" "`,
	`"hello, world"`,
	`"\"quoted\""`,
	`"back\\slash"`,
	`"solidus\/"`,
	`"controls\b\f\n\r\t"`,
	`"\u0000"`,
	`"\u00e9"`,
	`"\uD834\uDD1E"`,
	`"h\u00e9llo w\u00f6rld"`,
	`"héllo wörld"`, // raw UTF-8 bytes pass through unvalidated

	// Strings longer than the eight-byte warm-up, including escapes past it.
	`"aaaaaaaaaaaaaaaa"`,
	`"aaaaaaaaaaaa\n"`,
	`"aaaaaaaaaaaaaaaaaaaaaaaa\u00e9bbbbbbbbbbbbbbbbbbbbbbbb"`,
	"\"aaaaaaaaaaaaa\x7fb\"", // 0x7f is legal inside a string

	// Objects.
	`{}`,
	`{ }`,
	`{"a":1}`,
	`{"a":1,"b":2}`,
	`{"a":{"b":{"c":[]}}}`,
	`{"a":null,"b":true,"c":false,"d":"x","e":-1.5e-3}`,

	// Arrays.
	`[]`,
	`[ ]`,
	`[1,2,3]`,
	`[[],[[]],{}]`,
	`[{"a":[1,2]},null,true,false,"x"]`,

	// Whitespace between tokens and around the document.
	" \t\r\n{\"a\" : [ 1 , 2 ] } \n",
	"[\n  1,\n  2,\n  3\n]",
}

func TestValidDocuments(t *testing.T) {
	for _, in := range validDocuments {
		t.Run(strconv.Quote(in), func(t *testing.T) {
			if !Valid([]byte(in)) {
				t.Fatalf("Valid(%q) = false, want true", in)
			}
			if _, failed := validate([]byte(in)); failed {
				t.Fatalf("validate(%q) failed, want success", in)
			}
		})
	}
}

func TestValid(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{`{"a":[1,2]}`, true},
		{`[]`, true},
		{"  \"x\"", true},
		{`123`, true},
		{`true`, true},
		{`null`, true},
		{`{}`, true},
		{`123x`, false},
		{`tru`, false},
		{`{`, false},
		{``, false},
		{`{}x`, false},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			if got := Valid([]byte(tt.in)); got != tt.want {
				t.Fatalf("Valid(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidReuseAcrossCalls(t *testing.T) {
	// A failed scan must not leak state into the next call.
	docs := []struct {
		in   string
		want bool
	}{
		{`{"a":1}`, true},
		{`{"a":`, false},
		{`[]`, true},
		{`[1,]`, false},
		{`true`, true},
		{`tru`, false},
		{`-1.5e3`, true},
		{`1.2.3`, false},
	}

	for i := 0; i < 3; i++ {
		for _, doc := range docs {
			if got := Valid([]byte(doc.in)); got != doc.want {
				t.Fatalf("iteration %d: Valid(%q) = %v, want %v", i, doc.in, got, doc.want)
			}
		}
	}
}

func TestValidDoesNotAllocate(t *testing.T) {
	if testing.Short() {
		t.Skip("allocation test is skipped in short mode")
	}

	data := []byte(`{"a":[1,2,3],"b":{"c":true},"d":null,"e":"x"}`)
	if !Valid(data) {
		t.Fatal("Valid() = false for a valid document")
	}

	allocs := testing.AllocsPerRun(100, func() {
		if !Valid(data) {
			panic("Valid() = false for a valid document")
		}
	})
	if allocs != 0 {
		t.Errorf("Valid() allocated %v times per run, want 0", allocs)
	}
}

func TestValidDeepNesting(t *testing.T) {
	const depth = 50000

	var b strings.Builder
	b.Grow(2 * depth)
	for i := 0; i < depth; i++ {
		b.WriteByte('[')
	}
	for i := 0; i < depth; i++ {
		b.WriteByte(']')
	}

	if !Valid([]byte(b.String())) {
		t.Fatal("Valid() = false for deeply nested arrays")
	}

	b.WriteByte(']')
	if Valid([]byte(b.String())) {
		t.Fatal("Valid() = true for unbalanced nesting")
	}
}

func ExampleValid() {
	fmt.Println(Valid([]byte(`{"a":[1,2,3]}`)))
	fmt.Println(Valid([]byte(`{"a":[1,2,3]`)))
	// Output:
	// true
	// false
}

func FuzzValid(f *testing.F) {
	for _, seed := range validDocuments {
		f.Add([]byte(seed))
	}
	for _, seed := range []string{``, `{`, `[1,]`, `tru`, `01`, `"\u12`, `{}x`, "\"a\nb\""} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		valid := Valid(data)

		// The same input always yields the same answer.
		if again := Valid(data); again != valid {
			t.Fatalf("Valid(%q) is not deterministic", data)
		}

		// A failing scan must report a position inside the input.
		err, failed := validate(data)
		if failed == valid {
			t.Fatalf("Valid(%q) = %v but validate failed = %v", data, valid, failed)
		}
		if failed && (err.at < 0 || err.at > int64(len(data))) {
			t.Fatalf("Valid(%q) = false at position %d, want 0..%d", data, err.at, len(data))
		}

		// Surrounding whitespace does not change validity.
		wrapped := make([]byte, 0, len(data)+4)
		wrapped = append(wrapped, " \t\r\n"...)
		wrapped = append(wrapped, data...)
		wrapped = append(wrapped, " \t\r\n"...)
		if got := Valid(wrapped); got != valid {
			t.Fatalf("Valid(%q) = %v, but whitespace-wrapped input = %v", data, valid, got)
		}
	})
}
