package gentest

import (
	"errors"
	"strings"
	"testing"
	"time"

	"qi-json/cmd/qi-json-gen/internal/gentest/xtypes"
	"qi-json/marshal"
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
		"skipped": "ignored"
	}`)

	var c Config
	if err := marshal.UnmarshalInto[Config](data, &c); err != nil {
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
}

// TestGeneratedDecodeNums exercises the generated element helpers.
func TestGeneratedDecodeNums(t *testing.T) {
	var n Nums
	if err := marshal.UnmarshalInto[Nums]([]byte(`{"ints":[-128,127],"uints":[0,4294967295],"float":[1.5,2]}`), &n); err != nil {
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c Config
			err := marshal.UnmarshalInto[Config]([]byte(tc.data), &c)
			if err == nil {
				t.Fatal("error = nil, want a type error")
			}
			var e *marshal.Error
			if !errors.As(err, &e) {
				t.Fatalf("error type = %T, want *marshal.Error", err)
			}
			if e.Kind != marshal.KindType {
				t.Errorf("Kind = %v, want %v", e.Kind, marshal.KindType)
			}
		})
	}
}

// TestGeneratedStrict checks that a -strict schema rejects unknown and duplicate
// members end to end.
func TestGeneratedStrict(t *testing.T) {
	var s Strict
	if err := marshal.UnmarshalInto[Strict]([]byte(`{"a":1,"b":"x"}`), &s); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if s.A != 1 || s.B != "x" {
		t.Errorf("Strict = %+v", s)
	}

	var e *marshal.Error
	err := marshal.UnmarshalInto[Strict]([]byte(`{"a":1,"nope":2}`), &s)
	if !errors.As(err, &e) || e.Kind != marshal.KindUnknownField {
		t.Errorf("unknown member: err = %v, want KindUnknownField", err)
	}

	err = marshal.UnmarshalInto[Strict]([]byte(`{"a":1,"a":2}`), &s)
	if !errors.As(err, &e) || e.Kind != marshal.KindDuplicateField {
		t.Errorf("duplicate member: err = %v, want KindDuplicateField", err)
	}
}

// TestGeneratedEncodeConfig proves the generated EncodeJSON works end to end,
// writes members in declaration order, and honours omitempty.
func TestGeneratedEncodeConfig(t *testing.T) {
	data, err := marshal.Marshal[Config](Config{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"name":"","port":0,"rate":0,"debug":false,"level":0,"count":0,` +
		`"ratio":0,"size":0,"limit":null,"tags":null,"matrix":[0,0],"scores":null,` +
		`"blob":null,"meta":null,"origin":{"x":0,"y":0},"points":null,"ports":null,` +
		`"timeout":0,"deadline":null,"durations":null,"label":"","labels":null,"flag":false,` +
		`"status":"","prev":null,"statuses":null,"mode":false,"modes":null}`
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
	data, err = marshal.Marshal[Config](populated)
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
	if err := marshal.UnmarshalInto[Config](data, &back); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	data2, err := marshal.Marshal[Config](back)
	if err != nil {
		t.Fatalf("re-Marshal: %v", err)
	}
	if string(data2) != string(data) {
		t.Errorf("round trip not stable:\n got %s\nwant %s", data2, data)
	}
}
