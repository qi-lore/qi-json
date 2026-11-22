# Scanner rewrite: drop the state machine, keep the contract

Date: 2026-11-21

Decision: the user approved the rewrite — "state machine is a redundant entity if
we have a faster implementation. rewrite specs, and then rewrite code".

This file is a design record. Task state lives in `docs/tasks.md`.

## 1. Why

Head-to-head in one binary on identical inputs, this scanner is **2.3×–5.1×
slower** than the baseline it is meant to beat:

| Document | Before | `encoding/json.Valid` | Gap |
|---|---|---|---|
| 87-byte object | 251.3 MB/s | 570.5 MB/s | 2.27× |
| 1 000-object array | 262.3 MB/s | 749.2 MB/s | 2.86× |
| 10 000 numbers | 265.6 MB/s | 692.1 MB/s | 2.61× |
| 64 KiB string | 286.9 MB/s | 1 465.8 MB/s | 5.11× |

The cost is the architecture, not tuning. A CPU profile shows 48% of samples in
`Valid`'s dispatch loop and 42% in `inString`: one indirect call per byte. The
baseline moved to bulk consumption (`jsonwire.ConsumeStringResumable`,
`jsontext.(*decoderState).consumeString`); we did not.

Two cheap fixes recovered 25–35%, leaving a 1.8×–3.8× gap. Closing it needs the
architecture to change.

## 2. Decision

The state machine is not a second entity to keep as a reference. It is the
implementation that is being replaced. There is **one** implementation, and the
spec describes it.

Removed from the spec and the code:

- the `handlerFunc` table and per-state handler functions,
- the per-byte `step` / `finish` driver,
- the `opcode` event vocabulary and every opcode test,
- `reset` and the "`reset` preserves `consumed`" invariant,
- the `sync.Pool` of machines.

Kept and strengthened:

- the grammar, the error strings, and the 1-based error positions,
- "a value is only complete once a delimiter or EOF follows it",
- syntax-only validation (no UTF-8, no surrogate pairing, no depth limit),
- zero allocations in steady state,
- deterministic results.

## 3. New design

A single pass over the whole `[]byte` with an explicit index, an explicit
container stack, and bulk scanning of strings, numbers, literals and
whitespace. No indirect calls in the hot path.

```go
func Valid(data []byte) bool {
	_, failed := validate(data)
	return !failed
}

func validate(data []byte) (parseError, bool) {
	var inline [nestingInline]container
	v := validator{data: data, stack: inline[:nestingInline]}
	if v.run() { return parseError{}, false }
	return *v.failure, true
}
```

- `run` is one function with four labels — `scanValue`, `scanKey`, `afterKey`,
  `afterValue` — joined by `goto`. Not a switch over a position variable, and not
  a function pointer. The label form measured ~25% faster than the switch form.
- The nesting stack is a `[32]container` array on the caller's stack plus an
  explicit depth counter. `append` is never used on the hot path, so the array
  does not escape and nothing is allocated. Growth past 32 levels doubles into
  the heap.
- `sync.Pool` is deleted: measured at **8.4 ns/call**, versus **0.3 ns** for the
  stack-local array.
- `parseError` stays as the diagnostics contract. It is a value type and
  `validate` returns it by value: returning `*v.failure` would make the validator
  escape and drag the inline array to the heap (measured: 1 alloc/op).
- Whitespace and string terminators use 256-entry tables (`spaceTable`,
  `stringStop`); long string runs use a SWAR word test (`stringStopWord`), with a
  short byte loop first so short keys skip the word setup.

## 4. Measured facts behind the design

| Fact | Measurement |
|---|---|
| `sync.Pool` Get/Put round-trip | 8.37–8.43 ns/op |
| Stack-local inline array setup | 0.317 ns/op |
| `[32]container` + depth counter, `0 allocs/op` | confirmed |
| `append` into a local array | escapes to heap — must avoid |

## 5. Error contract (unchanged strings, cleaned EOF messages)

Tested positions and messages are reproduced exactly. The only deliberate
wording change is for EOF *inside* a value fragment, where the original
fabricated a synthetic space character:

