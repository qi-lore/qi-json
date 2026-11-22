package qijson

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"qi-json/scanner"
)

// Sentinel errors for misuse of the container constructors. They are
// programming errors rather than properties of the input, so they are plain
// errors rather than *qijson.Error. Each is defined once so a caller can
// inspect it with errors.Is.
var (
	// errNilElementDecoder reports a Slice, Map or Array field built with a
	// Codec whose Decode is nil.
	errNilElementDecoder = errors.New("qijson: nil element decoder")

	// errNilElementEncoder reports a Slice, Map or Array field built with a
	// Codec whose Encode is nil.
	errNilElementEncoder = errors.New("qijson: nil element encoder")

	// errUnsupportedDynamicType reports a value in an any field whose dynamic
	// type encodeAny does not know how to write, since qijson may not use
	// reflection to discover one.
	errUnsupportedDynamicType = errors.New("unsupported dynamic type")
)

// Field describes how to decode and encode one JSON object member into a field
// of T. Name is the JSON member name the field matches; Decode reads the member
// value from the Reader and stores it into v, and Encode writes it back to a
// Writer. Build fields with the type-specific constructors (String, Int, Float,
// Bool and their Ptr variants, Object, Slice, Map, Array, Any, Raw) or supply a
// custom Decode.
type Field[T any] struct {
	// Name is the JSON member name this field matches. Names must be unique
	// within a Schema.
	Name string

	// Decode reads exactly one JSON value from r and stores it into v.
	Decode func(*Reader, *T) error

	// Encode writes v's field as exactly one JSON value to w.
	Encode func(*Writer, *T) error

	// Empty reports whether the field is empty for `omitempty`. A nil Empty
	// means the field is never empty, which is how a nested struct and a
	// fixed-size array behave.
	Empty func(*T) bool

	// omitEmpty marks the field to be skipped by Schema.Encode when Empty
	// reports true. Set it with OmitEmpty.
	omitEmpty bool
}

// OmitEmpty returns a copy of f that Schema.Encode skips when f.Empty reports
// the field is empty. It implements `json:",omitempty"`.
func (f Field[T]) OmitEmpty() Field[T] {
	f.omitEmpty = true
	return f
}

// scalar builds a Field for a value field of type E. A JSON null leaves the
// field unchanged, which is zero for a freshly decoded value.
func scalar[T, E any](name string, set func(*T) *E, read func(*Reader) (E, error), write func(*Writer, E) error, empty func(*T) bool) Field[T] {
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
		Encode: func(w *Writer, v *T) error {
			return write(w, *set(v))
		},
		Empty: empty,
	}
}

// scalarPtr builds a Field for a pointer field of type *E. A JSON null sets the
// field to nil; otherwise the value is decoded into a fresh allocation.
func scalarPtr[T, E any](name string, set func(*T) **E, read func(*Reader) (E, error), write func(*Writer, E) error, empty func(*T) bool) Field[T] {
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
		Encode: func(w *Writer, v *T) error {
			if p := *set(v); p != nil {
				return write(w, *p)
			}
			return w.WriteNull()
		},
		Empty: empty,
	}
}

// String decodes a JSON string into a string field.
func String[T any](name string, field func(*T) *string) Field[T] {
	return scalar(name, field, (*Reader).ReadString, (*Writer).WriteString, func(v *T) bool { return *field(v) == "" })
}

// StringPtr decodes a JSON string into a *string field; a JSON null sets it to
// nil.
func StringPtr[T any](name string, field func(*T) **string) Field[T] {
	return scalarPtr(name, field, (*Reader).ReadString, (*Writer).WriteString, func(v *T) bool { return *field(v) == nil })
}

// Int decodes a JSON number into an int64 field.
func Int[T any](name string, field func(*T) *int64) Field[T] {
	return scalar(name, field, (*Reader).ReadInt, (*Writer).WriteInt, func(v *T) bool { return *field(v) == 0 })
}

// IntPtr decodes a JSON number into an *int64 field; a JSON null sets it to nil.
func IntPtr[T any](name string, field func(*T) **int64) Field[T] {
	return scalarPtr(name, field, (*Reader).ReadInt, (*Writer).WriteInt, func(v *T) bool { return *field(v) == nil })
}

// Float decodes a JSON number into a float64 field.
func Float[T any](name string, field func(*T) *float64) Field[T] {
	return scalar(name, field, (*Reader).ReadFloat, (*Writer).WriteFloat, func(v *T) bool { return *field(v) == 0 })
}

// FloatPtr decodes a JSON number into a *float64 field; a JSON null sets it to
// nil.
func FloatPtr[T any](name string, field func(*T) **float64) Field[T] {
	return scalarPtr(name, field, (*Reader).ReadFloat, (*Writer).WriteFloat, func(v *T) bool { return *field(v) == nil })
}

