# qi-json Constitution

Short, enforceable rules for this repository. The test suite is the executable
contract; this document governs how changes are made.

## Core Principles

### I. Spec First

`docs/task.md` is the authoritative specification. The implementation follows
its grammar, positions, scanners and invariants. Two strict grammar rules are
deliberate and must be preserved: a `0` integer part may not be followed by a
digit (`01` is invalid), and a number allows at most one fraction and one
exponent (`1.2.3`, `1e1e2` are invalid). Rationale:
`.opencode/plan/2026-11-21-scanner-rewrite.md`.

### II. Test-Driven Development (NON-NEGOTIABLE)

Every behavior change starts with a failing test (red), then the minimal
implementation (green), then refactoring without behavior change. Tests encode
exact error messages and 1-based positions; never weaken them to make code pass.

### III. Standard Library Only

No third-party dependencies. `encoding/json` MUST NOT be used anywhere,
including tests and benchmarks — writing our own validator is the point of the
project.

### IV. Syntax Contract

The validator is syntax-only: exact error strings and 1-based positions, sticky
first failure, delayed scalar completion, any top-level JSON value, no UTF-8 or
surrogate validation, no nesting depth limit. Changing any of these requires
updating the spec and the tests in the same change.

### V. Performance Is a Feature

`Valid` scans the whole slice in one pass and must stay allocation-free for
documents nesting at most `nestingInline` levels, guarded by
`TestValidDoesNotAllocate`. Per-byte dispatch through function pointers, closures
or a handler table is prohibited, and `goto` and statement labels are prohibited
throughout the scanner. The hot path is a structured pair of loops (see
`docs/task.md` §6.1); the object-member scanner is written twice on purpose so it
stays inline, because a shared helper costs about 8% on object-heavy documents.
Deeper documents may allocate the nesting stack once. Prefer the fastest correct
implementation and support structural changes with benchmarks.

### VI. Minimal Public Surface

Exactly one exported identifier module-wide: `scanner.Valid(data []byte) bool`,
matching `encoding/json.Valid` in name and signature. `scanner.validate` is
unexported and returns the first `parseError` plus whether the input is invalid,
so diagnostics stay testable without widening the API. Package `marshal` may
export `Marshal`/`Unmarshal`-style functions only after an amendment. Internal
identifiers are repo-specific (`validator`, `container`/`containerObject`/
`containerArray`, `parseError`, `skipSpace`, `scan*`); do not copy
`encoding/json` identifiers such as `Scanner`, `SyntaxError`, `scanBegin*`, or
`stateBegin*`.

## Verification Workflow (NON-NEGOTIABLE)

Every task MUST finish by running these commands in this order. Stop at the
first failure, fix it, then restart the sequence from step 1:

1. `go test ./...`
2. `gofmt -l .` — must print nothing; fix with `go fmt ./...`
3. `go vet ./...`
4. `golangci-lint run` — v2, default linters, no repo config

A task is not complete until all four pass. Report the commands and their
results in the final summary.

## Additional Constraints

- Go >= 1.23 (toolchain in `go.mod`: 1.27.3).
- Two packages under module `qi-json`, no root Go package:
  - `scanner/` — public `Valid` (`valid.go`) and the scanner (`scan.go`);
  - `marshal/` — placeholder for the encoding/json-compatible layer, exports
    nothing yet.
- New packages, files beyond the current layout, exported identifiers, or
  dependencies require an amendment to this constitution.

## Governance

This constitution supersedes ad-hoc workflow preferences. Amendments update this
file and `AGENTS.md` together. If code and tests disagree, resolve the
discrepancy against `docs/task.md`, then update both.

**Version**: 2.1.0 | **Ratified**: 2026-11-21 | **Last Amended**: 2026-11-21
