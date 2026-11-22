# Internal codec extraction

**Status**: done (2026-11-22) — implemented on `chore/extract-internal-codec`
**Branch**: `chore/extract-internal-codec` (from `main`, targets `dev`)
**Related**: `docs/constitution.md` v3.7.0 (Article VI, Additional Constraints);
`.opencode/plan/2026-11-21-package-rename.md`;
`.opencode/plan/2026-11-22-go-version-floor.md`.

## 1. Goal

Move every implementation file of the `qijson` semantic layer into a new
`internal/codec` package and leave the module root as a thin, documented facade
that re-exports the same public API. External importers keep using `qi-json`
exactly as before; the semantic engine becomes invisible so its internals can
change without a compatibility promise.

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

## 3. Decisions

- **D1 — Alias facade, Go floor 1.24.** `qijson.go` is type aliases, const
  re-exports and one-line wrapper functions. `go.mod` moves to `go 1.24.0`
  because `Schema[T]`, `Field[T]` and `Codec[E]` are generic and generic type
  aliases are a Go 1.24 language feature. `TestGoModGoDirectiveFloor`, the
  constitution, `AGENTS.md`, `README.md` and `CHANGELOG.md` move with it.
- **D2 — The scanner stays public** at `qi-json/scanner` (see §7). Only
  `internal/codec` is added; the generator and consumer files are untouched.
- **D3 — Sentinels stay unexported.** The facade returns whatever `codec`
  returns; the white-box tests move into `codec` and keep asserting the same
  sentinels. No new public `Err*` identifiers.
- **D4 — File names.** `unmarshal.go` → `decode.go` (the requested symmetry);
  `errors.go` keeps its idiomatic plural name.

## 4. Target layout

```text
qi-json/
├── go.mod
├── doc.go                 # package narrative
├── qijson.go              # facade: aliases, consts, wrapper funcs
├── internal/
│   └── codec/             # package codec — the semantic engine
│       ├── doc.go
│       ├── decode.go      # was unmarshal.go
│       ├── encode.go
│       ├── errors.go
│       ├── field.go
│       ├── reader.go
│       ├── schema.go
│       └── writer.go
├── scanner/               # unchanged, public
└── cmd/
    └── qi-json-gen/       # unchanged
```

## 5. Findings that shape the plan

### 5.1 Generic type aliases need Go 1.24

The development toolchain (`go1.27.3`) accepts generic aliases even with
`go 1.23.0` in `go.mod` (verified), but a real Go 1.23 compiler cannot parse
them. Keeping `go 1.23.0` while using them would repeat the silent-upgrade trap
that `.opencode/plan/2026-11-22-go-version-floor.md` closed. Hence D1.

### 5.2 The test suite is white-box

9 of 15 root test files assert against unexported identifiers, so they cannot
stay at the root once the engine moves. They move next to the code as
`package codec`; `goversion_test.go` stays at the root (it reads `go.mod`); a
new root contract test exercises the facade through its public API only.

### 5.3 `go doc` does not follow aliases for method sets

Verified on go1.27.3: for `type Alias = Box`, `go doc pkg.Alias` prints the
alias and the alias's own doc comment but not `Box`'s methods. With an
all-alias facade, `go doc qijson.Reader` stops listing `ReadString`, `Object`,
... Mitigation: put the full public doc comments on the facade aliases (the
constitution requires ownership/concurrency/lifecycle docs), keep the narrative
in `doc.go`, and check pkg.go.dev rendering.

### 5.4 The constitution requires an amendment

Adding `internal/codec` changes the package list and the sanctioned structure.
Article VI and Additional Constraints are amended, the version bumps to v3.8.0,
and `AGENTS.md` is updated in the same commit.

### 5.5 The scanner and the generator are untouched

Because `scanner` stays public (D2), `qijson.Reader.Peek()` keeps returning
`scanner.Kind`, the generator keeps emitting `scanner.KindNull` and the
`qi-json/scanner` import, and `go generate ./...` must produce zero diff. This
is the acceptance bar that proves the codec move did not disturb the syntax
layer.

## 6. Public surface checklist (`qijson`)

Every item below must remain resolvable from `qi-json` with the same type.

- **Types**: `Error`, `TypeError`, `ErrorKind`, `Reader`, `Writer`, `Schema[T]`,
  `Field[T]`, `Codec[E]`, `Decoder`, `Encoder`.
- **Constants**: `KindSyntax`, `KindType`, `KindUnknownField`,
  `KindDuplicateField`, `KindDepth`.
- **Methods**: `Error.Error/Unwrap`; `TypeError.Error`; `ErrorKind.String`;
  `Reader.Peek/ReadNull/ReadBool/ReadString/ReadInt/ReadFloat/Object/Array/
  Skip/Path/Offset`; `Writer.Reset/Bytes/Err/WriteNull/WriteBool/WriteString/
  WriteInt/WriteUint/WriteFloat/WriteRaw/BeginObject/EndObject/BeginArray/
  EndArray/Key`; `Schema.Decode/Encode/Marshal/Unmarshal/UnmarshalInto/
  RejectUnknown/RejectDuplicate`; `Field.OmitEmpty`.
