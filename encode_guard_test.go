package qijson

import (
	"errors"
	"testing"
)

// decodeOnlyInt is a Codec with no encoder, exactly what a decode-only caller
// builds with dec (see field_test.go).
var decodeOnlyInt = dec((*Reader).ReadInt)

// TestEncodeNilElementEncoder proves that encoding a non-empty container through
// a decode-only Codec is an error, not a nil-function panic, and that a nil
// container still encodes to null without needing an encoder.
func TestEncodeNilElementEncoder(t *testing.T) {
	type containers struct {
		S []int64
		M map[string]int64
	}
	cm := Struct(
		Slice[containers, int64]("s", func(c *containers) *[]int64 { return &c.S }, decodeOnlyInt),
		Map[containers, int64]("m", func(c *containers) *map[string]int64 { return &c.M }, decodeOnlyInt),
	)
	if _, err := cm.Marshal(containers{S: []int64{1}}); err == nil {
		t.Error("slice: Marshal error = nil, want a nil element encoder error")
	}
	if _, err := cm.Marshal(containers{M: map[string]int64{"k": 1}}); err == nil {
		t.Error("map: Marshal error = nil, want a nil element encoder error")
	}
	data, err := cm.Marshal(containers{})
	if err != nil {
		t.Fatalf("nil containers: %v", err)
	}
	if string(data) != `{"s":null,"m":null}` {
		t.Errorf("nil containers: got %s, want {\"s\":null,\"m\":null}", data)
	}

	type fixed struct{ A [1]int64 }
	fa := Struct(Array[fixed, int64]("a", func(c *fixed) []int64 { return c.A[:] }, decodeOnlyInt))
	if _, err := fa.Marshal(fixed{}); err == nil {
		t.Error("array: Marshal error = nil, want a nil element encoder error")
	}
}

// TestEncodeEmptyRaw proves a nil or empty []byte Raw field encodes to null
// rather than emitting an empty (invalid) literal.
func TestEncodeEmptyRaw(t *testing.T) {
	type cfg struct{ B []byte }
	schema := Struct(Raw[cfg]("b", func(c *cfg) *[]byte { return &c.B }))
	for _, in := range []cfg{{B: nil}, {B: []byte{}}} {
		data, err := schema.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if string(data) != `{"b":null}` {
			t.Errorf("got %s, want {\"b\":null}", data)
		}
	}
}

// incompleteEnc returns nil without writing a whole JSON value.
type incompleteEnc struct{}

func (incompleteEnc) EncodeJSON(w *Writer) error { return w.BeginObject() }

// TestMarshalIncomplete proves Marshal rejects an Encoder that returned nil
// without writing exactly one complete value, rather than returning invalid JSON.
func TestMarshalIncomplete(t *testing.T) {
	_, err := Marshal[incompleteEnc](incompleteEnc{})
	if !errors.Is(err, errIncomplete) {
		t.Errorf("err = %v, want errIncomplete", err)
	}
}

// swallowErr triggers a sticky Writer error but returns nil from EncodeJSON.
type swallowErr struct{}

func (swallowErr) EncodeJSON(w *Writer) error {
	_ = w.WriteInt(1)
	_ = w.WriteInt(2) // a second top-level value sets the sticky error
	return nil
}

// TestEncodeSurfacesWriterError proves Encode returns the Writer's sticky error
// even when the Encoder itself returned nil.
func TestEncodeSurfacesWriterError(t *testing.T) {
	var w Writer
	err := Encode(&w, swallowErr{})
	if !errors.Is(err, errMultipleRoots) {
		t.Errorf("err = %v, want errMultipleRoots", err)
	}
}
