# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Nothing yet.

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
