# AGENTS.md — qi-json

A from-scratch JSON library with two Go packages:

- `scanner/` (import `qi-json/scanner`) — the byte-at-a-time syntax state
  machine; exports only `Valid`.
- `marshal/` (import `qi-json/marshal`) — reserved for the
  encoding/json-compatible encoding layer; currently a documented placeholder
  with no exports.

The module root has no Go package. No third-party dependencies. `make` wraps
the common tasks. `docs/task.md` is the original spec; `docs/constitution.md`
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
- Internal names are repo-specific, not copies of encoding/json internals:
  `machine`, `opcode`/`op*`, `expectation`/`expect*`, handlers such as
  `beginValue`, `afterValue`, `afterDocument`, `numberFraction`. Do not add
  `Scanner`, `SyntaxError`, `scanBegin*`, or `stateBegin*`.
- Never use `encoding/json`, including in tests or benchmarks (hard requirement
  in `docs/task.md`); tests are hand-built tables.

## Contract details that are easy to break

- `machine.reset()` deliberately preserves `consumed`, so `parseError.at` is a
  cumulative stream position across resets, not document-relative
  (`TestFailureOffsetAccumulatesAcrossReset`).
- `machine.finish()` feeds one synthetic space to finish a pending
  number/literal; it must not increment `consumed` or shift failure positions.
- `opDocumentEnd` is delayed until the delimiter byte (or `finish` proves
  completion) and may repeat for trailing whitespace (`TestOpcodeSequence`,
  `TestDelayedDocumentEnd`).
- Numbers: `numberFraction` and `numberExponentDigits` call `finishNumber`, not
  `extendNumber`, so `1.2.3` / `1e1e2` are rejected. This is a deliberate
  deviation from the §12 pseudocode in `docs/task.md` (that pseudocode
  contradicts the stated grammar); do not "restore" it. Rationale:
  `.opencode/plan/2026-11-21-json-scanner.md`.
- Error messages and 1-based positions are asserted exactly; update tests
  deliberately when behavior changes, never just to make the suite pass.
- `TestValidDoesNotAllocate` guards the hot path: `Valid` reuses a pooled
  `machine` and must stay allocation-free in steady state.
- The validator is syntax-only by design: no UTF-8 validation, no surrogate
  pairing checks, no nesting depth limit.
- TDD is mandatory per the constitution: failing test first, minimal
  implementation, refactor. Keep state handlers as `func(*machine, byte) opcode`.

## Context

- Governing rules: `docs/constitution.md`.
- Spec: `docs/task.md` (do not modify).
- Design rationale and verification log: `.opencode/plan/2026-11-21-json-scanner.md`.
