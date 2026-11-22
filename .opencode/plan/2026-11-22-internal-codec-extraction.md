# Internal codec extraction

**Status**: proposed — awaiting approval (2026-11-22)
**Branch**: `chore/extract-internal-codec` (from `main`, targets `dev`)
**Related**: `docs/constitution.md` v3.7.0 (Article VI, Additional Constraints);
`.opencode/plan/2026-11-21-package-rename.md`;
`.opencode/plan/2026-11-22-go-version-floor.md`.

## 1. Goal

Move every implementation file of the `qijson` semantic layer into a new
`internal/codec` package, and leave the module root as a thin, documented facade
that re-exports the same public API. External importers keep using `qi-json`
exactly as before; the engine becomes invisible so its internals can change
without a compatibility promise.

**Non-goals**: no scanner change, no generator change, no behavior change, no
error-string/position change, no new public identifiers, no performance change.

## 2. Measured starting point

- 5 packages: `qi-json`, `qi-json/scanner`, `qi-json/cmd/qi-json-gen`,
  `qi-json/cmd/qi-json-gen/internal/gentest`, `.../gentest/xtypes`.
- Root non-test files: `doc.go`, `encode.go`, `errors.go`, `field.go`,
  `reader.go`, `schema.go`, `unmarshal.go`, `writer.go`.
- 15 root test files, all `package qijson`; 9 reference unexported identifiers
  (`errNilTarget`, `errNilWriter`, `errNoFieldEncoder`, `errInvalidUTF8`,
  `decodeAny`, `encodeAny`, `newReader`, `scalar`, ...).
- Unexported sentinels (not public today): `errNilTarget`, `errNilWriter`,
  `errNoFieldEncoder`, `errBadEscape`, `errInvalidUTF8`, `errNilElementDecoder`,
  `errNilElementEncoder`, `errUnsupportedDynamicType`, `errKeyOutsideObject`,
  `errKeyWithoutValue`, `errValueWithoutKey`, `errEndWithoutBegin`,
  `errMultipleRoots`, `errNonFinite`, `errIncomplete`.
- Baseline verification is green (`go test ./... -count=1`, `gofmt -l .`,
  `go vet ./...`, `golangci-lint run` → 0 issues).

## 3. Target layout

```text
qi-json/
├── go.mod
├── doc.go                 # package narrative (unchanged content, one added sentence)
├── qijson.go              # facade: aliases, consts, wrapper funcs
├── internal/
│   └── codec/             # package codec — the engine
│       ├── doc.go         # internal package comment
│       ├── decode.go      # was unmarshal.go (Decoder, Unmarshal, UnmarshalInto, Decode)
│       ├── encode.go      # Encoder, Marshal, Encode, Codec, NewCodec
│       ├── errors.go      # Error, ErrorKind, TypeError, sentinels
│       ├── field.go       # Field[T] and the constructors
│       ├── reader.go      # Reader
│       ├── schema.go      # Schema[T], Struct
│       └── writer.go      # Writer, NewWriter
├── scanner/               # unchanged, stays public at the module root
└── cmd/
    └── qi-json-gen/       # unchanged
```

`scanner/` stays at the module root: `scanner.Valid` is the sanctioned headline
export (Article VI) and generated code imports `qi-json/scanner`. Hiding it
under `internal/` would be a breaking change.

## 4. Invariants (the acceptance bar)

1. The public surface is identical: same identifiers, same signatures, same
   method sets. Guarded by a `go doc -all .` diff against a pre-change snapshot
   and by a new facade contract test.
2. Error strings, error types, `errors.As`/`errors.Is` behavior and byte
   offsets are unchanged.
3. No `encoding/json`, no `reflect`, no `unsafe`; no third-party dependency.
4. `scanner/` is untouched; its tests and benchmarks are untouched.
5. `go generate ./...` produces a byte-identical `cmd/qi-json-gen` fixture set
   (drift guard).
6. Benchmarks stay within noise (aliases and trivial wrappers inline).

## 5. Findings that shape the plan

### 5.1 Generic type aliases need the Go 1.24 language version

