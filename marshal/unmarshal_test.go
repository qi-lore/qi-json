package marshal

import (
	"errors"
	"testing"
)

// uhConfig is a target type that binds itself with a Schema, the usual way a
// type implements Decoder.
type uhConfig struct {
	Name string
	Port int64
}

var uhConfigSchema = Struct(
	String[uhConfig]("name", func(c *uhConfig) *string { return &c.Name }),
	Int[uhConfig]("port", func(c *uhConfig) *int64 { return &c.Port }),
)

func (c *uhConfig) DecodeJSON(r *Reader) error { return uhConfigSchema.Decode(r, c) }

// uhCounter is a target that implements Decoder directly, without a Schema, so a
// test can tell that Unmarshal dispatched to the method.
type uhCounter struct {
	calls int
	kind  string
}

func (c *uhCounter) DecodeJSON(r *Reader) error {
	c.calls++
	k, err := r.Peek()
	if err != nil {
		return err
	}
	c.kind = k.String()
	return r.Skip()
}

func TestUnmarshalGeneric(t *testing.T) {
	got, err := Unmarshal[uhConfig]([]byte(`{"name":"qi-json","port":8080}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	want := uhConfig{Name: "qi-json", Port: 8080}
	if got != want {
		t.Fatalf("Unmarshal() = %+v, want %+v", got, want)
	}
}

func TestUnmarshalIntoMerge(t *testing.T) {
	v := uhConfig{Name: "keep", Port: 1}
	if err := UnmarshalInto[uhConfig]([]byte(`{"port":9}`), &v); err != nil {
		t.Fatalf("UnmarshalInto() error = %v, want nil", err)
	}
	// A member absent from the JSON leaves the target unchanged.
	if v.Name != "keep" || v.Port != 9 {
		t.Fatalf("UnmarshalInto() = %+v, want {Name:keep Port:9}", v)
	}
}

func TestUnmarshalDecoderDispatch(t *testing.T) {
	var d uhCounter
	if err := UnmarshalInto[uhCounter]([]byte(`[1,2,3]`), &d); err != nil {
		t.Fatalf("UnmarshalInto() error = %v, want nil", err)
	}
	if d.calls != 1 {
		t.Fatalf("DecodeJSON calls = %d, want 1", d.calls)
	}
	if d.kind != "array" {
		t.Fatalf("DecodeJSON saw kind %q, want %q", d.kind, "array")
	}
}

func TestUnmarshalInvalidTarget(t *testing.T) {
	t.Run("nil pointer", func(t *testing.T) {
		var p *uhConfig
		err := UnmarshalInto[uhConfig]([]byte(`{}`), p)
		if err == nil {
			t.Fatal("UnmarshalInto(nil) error = nil, want an error")
		}
		if !errors.Is(err, errNilTarget) {
			t.Fatalf("error = %v, want %v", err, errNilTarget)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		_, err := Unmarshal[uhConfig]([]byte(`[1,2]`))
		if err == nil {
			t.Fatal("Unmarshal(array) error = nil, want a type error")
		}
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("error type = %T, want *Error", err)
		}
		if e.Kind != KindType {
			t.Errorf("Kind = %v, want %v", e.Kind, KindType)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := Unmarshal[uhConfig]([]byte(`{`))
		if err == nil {
			t.Fatal("Unmarshal(bad json) error = nil, want a syntax error")
		}
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("error type = %T, want *Error", err)
		}
		if e.Kind != KindSyntax {
			t.Errorf("Kind = %v, want %v", e.Kind, KindSyntax)
		}
	})

	t.Run("trailing data", func(t *testing.T) {
		_, err := Unmarshal[uhConfig]([]byte(`{"name":"a"} 1`))
		if err == nil {
			t.Fatal("Unmarshal(trailing) error = nil, want an error")
		}
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("error type = %T, want *Error", err)
		}
		if e.Kind != KindSyntax {
			t.Errorf("Kind = %v, want %v", e.Kind, KindSyntax)
		}
	})
}

func BenchmarkUnmarshalGeneric(b *testing.B) {
	data := []byte(`{"name":"qi-json","port":8080}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Unmarshal[uhConfig](data); err != nil {
			b.Fatal(err)
		}
	}
}

// TestUnmarshalIntoPreservesTargetOnError checks that a failed decode leaves the
// target as it was before the call, matching Schema's behaviour.
func TestUnmarshalIntoPreservesTargetOnError(t *testing.T) {
	v := uhConfig{Name: "keep", Port: 5}
	err := UnmarshalInto[uhConfig]([]byte(`{"port":"nope"}`), &v)
	if err == nil {
		t.Fatal("UnmarshalInto() error = nil, want a type error")
	}
	if v.Name != "keep" || v.Port != 5 {
		// The name is untouched; the port is overwritten only on success, so it
		// keeps its old value.
		t.Fatalf("UnmarshalInto() = %+v, want {Name:keep Port:5}", v)
	}
}
