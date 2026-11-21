# AGENTS.md — qi-json

A from-scratch JSON library with two Go packages:

- `scanner/` (import `qi-json/scanner`) — the single-pass syntax scanner;
  exports only `Valid`.
- `marshal/` (import `qi-json/marshal`) — reserved for the
  encoding/json-compatible encoding layer; currently a documented placeholder
  with no exports.

The module root has no Go package. No third-party dependencies. `make` wraps
the common tasks. `docs/task.md` is the specification; `docs/constitution.md`
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

## API and naming

- The only exported identifier is `scanner.Valid(data []byte) bool` — the same
  name and signature as `encoding/json.Valid`. `marshal` deliberately exports
  nothing yet. Do not export anything else without amending the constitution.
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
- Spec: `docs/task.md`.
- Design rationale and verification log:
  `.opencode/plan/2026-11-21-scanner-rewrite.md` (rewrite) and
  `.opencode/plan/2026-11-21-json-scanner.md` (superseded state machine).
