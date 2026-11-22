# qi-json — codegen for same-package named `string`/`bool` types

**Date**: 2026-11-21
**Related**: `docs/constitution.md`, `AGENTS.md`,
`.opencode/plan/2026-11-21-json-codegen.md` §8 (candidate),
`.opencode/plan/2026-11-21-post-decoder-backlog.md` §9,
`.opencode/plan/2026-11-21-json-encoder.md` §10 (the narrowed README claim).

This file is a design record. Task state lives in `docs/tasks.md`.

---

## 1. Why

`resolveBasic` recognises only numeric underlyings (plus `byte`/`rune`), so a
same-package named type whose underlying type is `string` or `bool` (for example
`type Status string`) falls through `identCodec`/`starCodec`/`elemCodec` to a
precise `unsupported type` error. The cross-package path already supports named
string/bool through `go/types`; the same-package path should match it using the
AST, with no new dependency. The README was narrowed to match the gap; this
closes it.

## 2. Design

- **Resolution.** Add `stringOrBoolUnderlying`/`resolveStringOrBool` to `gen.go`,
  walking same-package named types (cycle-guarded) to their underlying `string`
  or `bool`. Keep `resolveBasic` numeric-only so the two dispatches stay disjoint.
- **Field codec.** Generalise the existing cross-package `crossScalarClosure`
  into `scalarClosure` in `gen.go` (it is now shared; it derives the read/name
  from the kind and sets `needScanner` itself). `identCodec` and `starCodec`
  call it for a same-package named `string`/`bool`; `crossScalarCodec` calls it
  too, dropping its duplicated read/name strings.
- **Element codec.** `elemCodec` emits a `qiDecode<Type>`/`qiEncode<Type>` pair
  from shared `namedScalarDecodeBody`/`namedScalarEncodeBody` free functions,
  which `crossElemBody`/`crossElemEncodeBody` also use for their string/bool
  branches.
- **No new exported identifier** (generated code only); no amendment.

## 3. Subtasks (TDD)

- **N1 — tests (red).** `gen_test.go`: a same-package named `string`/`bool`
  fixture (value, pointer, slice, map; a named-through-named chain); assert the
  conversions, the `qiDecode*`/`qiEncode*` pairs, and the `scanner` import. An
  error-table case for a named type whose underlying is unsupported.
- **N2 — implementation (green).** `stringOrBoolUnderlying`, `scalarClosure`,
  `scalarHelper`/`scalarEncodeHelper`, `namedScalar*Body`; wire the three call
  sites and refactor the cross-package path onto the shared helpers.
- **N3 — fixture.** Add `Status string`/`Mode bool` and fields to `gentest`'s
  `Config`; regenerate `zz_qi_json_gen.go`; extend the decode test and the encode
  golden.
- **N4 — docs + verify.** `README.md`, `main.go` doc, `CHANGELOG.md`; mark the
  candidate done in the codegen plan §8, the backlog §9, and the encoder plan
  §10. `make verify`, `-race`, drift guard green.

**Acceptance**: a same-package named `string`/`bool` field (value, pointer, slice,
map) generates and round-trips; unsupported named underlyings still error
precisely; drift guard green; `make verify` and `-race` green.

## 4. Results (done, 2026-11-21)

- **Shipped**: `cmd/qi-json-gen/gen.go` (`stringOrBoolUnderlying`/
  `resolveStringOrBool`, `scalarClosure` — moved and generalised from the
  cross-package `crossScalarClosure`, `scalarHelper`/`scalarEncodeHelper`,
  `namedScalarDecodeBody`/`namedScalarEncodeBody`, and the `identCodec`/
  `starCodec`/`elemCodec` branches); `cmd/qi-json-gen/types.go` now delegates its
  string/bool field and element paths to the shared helpers, dropping the
  duplicated read/name strings.
- **Fixture**: `internal/gentest` adds `Status string`/`Mode bool` and `Config`
  fields (`status`, `prev`, `statuses`, `modes`); the checked-in
  `zz_qi_json_gen.go` is regenerated and the decode/encode tests cover them.
- **Tests**: `TestGenerateNamedStringBool` (value, pointer, slice, map, and a
  named-through-named chain) plus a named unsupported-underlying case in
  `TestGenerateErrors`; the fixture decode test and encode golden are extended.
- **Docs**: `README.md`, `cmd/qi-json-gen/main.go`, `CHANGELOG.md`, `AGENTS.md`;
  the codegen plan §2/§4/§7/§8, the backlog §1/§9, and the encoder plan §10 mark
  the candidate done.
- **Verification**: `make verify` green (0 lint issues); `go test -race ./...`
  green; the drift guard passes and `go generate ./...` is idempotent. No decoder
  or scanner change, so no fuzz run is required.

