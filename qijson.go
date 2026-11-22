package qijson

// This file is the public facade of the semantic layer. The implementation lives
// in qi-json/internal/codec and is re-exported here through type aliases,
// constant re-exports and one-line wrappers, so external code keeps using the
// qi-json import path and the engine's internals stay free to change.
//
// The aliases make the types identical across the boundary: a *Reader built by
// this package is a *codec.Reader, errors.As reaches a *Error, and a generated
// Field[T] literal compiles unchanged.
//
// The public documentation lives here. go doc does not follow aliases for method
// sets, so the method reference for the aliased types remains on the
// declarations in internal/codec; the type-level contract (ownership,
// concurrency and lifecycle) is repeated here.

import "qi-json/internal/codec"

// Error describes a failure to decode JSON into a Go value. It records where in
// the input the failure happened and wraps the underlying cause so callers can
// inspect it with errors.As and errors.Unwrap. A *Error returned by this package
// is immutable: its fields may be read, including from another goroutine, but
// never written. It implements error; Unwrap returns the cause.
type Error = codec.Error

// TypeError describes a JSON value whose type does not match the target. A
// returned *TypeError is immutable.
type TypeError = codec.TypeError

// ErrorKind classifies a decode error. String returns a short name for the kind.
type ErrorKind = codec.ErrorKind

const (
	// KindSyntax reports invalid JSON in the input.
	KindSyntax = codec.KindSyntax

	// KindType reports a JSON value whose type does not match the target.
	KindType = codec.KindType

	// KindUnknownField reports an object member that no field matched. It is
	// produced only in strict mode.
	KindUnknownField = codec.KindUnknownField

	// KindDuplicateField reports a repeated object member. It is produced only
	// in strict mode.
	KindDuplicateField = codec.KindDuplicateField

	// KindDepth reports input that nests deeper than the configured limit.
	KindDepth = codec.KindDepth
)

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
type Decoder = codec.Decoder

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
	return codec.Unmarshal[T, PT](data)
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
	return codec.UnmarshalInto[T, PT](data, v)
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
// A nil interface is reported as a nil-target sentinel error. Because qijson may
// not use reflection, a typed-nil target (an interface holding a nil pointer) is
// not detected here: its DecodeJSON is called with a nil receiver and must
// behave as any method does when its receiver is nil.
func Decode(data []byte, v Decoder) error {
	return codec.Decode(data, v)
}

// Encoder is implemented by types that encode themselves to JSON. An encoder
// writes exactly one JSON value to w. The usual implementation binds a Schema
// and delegates:
//
//	var configSchema = qijson.Struct(
//	    qijson.String[Config]("name", func(c *Config) *string { return &c.Name }),
//	)
//
//	func (c *Config) EncodeJSON(w *qijson.Writer) error {
//	    return configSchema.Encode(w, c)
//	}
type Encoder = codec.Encoder

// Marshal encodes v as JSON. The pointer type *T must implement Encoder, which
// the compiler enforces, so the type argument alone selects the encoder:
//
//	data, err := qijson.Marshal[Config](cfg)
//
// It is the counterpart to Unmarshal. Because qijson may not use reflection, a
// type is encoded by its EncodeJSON method, usually one generated by
// cmd/qi-json-gen or hand-written to delegate to a Schema.
//
// The returned []byte is freshly allocated and owned by the caller; it does not
// alias v.
func Marshal[T any, PT interface {
	*T
	Encoder
}](v T) ([]byte, error) {
	return codec.Marshal[T, PT](v)
}

// Encode encodes v into w. It is the non-generic counterpart to Marshal for a
// caller that already holds an Encoder or wants to reuse a Writer:
//
//	var w qijson.Writer
//	if err := qijson.Encode(&w, &cfg); err != nil { /* ... */ }
//
// A nil Writer is reported as a nil-writer sentinel error, and an untyped nil
// Encoder as a nil-target sentinel error. An error the Encoder left on the
// Writer is returned even if the Encoder itself returned nil. A typed nil — an
// interface holding a nil *T — is not detected, because doing so needs
// reflection, so an EncodeJSON method must tolerate a nil receiver.
//
// Encode writes only to w and does not retain v.
func Encode(w *Writer, v Encoder) error {
	return codec.Encode(w, v)
}

