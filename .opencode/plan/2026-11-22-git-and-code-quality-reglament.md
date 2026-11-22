# qi-json — Git and Code Quality Reglament (governance + audit)

**Date**: 2026-11-22
**Status**: governance added; audit recorded; **T-Q2 done** (2026-11-22);
refactoring tasks T-Q1, T-Q3 and T-Q4 open — **the next task is T-Q1** (pin the
error-wrapping policy).
**Related**: `docs/constitution.md` (§ Git and Review Discipline, § Rigorous Code
Quality), `AGENTS.md`, `README.md`, `.github/workflows/ci.yml`,
`.opencode/plan/2026-11-21-post-decoder-backlog.md` §9.

---

## 1. Context

The project adopted a Git and review discipline and a Rigorous Code Quality
block (constitution v3.5.0). This change adds the rules and the CI that enforces
the mechanically checkable parts; it is documentation and CI only, so the Go
module is unchanged. This plan records:

- where the rules live;
- a line-by-line audit of the current code against them;
- the refactoring tasks needed to close the gaps (TDD, per Article II).

## 2. What was added

- `docs/constitution.md` — two new sections:
  - **Git and Review Discipline**: two-branch model (`main` production, `dev`
    integration; feature branches from current `main` target `dev`; only
    `dev` → `main` may merge into `main`), Conventional Branch names,
    Conventional Commits with a full body, PR conventions, and the CI
    enforcement clause.
  - **Rigorous Code Quality**: Uber Go Style Guide plus error wrapping, sentinel
    placement, public-API documentation, goroutine ownership/shutdown, context
    propagation, and no panics in library code. The `core` package of the source
    text was adapted to this repository's owning packages (`qijson` root or
    `scanner`) because there is no `core` package; a new package would be an
    Article VI amendment.
- `AGENTS.md` — the same rules, condensed for agents.
- `README.md` — a "Contributing" section for humans.
- `.github/workflows/ci.yml` — CI: the verification sequence (test → gofmt → vet
  → golangci-lint, mirroring `make verify`), plus Conventional Branch,
  target-branch (`main` only from `dev`) and Conventional Commits checks.

## 3. Audit against Rigorous Code Quality

| Rule | Status | Evidence / gap |
|---|---|---|
| Uber Go Style Guide | ✅ (light review pending) | All exported identifiers are documented; error strings lowercase after the `qijson:` prefix; no `goto`/labels in the library. A dedicated style pass is T-Q4. |
| Errors wrapped with `%w`; no bare errors from deeper layers | ⚠️ | Public errors are the typed `*qijson.Error` (path + offset) returned unwrapped through `Decoder`/`Encoder`, and the `Writer` sentinels bubble up through `Schema.Encode`/`Marshal`/`Encode` unwrapped. See T-Q1 for the policy decision and the inspectability test. |
| Sentinel errors in the owning package | ✅ | `errNilTarget` (`unmarshal.go:26`), `errNilWriter` (`encode.go:22`), the seven `writer.go` sentinels, `errBadEscape`/`errInvalidUTF8` (`errors.go`) and the element-constructor guards live in `qijson`. T-Q2 (done) defined `errNilElementDecoder`/`errNilElementEncoder`/`errUnsupportedDynamicType` (`field.go`) and `errNoFieldEncoder` (`schema.go`) once each and routes them through `fmt.Errorf("...: %w", ...)`, so no `errors.New` remains inside a function body. |
| Public APIs document ownership, concurrency, lifecycle | ⚠️ | `Reader`, `Writer`, `Tokenizer`, `Valid` already document concurrency and no-copy/aliasing. Gaps: `Schema[T]` (mutating `RejectUnknown`/`RejectDuplicate` vs. concurrent use), `Field[T]`, `Codec[E]`/`NewCodec`, `Error`/`TypeError`/`ErrorKind`, and the ownership/lifetime of `data` in `Decode`/`Unmarshal*` and of the bytes returned by `Marshal`/`Writer.Bytes`. See T-Q3. |
| Goroutines: ownership and shutdown; no leaks | ✅ N/A | The library starts no goroutines (grep for `go func`/`sync.` is empty). |
| Context propagation; blocking ops accept `context.Context` | ✅ N/A | No exported operation blocks; all work is over in-memory `[]byte`. |
| No panics in library code | ✅ | No `panic(` outside tests. `cmd/qi-json-gen` is a `main` package and uses `os.Exit`. |

## 4. Refactoring tasks

All tasks are TDD (failing test first, minimal change, refactor) and finish with
the mandated verification (`make verify`, plus `-race`). None adds an exported
identifier, so no Article VI amendment is required; if a task decides to export a
sentinel, that decision needs its own amendment.

### T-Q2 — make every library sentinel inspectable (start here)

**Done (2026-11-22).** Results below.

- Define unexported package sentinels for the element-constructor guards:
  `errNilElementDecoder`, `errNilElementEncoder`, and replace the inline
  `errors.New` calls at `field.go:287/311/344/368/405/432`.
