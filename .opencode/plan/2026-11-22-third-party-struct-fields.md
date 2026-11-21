# qi-json — research: struct fields whose type is a struct from another package

**Date**: 2026-11-22
**Status**: **next task — research only, not started.** This is a release blocker
(`.opencode/plan/user-scenarios.md` §3.5, D1).
**Related**: `.opencode/plan/user-scenarios.md` §3, `docs/constitution.md`
(Article VI), `.opencode/plan/2026-11-21-json-codegen.md`,
`.opencode/plan/2026-11-21-json-encoder.md`,
`.opencode/plan/2026-11-21-post-decoder-backlog.md` §9.

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