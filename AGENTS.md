# AGENTS.md — qi-json

A from-scratch JSON library with four Go packages:

- `scanner/` (import `qi-json/scanner`) — the single-pass syntax layer; exports
  `Valid` and the tokenizer (`Kind` plus a tokenizing type).
- `.` (import `qi-json`, package `qijson`) — the public facade of the
  reflection-free semantic layer (the decoder and encoder): type aliases, const
  re-exports and one-line wrapper functions. Not `encoding/json`-compatible; no
  `reflect`, no `unsafe`.
- `internal/codec/` — the semantic engine behind that facade. Internal, so
  external code cannot import it and its internals may change without a
  compatibility promise.
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

## Git and review discipline

- Branching: `main` is production, `dev` is integration. Create feature/task
  branches from the current `main` (after `git fetch` and a fast-forward
  update) and target `dev`; only the `dev` → `main` pull request may merge into
  `main`. Branch names follow Conventional Branch
  (https://conventionalbranch.org/).
- Commits: strict Conventional Commits with a full body (what changed, why, and
  any non-obvious decision).
- Pull requests: start the description with `Closes: #<n>` when it closes an
  issue, state the problem and the solution, and confirm suggested labels with
  the user before applying.
- CI (`.github/workflows/ci.yml`) enforces the branch name, the target branch,
  the commit messages, and the verification sequence.

## Release flow

- `dev` integrates work; a release promotes it to `main`.
- Choose the version (SemVer; pre-1.0: patch for a fix or documentation change,
  minor for a new or changed capability).
- Prepare the release on `dev` (never on a feature branch): promote the
  `CHANGELOG.md` `[Unreleased]` entry to `[X.Y.Z] - YYYY-MM-DD` and reopen an
  empty `[Unreleased]`; update the `README.md` version block; do the housekeeping
  (mark done items, point the next task, refresh baseline references); commit as
  `chore(release): vX.Y.Z` with a full body; run `make verify`.
- Open the `dev` → `main` pull request (the only PR allowed into `main`) and let
  CI pass.
- Merge into `main`, then tag the merge commit: `git tag vX.Y.Z <merge-sha>` and
  push the tag.
- Keep `dev` — do not delete the integration branch.

## Code quality

All Go code follows the Uber Go Style Guide
(https://github.com/uber-go/guide/blob/master/style.md) without exception, plus:

- Wrap errors with context: `fmt.Errorf("...: %w", err)`; never return a bare
  error from a deeper layer. The typed `*qijson.Error` is itself the context
  (path + offset) and is returned unwrapped; use `%w` only where a layer adds new
  context (for example a sentinel cause).
- Define a sentinel in the package that owns the behavior (`internal/codec` for
  the decoder and encoder, `scanner` for the syntax layer) and expose it through
  the `qijson` facade only when a caller must inspect it.
- Public APIs document ownership, concurrency safety, and lifecycle.
- Goroutines need clear ownership and shutdown paths; no leaks.
- Propagate context explicitly; blocking operations accept `context.Context`.
- No panics in library code for recoverable conditions.

The library is synchronous (no goroutines, no blocking calls), so the goroutine
and context rules bind only when such code is added. Open gaps are tracked in
`.opencode/plan/2026-11-22-git-and-code-quality-reglament.md`.

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
  (the decoder rejects invalid UTF-8). `ObjectDecoder`/`ObjectDecoderPtr` and
  `DecoderCodec` bind a field or element whose type implements `DecodeJSON` and
  `EncodeJSON` by calling them (the delegation path the generator emits for a
  qi-json-aware foreign type; Article VI, added 2026-11-22). Plans and tasks:
  `.opencode/plan/2026-11-21-json-decoder.md` and
  `.opencode/plan/2026-11-21-json-encoder.md`.
- The `qijson` root is a facade: `internal/codec` holds the implementation and
  `qijson.go` re-exports it through type aliases, constants and wrapper
  functions. Keep the public contract (including ownership, concurrency and
  lifecycle docs) on the facade; the engine's internals stay unexported. The
  aliases are why the Go floor is 1.24. See
  `.opencode/plan/2026-11-22-internal-codec-extraction.md`.
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
    follow-up tasks (resume here for post-decoder work; its last open item, the
    Go-version floor mismatch, is **done** per
    `.opencode/plan/2026-11-22-go-version-floor.md`).
  - `.opencode/plan/2026-11-22-go-version-floor.md` — **done 2026-11-22**:
    `go.mod` reads `go 1.24.0` + `toolchain go1.27.3`, matching the documented
    `Go >= 1.24` floor (a 1.24 consumer is no longer silently upgraded or
    blocked). Guarded by `TestGoModGoDirectiveFloor`. The floor moved from 1.23
    to 1.24 with the facade's generic type aliases (see the internal-codec
    extraction plan).
  - `.opencode/plan/2026-11-22-internal-codec-extraction.md` — **done
    2026-11-22**: the semantic engine moved to `internal/codec` and the module
    root became a thin facade of type aliases and wrappers; the Go floor moved
    to 1.24 for generic type aliases. The scanner stays public at
    `qi-json/scanner`; hiding it is deferred to a separate breaking change.
  - `.opencode/plan/user-scenarios.md` — end-to-end user walkthroughs and the
    gaps they expose (the foreign-struct-field gap was a release blocker, closed
    in v0.2.0).
  - `.opencode/plan/2026-11-22-third-party-struct-fields.md` — research for
    struct fields whose type is a struct from another package (**done
    2026-11-22**; decision is **F**: A + B + C, with D as the escape hatch; E
    rejected).
  - `.opencode/plan/2026-11-22-foreign-struct-fields-impl.md` — **done
    2026-11-22**: implemented that decision (TDD tasks T-F1…T-F6; Article VI
    amendment v3.4.0 for the delegation constructor C).
  - `.opencode/plan/2026-11-22-git-and-code-quality-reglament.md` — the **Git
    and Review Discipline** and **Rigorous Code Quality** rules added to the
    constitution (v3.5.0) and `AGENTS.md`, plus the audit and the refactoring
    tasks T-Q1…T-Q4 they imply (resume here for code-quality work).
  - `.opencode/plan/2026-11-22-release-flow.md` — **done 2026-11-22**: the
    **Release flow** section added to the constitution (v3.6.0) and mirrored in
    `AGENTS.md` (choose the version, prepare on `dev`, the `dev` → `main` pull
    request, tag the merge commit, keep `dev`).
  - `.opencode/plan/2026-11-21-package-rename.md` — relocated the semantic layer
    to the module root and renamed it `qijson` (done 2026-11-21).
  - `.opencode/plan/2026-11-21-scanner-rewrite.md` — scanner rewrite rationale
    and verification log.
  - `.opencode/plan/2026-11-21-json-unmarshal-research.md` — decoder research
    (see the decoder plan for what is superseded).
  - `.opencode/plan/2026-11-21-json-scanner.md` — superseded state machine.
