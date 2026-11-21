# JSON Scanner Implementation Plan — 2026-11-21

## Status

The scanner was refactored after the first implementation: the public streaming
API (`Scanner`, `Step`, `EOF`, `SyntaxError`) is gone. The code now lives in the
`scanner` package (`qi-json/scanner`) and exposes only
`Valid(data []byte) bool`, matching `encoding/json.Valid` in name and
signature; all machinery is internal (`machine`, `opcode`, `expectation`).
`marshal` (`qi-json/marshal`) is a reserved placeholder for the future
encoding/json-compatible layer. Behavior is unchanged and covered by the same
corpora.

## Goal

Implement `docs/task.md`: a byte-at-a-time JSON syntax validator in Go as a
deterministic state machine. The machine still consumes one byte at a time
internally (`machine.step`, `machine.finish`); `Valid` wraps it for callers.

## Constraints

- Go >= 1.23 (toolchain: go1.27.3), zero external dependencies.
- Never use `encoding/json`, including in tests and benchmarks.
- Follow uber-go/guide; clean, readable, fault-tolerant code.
- TDD: failing tests first, then minimal implementation, then refactor.
- Performance matters: no per-byte allocations, pooled machine, inlinable
  byte predicates, no formatting on the happy path.

## API (`scanner` package, import `qi-json/scanner`)

```go
func Valid(data []byte) bool
```

Internal (`scanner/machine.go`): `machine`, `opcode` (`opContinue`, `opLiteralStart`,
`opObjectStart`, `opObjectEnd`, `opArrayStart`, `opArrayEnd`, `opDocumentEnd`,
`opFailure`), `expectation` (`expectKey`, `expectValue`, `expectElement`),
`parseError{at, problem}`.

## State machine

Faithful to docs/task.md §7–§13:

- `beginValue`, `afterValue`, `afterDocument`
- `beginKey`, `beginKeyOrEnd`, `beginValueOrEnd`
- `inString`, `inEscape`, `unicodeEscape1`…`unicodeEscape4`
- literals: `trueR`/`trueRU`/`trueRUE`, `falseA`/`falseAL`/`falseALS`/`falseALSE`,
  `nullU`/`nullUL`/`nullULL`
- numbers: `numberSign`, `numberZero`, `numberDigits`, `numberPoint`,
  `numberFraction`, `numberExponent`, `numberExponentSign`,
  `numberExponentDigits`, plus `extendNumber` and `finishNumber`
- `errorState`

Invariants: `opDocumentEnd` is delayed until the delimiter byte (or `finish`)
proves the value ended; `finish` feeds one synthetic space that is not counted
in `consumed`; `reset` preserves `consumed`.

## TDD steps

1. RED: `scanner/machine_test.go` (opcode sequences, delayed document end, reset and
   positions, sticky failures) and `scanner/valid_test.go` (valid/invalid corpora,
   allocations, deep nesting, fuzz seeds) plus `scanner/bench_test.go`.
2. GREEN: `scanner/machine.go` and `scanner/valid.go`.
3. REFACTOR: naming/comments per uber guide, no behavior change.
4. Verify: `make verify` (test → gofmt → vet → golangci-lint), race, fuzz,
   benchmarks with `-benchmem`.

## Deviation from the pseudocode (deliberate)

`docs/task.md` §12 routes `stateNumberFrac` and `stateNumberExpDigits` through
`handleNumberContinuation`, which would accept `1.2.3` and `1e1e2`. That
contradicts the grammar in §12 and the purpose "validate JSON syntax" (§1).
The implementation ends the number after the fraction/exponent instead
(`finishNumber`), so a second `.` or exponent marker is rejected by
`afterValue`, matching the grammar and `encoding/json.Valid` semantics. Covered
by `TestInvalidDocuments` cases `1.2.3`, `1e1e2`, `1e1.5`, `0e1e2`, `1e+1e2`,
`-1.5E2.3`, `[1.2.3]`, `{"a":1.5.6}`.

## Verification results

- `make verify`: pass; coverage 99.6%.
- `go test -race ./...`: pass.
- `go test -fuzz=FuzzValid`: pass (millions of executions, no panics).
- Benchmarks (Apple M1): 0 B/op, 0 allocs/op.

## Layout change (package split)

The code moved from the module root into `scanner/` (`machine.go`,
`machine_test.go`, `valid.go`, `valid_test.go`, `bench_test.go`). `marshal/`
holds only `doc.go` for now. The module root has no Go package. Re-verified
after the move: `make verify` pass, `go test -race ./...` pass, `FuzzValid`
2.1M executions without panics, benchmarks 0 B/op / 0 allocs/op.
