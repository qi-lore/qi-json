# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- `go.mod`: the module's `go` directive is now `go 1.23.0` (with a
  `toolchain go1.27.3` line) instead of `go 1.27.3`. Since Go 1.21 a
  dependency's `go` directive is a hard minimum, so the enforced floor was 1.27.3
  while the documentation promised Go >= 1.23: a 1.23 consumer was silently
  upgraded by `go mod tidy` or hard-blocked (`requires go >= 1.27.3`). The source
  is 1.23-language-compatible, so the floor now matches the documentation; the
  `toolchain` line names the toolchain used to develop the project and is ignored
  by consumers.

## [0.2.0] - 2026-11-22

### Added

- `qijson`: delegation constructors for a nested `Decoder`/`Encoder` —
  `ObjectDecoder` and `ObjectDecoderPtr` for a field, and `DecoderCodec` for a
  slice, map, or array element. They bind a field or element whose type
  implements `DecodeJSON`/`EncodeJSON` by calling those methods, the counterpart
  to `Object` for a type whose Schema is in another package. (Constitution
  Article VI amendment, v3.4.0.)
- `cmd/qi-json-gen`: struct fields whose type is a struct from **another
  package** are now supported, closing the release blocker:
  - A qi-json-aware foreign type (one implementing `DecodeJSON` and
    `EncodeJSON`) is bound by delegation (`ObjectDecoder`/`ObjectDecoderPtr`, or
    `DecoderCodec` as a container element).
  - A plain foreign struct is bound by a schema the generator synthesizes from
    its exported fields (a `<pkg><Type>Schema` var in the consumer package). The
    generator recurses into nested foreign structs and emits a type reachable by
    several paths once. Unexported foreign fields are skipped (the generated
    file notes this); an embedded foreign field is a precise error.
  - A `-foreign=pkg.T=ident` flag lets the consumer point a foreign type at a
    schema it owns, for a type the generator cannot introspect; a malformed or
    unused entry is a precise error.
  - A pointer to a plain foreign struct, `[]T`/`map[string]T` of a foreign
    struct, and `omitempty` on a foreign field are all supported.

## [0.1.1] - 2026-11-22

### Added

- `qijson`: the decoder rejects invalid UTF-8 in a string value or object
  member name as a syntax error. The scanner stays syntax-only, so it remains
  more permissive than the decoder on this one point.
- `cmd/qi-json-gen`: cross-package named types whose underlying type is a basic
  scalar (`time.Duration`, or any named `string`/`bool`/integer/float from
  another package) are now resolved with the standard library's type checker, as
  fields, pointer fields, and slice or map elements.
- `cmd/qi-json-gen`: same-package named types whose underlying type is `string`
  or `bool` (for example `type Status string`) are now generated, as a field,
  pointer field, and slice or map element, matching the cross-package path.
- `cmd/qi-json-gen`: a `-strict` flag that makes every generated schema reject
  unknown and duplicate object members.
- `qijson`: a reflection-free **encoder**, the mirror of the decoder:
  - `Writer`, an append-only JSON buffer with a sticky error, `Write*` methods,
    and `Begin*`/`End*` object and array framing.
  - The `Encoder` interface (`EncodeJSON(*Writer) error`) and two entry points:
    generic `Marshal[T](v)` and non-generic `Encode(w, v)`.
  - `Schema.Encode`/`Schema.Marshal` and `Field.Encode`, writing object members
    in declaration order and honouring `json:",omitempty"`.
  - The element `Codec[E]` (`NewCodec`), so `Slice`, `Map`, and `Array` carry
    both directions; map keys are written sorted, a nil slice or map as `null`,
    and invalid UTF-8 in a string as `U+FFFD`.
- `cmd/qi-json-gen`: generated types now get an `EncodeJSON` method alongside
  `DecodeJSON`, and `json:",omitempty"` is honoured on encode.

### Changed

- **Breaking:** the semantic layer moved from the `marshal` package
  (`qi-json/marshal`) to the module root, and was renamed to `qijson`
  (`qi-json`). Callers now `import "qi-json"` and use `qijson.Marshal`,
  `qijson.Unmarshal`, `qijson.Schema`, and so on; the diagnostic prefix changed
  from `marshal:` to `qijson:`. `scanner` (`qi-json/scanner`) is unchanged. The
  module is pre-1.0 and deliberately not `encoding/json`-compatible.
- `qijson` container field constructors (`Slice`, `Map`, `Array`) now take a
  `Codec[E]` instead of a bare element decode function, so one value carries both
  the decoder and the encoder. This is a breaking change to those three
  constructors (the module is pre-1.0 and deliberately not `encoding/json`-compatible).

## [0.1.0] - 2026-11-21

The first release with the decoder and the optional code generator.

### Added

- **`scanner`** — the goto-free, single-pass syntax layer:
  - `Valid`, the `encoding/json.Valid`-shaped validator. Allocation-free for
    documents nesting up to 32 levels, with exact error messages and 1-based
    positions.
  - A pull tokenizer (`Kind`, `Tokenizer`, `NewTokenizer` with `Peek`, `Read*`,
    `Begin*`/`End*`, `Skip`, `Offset`, `Err`) sharing the validator's scalar
    scanners and error contract.
- **`marshal`** — the reflection-free decoder:
  - `Reader` with typed reads (`ReadNull`, `ReadBool`, `ReadString`, `ReadInt`,
    `ReadFloat`), `Object`/`Array` iterators, `Skip`, and a lazily rendered JSON
    path.
  - `Schema[T]` / `Struct` and the field constructors `String`, `Int`, `Float`,
    `Bool` (and their `Ptr` variants), `Object`, `Slice`, `Map`, `Array`, `Any`,
    and `Raw`.
  - The `Decoder` interface (`DecodeJSON(*Reader) error`) and three entry
    points: generic `Unmarshal[T]` (fresh value), generic `UnmarshalInto[T]`
    (merge), and non-generic `Decode` (merge, no type argument).
  - Strict opt-ins `RejectUnknown` and `RejectDuplicate`; by default unknown
    members are ignored and the last duplicate wins.
  - `*Error` (JSON path, byte offset, kind, wrapped cause) and `*TypeError`,
    inspectable with `errors.As`/`errors.Unwrap`.
- **`cmd/qi-json-gen`** — an optional, stdlib-only code generator that emits
  `marshal` schemas and `DecodeJSON` methods from `json` struct tags. Driven by
  `-type=Name` or the `//qi-json:generate` marker; deterministic output with a
  drift-guard test.
- **Documentation** — a root `README.md` (quick start, generic and non-generic
  entry points, decoding rules, error inspection, codegen, performance) and
  package docs for all three packages.

### Changed

- The decoder API is deliberately **not** compatible with `encoding/json`; it
  binds struct fields with compile-time accessor closures instead of reflection.

## [0.0.1] - 2026-11-21

### Added

- Initial `scanner` package with `Valid`.
