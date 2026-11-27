package codec

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// goldenIndented is the expected output of Indent and MarshalIndent with an
// empty prefix and a two-space indent for the fmtDoc document below.
const goldenIndented = `{
  "a": 1,
  "b": {
    "c": [
      1,
      2
    ]
  },
  "d": {}
}`

type fmtEmpty struct{}

type fmtInner struct {
	C []int64
}

type fmtDoc struct {
	A int64
	B fmtInner
	D fmtEmpty
}

// fmtInt64 is an element Codec for []int64, since the container constructors
// need both directions of the element.
var fmtInt64 = NewCodec(
	func(r *Reader) (int64, error) { return r.ReadInt() },
	func(w *Writer, v int64) error { return w.WriteInt(v) },
)

var fmtInnerSchema = Struct(
	Slice[fmtInner, int64]("c", func(v *fmtInner) *[]int64 { return &v.C }, fmtInt64),
)

var fmtEmptySchema = Struct[fmtEmpty]()

var fmtDocSchema = Struct(
	Int[fmtDoc]("a", func(v *fmtDoc) *int64 { return &v.A }),
	Object[fmtDoc, fmtInner]("b", func(v *fmtDoc) *fmtInner { return &v.B }, fmtInnerSchema),
	Object[fmtDoc, fmtEmpty]("d", func(v *fmtDoc) *fmtEmpty { return &v.D }, fmtEmptySchema),
)

func (v *fmtDoc) EncodeJSON(w *Writer) error { return fmtDocSchema.Encode(w, v) }

// fmtGoldenDoc is the value whose encoding is goldenIndented.
func fmtGoldenDoc() fmtDoc {
	return fmtDoc{A: 1, B: fmtInner{C: []int64{1, 2}}}
}

