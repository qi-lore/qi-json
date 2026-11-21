package qijson

import "testing"

// odInner is a qi-json-aware type: it implements Decoder and Encoder by
// delegating to its own Schema, the exact shape the code generator emits for a
// foreign type that is qi-json-aware. The delegation constructors bind a field
// or element of this type by calling those methods.
type odInner struct {
	A int64
	B string
}

var odInnerSchema = Struct(
	Int[odInner]("a", func(v *odInner) *int64 { return &v.A }),
	String[odInner]("b", func(v *odInner) *string { return &v.B }),
)

func (v *odInner) DecodeJSON(r *Reader) error { return odInnerSchema.Decode(r, v) }
func (v *odInner) EncodeJSON(w *Writer) error { return odInnerSchema.Encode(w, v) }

// odOwner has ten nested fields, so the round-trip test exercises more than one
// delegation and proves declaration order is preserved on encode.
type odOwner struct {
	One   odInner
	Two   odInner
	Three odInner
	Four  odInner
	Five  odInner
	Six   odInner
	Seven odInner
	Eight odInner
	Nine  odInner
	Ten   odInner
}

func odOwnerSchema() *Schema[odOwner] {
	return Struct(
		ObjectDecoder("one", func(v *odOwner) *odInner { return &v.One }),
		ObjectDecoder("two", func(v *odOwner) *odInner { return &v.Two }),
		ObjectDecoder("three", func(v *odOwner) *odInner { return &v.Three }),
		ObjectDecoder("four", func(v *odOwner) *odInner { return &v.Four }),
		ObjectDecoder("five", func(v *odOwner) *odInner { return &v.Five }),
		ObjectDecoder("six", func(v *odOwner) *odInner { return &v.Six }),
		ObjectDecoder("seven", func(v *odOwner) *odInner { return &v.Seven }),
		ObjectDecoder("eight", func(v *odOwner) *odInner { return &v.Eight }),
		ObjectDecoder("nine", func(v *odOwner) *odInner { return &v.Nine }),
		ObjectDecoder("ten", func(v *odOwner) *odInner { return &v.Ten }),
	)
}

