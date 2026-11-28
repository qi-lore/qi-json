package codec

import (
	"errors"
	"testing"
)

// covScalars binds one field per scalar constructor so a single round trip calls
// String, StringPtr, Int, IntPtr, Float, FloatPtr, Bool and BoolPtr in both
// directions, including the scalarPtr null and nil-pointer branches.
type covScalars struct {
	Str  string
	StrP *string
	Num  int64
	NumP *int64
	Fl   float64
	FlP  *float64
	Bo   bool
	BoP  *bool
}

var covScalarsSchema = Struct(
	String[covScalars]("str", func(c *covScalars) *string { return &c.Str }),
	StringPtr[covScalars]("strP", func(c *covScalars) **string { return &c.StrP }),
	Int[covScalars]("num", func(c *covScalars) *int64 { return &c.Num }),
	IntPtr[covScalars]("numP", func(c *covScalars) **int64 { return &c.NumP }),
	Float[covScalars]("fl", func(c *covScalars) *float64 { return &c.Fl }),
	FloatPtr[covScalars]("flP", func(c *covScalars) **float64 { return &c.FlP }),
	Bool[covScalars]("bo", func(c *covScalars) *bool { return &c.Bo }),
	BoolPtr[covScalars]("boP", func(c *covScalars) **bool { return &c.BoP }),
)

