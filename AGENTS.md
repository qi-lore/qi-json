# AGENTS.md — qi-json

A from-scratch JSON library with four Go packages:

- `.` (import `github.com/qi-lore/qi-json`, package `qijson`) — the single public
  import path. The facade of the reflection-free semantic layer (the decoder and
  encoder) and of the syntax layer (`Valid`, `Kind` plus the tokenizer), through
  type aliases, const re-exports and one-line wrapper functions. Not
  `encoding/json`-compatible; no `reflect`, no `unsafe`.
- `internal/codec/` — the semantic engine behind that facade. Internal, so
  external code cannot import it and its internals may change without a
  compatibility promise.
- `internal/scanner/` — the single-pass syntax engine: `Valid` and the tokenizer
  (`Kind` plus a tokenizing type). Internal; the facade re-exports its whole
  surface.
- `cmd/qi-json-gen` — the optional, stdlib-only code generator that emits
  `qijson` `Schema` values and `DecodeJSON`/`EncodeJSON` methods from struct
  tags. No `reflect`/`unsafe`; it targets only the exported `qijson` surface.

The module root is the `qijson` package. No third-party dependencies in the
shipped module (the `bench/` comparative benchmark module is a separate,
non-shipped module and the sole exception). `make` wraps the common tasks.
`docs/tasks.md` is the single source of truth for task state;
`docs/constitution.md` governs changes. Plan files under `.opencode/plan/` hold
design rationale and history only.

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
- `go test ./internal/scanner -run '^TestInvalidDocuments$'` — run one test/table
- `go test -run=^$ -fuzz=FuzzValid -fuzztime=10s ./internal/scanner` — fuzz (`-run=^$` is required)
- `go test -run=^$ -bench=. -benchmem ./internal/scanner` — benchmarks
- `go generate ./...` — regenerate `cmd/qi-json-gen` fixtures — no other package
  emits generated code

## Git and review discipline

- Branching: `main` is production, `dev` is integration and carries the newest
  work (integrated changes live there for a long time before a release). Create
  feature/task branches from the current `dev` (after `git fetch` and a
  fast-forward update) and target `dev`; only the `dev` → `main` pull request
  may merge into `main`. Branch names follow Conventional Branch
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
  (mark done items in `docs/tasks.md`, refresh baseline references in the plans);
  commit as `chore(release): vX.Y.Z` with a full body; run `make verify`.
- Open the `dev` → `main` pull request (the only PR allowed into `main`) and let
  CI pass.
- Merge into `main`, then tag the merge commit: `git tag vX.Y.Z <merge-sha>` and
  push the tag.
- Keep `dev` — do not delete the integration branch.
- The tag publishes the tagged module; a consumer installs the optional
  `qi-json-gen` generator with
  `go install github.com/qi-lore/qi-json/cmd/qi-json-gen@vX.Y.Z` (see the README's
  "Installing the generator"). There are no prebuilt release artifacts.

## Documentation ownership

- `README.md` is owned by the user. Do not edit it, reformat it, or add anything
  to it without a direct, explicit instruction from the user: a request to read,
  check or review the README is not an authorization to change it. Any change to
  `README.md` requires the user's direct instruction beforehand.

## Task tracking

- `docs/tasks.md` is the single source of truth for task state: one line per task,
  `- [X] <PREFIX><NNN> - <SHORT_DESCRIPTION>` when done and `- [ ] …` when
  planned. Prefixes group by area; the file's legend lists them.
- Tasks MUST be recorded only in `docs/tasks.md`. Do not record task status or a
  backlog in `.opencode/plan/` files or in `AGENTS.md`.
- `docs/tasks.md` MUST be updated before every commit.

## Code quality

