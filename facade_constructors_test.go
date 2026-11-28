package qijson_test

import (
	"errors"
	"testing"

	"github.com/qi-lore/qi-json"
)

// facadePoint is a nested qi-json-aware type reached through the facade alone:
// its schema, its methods and its field closures name qijson, never codec. It
// drives the delegation constructors (ObjectDecoder, ObjectDecoderPtr and
// DecoderCodec).
type facadePoint struct {
	X int64
	Y int64
}

var facadePointSchema = qijson.Struct(
	qijson.Int[facadePoint]("x", func(p *facadePoint) *int64 { return &p.X }),
	qijson.Int[facadePoint]("y", func(p *facadePoint) *int64 { return &p.Y }),
)

func (p *facadePoint) DecodeJSON(r *qijson.Reader) error { return facadePointSchema.Decode(r, p) }

func (p *facadePoint) EncodeJSON(w *qijson.Writer) error { return facadePointSchema.Encode(w, p) }

// facadeIntCodec and facadeStringCodec are element codecs built with NewCodec,
// the facade's own pairing of a decode and an encode function, so Slice, Map and
// Array carry a Codec that is not provided by the delegation constructor.
var (
	facadeIntCodec = qijson.NewCodec(
		func(r *qijson.Reader) (int64, error) { return r.ReadInt() },
		func(w *qijson.Writer, v int64) error { return w.WriteInt(v) },
	)
	facadeStringCodec = qijson.NewCodec(
		func(r *qijson.Reader) (string, error) { return r.ReadString() },
		func(w *qijson.Writer, v string) error { return w.WriteString(v) },
	)
)

// facadeHolder binds one field per facade constructor, so a single round trip
// calls every constructor's closures in both directions.
type facadeHolder struct {
	Name      string
	NamePtr   *string
	Count     int64
	CountPtr  *int64
	Rate      float64
	RatePtr   *float64
	On        bool
	OnPtr     *bool
	Origin    facadePoint
	Nested    facadePoint
	NestedPtr *facadePoint
	Tags      []string
	Index     map[string]int64
	Pair      [2]int64
	Points    []facadePoint
	Anything  any
	Blob      []byte
}

var facadeHolderSchema = qijson.Struct(
	qijson.String[facadeHolder]("name", func(h *facadeHolder) *string { return &h.Name }),
	qijson.StringPtr[facadeHolder]("namePtr", func(h *facadeHolder) **string { return &h.NamePtr }),
	qijson.Int[facadeHolder]("count", func(h *facadeHolder) *int64 { return &h.Count }),
	qijson.IntPtr[facadeHolder]("countPtr", func(h *facadeHolder) **int64 { return &h.CountPtr }),
	qijson.Float[facadeHolder]("rate", func(h *facadeHolder) *float64 { return &h.Rate }),
	qijson.FloatPtr[facadeHolder]("ratePtr", func(h *facadeHolder) **float64 { return &h.RatePtr }),
	qijson.Bool[facadeHolder]("on", func(h *facadeHolder) *bool { return &h.On }),
	qijson.BoolPtr[facadeHolder]("onPtr", func(h *facadeHolder) **bool { return &h.OnPtr }),
	qijson.Object[facadeHolder, facadePoint]("origin", func(h *facadeHolder) *facadePoint { return &h.Origin }, facadePointSchema),
	qijson.ObjectDecoder[facadeHolder, facadePoint]("nested", func(h *facadeHolder) *facadePoint { return &h.Nested }),
	qijson.ObjectDecoderPtr[facadeHolder, facadePoint]("nestedPtr", func(h *facadeHolder) **facadePoint { return &h.NestedPtr }),
	qijson.Slice[facadeHolder, string]("tags", func(h *facadeHolder) *[]string { return &h.Tags }, facadeStringCodec),
	qijson.Map[facadeHolder, int64]("index", func(h *facadeHolder) *map[string]int64 { return &h.Index }, facadeIntCodec),
	qijson.Array[facadeHolder, int64]("pair", func(h *facadeHolder) []int64 { return h.Pair[:] }, facadeIntCodec),
	qijson.Slice[facadeHolder, facadePoint]("points", func(h *facadeHolder) *[]facadePoint { return &h.Points }, qijson.DecoderCodec[facadePoint, *facadePoint]()),
	qijson.Any[facadeHolder]("anything", func(h *facadeHolder) *any { return &h.Anything }),
	qijson.Raw[facadeHolder]("blob", func(h *facadeHolder) *[]byte { return &h.Blob }),
)