func TestCovScalarConstructors(t *testing.T) {
	const in = `{"str":"a","strP":"b","num":1,"numP":2,"fl":1.5,"flP":2.5,"bo":true,"boP":false}`
	var got covScalars
	if err := covScalarsSchema.UnmarshalInto([]byte(in), &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if got.Str != "a" || got.StrP == nil || *got.StrP != "b" {
		t.Errorf("Str/StrP = %q/%v", got.Str, got.StrP)
	}
	if got.Num != 1 || got.NumP == nil || *got.NumP != 2 {
		t.Errorf("Num/NumP = %d/%v", got.Num, got.NumP)
	}
	if got.Fl != 1.5 || got.FlP == nil || *got.FlP != 2.5 {
		t.Errorf("Fl/FlP = %v/%v", got.Fl, got.FlP)
	}
	if !got.Bo || got.BoP == nil || *got.BoP {
		t.Errorf("Bo/BoP = %v/%v", got.Bo, got.BoP)
	}

	// Encoding the decoded value reproduces the document exactly.
	data, err := covScalarsSchema.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != in {
		t.Errorf("Marshal = %s, want %s", data, in)
	}

	// A JSON null clears each pointer field.
	got = covScalars{}
	if err := covScalarsSchema.UnmarshalInto([]byte(
		`{"strP":null,"numP":null,"flP":null,"boP":null}`), &got); err != nil {
		t.Fatalf("UnmarshalInto null: %v", err)
	}
	if got.StrP != nil || got.NumP != nil || got.FlP != nil || got.BoP != nil {
		t.Errorf("nulls did not clear pointers: %+v", got)
	}

	// Encoding a zero value writes null for every nil pointer.
	data, err = covScalarsSchema.Marshal(covScalars{})
	if err != nil {
		t.Fatalf("Marshal(zero): %v", err)
	}
	want := `{"str":"","strP":null,"num":0,"numP":null,"fl":0,"flP":null,"bo":false,"boP":null}`
	if string(data) != want {
		t.Errorf("Marshal(zero) = %s, want %s", data, want)
	}
}

// TestCovScalarEmptyClosures drives the Empty closure of every scalar
// constructor through omitempty: a zero value omits the member, a non-zero value
// writes it.
func TestCovScalarEmptyClosures(t *testing.T) {
	type holder struct {
		Str  string
		StrP *string
		Num  int64
		NumP *int64
		Fl   float64
		FlP  *float64
		Bo   bool
		BoP  *bool
	}
	schema := Struct(
		String[holder]("str", func(c *holder) *string { return &c.Str }).OmitEmpty(),
		StringPtr[holder]("strP", func(c *holder) **string { return &c.StrP }).OmitEmpty(),
		Int[holder]("num", func(c *holder) *int64 { return &c.Num }).OmitEmpty(),
		IntPtr[holder]("numP", func(c *holder) **int64 { return &c.NumP }).OmitEmpty(),
		Float[holder]("fl", func(c *holder) *float64 { return &c.Fl }).OmitEmpty(),
		FloatPtr[holder]("flP", func(c *holder) **float64 { return &c.FlP }).OmitEmpty(),
		Bool[holder]("bo", func(c *holder) *bool { return &c.Bo }).OmitEmpty(),
		BoolPtr[holder]("boP", func(c *holder) **bool { return &c.BoP }).OmitEmpty(),
	)

	data, err := schema.Marshal(holder{})
	if err != nil {
		t.Fatalf("Marshal(zero): %v", err)
	}
	if string(data) != `{}` {
		t.Errorf("Marshal(zero) = %s, want {}", data)
	}

	s, n, f, b := "s", int64(1), 1.0, true
	full := holder{Str: "s", StrP: &s, Num: 1, NumP: &n, Fl: 1, FlP: &f, Bo: true, BoP: &b}
	data, err = schema.Marshal(full)
	if err != nil {
		t.Fatalf("Marshal(full): %v", err)
	}
	want := `{"str":"s","strP":"s","num":1,"numP":1,"fl":1,"flP":1,"bo":true,"boP":true}`
	if string(data) != want {
		t.Errorf("Marshal(full) = %s, want %s", data, want)
	}
}

// covPeekHolder binds one field per constructor whose Decode peeks before it
// reads, so an invalid member value drives each peek/read error branch.
type covPeekHolder struct {
	StrP *string
	Obj  fpPoint
	Dec  odInner
	DP   *odInner
	Sl   []string
	Mp   map[string]string
	Ar   [1]string
	Any  any
	Raw  []byte
}

var covPeekSchema = Struct(
	StringPtr[covPeekHolder]("strP", func(h *covPeekHolder) **string { return &h.StrP }),
	Object[covPeekHolder, fpPoint]("obj", func(h *covPeekHolder) *fpPoint { return &h.Obj }, fpPointSchema),
	ObjectDecoder[covPeekHolder, odInner]("dec", func(h *covPeekHolder) *odInner { return &h.Dec }),
	ObjectDecoderPtr[covPeekHolder, odInner]("dp", func(h *covPeekHolder) **odInner { return &h.DP }),
	Slice[covPeekHolder, string]("sl", func(h *covPeekHolder) *[]string { return &h.Sl }, dec((*Reader).ReadString)),
	Map[covPeekHolder, string]("mp", func(h *covPeekHolder) *map[string]string { return &h.Mp }, dec((*Reader).ReadString)),
	Array[covPeekHolder, string]("ar", func(h *covPeekHolder) []string { return h.Ar[:] }, dec((*Reader).ReadString)),
	Any[covPeekHolder]("any", func(h *covPeekHolder) *any { return &h.Any }),
	Raw[covPeekHolder]("raw", func(h *covPeekHolder) *[]byte { return &h.Raw }),
)

// TestCovFieldPeekErrors feeds an invalid member value to each field type, so
// the field's Decode sees a peek (or read) syntax error and returns it.
func TestCovFieldPeekErrors(t *testing.T) {
	for _, key := range []string{"strP", "obj", "dec", "dp", "sl", "mp", "ar", "any", "raw"} {
		t.Run(key, func(t *testing.T) {
			var h covPeekHolder
			if err := covPeekSchema.UnmarshalInto([]byte(`{"`+key+`":}`), &h); err == nil {
				t.Fatalf("UnmarshalInto(%s) = nil, want a syntax error", key)
			}
		})
	}
}

// TestCovScalarPtrReadError checks a pointer field whose peek succeeds but whose
// read mismatches returns a type error.
func TestCovScalarPtrReadError(t *testing.T) {
	var h covPeekHolder
	err := covPeekSchema.UnmarshalInto([]byte(`{"strP":1}`), &h)
	if err == nil {
		t.Fatal("UnmarshalInto = nil, want a type error")
	}
	var te *TypeError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As(*TypeError) = false for %T", err)
	}
	if te.Want != "string" || te.Got != "number" {
		t.Errorf("TypeError = %+v", te)
	}
}

