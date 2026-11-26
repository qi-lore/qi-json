# qi-json Constitution

Short, enforceable rules for this repository. The test suite is the executable
contract; this document governs how changes are made.

## Core Principles

### I. Spec First

The test suite is the authoritative specification. It pins the grammar, the
exact error messages and the 1-based positions; the implementation follows the
contract those tests enforce. Two strict grammar rules are deliberate and must be
preserved: a `0` integer part may not be followed by a digit (`01` is invalid),
and a number allows at most one fraction and one exponent (`1.2.3`, `1e1e2` are
invalid). Rationale: `.opencode/plan/2026-11-21-scanner-rewrite.md`.

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
`AGENTS.md`, "Contract details that are easy to break"); the object-member scanner
is written twice on purpose so it stays inline, because a shared helper costs
about 8% on object-heavy documents.
Deeper documents may allocate the nesting stack once. Prefer the fastest correct
implementation and support structural changes with benchmarks.

### VI. Public Surface

The public surface is reached through exactly one import path,
`github.com/qi-lore/qi-json` (package `qijson`). The syntax validator lives in
the internal package `internal/scanner` as `Valid(data []byte) bool`, matching
`encoding/json.Valid` in name and signature; `scanner.validate` stays unexported
there: it returns the first `parseError` plus whether the input is invalid, so
diagnostics stay testable without widening the API. The facade re-exports
`Valid`, so callers never import the syntax package directly.

Three further surfaces are sanctioned (amended 2026-11-21; the delegation
constructor added 2026-11-22; the scanner moved behind `internal/scanner` and
re-exported 2026-11-22; rationale:
`.opencode/plan/2026-11-21-json-decoder.md`,
`.opencode/plan/2026-11-21-json-codegen.md`,
`.opencode/plan/2026-11-21-json-encoder.md`,
`.opencode/plan/2026-11-22-third-party-struct-fields.md`,
`.opencode/plan/2026-11-22-scanner-consolidation.md`; the decoder/encoder
package was relocated to the module root and renamed `qijson` per
`.opencode/plan/2026-11-21-package-rename.md`):

- **`qijson` syntax re-exports** — the syntax engine's whole former surface,
  reached through the facade: `Valid`, the `Kind` type with its seven constants
  (`KindNull`, `KindBool`, `KindString`, `KindNumber`, `KindObject`, `KindArray`,
  `KindEnd`), and the tokenizing type with `NewTokenizer`, exposing
  `Peek`/`Read*`/`Begin*`/`End*`/`Skip`/`Offset`/`Err`. They are aliases and
  wrappers of `internal/scanner`, so `Reader.Peek`'s `Kind` and generated code's
  `KindNull` comparison stay identical across the boundary and the semantic layer
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

The implementation of the `qijson` decoder and encoder lives in `internal/codec`
(added 2026-11-22; rationale:
`.opencode/plan/2026-11-22-internal-codec-extraction.md`) and the syntax engine in
`internal/scanner` (moved there 2026-11-22; rationale:
`.opencode/plan/2026-11-22-scanner-consolidation.md`). Both are internal
packages, not a public surface: the module root is a thin facade of type aliases
and wrappers, so external code cannot import the engines and their internals may
change without a compatibility promise. The facade MUST re-export the entire
public contract, including the ownership, concurrency and lifecycle
documentation the public types and functions require.

## Verification Workflow (NON-NEGOTIABLE)

Every task MUST finish by running these commands in this order. Stop at the
first failure, fix it, then restart the sequence from step 1:

1. `go test ./...`
2. `gofmt -l .` — must print nothing; fix with `go fmt ./...`
3. `go vet ./...`
4. `golangci-lint run` — v2, default linters, no repo config

A task is not complete until all four pass. Report the commands and their
results in the final summary. `make verify` runs steps 1–4 in order and stops at
the first failure.

Additional commands, used where the task touches the relevant code:

```text
go test -race ./...
go test ./internal/scanner -run '^TestInvalidDocuments$'      # one test or table
go test -run=^$ -fuzz=FuzzValid -fuzztime=10s ./internal/scanner   # -run=^$ is required
go test -run=^$ -bench=. -benchmem ./internal/scanner
go generate ./...                                    # regenerate generator fixtures
```

Performance is a feature: `BenchmarkValid*` MUST NOT regress, and the `Valid`
success path MUST stay allocation-free (`TestValidDoesNotAllocate`).

## Additional Constraints

- Go >= 1.24: the `go.mod` `go` directive is the language floor
  (`go 1.24.0`), required because the facade re-exports generic types through
  generic type aliases; its `toolchain` line (`go1.27.3`) names the toolchain
  used to develop the project and is ignored by consumers.
- Four packages under module `github.com/qi-lore/qi-json` (the module root is a
  package):
  - `.` (module root) — `qijson`, the public facade: it re-exports the semantic
    layer (decode and encode) and the syntax layer (`Valid`, `Kind` and the
    tokenizer);
  - `internal/codec/` — the semantic engine behind that facade; internal, not a
    public surface;
  - `internal/scanner/` — the syntax engine: `Valid` and the tokenizer over the
    single-pass scanner (`valid.go`, `scan.go`, `token.go`, `doc.go`); internal,
    not a public surface;
  - `cmd/qi-json-gen/` — the optional code generator (stdlib only).
