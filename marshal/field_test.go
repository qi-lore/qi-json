package marshal

import (
	"bytes"
	"errors"
	"testing"
)

// fpPoint is a struct element used by the slice and map tests.
type fpPoint struct {
	X int64
	Y int64
}

var fpPointSchema = Struct(
	Int[fpPoint]("x", func(p *fpPoint) *int64 { return &p.X }),
	Int[fpPoint]("y", func(p *fpPoint) *int64 { return &p.Y }),
)

// decodeFPPoint decodes one JSON object into an fpPoint.
func decodeFPPoint(r *Reader) (fpPoint, error) {
	var p fpPoint
	err := fpPointSchema.Decode(r, &p)
	return p, err
}

func TestSliceOfPrimitives(t *testing.T) {
	type cfg struct {
		Tags []string
	}
	schema := Struct(
		Slice[cfg, string]("tags", func(c *cfg) *[]string { return &c.Tags }, (*Reader).ReadString),
	)

	got, err := schema.Unmarshal([]byte(`{"tags":["a","b","c"]}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	want := []string{"a", "b", "c"}
	if len(got.Tags) != len(want) {
		t.Fatalf("Tags = %v, want %v", got.Tags, want)
	}
	for i := range want {
		if got.Tags[i] != want[i] {
			t.Fatalf("Tags = %v, want %v", got.Tags, want)
		}
	}
}

func TestSliceOfStructs(t *testing.T) {
	type cfg struct {
		Points []fpPoint
	}
	schema := Struct(
		Slice[cfg, fpPoint]("points", func(c *cfg) *[]fpPoint { return &c.Points }, decodeFPPoint),
	)

	got, err := schema.Unmarshal([]byte(`{"points":[{"x":1,"y":2},{"x":3,"y":4}]}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	want := []fpPoint{{X: 1, Y: 2}, {X: 3, Y: 4}}
	if len(got.Points) != len(want) {
		t.Fatalf("Points = %v, want %v", got.Points, want)
	}
	for i := range want {
		if got.Points[i] != want[i] {
			t.Fatalf("Points = %v, want %v", got.Points, want)
		}
	}
}

func TestEmptySlice(t *testing.T) {
	type cfg struct {
		Nums []int64
	}
	schema := Struct(
		Slice[cfg, int64]("nums", func(c *cfg) *[]int64 { return &c.Nums }, (*Reader).ReadInt),
	)

	got, err := schema.Unmarshal([]byte(`{"nums":[]}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	if got.Nums == nil {
		t.Fatal("Nums = nil, want a non-nil empty slice")
	}
	if len(got.Nums) != 0 {
		t.Fatalf("Nums = %v, want empty", got.Nums)
	}
}

func TestSliceNullLeavesFieldUnchanged(t *testing.T) {
	type cfg struct {
		Tags []string
	}
	schema := Struct(
		Slice[cfg, string]("tags", func(c *cfg) *[]string { return &c.Tags }, (*Reader).ReadString),
	)

	v := cfg{Tags: []string{"keep"}}
	if err := schema.UnmarshalInto([]byte(`{"tags":null}`), &v); err != nil {
		t.Fatalf("UnmarshalInto() error = %v, want nil", err)
	}
	if len(v.Tags) != 1 || v.Tags[0] != "keep" {
		t.Fatalf("Tags = %v, want [keep]", v.Tags)
	}
}

func TestSliceTypeError(t *testing.T) {
	type cfg struct {
		Tags []string
	}
	schema := Struct(
		Slice[cfg, string]("tags", func(c *cfg) *[]string { return &c.Tags }, (*Reader).ReadString),
	)

	_, err := schema.Unmarshal([]byte(`{"tags":{}}`))
	if err == nil {
		t.Fatal("Unmarshal() error = nil, want a type error")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != KindType {
		t.Errorf("Kind = %v, want %v", e.Kind, KindType)
	}
	if e.Path != "$.tags" {
		t.Errorf("Path = %q, want %q", e.Path, "$.tags")
	}
}

func TestMapOfStrings(t *testing.T) {
	type cfg struct {
		Labels map[string]string
	}
	schema := Struct(
		Map[cfg, string]("labels", func(c *cfg) *map[string]string { return &c.Labels }, (*Reader).ReadString),
	)

	got, err := schema.Unmarshal([]byte(`{"labels":{"a":"x","b":"y"}}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	want := map[string]string{"a": "x", "b": "y"}
	if len(got.Labels) != len(want) {
		t.Fatalf("Labels = %v, want %v", got.Labels, want)
	}
	for k, v := range want {
		if got.Labels[k] != v {
			t.Fatalf("Labels[%q] = %q, want %q", k, got.Labels[k], v)
		}
	}
}

func TestMapOfStructs(t *testing.T) {
	type cfg struct {
		Points map[string]fpPoint
	}
	schema := Struct(
		Map[cfg, fpPoint]("points", func(c *cfg) *map[string]fpPoint { return &c.Points }, decodeFPPoint),
	)

	got, err := schema.Unmarshal([]byte(`{"points":{"a":{"x":1,"y":2},"b":{"x":3,"y":4}}}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	want := map[string]fpPoint{"a": {X: 1, Y: 2}, "b": {X: 3, Y: 4}}
	if len(got.Points) != len(want) {
		t.Fatalf("Points = %v, want %v", got.Points, want)
	}
	for k, v := range want {
		if got.Points[k] != v {
			t.Fatalf("Points[%q] = %v, want %v", k, got.Points[k], v)
		}
	}
}

func TestEmptyMap(t *testing.T) {
	type cfg struct {
		Labels map[string]string
	}
	schema := Struct(
		Map[cfg, string]("labels", func(c *cfg) *map[string]string { return &c.Labels }, (*Reader).ReadString),
	)

	got, err := schema.Unmarshal([]byte(`{"labels":{}}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	if got.Labels == nil {
		t.Fatal("Labels = nil, want a non-nil empty map")
	}
	if len(got.Labels) != 0 {
		t.Fatalf("Labels = %v, want empty", got.Labels)
	}
}

func TestMapTypeError(t *testing.T) {
	type cfg struct {
		Labels map[string]string
	}
	schema := Struct(
		Map[cfg, string]("labels", func(c *cfg) *map[string]string { return &c.Labels }, (*Reader).ReadString),
	)

	_, err := schema.Unmarshal([]byte(`{"labels":[]}`))
	if err == nil {
		t.Fatal("Unmarshal() error = nil, want a type error")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != KindType {
		t.Errorf("Kind = %v, want %v", e.Kind, KindType)
	}
}

func TestArrayFixedSize(t *testing.T) {
	type cfg struct {
		V [3]int64
	}
	// The view returns a slice aliasing the array, so decoding writes through.
	schema := Struct(
		Array[cfg, int64]("v", func(c *cfg) []int64 { return c.V[:] }, (*Reader).ReadInt),
	)

	t.Run("exact", func(t *testing.T) {
		got, err := schema.Unmarshal([]byte(`{"v":[1,2,3]}`))
		if err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if got.V != [3]int64{1, 2, 3} {
			t.Fatalf("V = %v, want [1 2 3]", got.V)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		_, err := schema.Unmarshal([]byte(`{"v":[1,2,3,4]}`))
		if err == nil {
			t.Fatal("Unmarshal() error = nil, want an overflow error")
		}
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("error type = %T, want *Error", err)
		}
		if e.Kind != KindType {
			t.Errorf("Kind = %v, want %v", e.Kind, KindType)
		}
		if e.Path != "$.v" {
			t.Errorf("Path = %q, want %q", e.Path, "$.v")
		}
	})

	t.Run("underflow", func(t *testing.T) {
		_, err := schema.Unmarshal([]byte(`{"v":[1,2]}`))
		if err == nil {
			t.Fatal("Unmarshal() error = nil, want an underflow error")
		}
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("error type = %T, want *Error", err)
		}
		if e.Kind != KindType {
			t.Errorf("Kind = %v, want %v", e.Kind, KindType)
		}
	})
}

func TestAny(t *testing.T) {
	type cfg struct {
		V any
	}
	schema := Struct(Any[cfg]("v", func(c *cfg) *any { return &c.V }))

	t.Run("object", func(t *testing.T) {
		got, err := schema.Unmarshal([]byte(`{"v":{"a":1,"b":[true,null]}}`))
		if err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		m, ok := got.V.(map[string]any)
		if !ok {
			t.Fatalf("V = %T, want map[string]any", got.V)
		}
		if m["a"] != float64(1) {
			t.Errorf(`m["a"] = %v, want 1`, m["a"])
		}
		arr, ok := m["b"].([]any)
		if !ok {
			t.Fatalf(`m["b"] = %T, want []any`, m["b"])
		}
		if len(arr) != 2 || arr[0] != true || arr[1] != nil {
			t.Errorf(`m["b"] = %v, want [true <nil>]`, arr)
		}
	})

	t.Run("array", func(t *testing.T) {
		got, err := schema.Unmarshal([]byte(`{"v":[1,2,3]}`))
		if err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		arr, ok := got.V.([]any)
		if !ok {
			t.Fatalf("V = %T, want []any", got.V)
		}
		if len(arr) != 3 || arr[0] != float64(1) {
			t.Errorf("V = %v, want [1 2 3]", arr)
		}
	})

	t.Run("string", func(t *testing.T) {
		got, err := schema.Unmarshal([]byte(`{"v":"hi"}`))
		if err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if got.V != "hi" {
			t.Errorf("V = %v, want hi", got.V)
		}
	})

	t.Run("number", func(t *testing.T) {
		got, err := schema.Unmarshal([]byte(`{"v":1.5}`))
		if err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if got.V != float64(1.5) {
			t.Errorf("V = %v, want 1.5", got.V)
		}
	})

	t.Run("bool", func(t *testing.T) {
		got, err := schema.Unmarshal([]byte(`{"v":true}`))
		if err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if got.V != true {
			t.Errorf("V = %v, want true", got.V)
		}
	})

	t.Run("null", func(t *testing.T) {
		got, err := schema.Unmarshal([]byte(`{"v":null}`))
		if err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if got.V != nil {
			t.Errorf("V = %v, want nil", got.V)
		}
	})
}

func TestRaw(t *testing.T) {
	type cfg struct {
		V []byte
	}
	schema := Struct(Raw[cfg]("v", func(c *cfg) *[]byte { return &c.V }))

	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"object", `{"v":{"a":[1,2]}}`, `{"a":[1,2]}`},
		{"array", `{"v":[1,2,3]}`, `[1,2,3]`},
		{"string", `{"v":"hi"}`, `"hi"`},
		{"number", `{"v":1.5}`, `1.5`},
		{"bool", `{"v":true}`, `true`},
		{"null", `{"v":null}`, `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := schema.Unmarshal([]byte(tc.in))
			if err != nil {
				t.Fatalf("Unmarshal() error = %v, want nil", err)
			}
			if !bytes.Equal(got.V, []byte(tc.want)) {
				t.Fatalf("V = %q, want %q", got.V, tc.want)
			}
		})
	}
}

func TestRawDoesNotAliasInput(t *testing.T) {
	type cfg struct {
		V []byte
	}
	schema := Struct(Raw[cfg]("v", func(c *cfg) *[]byte { return &c.V }))

	data := []byte(`{"v":[1,2,3]}`)
	got, err := schema.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	// Mutating the input after decoding must not change the captured bytes.
	for i := range data {
		data[i] = 'x'
	}
	if !bytes.Equal(got.V, []byte(`[1,2,3]`)) {
		t.Fatalf("V = %q, want %q (must be an independent copy)", got.V, `[1,2,3]`)
	}
}
