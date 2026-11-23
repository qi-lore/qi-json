package codec

import (
	"errors"
	"testing"
)

// TestDecodeNilElementDecoder proves that decoding a non-empty container through
// a Codec whose Decode is nil is an inspectable error, not a nil-function panic,
// and that a nil container still decodes without needing a decoder.
func TestDecodeNilElementDecoder(t *testing.T) {
	type containers struct {
		S []int64
		M map[string]int64
	}
	noDecode := NewCodec[int64](nil, (*Writer).WriteInt)
	sm := Struct(
		Slice[containers, int64]("s", func(c *containers) *[]int64 { return &c.S }, noDecode),
		Map[containers, int64]("m", func(c *containers) *map[string]int64 { return &c.M }, noDecode),
	)
	if _, err := sm.Unmarshal([]byte(`{"s":[1]}`)); !errors.Is(err, errNilElementDecoder) {
		t.Errorf("slice: err = %v, want errNilElementDecoder", err)
	}
	if _, err := sm.Unmarshal([]byte(`{"m":{"k":1}}`)); !errors.Is(err, errNilElementDecoder) {
		t.Errorf("map: err = %v, want errNilElementDecoder", err)
	}
	if _, err := sm.Unmarshal([]byte(`{"s":null,"m":null}`)); err != nil {
		t.Errorf("nil containers: err = %v, want nil", err)
	}

	type fixed struct{ A [1]int64 }
	fa := Struct(Array[fixed, int64]("a", func(c *fixed) []int64 { return c.A[:] }, noDecode))
	if _, err := fa.Unmarshal([]byte(`{"a":[1]}`)); !errors.Is(err, errNilElementDecoder) {
		t.Errorf("array: err = %v, want errNilElementDecoder", err)
	}
}

// TestObjectNilSchema proves that using an Object field built with a nil
// *Schema is an inspectable error rather than a nil-pointer panic, and that a
// JSON null still decodes without needing the schema.
func TestObjectNilSchema(t *testing.T) {
	type inner struct{ N int64 }
	type outer struct{ In inner }

	schema := Struct(Object[outer, inner]("in", func(o *outer) *inner { return &o.In }, nil))

	if _, err := schema.Unmarshal([]byte(`{"in":{"n":1}}`)); !errors.Is(err, errNilSchema) {
		t.Errorf("decode: err = %v, want errNilSchema", err)
	}
	if _, err := schema.Marshal(outer{}); !errors.Is(err, errNilSchema) {
		t.Errorf("encode: err = %v, want errNilSchema", err)
	}

	got, err := schema.Unmarshal([]byte(`{"in":null}`))
	if err != nil {
		t.Fatalf("decode null: err = %v, want nil", err)
	}
	if got.In != (inner{}) {
		t.Errorf("decode null: In = %+v, want the zero value", got.In)
	}
}

// TestEncodeFieldWithoutEncoder proves that encoding through a Field whose
// Encode is nil is an inspectable error rather than a nil-function panic.
func TestEncodeFieldWithoutEncoder(t *testing.T) {
	type cfg struct{ N int64 }
	schema := Struct(Field[cfg]{
		Name:   "n",
		Decode: func(*Reader, *cfg) error { return nil },
	})
	if _, err := schema.Marshal(cfg{}); !errors.Is(err, errNoFieldEncoder) {
		t.Errorf("err = %v, want errNoFieldEncoder", err)
	}
}

// TestEncodeAnyUnsupportedDynamicType proves that an any field holding a value
// encodeAny cannot write is an inspectable error, since qijson may not use
// reflection to discover the dynamic type.
func TestEncodeAnyUnsupportedDynamicType(t *testing.T) {
	type cfg struct{ V any }
	schema := Struct(Any[cfg]("v", func(c *cfg) *any { return &c.V }))
	if _, err := schema.Marshal(cfg{V: make(chan int)}); !errors.Is(err, errUnsupportedDynamicType) {
		t.Errorf("err = %v, want errUnsupportedDynamicType", err)
	}
}