| Situation | Message | Position |
|---|---|---|
| EOF at a value/key/colon/delimiter position | `unexpected end of JSON input` | `len(data)` |
| EOF inside a string body | `unexpected end of JSON input` | `len(data)` |
| EOF immediately after `\` | `in string escape code` | `len(data)` |
| EOF inside `\uXXXX` | `in \u hexadecimal character escape` | `len(data)` |
| EOF inside a literal | `in literal <lit> (expecting '<c>')` | `len(data)` |
| EOF inside a number | `in numeric literal` / `after decimal point in numeric literal` / `in exponent of numeric literal` | `len(data)` |

Every other error keeps the original `invalid character '<c>' <context>` form.

## 6. TDD sequence

1. Write `scanner/validator_test.go` against `validate` and `scan*` helpers.
   It does not compile → red.
2. Write `scanner/scan.go` and `scanner/valid.go` until green.
3. Delete `scanner/machine.go` and `scanner/machine_test.go`.
4. Update `scanner/valid_test.go` to stop calling `drive`.
5. Re-run the invalid-document table; it must pass unchanged in positions and
   message substrings.

## 7. Verification

`make verify` (test → gofmt → vet → golangci-lint), plus:
`go test -race ./...`, `make fuzz`, `make bench`, and a head-to-head benchmark
against the pre-rewrite numbers in §1.

## 8. Risks

- **Speed regression on the structure benchmarks.** The switch may not beat the
  indirect call by enough on object/array documents. Mitigation: measure after
  each step; if `BenchmarkValidArrayOfObjects` does not improve, flatten the
  hot loop before declaring done.
- **Deep nesting now allocates.** Growth past 32 levels allocates the stack.
  Documented in the spec and the constitution; `TestValidDeepNesting` still
  covers correctness at depth 50 000.
- **Lost opcode coverage.** The per-byte opcode tests are deleted with the
  opcodes. The invalid-document table, the valid corpus, the fuzz target and the
  deep-nesting test carry the contract. This is an accepted, explicit trade.

## 9. Outcome

All of the planned work landed in a single implementation; no parallel oracle or
second code path remains. Measured on the same machine (Apple M1) with the same
documents in one binary:

| Document | Before (state machine) | After | `encoding/json.Valid` | After vs stdlib |
|---|---|---|---|---|
| object | 251.3 MB/s | 846 MB/s | 566 MB/s | 1.49× |
| array of objects | 262.3 MB/s | 819 MB/s | 742 MB/s | 1.10× |
| numbers | 265.6 MB/s | 1048 MB/s | 689 MB/s | 1.52× |
| string | 286.9 MB/s | 2582 MB/s | 1452 MB/s | 1.78× |

All four documents are faster than `encoding/json.Valid`; the rewrite is 3.1×–9.0×
faster than the byte-at-a-time state machine. `0 allocs/op` on all four, and
`TestValidDoesNotAllocate` passes.

Optimizations that produced the result, in order of measured effect:

1. `goto`-labelled dispatch in place of the switch over a position variable:
   ~25% on the object and array documents.
2. Word-at-a-time string scanning (`stringStopWord`) with a byte warm-up and a
   `long` flag: long strings 1.2 → 2.6 GB/s without slowing short keys.
3. 256-entry tables for whitespace (`spaceTable`) and string terminators
   (`stringStop`): ~5% on the array document.

The one deliberate cost: `parseError` is a value type and `validate` returns
`(parseError, bool)`. Returning `*parseError` made the validator escape and moved
the inline stack to the heap (1 alloc/op); the value return keeps the hot path at
zero allocations. The diagnostic path may still allocate.

## 10. Session state (handoff)

### Repo state

The rewrite is **complete and verified in the working tree**. Nothing is
committed: the repo is a git repository with two commits (`chore(repo): initial`,
`feat(scanner): add scanner`) and the rewrite is uncommitted.

```text
 M AGENTS.md
 M docs/constitution.md
 M docs/task.md
 M scanner/valid.go
 M scanner/valid_test.go
 D scanner/machine.go
 D scanner/machine_test.go
