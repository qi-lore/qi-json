package gentest

import (
	"errors"
	"strings"
	"testing"
	"time"

	"qi-json"
	"qi-json/cmd/qi-json-gen/internal/gentest/xtypes"
)

// TestGeneratedDecodeConfig decodes a document covering every field family and
// checks each field, proving the generated code compiles and works end to end.
func TestGeneratedDecodeConfig(t *testing.T) {
	data := []byte(`{
		"name": "qi-json",
		"port": 8080,
		"rate": 1.5,
		"debug": true,
		"level": 100,
		"count": 65535,
		"ratio": 0.25,
		"size": 42,
		"limit": -7,
		"tags": ["a", "b"],
		"matrix": [3, 4],
		"scores": {"x": 1, "y": -2},
		"blob": "AQI=",
		"meta": {"k": [1, 2]},
		"origin": {"x": 1, "y": 2},
		"points": [{"x": 3, "y": 4}],
		"ports": {"http": {"x": 80, "y": 80}},
		"timeout": 5000000000,
		"deadline": 250,
		"durations": [1, 2, 3],
		"label": "prod",
		"labels": {"a": "x", "b": "y"},
		"flag": true,
		"status": "active",
		"prev": "idle",
		"statuses": ["a", "b"],
		"mode": true,
		"modes": {"x": true, "y": false},
		"thing": {"name": "t", "count": 7, "tiny": 3, "extra": {"code": 9}},
		"maybe": {"name": "m", "extra": {"code": 1}},
		"things": [{"name": "a", "extra": {"code": 1}}, {"name": "b", "extra": {"code": 2}}],
		"twin": {"a": {"code": 11}, "b": [{"code": 12}]},
		"aware": {"a": 5, "b": "ok"},
		"aware_map": {"k": {"a": 6, "b": "v"}},
		"skipped": "ignored"
	}`)

	var c Config
	if err := qijson.UnmarshalInto[Config](data, &c); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}

	if c.Name != "qi-json" || c.Port != 8080 || c.Rate != 1.5 || !c.Debug {
		t.Errorf("scalars = %+v", c)
	}
	if c.Level != 100 || c.Count != 65535 || c.Ratio != 0.25 || c.Size != 42 {
		t.Errorf("extended numerics = level %d count %d ratio %v size %d", c.Level, c.Count, c.Ratio, c.Size)
	}
	if c.Limit == nil || *c.Limit != -7 {
		t.Errorf("Limit = %v, want -7", c.Limit)
	}
	if len(c.Tags) != 2 || c.Tags[0] != "a" || c.Tags[1] != "b" {
		t.Errorf("Tags = %v", c.Tags)
	}
	if c.Matrix != [2]int{3, 4} {
		t.Errorf("Matrix = %v", c.Matrix)
	}
	if c.Scores["x"] != 1 || c.Scores["y"] != -2 {
		t.Errorf("Scores = %v", c.Scores)
	}
	if string(c.Blob) != `"AQI="` {
		t.Errorf("Blob = %q, want the exact literal bytes \"AQI=\"", c.Blob)
	}
	if c.Meta == nil {
		t.Errorf("Meta = nil, want the decoded object")
	}
	if c.Origin != (Point{X: 1, Y: 2}) {
		t.Errorf("Origin = %+v", c.Origin)
	}
	if len(c.Points) != 1 || c.Points[0] != (Point{X: 3, Y: 4}) {
		t.Errorf("Points = %v", c.Points)
	}
	if c.Ports["http"] != (Point{X: 80, Y: 80}) {
		t.Errorf("Ports = %v", c.Ports)
	}
	if c.Timeout != time.Duration(5000000000) {
		t.Errorf("Timeout = %v, want 5000000000", c.Timeout)
	}
	if c.Deadline == nil || *c.Deadline != 250 {
		t.Errorf("Deadline = %v, want 250", c.Deadline)
	}
	if len(c.Durations) != 3 || c.Durations[2] != 3 {
		t.Errorf("Durations = %v", c.Durations)
	}
	if c.Label != xtypes.Label("prod") {
		t.Errorf("Label = %q, want prod", c.Label)
	}
	if c.Labels["a"] != "x" || c.Labels["b"] != "y" {
		t.Errorf("Labels = %v", c.Labels)
	}
	if !c.Flag {
		t.Errorf("Flag = false, want true")
	}
	if c.Status != "active" {
		t.Errorf("Status = %q, want active", c.Status)
	}
	if c.Prev == nil || *c.Prev != "idle" {
		t.Errorf("Prev = %v, want idle", c.Prev)
	}
	if len(c.Statuses) != 2 || c.Statuses[0] != "a" || c.Statuses[1] != "b" {
		t.Errorf("Statuses = %v", c.Statuses)
	}
	if !c.Mode {
		t.Errorf("Mode = false, want true")
	}
	if !c.Modes["x"] || c.Modes["y"] {
		t.Errorf("Modes = %v", c.Modes)
	}
	if c.Skipped != "" || c.hidden != "" {
		t.Errorf("skipped/unexported fields must stay zero: %q %q", c.Skipped, c.hidden)
	}

	// Foreign struct fields: A (synthesized) and B (delegated).
	wantThing := xtypes.Thing{Name: "t", Count: 7, Tiny: 3, Extra: xtypes.Inner{Code: 9}}
	if c.Thing != wantThing {
		t.Errorf("Thing = %+v, want %+v", c.Thing, wantThing)
	}
	if c.Maybe == nil || c.Maybe.Name != "m" || c.Maybe.Extra.Code != 1 {
		t.Errorf("Maybe = %+v", c.Maybe)
	}
	if len(c.Things) != 2 || c.Things[0].Name != "a" || c.Things[1].Extra.Code != 2 {
		t.Errorf("Things = %+v", c.Things)
	}
	if c.Twin.A.Code != 11 || len(c.Twin.B) != 1 || c.Twin.B[0].Code != 12 {
		t.Errorf("Twin = %+v", c.Twin)
	}
	if c.Aware.A != 5 || c.Aware.B != "ok" {
		t.Errorf("Aware = %+v", c.Aware)
	}
	if c.AwareMap["k"].A != 6 || c.AwareMap["k"].B != "v" {
		t.Errorf("AwareMap = %+v", c.AwareMap)
	}
}

