package codec

import (
	"errors"
	"testing"
)

// covErrEncoder is an Encoder whose method always fails.
type covErrEncoder struct{}

func (covErrEncoder) EncodeJSON(*Writer) error { return errors.New("boom") }

// covStickyEncoder is an Encoder that leaves a sticky error on the Writer but
// reports success, so the caller must still notice the Writer error.
type covStickyEncoder struct{}

func (covStickyEncoder) EncodeJSON(w *Writer) error {
	_ = w.WriteRaw(nil) // sets errEmptyRaw
	return nil
}

// TestCovNewWriter checks the constructor returns a ready writer.
func TestCovNewWriter(t *testing.T) {
	w := NewWriter()
	if err := w.WriteInt(1); err != nil {
		t.Fatalf("WriteInt: %v", err)
	}
	if got := string(w.Bytes()); got != "1" {
		t.Errorf("Bytes() = %s, want 1", got)
	}
}

// TestCovWriterStickyErrorBranches calls every write method on a Writer that
// already holds a sticky error, so each early-return branch is exercised.
func TestCovWriterStickyErrorBranches(t *testing.T) {
	poison := func() *Writer {
		w := NewWriter()
		_ = w.WriteNull() // root
		_ = w.WriteNull() // multiple roots -> sticky error
		return w
	}
	tests := []struct {
		name string
		call func(*Writer) error
	}{
		{"WriteNull", func(w *Writer) error { return w.WriteNull() }},
		{"WriteBool", func(w *Writer) error { return w.WriteBool(true) }},
		{"WriteString", func(w *Writer) error { return w.WriteString("x") }},
		{"WriteInt", func(w *Writer) error { return w.WriteInt(1) }},
		{"WriteUint", func(w *Writer) error { return w.WriteUint(1) }},
		{"WriteFloat", func(w *Writer) error { return w.WriteFloat(1) }},
		{"WriteRaw", func(w *Writer) error { return w.WriteRaw([]byte("1")) }},
		{"BeginObject", func(w *Writer) error { return w.BeginObject() }},
		{"EndObject", func(w *Writer) error { return w.EndObject() }},
		{"BeginArray", func(w *Writer) error { return w.BeginArray() }},
		{"EndArray", func(w *Writer) error { return w.EndArray() }},
		{"Key", func(w *Writer) error { return w.Key("k") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(poison()); err == nil {
				t.Errorf("%s on a failed writer = nil, want the sticky error", tt.name)
			}
		})
	}
}

// TestCovWriteFloatSecondRoot checks WriteFloat reports a second top-level value
// through its own begin-value path (not the sticky-error fast path).
func TestCovWriteFloatSecondRoot(t *testing.T) {
	w := NewWriter()
	if err := w.WriteInt(1); err != nil {
		t.Fatalf("WriteInt: %v", err)
	}
	if err := w.WriteFloat(2); err == nil {
		t.Error("WriteFloat after a root value = nil, want errMultipleRoots")
	}
}

// TestCovMarshalIndentError checks MarshalIndent surfaces an encoder failure and
// writes nothing.
func TestCovMarshalIndentError(t *testing.T) {
	data, err := MarshalIndent[covErrEncoder, *covErrEncoder](covErrEncoder{}, "", "  ")
	if err == nil {
		t.Fatal("MarshalIndent = nil error, want the encoder failure")
	}
	if data != nil {
		t.Errorf("MarshalIndent data = %q, want nil", data)
	}
}

// TestCovFormatHelpers checks the string-end and whitespace-skip helpers on edge
// input: an unterminated string and all-whitespace input.
func TestCovFormatHelpers(t *testing.T) {
	if got := jsonStringEnd([]byte(`"abc`), 0); got != 4 {
		t.Errorf("jsonStringEnd(unterminated) = %d, want 4", got)
	}
	if got := jsonStringEnd([]byte(`"a\\"`), 0); got != 5 {
		t.Errorf("jsonStringEnd(escaped backslash) = %d, want 5", got)
	}
	if got := skipJSONSpace([]byte("  \t\n"), 0); got != 4 {
		t.Errorf("skipJSONSpace(all space) = %d, want 4", got)
	}
	if got := skipJSONSpace([]byte("  x"), 0); got != 2 {
		t.Errorf("skipJSONSpace = %d, want 2", got)
	}
}

