# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `qijson`: the decoder enforces a fixed maximum container nesting depth of
  10000 levels. A document that nests deeper fails with a `*Error` of `Kind`
  `KindDepth`, whose `Offset` points at the delimiter that would have crossed
  the limit, so recursion over hostile input cannot grow the path stack without
  bound. The syntax-only surface (`Valid` and the tokenizer) is unchanged and
  stays unlimited. (Task Q012.)
- Developer tooling: `make fuzz` now fuzzes the codec entry points
  (`FuzzUnmarshal`, `FuzzUnmarshalMatchesValid`, `FuzzMarshalRoundTrip`) in
  addition to the scanner's `FuzzValid`, driving the whole semantic layer over
  arbitrary input. (Task Q013.)

### Changed

- Documentation: every exported type alias in the facade now ends its type-level
  doc comment with an accurate "Its methods are ..." sentence, so the method
  names are visible through `go doc` even though it does not follow aliases for
  method sets. Docs only; no API change. (Task Q014.)

## [0.5.5] - 2026-11-28

### Changed

- `cmd/qi-json-gen`: generated output no longer carries the `Empty:` closure on
  a field that is not `omitempty`. The runtime reads a field's `Empty` closure
  only when the field is `omitempty` (see `Schema.Encode`), so it was dead code
  on any other field; it is now emitted only for `omitempty` fields, and the
  output for `omitempty` fields is byte-identical. Regenerating changes only
  non-`omitempty` fields. (Task Q011.)
- Tests: statement coverage is raised across the whole module — the reflection
  facade wrappers, the decoder and encoder error and constructor paths, the
  scanner tokenizer typed-read mismatches, the generator's type-resolution
  branches, its `gentest` fixtures and the example. The generator CLI and the
  example `main()` bodies are reduced to thin wrappers over a testable `run(...)`;
  behavior is unchanged. No public API change. (Task Q011.)

## [0.5.4] - 2026-11-27

### Changed

- The `example/` demo now binds its struct through the `qi-json-gen` code
  generator instead of hand-written accessor closures, and its type is renamed
  from `Server` to `User` with user-shaped fields
  (`name`/`age`/`verified`/`rating`). (Task M017.)

## [0.5.3] - 2026-11-27

### Added

- A runnable usage example under `example/`: a `package main` that validates a
  document with `Valid`, decodes it into a struct with `Unmarshal`, and
  re-encodes it with `Marshal`. The README describes it briefly. (Task M016.)

## [0.5.2] - 2026-11-27

### Added

- `qijson`: `Writer` gains `DumpTo(io.Writer) (int, error)` and
  `ReadCloser() (io.ReadCloser, error)` — the encode-streaming surface: write the
  built document directly to an `io.Writer` without materialising a second copy,
  or hand it out as an `io.ReadCloser`. Both reset the `Writer` on success so
  values can be built and streamed in a loop; a `Writer` holding a sticky error
  reports that error and streams nothing. The surface is sanctioned by the
  constitution's Article VI amendment. (Task N001.)
- `qijson`: `NewDecoder(io.Reader) *Tokenizer`, a reader-backed tokenizer that
  streams JSON from an `io.Reader`, pulling it in 4096-byte chunks into the
  tokenizer's buffer. The returned slices alias that growing buffer and are valid
  only until the next read, so callers must copy to retain them; a value that
  straddles a read boundary is refilled and rescanned, and a non-EOF read error
  surfaces wrapped as the tokenizer's sticky error. The slice-backed
  `NewTokenizer([]byte)` path is unchanged. (Task N002.)
- `qijson`: the reader-backed tokenizer reads successive top-level values from
  one stream (NDJSON) and reports `KindEnd` at the end of the input, instead of
  rejecting a trailing value with `after top-level value`. Only reader-backed
  tokenizers gain multi-value semantics; `NewTokenizer([]byte)` still holds a
  single document and rejects any trailing byte. (Task N003.)
