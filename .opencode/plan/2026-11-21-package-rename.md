# qi-json — relocate and rename the public API package

**Date**: 2026-11-21 (decision updated 2026-11-21)
**Related**: `AGENTS.md`, `docs/constitution.md` (Article VI), `doc.go`,
`.opencode/plan/2026-11-21-post-decoder-backlog.md` §9.

This file is a design record. Task state lives in `docs/tasks.md`.

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

Two problems, one change: the name is wrong, and the public API lives one
directory too deep. The library should sit at the **module root** so a caller
writes `import "qi-json"` and reaches the API directly, while the syntax layer
stays a subpackage that pairs with it.

## 2. Decision (2026-11-21)

- **Destination — the repository root.** The package's files move from
  `marshal/` to the module root (or, equivalently, are created there). The module
  root currently holds no Go package; this change introduces one, so callers
  `import "qi-json"` instead of `"qi-json/marshal"`.
- **Package clause — `qijson`.** The import path is fixed at `qi-json` by the
  module path, but `qi-json` is **not a valid Go identifier** (hyphens are illegal
  in identifiers), so `package qi-json` will not compile. The root package is
  therefore declared `package qijson`, giving call sites
  `qijson.Marshal(...)`, `qijson.Unmarshal[Config](data)`, `qijson.Schema`,
  `qijson.Reader`, and so on. (This is the same situation as any hyphenated
  module whose root is a library, e.g. `go.uber.org/zap` declares
  `package zap`.)
- **Only the semantic layer moves.** `scanner` stays a subpackage
  (`qi-json/scanner`). The root is exactly one package, so the syntax/semantic
  split is preserved rather than merged.
- **`Codec[E]` keeps its name.** The only earlier reason to rename it to
  `Element[E]` was the awkward `codec.Codec`; `qijson.Codec[E]` reads fine, so the
  element type is unchanged and the rename is smaller.

Call sites afterwards: `qijson.Marshal(cfg)`, `qijson.Unmarshal[Config](data)`,
`qijson.Decode(data, v)`, `qijson.Encode(w, v)`, `qijson.Schema`/`qijson.Struct`,
`qijson.Reader`/`qijson.Writer`, `qijson.Encoder`/`qijson.Decoder`,
`qijson.Codec[E]`, with `scanner.Valid` still at `qi-json/scanner`.

## 3. Scope

Mechanical but wide; it must land in **one commit** (the module will not build at
an intermediate step):

- Move `marshal/*.go` (source and tests) to the module root and change the
  `package marshal` clause to `package qijson` in every file. The root gains its
  first Go package.
- Update the import path `qi-json/marshal` → `qi-json` everywhere:
  `cmd/qi-json-gen` (the generator and the emitted import/qualifier), the
  `gentest` fixtures and their checked-in generated files, `README.md`,
  `AGENTS.md`, `docs/constitution.md`, and the plans' pointers.
- Update every qualifier `marshal.` → `qijson.` in Go source (fixtures,
  generated files, the generator's emitted text).
- Regenerate the fixtures (`make generate`); the generator emits the new import
  path and `qijson.` qualifier.
- `Makefile`, `go:generate` directives, and the tool's usage strings.
- `CHANGELOG.md`: an entry under Unreleased noting the import-path break (the
  module is pre-1.0 and deliberately not `encoding/json`-compatible).

## 4. Governance

The constitution names `marshal` explicitly — Article VI ("the `marshal`
decoder", "the `marshal` encoder") and Additional Constraints ("`marshal` and
`cmd/qi-json-gen` use neither `reflect` nor `unsafe`"). The rename-and-relocate is
therefore a **constitution amendment** (a rename and a relocation, no new
surface) plus a footer version bump, done in the same change, with `AGENTS.md`
updated to match.

## 5. Verification

The mandated sequence (`make verify`, then `go test -race ./...`), plus
`git grep '\bmarshal\b'` to prove no stale reference remains outside the
historical plan docs (which are a log and stay as written, with a one-line note).

## 6. Subtasks (when started)

- **R1 — decide and sign off** (done 2026-11-21): destination = repo root, package
  clause = `qijson`, only the semantic layer moves, `Codec[E]` unchanged. Note the
  Article VI amendment and `AGENTS.md` update as prerequisites.
- **R2 — relocate and rename** the package (move `marshal/*.go` to the root,
  `package marshal` → `package qijson`); update the generator and regenerate the
  fixtures; fix tests (red → green). One commit.
- **R3 — docs**: `README.md`, `CHANGELOG.md`, `marshal/doc.go` (moved to the
  root); a superseded-note in the plan docs that name the old package.
- **R4 — governance + verify**: amend Article VI and the footer version, run the
  full mandated verification, commit (no push).

---

## 7. Results (R2–R4 done, 2026-11-21)

- **R2 — relocate and rename.** `marshal/*.go` (source and tests) moved to the
  module root; `package marshal` → `package qijson` in every file. The generator
  (`cmd/qi-json-gen/gen.go`, `types.go`) now emits `import "qi-json"` and the
  `qijson.` qualifier; `main.go`'s package doc, `gen_test.go`, and the `gentest`
  fixtures were updated to match, and the checked-in generated files were
  regenerated (`go generate ./...`); the drift guards are green.
- **Error prefix.** The diagnostic prefix changed from `marshal:` to `qijson:`
  (for example `qijson: nil target`), so the §5 `\bmarshal\b` audit is clean.
  Tests compare sentinels with `errors.Is`/`errors.As`, so no test churn.
- **R3 — docs.** `README.md` (package table, quick start, decoding/encoding
  rules, error inspection, layout), `CHANGELOG.md` (an Unreleased **Breaking**
  entry for the import-path break and rename), `docs/task.md` (the tokenizer note
  and the file layout), and `doc.go` (moved from `marshal/doc.go`) were updated.
  `scanner/doc.go` and `scanner/scan.go` comments now say "package qijson".
- **R4 — governance.** Article VI and Additional Constraints in
  `docs/constitution.md` name `qijson`; the footer is **3.3.0** (rename and
  relocation, no new surface). `AGENTS.md` matches. The rename plan is the
  rationale pointer added to Article VI.
- **Audit.** `git grep '\bmarshal\b'` leaves only: the historical plan docs and
  released CHANGELOG entries (a log, kept as written), and the changelog and this
  plan's own prose that deliberately names the old package. No live code, import,
  qualifier, or `go:generate` directive refers to `marshal`.
- **Verification.** `make verify` green (0 lint issues); `go test -race ./...`
  green.

Older plan docs that name the package by its former name are a historical log
and are kept as written; from this plan forward the package is `qijson` at
import path `qi-json`.