// TestGeneratedDecodeNums exercises the generated element helpers.
func TestGeneratedDecodeNums(t *testing.T) {
	var n Nums
	if err := qijson.UnmarshalInto[Nums]([]byte(`{"ints":[-128,127],"uints":[0,4294967295],"float":[1.5,2]}`), &n); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if len(n.Ints) != 2 || n.Ints[0] != -128 || n.Ints[1] != 127 {
		t.Errorf("Ints = %v", n.Ints)
	}
	if len(n.Uints) != 2 || n.Uints[1] != 4294967295 {
		t.Errorf("Uints = %v", n.Uints)
	}
	if len(n.Float) != 2 || n.Float[0] != 1.5 {
		t.Errorf("Float = %v", n.Float)
	}
}

// TestGeneratedRangeErrors checks that an out-of-range number is a type error,
// the same shape the runtime produces.
func TestGeneratedRangeErrors(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"int8 overflow", `{"level":128}`},
		{"int8 underflow", `{"level":-129}`},
		{"uint16 negative", `{"count":-1}`},
		{"uint16 overflow", `{"count":65536}`},
		{"float32 overflow", `{"ratio":1e40}`},
		{"int32 pointer overflow", `{"limit":2147483648}`},
		{"foreign int8 overflow", `{"thing":{"tiny":200}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c Config
			err := qijson.UnmarshalInto[Config]([]byte(tc.data), &c)
			if err == nil {
				t.Fatal("error = nil, want a type error")
			}
			var e *qijson.Error
			if !errors.As(err, &e) {
				t.Fatalf("error type = %T, want *qijson.Error", err)
			}
			if e.Kind != qijson.KindType {
				t.Errorf("Kind = %v, want %v", e.Kind, qijson.KindType)
			}
		})
	}
}

// TestGeneratedForeignRoundTrip checks the foreign-struct fields survive an encode
// and decode, exercising A (synthesized) and B (delegated) end to end.
func TestGeneratedForeignRoundTrip(t *testing.T) {
	thing := xtypes.Thing{Name: "x", Count: 4, Tiny: -2, Extra: xtypes.Inner{Code: 8}}
	aware := xtypes.Aware{A: 3, B: "z"}
	in := Config{
		Name:     "round",
		Thing:    thing,
		Maybe:    &thing,
		Things:   []xtypes.Thing{thing},
		Twin:     xtypes.Twin{A: xtypes.Inner{Code: 1}, B: []xtypes.Inner{{Code: 2}}},
		Aware:    aware,
		AwareMap: map[string]xtypes.Aware{"k": aware},
	}
	data, err := qijson.Marshal[Config](in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back Config
	if err := qijson.UnmarshalInto[Config](data, &back); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if back.Thing != thing {
		t.Errorf("Thing = %+v, want %+v", back.Thing, thing)
	}
	if back.Maybe == nil || *back.Maybe != thing {
		t.Errorf("Maybe = %+v", back.Maybe)
	}
	if len(back.Things) != 1 || back.Things[0] != thing {
		t.Errorf("Things = %+v", back.Things)
	}
	if back.Twin.A != (xtypes.Inner{Code: 1}) || len(back.Twin.B) != 1 || back.Twin.B[0] != (xtypes.Inner{Code: 2}) {
		t.Errorf("Twin = %+v", back.Twin)
	}
	if back.Aware != aware {
		t.Errorf("Aware = %+v, want %+v", back.Aware, aware)
	}
	if back.AwareMap["k"] != aware {
		t.Errorf("AwareMap = %+v", back.AwareMap)
	}
}

// TestGeneratedForeignOverride checks a -foreign binding end to end: the field
// decodes through the schema the consumer owns.
func TestGeneratedForeignOverride(t *testing.T) {
	var o Overridden
	if err := qijson.UnmarshalInto[Overridden]([]byte(`{"inner":{"code":5}}`), &o); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if o.Inner.Code != 5 {
		t.Errorf("Inner = %+v, want code 5", o.Inner)
	}
	data, err := qijson.Marshal[Overridden](Overridden{Inner: xtypes.Inner{Code: 7}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"inner":{"code":7}}` {
		t.Errorf("Marshal = %s, want {\"inner\":{\"code\":7}}", data)
	}
}

