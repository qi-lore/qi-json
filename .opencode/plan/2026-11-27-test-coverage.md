# qi-json — raise test coverage across all packages

**Date**: 2026-11-27
**Branch**: `chore/raise-test-coverage` (based on `dev`)
**Related**: `docs/tasks.md` (Q011), `coverage.out`, `qijson.go`, `api_test.go`,
`internal/codec`, `internal/scanner`, `cmd/qi-json-gen`.

This file is a design record. Task state lives in `docs/tasks.md`.

---

## 1. Intent

Raise statement coverage across the whole module. The 84.2% total hides large
per-package gaps — most of all in the root facade — so the goal is to exercise
reachable-but-untested code, not to pad a number.

## 2. Baseline

Total statement coverage **84.2%** (current `coverage.out`). Per package:

| Package | Coverage |
|---|---|
| `internal/scanner` | 93.3% |
| `internal/codec` | 89.2% |
| root `qijson` facade | 46.9% |
| `cmd/qi-json-gen` | 77.1% |
| `cmd/qi-json-gen/internal/gentest` | 65.1% (`xtypes` 0%) |
| `example` | 59.1% |

## 3. Root causes

- The facade wrappers in `qijson.go` are referenced only as compile-time
  API-surface assertions in `api_test.go`; they are never called, so the whole
  re-export layer reads as uncovered.
- The tokenizer's typed-read mismatch error paths and several codec
  error/constructor paths are untested.
- Generator branches and the CLI glue are untested.

## 4. Decision

One task (Q011) on `chore/raise-test-coverage`, based on `dev`. The scope is all
packages — everything reachable. Tests are behavioral and error-path oriented,
not coverage padding. Where a `func main()` body would otherwise be unreachable,
make it reachable through a thin `run()` refactor.

## 5. Constraints

- No `encoding/json` in the shipped module or its tests (constitution Article
  III); the `bench/` module is the sole exception.
- Hand-built test tables.
- TDD: failing test first.
- `make verify` green (tests, `gofmt`, `go vet`, `golangci-lint`).

## 6. Outcome

`make verify` is green: `go test ./...`, `gofmt -l .` (no output), `go vet ./...`
and `golangci-lint run` (0 issues). No `encoding/json`, `reflect` or `unsafe`
appears outside `bench/`. No public API semantics changed.

Final per-package statement coverage:

| Package | Coverage |
|---|---|
| root `qijson` | 100.0% |
| `internal/codec` | 98.5% |
| `internal/scanner` | 99.1% |
| `cmd/qi-json-gen` | 97.6% |
| `cmd/qi-json-gen/internal/gentest/xtypes` | 100.0% |
| `example` | 95.8% |
| `cmd/qi-json-gen/internal/gentest` | 92.3% |

Total **98.3%** (headline) / **97.5%** (raw statement sum).

The only package below the aspirational target is
`cmd/qi-json-gen/internal/gentest` at 92.3%: its remaining uncovered statements
are provably unreachable or generated-dead code.

Root cause: `internal/codec/schema.go` (`Schema.Encode`) consults `Field.Empty`
only when `omitEmpty` is set, yet `cmd/qi-json-gen` emits an `Empty:` closure for
every scalar field — so the closures emitted on non-`omitempty` fields are dead.
The other residuals are defensive/no-op branches: 64-bit range guards, defensive
returns after a prior check, and CLI `os.Exit` glue.

Decision: per the user, the generator's dead `Empty:` emission is left as-is (no
production change). A future follow-up could emit `Empty:` only for `omitempty`
fields, which removes the dead code and lifts `gentest` to roughly 96%.

Production changes: `cmd/qi-json-gen/main.go` and `example/main.go` were
refactored so `main()` is a thin wrapper over a testable `run(...)`;
`cmd/qi-json-gen/internal/gentest/xtypes/xtypes.go` was extended as a test
fixture. All other changes are tests.