`Schema[T]`, `Field[T]` and `Codec[E]` are generic types, so a *pure* alias
facade needs generic type aliases, which are a Go 1.24 language feature. The
development toolchain (`go1.27.3`) accepts them even with `go 1.23.0` in
`go.mod` (verified), but a consumer compiling with a real Go 1.23 toolchain
cannot parse them. The module would then claim a floor it does not meet — the
exact silent-upgrade trap that `.opencode/plan/2026-11-22-go-version-floor.md`
closed and `TestGoModGoDirectiveFloor` guards.

Consequence: a pure alias facade and a 1.23 floor are mutually exclusive.
See §7 for the three options.

### 5.2 The test suite is white-box

9 of 15 root test files assert against unexported identifiers, so they cannot
stay in the root package once the engine moves. Plan: move them next to the
code as `package codec` (the executable contract follows the implementation),
keep `goversion_test.go` at the root (it reads `go.mod`), and add a new root
contract test that exercises the facade through its public API only.

### 5.3 `go doc` does not follow aliases for method sets

Verified on go1.27.3: for `type Alias = Box`, `go doc pkg.Alias` prints the alias
line and the alias's own doc comment but **not** `Box`'s methods. With a
facade-only package, `go doc qijson.Reader` would therefore no longer list
`ReadString`, `Object`, `Array`, ... Mitigation: put the full public doc
comments on the facade aliases (the constitution requires ownership/concurrency/
lifecycle docs), keep the narrative in `doc.go`, and verify pkg.go.dev
rendering. Option 7.B preserves method listings at the cost of ~40 forwarding
methods.

### 5.4 The constitution requires an amendment

The constitution's Additional Constraints list exactly three packages and say
new packages/exported identifiers require an amendment. This change adds a
fourth package (and, under Option 7.A, moves the Go floor). Plan: amend Article
VI / Additional Constraints, bump the constitution to v3.8.0, and update
`AGENTS.md` in the same commit.

### 5.5 Sentinels stay unexported

No export is needed: the facade wrappers return whatever `codec` returns, and
the white-box tests move into `codec` where they keep asserting the same
sentinels. Exporting `Err*` would be a new public API (an amendment) and is out
of scope unless requested.

### 5.6 The generator is unaffected

`cmd/qi-json-gen` only emits *text* that references the public `qijson` names
(`qijson.Struct`, `qijson.Field[T]{...}`, `qijson.Reader`, `qijson.Writer`,
`qijson.ObjectDecoder`, `qijson.DecoderCodec`, `qijson.NewCodec`,
`qijson.KindType`, ...). Aliases and wrappers keep every one of them
resolvable, including the `Field[T]` composite literal. The generator itself
imports only `qijson` and `qi-json/scanner`.

## 6. Public surface checklist

Every item below must remain resolvable from `qi-json` with the same type.

- **Types**: `Error`, `TypeError`, `ErrorKind`, `Reader`, `Writer`, `Schema[T]`,
  `Field[T]`, `Codec[E]`, `Decoder`, `Encoder`.
- **Constants**: `KindSyntax`, `KindType`, `KindUnknownField`,
  `KindDuplicateField`, `KindDepth` (typed `ErrorKind`).
- **Methods** (preserved through aliases): `Error.Error/Unwrap`;
  `TypeError.Error`; `ErrorKind.String`; `Reader.Peek/ReadNull/ReadBool/
  ReadString/ReadInt/ReadFloat/Object/Array/Skip/Path/Offset`;
  `Writer.Reset/Bytes/Err/WriteNull/WriteBool/WriteString/WriteInt/WriteUint/
  WriteFloat/WriteRaw/BeginObject/EndObject/BeginArray/EndArray/Key`;
  `Schema.Decode/Encode/Marshal/Unmarshal/UnmarshalInto/RejectUnknown/
  RejectDuplicate`; `Field.OmitEmpty`.
- **Functions**: `NewWriter`, `NewCodec`, `Struct`, `Unmarshal`,
  `UnmarshalInto`, `Decode`, `Marshal`, `Encode`, `String`, `StringPtr`, `Int`,
  `IntPtr`, `Float`, `FloatPtr`, `Bool`, `BoolPtr`, `Object`, `ObjectDecoder`,
  `ObjectDecoderPtr`, `DecoderCodec`, `Slice`, `Map`, `Array`, `Any`, `Raw`.

## 7. Decisions required (blocking)

### 7.A Pure alias facade, floor raised to Go 1.24 — **recommended**

`qijson.go` is type aliases + const re-exports + one-line wrapper functions.
Types are identical across the boundary (`errors.As` and composite literals keep
working), the facade is ~150 lines, and there is no divergence risk.

