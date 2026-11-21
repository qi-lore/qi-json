// Package marshal is the semantic layer of qi-json: a reflection-free JSON
// decoder built on the syntax engine in qi-json/scanner.
//
// The API is deliberately not compatible with encoding/json. Decoding arbitrary
// targets without reflection needs a compile-time field mapping, so struct
// binding uses accessor closures (Field[T]) rather than reflect or unsafe. See
// .opencode/plan/2026-11-21-json-decoder.md for the design and task list.
//
// A Reader decodes typed values from a document, tracking the JSON path so that
// errors point at the offending member. A Schema[T] binds JSON object members to
// struct fields: build one with Struct and the field constructors (String, Int,
// Float, Bool, Object, Slice, Map, Array, Any, Raw and their Ptr variants), then
// decode with Schema.Unmarshal or Schema.Decode.
//
//	var configSchema = marshal.Struct(
//	    marshal.String[Config]("name", func(c *Config) *string { return &c.Name }),
//	    marshal.Int[Config]("port", func(c *Config) *int64 { return &c.Port }),
//	)
//
//	cfg, err := configSchema.Unmarshal(data)
//
// A JSON null leaves a value field unchanged (zero for a fresh value) and sets a
// pointer field to nil. A member that no field matches is ignored unless
// RejectUnknown is set; a member that repeats is overwritten unless
// RejectDuplicate is set.
//
// A type that implements Decoder can be decoded without naming its Schema. The
// usual implementation delegates to a Schema:
//
//	func (c *Config) DecodeJSON(r *marshal.Reader) error {
//	    return configSchema.Decode(r, c)
//	}
//
//	cfg, err := marshal.Unmarshal[Config](data)        // fresh value
//	err = marshal.UnmarshalInto[Config](data, &cfg)    // merge into an existing value
//	err = marshal.Decode(data, &cfg)                   // merge, with no type argument
package marshal