- New packages, files beyond the current layout, exported identifiers, or
  dependencies require an amendment to this constitution.

## Git and Review Discipline

The repository uses a two-branch integration model. CI
(`.github/workflows/ci.yml`) enforces these rules where it technically can.

### Branching model

- `main` is the production-ready mainline.
- `dev` is the integration branch, and it carries the newest work: integrated
  changes live there for a long time before a release promotes them to `main`.
- Feature and task branches MUST be created from the current `dev` — not `main`
  — after `git fetch` and a fast-forward update, because the latest changes live
  on `dev` long before they reach `main`, and a branch cut from `main` would
  start from stale code.
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

### Release flow

`dev` accumulates integrated work; a release promotes it to `main`. The steps,
in order:

1. **Choose the version.** Semantic Versioning; the module is pre-1.0, so a
   backward-compatible fix or documentation change is a **patch** and a new or
   changed capability is a **minor**.
2. **Prepare the release on `dev`** (never on a feature branch):
   - promote the `CHANGELOG.md` `[Unreleased]` entry to `[X.Y.Z] - YYYY-MM-DD`
     and reopen an empty `[Unreleased]` section above it;
   - update the `README.md` version block;
   - do the housekeeping the release implies (mark finished items in
     `docs/tasks.md`, refresh baseline references in the plans);
   - commit as `chore(release): vX.Y.Z` with a full body and run the
     Verification Workflow.
3. **Open the `dev` → `main` pull request** — the only pull request allowed to
   merge into `main` — and let CI pass.
4. **Merge into `main`.**
5. **Tag the merge commit on `main`** with `vX.Y.Z` and push the tag; the tag
   marks the released commit (the merge commit, not the preparation commit).
6. **Keep `dev`.** The integration branch is permanent; do not delete it after
   the merge.

### CI enforcement

CI MUST enforce the branch-name, target-branch and commit-message rules and the
Verification Workflow below where technically possible. The workflow in
`.github/workflows/ci.yml` runs the verification sequence and checks the source
branch name, the target branch and the commit messages of each pull request.

## Task Tracking

`docs/tasks.md` is the single source of truth for task state.

- Tasks MUST be recorded only in `docs/tasks.md`.
- Tasks MUST NOT be recorded in files under `.opencode/plan/` or in `AGENTS.md`.
- `docs/tasks.md` MUST be updated before every commit.
- Files under `.opencode/plan/` remain design rationale and history. They MUST
  NOT carry task status or a backlog, and a plan file is not an authority on
  what is done or what is left.
- Every task line uses the fixed format `[X] <PREFIX><NNN> - <SHORT_DESCRIPTION>`
  for completed work and `[ ] <PREFIX><NNN> - <SHORT_DESCRIPTION>` for planned
  work. Identifiers are stable: never renumber and never reuse a retired one.

## Rigorous Code Quality

All Go code MUST follow the Uber Go Style Guide
(https://github.com/uber-go/guide/blob/master/style.md) without exception. In
addition:

- Errors MUST be wrapped with context using `fmt.Errorf("...: %w", err)`.
- Bare errors MUST NOT be returned from deeper layers.
- The typed `*qijson.Error` **is** the context: it carries the JSON path and byte
  offset of the failure and is returned unwrapped through `Decoder`/`Encoder`, so
  a caller reaches it with `errors.As` and its cause with `errors.Is`. The
  `fmt.Errorf("...: %w", err)` rule applies where a layer adds *new* context — for
  example, wrapping a sentinel cause, which is the only wrapping the decoder and
  encoder do today. Adding a literal `%w` at the public boundary instead would
  change error strings and is a deliberate behavior change, not a style tweak.
- Sentinel errors MUST be defined in the package that owns the surface where
  callers need to inspect them: the `qijson` module root or `internal/scanner`. There is
  no separate `core` package; this repository's surface is fixed by Article VI.
- Public APIs MUST document ownership, concurrency safety, and lifecycle
  expectations.
- Goroutines MUST have clear ownership and shutdown paths. Leaks are defects.
- Context propagation MUST be explicit. Blocking operations MUST accept
  `context.Context`.
- No panics in library code for recoverable error conditions.
- Error messages and exported doc comments MUST describe behavior. They MUST NOT
  cite project documents (`docs/`, `.opencode/plan/`, the constitution or the
  README): a consumer of the module does not have the repository, and a failure
  should explain what is wrong, not where to read about it.

The library is synchronous today: it starts no goroutines and no exported
operation blocks, so the goroutine and context rules bind only when such code is
added. Where the current code does not yet meet a rule, the gap is a refactoring
task tracked in `docs/tasks.md`.

## Governance

This constitution supersedes ad-hoc workflow preferences. Amendments update this
file and `AGENTS.md` together. If code and tests disagree, the tests are the
contract: resolve the discrepancy against the tests, then update both.

**Version**: 3.12.0 | **Ratified**: 2026-11-21 | **Last Amended**: 2026-11-26
