# qi-json — user scenarios

**Date**: 2026-11-22
**Status**: recorded. The third-party struct-field gap was a **release blocker**,
closed in **v0.2.0** (`.opencode/plan/2026-11-22-foreign-struct-fields-impl.md`);
the research that drove it is `.opencode/plan/2026-11-22-third-party-struct-fields.md`.
**D3**, the Go-version floor mismatch (§2.5), was the next task and is now
**done (2026-11-22)**, shipping in **v0.2.1**, per
`.opencode/plan/2026-11-22-go-version-floor.md`. All recorded gaps are closed;
the code-quality refactoring **T-Q2** is done (2026-11-22), **T-Q1** is done
(2026-11-22), **T-Q3** is done (2026-11-22), and the next task is **T-Q4**
(`.opencode/plan/2026-11-22-git-and-code-quality-reglament.md`).
**Related**: `docs/constitution.md`, `README.md`,
`.opencode/plan/2026-11-21-post-decoder-backlog.md` §9.

## 1. Purpose

End-to-end walkthroughs of what a Go developer who imports `qi-json` actually
experiences. Every finding below was **run against the code**, not recalled:
commands and outputs are reproduced in §5. The point is to find where the
reflection-free design makes a common task impossible or painful, and to turn
that into tasks.

Scenarios are written from the user's point of view: what they ask, then what
the library does back.

---

## 2. Scenario 1 — a `go 1.23.0` consumer imports the library

### 2.1 Question

> "I want to import the library into my project. My project's `go.mod` specifies
> the Go version as `go 1.23.0`. What happens next?"

### 2.2 Setup

`qi-json`'s own `go.mod` is:

```
module qi-json

go 1.27.3
```

The consumer, per the README, adds a `require` + `replace` (the module path
`qi-json` has no domain and there is no proxy, so `go get` cannot fetch it):

```
module consumer

go 1.23.0

require qi-json v0.0.0

replace qi-json => ../qi-json
```

### 2.3 What happens

Since Go 1.21 a dependency's `go` directive is a **hard minimum**, so the outcome
depends on the consumer's toolchain:

- **Toolchain ≥ 1.27.3, default `GOTOOLCHAIN=auto`:** `go build ./...` stops with
  `go: updates to go.mod needed; to update it: go mod tidy`. Running
  `go mod tidy` **silently rewrites the consumer's `go 1.23.0` to `go 1.27.3`**,
  and the build then succeeds (no `go.sum` is produced for a local `replace`).
- **A genuine 1.23 toolchain** (`GOTOOLCHAIN=go1.23.0`, or `GOTOOLCHAIN=local`
  with a 1.23 toolchain): the build **fails** with
  `go: module .../qi-json requires go >= 1.27.3 (running go 1.23.0; ...)`.
  With `GOTOOLCHAIN=auto` it instead downloads and switches to go1.27.3.

### 2.4 The gap

The repository documents a **Go >= 1.23** floor (`docs/constitution.md`
Additional Constraints) and the README states no Go requirement at all, but the
**enforced** floor is **1.27.3**, set by the `go` directive. Stated and enforced
support disagree by four minor versions; a 1.23 consumer is either silently
upgraded or hard-blocked.

A throwaway copy set to `go 1.23.0` + `toolchain go1.27.3` built and passed
`go test ./...`, so the source is genuinely 1.23-language-compatible and
`go 1.27.3` over-constrains consumers.

### 2.5 Decision

Recorded as a **separate open issue** (D3): candidate fix is
`go 1.23.0` + `toolchain go1.27.3` in `go.mod`, or correcting the documented
floor. **Originally** not the next task; the struct-field blocker closed in
v0.2.0, so **D3 became the next task**, and it is now **done (2026-11-22)** —
the candidate fix was applied (`go 1.23.0` + `toolchain go1.27.3`), so the
enforced floor matches the documented one. See
`.opencode/plan/2026-11-22-go-version-floor.md`.

---

## 3. Scenario 2 — deserialize a `[]byte` body into a struct

### 3.1 Question 2a — the basic flow

> "I imported `qi-json` already. My HTTP API gives me `body []byte` with
> JSON-encoded info. I need to deserialize it into
> `type MyType struct { Field1 someType; … }`. What is my next action?"

**Answer: the next action is a compile error, then add a `Decoder`.** `qijson`
has no reflection, so there is no `json.Unmarshal(body, &v)` that works from the
struct alone. Both natural attempts fail to compile:

