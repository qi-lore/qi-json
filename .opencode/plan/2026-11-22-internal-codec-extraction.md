# Internal codec extraction

**Status**: proposed — `internal/codec` settled; the scanner's fate is open (§7)
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

**Non-goals**: no behavior change, no error-string/position change, no
performance change, no new public identifiers (except the deliberate scanner
consolidation if §7.B is chosen).

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
- `scanner` exports: `Valid`, `Kind` (uint8) with 7 constants
  (`KindNull`, `KindBool`, `KindString`, `KindNumber`, `KindObject`,
  `KindArray`, `KindEnd`) and `Kind.String()`, and `Tokenizer` with 14 methods.
- `qijson.Reader.Peek()` returns `scanner.Kind`, so the scanner's kind type is
  already part of the public `qijson` signature.
- `cmd/qi-json-gen` emits `import "qi-json/scanner"` and `scanner.KindNull` in
  17 places (null checks for pointer and named scalar fields); `gen_test.go`
  asserts that emitted text.

## 3. Settled decisions

- **D1 — Alias facade, Go floor 1.24.** `qijson.go` is type aliases, const
  re-exports and one-line wrapper functions. `go.mod` moves to `go 1.24.0`
  because `Schema[T]`, `Field[T]` and `Codec[E]` are generic and generic type
  aliases are a Go 1.24 language feature. `TestGoModGoDirectiveFloor`,
  the constitution, `AGENTS.md`, `README.md` and `CHANGELOG.md` move with it.
- **D2 — Sentinels stay unexported.** The facade returns whatever `codec`
  returns; the white-box tests move into `codec` and keep asserting the same
  sentinels. No new public `Err*` identifiers.
- **D3 — File names.** `unmarshal.go` → `decode.go` (the plan's requested
  symmetry); `errors.go` keeps its idiomatic plural name.

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
├── scanner/               # §7.A: unchanged, public
│   └── (or internal/scanner/ under §7.B)
└── cmd/
    └── qi-json-gen/
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

Adding `internal/codec` (and, under §7.B, moving the scanner) changes the
package list and the sanctioned surface. Article VI and Additional Constraints
are amended, the version bumps to v3.8.0, and `AGENTS.md` is updated in the
same commit.

### 5.5 The generator's only scanner dependency is the null check

`gen.go` emits `scanner.KindNull` for pointer and named-scalar fields and adds
the `qi-json/scanner` import when it does. Nothing else in generated code names
the scanner.

## 6. Public surface checklist (`qijson`)

Every item below must remain resolvable from `qi-json` with the same type,
except for the deliberate scanner change in §7.

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

## 7. Open decision: keep the scanner public or hide it

### 7.A Leave `scanner/` public at `qi-json/scanner`

Only `internal/codec` is added. `qijson` keeps `Reader.Peek() (scanner.Kind,
error)`, so no `qijson.Kind` is needed.

### 7.B Move it to `qi-json/internal/scanner`

`Valid`, `Kind` + 7 constants, `Kind.String` and `Tokenizer` + 14 methods leave
the public API. Follow-on work is forced:

- `qijson.Kind` must exist because `Reader.Peek` returns it and generated code
  compares `KindNull` — either an alias to the internal type (leaks
  `internal/scanner.Kind` into the public signature and docs) or a distinct
  public type that mirrors the 7 constants (cleaner, more churn, coupling).
- `Valid` either disappears (a product regression; `encoding/json.Valid` has no
  equivalent here) or is re-exported as `qijson.Valid` (validation stays public,
  so the tokenizer is only partially hidden).
- The generator must drop `qi-json/scanner`, emit `qijson.KindNull`, regenerate
  every fixture and update the `gen_test.go` goldens. Every already-generated
  file in a consumer repository imports `qi-json/scanner` and stops compiling
  until it is regenerated.
- Tests, fuzz and benchmarks move to `./internal/scanner`; `Makefile`'s
  `SCANNER`, `AGENTS.md`, `README.md`, `docs/task.md` and Article VI all change.

### Comparison

