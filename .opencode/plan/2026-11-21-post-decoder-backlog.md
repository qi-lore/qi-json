# qi-json — post-decoder backlog (T11+)

**Date**: 2026-11-21
**Status**: plan agreed; tasks 4, 2, 1 and 3 (the encoder) done. Two open
follow-ups remain, tracked in §9.
**Related**: `docs/constitution.md`, `AGENTS.md`,
`.opencode/plan/2026-11-21-json-decoder.md` (§6 open questions, §8 next),
`.opencode/plan/2026-11-21-json-codegen.md` (§8 not-done), `README.md`.

---

## 1. Context

The decoder task list (T0–T10) and the optional code generator (T8) are
complete and verified. What remains is the backlog the two plans leave open:

| Ref | Work | Source |
|---|---|---|
| **4** | Decoder UTF-8 policy + pin the semantic defaults | decoder plan §6 |
| **2** | Cross-package named types in codegen | codegen plan §8 |
| **1** | `-strict` codegen flag | codegen plan §8 |
| **3** | Encoder (`Marshal`) | decoder plan §6, research §10 |

## 2. Sequence (agreed 2026-11-21)

**4 → 2 → 1 → 3.** Smallest, most self-contained first; the encoder last because
it needs its own plan and a constitution amendment.

Each task is TDD (failing test first), ends with the mandated verification
(`make verify`, plus `-race` and fuzz where relevant), and does not start until
the previous task is green.

---

## 3. Task 4 — decoder UTF-8 policy and semantic defaults

### 3.1 Decisions

- **Semantic defaults — keep as shipped (T3/T5), pin with tests + docs.**
  Unknown members *ignored* (`RejectUnknown` opt-in); duplicate members *last
  wins* (`RejectDuplicate` opt-in); JSON `null` leaves a value field unchanged
  and sets a pointer field to `nil`; `int64`/`float64` overflow is a *type
  error*, never silent truncation. These already match the research §10
  recommendation; no behaviour change, only explicit tests and documentation.
- **UTF-8 policy — the decoder rejects invalid UTF-8; the scanner stays
  syntax-only.** A decoded string value or object member name must be valid
  UTF-8. The scanner's `Valid` deliberately does not check UTF-8 (constitution
  IV; `docs/task.md` §8.1), so the decoder is *stricter* than `Valid` on this
  one axis. This follows the research §10 recommendation ("reject in the
  decoder, keep the scanner syntax-only").

### 3.2 Error shape (no amendment needed)

Invalid UTF-8 is reported as a `*marshal.Error` with `Kind = KindSyntax` (per
RFC 8259, a JSON text must be UTF-8, so this is invalid JSON), wrapping an
**unexported** sentinel `errInvalidUTF8`. It adds no exported identifier, so no
constitution amendment is required (Article VI already sanctions "the error
types").

Because a syntax error can now occur on a document the scanner calls valid, the
fuzz differential target `FuzzUnmarshalMatchesValid` must exempt the UTF-8 case
(the sound directions it asserts are unaffected: success ⇒ `Valid`, and
`!Valid` ⇒ decode fails).

### 3.3 Where validation lives

Validate the raw string content (escapes intact) at the two places the decoder
turns string bytes into Go strings:

- `Reader.ReadString` — string *values* (also the `Any` string path).
- `Reader.Object` and `Reader.objectRaw` — object *keys* (`Map` and `Schema`).

Escapes are pure ASCII and `\u` decoding always yields valid UTF-8 (a lone
surrogate becomes `U+FFFD`), so **validity of the raw bytes equals validity of
the decoded string**; validating the raw bytes is sufficient.

- ASCII fast path: only run `utf8.Valid` when a byte with the high bit is
  present (a byte-wise/SWAR high-bit scan). Pure-ASCII documents — the common
  case and the benchmark corpus — pay only the scan, not `utf8.Valid`.
- The error offset points at the first invalid byte:
  `tok.Offset() - len(raw) - 1 + firstInvalid(raw)`.
- `Raw` fields are *not* validated: `Raw` copies the value's exact literal bytes
  and is documented as byte-transparent.

### 3.4 Subtasks

- **T4a — tests (red).** Invalid UTF-8 in a value and in a key (via `Schema` and
  via `Any`) fails with `KindSyntax`, the right offset, and
  `errors.Is(err, errInvalidUTF8)`; valid multi-byte and 4-byte UTF-8 passes; a
  string mixing a raw high byte with a `\u` escape; `Valid` still accepts the
  same bytes (pins the intended divergence). Explicit tests for the four
  semantic defaults if any gap exists.
- **T4b — implementation (green).** `errInvalidUTF8`; `hasHighBit`,
  `firstInvalidUTF8`, `Reader.checkString`; call sites in `ReadString`,
  `Object`, `objectRaw`.
- **T4c — docs.** `marshal/doc.go`, `README.md` (the "Decoding rules" bullet),
  and the decoder plan §6 updated from "open" to "decided".
- **T4d — fuzz + verify.** Exempt the UTF-8 case in `FuzzUnmarshalMatchesValid`;
  add invalid-UTF-8 seeds. `make verify`, `-race`, and a 10s fuzz on all three
  targets green; re-measure the string benchmarks and record the impact.

**Acceptance**: decoder rejects invalid UTF-8 in values and keys; scanner
unchanged; semantic defaults pinned; `make verify`, `-race`, fuzz green.

### 3.5 Results (T4 done, 2026-11-21)

- **Shipped**: `marshal/errors.go` (`errInvalidUTF8`), `marshal/reader.go`
  (`checkString`, `hasHighBit`, `firstInvalidUTF8`; call sites in `ReadString`,
  `Object`, `objectRaw`), `marshal/utf8_test.go`, and the fuzz target update.
- **Offset anchor**: because the tokenizer consumes the `:` after an object key,
  `Offset()` after `ReadString` is not the string end. The reader captures the
  opening-quote offset right after `Peek` and derives the content start from it
  (`start + 1`), so key and value offsets are exact.
- **Semantic defaults** were already pinned by existing tests (unknown ignored /
  rejected, duplicate last-wins / rejected, null into scalar/pointer, int and
  float overflow as `KindType`); no new tests were needed, only documentation.
- **Verification**: `make verify` green (0 lint issues); `go test -race ./...`
  green; 10s `FuzzUnmarshal` and 10s `FuzzUnmarshalMatchesValid` green (the
  latter now exempts the UTF-8 case and seeds invalid UTF-8).
- **Performance** (Apple M1, go1.27.3; before → after): medium strings
  1119 → 1033 MB/s, long strings 2856 → 2350 MB/s, one 64 KiB string
  4157 → 3172 MB/s; `StructDecode` 417 → 447 ns/op; scanner benchmarks
  unchanged within noise. The UTF-8 pass costs ~8–25% on string-heavy input;
  the medium case still exceeds the 1 GB/s T7 target.

---

## 4. Task 2 — cross-package named types in codegen

Today `cmd/qi-json-gen` resolves only same-package named types; a selector type
(`pkg.T`, `time.Duration`) falls through to `unsupported type` in
`fieldDecoder`/`elemDecoder`. This task lifts that limit.

### 4.1 Design pressure

- **D8.4 forbids `go/types` in v1** because a source importer fighting module
  mode is fragile. Lifting it is the whole point of T2, so T2 is the isolated,
  deliberate upgrade D8.4 anticipated.
- Candidate approach: type-check the target package with `go/types` and a
  `go/importer` (`ForCompiler(fset, "source", nil)`) importer, then consult the
  resolved type instead of the AST for the underlying kind. Must confirm the
  "source" importer works under module mode; if not, fall back to a
  `go list`-driven package graph or `go/packages`-style loading (stdlib-only
  constraint still applies — `go/packages` is not stdlib, so a `go list`/`go
  build`-based loader or the source importer is required).
- Scope v1 of T2 to **cross-package named types whose underlying type is a
  supported basic scalar** (and pointers to them), emitted as an inline
  conversion + range check, reusing the existing extended-numeric machinery.
  Cross-package *structs* remain out of scope (they would need their own
  generated `DecodeJSON`, which lives in the other package) and stay a precise
  error.
- Keep generation deterministic (sort emitted collections) so the drift guard
  passes; add a fixture with a cross-package scalar type.

**Acceptance**: a field of a cross-package named scalar type generates and
decodes; unsupported cross-package shapes error precisely; drift guard green;
`make verify`, `-race` green.

### 4.2 Results (T2 done, 2026-11-21)

- **Approach**: the stdlib `go/types` + `importer.ForCompiler(fset, "source",
  nil)` source importer resolves selectors, empirically working in module mode
  for both stdlib (`time`) and module-local packages. The pass is **best-effort**:
  on a type-check error `g.info` stays nil and a cross-package field falls back to
  a precise unsupported-type error, so the change is additive.
- **Shipped**: `cmd/qi-json-gen/types.go` (`typecheck`, `namedOf`, `addImport`,
  `crossFieldCodec`/`crossFieldCodecPtr`, `crossNamedCodec`, `crossScalarCodec`,
  `crossElemDecoder`, `basicKindName`, `crossHelperSuffix`); `gen.go` gains the
  `info`/`extraImports` state, a `SelectorExpr` case in `fieldCodec`, selector
  handling in `starCodec` and `elemDecoder`, a type-checked file list in `load`,
  and alias-aware imports in `writeImports`.
- **Scope**: cross-package named types with a basic-scalar underlying
  (string/bool/integer/float), as a field, a pointer field, and a slice or map
  element. A non-scalar underlying (cross-package struct) is a precise error.
- **Fixture**: `internal/gentest/xtypes` adds named `Label string` / `Flag bool`;
  `Config` gains `time.Duration` (value, pointer, slice) and `xtypes` (value, map
  element) fields; the checked-in generated file is regenerated and the
  end-to-end fixture test decodes them.
- **Verification**: `make verify` green (0 lint issues); `-race` green; drift
  guard green.

---

## 5. Task 1 — `-strict` codegen flag

Add `-strict` to `cmd/qi-json-gen`: when set, generated schemas call
`RejectUnknown()` and `RejectDuplicate()` (chained on the schema value) so the
generated `DecodeJSON` rejects unknown and duplicate members.

- `main.go` flag; `gen.go` renders the chain; `gen_test.go` covers on/off.
- Extend the `gentest` fixture (or a second type) to exercise strict decoding
  end to end; regenerate the checked-in file; drift guard green.
- No new exported identifier (a CLI flag is not a Go export); no amendment.

**Acceptance**: `-strict` emits the chain; strict decode rejects unknown and
duplicate members; `make verify`, `-race` green.

### 5.1 Results (T1 done, 2026-11-21)

- **Shipped**: `-strict` flag in `main.go`; `generateWith(dir, types, strict)`
  in `gen.go` (with `generate` kept as the non-strict wrapper); `renderType`
  chains `.RejectUnknown().RejectDuplicate()` when strict.
- **Fixture**: `Strict` type generated by a second `//go:generate` directive into
  `zz_qi_json_gen_strict.go`; `TestGeneratedStrict` decodes a normal document and
  checks that an unknown member is `KindUnknownField` and a duplicate is
  `KindDuplicateField`. `TestGeneratedStrictFixtureUpToDate` is the drift guard.
- **Unit tests**: `TestGenerateStrict` / `TestGenerateNotStrict`.
- **Verification**: `make verify` green; `-race` green.

---

## 6. Task 3 — encoder (`Marshal`)

**Done (2026-11-21).** Plan and results:
`.opencode/plan/2026-11-21-json-encoder.md`. The reflection-free,
codegen-symmetric encoder shipped (`Encoder`/`Writer`/`Marshal`/`Encode`,
`Schema`/`Field` encode with declaration order and `omitempty`, the element
`Codec[E]`, and generated `EncodeJSON`), with the Article VI amendment and the
3.2.0 version bump in the same change. `make verify`, `-race` and a round-trip
fuzz target are green.

---

## 7. Governance

| Task | New exported identifiers | Amendment? |
|---|---|---|
| 4 | none (`errInvalidUTF8` is unexported; reuses `KindSyntax`) | no |
| 2 | none (generated code only) | no |
| 1 | none (a CLI flag) | no |
| 3 | new encoder surface | **yes** |

## 8. Verification

Every task finishes with `make verify` in order (test → gofmt → vet →
golangci-lint), plus `go test -race ./...` and a fuzz run where the task touches
the decoder or scanner. Results are recorded in each task's handoff.

---

## 9. Open follow-ups (T12+)

Not started; recorded so a later session can pick them up.

- **Package rename** — rename `marshal` to a name that reflects that it both
  decodes *and* encodes (recommended: `codec`). Needs a name decision and a
  constitution amendment (Article VI names the package). Plan:
  `.opencode/plan/2026-11-21-package-rename.md`.
- **Codegen named `string`/`bool` types** — generate for a same-package named
  type whose underlying type is `string` or `bool` (D8.2 covers numeric
  underlyings only). Recorded in `.opencode/plan/2026-11-21-json-codegen.md` §8.
