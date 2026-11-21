package qijson

import (
	"errors"
	"testing"
)

// manual is a hand-written Encoder used to exercise Marshal and Encode before
// the Schema encode layer exists.
type manual struct{ n int }

func (m *manual) EncodeJSON(w *Writer) error {
	if err := w.BeginObject(); err != nil {
		return err
	}
	if err := w.Key("n"); err != nil {
		return err
	}
	if err := w.WriteInt(int64(m.n)); err != nil {
		return err
	}
	return w.EndObject()
}

// failEnc is an Encoder that always fails, used to check error propagation.
type failEnc struct{}

func (failEnc) EncodeJSON(*Writer) error { return errNonFinite }

func TestMarshalManual(t *testing.T) {
	got, err := Marshal[manual](manual{n: 3})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(got) != `{"n":3}` {
		t.Errorf("got %s, want {\"n\":3}", got)
	}
}

func TestMarshalPropagatesError(t *testing.T) {
	_, err := Marshal[failEnc](failEnc{})
	if !errors.Is(err, errNonFinite) {
		t.Errorf("err = %v, want errNonFinite", err)
	}
}

func TestEncode(t *testing.T) {
	var w Writer
	if err := Encode(&w, &manual{n: 4}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got := string(w.Bytes()); got != `{"n":4}` {
		t.Errorf("got %s, want {\"n\":4}", got)
	}
}

func TestEncodeNil(t *testing.T) {
	var w Writer
	if err := Encode(&w, nil); err != errNilTarget {
		t.Errorf("Encode(nil encoder) = %v, want errNilTarget", err)
	}
	if err := Encode(nil, &manual{}); err != errNilWriter {
		t.Errorf("Encode(nil writer) = %v, want errNilWriter", err)
	}
}

func TestNewCodec(t *testing.T) {
	c := NewCodec((*Reader).ReadInt, func(w *Writer, v int64) error { return w.WriteInt(v) })
	if c.Decode == nil || c.Encode == nil {
		t.Fatal("NewCodec must set both directions")
	}
}
