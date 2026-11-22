# qi-json — research: struct fields whose type is a struct from another package

**Date**: 2026-11-22
**Scope**: §9 answers §4's questions, §10 scores §5's candidates against §3's
constraints, and §11 records the recommendation. Documentation only (no code);
the follow-up implementation plan is
`.opencode/plan/2026-11-22-foreign-struct-fields-impl.md`. This is a release
blocker (`user-scenarios.md` §3.5, D1).
**Related**: `.opencode/plan/user-scenarios.md` §3, `docs/constitution.md`
(Article VI), `.opencode/plan/2026-11-21-json-codegen.md`,
`.opencode/plan/2026-11-21-json-encoder.md`,
`.opencode/plan/2026-11-21-post-decoder-backlog.md` §9.

This file is a design record. Task state lives in `docs/tasks.md`.

---

## 1. Problem

A struct field whose type is a **struct defined in another package** cannot be
bound today (evidence: `user-scenarios.md` §3.4, §5.5):

- **The generator refuses it** — `unsupported cross-package type thirdparty.Thing
  (only basic scalar underlying types are supported)`. Cross-package *scalars*
  are fine; cross-package *structs* are not.
- **The manual fallback is limited** — it can bind only the foreign struct's
  **exported** fields, needs full `Field[T]` closures for non-canonical numeric
  widths (`int`, `int32`, `uint16`, …), and has no constructor for delegating to a
  nested `Decoder`.

Using a struct from another package as a field is a common Go pattern. Without
support, the library is not usable for many real types, so this must be closed
before the library can be considered useful.

## 2. Goal of this task

Produce a **decision document** — not code — that recommends how to support
foreign struct fields, with enough detail (chosen approach, rejected
alternatives, and a TDD task list) that a following session can implement it.

## 3. Constraints (non-negotiable)

- No `reflect`, no `unsafe`; no import of `encoding/json` anywhere.
- The code generator stays **stdlib-only** and **deterministic** (sorted output;
  the drift guard must keep passing).
- The decoder and the encoder must stay **symmetric** (decode and encode for the
  same field set).
- New **exported** identifiers require a constitution amendment (Article VI).
- Module-mode correctness: the generator already uses `go/types` with the stdlib
  source importer (`cmd/qi-json-gen/types.go`); any new resolution must work in
  module mode and must not add third-party dependencies.
- Prefer **additive** changes; the module is pre-1.0 but gratuitous breakage is
  not wanted.

## 4. Questions the research must answer

1. What exactly does "support a foreign struct field" mean for **decode** and for
   **encode**? What is the minimal useful slice?
2. Where should the binding live?
   - generated into the **consumer** package, or
   - provided by the **foreign** package (its own generated `DecodeJSON`), or
   - supplied by the caller (a schema/`Codec` registration)?
3. **Exported vs unexported** foreign fields: policy and error behavior.
4. Foreign structs with **non-canonical numeric widths** — how are they bound?
5. How is the foreign type's **shape** discovered (go/types, source importer,
   module cache) and how does that behave across modules?
6. **Transitive** nesting: a foreign struct that itself contains a foreign struct.
7. Interactions: `omitempty`, **pointers** to foreign structs, **slices/maps** of
   foreign structs, and `any`.
8. Can a foreign type that already implements `qijson.Decoder` be bound cheaply
   (a one-liner), and does that cover the common case?

## 5. Candidate approaches to evaluate

For each: feasibility in module mode, exported-field limits, determinism,
amendment cost, encoder symmetry, API impact.

- **A — Generator emits an inline schema for the foreign struct** into the
  consumer package (via `go/types` + the source importer). Exported fields only;
  precise error on unexported; full closures for non-canonical widths. Extends
  the existing cross-package scalar path.
- **B — Generator detects a foreign type implementing `qijson.Decoder`** and
  emits a `Field[T]` closure that calls its `DecodeJSON`/`EncodeJSON`.
- **C — New exported constructor** for "bind a nested `Decoder`", e.g.
  `ObjectDecoder[T, S]` / a `Decoder`-based `Object` (small amendment). Gives
  hand-writers a one-liner and backs approach B.
- **D — Caller-supplied schema/`Codec` for a foreign type** (a directive or flag,
  e.g. `-foreign=thirdparty.Thing=thingSchema`), so the consumer points the
  generator at a schema it owns.