- **Functions**: `NewWriter`, `NewCodec`, `Struct`, `Unmarshal`,
  `UnmarshalInto`, `Decode`, `Marshal`, `Encode`, `String`, `StringPtr`, `Int`,
  `IntPtr`, `Float`, `FloatPtr`, `Bool`, `BoolPtr`, `Object`, `ObjectDecoder`,
  `ObjectDecoderPtr`, `DecoderCodec`, `Slice`, `Map`, `Array`, `Any`, `Raw`.

## 7. Decision record: the scanner stays public

Option A was chosen: `scanner/` remains at `qi-json/scanner`. Option B (moving
it to `internal/scanner`) is deferred to a separate breaking change (§11).

Why A: the scanner is a deliberately public, specified syntax layer
(`docs/task.md`, Article VI) whose internals are already unexported, so hiding
it adds little freedom while costing a public API break, a generator rewrite and
regeneration in every consumer repository. Hiding `codec` delivers the actual
goal at no consumer cost.

| Dimension | A. Keep public (chosen) | B. Hide (deferred) |
|---|---|---|
| Public API | unchanged | removes `Valid`, `Kind` + 7 consts, `Tokenizer` |
| Consumers / generated files | unaffected | break until regenerated |
| Generator | unchanged | new import set and goldens |
| Hiding achieved | semantic engine hidden | syntax hidden, kind taxonomy still leaks via `Peek` |
| Effort / risk | low / low | high / medium-high |

## 8. Task list

- **T-R1** Amend the constitution (Article VI + Additional Constraints: fourth
  package; Go floor 1.24) and `AGENTS.md`; bump to v3.8.0.
- **T-R2** Snapshot `go doc -all .` and benchmarks outside the repo as the
  baseline.
- **T-R3** Create `internal/codec/doc.go`; move `errors.go`, `reader.go`,
  `writer.go`, `field.go`, `schema.go`, `encode.go`, and `unmarshal.go` →
  `decode.go`; change the package clause; keep every exported name and the
  sentinels unexported; delete the originals from the root.
- **T-R4** Write the root `qijson.go` facade (aliases, consts, wrapper funcs);
  keep `doc.go` as the narrative and add one sentence pointing at
  `internal/codec`.
- **T-R5** Move the white-box tests to `internal/codec` as `package codec`;
  rename `unmarshal_test.go` → `decode_test.go`; keep `goversion_test.go` at the
  root (`wantGo` = `1.24.0`).
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
6. `go generate ./... && git diff --exit-code` (must be empty)
7. `go doc -all .` diff against the T-R2 snapshot: only the package-layout
   lines may differ; no identifier may disappear or change signature.
8. `go test -run=^$ -bench=. -benchmem ./...` compared with T-R2.

## 10. Risks and rollback

- **Alias/doc regression** (§5.3): full facade docs; verify pkg.go.dev.
- **Test relocation hides a facade bug**: mitigated by T-R6; the gentest package
  also exercises the facade end to end.
- **Floor move surprises a consumer**: documented in the constitution,
  `AGENTS.md`, `CHANGELOG.md` and the goversion test.
- **Rollback**: one branch of pure moves plus a facade; a revert restores the
  single-package layout with no data or API migration.

## 11. Future work (not in this change)

- **Scanner consolidation** — move `scanner/` to `internal/scanner/`, publish
  `qijson.Kind` (and decide whether `Valid` is re-exported or dropped), rewrite
  the generator to emit `qijson.KindNull`, regenerate fixtures, and ship a
  migration note. Deliberate breaking change; its own branch and release.

## 12. Verification log (2026-11-22)

- `go test ./... -count=1` — pass (`qijson`, `internal/codec`, `scanner`, the
  generator and its gentest fixture).
- `gofmt -l .` — empty.
- `go vet ./...` — pass.
- `golangci-lint run` — 0 issues.
- `go test -race ./... -count=1` — pass.
- `go generate ./...` — no change to any `zz_qi_json_gen*.go` fixture.
- `go doc -all .` — every exported function and constant is identical to the
  baseline; the types are now aliases, so their method declarations live on the
  `internal/codec` declarations (the expected §5.3 tradeoff). The facade docs
  list each aliased type's methods so `go doc qijson` stays navigable.
- Benchmarks (`-benchtime=300ms`) — within noise, allocations identical.

One unplanned fix: `cmd/qi-json-gen` identified `qijson.Reader`/`Writer` by
package name, which aliases break. `qijsonMethod` now accepts the facade alias
and the engine type by import path (`qi-json` and `qi-json/internal/codec`), so
generated output is byte-identical.
