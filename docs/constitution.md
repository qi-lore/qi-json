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

No third-party dependencies. `encoding/json` MUST NOT be used anywhere in the
module, including tests and benchmarks — writing our own scanner and decoder is
the point of the project.

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

### VI. Public Surface

`scanner.Valid(data []byte) bool` is the scanner's headline export, matching
`encoding/json.Valid` in name and signature. `scanner.validate` stays unexported:
it returns the first `parseError` plus whether the input is invalid, so
diagnostics stay testable without widening the API.

Four further surfaces are sanctioned (amended 2026-11-21; the delegation
constructor added 2026-11-22; rationale:
`.opencode/plan/2026-11-21-json-decoder.md`,
`.opencode/plan/2026-11-21-json-codegen.md`,
`.opencode/plan/2026-11-21-json-encoder.md`,
`.opencode/plan/2026-11-22-third-party-struct-fields.md`; the decoder/encoder
package was relocated to the module root and renamed `qijson` per
`.opencode/plan/2026-11-21-package-rename.md`):

- **`scanner` tokenizer** — a `Kind` and a tokenizing type exposing
  `Peek`/`Read*`/`Begin*`/`End*`/`Skip`/`Offset`/`Err`, so the semantic layer
  consumes the same engine.
- **`qijson` decoder** — the reflection-free semantic layer: a generic
  `Unmarshal`/`UnmarshalInto` and the non-generic `Decode`, the `Decoder`
  interface (`DecodeJSON(*Reader) error`), `Schema[T]`/`Struct`, the `Field[T]`
  constructors, a delegation constructor for a nested `Decoder`/`Encoder`
  (`ObjectDecoder`/`ObjectDecoderPtr`, `DecoderCodec`) so a field or element
  whose type implements `DecodeJSON`/`EncodeJSON` binds by calling them,
  `Reader`, and the error types.
- **`qijson` encoder** — the reflection-free encode layer, the mirror of the
  decoder: the `Encoder` interface (`EncodeJSON(*Writer) error`), `Writer` and
  `NewWriter`, the generic `Marshal` and non-generic `Encode`,
  `Schema.Encode`/`Schema.Marshal`, `Field.Encode`/`Field.Empty`/`Field.OmitEmpty`,
  and the element `Codec` with `NewCodec`. Like the decoder it MUST NOT import
  `encoding/json` and MUST NOT use `reflect` or `unsafe`.
- **`cmd/qi-json-gen`** — an optional, stdlib-only code generator that emits
  `qijson` `Schema` values and `DecodeJSON`/`EncodeJSON` methods from struct
  tags, so callers need not hand-write the accessor closures. It is a `main`
  package, targets only the exported `qijson` surface, and MUST NOT import
  `encoding/json` or use `reflect`/`unsafe`. Generated code is ordinary source
  in the caller's package; the `qijson` runtime is unchanged by it.

`qijson` is **not** API-compatible with `encoding/json`; that is deliberate. It
MUST NOT import `encoding/json` and MUST NOT use `reflect` or `unsafe`; struct
binding is by compile-time accessor closures. Internal identifiers stay
repo-specific (`validator`, `container`/`containerObject`/`containerArray`,
`parseError`, `skipSpace`, `scan*`); do not copy `encoding/json` identifiers such
as `Scanner`, `SyntaxError`, `scanBegin*`, or `stateBegin*`.

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

- Go >= 1.23: the `go.mod` `go` directive is the language floor
  (`go 1.23.0`); its `toolchain` line (`go1.27.3`) names the toolchain used to
  develop the project and is ignored by consumers.
- Three packages under module `qi-json` (the module root is a package):
  - `.` (module root) — `qijson`, the reflection-free semantic layer (decode
    and encode);
  - `scanner/` — `Valid` and the tokenizer over the single-pass scanner
    (`valid.go`, `scan.go`, `token.go`, `doc.go`);
  - `cmd/qi-json-gen/` — the optional code generator (stdlib only).
- New packages, files beyond the current layout, exported identifiers, or
  dependencies require an amendment to this constitution.

## Git and Review Discipline

The repository uses a two-branch integration model. CI
(`.github/workflows/ci.yml`) enforces these rules where it technically can.

### Branching model

- `main` is the production-ready mainline.
- `dev` is the integration branch.
- Feature and task branches MUST be created from the current `main`, after
  `git fetch` and a fast-forward update.
- Feature branches MUST target `dev`.
- Only the `dev` → `main` pull request is allowed to merge into `main`.
- Branch names MUST strictly follow Conventional Branch
  (https://conventionalbranch.org/): a type (`feature`/`feat`, `bugfix`/`fix`,
  `hotfix`, `release` or `chore`) or an AI-agent prefix (`ai`, `copilot`,
  `cursor`, `claude`, `codex`), then a short kebab-case summary.

### Commits

- Commit messages MUST strictly follow Conventional Commits
  (https://www.conventionalcommits.org/) and carry a full body: what changed,
  why, and any non-obvious decision.

### Pull requests

- If the PR closes an issue, its description MUST start with
  `Closes: #[issue-number]`.
- The description MUST contain a concise problem statement and a solution
  summary.
- Suggested labels MUST be proposed and confirmed with the user before applying.
- Feature PRs target `dev`; only the integration PR targets `main`.

### CI enforcement

CI MUST enforce the branch-name, target-branch and commit-message rules and the
Verification Workflow below where technically possible. The workflow in
`.github/workflows/ci.yml` runs the verification sequence and checks the source
branch name, the target branch and the commit messages of each pull request.

## Rigorous Code Quality

All Go code MUST follow the Uber Go Style Guide
(https://github.com/uber-go/guide/blob/master/style.md) without exception. In
addition:

- Errors MUST be wrapped with context using `fmt.Errorf("...: %w", err)`.
- Bare errors MUST NOT be returned from deeper layers.
- Sentinel errors MUST be defined in the package that owns the surface where
  callers need to inspect them: the `qijson` module root or `scanner`. There is
  no separate `core` package; this repository's surface is fixed by Article VI.
- Public APIs MUST document ownership, concurrency safety, and lifecycle
  expectations.
- Goroutines MUST have clear ownership and shutdown paths. Leaks are defects.
- Context propagation MUST be explicit. Blocking operations MUST accept
  `context.Context`.
- No panics in library code for recoverable error conditions.

The library is synchronous today: it starts no goroutines and no exported
operation blocks, so the goroutine and context rules bind only when such code is
added. Where the current code does not yet meet a rule, the gap is a refactoring
task tracked in `.opencode/plan/2026-11-22-git-and-code-quality-reglament.md`.

## Governance

This constitution supersedes ad-hoc workflow preferences. Amendments update this
file and `AGENTS.md` together. If code and tests disagree, resolve the
discrepancy against `docs/task.md`, then update both.

**Version**: 3.5.0 | **Ratified**: 2026-11-21 | **Last Amended**: 2026-11-22