Cost: `go.mod` moves to `go 1.24.0`, `TestGoModGoDirectiveFloor` and the
documented floor move with it (constitution amendment + CHANGELOG note), and
per-type method lists disappear from `go doc qijson.X` (§5.3).

### 7.B Explicit wrapper types, floor stays at Go 1.23

Root declares real types (`type Reader struct{ inner *codec.Reader }`, ~40
forwarding methods, `Schema[T]`/`Field[T]`/`Codec[E]` wrappers with conversions
to the codec equivalents). Keeps the floor and keeps `go doc` method listings.

Cost: substantially more facade code, two definitions of each public type, and a
standing divergence risk — which weakens the "thin facade" goal.

### 7.C Mixed: aliases where legal, wrappers for generic types, floor stays 1.23

Keeps the floor but still loses method listings for the aliased non-generic
types, and is the most complex of the three. Not recommended.

## 8. Task list

- **T-R1** Amend the constitution (Article VI + Additional Constraints: fourth
  package, and the floor under 7.A) and `AGENTS.md`; bump to v3.8.0.
- **T-R2** Snapshot the baseline outside the repo: `go doc -all .`,
  `go test -run=^$ -bench=. -benchmem ./...`, and the verification results.
- **T-R3** Create `internal/codec/doc.go`; move `errors.go`, `reader.go`,
  `writer.go`, `field.go`, `schema.go`, `encode.go`, and `unmarshal.go` →
  `decode.go`; change the package clause; keep every exported name; keep the
  sentinels unexported; delete the originals from the root.
- **T-R4** Write the root `qijson.go` facade (aliases, consts, wrapper funcs)
  per the chosen option; leave `doc.go` as the package narrative and add one
  sentence pointing at `internal/codec`.
- **T-R5** Move the white-box tests to `internal/codec` as `package codec`;
  rename `unmarshal_test.go` → `decode_test.go`; keep `goversion_test.go` at the
  root (updating `wantGo` only under 7.A).
- **T-R6** Add a root facade contract test: compile-time assertions that each
  public identifier has the expected type (and is identical to its `codec`
  counterpart), plus a behavioral smoke test through `Struct`/`Marshal`/
  `Unmarshal`/`Decode`/`errors.As`.
- **T-R7** Prove generator drift is zero: `go generate ./...`, then
  `git diff --exit-code`; run the gentest package.
- **T-R8** Docs: README package-layout section, `CHANGELOG.md` `[Unreleased]`,
  `AGENTS.md` context, Makefile header comment.
- **T-R9** Full verification (§9) and a before/after benchmark comparison.

## 9. Verification

1. `go test ./... -count=1`
2. `gofmt -l .` (empty)
3. `go vet ./...`
4. `golangci-lint run` (v2, 0 issues)
5. `go test -race ./... -count=1`
6. `go generate ./... && git diff --exit-code`
7. `go doc -all .` diff against the T-R2 snapshot: only the package-level
   layout lines may differ; no identifier may disappear or change signature.
8. `go test -run=^$ -bench=. -benchmem ./...` compared with T-R2.

## 10. Risks and rollback

- **Alias/doc regression** (§5.3): mitigate with full facade docs; verify
  pkg.go.dev if available.
- **Test relocation hides a facade bug**: mitigated by T-R6; the gentest package
  also exercises the facade end to end.
- **Floor move surprises a consumer**: only under 7.A, documented in the
  constitution, `AGENTS.md`, `CHANGELOG.md` and the goversion test.
- **Rollback**: the whole change is one branch of pure moves plus a facade; a
  revert restores the single-package layout with no data or API migration.

## 11. Open questions

1. Option 7.A, 7.B or 7.C for the Go floor / alias strategy?
2. Confirm `scanner/` stays public at `qi-json/scanner` (the target-layout
   sketch's `scanner/` line is ambiguous; hiding it would break Article VI and
   generated imports).
3. Keep the sentinels unexported (§5.5), or take the opportunity to export
   `Err*` (new public API + amendment)?
4. Is the `errors.go` → `error.go` rename in the plan's sketch wanted, or keep
   the idiomatic plural `errors.go`? Same question for `unmarshal.go` →
   `decode.go` (the plan explicitly asks for `decode.go`, so that rename is
   assumed).
