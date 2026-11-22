package codec

import "testing"

// encPoint is the nested struct used by the schema encode tests.
type encPoint struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

var encPointSchema = Struct(
	Int[encPoint]("x", func(p *encPoint) *int64 { return &p.X }),
	Int[encPoint]("y", func(p *encPoint) *int64 { return &p.Y }),
)

func decodeEncPoint(r *Reader) (encPoint, error) {
	var p encPoint
	err := encPointSchema.Decode(r, &p)
	return p, err
}

func encodeEncPoint(w *Writer, v encPoint) error {
	return encPointSchema.Encode(w, &v)
}

// encAll exercises every field family through both directions. Its fields are
// deliberately not in alphabetical order, so the encode tests prove members are
// written in declaration order.
type encAll struct {
	Name   string
	Port   int64
	Rate   float64
	Debug  bool
	Limit  *int64
	Tags   []string
	Matrix [2]int64
	Scores map[string]int64
	Blob   []byte
	Meta   any
	Origin encPoint
	Points []encPoint
	Ports  map[string]encPoint
}

var encInt = NewCodec((*Reader).ReadInt, (*Writer).WriteInt)
var encStr = NewCodec((*Reader).ReadString, (*Writer).WriteString)

var encAllSchema = Struct(
	String[encAll]("name", func(c *encAll) *string { return &c.Name }),
	Int[encAll]("port", func(c *encAll) *int64 { return &c.Port }),
	Float[encAll]("rate", func(c *encAll) *float64 { return &c.Rate }),
	Bool[encAll]("debug", func(c *encAll) *bool { return &c.Debug }),
	IntPtr[encAll]("limit", func(c *encAll) **int64 { return &c.Limit }),
	Slice[encAll, string]("tags", func(c *encAll) *[]string { return &c.Tags }, encStr),
	Array[encAll, int64]("matrix", func(c *encAll) []int64 { return c.Matrix[:] }, encInt),
	Map[encAll, int64]("scores", func(c *encAll) *map[string]int64 { return &c.Scores }, encInt),
	Raw[encAll]("blob", func(c *encAll) *[]byte { return &c.Blob }),
	Any[encAll]("meta", func(c *encAll) *any { return &c.Meta }),
	Object[encAll, encPoint]("origin", func(c *encAll) *encPoint { return &c.Origin }, encPointSchema),
	Slice[encAll, encPoint]("points", func(c *encAll) *[]encPoint { return &c.Points }, NewCodec(decodeEncPoint, encodeEncPoint)),
	Map[encAll, encPoint]("ports", func(c *encAll) *map[string]encPoint { return &c.Ports }, NewCodec(decodeEncPoint, encodeEncPoint)),
)

// EncodeJSON makes encAll a top-level Encoder, so Marshal[T] and
// qijson.Encode can be tested against the same schema.
func (c *encAll) EncodeJSON(w *Writer) error { return encAllSchema.Encode(w, c) }

func TestSchemaMarshalRoundTrip(t *testing.T) {
	limit := int64(-7)
	v := encAll{
		Name:   "qi-json",
		Port:   8080,
		Rate:   1.5,
		Debug:  true,
		Limit:  &limit,
		Tags:   []string{"a", "b"},
		Matrix: [2]int64{3, 4},
		Scores: map[string]int64{"x": 1, "y": -2},
		Blob:   []byte(`[1,2]`),
		Meta:   map[string]any{"k": []any{1.0, 2.0}},
		Origin: encPoint{X: 1, Y: 2},
		Points: []encPoint{{X: 3, Y: 4}},
		Ports:  map[string]encPoint{"http": {X: 80, Y: 80}},
	}
	want := `{"name":"qi-json","port":8080,"rate":1.5,"debug":true,"limit":-7,` +
		`"tags":["a","b"],"matrix":[3,4],"scores":{"x":1,"y":-2},"blob":[1,2],` +
		`"meta":{"k":[1,2]},"origin":{"x":1,"y":2},"points":[{"x":3,"y":4}],` +
		`"ports":{"http":{"x":80,"y":80}}}`

	// Both entry points: the schema method and the generic Marshal.
	data, err := encAllSchema.Marshal(v)
	if err != nil {
		t.Fatalf("Schema.Marshal: %v", err)
	}
	if string(data) != want {
		t.Errorf("Schema.Marshal:\n got %s\nwant %s", data, want)
	}

	data2, err := Marshal[encAll](v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data2) != want {
		t.Errorf("Marshal:\n got %s\nwant %s", data2, want)
	}

	// Re-decoding and re-encoding must be stable.
	var back encAll
	if err := encAllSchema.UnmarshalInto(data, &back); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	data3, err := encAllSchema.Marshal(back)
	if err != nil {
		t.Fatalf("re-Marshal: %v", err)
	}
	if string(data3) != want {
		t.Errorf("round trip:\n got %s\nwant %s", data3, want)
	}
}

