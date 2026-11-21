package marshal

import (
	"strconv"

	"qi-json/scanner"
)

// Field describes how to decode one JSON object member into a field of T. Name
// is the JSON member name the field matches; Decode reads the member value from
// the Reader and stores it into v. Build fields with the type-specific
// constructors (String, Int, Float, Bool and their Ptr variants, Object) or
// supply a custom Decode.
type Field[T any] struct {
	// Name is the JSON member name this field matches. Names must be unique
	// within a Schema.
	Name string

	// Decode reads exactly one JSON value from r and stores it into v.
	Decode func(*Reader, *T) error
}

// scalar builds a Field for a value field of type E. A JSON null leaves the
// field unchanged, which is zero for a freshly decoded value.
func scalar[T, E any](name string, set func(*T) *E, read func(*Reader) (E, error)) Field[T] {
	return Field[T]{
		Name: name,
		Decode: func(r *Reader, v *T) error {
			k, err := r.Peek()
			if err != nil {
				return err
			}
			if k == scanner.KindNull {
				return r.ReadNull()
			}
			e, err := read(r)
			if err != nil {
				return err
			}
			*set(v) = e
			return nil
		},
	}
}

// scalarPtr builds a Field for a pointer field of type *E. A JSON null sets the
// field to nil; otherwise the value is decoded into a fresh allocation.
func scalarPtr[T, E any](name string, set func(*T) **E, read func(*Reader) (E, error)) Field[T] {
	return Field[T]{
		Name: name,
		Decode: func(r *Reader, v *T) error {
			k, err := r.Peek()
			if err != nil {
				return err
			}
			if k == scanner.KindNull {
				if err := r.ReadNull(); err != nil {
					return err
				}
				*set(v) = nil
				return nil
			}
			e, err := read(r)
			if err != nil {
				return err
			}
			*set(v) = &e
			return nil
		},
	}
}

// String decodes a JSON string into a string field.
func String[T any](name string, field func(*T) *string) Field[T] {
	return scalar(name, field, (*Reader).ReadString)
}

// StringPtr decodes a JSON string into a *string field; a JSON null sets it to
// nil.
func StringPtr[T any](name string, field func(*T) **string) Field[T] {
	return scalarPtr(name, field, (*Reader).ReadString)
}

// Int decodes a JSON number into an int64 field.
func Int[T any](name string, field func(*T) *int64) Field[T] {
	return scalar(name, field, (*Reader).ReadInt)
}

// IntPtr decodes a JSON number into an *int64 field; a JSON null sets it to nil.
func IntPtr[T any](name string, field func(*T) **int64) Field[T] {
	return scalarPtr(name, field, (*Reader).ReadInt)
}

// Float decodes a JSON number into a float64 field.
func Float[T any](name string, field func(*T) *float64) Field[T] {
	return scalar(name, field, (*Reader).ReadFloat)
}

// FloatPtr decodes a JSON number into a *float64 field; a JSON null sets it to
// nil.
func FloatPtr[T any](name string, field func(*T) **float64) Field[T] {
	return scalarPtr(name, field, (*Reader).ReadFloat)
}

// Bool decodes a JSON boolean into a bool field.
func Bool[T any](name string, field func(*T) *bool) Field[T] {
	return scalar(name, field, (*Reader).ReadBool)
}

// BoolPtr decodes a JSON boolean into a *bool field; a JSON null sets it to nil.
func BoolPtr[T any](name string, field func(*T) **bool) Field[T] {
	return scalarPtr(name, field, (*Reader).ReadBool)
}

// Object decodes a JSON object into a nested struct field using schema. A JSON
// null leaves the field unchanged.
func Object[T, S any](name string, field func(*T) *S, schema *Schema[S]) Field[T] {
	return Field[T]{
		Name: name,
		Decode: func(r *Reader, v *T) error {
			k, err := r.Peek()
			if err != nil {
				return err
			}
			if k == scanner.KindNull {
				return r.ReadNull()
			}
			return schema.Decode(r, field(v))
		},
	}
}

// Slice decodes a JSON array into a slice field, decoding each element with
// elem. A JSON null leaves the field unchanged; an empty JSON array produces a
// non-nil empty slice.
func Slice[T, E any](name string, field func(*T) *[]E, elem func(*Reader) (E, error)) Field[T] {
	return Field[T]{
		Name: name,
		Decode: func(r *Reader, v *T) error {
			k, err := r.Peek()
			if err != nil {
				return err
			}
			if k == scanner.KindNull {
				return r.ReadNull()
			}

			out := make([]E, 0)
			err = r.Array(func(int) error {
				e, err := elem(r)
				if err != nil {
					return err
				}
				out = append(out, e)
				return nil
			})
			if err != nil {
				return err
			}
			*field(v) = out
			return nil
		},
	}
}