// TestCovContainerDecodeErrors checks Map and objectRaw surface an element decode
// error and a non-string object key respectively.
func TestCovContainerDecodeErrors(t *testing.T) {
	var h covPeekHolder
	if err := covPeekSchema.UnmarshalInto([]byte(`{"mp":{"k":1}}`), &h); err == nil {
		t.Error("Map with a mistyped element: want an error")
	}
	if err := covPeekSchema.UnmarshalInto([]byte(`{1:2}`), &h); err == nil {
		t.Error("object with a non-string key: want an error")
	}
}

// TestCovObjectDecoderPtrEncode checks the pointer delegation encodes a non-nil
// field through its EncodeJSON and uses the nil check as its emptiness hook.
func TestCovObjectDecoderPtrEncode(t *testing.T) {
	type holder struct{ P *odInner }
	schema := Struct(ObjectDecoderPtr[holder, odInner]("p", func(h *holder) **odInner { return &h.P }))

	data, err := schema.Marshal(holder{P: &odInner{A: 1, B: "x"}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"p":{"a":1,"b":"x"}}` {
		t.Errorf("Marshal = %s", data)
	}

	omit := Struct(ObjectDecoderPtr[holder, odInner]("p", func(h *holder) **odInner { return &h.P }).OmitEmpty())
	data, err = omit.Marshal(holder{})
	if err != nil {
		t.Fatalf("Marshal(empty): %v", err)
	}
	if string(data) != `{}` {
		t.Errorf("Marshal(empty) = %s, want {}", data)
	}
}

// TestCovDecoderCodecDecodeError checks a DecoderCodec element whose own decode
// fails surfaces the error.
func TestCovDecoderCodecDecodeError(t *testing.T) {
	type holder struct{ List []odInner }
	schema := Struct(Slice[holder, odInner]("list",
		func(h *holder) *[]odInner { return &h.List },
		DecoderCodec[odInner, *odInner]()),
	)
	var h holder
	if err := schema.UnmarshalInto([]byte(`{"list":[1]}`), &h); err == nil {
		t.Fatal("UnmarshalInto = nil, want an element type error")
	}
}

// TestCovSliceEncodeErrors drives the Slice encode failure branches: BeginArray
// on a writer that already holds a value, and an element encoder that fails.
func TestCovSliceEncodeErrors(t *testing.T) {
	type holder struct{ V []string }
	field := Slice[holder, string]("v", func(h *holder) *[]string { return &h.V },
		NewCodec(nil, func(w *Writer, s string) error { return w.WriteString(s) }))
	h := holder{V: []string{"a"}}

	w := NewWriter()
	_ = w.WriteNull() // a root value already written
	if err := field.Encode(w, &h); err == nil {
		t.Error("Slice encode on a used writer: want an error")
	}

	boom := errors.New("boom")
	failField := Slice[holder, string]("v", func(h *holder) *[]string { return &h.V },
		NewCodec(nil, func(w *Writer, s string) error { return boom }))
	if err := failField.Encode(NewWriter(), &h); !errors.Is(err, boom) {
		t.Errorf("Slice element encode error = %v, want boom", err)
	}
}

// covLeaveKey is an element encoder that leaves a pending object key, modelling a
// misbehaving encoder whose next Key call must fail.
type covLeaveKey struct{}

func (covLeaveKey) EncodeJSON(w *Writer) error {
	if err := w.BeginObject(); err != nil {
		return err
	}
	return w.Key("p")
}

// TestCovMapEncodeErrors drives the Map encode failure branches.
func TestCovMapEncodeErrors(t *testing.T) {
	type holder struct{ V map[string]string }
	field := Map[holder, string]("v", func(h *holder) *map[string]string { return &h.V },
		NewCodec(nil, func(w *Writer, s string) error { return w.WriteString(s) }))
	h := holder{V: map[string]string{"a": "x"}}

	w := NewWriter()
	_ = w.WriteNull()
	if err := field.Encode(w, &h); err == nil {
		t.Error("Map encode on a used writer: want an error")
	}

	boom := errors.New("boom")
	failField := Map[holder, string]("v", func(h *holder) *map[string]string { return &h.V },
		NewCodec(nil, func(w *Writer, s string) error { return boom }))
	if err := failField.Encode(NewWriter(), &h); !errors.Is(err, boom) {
		t.Errorf("Map element encode error = %v, want boom", err)
	}

	// Two members: the first value leaves a pending key, so the second Key fails.
	leave := func(w *Writer, s string) error {
		if err := w.BeginObject(); err != nil {
			return err
		}
		return w.Key("p")
	}
	leaveField := Map[holder, string]("v", func(h *holder) *map[string]string { return &h.V }, NewCodec(nil, leave))
	h2 := holder{V: map[string]string{"a": "", "b": ""}}
	if err := leaveField.Encode(NewWriter(), &h2); err == nil {
		t.Error("Map second Key after a pending key: want an error")
	}
}

// TestCovArrayEncodeErrors drives the Array encode failure branches.
func TestCovArrayEncodeErrors(t *testing.T) {
	type holder struct{ V [1]string }
	field := Array[holder, string]("v", func(h *holder) []string { return h.V[:] },
		NewCodec(nil, func(w *Writer, s string) error { return w.WriteString(s) }))
	h := holder{V: [1]string{"a"}}

	w := NewWriter()
	_ = w.WriteNull()
	if err := field.Encode(w, &h); err == nil {
		t.Error("Array encode on a used writer: want an error")
	}

	boom := errors.New("boom")
	failField := Array[holder, string]("v", func(h *holder) []string { return h.V[:] },
		NewCodec(nil, func(w *Writer, s string) error { return boom }))
	if err := failField.Encode(NewWriter(), &h); !errors.Is(err, boom) {
		t.Errorf("Array element encode error = %v, want boom", err)
	}
}

// TestCovArrayDecode checks Array's null branch, an element decode error and a
// length underflow.
func TestCovArrayDecode(t *testing.T) {
	type holder struct{ V [2]string }
	schema := Struct(Array[holder, string]("v", func(h *holder) []string { return h.V[:] }, dec((*Reader).ReadString)))

	h := holder{V: [2]string{"keep", "me"}}
	if err := schema.UnmarshalInto([]byte(`{"v":null}`), &h); err != nil {
		t.Fatalf("UnmarshalInto(null): %v", err)
	}
	if h.V != [2]string{"keep", "me"} {
		t.Errorf("null changed the array: %v", h.V)
	}

	var h2 holder
	if err := schema.UnmarshalInto([]byte(`{"v":[1,2]}`), &h2); err == nil {
		t.Error("element decode error: want an error")
	}

	var h3 holder
	err := schema.UnmarshalInto([]byte(`{"v":["only"]}`), &h3)
	var te *TypeError
	if !errors.As(err, &te) {
		t.Fatalf("underflow: errors.As(*TypeError) = false for %T", err)
	}
	if te.Want != "array of length 2" || te.Got != "array of length 1" {
		t.Errorf("TypeError = %+v", te)
	}
}

// TestCovAnyEmptyAndInnerError drives Any's empty hook and an error raised inside
// a decoded array element.
func TestCovAnyEmptyAndInnerError(t *testing.T) {
	type holder struct{ V any }
	schema := Struct(Any[holder]("v", func(h *holder) *any { return &h.V }).OmitEmpty())

	data, err := schema.Marshal(holder{})
	if err != nil {
		t.Fatalf("Marshal(empty): %v", err)
	}
	if string(data) != `{}` {
		t.Errorf("Marshal(empty) = %s, want {}", data)
	}
	data, err = schema.Marshal(holder{V: 1})
	if err != nil {
		t.Fatalf("Marshal(value): %v", err)
	}
	if string(data) != `{"v":1}` {
		t.Errorf("Marshal(value) = %s", data)
	}

	var h holder
	if err := schema.UnmarshalInto([]byte(`{"v":[[1}`), &h); err == nil {
		t.Error("nested unterminated array: want an error")
	}
}

// TestCovRawEmpty drives Raw's empty hook.
func TestCovRawEmpty(t *testing.T) {
	type holder struct{ V []byte }
	schema := Struct(Raw[holder]("v", func(h *holder) *[]byte { return &h.V }).OmitEmpty())

	data, err := schema.Marshal(holder{})
	if err != nil {
		t.Fatalf("Marshal(empty): %v", err)
	}
	if string(data) != `{}` {
		t.Errorf("Marshal(empty) = %s, want {}", data)
	}
	data, err = schema.Marshal(holder{V: []byte(`1`)})
	if err != nil {
		t.Fatalf("Marshal(value): %v", err)
	}
	if string(data) != `{"v":1}` {
		t.Errorf("Marshal(value) = %s", data)
	}
}

// covUnencodable is a dynamic type encodeAny does not know, since it implements
// no branch of the switch.
type covUnencodable struct{ X int }

// TestCovEncodeAnyWidths checks every numeric width encodeAny handles writes its
// natural value.
func TestCovEncodeAnyWidths(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"bool", true, "true"},
		{"string", "hi", `"hi"`},
		{"float64", 1.5, "1.5"},
		{"float32", float32(1.5), "1.5"},
		{"int", int(-1), "-1"},
		{"int8", int8(-2), "-2"},
		{"int16", int16(-3), "-3"},
		{"int32", int32(-4), "-4"},
		{"int64", int64(-5), "-5"},
		{"uint", uint(6), "6"},
		{"uint8", uint8(7), "7"},
		{"uint16", uint16(8), "8"},
		{"uint32", uint32(9), "9"},
		{"uint64", uint64(10), "10"},
		{"nil", nil, "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewWriter()
			if err := encodeAny(w, tt.v); err != nil {
				t.Fatalf("encodeAny(%v): %v", tt.v, err)
			}
			if got := string(w.Bytes()); got != tt.want {
				t.Errorf("encodeAny(%v) = %s, want %s", tt.v, got, tt.want)
			}
		})
	}
}

// TestCovEncodeAnyContainers checks the []any and map[string]any branches and
// the Encoder branch.
func TestCovEncodeAnyContainers(t *testing.T) {
	w := NewWriter()
	if err := encodeAny(w, []any{1, "two", true}); err != nil {
		t.Fatalf("encodeAny([]any): %v", err)
	}
	if got, want := string(w.Bytes()), `[1,"two",true]`; got != want {
		t.Errorf("encodeAny([]any) = %s, want %s", got, want)
	}

	w = NewWriter()
	if err := encodeAny(w, map[string]any{"b": 2, "a": 1}); err != nil {
		t.Fatalf("encodeAny(map): %v", err)
	}
	if got, want := string(w.Bytes()), `{"a":1,"b":2}`; got != want {
		t.Errorf("encodeAny(map) = %s, want %s", got, want)
	}

	w = NewWriter()
	if err := encodeAny(w, &odInner{A: 1, B: "x"}); err != nil {
		t.Fatalf("encodeAny(Encoder): %v", err)
	}
	if got, want := string(w.Bytes()), `{"a":1,"b":"x"}`; got != want {
		t.Errorf("encodeAny(Encoder) = %s, want %s", got, want)
	}
}

// TestCovEncodeAnyErrors drives every failure branch of encodeAny: a Begin that
// fails on a used writer, a failing element, a failing map value and a Key that
// follows a pending key.
func TestCovEncodeAnyErrors(t *testing.T) {
	used := func() *Writer {
		w := NewWriter()
		_ = w.WriteNull()
		return w
	}

	if err := encodeAny(used(), []any{1}); err == nil {
		t.Error("[]any BeginArray on a used writer: want an error")
	}
	if err := encodeAny(NewWriter(), []any{covUnencodable{}}); err == nil {
		t.Error("[]any unencodable element: want an error")
	}
	if err := encodeAny(used(), map[string]any{"k": 1}); err == nil {
		t.Error("map BeginObject on a used writer: want an error")
	}
	if err := encodeAny(NewWriter(), map[string]any{"k": covUnencodable{}}); err == nil {
		t.Error("map unencodable value: want an error")
	}
	if err := encodeAny(NewWriter(), map[string]any{"a": covLeaveKey{}, "b": 1}); err == nil {
		t.Error("map Key after a pending key: want an error")
	}
}

// TestCovObjectNilSchemaEncode checks an Object field with a nil schema reports
// the misuse on encode as well as on decode.
func TestCovObjectNilSchemaEncode(t *testing.T) {
	type holder struct{ V fpPoint }
	schema := Struct(Object[holder, fpPoint]("v", func(h *holder) *fpPoint { return &h.V }, nil))
	if _, err := schema.Marshal(holder{}); !errors.Is(err, errNilSchema) {
		t.Errorf("Object nil schema encode = %v, want errNilSchema", err)
	}
}