- Decide on `schema.go:98` (`field %q has no encoder`) and `field.go:613`
  (`cannot encode %T as JSON`): either promote each to an unexported sentinel
  used with `fmt.Errorf("...: %w", ...)`, or document why it stays a one-off.
- Tests: `errors.Is` on each guard path (in-package tests).
- **Acceptance**: no `errors.New` inside a function body in the library; every
  returned sentinel is defined once and is `errors.Is`-testable.

#### T-Q2 results

- **Shipped**: `field.go` defines `errNilElementDecoder`, `errNilElementEncoder`
  and `errUnsupportedDynamicType` once each (a commented `var` block); the six
  inline `errors.New` calls in `Slice`/`Map`/`Array` now return the sentinels,
  and `encodeAny` wraps the dynamic-type one:
  `fmt.Errorf("qijson: cannot encode %T as JSON: %w", v, errUnsupportedDynamicType)`.
  `schema.go` defines `errNoFieldEncoder` and wraps it:
  `fmt.Errorf("qijson: field %q: %w", f.Name, errNoFieldEncoder)` (the message
  reads `qijson: field "x": no encoder` — the colon form the rule prescribes).
- **Decision** (the plan's "or document why": both are promoted, not one-offs):
  a `nil` `Codec` direction and a `Field` without an `Encode` are reachable by
  hand-building a container/codec, so both are now inspectable. The only
  non-sentinel library error left is the `*qijson.Error` path error, which is
  itself the context (T-Q1).
- **Tests**: new `sentinel_test.go` — `TestDecodeNilElementDecoder` (Slice, Map,
  Array; nil containers still decode), `TestEncodeFieldWithoutEncoder`,
  `TestEncodeAnyUnsupportedDynamicType`; `TestEncodeNilElementEncoder` upgraded
  from `err != nil` to `errors.Is(err, errNilElementEncoder)` for the slice, map
  and array paths.
- **Verification**: `make verify` green (0 lint issues); `go test -race ./...`
  green. No exported identifier added, so no Article VI amendment.

### T-Q1 — pin the error-wrapping policy

- Record the decision in `docs/constitution.md` / `doc.go`: the typed
  `*qijson.Error` *is* the contextual error (it carries path + offset), so it is
  returned unwrapped; the rule's "wrap with `%w`" applies where a layer adds new
  context, which today is nowhere. If a reviewer prefers literal `%w` wrapping
  at the public boundary, that is a behavior change (error strings change) and
  every asserting test must be updated deliberately.
- Tests: `errors.As(err, **qijson.Error)` through `Unmarshal`, `Decode`,
  `Marshal` and `Encode`; `errors.Is` through a nested `ObjectDecoder`.
- **Acceptance**: a test proves the guarantees (`errors.Is`/`errors.As`) hold end
  to end, and the policy is written down where a contributor will find it.

### T-Q3 — public-API documentation pass (ownership, concurrency, lifecycle)

- `Schema[T]`: state that a `Schema` is immutable once built and safe for
  concurrent decode/encode, but `RejectUnknown`/`RejectDuplicate` mutate it and
  must not race with use (finish building it before sharing).
- `Field[T]`, `Codec[E]`/`NewCodec`: state that they are plain value + closures,
  with the concurrency of the captured target.
- `Error`/`TypeError`/`ErrorKind`: state that a returned `*Error` is immutable.
- Entry points: state that `data` is not retained after the call, that `v` is
  written in place, and that the `[]byte` from `Marshal` is owned by the caller.
- `scanner`: confirm `NewTokenizer` documents that the returned tokenizer aliases
  `data`, which the caller must keep alive and unmodified.
- **Acceptance**: every exported identifier has a sentence covering ownership,
  concurrency and/or lifecycle as applicable; `golangci-lint` stays green.

### T-Q4 — Uber Go Style Guide review pass

- Walk the three packages against the guide (naming, comment style, error
  handling, slice/map pre-sizing, early returns, `context` placement). Record
  findings and fix the real ones; this is expected to be small.
- Keep the linter policy as-is (default linters, no repo config) unless a rule
  cannot be upheld by review alone; adding `wrapcheck`/`revive` would need an
  `AGENTS.md`/constitution change and is its own decision.
- **Acceptance**: findings recorded in this plan; fixes verified.

## 5. Not code — repository settings

GitHub branch protection cannot be enforced by a workflow file:

- Protect `main`: require a pull request, require the `verify` status check, and
  restrict who may merge (only the `dev` → `main` PR).
- Protect `dev`: require the `verify` status check.

Record the settings change in the PR description when it is applied.

## 6. Verification

Governance change (this one): `make verify` green (docs/CI only), YAML of
`.github/workflows/ci.yml` parsed. Each refactoring task (T-Q1…T-Q4):
`make verify` in order, plus `go test -race ./...`. Results are appended here as
each task completes.