// *facadeHolder delegates to facadeHolderSchema, so the type is qi-json-aware
// and Unmarshal/Marshal (which require the pointer to implement
// Decoder/Encoder) accept it.
func (h *facadeHolder) DecodeJSON(r *qijson.Reader) error { return facadeHolderSchema.Decode(r, h) }

func (h *facadeHolder) EncodeJSON(w *qijson.Writer) error { return facadeHolderSchema.Encode(w, h) }

// facadeFullDoc populates every field, so a decode exercises the value branch of
// each constructor.
const facadeFullDoc = `{
	"name": "qi",
	"namePtr": "ptr",
	"count": 7,
	"countPtr": 8,
	"rate": 1.5,
	"ratePtr": 2.5,
	"on": true,
	"onPtr": false,
	"origin": {"x": 1, "y": 2},
	"nested": {"x": 3, "y": 4},
	"nestedPtr": {"x": 5, "y": 6},
	"tags": ["a", "b"],
	"index": {"k": 9},
	"pair": [10, 11],
	"points": [{"x": 12, "y": 13}],
	"anything": {"z": [1, true]},
	"blob": {"raw":1}
}`

const facadeFullOut = `{"name":"qi","namePtr":"ptr","count":7,"countPtr":8,"rate":1.5,` +
	`"ratePtr":2.5,"on":true,"onPtr":false,"origin":{"x":1,"y":2},"nested":{"x":3,"y":4},` +
	`"nestedPtr":{"x":5,"y":6},"tags":["a","b"],"index":{"k":9},"pair":[10,11],` +
	`"points":[{"x":12,"y":13}],"anything":{"z":[1,true]},"blob":{"raw":1}}`

