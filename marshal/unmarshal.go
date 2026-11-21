package marshal

import (
	"errors"

	"qi-json/scanner"
)

// Decoder is implemented by types that decode themselves from JSON. A decoder
// reads exactly one JSON value from r. The usual way to implement it is to bind a
// Schema and delegate:
//
//	var configSchema = marshal.Struct(
//	    marshal.String[Config]("name", func(c *Config) *string { return &c.Name }),
//	)
//
//	func (c *Config) DecodeJSON(r *marshal.Reader) error {
//	    return configSchema.Decode(r, c)
//	}
type Decoder interface {
	DecodeJSON(*Reader) error
}

// errNilTarget reports UnmarshalInto called with a nil target pointer. It is a
// programming error rather than a property of the input, so it is not an *Error.
var errNilTarget = errors.New("marshal: nil target")

// Unmarshal decodes data into a new T. The pointer type *T must implement
// Decoder, which the compiler enforces, so the type argument alone selects the
// decoder:
//
//	cfg, err := marshal.Unmarshal[Config](data)
//
// The document must hold exactly one JSON value; trailing input is an error.
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
func UnmarshalInto[T any, PT interface {
	*T
	Decoder
}](data []byte, v *T) error {
	if v == nil {
		return errNilTarget
	}

	r := newReader(scanner.NewTokenizer(data))
	if err := PT(v).DecodeJSON(r); err != nil {
		return err
	}
	// The document must hold exactly one value; Peek reports trailing content.
	if _, err := r.Peek(); err != nil {
		return err
	}
	return nil
}