// TestGeneratedStrict checks that a -strict schema rejects unknown and duplicate
// members end to end.
func TestGeneratedStrict(t *testing.T) {
	var s Strict
	if err := qijson.UnmarshalInto[Strict]([]byte(`{"a":1,"b":"x"}`), &s); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if s.A != 1 || s.B != "x" {
		t.Errorf("Strict = %+v", s)
	}

	var e *qijson.Error
	err := qijson.UnmarshalInto[Strict]([]byte(`{"a":1,"nope":2}`), &s)
	if !errors.As(err, &e) || e.Kind != qijson.KindUnknownField {
		t.Errorf("unknown member: err = %v, want KindUnknownField", err)
	}

	err = qijson.UnmarshalInto[Strict]([]byte(`{"a":1,"a":2}`), &s)
	if !errors.As(err, &e) || e.Kind != qijson.KindDuplicateField {
		t.Errorf("duplicate member: err = %v, want KindDuplicateField", err)
	}
}

// TestGeneratedEncodeConfig proves the generated EncodeJSON works end to end,
// writes members in declaration order, and honours omitempty.
func TestGeneratedEncodeConfig(t *testing.T) {
	data, err := qijson.Marshal[Config](Config{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"name":"","port":0,"rate":0,"debug":false,"level":0,"count":0,` +
		`"ratio":0,"size":0,"limit":null,"tags":null,"matrix":[0,0],"scores":null,` +
		`"blob":null,"meta":null,"origin":{"x":0,"y":0},"points":null,"ports":null,` +
		`"timeout":0,"deadline":null,"durations":null,"label":"","labels":null,"flag":false,` +
		`"status":"","prev":null,"statuses":null,"mode":false,"modes":null,` +
		`"thing":{"name":"","extra":{"code":0}},"things":null,"twin":{"a":{"code":0},"b":null},` +
		`"aware":{"a":0,"b":""},"aware_map":null}`
	if string(data) != want {
		t.Errorf("Marshal:\n got %s\nwant %s", data, want)
	}

	limit := int32(7)
	populated := Config{
		Name:    "qi-json",
		Port:    8080,
		Rate:    1.5,
		Debug:   true,
		Limit:   &limit,
		Tags:    []string{"a", "b"},
		Matrix:  [2]int{3, 4},
		Scores:  map[string]int32{"x": 1},
		Origin:  Point{X: 1, Y: 2},
		Note:    "hi",
		Timeout: 5 * time.Second,
		Status:  "active",
		Mode:    true,
	}
	data, err = qijson.Marshal[Config](populated)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"note":"hi"`) {
		t.Errorf("omitempty field with a value must be written: %s", data)
	}
	if !strings.Contains(string(data), `"status":"active"`) || !strings.Contains(string(data), `"mode":true`) {
		t.Errorf("named string/bool fields must be written: %s", data)
	}
	var back Config
	if err := qijson.UnmarshalInto[Config](data, &back); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	data2, err := qijson.Marshal[Config](back)
	if err != nil {
		t.Fatalf("re-Marshal: %v", err)
	}
	if string(data2) != string(data) {
		t.Errorf("round trip not stable:\n got %s\nwant %s", data2, data)
	}
}
