# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Distribution for the optional `cmd/qi-json-gen` generator. `.goreleaser.yaml`
  and a tag-triggered `.github/workflows/release.yml` build prebuilt archives
  (linux, macOS and Windows; amd64 and arm64) with a `checksums.txt`, and publish
  a Homebrew cask to a separate `qi-lore/homebrew-tap` repository, so the command
  can be installed
  without the repository. The README gains an **Installing the generator**
  section covering `go run`, the Go 1.24 `tool` directive, `go install` and
  `brew`, and the stale `qi-json/cmd/qi-json-gen` module path in the codegen
  examples is corrected. No module, runtime or generated-output change;
  `go.mod` gains no dependency. The release flow in `docs/constitution.md` and
  `AGENTS.md` is amended (v3.12.0).

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