- `qijson`: `Tokenizer.Token() (Token, error)` plus the `Token`, `Number` and
  `Delim` types, the single-call alternative to the pull-based `Peek`/`Read*`
  surface. It returns the next token (`nil`, `bool`, `Number`, `string` or
  `Delim`) and `io.EOF` once the stream is exhausted; numbers stay raw literals
  (`Number`, never a `float64`), strings keep their escape sequences, and the
  separators between members and elements are consumed internally. A returned
  token is a copy and safe to retain. (Task N004.)
- `qijson`: the formatting and escaping surface — `Indent`, `Compact`,
  `HTMLEscape`, `MarshalIndent`, and the sticky `Writer.SetIndent` and
  `SetEscapeHTML`. `Indent`/`Compact`/`HTMLEscape` mirror the `encoding/json`
  signatures (`*bytes.Buffer` destination plus `src []byte`); HTML escaping stays
  off by default, a deliberate divergence from `encoding/json`, and is enabled
  explicitly. (Task N005.)
- `qijson`: the omit hooks `Optional` (`IsDefined() bool`) and `IsZero`
  (`IsZero() bool`), so a field type can decide for itself whether it carries a
  value under `omitempty`. The generator detects them through `go/types`
  method-set inspection and wraps the emitted `Field`'s `Empty` closure; `IsZero`
  takes precedence over `Optional` and a pointer field is nil-guarded. Detection
  is by method name, so any type with an `IsZero() bool` method participates.
  (Task H001.)
- `qijson`: the `UnmarshalUnknown`/`MarshalUnknowns` unknown-fields proxy, a
  reflection-free, opt-in round-trip for object members that bind to no field. A
  decode target implementing `UnmarshalUnknown` receives each unmatched member
  (decoded name plus a copied raw JSON value it owns); an encode target
  implementing `MarshalUnknowns` writes those members back after the declared
  fields. `RejectUnknown` takes precedence over `UnmarshalUnknown`. (Task H002.)
- `qijson`: `Writer.BuildBytes(reuse ...[]byte) []byte`, the writer-buffer reuse
  surface. Unlike `Bytes` it never hands out the internal buffer: the document is
  appended to the optional caller-supplied buffer (reusing its capacity) or to a
  fresh one, the result is owned by the caller, and the `Writer` stays valid and
  reusable across a `Reset`; a failed `Writer` returns `nil`. `Marshal` and
  `Schema.Marshal` now route through it. (Task E004.)
- Tests: a `bench/` comparative benchmark harness, a separate, non-shipped Go
  module that benchmarks qi-json against `encoding/json` and `mailru/easyjson`
  on identical documents (marshal and unmarshal of 1 and 100 records), so the
  three-way numbers are reproducible. (Task Q010.)

### Changed

- Performance: the encoder pools reusable `Writer` buffers (a `sync.Pool`-backed
  buffer, 1 KiB fresh sizing and a 64 KiB retention cap) and routes `Marshal` and
  `Schema.Marshal` through the pool, returning a freshly allocated, caller-owned
  copy so the result never aliases the shared buffer. No exported name changed.
  (Tasks E002, E003.)
- Performance: the decoder builds one library-owned copy of the document and
  returns clean strings and number literals as zero-copy substrings of it
  (escape sequences keep the existing unescape path), and preallocates slice and
  map capacity from a look-ahead count of the container's own separators. Decoded
  values still never alias the caller's input buffer and the exported surface is
  unchanged. (Tasks D011, D012.)
- Governance: feature and task branches are now created from `dev` instead of
  `main`, because the newest integrated work lives on `dev` for a long time
  before a release promotes it to `main`, so a branch cut from `main` would start
  from stale code. Recorded in the constitution (v3.12.0), `AGENTS.md` and the
  README. No code or public API change.
