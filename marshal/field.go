package marshal

import "qi-json/scanner"

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
