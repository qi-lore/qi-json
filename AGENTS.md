# AGENTS.md — qi-json

A from-scratch JSON library with three Go packages:

- `scanner/` (import `qi-json/scanner`) — the single-pass syntax layer; exports
  `Valid` and the tokenizer (`Kind` plus a tokenizing type).
- `.` (import `qi-json`, package `qijson`) — the reflection-free semantic layer
  (the decoder and encoder). Not `encoding/json`-compatible; no `reflect`, no
  `unsafe`.
- `cmd/qi-json-gen` — the optional, stdlib-only code generator that emits
  `qijson` `Schema` values and `DecodeJSON`/`EncodeJSON` methods from struct
  tags. No `reflect`/`unsafe`; it targets only the exported `qijson` surface.

The module root is the `qijson` package. No third-party dependencies. `make` wraps the
common tasks. `docs/task.md` is the syntax specification;
`.opencode/plan/2026-11-21-json-decoder.md` is the decoder plan and task list and
`.opencode/plan/2026-11-21-json-encoder.md` is the encoder plan; `docs/constitution.md`
governs changes.

## Required verification (in this order)

Every task MUST finish by running these commands in this order. Stop at the
first failure, fix it, then restart the sequence from step 1:

1. `go test ./...`
2. `gofmt -l .` — must print nothing; fix with `go fmt ./...`
3. `go vet ./...`
4. `golangci-lint run` — v2, default linters, no repo config

Shortcut: `make verify` runs steps 1–4 in order and stops at the first failure.

## Other commands

- `go test -race ./...`
- `go test ./scanner -run '^TestInvalidDocuments$'` — run one test/table
- `go test -run=^$ -fuzz=FuzzValid -fuzztime=10s ./scanner` — fuzz (`-run=^$` is required)
- `go test -run=^$ -bench=. -benchmem ./scanner` — benchmarks
- `go generate ./...` — regenerate `cmd/qi-json-gen` fixtures — no other package
  emits generated code

## API and naming

- `scanner.Valid(data []byte) bool` is the scanner's headline export — the same
  name and signature as `encoding/json.Valid`. The sanctioned public surface is
  exactly the scanner tokenizer and the `qijson` decoder and encoder
  (constitution Article VI); do not export anything else without a new
  amendment.
- `scanner.validate(data []byte) (parseError, bool)` is unexported and returns
  the first failure plus whether the input is invalid. Tests assert messages and
  1-based positions through it; `Valid` is `!failed`. The error is a value type so
  that returning it does not drag the validator's inline stack to the heap.
- Internal names are repo-specific, not copies of encoding/json internals:
  `validator`, `container`/`containerObject`/`containerArray`, `parseError`,
  `skipSpace`, `scanString`, `scanEscape`, `scanLiteral`, `scanNumber`,
  `fail`/`failEnd`/`failAt`, and the tables `spaceTable`/`stringStop`. Do not add
  `Scanner`, `SyntaxError`, `scanBegin*`, `stateBegin*`, `opcode`, `machine`,
  `step` or `finish`.
- The scanner uses no `goto` and no statement labels. `run` is two nested loops:
  an outer loop that scans one value, and an inner loop that consumes the
  delimiter after a completed value (see `docs/task.md` §6.1). The object-member
  scan is deliberately written twice so it stays inline; extracting it into a
  helper costs about 8% on object-heavy documents.
- `qijson` is the semantic layer: a reflection-free decoder and encoder over
  `scanner`. It MUST NOT import `encoding/json` and MUST NOT use `reflect` or
  `unsafe`; struct binding is by compile-time accessor closures. Its API is
  deliberately not `encoding/json`-compatible. The non-generic `Decode(data, v
  Decoder) error` mirrors `UnmarshalInto` for a caller that does not want to name
  a type argument; the encoder mirrors it with `Marshal[T, PT](v T) ([]byte,
  error)` and the non-generic `Encode(w *Writer, v Encoder) error`, built on a
  `Writer` and an element `Codec[E]`. A container field constructor
  (`Slice`/`Map`/`Array`) takes a `Codec[E]` carrying both directions; the
  encoder honours `json:",omitempty"` and replaces invalid UTF-8 with `U+FFFD`
  (the decoder rejects invalid UTF-8). Plans and tasks:
  `.opencode/plan/2026-11-21-json-decoder.md` and
  `.opencode/plan/2026-11-21-json-encoder.md`.