func TestSchemaEncodeOmitEmpty(t *testing.T) {
	type cfg struct {
		A string
		B int64
		C *int64
		D []string
		E map[string]int64
	}
	schema := Struct(
		String[cfg]("a", func(c *cfg) *string { return &c.A }).OmitEmpty(),
		Int[cfg]("b", func(c *cfg) *int64 { return &c.B }).OmitEmpty(),
		IntPtr[cfg]("c", func(c *cfg) **int64 { return &c.C }).OmitEmpty(),
		Slice[cfg, string]("d", func(c *cfg) *[]string { return &c.D }, encStr).OmitEmpty(),
		Map[cfg, int64]("e", func(c *cfg) *map[string]int64 { return &c.E }, encInt).OmitEmpty(),
	)

	data, err := schema.Marshal(cfg{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{}` {
		t.Errorf("empty value: got %s, want {}", data)
	}

	n := int64(5)
	data, err = schema.Marshal(cfg{A: "x", B: 1, C: &n, D: []string{"y"}, E: map[string]int64{"k": 2}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"a":"x","b":1,"c":5,"d":["y"],"e":{"k":2}}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestSchemaEncodeNilContainers(t *testing.T) {
	type cfg struct {
		S []string
		M map[string]int64
		B []byte
	}
	schema := Struct(
		Slice[cfg, string]("s", func(c *cfg) *[]string { return &c.S }, encStr),
		Map[cfg, int64]("m", func(c *cfg) *map[string]int64 { return &c.M }, encInt),
		Raw[cfg]("b", func(c *cfg) *[]byte { return &c.B }),
	)
	data, err := schema.Marshal(cfg{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"s":null,"m":null,"b":null}` {
		t.Errorf("got %s, want nil containers as null", data)
	}
}

func TestSchemaEncodeEmptyContainers(t *testing.T) {
	type cfg struct {
		S []string
		M map[string]int64
	}
	schema := Struct(
		Slice[cfg, string]("s", func(c *cfg) *[]string { return &c.S }, encStr),
		Map[cfg, int64]("m", func(c *cfg) *map[string]int64 { return &c.M }, encInt),
	)
	data, err := schema.Marshal(cfg{S: []string{}, M: map[string]int64{}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"s":[],"m":{}}` {
		t.Errorf("got %s, want empty containers as [] and {}", data)
	}
}

func TestSchemaEncodeAny(t *testing.T) {
	type cfg struct {
		V any
	}
	schema := Struct(Any[cfg]("v", func(c *cfg) *any { return &c.V }))

	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, `{"v":null}`},
		{"bool", true, `{"v":true}`},
		{"string", "hi", `{"v":"hi"}`},
		{"int", 3, `{"v":3}`},
		{"float", 1.5, `{"v":1.5}`},
		{"array", []any{1.0, "x"}, `{"v":[1,"x"]}`},
		{"object", map[string]any{"b": 2.0, "a": 1.0}, `{"v":{"a":1,"b":2}}`},
		{"encoder", &manual{n: 9}, `{"v":{"n":9}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := schema.Marshal(cfg{V: tc.in})
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(data) != tc.want {
				t.Errorf("got %s, want %s", data, tc.want)
			}
		})
	}
}

func TestSchemaEncodeAnyUnsupported(t *testing.T) {
	type cfg struct {
		V any
	}
	schema := Struct(Any[cfg]("v", func(c *cfg) *any { return &c.V }))
	if _, err := schema.Marshal(cfg{V: struct{}{}}); err == nil {
		t.Fatal("Marshal error = nil, want an error for a dynamic type with no encoding")
	}
}
