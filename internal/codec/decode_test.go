package codec

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

func TestDecodeNonGeneric(t *testing.T) {
	var v uhConfig
	if err := Decode([]byte(`{"name":"qi-json","port":8080}`), &v); err != nil {
		t.Fatalf("Decode() error = %v, want nil", err)
	}
	if v.Name != "qi-json" || v.Port != 8080 {
		t.Fatalf("Decode() = %+v, want {Name:qi-json Port:8080}", v)
	}
}

func TestDecodeMerge(t *testing.T) {
	v := uhConfig{Name: "keep", Port: 1}
	if err := Decode([]byte(`{"port":9}`), &v); err != nil {
		t.Fatalf("Decode() error = %v, want nil", err)
	}
	if v.Name != "keep" || v.Port != 9 {
		t.Fatalf("Decode() = %+v, want {Name:keep Port:9}", v)
	}
}

// TestDecodeMatchesUnmarshalInto asserts that the non-generic Decode behaves
// exactly like the generic UnmarshalInto: the same success, the same target and
// the same error for every input.
func TestDecodeMatchesUnmarshalInto(t *testing.T) {
	inputs := []struct {
		name string
		data string
	}{
		{"ok", `{"name":"qi-json","port":8080}`},
		{"merge", `{"port":9}`},
		{"empty object", `{}`},
		{"wrong type", `[1,2]`},
		{"invalid json", `{`},
		{"trailing data", `{"name":"a"} 1`},
		{"field type error", `{"port":"nope"}`},
	}
	for _, tc := range inputs {
		t.Run(tc.name, func(t *testing.T) {
			var gotV, wantV uhConfig
			gotErr := Decode([]byte(tc.data), &gotV)
			wantErr := UnmarshalInto[uhConfig]([]byte(tc.data), &wantV)
			if (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("Decode() error = %v, UnmarshalInto() error = %v", gotErr, wantErr)
			}
			if gotErr != nil && gotErr.Error() != wantErr.Error() {
				t.Fatalf("Decode() error = %q, UnmarshalInto() error = %q", gotErr, wantErr)
			}
			// A failed decode leaves the target unchanged in both paths.
			if gotV != wantV {
				t.Fatalf("Decode() = %+v, UnmarshalInto() = %+v", gotV, wantV)
			}
		})
	}
}

func TestDecodeErrorKinds(t *testing.T) {
	t.Run("nil decoder", func(t *testing.T) {
		if err := Decode([]byte(`{}`), nil); !errors.Is(err, errNilTarget) {
			t.Fatalf("Decode(nil) error = %v, want %v", err, errNilTarget)
		}
	})

	t.Run("type error", func(t *testing.T) {
		var v uhConfig
		err := Decode([]byte(`[1,2]`), &v)
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("error type = %T, want *Error", err)
		}
		if e.Kind != KindType {
			t.Errorf("Kind = %v, want %v", e.Kind, KindType)
		}
	})

	t.Run("syntax error", func(t *testing.T) {
		var v uhConfig
		err := Decode([]byte(`{"name":"a"} 1`), &v)
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("error type = %T, want *Error", err)
		}
		if e.Kind != KindSyntax {
			t.Errorf("Kind = %v, want %v", e.Kind, KindSyntax)
		}
	})
}

func TestDecodeDispatchesToDecoder(t *testing.T) {
	var d uhCounter
	if err := Decode([]byte(`[1,2,3]`), &d); err != nil {
		t.Fatalf("Decode() error = %v, want nil", err)
	}
	if d.calls != 1 || d.kind != "array" {
		t.Fatalf("Decode() calls = %d, kind = %q; want 1, array", d.calls, d.kind)
	}
}

func BenchmarkDecodeNonGeneric(b *testing.B) {
	data := []byte(`{"name":"qi-json","port":8080}`)
	var v uhConfig
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := Decode(data, &v); err != nil {
			b.Fatal(err)
		}
	}
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