```
qijson.Decode(body, &v)
// cannot use &v (value of type *MyType) as qijson.Decoder value:
// *MyType does not implement qijson.Decoder (missing method DecodeJSON)

qijson.Unmarshal[MyType](body)
// *MyType does not satisfy interface{*MyType; qijson.Decoder}
// (missing method DecodeJSON)
```

The user must make `*MyType` satisfy `qijson.Decoder` by binding a `Schema`
(compile-time accessor closures), then the `body []byte` call works in either
form:

```go
var v MyType
err := qijson.Decode(body, &v)            // in place, no type argument
v, err := qijson.Unmarshal[MyType](body)  // fresh value
```

HTTP-body edge cases, verified: a trailing newline and leading spaces are fine;
an empty body, two concatenated documents, a top-level `null`, and a type
mismatch are all hard `*qijson.Error`s (see §5).

### 3.2 Question 2b — explanation of the tags remark

> "No, don't focus on tags. More common scope: I need to deserialize `[]byte`
> into my struct."

**Recorded as the framing of this scenario.** Tags are only the *input* to the
code generator; the real requirement is "bytes → struct", which is gated on the
`Decoder` requirement in §3.1.

### 3.3 Question 2c — 40 fields: must the user write all 40 instructions?

> "So if `MyType` has 40 fields — the user must write `qijson.Struct[MyType]`
> with instructions for 40 fields?"

**Answer: no.** Hand-writing the 40 closures is the fallback. The intended path
is `cmd/qi-json-gen`: the user writes the 40 tagged fields plus one
`//go:generate` directive, and the generator emits the whole schema and both
methods. Verified: a 46-line source produced a 58-line `zz_qi_json_gen.go` that
decoded correctly, and `go generate ./...` was idempotent (same md5 before/after).

The trade is **40 declarative fields + 1 directive** versus **40 hand-written
closures**. Tags are just how the generator learns each JSON member name
(without a tag it uses the field name).

### 3.4 Question 2d — struct fields from the same package vs another package

> "What if `MyType` looks like
> `type MyType struct { Field1 self_package.StructType; Field1 other_third-party-package.StructType }`?"

**Case A — a struct from the same package: supported.** The generator emits
`qijson.Object[MyType, StructType](...)`, but the nested struct must also be
generated (`-type=MyType,StructType`, or mark it `//qi-json:generate`); otherwise
`nested struct StructType is not generated; add -type=StructType`.

**Case B — a struct from another (third-party) package: NOT supported.**
Verified:

```
qi-json-gen: MyType.B: unsupported cross-package type thirdparty.Thing
             (only basic scalar underlying types are supported)
```

Cross-package **scalars** are fine (`time.Duration`, a named string/bool/int from
another package). Cross-package **structs** are out of scope for the generator —
even if that struct was itself built with `qi-json` and has a `DecodeJSON`; there
is no cross-package schema discovery.

A hand-written schema can bind a foreign struct's **exported** fields (verified:
it decodes), but with real limits:

- **Unexported fields are unreachable** — `v.secret undefined (cannot refer to
  unexported field secret)`. Members matching them are silently ignored.
- **Only canonical-width fields get one-liner constructors.** `String`/`Bool`/
  `Int`/`Float` bind only `*string`/`*bool`/`*int64`/`*float64`. A foreign struct
  with `int`, `int32`, `uint16`, … forces a full `qijson.Field[T]{Decode, Encode,
  Empty}` closure with a range check (as the generator emits), because
  `qijson.Int` is `*int64`-only: `cannot use func(...) *int … as func(...) *int64`.
- **No "decode via nested `Decoder`" constructor.** The exported constructors are
  `String`, `StringPtr`, `Int`, `IntPtr`, `Float`, `FloatPtr`, `Bool`, `BoolPtr`,
  `Object`, `Slice`, `Map`, `Array`, `Any`, `Raw` — none wraps a nested type's
  `DecodeJSON`. Even a `qijson`-aware foreign type has no one-liner; the caller
  writes a raw `Field[T]` closure that calls `DecodeJSON`.

### 3.5 The gap (release blocker, closed in v0.2.0)

A struct field whose type is a **struct defined in another package** is the worst
case for the library:

- the code generator — which handles 40 flat fields effortlessly — **cannot emit
  it**;
- the manual fallback works only for exported fields, needs full closures for
  non-canonical numeric widths, and has no helper for delegating to a nested
  `Decoder`.

Using a struct from another package as a field is a common Go pattern across the
whole ecosystem. Without support the library is not usable for many real types;
this gap was closed in **v0.2.0** (see §3.6 and §4).

