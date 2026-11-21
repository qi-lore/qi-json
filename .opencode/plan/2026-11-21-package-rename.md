# qi-json — rename the `marshal` package

**Date**: 2026-11-21
**Status**: **proposed** — recorded, not started. Needs a name decision (this doc)
and a constitution amendment before any code.
**Related**: `AGENTS.md`, `docs/constitution.md` (Article VI), `marshal/doc.go`,
`.opencode/plan/2026-11-21-post-decoder-backlog.md` §9.

---

## 1. Why

`marshal` names only half of what the package does. In Go, "marshal" is the
**encode** direction (`encoding/json.Marshal`), yet the package's essence is
**both** directions: it is the reflection-free semantic layer that decodes JSON
into Go values *and* encodes Go values back to JSON. `AGENTS.md` calls it "the
reflection-free semantic layer (the decoder and encoder)". The current name is
therefore misleading, and it is asymmetrical with the package's own API, which
exports `Marshal`/`Unmarshal`, `Encoder`/`Decoder`, and `Writer`/`Reader` in
equal measure.

The sibling package is `scanner` — the **syntax** layer. The rename should name
the **semantic** layer so it pairs with `scanner` and captures the bidirectional
essence.

## 2. Naming

Criteria: (a) means *both* encode and decode; (b) idiomatic Go; (c) not a copy of
an `encoding/json` identifier (constitution VI); (d) reads well in call position
(`x.Marshal(...)`, `x.Schema`, `x.Reader`); (e) ideally no collision with the
existing exported `Codec[E]` type.

| Candidate | Rationale | Con |
|---|---|---|
| **`codec`** (recommended) | Literally *coder–decoder*: one word for both directions; standard Go use for a JSON ⇄ value layer; pairs conceptually with `scanner`. | Overlaps the existing `Codec[E]` type (`codec.Codec`), so that type should be renamed too (see §3). |
| `serde` | Serialisation **+** deserialisation, exactly bidirectional; `serde.Codec[E]` composes cleanly. | Borrowed (Rust) term, not idiomatic Go. |
| `binding` | Names the defining technique — compile-time accessor-closure binding. | Leans decode-only ("bind input"), under-sells encoding. |
| `mapping` | JSON ⇄ Go mapping. | Vague; close to Go's `map`. |

**Recommendation: `codec`.** It is *decode + encode* in one idiomatic word and
pairs with `scanner` (syntax vs. codec). Its only cost is the overlap with the
element type `Codec[E]`; resolve it by renaming that type to **`Element[E]`**
(`codec.Element[E]`, `codec.NewElement(...)`), which also reads better — it is
the decode/encode closure pair for a *single element type*. If a single rename is
preferred, **`serde`** avoids the overlap entirely (`serde.Codec[E]`).

Call sites afterwards: `codec.Marshal(cfg)`, `codec.Unmarshal[Config](data)`,
`codec.Decode(data, v)`, `codec.Encode(w, v)`, `codec.Schema`/`codec.Struct`,
`codec.Reader`/`codec.Writer`, `codec.Encoder`/`codec.Decoder`.

## 3. Scope

Mechanical but wide; it must land in **one commit** (the module will not build at
an intermediate step):

- Rename the directory `marshal/` → `codec/` and the `package marshal` clause in
  every file.
- Update the import path `qi-json/marshal` → `qi-json/codec` everywhere:
  `cmd/qi-json-gen` (generator and emitted import/qualifier), the `gentest`
  fixtures and their checked-in generated files, `README.md`, `AGENTS.md`,
  `docs/constitution.md`, and the plans' pointers.
- Regenerate the fixtures (`make generate`); the generator emits the new import
  path and `codec.` qualifier.
- `Makefile`, `go:generate` directives, and the tool's usage strings.
- If `Codec[E]` → `Element[E]`: update `marshal/field.go`
  (`Slice`/`Map`/`Array` signatures), `marshal/encode.go` (`NewCodec`), the
  generator's `marshal.NewCodec(...)` emissions, and all tests.
- `CHANGELOG.md`: an entry under Unreleased noting the import-path break (the
  module is pre-1.0 and deliberately not `encoding/json`-compatible).

## 4. Governance

The constitution names `marshal` explicitly — Article VI ("the `marshal`
decoder", "the `marshal` encoder") and Additional Constraints ("`marshal` and
`cmd/qi-json-gen` use neither `reflect` nor `unsafe`"). The rename is therefore a
**constitution amendment** (a rename, no new surface) plus a footer version bump,
done in the same change, with `AGENTS.md` updated to match.

## 5. Verification

The mandated sequence (`make verify`, then `go test -race ./...`), plus
`git grep '\bmarshal\b'` to prove no stale reference remains outside the
historical plan docs (which are a log and stay as written, with a one-line note).

## 6. Subtasks (when started)

- **R1 — decide and sign off** the final name (this doc); note the Article VI
  amendment and `AGENTS.md` update as prerequisites.
- **R2 — rename** the package/dir; update the generator and regenerate the
  fixtures; fix tests (red → green). One commit.
- **R3 — docs**: `README.md`, `CHANGELOG.md`, `marshal/doc.go`; a superseded-note
  in the plan docs that name the old package.
- **R4 — governance + verify**: amend Article VI and the footer version, run the
  full mandated verification, commit (no push).
