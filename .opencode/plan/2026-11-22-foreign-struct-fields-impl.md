# qi-json — implementation: foreign struct fields

**Date**: 2026-11-22
**Status**: **done (2026-11-22).** Implementation of the decisions in
`.opencode/plan/2026-11-22-third-party-struct-fields.md` (§11). This plan is the
TDD task list, the fixtures, the amendment text and the verification steps for
closing the release blocker (`.opencode/plan/user-scenarios.md` §3.5, D1).
T-F1…T-F6 are implemented and verified (see §9).
**Related**: `.opencode/plan/2026-11-22-third-party-struct-fields.md` (research;
the source of truth for *why*), `docs/constitution.md`, `AGENTS.md`,
`.opencode/plan/2026-11-21-json-codegen.md`,
`.opencode/plan/2026-11-21-json-encoder.md`,
`.opencode/plan/2026-11-21-post-decoder-backlog.md` §9.

---

## 1. Decision summary (from the research doc §11)

Chosen: **F — a layered design**, all four in one effort:

- **C** — one or two new exported constructors that bind a nested
  `Decoder`/`Encoder`. *(Small constitution amendment.)*
- **B** — the generator detects a foreign `*S` implementing `qijson.Decoder`
  **and** `qijson.Encoder` and emits the C one-liner.
- **A** — for a foreign struct that is **not** qi-json-aware, the generator
  synthesizes an exported-field schema in the consumer package.
- **D** — `-foreign=pkg.T=ident`, a caller-supplied schema/codec for the cases the
  generator cannot introspect.

Rejected: **E** (a foreign package exporting its schema — redundant with B, and a
larger amendment). Not in v1: embedded-struct promotion inside a foreign struct,
and any `encoding/json` (`MarshalJSON`/`UnmarshalJSON`) hook support.

Selection tree at a field whose type is a foreign named type `pkg.T`:

1. `-foreign=pkg.T=ident` given → **D**: emit an `Object`/`Codec` against `ident`.
2. `*pkg.T` implements `Decoder` and `Encoder` → **B**: emit the C one-liner.
3. `pkg.T`'s underlying type is a `*types.Struct` → **A**: synthesize a schema
   from its exported fields (recurse for foreign fields inside it).
4. otherwise → the existing precise error (e.g. `uuid.UUID` = `[16]byte`, an
   unexported foreign type, or a one-sided `Decoder`).

## 2. Exported/unexported field policy (frozen)

- **Exported foreign fields are bound.** Read from `types.Struct` `Field(i)` +
  `Tag(i)`: name (json tag or field name), `omitempty`, and type.
- **Unexported foreign fields are skipped, never an error** — matching
  `encoding/json` and the generator's existing rule (`fieldCodecs` skips
  `!ast.IsExported`). Because this makes A approximate, the generator adds a
  one-line note to the generated file's doc comment when a bound foreign struct
  has unexported fields (no warning plumbing; documented, deterministic).
- **An unexported foreign *type*** (an exported field whose type is `tp.inner`)
  cannot be denoted from the consumer package → **precise error**.
- Unexported fields are irrelevant to B (the foreign package owns them).

## 3. API to add (C) — exact shape, to be confirmed by its failing test

Mirrors `Marshal[T, PT]`'s constraint-inference style (`encode.go`). For a **value**
field:

```go
// ObjectDecoder decodes and encodes a nested field of type S by delegating to
// *S's own DecodeJSON and EncodeJSON. It is the delegation counterpart to
// Object (which needs the other package's *Schema) and the constructor the code
// generator emits for a foreign type that is qi-json-aware.
func ObjectDecoder[T, S any, PS interface {
	*S
	Decoder
	Encoder
}](name string, field func(*T) *S) Field[T]
```

Plus, in the same change:

- `ObjectDecoderPtr[T, S any, PS interface{ *S; Decoder; Encoder }](name string, field func(*T) **S) Field[T]`
  — a pointer field, JSON null ⇒ nil.
- `DecoderCodec[S any, PS interface{ *S; Decoder; Encoder }]() Codec[S]` —
  the element codec for slices/maps/arrays of a foreign type.

Semantics: a JSON null leaves a value field unchanged / sets a pointer field to
nil (matching `Object`/`scalarPtr`); a non-null value delegates to the method;
`Empty` is `nil` for the pointer form and "never empty" for the value form (a
struct is never empty, matching `Object`). Decode and encode are both required by
the constraint, so §3's symmetry holds by construction.

*(Alternative if the `PS` type parameter proves awkward: take a `Codec[S]` built
with `NewCodec`. The `PS` form matches `Marshal` and is preferred; the failing
test decides.)*

## 4. Constitution amendment (drafted; land with T-F1)

**Article VI**, in the `qijson` decoder/encoder surface list, after `Field[T]`
constructors / the element `Codec`:

> a delegation constructor for a nested `Decoder`/`Encoder`
> (`ObjectDecoder`/`ObjectDecoderPtr`, `DecoderCodec`), so a field or element
> whose type implements `DecodeJSON`/`EncodeJSON` binds by calling them.

Bump **Version** in the footer and update `AGENTS.md` in the same change
(Article VI / "Public Surface" names the sanctioned surfaces). **No** other
candidate adds an exported identifier: A and B emit only generated code, D is a
CLI flag.

## 5. Tasks (TDD — failing test first, minimal green, then refactor)

Every task ends with the mandated verification (`make verify` in order, then
`go test -race ./...`); tasks touching decode/encode also re-run the round-trip
fuzz. Stop at the first failure and restart from `go test ./...`.

### [X] T-F1 — runtime delegation constructors (C) + amendment

- **Red**: `object_decoder_test.go` — `ObjectDecoder` decodes/encodes a value
  field through a type implementing both interfaces; JSON null leaves it
  unchanged; a tenfield document round-trips; `ObjectDecoderPtr` sets nil on null;
  `DecoderCodec` round-trips a `[]S` and a `map[string]S`; a type implementing
  only `Decoder` does not compile against the constructor (documented, not tested
  at runtime). Register the fix in the existing round-trip fuzz target.
- **Green**: `field.go` (`ObjectDecoder`, `ObjectDecoderPtr`, `DecoderCodec`).
- **Amendment**: §4 above, plus `AGENTS.md` and the footer version bump.
- **Acceptance**: new constructors work end to end; `make verify`, `-race`, fuzz
  green; amendment + `AGENTS.md` updated atomically.

### [X] T-F2 — generator: delegate to a qi-json-aware foreign type (B)

- **Red**: `gen_test.go` — a target whose field type is a foreign type that
  implements `DecodeJSON`/`EncodeJSON` generates an `ObjectDecoder` field (and
  `DecoderCodec` as a slice/map element); a type implementing only one of the two
  is a precise error; the existing `TestGenerateCrossPackageUnsupported` (a
  plain foreign struct, `time.Time`) is updated once A lands.
- **Green**: `types.go` — a `decoderEncoder` predicate using
  `types.NewMethodSet(types.NewPointer(named))`, matching a method `DecodeJSON`
  with signature `func(*qijson.Reader) error` and `EncodeJSON` with
  `func(*qijson.Writer) error` (compare against the `qi-json` types loaded by the
  same importer, or match name + arity + result). Hook it into
  `crossFieldCodec`/`crossFieldCodecPtr`/`crossElemCodec` *before* the
  scalar/struct branches.
- **Acceptance**: B selected only when both interfaces are present; deterministic;
  drift guard green.

### [X] T-F3 — generator: synthesize a schema for a plain foreign struct (A)

- **Red**: `gen_test.go` — a target with a foreign struct field generates a
  `<pkg><Type>Schema` var and exported-field accessors in the consumer package;
  an unexported foreign field is skipped (not emitted, not an error); a foreign
  field of a non-canonical width emits the existing range-check closure; a
  transitive foreign struct is emitted once and memoized; generation is
  deterministic; an embedded foreign field is a precise error (v1).
- **Green**: `types.go`/`gen.go` — a `foreignStructCodec` that walks
  `types.Struct`, memoizes per qualified name (extend `known`/a new `foreign`
  map), reuses `basicKindName`/`numericCodec`/`scalarClosure`/`objectCodec`/
  container helpers, and emits the schema var in sorted qualified-name order.
- **Acceptance**: a plain foreign struct (exported fields + tags) generates and
  round-trips; A never runs when B applies (guarded by selection order);
  drift guard green.

### [X] T-F4 — generator: caller-supplied schema/codec (D)

- **Red**: `gen_test.go` — `-foreign=pkg.T=ident` makes a foreign `pkg.T` field
  emit `qijson.Object[Owner, pkg.T](name, field, ident)`; a `-foreign` entry for a
  slice/map element emits the matching `Codec`; a malformed or unmatched entry is
  a precise error.
- **Green**: `main.go` (repeatable `-foreign` flag, `pkg.T=ident` parsing),
  `gen.go` (`foreign map[string]string` consulted first in the selection tree),
  `types.go` (resolve `pkg.T` to qualify the emitted type name).
- **Acceptance**: D overrides A/B; deterministic; no runtime change.

### [X] T-F5 — interactions

- **Red** (extend T-F2/T-F3 tests): pointer-to-foreign-struct; `[]pkg.T` and
  `map[string]pkg.T`; `omitempty` on a foreign field (no-op for a value struct,
  meaningful for a pointer); transitive nesting (`pkg.Outer` containing
  `pkg.Inner`); a foreign type reachable by two paths emitted once.
- **Green**: `starCodec` gains a foreign-pointer branch; container element
  helpers reuse `DecoderCodec` (B) or the synthesized helper (A).
- **Acceptance**: all interaction cases generate, compile and round-trip.

### [X] T-F6 — docs + fixtures + verify

- Extend the fixture `cmd/qi-json-gen/internal/gentest` (and a sibling package)
  with: a plain foreign struct (exported fields, a `json` tag, a non-canonical
  width, one unexported field, one nested foreign struct); a qi-json-aware foreign
  struct (hand-written `DecodeJSON`/`EncodeJSON` delegating to a `Schema`); add
  fields to `Config`; regenerate the checked-in file; extend `fixture_test.go`
  with decode, encode and round-trip assertions plus a range-error case.