- Governance: Article VI is amended (v3.13.0) with a fourth sanctioned surface
  covering the network/streaming, formatting and hooks identifiers above, which
  had to be registered before their implementation tasks could land. (Task M009.)
- Governance: the `encoding/json` parity track (Track B: `any`-based entry
  points, cached tag binding, `Marshaler`/`Unmarshaler`, stdlib-shaped error
  types) is declined; the no-reflection identity is preserved and Article III/VI
  are unchanged, since a decline is not an amendment. (Task M010.)
- Governance: Article VI is amended (v3.14.0) to register
  `Writer.BuildBytes(reuse ...[]byte)` on the encoder surface before E004 landed.
  (Task M012.)
- Governance: Article III is amended (v3.15.0) to carve out the single in-repo,
  non-shipped `bench/` comparative benchmark module; the `encoding/json` and
  third-party-dependency prohibitions now scope to the shipped module. (Task
  M013.)
- Governance: the constitution gains a **Documentation Ownership** section
  (v3.16.0) — `README.md` may be changed only on the user's direct instruction,
  and a request to read, check or review it is not authorization to change it —
  mirrored in `AGENTS.md`. No code or public API change. (Task M014.)
- Documentation: `README.md` is rewritten to a concise form (531 → 119 lines),
  correcting the `qijson.Error` reference and normalizing the benchmark table to
  plain spaces. No code change. (Task M015.)

## [0.4.2] - 2026-11-23

### Added

- Documentation: two code-level assessment records. `docs/fn-comparison.md`
  documents the semantic differences between the generic and non-generic entry
  points (`Unmarshal`/`UnmarshalInto` versus `Decode`, `Marshal` versus `Encode`)
  from the code rather than from the doc comments; `docs/panics-issues.md` audits
  the engine for panics, defeatable guards and silent misbehavior and classifies
  each finding as a defect, a deliberate trade-off, or correct. No code change.

### Changed

- Repository settings: `main` and `dev` are protected by branch rulesets. `main`
  requires a pull request (zero approvals) and the `verify` status check; `dev`
  requires the `verify` status check. Both block deletions and force pushes. The
  `main` ruleset keeps an organization-admin bypass as an emergency escape hatch.
  No code or public API change.

### Fixed

- `qijson`: `Schema.UnmarshalInto` and `Schema.Decode` return the `errNilTarget`
  sentinel for a nil `*T` target instead of panicking with a nil-pointer
  dereference, matching the generic `UnmarshalInto` and the non-generic
  `Decode`/`Encode`, which already reported the same misuse as a recoverable
  error. The panic was a defect: a public entry point crashed where every sibling
  returned an inspectable sentinel. (Code-quality task Q006.)
- `qijson`: `Writer.WriteRaw` rejects an empty slice with the `errEmptyRaw`
  sentinel, and `Marshal`'s completeness guard now requires the root value to have
  produced at least one byte. An `EncodeJSON` that called `WriteRaw(nil)` used to
  satisfy the guard without emitting a value, so `Marshal` returned `(nil, nil)`
  instead of an error; the same empty write inside a container produced the
  invalid text `[1,]` or `{"a":}`. (Code-quality task Q007.)
- `qijson`: `Object` returns the `errNilSchema` sentinel for a nil nested
  `*Schema[S]` instead of dereferencing the nil receiver. A JSON `null` member
  still decodes without needing the schema, matching how a null container decodes
  without its element codec. (Code-quality task Q008.)
- `qijson`: `Struct` now enforces its documented unique-field-name requirement.
  Two fields sharing a name used to bind a member to whichever duplicate the
  binary search reached and made `Encode` write the key twice (`{"x":1,"x":2}`).
  `Struct` records the `errDuplicateFieldName` sentinel (wrapped with the
  offending name) and `Decode`/`Encode` return it before doing any work, so
  `Marshal`, `Unmarshal` and `UnmarshalInto` surface it too. The constructor keeps
  its `*Schema[T]` signature, so generated code and fixtures are unchanged.
  (Code-quality task Q009.)