// TestCovSchemaEncodeBuildError checks a Schema built with a duplicate field name
// refuses to encode, and that a valid Schema whose Writer already holds a value
// reports the BeginObject failure.
func TestCovSchemaEncodeBuildError(t *testing.T) {
	type holder struct{ A, B int64 }
	dup := Struct(
		Int[holder]("x", func(h *holder) *int64 { return &h.A }),
		Int[holder]("x", func(h *holder) *int64 { return &h.B }),
	)
	if err := dup.Encode(NewWriter(), &holder{}); !errors.Is(err, errDuplicateFieldName) {
		t.Errorf("Encode with a duplicate field name = %v, want errDuplicateFieldName", err)
	}

	schema := Struct(Int[holder]("a", func(h *holder) *int64 { return &h.A }))
	w := NewWriter()
	_ = w.WriteNull() // a root value already written
	if err := schema.Encode(w, &holder{}); err == nil {
		t.Error("Encode with a used Writer: want a BeginObject error")
	}
}

// TestCovSchemaEncodeKeyError checks Encode reports the failure when the Writer
// already holds an error at the next field's Key call.
func TestCovSchemaEncodeKeyError(t *testing.T) {
	type holder struct{ A, B int64 }
	schema := Struct(
		Field[holder]{
			Name:   "a",
			Encode: func(w *Writer, h *holder) error { _ = w.WriteRaw(nil); return nil },
		},
		Field[holder]{
			Name:   "b",
			Encode: func(w *Writer, h *holder) error { return w.WriteInt(h.B) },
		},
	)
	if err := schema.Encode(NewWriter(), &holder{}); err == nil {
		t.Error("Encode after a field poisoned the Writer: want an error")
	}
}

// TestCovLookupEscapedKey checks a member key written with a \u escape binds to
// the field it decodes to, exercising the string-key binary search.
func TestCovLookupEscapedKey(t *testing.T) {
	type holder struct{ A, B, C int64 }
	schema := Struct(
		Int[holder]("a", func(h *holder) *int64 { return &h.A }),
		Int[holder]("b", func(h *holder) *int64 { return &h.B }),
		Int[holder]("c", func(h *holder) *int64 { return &h.C }),
	)
	var h holder
	if err := schema.UnmarshalInto([]byte(`{"\u0061":1,"\u0062":2,"\u0063":3}`), &h); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if h.A != 1 || h.B != 2 || h.C != 3 {
		t.Errorf("decoded %+v, want A=1 B=2 C=3", h)
	}
}

// TestCovEncodeMarshalStickyError checks Marshal reports a Writer error the
// Encoder left behind while returning nil.
func TestCovEncodeMarshalStickyError(t *testing.T) {
	if _, err := Marshal[covStickyEncoder, *covStickyEncoder](covStickyEncoder{}); err == nil {
		t.Error("Marshal with a sticky writer error: want an error")
	}
}

// TestCovEncodeEncoderError checks the non-generic Encode returns the Encoder's
// own error.
func TestCovEncodeEncoderError(t *testing.T) {
	if err := Encode(NewWriter(), covErrEncoder{}); err == nil {
		t.Error("Encode with a failing Encoder: want an error")
	}
}

// covUnknownTarget captures unmatched members and delegates the rest.
type covUnknownTarget struct {
	A     int64
	extra int
}

func (v *covUnknownTarget) UnmarshalUnknown(string, []byte) error {
	v.extra++
	return nil
}

// TestCovCaptureUnknownError checks captureUnknown surfaces a syntax error in the
// unmatched member's value.
func TestCovCaptureUnknownError(t *testing.T) {
	schema := Struct(Int[covUnknownTarget]("a", func(v *covUnknownTarget) *int64 { return &v.A }))
	var v covUnknownTarget
	if err := schema.UnmarshalInto([]byte(`{"a":1,"extra":}`), &v); err == nil {
		t.Error("captureUnknown with an invalid value: want a syntax error")
	}
}
