# qi-json — align the Go-version floor (go.mod `go` directive vs documented 1.23)

**Date**: 2026-11-22
**Status**: done (2026-11-22); shipped in v0.2.1. **Superseded in part:** the
floor later moved from 1.23 to **1.24** with the facade's generic type aliases
(v0.3.0, `.opencode/plan/2026-11-22-internal-codec-extraction.md`), so the
`go 1.23.0` discussed below is the historical fix, not the current floor.
**Related**: `docs/constitution.md` (Additional Constraints), `docs/task.md` §3,
`AGENTS.md`, `.opencode/plan/user-scenarios.md` §2 / D3,
`.opencode/plan/2026-11-21-post-decoder-backlog.md` §9.

---

## 1. Why (the mismatch)

Three documents disagree about the supported Go version:

- `docs/constitution.md` (Additional Constraints) and `docs/task.md` §3 both state
  a **Go >= 1.23** floor.
- `go.mod` declares `go 1.27.3`.

Since Go 1.21 a dependency's `go` directive is a **hard minimum**, so the
*enforced* floor is 1.27.3 while the *stated* floor is 1.23 — four minor versions
apart. A 1.23 consumer is either silently upgraded (`go mod tidy` rewrites its
`go` line to 1.27.3) or hard-blocked
(`go: module qi-json requires go >= 1.27.3`). Evidence: `user-scenarios.md` §2 /
§5.1, and §5 below.

The source is genuinely 1.23-language-compatible, so `go 1.27.3` over-constrains
consumers.

## 2. Decision

**Make `go.mod` match the documented floor**: `go 1.23.0` + `toolchain go1.27.3`.

- The `go 1.23.0` line is the language floor; a consumer at 1.23.0 now satisfies
  it, so no silent upgrade and no block.
- The `toolchain go1.27.3` line names the toolchain this module prefers for its
  own development. A dependency's `toolchain` line is **ignored** by consumers,
  so it does not raise the consumer's floor; a developer building *this* module
  still gets 1.27.3 under `GOTOOLCHAIN=auto`. This makes the existing
  constitution clause ("Go >= 1.23 (toolchain in `go.mod`: 1.27.3)") literally
  true.
- Rejected: correcting the documented floor to 1.27.3. That would contradict two
  governing documents and needlessly drop genuine 1.23 support; the honest fix is
  to lower the enforced floor to the real one.

No new exported identifier and no new dependency; this is build metadata, so **no
constitution amendment** (a wording clarification only).

## 3. Subtasks (TDD)

- **G1 — guard test (red).** Add `goversion_test.go` (package `qijson`) asserting
  the `go.mod` `go` directive is exactly `1.23.0` and the `toolchain` directive is
  `go1.27.3`, with the documented floor named in the failure message. It fails
  against the current `go 1.27.3`.
- **G2 — fix (green).** Rewrite `go.mod` to `go 1.23.0` + `toolchain go1.27.3`;
  the guard passes.
- **G3 — docs.**
  - `docs/constitution.md`: clarify the Additional Constraints clause to name the
    `go` directive (`go >= 1.23`) and the `toolchain` line; patch version bump.
  - `README.md`: state the Go requirement (it states none today).
  - `CHANGELOG.md`: an `[Unreleased]` **Fixed** entry.
  - `AGENTS.md`, `user-scenarios.md` §2/D3, backlog §9: mark D3 done.
- **G4 — verify + evidence.** `make verify` in order, `go test -race ./...`, and a
  recorded before/after with a go1.23.0 consumer (both a genuine 1.23 toolchain
  and the default 1.27.3 toolchain).

**Acceptance**: `go.mod` reads `go 1.23.0` + `toolchain go1.27.3`; a `go 1.23.0`
consumer builds against the module on both a real 1.23 toolchain and a 1.27.3
toolchain with no rewrite and no block; the guard test, `make verify`, and
`-race` are green.

## 4. Governance

| New exported identifiers | New dependencies | Amendment? |
|---|---|---|
| none (a test) | none | no (wording clarification only) |

## 5. Verification (evidence)

Baseline reproduces `user-scenarios.md` §5.1 exactly (Apple M1, go1.27.3).

| Scenario | Before (`go 1.27.3`) | After (`go 1.23.0` + `toolchain go1.27.3`) |
|---|---|---|
| consumer `go 1.23.0`, `GOTOOLCHAIN=go1.23.0 go build` | `requires go >= 1.27.3 (running go 1.23.0)` — fail | build + run OK; consumer `go.mod` unchanged |
| consumer `go 1.23.0`, default toolchain `go build` | `updates to go.mod needed` — fail | build + run OK; no rewrite |
| library itself, `GOTOOLCHAIN=go1.23.0 go test ./...` | fail (floor) | all packages `ok` |
| library itself, local go1.27.3 `go test ./...` | `ok` | `ok` |

## 6. Results (done, 2026-11-22)

- **G1/G2 — guard + fix.** `go.mod` is now `go 1.23.0` + `toolchain go1.27.3`
  (was `go 1.27.3`). `goversion_test.go` adds `TestGoModGoDirectiveFloor` (with a
  small stdlib-only `readGoModDirectives` helper) asserting the `go` directive is
  `1.23.0` and the `toolchain` directive is `go1.27.3`; it was red against
  `go 1.27.3` and is green now. No new exported identifier; no new dependency.
- **G3 — docs.** `docs/constitution.md` Additional Constraints now names the `go`
  directive as the floor and the `toolchain` line as development-only (a
  clarification, no version bump: the branch was integrated with `dev`, which had
  already raised the constitution to v3.5.0 with the Git and Review Discipline
  rules); `README.md` ("Using it") states the Go 1.23 requirement;
  `CHANGELOG.md` gains an `[Unreleased]` **Fixed** entry (kept alongside the
  `dev` `Added` entry); `AGENTS.md`, `user-scenarios.md` (§2.5, D3) and the
  backlog §9/§Status mark D3 done and index this plan.
- **Workflow.** The task was a `bugfix/go-version-floor` branch cut from `main`,
  targeting `dev` (Conventional Branch / Conventional Commits per the
  constitution's Git and Review Discipline). `dev` had advanced (PR #1,
  constitution v3.5.0), so `origin/dev` was merged in to resolve the overlap in
  `CHANGELOG.md` and the constitution footer.
- **G4 — verification.** `make verify` green (0 lint issues); `go test -race
  ./...` green; `GOTOOLCHAIN=go1.23.0 go test ./...` green on the real repository;
  `go generate ./...` is idempotent (no fixture drift). The §5 before/after table
  reproduces `user-scenarios.md` §5.1 exactly and matches the fix column.
- **Behavioral note.** The decoder, encoder, scanner and generator are unchanged;
  this is build metadata plus a documentation/guard change, so no fuzz run is
  required. The only new test is the go.mod guard.