### 3.6 Decision

**D1** — the third-party struct-field gap was a **release blocker**. Research
(`.opencode/plan/2026-11-22-third-party-struct-fields.md`) led to the layered fix,
and the implementation (`.opencode/plan/2026-11-22-foreign-struct-fields-impl.md`)
**closed the blocker in v0.2.0**. The next task was **D3** (§4), now **done**.

---

## 4. Decisions

- **D1 (release blocker — closed).** Struct fields whose type is a struct from
  another package must be supported. Research task opened; see
  `.opencode/plan/2026-11-22-third-party-struct-fields.md`. **Closed in v0.2.0**
  (implementation `.opencode/plan/2026-11-22-foreign-struct-fields-impl.md`).
- **D2 (research constraints).** Any fix must preserve: no `reflect`, no
  `unsafe`, no `encoding/json`; the code generator stays stdlib-only,
  deterministic and drift-guarded; the decoder and encoder stay symmetric; new
  exported identifiers need a constitution amendment.
- **D3 (Go-version floor — done 2026-11-22).** `docs/constitution.md` documented
  a `Go >= 1.23` floor but `go.mod` declared `go 1.27.3`. **Fixed** with
  `go 1.23.0` + `toolchain go1.27.3`, so the enforced floor matches the stated
  one. Plan and results: `.opencode/plan/2026-11-22-go-version-floor.md`.
- **D4 (baseline + handoff).** This session recorded the scenarios at the
  **v0.1.1** baseline and opened the research task; the struct work shipped in
  **v0.2.0**, the Go-floor fix in **v0.2.1** and the release-flow documentation
  in **v0.2.2**, so the current baseline is now **v0.2.2**.

---

## 5. Evidence log (reproduction)

Environment: Apple M1, go1.27.3, module `qi-json` at `go 1.27.3`.

### 5.1 Scenario 1

| Step | Command | Result |
|---|---|---|
| build with `go 1.23.0` consumer | `go build ./...` | `go: updates to go.mod needed; to update it: go mod tidy` |
| tidy | `go mod tidy` | rewrites consumer `go` to `1.27.3`; no `go.sum`; build then succeeds |
| real 1.23 toolchain | `GOTOOLCHAIN=go1.23.0 go build ./...` | `go: module …/qi-json requires go >= 1.27.3 (running go 1.23.0; GOTOOLCHAIN=go1.23.0)` |
| library at 1.23 language | copy with `go 1.23.0` + `toolchain go1.27.3` | `go build ./...` exit 0; `go test ./...` exit 0 |

### 5.2 Scenario 2 — entry points

```
qijson.Decode(body, &v)           -> *MyType does not implement qijson.Decoder (missing method DecodeJSON)
qijson.Unmarshal[MyType](body)    -> *MyType does not satisfy interface{*MyType; qijson.Decoder}
```

With a hand-written `Schema` + `DecodeJSON`, both print `{Field1:x Field2:7}`.

### 5.3 Scenario 2 — `body` edge cases

| body | result |
|---|---|
| `{...}\n` | OK (trailing whitespace allowed) |
| `  {...}` | OK (leading whitespace allowed) |
| `""` (empty) | `qijson: $: unexpected end of JSON input (at offset 0)` |
| `{...}{...}` | `qijson: $: invalid character '{' after top-level value (at offset 14)` |
| `null` | `qijson: $: expected object, found null (at offset 0)` |
| `{"Field1":42}` | `qijson: $.Field1: expected string, found number (at offset 10)` |

### 5.4 Scenario 2 — codegen, 40 fields

- 46-line source (40 tagged fields + directive) → 58-line `zz_qi_json_gen.go`.
- Decoded: `Field1="x" Field2=7 Field5=[a] Field6=map[k:9] Field40=true`.
- `go generate ./...` idempotent (md5 unchanged).

### 5.5 Scenario 2 — foreign struct field

| Attempt | Result |
|---|---|
| generator with a same-package nested struct omitted | `nested struct LocalStruct is not generated; add -type=LocalStruct` |
| generator with a third-party struct field | `unsupported cross-package type thirdparty.Thing (only basic scalar underlying types are supported)` |
| hand-written schema, exported fields only | decodes: `A={X:1 Y:hi} B={Name:n Count:9 secret:}` |
| binding an unexported foreign field | `v.secret undefined (cannot refer to unexported field secret)` |
| `qijson.Int` on an `int` field | `cannot use func(v *LocalStruct) *int … as func(*LocalStruct) *int64 value` |