## [0.4.1] - 2026-11-22

### Added

- Documentation for installing the optional `cmd/qi-json-gen` generator: the
  README gains an **Installing the generator** section covering `go run`, the
  Go 1.24 `tool` directive and `go install`, and the stale
  `qi-json/cmd/qi-json-gen` module path in the codegen examples is corrected. No
  module, runtime or generated-output change; `go.mod` gains no dependency.

### Changed

- Code style: an Uber Go Style Guide review pass over the library and the
  generator. The generator now writes a foreign-struct pointer with `&T{}`
  instead of `new(T)`, so regenerated `qi-json-gen` output differs on that one
  line; a `Writer` frame that set `object: false` now relies on the zero value;
  and the generator pre-sizes a few slices and maps and flattens an `else if`
  that followed a returning branch. Compile-time assertions that `*Error`,
  `*TypeError` and the scanner's `parseError` implement `error` were added. The
  runtime behavior, error messages and public API are unchanged. The guide's
  rule that unexported top-level vars and consts be prefixed with `_` is a
  recorded, deliberate deviation: the descriptive names are kept.
- Documentation: the `.opencode/plan/` design records no longer carry task state.
  Each plan file's header drops the `**Status**` line (and the scanner plan's
  `## Status` section), which stated which tasks were done or next and pointed at
  the following task. Design content that had lived in those lines (the encoder's
  locked decisions, the Go-floor supersession note) is kept under neutral labels,
  and every header now carries the line "This file is a design record. Task state
  lives in `docs/tasks.md`." No code or public API change.

## [0.4.0] - 2026-11-22

### Changed

- **Breaking.** The syntax layer moved behind the facade. The
  `github.com/qi-lore/qi-json/scanner` subpackage is gone; the syntax engine now
  lives in `internal/scanner` and `qijson` re-exports its whole surface —
  `Valid`, the `Kind` type with its seven constants, and the tokenizing
  `Tokenizer` with `NewTokenizer` — so there is a single public import path,
  `github.com/qi-lore/qi-json`. Names, signatures and behavior are unchanged
  (`qijson.Kind` is the type `Reader.Peek` returns), so migration is a qualifier
  and import-path rewrite (`scanner.X` → `qijson.X`). `qi-json-gen` output must
  be regenerated: it no longer imports `scanner` and compares against
  `qijson.KindNull`.

## [0.3.1] - 2026-11-22

### Changed

- Documentation: `docs/task.md` is retired and `docs/tasks.md` becomes the single
  source of truth for task state. The scanner specification's surviving content
  moved into the constitution (the extra verification commands and the
  performance invariant) and into the tests that already pin the contract; the
  constitution gains a **Task Tracking** section (v3.9.0) that records tasks only
  in `docs/tasks.md`, forbids task state in `.opencode/plan/` files and
  `AGENTS.md`, and requires `docs/tasks.md` to be updated before every commit.
  No code or public API change.
- Error messages and exported doc comments no longer cite project documents. The
  `go.mod` version guards, the `qijson` package doc, the `scanner` package doc and
  the error-wrapping test now describe the behavior or the invariant instead of
  pointing at `docs/constitution.md`, the README or the plan files. The
  constitution gains the matching rule (v3.10.0): a consumer of the module does
  not have the repository, and a failure should say what is wrong, not where to
  read about it. No behavior change.

## [0.3.0] - 2026-11-22

### Changed

- **Breaking.** The canonical module path is now `github.com/qi-lore/qi-json`
  (was `qi-json`). Every import path, the generator's emitted imports, the
  checked-in fixtures and the docs moved with it; the package name stays
  `qijson`. Importers must rewrite every import, including the `scanner`
  subpackage, and regenerate any `qi-json-gen` output because the emitted
  import path changed too.