All Go code follows the Uber Go Style Guide
(https://github.com/uber-go/guide/blob/master/style.md) without exception, plus:

- Wrap errors with context: `fmt.Errorf("...: %w", err)`; never return a bare
  error from a deeper layer. The typed `*qijson.Error` is itself the context
  (path + offset) and is returned unwrapped; use `%w` only where a layer adds new
  context (for example a sentinel cause).
- Define a sentinel in the package that owns the behavior (`internal/codec` for
  the decoder and encoder, `internal/scanner` for the syntax layer) and expose it
  through the `qijson` facade only when a caller must inspect it.
- Public APIs document ownership, concurrency safety, and lifecycle.
- Goroutines need clear ownership and shutdown paths; no leaks.
- Propagate context explicitly; blocking operations accept `context.Context`.
- No panics in library code for recoverable conditions.
- Error messages and exported doc comments describe behavior; they never cite
  project documents (`docs/`, `.opencode/plan/`, the constitution or the README).
  A consumer does not have the repository, and a failure should say what is wrong,
  not where to read about it.

The library is synchronous (no goroutines, no blocking calls), so the goroutine
and context rules bind only when such code is added. Open gaps are tracked in
`docs/tasks.md`.

## API and naming

- `Valid(data []byte) bool` (in `internal/scanner`, re-exported as `qijson.Valid`)
  is the syntax layer's headline export — the same name and signature as
  `encoding/json.Valid`. The sanctioned public surface is exactly the `qijson`
  decoder and encoder, the re-exported syntax surface (`Valid`, `Kind` and the
  tokenizer), and the network/formatting/hooks additions: streaming
  (`Writer.DumpTo`, `Writer.ReadCloser`, `NewDecoder(io.Reader)` with a
  multi-value loop, `Token` plus `Number`/`Delim`/`Token`), formatting
  (`MarshalIndent`, `Indent`, `Compact`, `HTMLEscape`, `SetIndent`,
  `SetEscapeHTML`), and the omit hooks (`Optional`/`IsDefined`, `IsZero`) plus the
  `UnmarshalUnknown`/`MarshalUnknowns` unknown-fields proxy, and the encoder's
  `Writer.BuildBytes(reuse ...[]byte)` writer-buffer reuse surface (constitution
  Article VI, amended 2026-11-26; rationale:
  `.opencode/plan/2026-11-23-roadmap-network-perf-parity.md`). Do not export
  anything else without a new amendment.
- `validate(data []byte) (parseError, bool)` in `internal/scanner` is unexported
  and returns the first failure plus whether the input is invalid. Tests assert
  messages and 1-based positions through it; `Valid` is `!failed`. The error is a
  value type so that returning it does not drag the validator's inline stack to
  the heap.
- Internal names are repo-specific, not copies of encoding/json internals:
  `validator`, `container`/`containerObject`/`containerArray`, `parseError`,
  `skipSpace`, `scanString`, `scanEscape`, `scanLiteral`, `scanNumber`,
  `fail`/`failEnd`/`failAt`, and the tables `spaceTable`/`stringStop`. Do not add
  `Scanner`, `SyntaxError`, `scanBegin*`, `stateBegin*`, `opcode`, `machine`,
  `step` or `finish`.
- The scanner uses no `goto` and no statement labels. `run` is two nested loops:
  an outer loop that scans one value, and an inner loop that consumes the
  delimiter after a completed value (see "Contract details that are easy to
  break" below). The object-member scan is deliberately written twice so it stays
  inline; extracting it into a helper costs about 8% on object-heavy documents.
- `qijson` is the semantic layer: a reflection-free decoder and encoder over the
  syntax engine in `internal/scanner`. It MUST NOT import `encoding/json` and
  MUST NOT use `reflect` or `unsafe`; struct binding is by compile-time accessor
  closures. Its API is
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
- The `qijson` root is a facade: `internal/codec` and `internal/scanner` hold
  the implementations and `qijson.go` re-exports them through type aliases,
  constants and wrapper functions. Keep the public contract (including ownership,
  concurrency and lifecycle docs) on the facade; the engines' internals stay
  unexported. The generic aliases are why the Go floor is 1.24. See
  `.opencode/plan/2026-11-22-internal-codec-extraction.md` and
  `.opencode/plan/2026-11-22-scanner-consolidation.md`.
- `cmd/qi-json-gen` is the optional code generator (stdlib only: `go/ast`,
  `go/parser`, `go/format`, and `go/types` via the source importer for
  cross-package named types; same-package named scalar types are resolved from
  the AST). It emits code against the exported `qijson`
  surface only; the runtime is unchanged by it. It emits a `Schema` var plus both
  `DecodeJSON` and `EncodeJSON` for each target type. It MUST NOT import
  `encoding/json` and MUST NOT use `reflect` or `unsafe`.
  Generation must be deterministic (sort all emitted collections) so the drift
  guard passes. Plan: `.opencode/plan/2026-11-21-json-codegen.md`.
- Never use `encoding/json` in the shipped module, including in its tests or
  benchmarks (hard requirement in the constitution, Article III); tests are
  hand-built tables. The `bench/` comparative benchmark module is the sole
  exception: a separate, non-shipped module that may import `encoding/json` and
  `mailru/easyjson`.

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
- Task state: `docs/tasks.md` — the single source of truth for what is done and
  what is planned. Do not look for task status in the plan files.
- Design records (rationale and history only):
  - `.opencode/plan/2026-11-21-json-decoder.md` — decoder design decisions.
  - `.opencode/plan/2026-11-21-json-encoder.md` — encoder design decisions.
  - `.opencode/plan/2026-11-21-json-codegen.md` — the `cmd/qi-json-gen` design.
  - `.opencode/plan/2026-11-21-scanner-rewrite.md` — scanner rewrite rationale.
  - `.opencode/plan/2026-11-21-package-rename.md` — the relocation and rename.
  - `.opencode/plan/2026-11-22-internal-codec-extraction.md` — the `internal/codec`
    extraction and the canonical module path.
  - `.opencode/plan/2026-11-22-scanner-consolidation.md` — the move of the syntax
    layer to `internal/scanner` and its re-export through the facade.
  - `.opencode/plan/2026-11-22-third-party-struct-fields.md` and
    `.opencode/plan/2026-11-22-foreign-struct-fields-impl.md` — foreign struct
    fields: research and implementation.
  - `.opencode/plan/2026-11-22-git-and-code-quality-reglament.md` — the governance
    amendment and its audit.
  - `.opencode/plan/2026-11-22-release-flow.md` — the release-flow governance.
  - `.opencode/plan/2026-11-22-go-version-floor.md` — the Go-floor fix.
  - `.opencode/plan/2026-11-22-tasks-registry.md` — this registry's design.
  - `.opencode/plan/2026-11-23-roadmap-network-perf-parity.md` — the Track A
    network/formatting/hooks roadmap behind the Article VI amendment.
  - `.opencode/plan/user-scenarios.md` — end-to-end user walkthroughs.
  - `.opencode/plan/2026-11-21-json-unmarshal-research.md` — decoder research.
  - `.opencode/plan/2026-11-21-json-scanner.md` — superseded state machine.
  - `.opencode/plan/2026-11-21-post-decoder-backlog.md` — historical backlog,
    superseded by `docs/tasks.md`.