// Bool decodes a JSON boolean into a bool field.
func Bool[T any](name string, field func(*T) *bool) Field[T] {
	return scalar(name, field, (*Reader).ReadBool, (*Writer).WriteBool, func(v *T) bool { return !*field(v) })
}

// BoolPtr decodes a JSON boolean into a *bool field; a JSON null sets it to nil.
func BoolPtr[T any](name string, field func(*T) **bool) Field[T] {
	return scalarPtr(name, field, (*Reader).ReadBool, (*Writer).WriteBool, func(v *T) bool { return *field(v) == nil })
}

// Object decodes a JSON object into a nested struct field using schema. A JSON
// null leaves the field unchanged. Because a struct is never empty, an Object
// field is never omitted by omitempty.
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
		Encode: func(w *Writer, v *T) error {
			return schema.Encode(w, field(v))
		},
	}
}

// ObjectDecoder decodes a JSON object into a nested field of type S by
// delegating to *S's own DecodeJSON and EncodeJSON. It is the delegation
// counterpart to Object: where Object needs the other package's *Schema,
// ObjectDecoder needs only that the type is qi-json-aware. A JSON null leaves
// the field unchanged, and because a struct is never empty, an ObjectDecoder
// field is never omitted by omitempty.
//
// The PS type parameter may be omitted: it is inferred from S, as with Marshal.
// The constraint requires both Decoder and Encoder, so decode and encode stay
// symmetric by construction; a type implementing only one does not compile.
func ObjectDecoder[T, S any, PS interface {
	*S
	Decoder
	Encoder
}](name string, field func(*T) *S) Field[T] {
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
			return PS(field(v)).DecodeJSON(r)
		},
		Encode: func(w *Writer, v *T) error {
			return PS(field(v)).EncodeJSON(w)
		},
	}
}

// ObjectDecoderPtr decodes a JSON object into a *S field by delegating to *S's
// own DecodeJSON and EncodeJSON. A JSON null sets the field to nil; otherwise a
// nil field is allocated before decoding. On encode a nil field is written as
// null, and an ObjectDecoderPtr field is empty for omitempty when nil.
func ObjectDecoderPtr[T, S any, PS interface {
	*S
	Decoder
	Encoder
}](name string, field func(*T) **S) Field[T] {
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
				*field(v) = nil
				return nil
			}
			s := *field(v)
			if s == nil {
				s = new(S)
				*field(v) = s
			}
			return PS(s).DecodeJSON(r)
		},
		Encode: func(w *Writer, v *T) error {
			s := *field(v)
			if s == nil {
				return w.WriteNull()
			}
			return PS(s).EncodeJSON(w)
		},
		Empty: func(v *T) bool { return *field(v) == nil },
	}
}

// DecoderCodec returns a Codec for an element of type S by delegating to *S's
// own DecodeJSON and EncodeJSON. It is the delegation counterpart to a
// struct-backed element codec, for a Slice, Map, or Array of a qi-json-aware
// type; the PS type parameter must be given explicitly (as with a zero-argument
// generic function).
func DecoderCodec[S any, PS interface {
	*S
	Decoder
	Encoder
}]() Codec[S] {
	return Codec[S]{
		Decode: func(r *Reader) (S, error) {
			var s S
			if err := PS(&s).DecodeJSON(r); err != nil {
				return s, err
			}
			return s, nil
		},
		Encode: func(w *Writer, s S) error {
			return PS(&s).EncodeJSON(w)
		},
	}
}