- **Breaking.** The module now requires **Go 1.24** (`go 1.24.0`): the facade
  re-exports the generic `Schema`, `Field` and `Codec` through generic type
  aliases, which are a Go 1.24 language feature.
- `qijson`: the semantic engine moved to `internal/codec` and the module root is
  now a thin facade of type aliases, constant re-exports and one-line wrappers.
  The public API is unchanged — `qijson.Reader` *is* `codec.Reader` — so
  `errors.As`, generated `Field` literals and every call site keep working, while
  the engine's internals are no longer importable. `unmarshal.go` became
  `internal/codec/decode.go`, and the white-box tests moved next to the engine;
  `api_test.go` pins the facade's surface from outside the package.
- `qi-json-gen`: the awareness check now accepts the facade's `Reader`/`Writer`
  aliases as well as the engine types they resolve to, so generated output is
  byte-identical apart from the module path.

## [0.2.3] - 2026-11-22

### Changed

- `qijson`: every library sentinel is now defined once and inspectable with
  `errors.Is` (code-quality task T-Q2). The element-constructor guards
  (`errNilElementDecoder`, `errNilElementEncoder`) plus `errUnsupportedDynamicType`
  (`field.go`) and `errNoFieldEncoder` (`schema.go`) replace the `errors.New`
  calls that were made inside function bodies and are wrapped with `%w`. No
  exported identifier changed; the only visible effect is two programming-error
  messages that now read `qijson: cannot encode <T> as JSON: unsupported dynamic
  type` and `qijson: field "x": no encoder`.
- Documented the error-wrapping policy (code-quality task T-Q1) in `doc.go` and
  the constitution (v3.7.0), mirrored in `AGENTS.md`: a decode failure is a typed
  `*qijson.Error` carrying the JSON path and offset, returned unwrapped, so
  `errors.As` reaches it and `errors.Is` reaches its cause; a programming error
  is an inspectable sentinel. New `error_policy_test.go` pins the guarantee end to
  end across every entry point and through a nested `ObjectDecoder` delegation.
  No behavior change.
- Documented ownership, concurrency and lifecycle on the public API (code-quality
  task T-Q3): `Schema` is immutable once built (the reject setters mutate it and
  must not race with use), `Field`/`Codec`/`NewCodec` are plain closures,
  `*Error`/`*TypeError` are immutable, `Writer` is not safe for concurrent use
  and `Bytes` aliases its buffer, the entry points do not retain `data` and the
  `[]byte` from `Marshal` is caller-owned, and `scanner.NewTokenizer` documents
  that it aliases `data`. Comments only; no behavior change.

## [0.2.2] - 2026-11-22

### Added

- Governance: `docs/constitution.md` gains a **Release flow** section (v3.6.0),
  mirrored in `AGENTS.md`: how a release is cut — choose the version, prepare it
  on `dev` (changelog promotion and README version block), open the `dev` →
  `main` pull request, merge, tag the merge commit on `main`, and keep `dev`.

## [0.2.1] - 2026-11-22

### Added

- Governance: `docs/constitution.md` gains two sections (v3.5.0) — **Git and
  Review Discipline** (two-branch model, Conventional Branch names,
  Conventional Commits, PR conventions, CI enforcement) and **Rigorous Code
  Quality** (Uber Go Style Guide, error wrapping, sentinel placement, public-API
  ownership/concurrency/lifecycle, goroutine and context rules, no panics in
  library code). The same rules are mirrored in `AGENTS.md` and `README.md`.
- CI: `.github/workflows/ci.yml` runs the verification sequence (test → gofmt →
  vet → golangci-lint) and checks Conventional Branch names, the target branch
  (`main` only from `dev`) and Conventional Commits with a non-empty body.
- `.opencode/plan/2026-11-22-git-and-code-quality-reglament.md` records the
  audit of the code against the new rules and the follow-up refactoring tasks
  T-Q1…T-Q4.

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