- **E — Require the foreign package to generate its own schema/`DecodeJSON`**, and
  have the consumer generator discover and import it (cross-package schema
  discovery). Works when the consumer controls the foreign package; not when it
  is a third-party module.
- **F — combinations** (for example A for plain structs, B/C for
  `Decoder`-implementing types, D as the escape hatch).

Explicitly consider **which of these cover a truly third-party module** (source
not under the consumer's control) versus only same-module sibling packages.

## 6. Deliverable and acceptance

- A written **recommendation**: the chosen approach, why, and what was rejected.
- An explicit **exported/unexported field policy**.
- A **follow-up implementation plan** (new plan file) with a TDD task list
  (failing test first), the fixtures to add, the constitution amendment needed
  (if any), and the verification steps (`make verify`, `-race`, fuzz, drift
  guard).
- The research task itself changes **documentation only**; throwaway experiments
  are allowed but must not be committed as library code.
- Every library fact used is **verified against the code**, not recalled.

## 7. Out of scope

- The Go-version floor discrepancy (`user-scenarios.md` §2.4 / D3) — a separate
  open issue.
- Encoder pretty-printing, `io.Writer` streaming, encode-time `Raw` validation.
- Any new JSON feature beyond binding foreign struct fields.

## 8. How to resume

Read `.opencode/plan/user-scenarios.md` §3 for the user-facing evidence, then
`cmd/qi-json-gen/types.go` (`typecheck`, `namedOf`, `crossNamedCodec`,
`crossElemCodec`) and `cmd/qi-json-gen/gen.go` (`fieldCodec`, `elemCodec`) for
the current cross-package scalar machinery that approach A would extend. Start by
answering §4 questions 1–2, then score §5's candidates against §3's constraints.

---

## 9. Answers to §4 (verified against the code)

### 9.0 Method and new evidence

Everything below was checked against the code and the toolchain, not recalled.
New evidence gathered this session (Apple M1, go1.27.3) with throwaway modules
under `/private/tmp` (not committed):

- `go/types` + `importer.ForCompiler(fset, "source", nil)` — the machinery
  `typecheck` already uses — resolves a **truly third-party** type in module mode:
  `github.com/google/uuid.UUID` straight from the module cache, and a `replace`d
  sibling module, both with `0` type errors. §5's "feasibility in module mode"
  question is settled for both cases.
- A resolved cross-package `*types.Named` exposes exactly what the candidates
  need: `named.Underlying().(*types.Struct)` with `Field(i)`, `Var.Exported()`,
  `Var.Embedded()`, `Var.Type()` and `Struct.Tag(i)`; and
  `types.NewMethodSet(types.NewPointer(named))` with full signatures (verified: a
  foreign struct's `DecodeJSON` and `uuid.UUID`'s 15 methods all appear).
- **New caveat — the importer follows the process CWD, not `dir`.** The same
  `qi-json-gen` binary, run with CWD = the consumer module, resolved `uuid.UUID`
  and reported the precise `unsupported cross-package type uuid.UUID (only basic
  scalar underlying types are supported)`; run with CWD = the library repo
  pointing at the same directory, resolution failed and the field degraded to the
  vaguer `unsupported type uuid.UUID`. The supported usage is fine — `go generate`
  runs the directive with CWD set to the package directory — but any A/B work
  inherits this and needs a precise-error fallback (§11).

### 9.1 Q1 — what "support" means for decode and encode; the minimal useful slice

Two capabilities must be named separately; conflating them is what makes this look
like one problem.

**(a) Delegation (exact).** When the foreign type implements `qijson.Decoder`
(`DecodeJSON(*Reader) error`, `unmarshal.go`) and `qijson.Encoder`
(`EncodeJSON(*Writer) error`, `encode.go`), "support" means the consumer's schema
binds the field by calling those methods: decode `field(v).DecodeJSON(r)`, encode
`field(v).EncodeJSON(w)`. The foreign package owns the mapping, so this is exact —
unexported fields, custom shapes, every numeric width, arbitrary nesting — for
free. It is the *only* faithful binding for a type whose JSON shape is not "its
exported fields".

**(b) Synthesized binding (exported fields only, approximate).** When the foreign
type is a plain struct (not qi-json-aware), "support" means binding its default
`encoding/json`-style view: its **exported** fields, their `json`/`omitempty`
tags, exactly as a same-package struct is bound today (`fieldCodecs`,
`objectCodec`). Decode and encode stay symmetric for free because both come from a
single `Schema`. Limits: unexported fields are invisible (so a foreign type with
unexported JSON state does not round-trip), and field-selection rules (embedded
promotion, tag precedence) must be replicated.

**Minimal useful slice.** Neither alone closes the blocker:

- (a) alone leaves the recorded scenario open — `user-scenarios.md` §3.4's example
  is `other_third-party-package.StructType`, a plain struct that is not
  qi-json-aware.
- (b) alone is *unsafe* — applied to a qi-json-aware foreign type it would bind the
  exported fields and **silently disagree** with the type's own `DecodeJSON`: a
  correctness bug, not merely a limitation.

So the minimal useful slice is **(a) as the exact path and (b) as the approximate
path, chosen by detecting whether the foreign type is qi-json-aware**, plus a loud
escape hatch when neither is faithful. Concretely, "supported" means: a consumer
struct with a foreign struct field generates and round-trips without hand-written
closures, and a shape mismatch is a `*qijson.Error` (`KindType`), exactly as for a
same-package struct.

- *Decode* must set every bindable exported member and leave absent members
  unchanged (current null semantics).
- *Encode* must write the same member set in declaration order and honour the
  field's own `omitempty`. Both fall out of one `Schema`.

**Explicitly out of the minimal slice:** private foreign state; `encoding/json`
`MarshalJSON`/`UnmarshalJSON` hooks (qi-json deliberately is not encoding/json and
does not call them); embedded-struct promotion beyond what a same-package struct
gets (`fieldCodecs` already rejects embedded fields); foreign generics beyond
instantiated named types.

### 9.2 Q2 — where should the binding live?

Three placements, scored on the candidates' axes:

1. **Consumer package, generated inline** (`zz_qi_json_gen.go`) — either a `Field`
   closure calling the foreign `DecodeJSON`/`EncodeJSON` (a), or a synthesized
   `<pkg><Type>Schema` var with exported-field accessors (b). Zero consumer
   boilerplate; the `//go:generate` flow "just works"; matches where every other
   binding lives; **no new exported API for (a)**; for (b) risks drifting from a
   schema the foreign package might also define.
2. **Foreign package** — requires the foreign package to export a `Schema` (a new
   exported identifier and a new cross-package convention, against today's
   unexported `<t>Schema`) or to expose `DecodeJSON`/`EncodeJSON` (which is really
   placement 1a). Only works when the consumer controls the foreign package; fails
   a third-party module. **Rejected as the primary.**
3. **Caller-supplied** — the consumer names a schema/codec it owns via a flag
   (e.g. `-foreign=pkg.T=ident`). Works for a truly third-party module the
   generator cannot introspect, and for a foreign shape ≠ exported fields;
   additive; deterministic; sidesteps the CWD caveat. Cost: consumer boilerplate.

**Recommendation for Q2: placement 1 (consumer, generated) by default, with
placement 3 (caller-supplied) as the explicit escape hatch.** Placement 1 keeps
the "tags in, schema out" promise and needs no amendment for delegation; placement
3 is the pressure valve for the cases placement 1 cannot cover.

### 9.3 Q3 — exported vs unexported foreign fields (policy)

- **Exported fields: bound.** For (b), name, `json`/`omitempty` tag and type are
  read from `types.Struct` (`Field(i)`, `Tag(i)`).
- **Unexported fields: skipped, not an error** — matching `encoding/json` and the
  existing rule for the consumer's own structs (`fieldCodecs` skips
  `!ast.IsExported`). Because skipping makes (b) *approximate*, the generator
  documents this and may warn, but does not fail. *(This refines §5 A's "precise
  error on unexported": an unexported foreign **field** is skipped, not fatal.)*
- **An unexported foreign *type*** (e.g. an exported field whose type is
  `tp.inner`) cannot be denoted from the consumer package at all — precise error.
- For (a) the policy is moot: the foreign package handles its own fields.

### 9.4 Q4 — foreign non-canonical numeric widths

- (a): a non-issue; the foreign codec owns its widths.
- (b): reuse the existing machinery verbatim. `numericCodec`, `scalarClosure` and
  `crossScalarCodec` already emit a full `qijson.Field[T]{Decode, Encode, Empty}`
  literal with a round-trip range check (`qiRangeError`) for `int`, `int32`,
  `uint16`, … The foreign-struct path composes the same per-field logic against the
  foreign accessor `&v.Field`.

### 9.5 Q5 — shape discovery across modules

See §9.0. `go/types` + the stdlib source importer discovers both the struct shape
(fields, tags, exportedness) and the method set, in module mode, for stdlib,
same-module siblings, `replace`d modules and module-cache third-party packages —
**provided the process CWD is the consumer's module** (true under `go generate`).
Recorded as a caveat, not a blocker.

### 9.6 Q6 — transitive nesting

- (a): free — the foreign `DecodeJSON` recurses internally.
- (b): a foreign struct's field may itself be a foreign struct; the generator
  recurses, memoizing per qualified type name (like the existing `known` map and
  the `helpers`/`helperOrder` dedupe). Cycles arise only through pointers
  (unsupported today, §9.7) or slices/maps; memoize on entry to break them.
  Determinism: process foreign types in sorted qualified-name order.

### 9.7 Q7 — interactions

- **`omitempty`:** for (b) the foreign field's own tag drives `Field.OmitEmpty`, as
  for a same-package field; a struct is never empty (matches `Object` and
  `encoding/json`), so `omitempty` is a no-op on a value struct and meaningful only
  on a pointer. For (a) the outer field's tag is the consumer's; the interior is
  the foreign type's.
- **Pointers to a foreign struct:** (a) is natural (`field(v)` is already `*S`).
  (b) needs `*S` null↔nil semantics. Both are new: `starCodec` today rejects
  pointer-to-struct (`unsupported pointer type`) for same-package and cross.
- **Slices/maps of foreign structs:** build an element `Codec[E]` — (a)
  `qijson.NewCodec(decode, encode)` wrapping the methods; (b) a paired
  `qiDecode<Type>`/`qiEncode<Type>` helper, exactly as for a same-package struct
  (`structHelper`/`structEncodeHelper`).
- **`any`:** unchanged. `encodeAny` already round-trips a value that implements
  `Encoder`; a plain foreign struct inside an `any` remains non-encodable (no
  reflection) — a documented limitation.

### 9.8 Q8 — a cheap binding for a foreign `Decoder`; does it cover the common case?

Yes to the first: delegation is a one-liner — a field via a new constructor (C) or
a raw `Field[T]{Decode:…, Encode:…}` literal, and an element via `NewCodec(…)`.
No to the second: it covers only a **qi-json-aware** foreign type and does nothing
for the recorded scenario's plain struct. It is necessary — it prevents the unsafe
(b)-on-aware-type bug — but not sufficient.

---

## 10. §5 candidates scored against §3's constraints

Legend for "3rd-party coverage": **3P** = a truly third-party module (source not
under the consumer's control); **sib** = same-module sibling only.

| Candidate | Module-mode feasibility | Exported-field limits | Determinism | Amendment | Encoder symmetry | Coverage | Verdict |
|---|---|---|---|---|---|---|---|
| **A** inline schema for a plain foreign struct | Feasible (go/types, verified); CWD caveat | exported only; unexported skipped (lossy); embedded must be rejected in v1 | fine (sort foreign types; field order is source order) | none (generated code) | automatic — one `Schema` carries both directions | **3P + sib** | **adopt (default)** |
| **B** delegate to a foreign `qijson.Decoder` | Feasible (`NewMethodSet`, verified); CWD caveat | none (exact) | fine | none | **needs both** `DecodeJSON`+`EncodeJSON`; one-sided ⇒ error | 3P/sib *iff* foreign pkg is qi-json-aware | **adopt (exact path + safety guard)** |
| **C** new exported "bind a nested `Decoder`" constructor | N/A (runtime) | none | N/A | **yes — one/two exported ids** | enforce by construction (require both directions) | enables hand-writers; backs B | **adopt (small amendment)** |
| **D** caller-supplied schema/codec (`-foreign=…`) | Feasible; no introspection needed — immune to the CWD caveat | whatever the consumer's schema binds | fine (flag is deterministic input) | none (a CLI flag) | require a two-directional schema, not a bare decoder | **3P + sib** | **adopt (escape hatch)** |
| **E** foreign package exports its schema; consumer imports it | importing the schema needs it *exported* | none | fine | **yes (foreign exported id)** | fine | **sib only** | **reject — collapses to B** |
| **F** combinations | — | — | — | — | — | — | **chosen (see §11)** |

Notes:

- **A** is the general default and the only thing that closes the recorded
  blocker for a plain struct. Its cost is reimplementing `encoding/json`'s
  field-selection for foreign structs (v1 can reject embedded and take the simple
  exported-fields view) and the loss of unexported state. It extends the existing
  cross-package scalar path (`crossNamedCodec` → `crossNamedCodec` for structs).
- **B** must win over A whenever the foreign type is qi-json-aware, otherwise A
  mis-binds it. So B is a **correctness requirement**, and it needs both methods to
  preserve §3's decode/encode symmetry.
- **C** is the smallest amendment that gives hand-writers the same one-liner the
  generator emits, and it is B's emission target. Sketch:
  `ObjectDecoder[T, S any, PS interface{ *S; Decoder; Encoder }](name string, field func(*T) *S) Field[T]`
  (mirroring `Marshal[T, PT]`'s constraint-inference style), a `*S` variant, and a
  `DecoderCodec[S, PS]() Codec[S]` for containers. Exact shape pinned in the
  follow-up plan.
- **E** is not a distinct option: the only version that works for an arbitrary
  dependency is "call the foreign type's methods", i.e. B. Requiring an *exported*
  foreign schema is a larger amendment and breaks the unexported-`<t>Schema`
  convention.
- **D** is the only candidate fully immune to the CWD caveat, because it removes
  introspection entirely.

---

## 11. Recommendation

**Choose F — a layered design: A + B + C, with D as the escape hatch; reject E.**

- **B + C first (small, exact, independent of field discovery).** C adds one or
  two exported constructors (the amendment); B makes the generator detect a
  foreign `*S` implementing `Decoder` **and** `Encoder` (via `types.NewMethodSet`)
  and emit the C one-liner. This fixes the qi-json-aware case exactly and,
  critically, prevents A from mis-binding such types.
- **Then A (the default, and the blocker-closer).** For a foreign struct that is
  *not* qi-json-aware, synthesize `<pkg><Type>Schema` + exported-field accessors in
  the consumer package, reusing the numeric/scalar/container logic; memoize
  transitively; reject embedded in v1; skip unexported fields per §9.3.
- **D as the escape hatch.** A `-foreign=pkg.T=ident` flag lets the consumer point
  the generator at a schema it owns, covering a truly third-party module that
  cannot be introspected (the CWD caveat) and a foreign shape ≠ its exported
  fields.
- **Selection tree at a foreign struct field:** *implements Decoder+Encoder* → B;
  *plain struct* → A; *specified by `-foreign`* → D; *otherwise* (e.g. `uuid.UUID`
  = `[16]byte`, an unexported foreign type, a one-sided `Decoder`) → the precise
  existing error.

**Rejected:** E (redundant with B). **Not adopted in v1:** binding embedded
promotion inside a foreign struct, and any `encoding/json` hook support.

**Constitution amendment (one):** Article VI's decoder/encoder surface gains C's
nested-decoder constructor(s); A/B/D add no exported identifiers (generated code
and a CLI flag). The amendment text and version bump are drafted in the follow-up
plan.

**Reasons A is worth its cost:** it is the only path that makes the recorded
scenario (`.opencode/plan/user-scenarios.md` §3.4) generate and round-trip without
hand-written closures, and it reuses machinery (`go/types` resolution, numeric
closures, container helpers, the drift guard) that already exists.

**Reasons B is not optional:** without it, A silently produces a lossy, wrong
binding for any qi-json-aware foreign type — a correctness bug, and a direct
violation of §3's "decoder and encoder must stay symmetric" in spirit.

---

## 12. Handoff

Implementation plan (TDD task list, fixtures, amendment, verification):
**`.opencode/plan/2026-11-22-foreign-struct-fields-impl.md`**.

Files a following session will touch first:
`cmd/qi-json-gen/types.go` (`typecheck`, `namedOf`, `crossNamedCodec`,
`crossElemCodec`), `cmd/qi-json-gen/gen.go` (`fieldCodec`, `starCodec`,
`elemCodec`, `objectCodec`, `renderType`), `cmd/qi-json-gen/main.go` (the new
flag), the `qijson` root (the new constructor + `field.go`/`unmarshal.go` docs),
`docs/constitution.md` (Article VI amendment) and `cmd/qi-json-gen/internal/gentest`
(the fixture + drift guard).