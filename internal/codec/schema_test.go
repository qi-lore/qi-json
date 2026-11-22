package codec

import (
	"errors"
	"fmt"
	"testing"

	"github.com/qi-lore/qi-json/internal/scanner"
)

func TestStructDecode(t *testing.T) {
	type config struct {
		Name  string
		Port  int64
		Ratio float64
		Debug bool
	}

	schema := Struct(
		String[config]("name", func(c *config) *string { return &c.Name }),
		Int[config]("port", func(c *config) *int64 { return &c.Port }),
		Float[config]("ratio", func(c *config) *float64 { return &c.Ratio }),
		Bool[config]("debug", func(c *config) *bool { return &c.Debug }),
	)

	data := []byte(`{"name":"qi-json","port":8080,"ratio":0.5,"debug":true}`)
	got, err := schema.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	want := config{Name: "qi-json", Port: 8080, Ratio: 0.5, Debug: true}
	if got != want {
		t.Fatalf("Unmarshal() = %+v, want %+v", got, want)
	}
}

func TestStructDecodeKeyOrderIndependent(t *testing.T) {
	type config struct {
		A int64
		B int64
	}
	schema := Struct(
		Int[config]("a", func(c *config) *int64 { return &c.A }),
		Int[config]("b", func(c *config) *int64 { return &c.B }),
	)
	got, err := schema.Unmarshal([]byte(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	if got.A != 1 || got.B != 2 {
		t.Fatalf("Unmarshal() = %+v, want {A:1 B:2}", got)
	}
}

func TestStructDecodeEscapedKey(t *testing.T) {
	type config struct {
		Name string
	}
	schema := Struct(String[config]("name", func(c *config) *string { return &c.Name }))

	// The key "name" written with a \u escape must match the field.
	got, err := schema.Unmarshal([]byte(`{"\u006eame":"v"}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	if got.Name != "v" {
		t.Fatalf("Unmarshal() = %+v, want Name=v", got)
	}
}

func TestStructUnknownFieldIgnored(t *testing.T) {
	type config struct {
		A int64
	}
	schema := Struct(Int[config]("a", func(c *config) *int64 { return &c.A }))

	got, err := schema.Unmarshal([]byte(`{"a":1,"b":{"nested":[1,2,3]},"c":null}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	if got.A != 1 {
		t.Fatalf("Unmarshal() = %+v, want A=1", got)
	}
}

func TestStructUnknownFieldRejected(t *testing.T) {
	type config struct {
		A int64
	}
	schema := Struct(Int[config]("a", func(c *config) *int64 { return &c.A })).RejectUnknown()

	_, err := schema.Unmarshal([]byte(`{"a":1,"b":2}`))
	if err == nil {
		t.Fatal("Unmarshal() error = nil, want an unknown-field error")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != KindUnknownField {
		t.Errorf("Kind = %v, want %v", e.Kind, KindUnknownField)
	}
	if e.Path != "$.b" {
		t.Errorf("Path = %q, want %q", e.Path, "$.b")
	}
}

func TestStructDuplicateFieldLastWins(t *testing.T) {
	type config struct {
		A int64
	}
	schema := Struct(Int[config]("a", func(c *config) *int64 { return &c.A }))

	got, err := schema.Unmarshal([]byte(`{"a":1,"a":2}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	if got.A != 2 {
		t.Fatalf("Unmarshal() = %+v, want A=2 (last wins)", got)
	}
}

func TestStructDuplicateFieldRejected(t *testing.T) {
	type config struct {
		A int64
	}
	schema := Struct(Int[config]("a", func(c *config) *int64 { return &c.A })).RejectDuplicate()

	_, err := schema.Unmarshal([]byte(`{"a":1,"a":2}`))
	if err == nil {
		t.Fatal("Unmarshal() error = nil, want a duplicate-field error")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != KindDuplicateField {
		t.Errorf("Kind = %v, want %v", e.Kind, KindDuplicateField)
	}
}

func TestStructNested(t *testing.T) {
	type point struct {
		X int64
		Y int64
	}
	type shape struct {
		Origin point
		Name   string
	}

	pointSchema := Struct(
		Int[point]("x", func(p *point) *int64 { return &p.X }),
		Int[point]("y", func(p *point) *int64 { return &p.Y }),
	)
	shapeSchema := Struct(
		Object[shape]("origin", func(s *shape) *point { return &s.Origin }, pointSchema),
		String[shape]("name", func(s *shape) *string { return &s.Name }),
	)

	got, err := shapeSchema.Unmarshal([]byte(`{"origin":{"x":3,"y":4},"name":"sq"}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	want := shape{Origin: point{X: 3, Y: 4}, Name: "sq"}
	if got != want {
		t.Fatalf("Unmarshal() = %+v, want %+v", got, want)
	}
}

func TestStructNestedErrorPath(t *testing.T) {
	type point struct {
		X int64
	}
	type shape struct {
		Origin point
	}
	pointSchema := Struct(Int[point]("x", func(p *point) *int64 { return &p.X }))
	shapeSchema := Struct(
		Object[shape]("origin", func(s *shape) *point { return &s.Origin }, pointSchema),
	)

	_, err := shapeSchema.Unmarshal([]byte(`{"origin":{"x":"nope"}}`))
	if err == nil {
		t.Fatal("Unmarshal() error = nil, want a type error")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Path != "$.origin.x" {
		t.Errorf("Path = %q, want %q", e.Path, "$.origin.x")
	}
	if e.Kind != KindType {
		t.Errorf("Kind = %v, want %v", e.Kind, KindType)
	}
}

func TestStructNullField(t *testing.T) {
	type config struct {
		N int64
		P *int64
		S string
		Q *string
	}
	schema := Struct(
		Int[config]("n", func(c *config) *int64 { return &c.N }),
		IntPtr[config]("p", func(c *config) **int64 { return &c.P }),
		String[config]("s", func(c *config) *string { return &c.S }),
		StringPtr[config]("q", func(c *config) **string { return &c.Q }),
	)

	t.Run("fresh", func(t *testing.T) {
		got, err := schema.Unmarshal([]byte(`{"n":null,"p":null,"s":null,"q":null}`))
		if err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if got.N != 0 || got.P != nil || got.S != "" || got.Q != nil {
			t.Fatalf("Unmarshal() = %+v, want all zero", got)
		}
	})

	t.Run("merge", func(t *testing.T) {
		five := int64(5)
		pre := "pre"
		v := config{N: 9, P: &five, S: "keep", Q: &pre}
		if err := schema.UnmarshalInto([]byte(`{"n":null,"p":null,"s":null,"q":null}`), &v); err != nil {
			t.Fatalf("UnmarshalInto() error = %v, want nil", err)
		}
		// A null leaves a scalar unchanged but clears a pointer.
		if v.N != 9 || v.S != "keep" {
			t.Errorf("scalars = {N:%d S:%q}, want {N:9 S:keep}", v.N, v.S)
		}
		if v.P != nil || v.Q != nil {
			t.Errorf("pointers = {P:%v Q:%v}, want nil", v.P, v.Q)
		}
	})
}

func TestStructPointerField(t *testing.T) {
	type config struct {
		P *int64
		Q *string
	}
	schema := Struct(
		IntPtr[config]("p", func(c *config) **int64 { return &c.P }),
		StringPtr[config]("q", func(c *config) **string { return &c.Q }),
	)

	got, err := schema.Unmarshal([]byte(`{"p":42,"q":"hi"}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	if got.P == nil || *got.P != 42 {
		t.Errorf("P = %v, want 42", got.P)
	}
	if got.Q == nil || *got.Q != "hi" {
		t.Errorf("Q = %v, want hi", got.Q)
	}
}

func TestStructFieldLookup(t *testing.T) {
	for _, n := range []int{1, 2, 7, 64} {
		t.Run(fmt.Sprintf("%d fields", n), func(t *testing.T) {
			fields := make([]Field[struct{}], n)
			for i := range fields {
				fields[i] = Field[struct{}]{Name: fmt.Sprintf("f%02d", i)}
			}
			s := Struct(fields...)

			for _, f := range fields {
				idx, got := s.lookup([]byte(f.Name))
				if got == nil {
					t.Fatalf("lookup(%q) = nil, want a field", f.Name)
				}
				if got.Name != f.Name {
					t.Errorf("lookup(%q).Name = %q", f.Name, got.Name)
				}
				if idx < 0 || idx >= len(s.fields) || s.fields[idx].Name != f.Name {
					t.Errorf("lookup(%q) index = %d, want the field's index", f.Name, idx)
				}
			}

			for _, miss := range []string{"", "zzz", "f", "f999"} {
				if _, got := s.lookup([]byte(miss)); got != nil {
					t.Errorf("lookup(%q) = %q, want nil", miss, got.Name)
				}
			}
		})
	}
}

func TestStructNotAnObject(t *testing.T) {
	type config struct {
		A int64
	}
	schema := Struct(Int[config]("a", func(c *config) *int64 { return &c.A }))

	for _, in := range []string{`1`, `"x"`, `[1]`, `null`, `true`} {
		t.Run(in, func(t *testing.T) {
			_, err := schema.Unmarshal([]byte(in))
			if err == nil {
				t.Fatalf("Unmarshal(%s) error = nil, want a type error", in)
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
}

func TestStructTrailingData(t *testing.T) {
	type config struct {
		A int64
	}
	schema := Struct(Int[config]("a", func(c *config) *int64 { return &c.A }))

	_, err := schema.Unmarshal([]byte(`{"a":1} 2`))
	if err == nil {
		t.Fatal("Unmarshal() error = nil, want a trailing-data error")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != KindSyntax {
		t.Errorf("Kind = %v, want %v", e.Kind, KindSyntax)
	}
}

func TestStructEmptyObject(t *testing.T) {
	type config struct {
		A int64
		B string
	}
	schema := Struct(
		Int[config]("a", func(c *config) *int64 { return &c.A }),
		String[config]("b", func(c *config) *string { return &c.B }),
	)

	got, err := schema.Unmarshal([]byte(`{}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	if got != (config{}) {
		t.Fatalf("Unmarshal() = %+v, want the zero value", got)
	}
}

// TestSchemaNilTarget pins that the Schema entry points report a nil target as
// errNilTarget rather than dereferencing it and panicking, matching the generic
// UnmarshalInto and the non-generic Decode.
func TestSchemaNilTarget(t *testing.T) {
	type config struct {
		Name string
	}
	schema := Struct(
		String[config]("name", func(c *config) *string { return &c.Name }),
	)

	t.Run("Decode", func(t *testing.T) {
		r := newReader(scanner.NewTokenizer([]byte(`{"name":"x"}`)))
		if err := schema.Decode(r, nil); !errors.Is(err, errNilTarget) {
			t.Fatalf("Decode(nil) error = %v, want %v", err, errNilTarget)
		}
	})

	t.Run("UnmarshalInto", func(t *testing.T) {
		if err := schema.UnmarshalInto([]byte(`{"name":"x"}`), nil); !errors.Is(err, errNilTarget) {
			t.Fatalf("UnmarshalInto(nil) error = %v, want %v", err, errNilTarget)
		}
	})
}

func TestStructNoAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("allocation test is skipped in short mode")
	}

	type point struct {
		X  int64
		Y  int64
		On bool
	}
	schema := Struct(
		Int[point]("x", func(p *point) *int64 { return &p.X }),
		Int[point]("y", func(p *point) *int64 { return &p.Y }),
		Bool[point]("on", func(p *point) *bool { return &p.On }),
	)

	data := []byte(`{"x":1,"y":2,"on":true}`)
	r := newReader(scanner.NewTokenizer(data))

	// p is hoisted so that taking its address inside the measured region does
	// not itself allocate; the decode machinery must not allocate at all.
	var p point

	base := testing.AllocsPerRun(100, func() {
		r.tok = scanner.NewTokenizer(data)
	})

	full := testing.AllocsPerRun(100, func() {
		r.tok = scanner.NewTokenizer(data)
		p = point{}
		if err := schema.Decode(r, &p); err != nil {
			panic(err)
		}
	})

	if full != base {
		t.Errorf("struct decode allocated %v times per run, baseline %v; want equal (no allocations)", full, base)
	}
}

func BenchmarkStructDecode(b *testing.B) {
	type config struct {
		Name  string
		Port  int64
		Ratio float64
		Debug bool
	}
	schema := Struct(
		String[config]("name", func(c *config) *string { return &c.Name }),
		Int[config]("port", func(c *config) *int64 { return &c.Port }),
		Float[config]("ratio", func(c *config) *float64 { return &c.Ratio }),
		Bool[config]("debug", func(c *config) *bool { return &c.Debug }),
	)
	data := []byte(`{"name":"qi-json","port":8080,"ratio":0.5,"debug":true}`)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := schema.Unmarshal(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStructEncode(b *testing.B) {
	type config struct {
		Name  string
		Port  int64
		Ratio float64
		Debug bool
	}
	schema := Struct(
		String[config]("name", func(c *config) *string { return &c.Name }),
		Int[config]("port", func(c *config) *int64 { return &c.Port }),
		Float[config]("ratio", func(c *config) *float64 { return &c.Ratio }),
		Bool[config]("debug", func(c *config) *bool { return &c.Debug }),
	)
	v := config{Name: "qi-json", Port: 8080, Ratio: 0.5, Debug: true}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := schema.Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}
