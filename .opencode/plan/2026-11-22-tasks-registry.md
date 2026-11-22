# qi-json — retire `docs/task.md` and introduce the task registry

**Date**: 2026-11-22
**Branch**: `chore/tasks-registry` (targets `dev`)
**Related**: `docs/constitution.md`, `docs/tasks.md`, `AGENTS.md`, `README.md`,
`CHANGELOG.md`, `docs/task.md` (deleted).

This file is a design record. Task state lives in `docs/tasks.md`.

---

## 1. Problem

Two documentation problems.

1. **`docs/task.md` is obsolete.** It is a 491-line scanner specification whose
   contract is already pinned by the test suite (exact error strings and 1-based
   positions), by the constitution (Articles I, III, IV), and by `AGENTS.md`'s
   "Contract details that are easy to break". Only §12 (Verification) carries
   information not already present in `docs/`: the extra verification commands
   (`-race`, single test, fuzz, benchmarks) and the `Valid` allocation invariant.
2. **Task state is scattered.** Task status lives in eleven plan files under
   `.opencode/plan/`, in `AGENTS.md`, and in the CHANGELOG. There is no single
   place to answer "what is done and what is left".

## 2. Decisions

### D1 — Delete `docs/task.md` after moving its unique content

Every block was checked against a surviving authority:

| Block | Preserved where |
|---|---|
| §1–§2 purpose and role | Constitution Articles I, III; `AGENTS.md` |
| §3–§5 core principles, surface, grammar | Constitution Articles I, III, IV, VI; tests |
| §6–§8 scanner design and scalars | Tests; `AGENTS.md` "Contract details…" |
| §9 error contract | Tests; `AGENTS.md` |
| §10 invariants | Constitution Articles IV and V |
| §11 file layout | `AGENTS.md`; `README.md` "Repository layout" |
| §12 verification | **moved** into the constitution's Verification Workflow |

Only §12 needed new text: the extra commands and the `Valid` allocation
invariant. The rest was already duplicated or is enforced by tests.

### D2 — `docs/tasks.md` is the single source of truth for task state

Fixed line format, one task per line:

```text
- [X] <PREFIX><NNN> - <SHORT_DESCRIPTION>   completed
- [ ] <PREFIX><NNN> - <SHORT_DESCRIPTION>   planned
```

Prefixes name the area (`S` scanner, `D` decoder, `E` encoder, `G` generator,
`F` generator foreign-struct fields, `R` refactor, `Q` code quality, `M` module
and governance, `P` process). Numbers are zero-padded and allocated in completion
order; identifiers are never renumbered or reused. Legacy identifiers from the
plan files (`T4`, `T-Q1`, `T-F3`, …) are dropped — the registry line is the
identifier from now on, and the plan files keep their historical identifiers only
in prose.

### D3 — Constitution amendment v3.9.0

New **Task Tracking** section:

- Tasks MUST be recorded only in `docs/tasks.md`.
- Task status and backlogs MUST NOT appear in `.opencode/plan/` files or in
  `AGENTS.md`.
- `docs/tasks.md` MUST be updated before every commit.
- Plan files are design rationale and history only.

Article I changes from "`docs/task.md` is the specification" to "the tests are the
specification"; the Governance section's "if code and tests disagree" rule now
points at the tests instead of the deleted file.

### D4 — One commit

The new rule requires `docs/tasks.md` to be updated before every commit. A single
docs commit satisfies it without artificial splitting of one coherent change.

## 3. Changes

- `docs/tasks.md` — new registry: legend, completed tasks by area, planned tasks.
- `docs/task.md` — deleted.
- `docs/constitution.md` — Article I reworded; Article V's cross-reference
  retargeted; Verification Workflow gains the extra commands and the allocation
  invariant; new **Task Tracking** section; Governance rule retargeted; version
  bumped to 3.9.0.
- `AGENTS.md` — `docs/task.md` references removed; the Context section stops
  carrying task status and points at `docs/tasks.md`; a **Task tracking** section
  mirrors the constitution.
- `README.md` — the "Independent implementation" bullet and the repository layout
  table updated.
- `goversion_test.go` — comments and the failure message no longer cite
  `docs/task.md`.
- `CHANGELOG.md` — an `[Unreleased]` entry.

## 4. Verification

`make verify` (test → gofmt → vet → golangci-lint), plus a repository-wide grep
proving no surviving document points at `docs/task.md`.
