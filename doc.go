// Package qijson is the semantic layer of qi-json: a reflection-free JSON
// decoder and encoder built on an internal single-pass syntax engine. The
// same package also exposes that engine's syntax validator (Valid) and its
// pull tokenizer (Tokenizer), so a caller can validate or walk a document
// without going through the semantic layer.
//
// The API is deliberately not compatible with encoding/json. Decoding and
// encoding arbitrary targets without reflection needs a compile-time field
// mapping, so struct binding uses accessor closures (Field[T]) rather than
// reflect or unsafe.
//
// A Reader decodes typed values from a document, tracking the JSON path so that
// errors point at the offending member. A Schema[T] binds JSON object members to
// struct fields: build one with Struct and the field constructors (String, Int,
// Float, Bool, Object, Slice, Map, Array, Any, Raw and their Ptr variants), then
// decode with Schema.Unmarshal or Schema.Decode.
//
//	var configSchema = qijson.Struct(
//	    qijson.String[Config]("name", func(c *Config) *string { return &c.Name }),
//	    qijson.Int[Config]("port", func(c *Config) *int64 { return &c.Port }),
//	)
//
//	cfg, err := configSchema.Unmarshal(data)
//
// A JSON null leaves a value field unchanged (zero for a fresh value) and sets a
// pointer field to nil. A member that no field matches is ignored unless
// RejectUnknown is set; a member that repeats is overwritten unless
// RejectDuplicate is set.
//
// A decoded string value or object member name must be valid UTF-8; invalid
// UTF-8 is a syntax error. Valid is syntax-only and does not check UTF-8, so
// the decoder is stricter than Valid on this one point.
//
// A type that implements Decoder can be decoded without naming its Schema. The
// usual implementation delegates to a Schema:
//
//	func (c *Config) DecodeJSON(r *qijson.Reader) error {
//	    return configSchema.Decode(r, c)
//	}
//
//	cfg, err := qijson.Unmarshal[Config](data)        // fresh value
//	err = qijson.UnmarshalInto[Config](data, &cfg)    // merge into an existing value
//	err = qijson.Decode(data, &cfg)                   // merge, with no type argument
//
// The encoder is the mirror image. A type implements Encoder by writing itself
// to a Writer, usually by delegating to its Schema:
//
//	func (c *Config) EncodeJSON(w *qijson.Writer) error {
//	    return configSchema.Encode(w, c)
//	}
//
//	data, err := qijson.Marshal[Config](cfg)   // generic, value in
//	err = qijson.Encode(&w, &cfg)              // non-generic, into a Writer
//
// Schema.Encode writes object members in declaration order and skips a field
// tagged `json:",omitempty"` whose value is empty. A container field constructor
// (Slice, Map, Array) takes a Codec[E] pairing the element's decoder and
// encoder. The encoder writes a nil slice or map as null, sorts map keys, and
// replaces invalid UTF-8 in a string with U+FFFD (the decoder, in contrast,
// rejects invalid UTF-8). It never uses reflection.
//
// A decode failure is a *Error: it records the JSON path of the offending value,
// the byte offset, a Kind (syntax, type, unknown field, duplicate field or
// depth) and an underlying cause. The *Error is returned unwrapped — it is
// itself the context a caller needs — so errors.As reaches it and errors.Is
// reaches its cause (a *TypeError, or a decoder sentinel such as the
// invalid-UTF-8 cause). A misuse of the API (a nil target, a nil Codec
// direction, a Field without an encoder, a non-finite number) is instead a plain
// sentinel error, defined once in this package and reachable with errors.Is.
// The wrapping rule is: a layer adds context with %w only when it has new
// context to add; the *Error already carries the path and offset, so it is
// returned unwrapped.
//
// A field or element whose type is a struct from another package can be bound
// without hand-written closures. If the foreign type implements Decoder and
// Encoder, build the field with ObjectDecoder (or ObjectDecoderPtr for a pointer
// field) and a container element with DecoderCodec, so it delegates to the
// type's own DecodeJSON/EncodeJSON. The code generator emits exactly these
// constructors for a qi-json-aware foreign type; for a plain foreign struct it
// synthesizes a schema from the struct's exported fields. Only a foreign
// struct's exported fields round-trip: unexported fields are skipped, as with
// encoding/json.
package qijson
