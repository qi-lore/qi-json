package codec

import (
	"errors"
	"math"
	"testing"
)

// The error-wrapping policy (see doc.go and the constitution's "Rigorous Code
// Quality" section) is: a decode failure is a typed *Error that carries the JSON
// path and offset and is returned unwrapped, so its cause stays reachable with
// errors.As and errors.Is; a programming error is an inspectable sentinel. These
// tests pin that guarantee end to end, through every public entry point and
// across a nested ObjectDecoder delegation.

// assertDecodeError asserts that err is a *Error with the given kind and path,
// returned directly rather than wrapped in a message the caller cannot inspect.
func assertDecodeError(t *testing.T, err error, kind ErrorKind, path string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want a *Error")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != kind {
		t.Errorf("Kind = %v, want %v", e.Kind, kind)
	}
	if e.Path != path {
		t.Errorf("Path = %q, want %q", e.Path, path)
	}
}

// TestDecodeErrorPolicy proves every decode entry point returns the same typed
// *Error for a field type mismatch: the caller can inspect it with errors.As and
// reach the *TypeError cause through the chain.
func TestDecodeErrorPolicy(t *testing.T) {
	// Valid JSON, wrong Go type: a field type mismatch at $.port.
	const bad = `{"name":"qi-json","port":"nope"}`

	t.Run("Unmarshal", func(t *testing.T) {
		_, err := Unmarshal[uhConfig]([]byte(bad))
		assertDecodeError(t, err, KindType, "$.port")
		var te *TypeError
		if !errors.As(err, &te) {
			t.Errorf("errors.As(*TypeError) = false, want true; err = %v", err)
		}
	})

	t.Run("Decode", func(t *testing.T) {
		var v uhConfig
		err := Decode([]byte(bad), &v)
		assertDecodeError(t, err, KindType, "$.port")
		// The *Error is returned unwrapped, so its message is the policy's
		// exact shape and not a second, caller-invisible wrapper.
		if got, want := err.Error(), `qijson: $.port: expected integer, found string (at offset 25)`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("Schema.Unmarshal", func(t *testing.T) {
		_, err := uhConfigSchema.Unmarshal([]byte(bad))
		assertDecodeError(t, err, KindType, "$.port")
	})

	t.Run("Schema.UnmarshalInto", func(t *testing.T) {
		var v uhConfig
		err := uhConfigSchema.UnmarshalInto([]byte(bad), &v)
		assertDecodeError(t, err, KindType, "$.port")
	})
}

// TestDecodeSyntaxErrorIsTyped pins that a syntax failure is also a *Error the
// caller can classify, distinct from the type failure above.
func TestDecodeSyntaxErrorIsTyped(t *testing.T) {
	_, err := Unmarshal[uhConfig]([]byte(`{"name":`))
	assertDecodeError(t, err, KindSyntax, "$.name")
}

// TestEncodeErrorPolicy proves the encode entry points report their failures as
// inspectable sentinels, the counterpart of the typed decode error: the caller
// reaches them with errors.Is.
func TestEncodeErrorPolicy(t *testing.T) {
	t.Run("Marshal encoder error", func(t *testing.T) {
		if _, err := Marshal[failEnc](failEnc{}); !errors.Is(err, errNonFinite) {
			t.Errorf("err = %v, want errNonFinite", err)
		}
	})

	t.Run("Marshal incomplete value", func(t *testing.T) {
		if _, err := Marshal[incompleteEnc](incompleteEnc{}); !errors.Is(err, errIncomplete) {
			t.Errorf("err = %v, want errIncomplete", err)
		}
	})

	t.Run("Encode nil writer", func(t *testing.T) {
		if err := Encode(nil, &manual{}); !errors.Is(err, errNilWriter) {
			t.Errorf("err = %v, want errNilWriter", err)
		}
	})

	t.Run("Encode nil target", func(t *testing.T) {
		var w Writer
		if err := Encode(&w, nil); !errors.Is(err, errNilTarget) {
			t.Errorf("err = %v, want errNilTarget", err)
		}
	})

	t.Run("Schema.Marshal non-finite field", func(t *testing.T) {
		type cfg struct{ R float64 }
		schema := Struct(Float[cfg]("r", func(c *cfg) *float64 { return &c.R }))
		if _, err := schema.Marshal(cfg{R: math.Inf(1)}); !errors.Is(err, errNonFinite) {
			t.Errorf("err = %v, want errNonFinite", err)
		}
	})
}

// TestErrorIsThroughObjectDecoder proves the decode error survives a nested
// ObjectDecoder delegation and its cause stays reachable: errors.Is reaches the
// unexported sentinel the nested decode wrapped, and errors.As still finds the
// typed *Error with the nested path.
func TestErrorIsThroughObjectDecoder(t *testing.T) {
	schema := Struct(
		ObjectDecoder("inner", func(v *odOwner) *odInner { return &v.One }),
	)

	t.Run("invalid UTF-8 cause", func(t *testing.T) {
		var got odOwner
		// The nested field "b" is a string with a raw 0xFF byte: valid JSON to
		// the scanner, invalid UTF-8 to the decoder.
		err := schema.UnmarshalInto([]byte("{\"inner\":{\"a\":1,\"b\":\"\xff\"}}"), &got)
		assertDecodeError(t, err, KindSyntax, "$.inner.b")
		if !errors.Is(err, errInvalidUTF8) {
			t.Errorf("errors.Is(err, errInvalidUTF8) = false, want true; err = %v", err)
		}
	})

	t.Run("type error cause", func(t *testing.T) {
		var got odOwner
		err := schema.UnmarshalInto([]byte(`{"inner":{"a":"x"}}`), &got)
		assertDecodeError(t, err, KindType, "$.inner.a")
		var te *TypeError
		if !errors.As(err, &te) {
			t.Fatalf("errors.As(*TypeError) = false, want true; err = %v", err)
		}
		if te.Want != "integer" || te.Got != "string" {
			t.Errorf("TypeError = %+v, want {integer string}", te)
		}
	})
}
