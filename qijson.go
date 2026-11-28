package qijson

// This file is the public facade of the semantic layer. The implementation lives
// in github.com/qi-lore/qi-json/internal/codec and is re-exported here through
// type aliases, constant re-exports and one-line wrappers, so external code keeps
// using the github.com/qi-lore/qi-json import path and the engine's internals
// stay free to change.
//
// The aliases make the types identical across the boundary: a *Reader built by
// this package is a *codec.Reader, errors.As reaches a *Error, and a generated
// Field[T] literal compiles unchanged.
//
// The public documentation lives here. go doc does not follow aliases for method
// sets, so the method reference for the aliased types remains on the
// declarations in internal/codec; the type-level contract (ownership,
// concurrency and lifecycle) is repeated here.
//
// This file also re-exports the syntax engine from internal/scanner — Valid, the
// Kind taxonomy and the Tokenizer — so the whole module has one public import
// path, github.com/qi-lore/qi-json.

import (
	"bytes"
	"io"

	"github.com/qi-lore/qi-json/internal/codec"
	"github.com/qi-lore/qi-json/internal/scanner"
)

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

	// KindDepth reports input that nests deeper than the decoder's fixed
	// nesting limit. The limit matches the Go standard library: encoding/json
	// defines maxNestingDepth = 10000 and rejects deeper input with "exceeded
	// max depth".
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
// github.com/qi-lore/qi-json/cmd/qi-json-gen or hand-written to delegate to a
// Schema.
//
// The returned []byte is freshly allocated and owned by the caller; it does not
// alias v.
func Marshal[T any, PT interface {
	*T
	Encoder
}](v T) ([]byte, error) {
	return codec.Marshal[T, PT](v)
}

// MarshalIndent encodes v as JSON and returns a pretty-printed form of it. prefix
// begins every new line and indent is repeated once per level of nesting, so
// MarshalIndent(v, "", "  ") indents each level by two spaces:
//
//	data, err := qijson.MarshalIndent[Config](cfg, "", "  ")
//
// It mirrors Marshal's constraint — the pointer type *T must implement Encoder —
// and, like Marshal, it leaves HTML escaping off. An empty object or array stays
// on one line as "{}" or "[]", and no newline follows the final token.
//
// The returned []byte is freshly allocated and owned by the caller; it does not
// alias v. On an encoding failure MarshalIndent returns (nil, err) and writes
// nothing. It is safe for concurrent use.
func MarshalIndent[T any, PT interface {
	*T
	Encoder
}](v T, prefix, indent string) ([]byte, error) {
	return codec.MarshalIndent[T, PT](v, prefix, indent)
}

// Indent appends a pretty-printed form of the JSON text in src to dst. prefix
// begins every new line and indent is repeated once per level of nesting. Inside a
// container each element or member starts on its own line, a ':' is followed by a
// single space, and an empty object or array stays on one line as "{}" or "[]".
// No newline is written after the final token. The bytes inside JSON strings are
// preserved verbatim; only insignificant whitespace is normalised.
//
// On malformed input Indent appends nothing and returns a *Error (Kind KindSyntax)
// that records the byte offset where the input became invalid. src is not retained
// after the call.
//
// Indent is a pure function of its arguments and holds no state, so it is safe for
// concurrent use. Because it appends to dst, callers must not share one
// *bytes.Buffer across goroutines.
func Indent(dst *bytes.Buffer, src []byte, prefix, indent string) error {
	return codec.Indent(dst, src, prefix, indent)
}

// Compact appends the JSON text in src to dst with all insignificant whitespace
// removed, so the result is a single compact line. Whitespace inside JSON strings
// is part of the string and is preserved.
//
// On malformed input Compact appends nothing and returns a *Error (Kind KindSyntax)
// that records the byte offset where the input became invalid. src is not retained
// after the call.
//
// Compact is a pure function of its arguments and holds no state, so it is safe
// for concurrent use. Because it appends to dst, callers must not share one
// *bytes.Buffer across goroutines.
func Compact(dst *bytes.Buffer, src []byte) error {
	return codec.Compact(dst, src)
}

