package codec

import (
	"math"
	"testing"
)

func TestWriterScalars(t *testing.T) {
	cases := []struct {
		name string
		fn   func(*Writer) error
		want string
	}{
		{"null", func(w *Writer) error { return w.WriteNull() }, "null"},
		{"true", func(w *Writer) error { return w.WriteBool(true) }, "true"},
		{"false", func(w *Writer) error { return w.WriteBool(false) }, "false"},
		{"string", func(w *Writer) error { return w.WriteString("hi") }, `"hi"`},
		{"empty string", func(w *Writer) error { return w.WriteString("") }, `""`},
		{"int", func(w *Writer) error { return w.WriteInt(-7) }, "-7"},
		{"uint", func(w *Writer) error { return w.WriteUint(18446744073709551615) }, "18446744073709551615"},
		{"float", func(w *Writer) error { return w.WriteFloat(1.5) }, "1.5"},
		{"float integer-valued", func(w *Writer) error { return w.WriteFloat(100) }, "100"},
		{"raw", func(w *Writer) error { return w.WriteRaw([]byte(`{"a":1}`)) }, `{"a":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w Writer
			if err := tc.fn(&w); err != nil {
				t.Fatalf("write: %v", err)
			}
			if got := string(w.Bytes()); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
			if w.Err() != nil {
				t.Errorf("Err = %v, want nil", w.Err())
			}
		})
	}
}

func TestWriterStringEscaping(t *testing.T) {
	var w Writer
	// Quote, backslash, the short escapes, a bare control byte, characters that
	// encoding/json would HTML-escape but we deliberately do not, and multi-byte
	// UTF-8 that must pass through unchanged.
	if err := w.WriteString("a\"b\\c\nd\te\x01f<>&é😀"); err != nil {
		t.Fatal(err)
	}
	want := "\"a\\\"b\\\\c\\nd\\te\\u0001f<>&é😀\""
	if got := string(w.Bytes()); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestWriterStringShortEscapes(t *testing.T) {
	var w Writer
	if err := w.WriteString("\b\f\n\r\t"); err != nil {
		t.Fatal(err)
	}
	if got := string(w.Bytes()); got != `"\b\f\n\r\t"` {
		t.Errorf("got %s, want %s", got, `"\b\f\n\r\t"`)
	}
}

func TestWriterStringInvalidUTF8(t *testing.T) {
	// Each invalid byte becomes U+FFFD, matching encoding/json, so encoding is
	// total and never fails on a string.
	var w Writer
	if err := w.WriteString("a\xffb\xc3"); err != nil {
		t.Fatal(err)
	}
	want := "\"a\uFFFDb\uFFFD\""
	if got := string(w.Bytes()); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriterNonFinite(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		var w Writer
		if err := w.WriteFloat(v); err == nil {
			t.Errorf("WriteFloat(%v) err = nil, want an error", v)
		}
		if w.Bytes() != nil {
			t.Errorf("Bytes = %q, want nil after an error", w.Bytes())
		}
	}
}

func TestWriterObject(t *testing.T) {
	var w Writer
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(w.BeginObject())
	must(w.Key("a"))
	must(w.WriteInt(1))
	must(w.Key("b"))
	must(w.BeginArray())
	must(w.WriteString("x"))
	must(w.WriteNull())
	must(w.EndArray())
	must(w.EndObject())
	if got := string(w.Bytes()); got != `{"a":1,"b":["x",null]}` {
		t.Errorf("got %s", got)
	}
}

func TestWriterEmptyContainers(t *testing.T) {
	var obj Writer
	if err := obj.BeginObject(); err != nil {
		t.Fatal(err)
	}
	if err := obj.EndObject(); err != nil {
		t.Fatal(err)
	}
	if got := string(obj.Bytes()); got != `{}` {
		t.Errorf("object got %s, want {}", got)
	}

	var arr Writer
	if err := arr.BeginArray(); err != nil {
		t.Fatal(err)
	}
	if err := arr.EndArray(); err != nil {
		t.Fatal(err)
	}
	if got := string(arr.Bytes()); got != `[]` {
		t.Errorf("array got %s, want []", got)
	}
}

func TestWriterNestedContainers(t *testing.T) {
	var w Writer
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(w.BeginArray())
	must(w.BeginObject())
	must(w.Key("k"))
	must(w.BeginArray())
	must(w.WriteInt(1))
	must(w.WriteInt(2))
	must(w.EndArray())
	must(w.EndObject())
	must(w.EndArray())
	if got := string(w.Bytes()); got != `[{"k":[1,2]}]` {
		t.Errorf("got %s", got)
	}
}

func TestWriterMisuse(t *testing.T) {
	cases := []struct {
		name string
		fn   func(*Writer) error
	}{
		{"key at top level", func(w *Writer) error { return w.Key("a") }},
		{"value in object without a key", func(w *Writer) error {
			_ = w.BeginObject()
			return w.WriteInt(1)
		}},
		{"end object without begin", func(w *Writer) error { return w.EndObject() }},
		{"end array on an object", func(w *Writer) error {
			_ = w.BeginObject()
			return w.EndArray()
		}},
		{"key with no value", func(w *Writer) error {
			_ = w.BeginObject()
			_ = w.Key("a")
			return w.EndObject()
		}},
		{"two keys for one member", func(w *Writer) error {
			_ = w.BeginObject()
			_ = w.Key("a")
			return w.Key("b")
		}},
		{"two top-level values", func(w *Writer) error {
			_ = w.WriteNull()
			return w.WriteNull()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w Writer
			if err := tc.fn(&w); err == nil {
				t.Fatal("err = nil, want an error")
			}
			if w.Bytes() != nil {
				t.Errorf("Bytes = %q, want nil after an error", w.Bytes())
			}
			if w.Err() == nil {
				t.Error("Err = nil, want the sticky error")
			}
		})
	}
}

func TestWriterStickyError(t *testing.T) {
	var w Writer
	first := w.EndObject()
	if first == nil {
		t.Fatal("want an error")
	}
	if err := w.WriteNull(); err != first {
		t.Errorf("second err = %v, want the sticky first error", err)
	}
	if w.Bytes() != nil {
		t.Errorf("Bytes = %q, want nil", w.Bytes())
	}
}

func TestWriterReset(t *testing.T) {
	var w Writer
	if err := w.WriteInt(1); err != nil {
		t.Fatal(err)
	}
	w.Reset()
	if err := w.WriteInt(2); err != nil {
		t.Fatal(err)
	}
	if got := string(w.Bytes()); got != "2" {
		t.Errorf("got %s, want 2", got)
	}
}