// Reader decodes typed values from a JSON document, tracking the JSON path of
// the value being decoded so that errors point at the offending member. It reads
// from the pull tokenizer in qi-json/scanner: whitespace, separators and object
// keys are handled by the tokenizer and the Object and Array iterators.
//
// The JSON path is built lazily: descending into a container records a component
// and the path string is only rendered when Path (or an error that carries it)
// needs it, so a successful decode never formats the path.
//
// A Reader is created by the decoder entry points and passed to Decoder
// implementations. It is not safe for concurrent use and must not be copied
// after first use (its component stack points into itself). Its methods are
// Peek, ReadNull, ReadBool, ReadString, ReadInt, ReadFloat, Object, Array, Skip,
// Path and Offset.
type Reader = codec.Reader

// Writer builds a JSON document in memory. The zero value is ready to use; call
// NewWriter only for a fresh, explicitly allocated writer. Write one JSON value
// with the Write* methods, framing objects and arrays with Begin/End and naming
// object members with Key:
//
//	var w qijson.Writer
//	w.BeginObject()
//	w.Key("name")
//	w.WriteString("qi-json")
//	w.Key("port")
//	w.WriteInt(8080)
//	w.EndObject()
//	data := w.Bytes()
//
// A mistake (a Key outside an object, an unbalanced End, a second top-level
// value) sets a sticky error: the remaining calls are no-ops and Bytes returns
// nil. Read the failure with Err. Encoding into a Writer never uses reflection. A
// Writer is not safe for concurrent use; give each goroutine its own. Its methods
// are WriteNull, WriteBool, WriteString, WriteInt, WriteUint, WriteFloat,
// WriteRaw, BeginObject, EndObject, BeginArray, EndArray, Key, Bytes, Reset and
// Err.
type Writer = codec.Writer

// NewWriter returns a new, empty Writer.
func NewWriter() *Writer {
	return codec.NewWriter()
}

// Schema decodes and encodes JSON objects into and from values of T. Build one
// with Struct; the zero value is not usable.
//
// A Schema is immutable once built and safe for concurrent use by Decode, Encode,
// Marshal, Unmarshal and UnmarshalInto. RejectUnknown and RejectDuplicate mutate
// it, so finish configuring it before sharing it. Its methods are Decode, Encode,
// Marshal, Unmarshal, UnmarshalInto, RejectUnknown and RejectDuplicate.
type Schema[T any] = codec.Schema[T]

// Struct returns a Schema that decodes and encodes a JSON object using fields.
// Field names must be unique; the order of the arguments does not matter, and
// Encode writes members in that argument (declaration) order.
func Struct[T any](fields ...Field[T]) *Schema[T] {
	return codec.Struct(fields...)
}

// Field describes how to decode and encode one JSON object member into a field
// of T. Name is the JSON member name the field matches; Decode reads the member
// value from the Reader and stores it into v, and Encode writes it back to a
// Writer. Build fields with the type-specific constructors (String, Int, Float,
// Bool and their Ptr variants, Object, Slice, Map, Array, Any, Raw) or supply a
// custom Decode.
//
// A Field is a plain value holding closures and carries no state of its own, so
// it is safe to share exactly when the target its closures read and write is not
// shared.
type Field[T any] = codec.Field[T]

// String binds a JSON string member to a *string field of T. A JSON null leaves
// the field unchanged.
func String[T any](name string, field func(*T) *string) Field[T] {
	return codec.String(name, field)
}

// StringPtr binds a JSON string member to a **string field of T. A JSON null
// sets the field to nil.
func StringPtr[T any](name string, field func(*T) **string) Field[T] {
	return codec.StringPtr(name, field)
}

// Int binds a JSON number with no fraction or exponent to an *int64 field of T.
// A JSON null leaves the field unchanged.
func Int[T any](name string, field func(*T) *int64) Field[T] {
	return codec.Int(name, field)
}

// IntPtr binds a JSON integer to an **int64 field of T. A JSON null sets the
// field to nil.
func IntPtr[T any](name string, field func(*T) **int64) Field[T] {
	return codec.IntPtr(name, field)
}

