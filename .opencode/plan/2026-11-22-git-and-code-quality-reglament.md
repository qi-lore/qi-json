# qi-json — Git and Code Quality Reglament (governance + audit)

**Date**: 2026-11-22
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

**Done (2026-11-22).** Results below.

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

#### T-Q1 results

- **Policy recorded**: `doc.go` gains an errors paragraph; `docs/constitution.md`
  (v3.7.0, "Rigorous Code Quality") states that the typed `*qijson.Error` is the
  context and is returned unwrapped, with `%w` reserved for a layer that adds new
  context (a sentinel cause) and a boundary `%w` a deliberate behavior change;
  `AGENTS.md` mirrors the one-line rule. No exported identifier changed.
- **Tests**: new `error_policy_test.go` — `TestDecodeErrorPolicy` pins
  `errors.As(*Error)` (and the `*TypeError` cause) through `Unmarshal`, `Decode`,
  `Schema.Unmarshal` and `Schema.UnmarshalInto`, plus one exact error string
  proving the `*Error` is not double-wrapped; `TestDecodeSyntaxErrorIsTyped` pins
  a syntax failure; `TestEncodeErrorPolicy` pins `errors.Is` for the encode
  sentinels (`errNonFinite`, `errIncomplete`, `errNilWriter`, `errNilTarget`)
  through `Marshal`, `Encode` and `Schema.Marshal`; `TestErrorIsThroughObjectDecoder`
  proves `errors.Is(err, errInvalidUTF8)` and `errors.As(*TypeError)` across a
  nested `ObjectDecoder` delegation with the nested path.
- **Note**: T-Q1 is documentation plus characterization — the guarantees already
  held, so there was no behavior change to drive red-first; the tests pin the
  existing contract (per Article II, only a behavior change needs a red test).
- **Verification**: `make verify` green (0 lint issues); `go test -race ./...`
  green.

### T-Q3 — public-API documentation pass (ownership, concurrency, lifecycle)

**Done (2026-11-22).** Results below.

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

#### T-Q3 results

- **Shipped** (doc comments only, no code change):
  - `schema.go` — `Schema[T]` states it is immutable once built and safe for
    concurrent use, with `RejectUnknown`/`RejectDuplicate` mutating it and not
    racing with use; both setters now say they mutate `s`; `Marshal`,
    `Unmarshal` and `UnmarshalInto` state the returned `[]byte` is caller-owned
    and that `data` is not retained.
  - `field.go` — `Field[T]` states it is a plain value of closures, shareable
    exactly when its captured target is not shared.
  - `encode.go` — `Marshal` (caller-owned `[]byte`), `Encode` (writes only to `w`,
    does not retain `v`), `Codec[E]` and `NewCodec` (plain function pair, whose
    concurrency is the closures').
  - `errors.go` — `*Error` and `*TypeError` returned by the package are
    immutable.
  - `writer.go` — `Writer` is not safe for concurrent use; `Bytes` aliases the
    internal buffer and is valid only until the next write or reset (clone to
    keep it).
  - `unmarshal.go` — `Unmarshal`, `UnmarshalInto` and `Decode` state `v` is
    written in place and `data` is not retained.
  - `scanner/token.go` — `NewTokenizer` states the tokenizer aliases `data`
    (keep it alive and unmodified) and is not safe for concurrent use; the
    package doc already carried the equivalent notes.
- **Already present** (audited, left as is): `Reader` documents no-copy and
  not-safe-for-concurrent-use; `scanner.Valid` documents concurrent safety;
  the `scanner` package doc documents aliasing and the no-copy rule.
- **Not applicable**: `ErrorKind` and its constants are plain value types with
  no resource, so they carry no ownership sentence.
- **Verification**: `make verify` green (0 lint issues); `go test -race ./...`
  green.

### T-Q4 — Uber Go Style Guide review pass

**Done (2026-11-22).** Results below.

- Walk the three packages against the guide (naming, comment style, error
  handling, slice/map pre-sizing, early returns, `context` placement). Record
  findings and fix the real ones; this is expected to be small.
- Keep the linter policy as-is (default linters, no repo config) unless a rule
  cannot be upheld by review alone; adding `wrapcheck`/`revive` would need an
  `AGENTS.md`/constitution change and is its own decision.
- **Acceptance**: findings recorded in this plan; fixes verified.

#### T-Q4 results

A line-by-line walk of the library and the generator against the guide (naming,
comment style, error handling, slice/map pre-sizing, early returns, interface
compliance). The code was already close to the guide; the real deviations and
their fixes:

- **Omit zero-value struct fields** (`internal/codec/writer.go`): `BeginArray`
  pushed `writerFrame{object: false}`; the explicit zero field is dropped
  (`writerFrame{}`), per the guide's "Omit Zero Value Fields in Structs".
- **Prefer `&T{}` over `new(T)`** (`cmd/qi-json-gen/foreign.go`): the synthesized
  schema for a pointer to a foreign struct emitted `e := new(T)`; it now emits
  `e := &T{}`. The checked-in fixture (`zz_qi_json_gen.go`) was regenerated with
  `go generate ./...`, so the drift guard stays green. The `new(S)` in
  `codec.ObjectDecoderPtr` stays: `S` is an unconstrained type parameter, where
  `&S{}` is not legal.
- **Verify interface compliance** (`internal/codec/errors.go`,
  `internal/scanner/scan.go`): added compile-time assertions
  `var _ error = (*Error)(nil)`, `var _ error = (*TypeError)(nil)` and
  `var _ error = parseError{}`, so a dropped `error` method fails the build.
- **Reduce nesting** (`cmd/qi-json-gen/foreign.go`): three
  `if cond { return … } else if !cond { return err }` blocks became two flat
  `if`s after the first branch returns.
- **Prefer specifying container capacity** (`cmd/qi-json-gen/gen.go`,
  `foreign.go`): `seen`, the two `used` member-name sets and the per-field
  `[]codec` are now pre-sized from a known length.

Reviewed and deliberately left as they are:

- **Prefix Unexported Globals with `_`** — the one guide rule not applied.
  Renaming `nestingInline`, `spaceTable`, `stringStop`, `compsInline`,
  `hexDigits`, `marker`, `defaultOutput`, `rangeHelper`, `qijsonPath` and
  `qijsonEnginePath` would also force the generator to emit `_`-prefixed schema
  vars and rename the checked-in generated and hand-written fixtures. The
  maintainer chose to keep the descriptive names; recorded here as an accepted
  deviation.
- **Error-string capitalization** — messages such as `qijson: nil Writer` and
  `qijson: Key called outside an object` capitalize exported API names
  (`Writer`, `Key`, `Begin`, `End`). The Uber guide does not require lowercase
  error strings (that guidance is Go Code Review Comments), and the strings are
  pinned by tests; unchanged.
- **Enum zero values** — `KindNull` and `KindSyntax` are the zero value of their
  enums, which the guide permits when the zero value is the meaningful default.
- **Line length** — the only lines over the 99-column soft limit are embedded
  source templates in the generator, and the guide allows exceeding the soft
  limit.
- **Sorting idiom** — `cmd/qi-json-gen` uses `sort`, `internal/codec` uses
  `slices`; each package is internally consistent, which the guide's
  package-level "Be Consistent" rule asks for.

**Verification**: `make verify` green (tests, `gofmt`, `go vet`, `golangci-lint`
0 issues) and `go test -race ./...` green. No exported identifier changed, so no
Article VI amendment.

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