// TestObjectDecoderDecodesValueField checks the value form decodes through the
// nested type's own DecodeJSON.
func TestObjectDecoderDecodesValueField(t *testing.T) {
	schema := Struct(
		ObjectDecoder("inner", func(v *odOwner) *odInner { return &v.One }),
	)
	var got odOwner
	if err := schema.UnmarshalInto([]byte(`{"inner":{"a":42,"b":"hi"}}`), &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if got.One != (odInner{A: 42, B: "hi"}) {
		t.Errorf("One = %+v, want {42 hi}", got.One)
	}
}

// TestObjectDecoderNullLeavesValue checks that a JSON null leaves a value field
// unchanged, matching Object.
func TestObjectDecoderNullLeavesValue(t *testing.T) {
	schema := Struct(
		ObjectDecoder("inner", func(v *odOwner) *odInner { return &v.One }),
	)
	got := odOwner{One: odInner{A: 7, B: "keep"}}
	if err := schema.UnmarshalInto([]byte(`{"inner":null}`), &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if got.One != (odInner{A: 7, B: "keep"}) {
		t.Errorf("One = %+v, want it unchanged", got.One)
	}
}

// TestObjectDecoderRoundTrip encodes and decodes a ten-field document, proving
// both directions and declaration order.
func TestObjectDecoderRoundTrip(t *testing.T) {
	schema := odOwnerSchema()
	in := odOwner{
		One:   odInner{A: 1, B: "a"},
		Two:   odInner{A: 2, B: "b"},
		Three: odInner{A: 3, B: "c"},
		Four:  odInner{A: 4, B: "d"},
		Five:  odInner{A: 5, B: "e"},
		Six:   odInner{A: 6, B: "f"},
		Seven: odInner{A: 7, B: "g"},
		Eight: odInner{A: 8, B: "h"},
		Nine:  odInner{A: 9, B: "i"},
		Ten:   odInner{A: 10, B: "j"},
	}
	data, err := schema.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"one":{"a":1,"b":"a"},"two":{"a":2,"b":"b"},"three":{"a":3,"b":"c"},` +
		`"four":{"a":4,"b":"d"},"five":{"a":5,"b":"e"},"six":{"a":6,"b":"f"},` +
		`"seven":{"a":7,"b":"g"},"eight":{"a":8,"b":"h"},"nine":{"a":9,"b":"i"},` +
		`"ten":{"a":10,"b":"j"}}`
	if string(data) != want {
		t.Errorf("Marshal:\n got %s\nwant %s", data, want)
	}
	var back odOwner
	if err := schema.UnmarshalInto(data, &back); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if back != in {
		t.Errorf("round trip = %+v, want %+v", back, in)
	}
}

// odPtrOwner holds a pointer to a qi-json-aware type.
type odPtrOwner struct {
	Inner *odInner
}

// TestObjectDecoderPtr checks the pointer form: a value allocates and decodes, a
// JSON null sets nil, and a nil encodes as null.
func TestObjectDecoderPtr(t *testing.T) {
	schema := Struct(
		ObjectDecoderPtr("inner", func(v *odPtrOwner) **odInner { return &v.Inner }),
	)

	var got odPtrOwner
	if err := schema.UnmarshalInto([]byte(`{"inner":{"a":1,"b":"x"}}`), &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if got.Inner == nil || *got.Inner != (odInner{A: 1, B: "x"}) {
		t.Fatalf("Inner = %v, want {1 x}", got.Inner)
	}

	if err := schema.UnmarshalInto([]byte(`{"inner":null}`), &got); err != nil {
		t.Fatalf("UnmarshalInto null: %v", err)
	}
	if got.Inner != nil {
		t.Errorf("Inner = %v, want nil after null", got.Inner)
	}

	data, err := schema.Marshal(odPtrOwner{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"inner":null}` {
		t.Errorf("Marshal = %s, want {\"inner\":null}", data)
	}
}

// odContainer holds a slice and a map of a qi-json-aware type, decoded with the
// element codec DecoderCodec produces.
type odContainer struct {
	List []odInner
	M    map[string]odInner
}

// TestDecoderCodecContainers checks DecoderCodec round-trips a []S and a map
// string to S through the element type's own methods.
func TestDecoderCodecContainers(t *testing.T) {
	schema := Struct(
		Slice("list", func(v *odContainer) *[]odInner { return &v.List }, DecoderCodec[odInner, *odInner]()),
		Map("m", func(v *odContainer) *map[string]odInner { return &v.M }, DecoderCodec[odInner, *odInner]()),
	)
	var got odContainer
	if err := schema.UnmarshalInto([]byte(`{"list":[{"a":1,"b":"x"},{"a":2,"b":"y"}],"m":{"k":{"a":3,"b":"z"}}}`), &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if len(got.List) != 2 || got.List[0] != (odInner{A: 1, B: "x"}) || got.List[1] != (odInner{A: 2, B: "y"}) {
		t.Errorf("List = %v", got.List)
	}
	if got.M["k"] != (odInner{A: 3, B: "z"}) {
		t.Errorf("M = %v", got.M)
	}

	data, err := schema.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"list":[{"a":1,"b":"x"},{"a":2,"b":"y"}],"m":{"k":{"a":3,"b":"z"}}}` {
		t.Errorf("Marshal = %s", data)
	}
}

// TestDecoderCodecArray checks the fixed-size array form uses the same element
// codec.
func TestDecoderCodecArray(t *testing.T) {
	type arrOwner struct {
		Pair [2]odInner
	}
	schema := Struct(
		Array("pair", func(v *arrOwner) []odInner { return v.Pair[:] }, DecoderCodec[odInner, *odInner]()),
	)
	var got arrOwner
	if err := schema.UnmarshalInto([]byte(`{"pair":[{"a":1,"b":"x"},{"a":2,"b":"y"}]}`), &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if got.Pair != [2]odInner{{A: 1, B: "x"}, {A: 2, B: "y"}} {
		t.Errorf("Pair = %v", got.Pair)
	}
}