// HTMLEscape appends the JSON text in src to dst with the HTML-sensitive bytes
// replaced: '<' becomes \u003c, '>' becomes \u003e, '&' becomes \u0026, U+2028
// becomes \u2028 and U+2029 becomes \u2029. Every other byte is copied unchanged.
// The replacements are the ones encoding/json applies so a document can be
// embedded in an HTML <script> element.
//
// HTMLEscape never fails and has no error return: it copies its input rather than
// validating it, so it does not report malformed JSON. src is not retained after
// the call.
//
// HTMLEscape is a pure function of its arguments and holds no state, so it is safe
// for concurrent use. Because it appends to dst, callers must not share one
// *bytes.Buffer across goroutines.
func HTMLEscape(dst *bytes.Buffer, src []byte) {
	codec.HTMLEscape(dst, src)
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
// from the pull tokenizer: whitespace,
// separators and object keys are handled by the tokenizer and the Object and
// Array iterators.
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
// WriteRaw, BeginObject, EndObject, BeginArray, EndArray, Key, Bytes, BuildBytes,
// DumpTo, ReadCloser, Reset, SetIndent, SetEscapeHTML and Err. By default the
// document is compact and HTML escaping is off; SetIndent and SetEscapeHTML
// configure those styles and are sticky across Reset.
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
// Encode writes members in that argument (declaration) order. A repeated name is
// a programming error: every Decode and Encode of the returned Schema fails with
// an error instead of silently binding and writing an ambiguous member.
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

// Optional is implemented by a field type that decides for itself whether it
// carries a value. Under `omitempty` a field whose declared type implements
// Optional is omitted exactly when its IsDefined method reports false; a field
// not marked for omission is always written. The generator wires this hook into
// the field's emptiness check; a hand-written Field may call IsDefined from its
// Empty closure instead.
//
// A field type satisfies Optional through a value receiver or a pointer
// receiver: it is enough that the type or a pointer to it has the method.
type Optional = codec.Optional

// IsZero is implemented by a field type that defines its own notion of
// emptiness. Under `omitempty` a field whose declared type implements IsZero is
// omitted exactly when its IsZero method reports true; a field not marked for
// omission is always written. When a type implements both IsZero and Optional,
// IsZero takes precedence. The generator wires this hook into the field's
// emptiness check; a hand-written Field may call IsZero from its Empty closure
// instead.
//
// A field type satisfies IsZero through a value receiver or a pointer receiver:
// it is enough that the type or a pointer to it has the method.
type IsZero = codec.IsZero

// UnmarshalUnknown is implemented by a decode target that wants to receive the
// object members no field matches, instead of seeing them ignored. During
// Schema.Decode the decoder calls UnmarshalUnknown once per unmatched member:
// name is the member name, decoded from any JSON escape sequences, and raw is the
// member value as its exact JSON bytes, passed as a copy the callee owns and may
// retain. A non-nil error aborts the decode and is returned unchanged.
//
// RejectUnknown takes precedence: when a Schema has both, an unmatched member is
// reported as an unknown-field error and UnmarshalUnknown is not called. Known
// members bind to their fields as usual, and a duplicate known member follows the
// RejectDuplicate policy independently.
//
// A target satisfies UnmarshalUnknown through a value receiver or a pointer
// receiver; it is discovered by a type assertion on the decode target, so no
// reflection is involved. Pair it with MarshalUnknowns to round-trip the captured
// members back on encode.
type UnmarshalUnknown = codec.UnmarshalUnknown

// MarshalUnknowns is implemented by an encode target that holds members captured
// by UnmarshalUnknown and wants them written back. During Schema.Encode the
// encoder calls MarshalUnknowns once, after the declared fields and while the
// object is still open, so the implementation writes each extra member with
// Writer.Key followed by a value. The extra members follow the declared fields,
// in the order the implementation writes them. A non-nil error aborts the encode
// and is returned unchanged.
//
// A target satisfies MarshalUnknowns through a value receiver or a pointer
// receiver; it is discovered by a type assertion on the encode target, so no
// reflection is involved.
type MarshalUnknowns = codec.MarshalUnknowns

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

// Kind identifies the JSON token a Tokenizer is positioned on: the token
// taxonomy of the syntax engine. Peek reports one of these, and a decoder or
// Field that branches on the shape of the next JSON value compares it — for
// example, a JSON null sets a pointer field to nil.
//
// A Kind returned by Reader.Peek is identical to the syntax engine's, so it
// compares equal to the Kind constants below.
type Kind = scanner.Kind

const (
	// KindNull is the null literal.
	KindNull = scanner.KindNull

	// KindBool is the true or false literal.
	KindBool = scanner.KindBool

	// KindString is a string value or an object key.
	KindString = scanner.KindString

	// KindNumber is a number literal.
	KindNumber = scanner.KindNumber

	// KindObject is an object value, introduced by '{'.
	KindObject = scanner.KindObject

	// KindArray is an array value, introduced by '['.
	KindArray = scanner.KindArray

	// KindEnd is reported by Peek at the closing '}' or ']' of the current
	// object or array, and at the end of the document once the top-level value
	// and any trailing whitespace have been consumed.
	KindEnd = scanner.KindEnd
)

// Token is one token of a JSON document, as returned by Tokenizer.Token: nil for
// null, a bool for true or false, a Number for a number, a string for a string
// value or object key, and a Delim for an object or array delimiter. It is an
// alias of the syntax engine's token union, so a token produced by Token and
// type-asserted here is identical to the engine's.
type Token = scanner.Token

// Number is a JSON number literal, held as its raw bytes exactly as they appear
// in the input. Token returns a Number rather than a float64 so the literal is
// preserved with full precision; convert it with strconv when a numeric value is
// needed.
type Number = scanner.Number

// Delim is a JSON delimiter: one of '{', '}', '[' or ']'. Token returns it to
// mark the opening and closing of an object or array.
type Delim = scanner.Delim

// Valid reports whether data is a syntactically valid JSON text. It is a single
// pass over data: it walks with an index, scans strings, numbers, whitespace and
// literals in bulk, and keeps an explicit stack of open containers. It does not
// build values and does not validate UTF-8 or surrogate pairs, and unlike
// encoding/json.Valid it accepts any JSON value at the top level and applies no
// nesting depth limit. Valid is safe for concurrent use.
func Valid(data []byte) bool {
	return scanner.Valid(data)
}

// Tokenizer turns a JSON document into a stream of typed tokens. It reads JSON
// without building Go values, which suits a minifier, a streaming splitter or an
// analyzer that walks arbitrary documents. Construct one with NewTokenizer for a
// byte slice or NewDecoder for an io.Reader; the zero value is not ready to use.
//
// The methods are pull-based: Peek reports the kind of the next value token
// without consuming it, and the Read*/Begin*/End* methods consume it. Token is
// the single-call alternative: it returns the next token as a Token (nil, bool,
// Number, string or Delim) and io.EOF at the end of the stream.
// Whitespace, commas and the ':' after an object key are consumed
// automatically, so inside an object the stream is key, value, key, value, ...
// and Peek reports KindEnd at the closing brace. Inside an array it is value,
// value, ... and Peek reports KindEnd at the closing bracket. At the top level a
// NewTokenizer tokenizer reports KindEnd once the single top-level value and its
// trailing whitespace are consumed, while a NewDecoder tokenizer reads
// successive top-level values (NDJSON) and reports KindEnd at the end of the
// stream.
//
// Byte slices returned by ReadString, ReadNumber and ReadRaw alias the input
// passed to NewTokenizer and must not be modified. A string or Number returned
// by Token is instead a copy: escape sequences are left intact (the syntax layer
// does not unescape) and the value is independent of the Tokenizer, so it is
// safe to retain.
//
// A Tokenizer must not be copied after first use, and it is not safe for
// concurrent use: give each goroutine its own.
type Tokenizer = scanner.Tokenizer

// NewTokenizer returns a Tokenizer positioned at the start of data. The
// Tokenizer reads from data without copying: the caller must keep data alive and
// unmodified for the Tokenizer's lifetime, and every byte slice a read returns
// aliases data.
func NewTokenizer(data []byte) *Tokenizer {
	return scanner.NewTokenizer(data)
}

// NewDecoder returns a Tokenizer that reads one or more JSON documents from r.
// It is the streaming counterpart to NewTokenizer: instead of a byte slice, the
// Tokenizer pulls bytes from r on demand and refills its internal buffer as it
// goes, so a value may straddle a read boundary and r may return short reads.
// For a single document the Kind sequence, the scalar values and the Offset
// values it reports are the same as NewTokenizer's over the same bytes.
//
// It reads several top-level values from one stream (NDJSON): once a value is
// complete, a following value that can begin here starts the next one, and Peek
// reports KindEnd at the end of the input. A byte that cannot begin a value
// after a completed value is an "after top-level value" error.
//
// Ownership and lifecycle: the Tokenizer does not own r and never closes it; the
// caller keeps r open and valid for the Tokenizer's lifetime and closes it when
// done. The Tokenizer reads from r only while a method needs more input, and it
// stops once the stream is exhausted. A Tokenizer is not safe for concurrent
// use: give each goroutine its own, with its own reader.
//
// Aliasing: byte slices returned by ReadString, ReadNumber and ReadRaw alias the
// Tokenizer's internal buffer, which grows as r is drained. Unlike NewTokenizer,
// where a returned slice aliases the caller's data and stays valid for the
// Tokenizer's lifetime, a slice returned here is valid only until the next read
// method, which may append to the buffer and move it. Copy the bytes to keep
// them. A string or Number returned by Token is a copy and is always safe to
// retain.
//
// Token reports io.EOF once every value in the stream has been consumed.
//
// Errors: a read error from r other than io.EOF is returned, wrapped, as the
// Tokenizer's sticky error; a syntax error is reported exactly as NewTokenizer
// reports it for the same bytes. A nil r yields a nil-reader error rather than a
// panic.
func NewDecoder(r io.Reader) *Tokenizer {
	return scanner.NewDecoder(r)
}