func TestIndent(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		prefix string
		indent string
		want   string
	}{
		{
			name:   "nested object and array with empty container",
			src:    `{"a":1,"b":{"c":[1,2]},"d":{}}`,
			prefix: "",
			indent: "  ",
			want:   goldenIndented,
		},
		{
			name:   "empty containers stay inline",
			src:    `{"a":[],"b":{}}`,
			prefix: "",
			indent: "  ",
			want:   "{\n  \"a\": [],\n  \"b\": {}\n}",
		},
		{
			name:   "prefix and tab indent",
			src:    `[1,2]`,
			prefix: ">",
			indent: "\t",
			want:   "[\n>\t1,\n>\t2\n>]",
		},
		{
			name:   "input whitespace is normalised",
			src:    "{ \"a\" : [ 1 , 2 ] , \"b\" : { } }",
			prefix: "",
			indent: "  ",
			want:   "{\n  \"a\": [\n    1,\n    2\n  ],\n  \"b\": {}\n}",
		},
		{
			name:   "string bytes are preserved verbatim",
			src:    `{"a b":"c d\n"}`,
			prefix: "",
			indent: "  ",
			want:   "{\n  \"a b\": \"c d\\n\"\n}",
		},
		{
			name:   "top-level scalar",
			src:    `   1  `,
			prefix: "",
			indent: "  ",
			want:   "1",
		},
		{
			name:   "top-level empty array",
			src:    `[]`,
			prefix: "",
			indent: "  ",
			want:   "[]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dst bytes.Buffer
			if err := Indent(&dst, []byte(tc.src), tc.prefix, tc.indent); err != nil {
				t.Fatalf("Indent: %v", err)
			}
			if got := dst.String(); got != tc.want {
				t.Errorf("Indent = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIndentMalformed(t *testing.T) {
	cases := []string{
		`{"a":}`,
		`[1,2`,
		`{"a":1}extra`,
		``,
		`{"a":1,}`,
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			dst := bytes.NewBufferString("keep")
			err := Indent(dst, []byte(src), "", "  ")
			if err == nil {
				t.Fatalf("Indent(%q) = nil, want an error", src)
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("Indent error type = %T, want *Error", err)
			}
			if e.Kind != KindSyntax {
				t.Errorf("Kind = %v, want %v", e.Kind, KindSyntax)
			}
			if got := dst.String(); got != "keep" {
				t.Errorf("dst = %q, want the buffer unchanged at %q", got, "keep")
			}
		})
	}
}

func TestCompact(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"whitespace between tokens", " { \"a\" : 1 ,\n\t\"b\" : [ 1 , 2 ] } ", `{"a":1,"b":[1,2]}`},
		{"whitespace inside a string is kept", `{ "a" : "x y\tz" }`, `{"a":"x y\tz"}`},
		{"empty containers", `{ "a" : [ ] , "b" : { } }`, `{"a":[],"b":{}}`},
		{"scalars", "  true  ", "true"},
		{"already compact", `{"a":1}`, `{"a":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dst bytes.Buffer
			if err := Compact(&dst, []byte(tc.src)); err != nil {
				t.Fatalf("Compact: %v", err)
			}
			if got := dst.String(); got != tc.want {
				t.Errorf("Compact = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCompactAppends(t *testing.T) {
	var dst bytes.Buffer
	dst.WriteString("prefix:")
	if err := Compact(&dst, []byte(`{ "a" : 1 }`)); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if got := dst.String(); got != `prefix:{"a":1}` {
		t.Errorf("Compact = %q, want %q", got, `prefix:{"a":1}`)
	}
}

func TestCompactMalformed(t *testing.T) {
	for _, src := range []string{`{"a":}`, `[1,2`, `nul`, `{"a":1,}`} {
		t.Run(src, func(t *testing.T) {
			dst := bytes.NewBufferString("keep")
			err := Compact(dst, []byte(src))
			if err == nil {
				t.Fatalf("Compact(%q) = nil, want an error", src)
			}
			var e *Error
			if !errors.As(err, &e) || e.Kind != KindSyntax {
				t.Fatalf("Compact error = %v, want a *Error of Kind %v", err, KindSyntax)
			}
			if got := dst.String(); got != "keep" {
				t.Errorf("dst = %q, want the buffer unchanged", got)
			}
		})
	}
}

func TestHTMLEscape(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"angle and ampersand", `{"a":"<b>&c>"}`, `{"a":"\u003cb\u003e\u0026c\u003e"}`},
		{"plain text untouched", `{"a":1,"b":"plain"}`, `{"a":1,"b":"plain"}`},
		{"line separator", "\"x\u2028y\"", `"x\u2028y"`},
		{"paragraph separator", "\"x\u2029y\"", `"x\u2029y"`},
		{"multi-byte UTF-8 untouched", `"é😀"`, `"é😀"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dst bytes.Buffer
			HTMLEscape(&dst, []byte(tc.src))
			if got := dst.String(); got != tc.want {
				t.Errorf("HTMLEscape = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHTMLEscapeAppends(t *testing.T) {
	var dst bytes.Buffer
	dst.WriteString("prefix:")
	HTMLEscape(&dst, []byte(`"<"`))
	if got := dst.String(); got != `prefix:"\u003c"` {
		t.Errorf("HTMLEscape = %q, want %q", got, `prefix:"\u003c"`)
	}
}

func TestMarshalIndentGolden(t *testing.T) {
	data, err := MarshalIndent[fmtDoc, *fmtDoc](fmtGoldenDoc(), "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if got := string(data); got != goldenIndented {
		t.Errorf("MarshalIndent =\n%s\nwant\n%s", got, goldenIndented)
	}
}

func TestMarshalIndentPrefix(t *testing.T) {
	data, err := MarshalIndent[fmtDoc, *fmtDoc](fmtEmptyDoc(), ">", "\t")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	want := "{\n>\t\"a\": 0,\n>\t\"b\": {\n>\t\t\"c\": null\n>\t},\n>\t\"d\": {}\n>}"
	if got := string(data); got != want {
		t.Errorf("MarshalIndent = %q, want %q", got, want)
	}
}

// fmtEmptyDoc is a fmtDoc whose slice field is nil, to pin the null and empty
// object branches under indentation.
func fmtEmptyDoc() fmtDoc { return fmtDoc{} }

func TestMarshalIndentDoesNotEscapeHTML(t *testing.T) {
	// MarshalIndent must follow Marshal's default and leave HTML bytes alone.
	data, err := MarshalIndent[htmlValue, *htmlValue](htmlValue{S: "<b>"}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if got := string(data); got != "{\n  \"s\": \"<b>\"\n}" {
		t.Errorf("MarshalIndent = %q, want %q", got, "{\n  \"s\": \"<b>\"\n}")
	}
}

type htmlValue struct {
	S string
}

var htmlSchema = Struct(
	String[htmlValue]("s", func(v *htmlValue) *string { return &v.S }),
)

func (v *htmlValue) EncodeJSON(w *Writer) error { return htmlSchema.Encode(w, v) }
func (v *htmlValue) DecodeJSON(r *Reader) error { return htmlSchema.Decode(r, v) }

func TestWriterSetIndentGolden(t *testing.T) {
	var w Writer
	w.SetIndent("", "  ")
	if err := fmtDocSchema.Encode(&w, ptr(fmtGoldenDoc())); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got := string(w.Bytes()); got != goldenIndented {
		t.Errorf("indented encoding =\n%s\nwant\n%s", got, goldenIndented)
	}
}

func TestWriterSetIndentDisabled(t *testing.T) {
	var w Writer
	w.SetIndent("", "  ")
	w.SetIndent("", "")
	if err := htmlSchema.Encode(&w, ptr(htmlValue{S: "x"})); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got := string(w.Bytes()); got != `{"s":"x"}` {
		t.Errorf("disabled indentation = %q, want %q", got, `{"s":"x"}`)
	}
}

func TestWriterSetIndentPersistsAcrossReset(t *testing.T) {
	var w Writer
	w.SetIndent("", "  ")
	if err := htmlSchema.Encode(&w, ptr(htmlValue{S: "x"})); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got := string(w.Bytes()); got != "{\n  \"s\": \"x\"\n}" {
		t.Fatalf("first Encode = %q", got)
	}
	w.Reset()
	if err := htmlSchema.Encode(&w, ptr(htmlValue{S: "y"})); err != nil {
		t.Fatalf("Encode after Reset: %v", err)
	}
	if got := string(w.Bytes()); got != "{\n  \"s\": \"y\"\n}" {
		t.Errorf("Encode after Reset = %q, want indentation preserved", got)
	}
}

func TestWriterSetIndentPersistsAcrossDumpTo(t *testing.T) {
	var w Writer
	w.SetIndent("", "  ")
	for _, s := range []string{"x", "y"} {
		if err := htmlSchema.Encode(&w, ptr(htmlValue{S: s})); err != nil {
			t.Fatalf("Encode(%q): %v", s, err)
		}
		var out bytes.Buffer
		if _, err := w.DumpTo(&out); err != nil {
			t.Fatalf("DumpTo(%q): %v", s, err)
		}
		want := "{\n  \"s\": \"" + s + "\"\n}"
		if got := out.String(); got != want {
			t.Errorf("DumpTo(%q) = %q, want %q", s, got, want)
		}
	}
}

func TestWriterSetEscapeHTML(t *testing.T) {
	// Default off: HTML bytes pass through.
	var plain Writer
	if err := plain.WriteString("<>&"); err != nil {
		t.Fatal(err)
	}
	if got := string(plain.Bytes()); got != `"<>&"` {
		t.Errorf("default WriteString = %q, want %q", got, `"<>&"`)
	}

	// On: values and keys escape.
	var w Writer
	w.SetEscapeHTML(true)
	if err := w.WriteString("<>&"); err != nil {
		t.Fatal(err)
	}
	if got := string(w.Bytes()); got != `"\u003c\u003e\u0026"` {
		t.Errorf("escaped WriteString = %q, want %q", got, `"\u003c\u003e\u0026"`)
	}

	var obj Writer
	obj.SetEscapeHTML(true)
	if err := obj.BeginObject(); err != nil {
		t.Fatal(err)
	}
	if err := obj.Key("a<b"); err != nil {
		t.Fatal(err)
	}
	if err := obj.WriteString("c>d"); err != nil {
		t.Fatal(err)
	}
	if err := obj.EndObject(); err != nil {
		t.Fatal(err)
	}
	if got := string(obj.Bytes()); got != `{"a\u003cb":"c\u003ed"}` {
		t.Errorf("escaped key/value = %q, want %q", got, `{"a\u003cb":"c\u003ed"}`)
	}
}

func TestWriterSetEscapeHTMLSeparators(t *testing.T) {
	var w Writer
	w.SetEscapeHTML(true)
	if err := w.WriteString("a\u2028b\u2029c"); err != nil {
		t.Fatal(err)
	}
	if got := string(w.Bytes()); got != `"a\u2028b\u2029c"` {
		t.Errorf("escaped separators = %q, want %q", got, `"a\u2028b\u2029c"`)
	}
}

func TestWriterSetEscapeHTMLPersistsAcrossReset(t *testing.T) {
	var w Writer
	w.SetEscapeHTML(true)
	if err := w.WriteString("<"); err != nil {
		t.Fatal(err)
	}
	if got := string(w.Bytes()); got != `"\u003c"` {
		t.Fatalf("first WriteString = %q", got)
	}
	w.Reset()
	if err := w.WriteString(">"); err != nil {
		t.Fatal(err)
	}
	if got := string(w.Bytes()); got != `"\u003e"` {
		t.Errorf("WriteString after Reset = %q, want escaping preserved", got)
	}
}

func TestWriterIndentAndEscapeCombined(t *testing.T) {
	var w Writer
	w.SetIndent("", "  ")
	w.SetEscapeHTML(true)
	if err := htmlSchema.Encode(&w, ptr(htmlValue{S: "<x>"})); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want := "{\n  \"s\": \"\\u003cx\\u003e\"\n}"
	if got := string(w.Bytes()); got != want {
		t.Errorf("combined = %q, want %q", got, want)
	}
}

func TestIndentAndCompactIdempotent(t *testing.T) {
	// Indenting then compacting round-trips to the compact form, and a second
	// indent of already-indented output is a no-op.
	compactIn := `{"a":1,"b":{"c":[1,2]},"d":{}}`
	var indented bytes.Buffer
	if err := Indent(&indented, []byte(compactIn), "", "  "); err != nil {
		t.Fatalf("Indent: %v", err)
	}
	var again bytes.Buffer
	if err := Indent(&again, indented.Bytes(), "", "  "); err != nil {
		t.Fatalf("Indent(2): %v", err)
	}
	if again.String() != indented.String() {
		t.Errorf("Indent is not idempotent:\n%s\nvs\n%s", indented.String(), again.String())
	}
	if !strings.Contains(indented.String(), "\n  \"a\": 1") {
		t.Errorf("Indent output missing expected indentation: %q", indented.String())
	}
	var compact bytes.Buffer
	if err := Compact(&compact, indented.Bytes()); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if compact.String() != compactIn {
		t.Errorf("Indent then Compact = %q, want %q", compact.String(), compactIn)
	}
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }
