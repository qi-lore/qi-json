# Scanner consolidation

**Branch**: `chore/scanner-consolidation` (from `main`, targets `dev`)
**Related**: `docs/constitution.md` v3.11.0 (Article VI, Additional Constraints);
`.opencode/plan/2026-11-22-internal-codec-extraction.md` §7, §11;
`.opencode/plan/2026-11-21-json-scanner.md`;
`.opencode/plan/2026-11-21-scanner-rewrite.md`.

This file is a design record. Task state lives in `docs/tasks.md`.

## 1. Goal

There is one public import path for the module — `github.com/qi-lore/qi-json` —
and the syntax engine is an implementation detail like the semantic engine
already is. Concretely: move `scanner/` to `internal/scanner/` (the package name
stays `scanner`), and re-export the former public surface through the `qijson`
facade so no capability and no call site changes shape — only the import path.

This is the deferred Option B of
`.opencode/plan/2026-11-22-internal-codec-extraction.md` §7 (that plan kept the
scanner public to avoid a breaking change; §11 recorded it as future work). It is
a deliberate **breaking change**.

## 2. Public surface decision

The scanner's public surface today is `Valid`, the `Kind` type with its seven
constants (`KindNull`, `KindBool`, `KindString`, `KindNumber`, `KindObject`,
`KindArray`, `KindEnd`), and `Tokenizer` with `NewTokenizer`. After the move the
facade re-exports **all three**, so the migration is purely `scanner.X` →
`qijson.X`:

- **`Kind` + constants — mandatory.** `qijson.Reader.Peek()` returns the token
  kind, and any hand-written `Field.Decode`/`Decoder` that branches on the JSON
  value shape compares it (`if k == qijson.KindNull { ... }`). Generated code
  does exactly this. The return type of a public method cannot be hidden.
- **`Valid` — kept.** It is the project's `encoding/json.Valid`-shaped syntax
  validator: validate untrusted input or gate a pipeline without importing the
  semantic layer, with a target-free, stateless call.
- **`Tokenizer`/`NewTokenizer` — kept.** The value-free streaming API for a
  minifier, a JSONL splitter, an analyzer, or a hand-rolled decoder that wants
  the same error contract with incremental control.

Re-exporting (rather than dropping) keeps the change to an import-path rewrite
and does not remove a documented feature. The alternative — publishing only the
`Kind` taxonomy and dropping `Valid`/`Tokenizer` — was rejected: it buys no extra
freedom (the scanner's internals are already unexported) while costing
capabilities the README advertises.

Type identity is preserved by aliases, exactly as with `internal/codec`:
`qijson.Kind` **is** `scanner.Kind`, so `Reader.Peek()`'s return type is
unchanged across the boundary and `errors`-style comparisons keep working.

## 3. Target layout

```text
qi-json/
├── go.mod
├── doc.go                 # package narrative (unchanged role)
├── qijson.go              # facade: codec aliases + scanner re-exports
├── internal/
│   ├── codec/             # package codec — semantic engine (unchanged)
│   └── scanner/           # package scanner — syntax engine (was scanner/)
└── cmd/
    └── qi-json-gen/       # emits qijson.Kind*, no scanner import
```

## 4. Decisions

- **D1 — Package name stays `scanner`.** Only the import path changes; every
  in-repo reference `scanner.KindNull` etc. keeps its identifier. Internals
  (`validator`, `container`, `parseError`, `scan*`, `nestingInline`) are
  untouched and stay unexported.
- **D2 — `Valid`, `Kind` + constants and `Tokenizer` are re-exported** as
  aliases (`Kind`, `Tokenizer`) and one-line wrappers (`Valid`, `NewTokenizer`).
  No new engine internals become public.
- **D3 — The generator emits `qijson.KindNull`** and drops the
  `github.com/qi-lore/qi-json/scanner` import it used to add. Generated output
  stays deterministic; checked-in fixtures are regenerated.
- **D4 — Breaking change, migration note.** The `CHANGELOG.md` `[Unreleased]`
  entry and the README migration section record the single rewrite: change every
  `github.com/qi-lore/qi-json/scanner` import and `scanner.` qualifier to
  `github.com/qi-lore/qi-json` / `qijson.`, and regenerate `qi-json-gen` output.
- **D5 — No behavior change.** Same error strings, same 1-based positions, same
  allocation profile, same generator output modulo the qualifier and the dropped
  import.

## 5. Findings that shape the plan

### 5.1 The facade may import `internal/scanner`

`internal/` is importable by every package rooted at its parent — the whole
module, including the `qijson` root. So the facade imports both `internal/codec`
and `internal/scanner`. No module-boundary problem.

### 5.2 The generator's scanner import is the only emitted path

`g.needScanner` gates a `qijsonPath + "/scanner"` import line and is set by the
scalar-closure, numeric-closure and foreign-pointer-closure builders. Once those
emit `qijson.KindNull`, the field and the import line are removed. The `separate`
`isQijsonType` alias handling (for `Reader`/`Writer`) is unaffected.

### 5.3 `go doc` does not follow aliases (carried from the codec move)

For `type Kind = scanner.Kind`, `go doc qijson.Kind` prints the alias and its own
doc comment, not the engine methods. Mitigation is the same as for the codec
facade: put the full public doc comments on the facade aliases.

### 5.4 The constitution and `AGENTS.md` move together

The package list and the sanctioned surface change, so Article VI and Additional
Constraints are amended, the version bumps to v3.11.0, and `AGENTS.md` is
updated in the same change.

### 5.5 `Valid`'s hot-path invariant follows the package

`TestValidDoesNotAllocate`, `FuzzValid` and `BenchmarkValid*` move with the code
to `internal/scanner`; the `scanner` test files are already `package scanner`, so
no test edit is needed beyond the directory move.

## 6. Migration (recorded in the README and CHANGELOG)

```diff
-import "github.com/qi-lore/qi-json/scanner"
+import "github.com/qi-lore/qi-json"

-ok := scanner.Valid(data)
-tok := scanner.NewTokenizer(data)
-if k == scanner.KindNull { ... }
+ok := qijson.Valid(data)
+tok := qijson.NewTokenizer(data)
+if k == qijson.KindNull { ... }
```

Any `qi-json-gen`-generated file must be regenerated (it no longer imports
`scanner`).

## 7. Risks and rollback

- **Alias/doc regression** (§5.3): full facade docs on the aliases; verify
  `go doc -all .` and pkg.go.dev rendering.
- **Generator drift**: `go generate ./... && git diff --exit-code` must be empty.
- **A consumer that imported `scanner` directly**: documented breaking change
  with a migration note; pre-1.0, so it ships as a minor.
- **Rollback**: a pure move plus a facade; a revert restores the public
  `scanner/` package with no data migration.

## 8. Verification

The required sequence (constitution, Verification Workflow), plus:

1. `go test ./... -count=1`
2. `gofmt -l .` (empty)
3. `go vet ./...`
4. `golangci-lint run` (v2, 0 issues)
5. `go test -race ./... -count=1`
6. `go generate ./... && git diff --exit-code` (empty)
7. `go test -run=^$ -bench=. -benchmem ./...` (no regression on `BenchmarkValid*`)