// Map decodes a JSON object into a map field with string keys, decoding each
// member value with elem. A JSON null leaves the field unchanged; an empty JSON
// object produces a non-nil empty map.
func Map[T, V any](name string, field func(*T) *map[string]V, elem func(*Reader) (V, error)) Field[T] {
	return Field[T]{
		Name: name,
		Decode: func(r *Reader, v *T) error {
			k, err := r.Peek()
			if err != nil {
				return err
			}
			if k == scanner.KindNull {
				return r.ReadNull()
			}

			m := make(map[string]V)
			err = r.Object(func(key string) error {
				e, err := elem(r)
				if err != nil {
					return err
				}
				m[key] = e
				return nil
			})
			if err != nil {
				return err
			}
			*field(v) = m
			return nil
		},
	}
}

// Array decodes a JSON array into a fixed-size array field. field returns a
// slice view of the array (such as arr[:] for an array arr), so the decoder
// writes through to the array. The JSON array must hold exactly len(field(v))
// elements: fewer is an underflow error and more is an overflow error, both
// reported as type errors. A JSON null leaves the field unchanged.
func Array[T, E any](name string, field func(*T) []E, elem func(*Reader) (E, error)) Field[T] {
	return Field[T]{
		Name: name,
		Decode: func(r *Reader, v *T) error {
			k, err := r.Peek()
			if err != nil {
				return err
			}
			if k == scanner.KindNull {
				return r.ReadNull()
			}

			view := field(v)
			n := len(view)
			start := r.Offset()

			out := make([]E, 0, n)
			err = r.Array(func(int) error {
				e, err := elem(r)
				if err != nil {
					return err
				}
				out = append(out, e)
				return nil
			})
			if err != nil {
				return err
			}
			if len(out) != n {
				return r.arrayLengthError(n, len(out), start)
			}
			copy(view, out)
			return nil
		},
	}
}

// Any decodes any JSON value into an any field: an object becomes a
// map[string]any, an array a []any, a string a string, a number a float64, a
// boolean a bool, and null nil.
func Any[T any](name string, field func(*T) *any) Field[T] {
	return Field[T]{
		Name: name,
		Decode: func(r *Reader, v *T) error {
			a, err := decodeAny(r)
			if err != nil {
				return err
			}
			*field(v) = a
			return nil
		},
	}
}

// Raw decodes any JSON value into a []byte field holding the value's exact
// literal bytes. The bytes are copied, so they do not alias the input.
func Raw[T any](name string, field func(*T) *[]byte) Field[T] {
	return Field[T]{
		Name: name,
		Decode: func(r *Reader, v *T) error {
			raw, err := r.raw()
			if err != nil {
				return err
			}
			out := make([]byte, len(raw))
			copy(out, raw)
			*field(v) = out
			return nil
		},
	}
}

// decodeAny decodes one JSON value into its natural Go representation, without
// reflection: object to map[string]any, array to []any, string to string,
// number to float64, boolean to bool, and null to nil.
func decodeAny(r *Reader) (any, error) {
	k, err := r.Peek()
	if err != nil {
		return nil, err
	}
	switch k {
	case scanner.KindNull:
		if err := r.ReadNull(); err != nil {
			return nil, err
		}
		return nil, nil
	case scanner.KindBool:
		return r.ReadBool()
	case scanner.KindString:
		return r.ReadString()
	case scanner.KindNumber:
		return r.ReadFloat()
	case scanner.KindObject:
		m := make(map[string]any)
		err := r.Object(func(key string) error {
			v, err := decodeAny(r)
			if err != nil {
				return err
			}
			m[key] = v
			return nil
		})
		if err != nil {
			return nil, err
		}
		return m, nil
	case scanner.KindArray:
		out := make([]any, 0)
		err := r.Array(func(int) error {
			v, err := decodeAny(r)
			if err != nil {
				return err
			}
			out = append(out, v)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return out, nil
	default:
		return nil, r.mismatch("value", k)
	}
}

// arrayLengthError reports a JSON array whose length does not match a
// fixed-size array field. want is the array's length, got the JSON array's
// length, and start the byte offset of the JSON array's '['.
func (r *Reader) arrayLengthError(want, got int, start int64) error {
	return &Error{
		Path:   r.Path(),
		Offset: start,
		Kind:   KindType,
		Err: &TypeError{
			Want: "array of length " + strconv.Itoa(want),
			Got:  "array of length " + strconv.Itoa(got),
		},
	}
}