// Float binds a JSON number to a *float64 field of T. A JSON null leaves the
// field unchanged.
func Float[T any](name string, field func(*T) *float64) Field[T] {
	return codec.Float(name, field)
}

// FloatPtr binds a JSON number to a **float64 field of T. A JSON null sets the
// field to nil.
func FloatPtr[T any](name string, field func(*T) **float64) Field[T] {
	return codec.FloatPtr(name, field)
}

// Bool binds a JSON boolean to a *bool field of T. A JSON null leaves the field
// unchanged.
func Bool[T any](name string, field func(*T) *bool) Field[T] {
	return codec.Bool(name, field)
}

// BoolPtr binds a JSON boolean to a **bool field of T. A JSON null sets the
// field to nil.
func BoolPtr[T any](name string, field func(*T) **bool) Field[T] {
	return codec.BoolPtr(name, field)
}

// Object binds a JSON object member to an *S field of T using schema. A JSON
// null leaves the field unchanged.
func Object[T, S any](name string, field func(*T) *S, schema *Schema[S]) Field[T] {
	return codec.Object[T, S](name, field, schema)
}

// ObjectDecoder binds a JSON object member to an *S field of T by delegating to
// *S's own DecodeJSON and EncodeJSON. A JSON null leaves the field unchanged.
//
// The PS type parameter may be omitted: it is inferred from S, as with Marshal.
// The constraint requires both Decoder and Encoder, so decode and encode stay
// symmetric by construction; a type implementing only one does not compile.
func ObjectDecoder[T, S any, PS interface {
	*S
	Decoder
	Encoder
}](name string, field func(*T) *S) Field[T] {
	return codec.ObjectDecoder[T, S, PS](name, field)
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
	return codec.ObjectDecoderPtr[T, S, PS](name, field)
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
	return codec.DecoderCodec[S, PS]()
}

// Slice decodes a JSON array into a slice field, decoding each element with the
// elem codec's Decode. A JSON null leaves the field unchanged; an empty JSON
// array produces a non-nil empty slice. On encode, a nil slice is written as
// null and a non-nil slice as an array, and an empty slice is "empty" for
// omitempty.
func Slice[T, E any](name string, field func(*T) *[]E, elem Codec[E]) Field[T] {
	return codec.Slice[T, E](name, field, elem)
}

// Map decodes a JSON object into a map field, decoding each member value with
// the elem codec's Decode. A JSON null leaves the field unchanged. On encode,
// a nil map is written as null, map keys are sorted, and an empty map is "empty"
// for omitempty.
func Map[T, V any](name string, field func(*T) *map[string]V, elem Codec[V]) Field[T] {
	return codec.Map[T, V](name, field, elem)
}

// Array decodes a JSON array into a fixed-size array field through the field
// slice, decoding each element with the elem codec's Decode. The JSON array
// length must match the Go array length. A JSON null leaves the field unchanged.
func Array[T, E any](name string, field func(*T) []E, elem Codec[E]) Field[T] {
	return codec.Array[T, E](name, field, elem)
}

// Any binds a JSON member of any type to an *any field of T. The decoded value
// is a nil, bool, string, float64, []any or map[string]any. A JSON null sets the
// field to nil.
func Any[T any](name string, field func(*T) *any) Field[T] {
	return codec.Any(name, field)
}

// Raw binds a JSON member to a *[]byte field of T, storing the member's raw JSON
// bytes. A JSON null leaves the field unchanged.
func Raw[T any](name string, field func(*T) *[]byte) Field[T] {
	return codec.Raw(name, field)
}

// Codec decodes and encodes one JSON value of type E. It pairs the decode and
// encode functions of a container element, so a Slice, Map, or Array field
// carries both directions; build one with NewCodec. A Codec is a plain pair of
// functions with no state of its own: its concurrency is that of the target the
// functions read and write, so a Codec is safe to share only when the values it
// touches are not shared.
type Codec[E any] = codec.Codec[E]

// NewCodec returns a Codec pairing a decoder and an encoder for element type E.
// Either function may be nil when that direction is not used. The Codec holds no
// state beyond the two functions, so its safety for concurrent use is the
// safety of the closures.
func NewCodec[E any](decode func(*Reader) (E, error), encode func(*Writer, E) error) Codec[E] {
	return codec.NewCodec(decode, encode)
}