?? .opencode/plan/2026-11-21-scanner-rewrite.md
?? scanner/scan.go
?? scanner/validator_test.go
```

Last full verification (`make verify`): `go test ./...` ok, `gofmt -l .` empty,
`go vet ./...` clean, `golangci-lint run` → `0 issues`. Also clean:
`make race`, `make fuzz` (2.9M execs, PASS), `make bench`. Numbers in §9.

Implementation is the `goto`-labelled version in `scanner/scan.go`; docs
(`docs/task.md`, `docs/constitution.md`, `AGENTS.md`) describe that version, and
regression tests for the word-at-a-time string path were added to
`scanner/valid_test.go` and `scanner/validator_test.go`.

### Open question: remove the `goto` labels?

The user asked whether the `goto` labels in `run` can be dropped. Measured on
the same machine with the same documents:

| Variant | object | array | numbers | string |
|---|---|---|---|---|
| `goto` labels (current) | 846 MB/s | 819 MB/s | 1048 MB/s | 2582 MB/s |
| switch over position variable | 687 MB/s | 665 MB/s | 970 MB/s | 2575 MB/s |
| `encoding/json.Valid` | 566 MB/s | 742 MB/s | 689 MB/s | 1452 MB/s |

Removing `goto` costs about 19% on object and array and 7% on numbers, and drops
the array document below `encoding/json.Valid` (665 vs 742 MB/s). String scanning
is unaffected because it never touches the dispatch. Both variants pass the full
test suite and the invalid-document table unchanged.

A third variant — a structured `for` loop with an `afterValue` inner loop and a
`scanKey` helper, no `goto` and no duplicated key code — is implemented and green
at `/private/tmp/opencode/structured/scanner/scan.go`. Its benchmark was
interrupted before results were collected, so it is **unmeasured**.

### Disposable experiment trees (not project code)

- `/private/tmp/opencode/nogoto/` — switch-over-position variant, tests green.
- `/private/tmp/opencode/structured/` — structured `for`-loop variant, tests green,
  not yet benchmarked.
- `/private/tmp/opencode/compare/` — head-to-head against `encoding/json.Valid`.
- `/private/tmp/opencode/esc/`, `vbench/`, `scannerexp/` — earlier escape-analysis
  and pool experiments.

### Next steps

1. Benchmark `/private/tmp/opencode/structured` (`go test -run=^$ -bench=. -benchmem -count=3`).
2. Decide with the user: keep `goto` (fastest), or accept ~19% on object/array for
   the structured form. If the structured form lands within ~5% of `goto`, prefer
   it for readability; otherwise keep `goto` and record why.
3. If the form changes, update `docs/task.md` §6.1/§7, `AGENTS.md` and
   `docs/constitution.md` Article V in the same change, then re-run `make verify`.
4. Commit the rewrite (conventional commit, e.g. `perf(scanner): replace state
   machine with single-pass scanner`) once the form is settled.

## 11. Decision: remove `goto` (2026-11-21)

The user ruled that `goto` labels are unacceptable style, so the label form is
retired. All no-`goto` forms were benchmarked head to head on the same machine
and documents (`go test -run=^$ -bench=. -benchmem -count=3 ./scanner`):

| Variant | object | array-of-objects | numbers | string |
|---|---|---|---|---|
| labels + `goto` (retired) | 854 MB/s | 823 | 1056 | 2590 |
| switch over a position variable | 684 | 663 | 968 | 2576 |
| structured loops + `scanKey` helper | 785 | 759 | 1042 | 2587 |
| bool flag, single key path | 783 | 770 | 1014 | 2570 |
| bool flag, fall-through key | 776 | 778 | 1011 | 2560 |
| **structured loops, key scan inlined** | **835** | **820** | **1046** | **2579** |

The regression in every "clean" variant is the non-inlinable `scanKey` helper on
the key path (it calls `scanString`, so it never inlines). Inlining the
object-member scan at its two sites — just after `{` and just after an object
`,` — recovers essentially all of it, within ~2% of the retired label form, with
no `goto`. That variant is now the implementation in `scanner/scan.go`.

Trade accepted: the object-member block is duplicated. It is the only way to
keep the key path inline without `goto` or a flag, and both alternatives cost
8–20%. The duplication is called out in `run`'s doc comment, at both sites, and
in `docs/task.md` §6.1 so a future "deduplicate" does not silently regress it.
`docs/task.md` §6.1/§7, `docs/constitution.md` (Article V, v2.1.0) and
`AGENTS.md` were amended in the same change.