// Slice decodes a JSON array into a slice field, decoding each element with the
// elem codec's Decode. A JSON null leaves the field unchanged; an empty JSON
// array produces a non-nil empty slice. On encode, a nil slice is written as
// null and a non-nil slice as an array, and an empty slice is "empty" for
// omitempty.
func Slice[T, E any](name string, field func(*T) *[]E, elem Codec[E]) Field[T] {
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
			if elem.Decode == nil {
				return errNilElementDecoder
			}

			out := make([]E, 0)
			err = r.Array(func(int) error {
				e, err := elem.Decode(r)
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
		Encode: func(w *Writer, v *T) error {
			s := *field(v)
			if s == nil {
				return w.WriteNull()
			}
			if elem.Encode == nil {
				return errNilElementEncoder
			}
			if err := w.BeginArray(); err != nil {
				return err
			}
			for _, e := range s {
				if err := elem.Encode(w, e); err != nil {
					return err
				}
			}
			return w.EndArray()
		},
		Empty: func(v *T) bool { return len(*field(v)) == 0 },
	}
}

// Map decodes a JSON object into a map field with string keys, decoding each
// member value with the elem codec's Decode. A JSON null leaves the field
// unchanged; an empty JSON object produces a non-nil empty map. On encode, keys
// are written in sorted order for a stable result, a nil map becomes null, and
// an empty map is "empty" for omitempty.
func Map[T, V any](name string, field func(*T) *map[string]V, elem Codec[V]) Field[T] {
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
			if elem.Decode == nil {
				return errNilElementDecoder
			}

			m := make(map[string]V)
			err = r.Object(func(key string) error {
				e, err := elem.Decode(r)
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
		Encode: func(w *Writer, v *T) error {
			m := *field(v)
			if m == nil {
				return w.WriteNull()
			}
			if elem.Encode == nil {
				return errNilElementEncoder
			}
			if err := w.BeginObject(); err != nil {
				return err
			}
			for _, key := range slices.Sorted(maps.Keys(m)) {
				if err := w.Key(key); err != nil {
					return err
				}
				if err := elem.Encode(w, m[key]); err != nil {
					return err
				}
			}
			return w.EndObject()
		},
		Empty: func(v *T) bool { return len(*field(v)) == 0 },
	}
}

// Array decodes a JSON array into a fixed-size array field. field returns a
// slice view of the array (such as arr[:] for an array arr), so the decoder
// writes through to the array. The JSON array must hold exactly len(field(v))
// elements: fewer is an underflow error and more is an overflow error, both
// reported as type errors. A JSON null leaves the field unchanged. On encode a
// fixed-size array is always written and is never omitted by omitempty.
func Array[T, E any](name string, field func(*T) []E, elem Codec[E]) Field[T] {
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
			if elem.Decode == nil {
				return errNilElementDecoder
			}

			view := field(v)
			n := len(view)
			start := r.Offset()

			out := make([]E, 0, n)
			err = r.Array(func(int) error {
				e, err := elem.Decode(r)
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
		Encode: func(w *Writer, v *T) error {
			if elem.Encode == nil {
				return errNilElementEncoder
			}
			view := field(v)
			if err := w.BeginArray(); err != nil {
				return err
			}
			for _, e := range view {
				if err := elem.Encode(w, e); err != nil {
					return err
				}
			}
			return w.EndArray()
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
		Encode: func(w *Writer, v *T) error {
			return encodeAny(w, *field(v))
		},
		Empty: func(v *T) bool { return *field(v) == nil },
	}
}

// Raw decodes any JSON value into a []byte field holding the value's exact
// literal bytes. The bytes are copied, so they do not alias the input. On encode
// a nil or empty slice is written as null; otherwise the bytes are written back
// verbatim, without validation.
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
		Encode: func(w *Writer, v *T) error {
			b := *field(v)
			if len(b) == 0 {
				return w.WriteNull()
			}
			return w.WriteRaw(b)
		},
		Empty: func(v *T) bool { return len(*field(v)) == 0 },
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

// encodeAny writes a value decoded into an any field, or a value a caller put
// there, back as JSON: nil to null, bool, string, float and integer kinds at
// their natural width, []any to an array, map[string]any to an object with
// sorted keys, and an Encoder through its EncodeJSON. Any other dynamic type is
// an error, since qijson may not use reflection to discover one.
func encodeAny(w *Writer, v any) error {
	switch x := v.(type) {
	case nil:
		return w.WriteNull()
	case bool:
		return w.WriteBool(x)
	case string:
		return w.WriteString(x)
	case float64:
		return w.WriteFloat(x)
	case float32:
		return w.WriteFloat(float64(x))
	case int:
		return w.WriteInt(int64(x))
	case int8:
		return w.WriteInt(int64(x))
	case int16:
		return w.WriteInt(int64(x))
	case int32:
		return w.WriteInt(int64(x))
	case int64:
		return w.WriteInt(x)
	case uint:
		return w.WriteUint(uint64(x))
	case uint8:
		return w.WriteUint(uint64(x))
	case uint16:
		return w.WriteUint(uint64(x))
	case uint32:
		return w.WriteUint(uint64(x))
	case uint64:
		return w.WriteUint(x)
	case []any:
		if err := w.BeginArray(); err != nil {
			return err
		}
		for _, e := range x {
			if err := encodeAny(w, e); err != nil {
				return err
			}
		}
		return w.EndArray()
	case map[string]any:
		if err := w.BeginObject(); err != nil {
			return err
		}
		for _, key := range slices.Sorted(maps.Keys(x)) {
			if err := w.Key(key); err != nil {
				return err
			}
			if err := encodeAny(w, x[key]); err != nil {
				return err
			}
		}
		return w.EndObject()
	case Encoder:
		return x.EncodeJSON(w)
	default:
		return w.fail(fmt.Errorf("qijson: cannot encode %T as JSON: %w", v, errUnsupportedDynamicType))
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