| Dimension | A. Keep public | B. Hide |
|---|---|---|
| Public API | unchanged | removes `Valid`, `Kind` + 7 consts, `Tokenizer` + 14 methods |
| SemVer | non-breaking | breaking (pre-1.0: minor, plus a migration note) |
| Consumers using `Valid` | unaffected | break, unless `qijson.Valid` is re-exported |
| Consumers using `Tokenizer` | unaffected | break, with no replacement |
| Already-generated consumer files | unaffected | fail to compile until regenerated |
| Generator source | unchanged | drop scanner import, emit `qijson.KindNull`, new goldens |
| `qijson.Kind` | not needed | required (alias leaks internal type; distinct type mirrors consts) |
| Hiding achieved | semantic engine hidden; syntax layer stays public by design | syntax + semantic hidden, but `Peek` still exposes the kind taxonomy |
| Article VI | add `internal/codec` | rewrite the scanner bullet and the package list |
| Docs / tooling churn | one sentence | `task.md`, `README`, `AGENTS`, `Makefile`, `CHANGELOG`, migration note |
| Effort | low | high |
| Risk | low | medium-high (public break + generator churn) |
| Reversibility | trivial | hard |

### Non-obvious findings

1. **Hiding the scanner does not fully hide the tokenizer.** `Reader.Peek` is a
   public method returning a kind. Either `qijson.Kind` is published (the kind
   taxonomy stays public) or `Peek` changes (another break). The hide buys the
   `Valid` and `Tokenizer` surfaces, not the kind model.
2. **The sharpest cost is consumer regeneration.** Generated files name
   `qi-json/scanner`; after B they do not compile until regenerated. That is a
   real upgrade tax on every codegen user, unrelated to the codec refactor.
3. **`scanner` is already a thin public surface.** Its internals (`validator`,
   `parseError`, `scan*`) are unexported and spec'd in `docs/task.md`. Hiding it
   adds little internal freedom; hiding `codec` adds a lot.
4. **Bundling B into this refactor mixes a structural move with a breaking
   API change.** A non-breaking `internal/codec` extraction stays reviewable and
   bisectable; the scanner consolidation deserves its own release, changelog
   entry and migration note.

### Recommendation

Do **A** now, and treat **B** as a separate, deliberate breaking change if a
single-public-package API is the goal. A delivers the refactor's actual goal
(hide the semantic engine) with no consumer breakage, no generator churn and a
small review surface; B's hiding is partial and its cost lands on every consumer
with generated code.

## 8. Task list (A; B adds the bracketed items)

- **T-R1** Amend the constitution (Article VI + Additional Constraints: fourth
  package; Go floor 1.24) and `AGENTS.md`; bump to v3.8.0.
- **T-R2** Snapshot `go doc -all .`, `go doc -all ./scanner` and benchmarks
  outside the repo as the baseline.
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
- **B-only**: T-R3b move `scanner/` → `internal/scanner/`; T-R3c add
  `qijson.Kind` (+ consts, and `Valid` if re-exported); T-R7b update `gen.go`
  to emit `qijson.KindNull` and drop the scanner import, regenerate fixtures,
  update `gen_test.go`; T-R8b rewrite the scanner docs and Article VI bullet;
  T-R9b add a migration note.

## 9. Verification

1. `go test ./... -count=1`
2. `gofmt -l .` (empty)
3. `go vet ./...`
4. `golangci-lint run` (v2, 0 issues)
5. `go test -race ./... -count=1`
6. `go generate ./... && git diff --exit-code` (under B this diff is expected
   once, then committed)
7. `go doc -all .` diff against the T-R2 snapshot: only the package-layout
   lines may differ; no identifier may disappear or change signature (under B,
   the documented scanner removals are the only exceptions).
8. `go test -run=^$ -bench=. -benchmem ./...` compared with T-R2.

## 10. Risks and rollback

- **Alias/doc regression** (§5.3): full facade docs; verify pkg.go.dev.
- **Test relocation hides a facade bug**: mitigated by T-R6; the gentest package
  also exercises the facade end to end.
- **Floor move surprises a consumer**: documented in the constitution,
  `AGENTS.md`, `CHANGELOG.md` and the goversion test.
- **B breaks consumer builds** (generated files and `scanner` imports): needs a
  migration note and a deliberate release.
- **Rollback**: A is one branch of pure moves plus a facade; a revert restores
  the single-package layout. B is harder to revert once consumers regenerate.

## 11. Open questions

1. §7.A or §7.B for the scanner?
2. Under B: re-export `qijson.Valid` or drop validation entirely?
3. Under B: `qijson.Kind` as an alias to the internal kind, or a distinct public
   type that mirrors the seven constants?
