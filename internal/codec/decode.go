package codec

import "errors"

// Decoder is implemented by types that decode themselves from JSON. A decoder
// reads exactly one JSON value from r. The usual way to implement it is to bind a
// Schema and delegate:
//
//	var configSchema = qijson.Struct(
//	    qijson.String[Config]("name", func(c *Config) *string { return &c.Name }),
//	)
//
//	func (c *Config) DecodeJSON(r *qijson.Reader) error {
//	    return configSchema.Decode(r, c)
//	}
type Decoder interface {
	DecodeJSON(*Reader) error
}

// errNilTarget reports UnmarshalInto called with a nil target pointer. It is a
// programming error rather than a property of the input, so it is not an *Error.
var errNilTarget = errors.New("qijson: nil target")

// Unmarshal decodes data into a new T. The pointer type *T must implement
// Decoder, which the compiler enforces, so the type argument alone selects the
// decoder:
//
//	cfg, err := qijson.Unmarshal[Config](data)
//
// The document must hold exactly one JSON value; trailing input is an error.
// data is not retained after the call.
func Unmarshal[T any, PT interface {
	*T
	Decoder
}](data []byte) (T, error) {
	var v T
	if err := UnmarshalInto[T, PT](data, &v); err != nil {
		return v, err
	}
	return v, nil
}

// UnmarshalInto decodes data into v, which must be non-nil. v is decoded in
// place: members present in data overwrite the corresponding parts of v and
// members absent from data leave v unchanged. It is the merge counterpart to
// Unmarshal, matching Schema.UnmarshalInto.
//
// The document must hold exactly one JSON value; trailing input is an error.
// data is not retained after the call.
func UnmarshalInto[T any, PT interface {
	*T
	Decoder
}](data []byte, v *T) error {
	if v == nil {
		return errNilTarget
	}
	return Decode(data, PT(v))
}

// Decode decodes the single JSON value in data into v. It is the non-generic
// counterpart to UnmarshalInto for a caller who does not want to name the type
// argument:
//
//	var cfg Config
//	if err := qijson.Decode(data, &cfg); err != nil { /* ... */ }
//
// v must implement Decoder, which a type does by defining DecodeJSON (usually by
// delegating to a Schema). Decode works in place, merging into whatever v already
// holds, and reports the same errors as UnmarshalInto: a syntax error, a JSON
// type that does not match v, or trailing input after the single value. v is
// written in place and data is not retained after the call.
//
// A nil interface is reported as errNilTarget. Because qijson may not use
// reflection, a typed-nil target (an interface holding a nil pointer) is not
// detected here: its DecodeJSON is called with a nil receiver and must behave as
// any method does when its receiver is nil.
func Decode(data []byte, v Decoder) error {
	if v == nil {
		return errNilTarget
	}

	r := newReader(data)
	err := v.DecodeJSON(r)
	if err == nil {
		// The document must hold exactly one value; Peek reports trailing content.
		if _, perr := r.Peek(); perr != nil {
			err = perr
		}
	}
	r.release()
	return err
}