- `cmd/qi-json-gen` is the optional code generator (stdlib only: `go/ast`,
  `go/parser`, `go/format`, and `go/types` via the source importer for
  cross-package named types; same-package named scalar types are resolved from
  the AST). It emits code against the exported `qijson`
  surface only; the runtime is unchanged by it. It emits a `Schema` var plus both
  `DecodeJSON` and `EncodeJSON` for each target type. It MUST NOT import
  `encoding/json` and MUST NOT use `reflect` or `unsafe`.
  Generation must be deterministic (sort all emitted collections) so the drift
  guard passes. Plan: `.opencode/plan/2026-11-21-json-codegen.md`.
- Never use `encoding/json`, including in tests or benchmarks (hard requirement
  in `docs/task.md`); tests are hand-built tables.

## Contract details that are easy to break

- `scanNumber` and `scanLiteral` return the index of the first byte that cannot
  extend the scalar. They never consume the delimiter; the inner delimiter loop
  judges it. That is what makes `1.2.3` fail with `after top-level value` at
  offset 4.
- A `0` integer part followed by a digit is an error inside the number scanner
  (`in numeric literal`), not `after top-level value`. There is no
  `stateNumberZero`-style handler any more; `scanNumber` checks it inline.
- The inner loop with `depth == 0` means the top-level value is complete; only
  whitespace may follow, else `after top-level value`.
- End of input reports `unexpected end of JSON input` at `len(data)`, except
  inside a literal, an escape, a `\u` escape or a number, which keep their own
  message at `len(data)` (`scanLiteral`, `scanEscape`, `failAt`).
- Error messages and 1-based positions are asserted exactly; update tests
  deliberately when behavior changes, never just to make the suite pass.
- `TestValidDoesNotAllocate` guards the hot path: the nesting stack is a
  `[nestingInline]container` array on the caller's stack plus a depth counter,
  never `append`ed on the hot path, so `Valid` allocates nothing for documents
  nesting at most `nestingInline` levels. Do not reintroduce a `sync.Pool`, a
  per-byte handler dispatch, or `goto`/labels in the hot path.
- The validator is syntax-only by design: no UTF-8 validation, no surrogate
  pairing checks, no nesting depth limit.
- TDD is mandatory per the constitution: failing test first, minimal
  implementation, refactor.

## Context

- Governing rules: `docs/constitution.md`.
- Specs and plans:
  - `docs/task.md` — scanner specification.
  - `.opencode/plan/2026-11-21-json-decoder.md` — decoder decisions, tasks and
    handoff (resume here for decoder work).
  - `.opencode/plan/2026-11-21-json-codegen.md` — the `cmd/qi-json-gen` codegen
    design, tasks and handoff (resume here for codegen work).
  - `.opencode/plan/2026-11-21-post-decoder-backlog.md` — the running backlog of
    follow-up tasks (resume here for post-decoder work).
  - `.opencode/plan/user-scenarios.md` — end-to-end user walkthroughs and the
    gaps they expose (the foreign-struct-field gap is a release blocker).
  - `.opencode/plan/2026-11-22-third-party-struct-fields.md` — **next task**:
    research a fix for struct fields whose type is a struct from another package.
  - `.opencode/plan/2026-11-21-package-rename.md` — relocated the semantic layer
    to the module root and renamed it `qijson` (done 2026-11-21).
  - `.opencode/plan/2026-11-21-scanner-rewrite.md` — scanner rewrite rationale
    and verification log.
  - `.opencode/plan/2026-11-21-json-unmarshal-research.md` — decoder research
    (see the decoder plan for what is superseded).
  - `.opencode/plan/2026-11-21-json-scanner.md` — superseded state machine.