// TestFacadeConstructorsRoundTrip calls every field constructor through the
// facade: it decodes a fully populated document into facadeHolder and re-encodes
// it, asserting the exact member order (declaration order) and values.
func TestFacadeConstructorsRoundTrip(t *testing.T) {
	var got facadeHolder
	if err := qijson.UnmarshalInto[facadeHolder, *facadeHolder]([]byte(facadeFullDoc), &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}

	if got.Name != "qi" || got.NamePtr == nil || *got.NamePtr != "ptr" {
		t.Errorf("Name/NamePtr = %q/%v", got.Name, got.NamePtr)
	}
	if got.Count != 7 || got.CountPtr == nil || *got.CountPtr != 8 {
		t.Errorf("Count/CountPtr = %d/%v", got.Count, got.CountPtr)
	}
	if got.Rate != 1.5 || got.RatePtr == nil || *got.RatePtr != 2.5 {
		t.Errorf("Rate/RatePtr = %v/%v", got.Rate, got.RatePtr)
	}
	if !got.On || got.OnPtr == nil || *got.OnPtr {
		t.Errorf("On/OnPtr = %v/%v", got.On, got.OnPtr)
	}
	if got.Origin != (facadePoint{X: 1, Y: 2}) || got.Nested != (facadePoint{X: 3, Y: 4}) {
		t.Errorf("Origin/Nested = %+v/%+v", got.Origin, got.Nested)
	}
	if got.NestedPtr == nil || *got.NestedPtr != (facadePoint{X: 5, Y: 6}) {
		t.Errorf("NestedPtr = %v", got.NestedPtr)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "a" || got.Tags[1] != "b" {
		t.Errorf("Tags = %v", got.Tags)
	}
	if got.Index["k"] != 9 {
		t.Errorf("Index = %v", got.Index)
	}
	if got.Pair != [2]int64{10, 11} {
		t.Errorf("Pair = %v", got.Pair)
	}
	if len(got.Points) != 1 || got.Points[0] != (facadePoint{X: 12, Y: 13}) {
		t.Errorf("Points = %v", got.Points)
	}
	m, ok := got.Anything.(map[string]any)
	if !ok {
		t.Fatalf("Anything = %T, want map[string]any", got.Anything)
	}
	arr, ok := m["z"].([]any)
	if !ok || len(arr) != 2 || arr[0] != float64(1) || arr[1] != true {
		t.Errorf("Anything[z] = %v", m["z"])
	}
	if string(got.Blob) != `{"raw":1}` {
		t.Errorf("Blob = %q", got.Blob)
	}

	data, err := facadeHolderSchema.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != facadeFullOut {
		t.Errorf("Marshal:\n got %s\nwant %s", data, facadeFullOut)
	}
}

// TestFacadeConstructorsNulls sets every nullable member to JSON null and checks
// that a null clears a pointer field while leaving a value field unchanged.
func TestFacadeConstructorsNulls(t *testing.T) {
	start := facadeHolder{
		NamePtr:   ptrTo("keepname"),
		CountPtr:  ptrTo(int64(1)),
		RatePtr:   ptrTo(1.0),
		OnPtr:     ptrTo(true),
		NestedPtr: &facadePoint{X: 1, Y: 1},
	}
	got := start
	if err := qijson.UnmarshalInto[facadeHolder, *facadeHolder]([]byte(`{
		"namePtr": null, "countPtr": null, "ratePtr": null, "onPtr": null,
		"nestedPtr": null, "origin": null, "nested": null,
		"tags": null, "index": null, "pair": null, "points": null,
		"anything": null, "blob": null
	}`), &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}

	if got.NamePtr != nil || got.CountPtr != nil || got.RatePtr != nil || got.OnPtr != nil {
		t.Errorf("scalar pointers not cleared: %+v", got)
	}
	if got.NestedPtr != nil {
		t.Errorf("NestedPtr = %v, want nil", got.NestedPtr)
	}
	if got.Origin != start.Origin || got.Nested != start.Nested {
		t.Errorf("value objects changed: %+v", got)
	}
	if got.Tags != nil || got.Index != nil {
		t.Errorf("containers changed: %+v", got)
	}
	if got.Pair != start.Pair {
		t.Errorf("fixed array changed: %v", got.Pair)
	}
	if got.Anything != nil {
		t.Errorf("Anything not cleared: %v", got.Anything)
	}
	// Raw stores the member's exact literal bytes, so a JSON null is captured as
	// the four bytes "null" rather than clearing the field.
	if string(got.Blob) != "null" {
		t.Errorf("Blob = %q, want the literal null bytes", got.Blob)
	}
}

// TestFacadeConstructorsEncodeNulls writes a zero holder and checks that a nil
// pointer, nil slice, nil map and empty Raw all encode as null.
func TestFacadeConstructorsEncodeNulls(t *testing.T) {
	data, err := facadeHolderSchema.Marshal(facadeHolder{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"name":"","namePtr":null,"count":0,"countPtr":null,"rate":0,"ratePtr":null,` +
		`"on":false,"onPtr":null,"origin":{"x":0,"y":0},"nested":{"x":0,"y":0},` +
		`"nestedPtr":null,"tags":null,"index":null,"pair":[0,0],"points":null,` +
		`"anything":null,"blob":null}`
	if string(data) != want {
		t.Errorf("Marshal:\n got %s\nwant %s", data, want)
	}
}

// TestFacadeConstructorsErrorPaths drives the type-mismatch branch of each
// constructor and asserts the error kind and JSON path reported through the
// facade.
func TestFacadeConstructorsErrorPaths(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		path string
	}{
		{"name", `{"name": 1}`, "$.name"},
		{"namePtr", `{"namePtr": 1}`, "$.namePtr"},
		{"count", `{"count": "x"}`, "$.count"},
		{"countPtr", `{"countPtr": "x"}`, "$.countPtr"},
		{"rate", `{"rate": "x"}`, "$.rate"},
		{"ratePtr", `{"ratePtr": "x"}`, "$.ratePtr"},
		{"on", `{"on": 1}`, "$.on"},
		{"onPtr", `{"onPtr": 1}`, "$.onPtr"},
		{"origin", `{"origin": 1}`, "$.origin"},
		{"nested", `{"nested": 1}`, "$.nested"},
		{"nestedPtr", `{"nestedPtr": 1}`, "$.nestedPtr"},
		{"tags", `{"tags": 1}`, "$.tags"},
		{"index", `{"index": 1}`, "$.index"},
		{"pair", `{"pair": 1}`, "$.pair"},
		{"points", `{"points": 1}`, "$.points"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got facadeHolder
			err := qijson.UnmarshalInto[facadeHolder, *facadeHolder]([]byte(tc.doc), &got)
			if err == nil {
				t.Fatalf("UnmarshalInto(%s): want error, got nil", tc.doc)
			}
			var e *qijson.Error
			if !errors.As(err, &e) {
				t.Fatalf("errors.As(*qijson.Error) = false for %T", err)
			}
			if e.Kind != qijson.KindType {
				t.Errorf("Kind = %v, want %v", e.Kind, qijson.KindType)
			}
			if e.Path != tc.path {
				t.Errorf("Path = %q, want %q", e.Path, tc.path)
			}
		})
	}
}

// TestFacadeArrayLengthError checks a fixed-size array whose JSON length does not
// match is reported as a type error naming both lengths.
func TestFacadeArrayLengthError(t *testing.T) {
	var got facadeHolder
	err := qijson.UnmarshalInto[facadeHolder, *facadeHolder]([]byte(`{"pair": [1, 2, 3]}`), &got)
	if err == nil {
		t.Fatal("UnmarshalInto: want error, got nil")
	}
	var te *qijson.TypeError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As(*qijson.TypeError) = false for %T", err)
	}
	if te.Want != "array of length 2" || te.Got != "array of length 3" {
		t.Errorf("TypeError = %+v", te)
	}
}

// TestFacadeNewCodecNilDirections checks a Codec built with NewCodec can leave a
// direction nil: Slice then reports the missing decoder (on decode) or encoder
// (on encode) rather than panicking.
func TestFacadeNewCodecNilDirections(t *testing.T) {
	type holder struct{ N []int64 }

	encOnly := qijson.NewCodec[int64](nil, func(w *qijson.Writer, v int64) error { return w.WriteInt(v) })
	decOnly := qijson.NewCodec[int64](func(r *qijson.Reader) (int64, error) { return r.ReadInt() }, nil)

	decSchema := qijson.Struct(
		qijson.Slice[holder, int64]("n", func(h *holder) *[]int64 { return &h.N }, encOnly),
	)
	var h1 holder
	if err := decSchema.UnmarshalInto([]byte(`{"n": [1]}`), &h1); err == nil {
		t.Error("decode with nil element decoder: want error, got nil")
	}

	encSchema := qijson.Struct(
		qijson.Slice[holder, int64]("n", func(h *holder) *[]int64 { return &h.N }, decOnly),
	)
	h2 := holder{N: []int64{1}}
	if _, err := encSchema.Marshal(h2); err == nil {
		t.Error("encode with nil element encoder: want error, got nil")
	}
}

// ptrTo returns a pointer to a copy of v, so tests can build the pointer fields
// facadeHolder uses.
func ptrTo[T any](v T) *T { return &v }