- Docs: `qijson/doc.go` (a "foreign struct fields" paragraph), `README.md` (the
  field-family table + a codegen note), `CHANGELOG.md`, the constitution
  (amendment from T-F1, footer version), `AGENTS.md`, and the codegen plan §8
  "not done / next".
- **Acceptance**: `make verify` green (0 lint); `-race` green; `make generate`
  reproduces the checked-in files with no diff (drift guard); a 10s round-trip
  fuzz run green.

## 6. Fixtures to add

- `cmd/qi-json-gen/internal/gentest/xtypes` (extend): `Thing` (plain struct:
  `Name string json:"name"`, `Count int json:"count,omitempty"`, `Extra Inner`,
  one unexported field), `Inner` (plain struct), `Aware` (a qi-json-aware struct
  with hand-written `DecodeJSON`/`EncodeJSON` over its own `Schema`).
- `Config` gains: `Thing xtypes.Thing`, `Aware xtypes.Aware`, `Things []xtypes.Thing`,
  `AwareMap map[string]xtypes.Aware`, `Maybe *xtypes.Thing` (pointer), and a
  `-foreign`-driven type in a separate generated file (T-F4).
- Drift guards: `TestGeneratedFixtureUpToDate` and the strict guard already
  cover the regenerated files.

## 7. Governance and verification summary

| Task | New exported identifiers | Amendment? |
|---|---|---|
| T-F1 (C) | `ObjectDecoder`, `ObjectDecoderPtr`, `DecoderCodec` | **yes** |
| T-F2 (B) | none (generated code) | no |
| T-F3 (A) | none (generated code) | no |
| T-F4 (D) | none (a CLI flag) | no |
| T-F5/T-F6 | none | no |

Verification: `make verify` (test → gofmt → vet → golangci-lint, in order),
`go test -race ./...`, the round-trip fuzz target, and `make generate` for the
drift guard. Constraints unchanged: no `reflect`, no `unsafe`, no
`encoding/json`; the generator stays stdlib-only and deterministic.

## 8. Risks

| Risk | Mitigation |
|---|---|
| Source importer follows the process CWD, not `dir` (research §9.0) | Supported usage is `go generate` in the package dir; keep the precise-error fallback and provide D as the escape hatch. Document it. |
| A mis-binds a qi-json-aware foreign type | B is applied first in the selection tree (T-F2 before T-F3); add a regression test. |
| Unexported foreign fields silently dropped | Documented policy (§2) + a generated-file note; D covers shapes ≠ exported fields. |
| Non-deterministic foreign-type emission | Process foreign types and fields in sorted order; the drift guard enforces it. |
| `PS` constraint-inference ergonomics | Falling back to a `Codec[S]` parameter is a one-line change; the T-F1 failing test decides. |
| Drift between a foreign package's own schema and A's synthesized one | B prefers the foreign type's own methods; A only for non-qi-json types. |

---

## 9. Verification log (Apple M1, go1.27.3)

All six tasks were implemented TDD (failing test first). Files: `field.go` (C:
`ObjectDecoder`/`ObjectDecoderPtr`/`DecoderCodec`), `object_decoder_test.go`;
`cmd/qi-json-gen/types.go`, `foreign.go` (new), `gen.go`, `main.go`,
`gen_test.go`; fixtures `cmd/qi-json-gen/internal/gentest/{fixture.go,
fixture_test.go, xtypes/xtypes.go}` and the regenerated
`zz_qi_json_gen.go`/`zz_qi_json_gen_foreign.go`/`zz_qi_json_gen_strict.go`;
docs `doc.go`, `README.md`, `CHANGELOG.md`, `docs/constitution.md` (v3.4.0),
`AGENTS.md`.

- `make verify` green: `go test ./...` (all packages), `gofmt -l .` empty,
  `go vet ./...`, `golangci-lint run` → `0 issues`.
- `go test -race ./...` green.
- `make generate` reproduces the checked-in fixtures with no diff (the three
  drift guards pass).
- 8s `FuzzMarshalRoundTrip` green.
- Design resolution notes: one `foreign.go` file holds approach A (a `go/types`
  walk of a foreign struct's exported fields, memoized by qualified name and
  emitted in sorted order, with a file-comment note when unexported fields are
  skipped); D is a repeatable `-foreign=pkg.T=ident` flag consulted first in the
  selection tree, and an unused/malformed entry is a precise error. A pointer to a
  plain foreign struct binds through a generated null-aware `Field` literal
  (`schemaPtrCodec`), since `qijson` has no `ObjectPtr` for a synthesized schema.

### Noted for a future session

- `time.Time` is a plain struct with only unexported fields, so A now synthesizes
  an empty `timeTimeSchema` for it (an `encoding/json`-visible `time.Time` would
  need a qi-json-aware type or a `-foreign` entry). This follows the frozen
  policy (unexported fields skipped, not an error).
- Embedded-struct promotion inside a foreign struct, and `encoding/json` hook
  support, remain out of scope (as planned).
