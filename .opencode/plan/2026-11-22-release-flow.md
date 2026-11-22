# qi-json — release flow (document the release process)

**Date**: 2026-11-22
**Status**: done. The **Release flow** section is added to `docs/constitution.md`
(v3.6.0) and mirrored in `AGENTS.md`.
**Related**: `docs/constitution.md` (§ Git and Review Discipline, § Release
flow), `AGENTS.md` (§ Release flow), `CHANGELOG.md`,
`.opencode/plan/2026-11-22-git-and-code-quality-reglament.md`.

---

## 1. Context

The v0.2.1 release was cut end-to-end (see `git log` around `f8fb433`,
`68c1dd6`): the version was chosen, the changelog and README were prepared on
`dev`, a `dev` → `main` pull request was merged, and `v0.2.1` was tagged on the
merge commit. The process was **not written down anywhere** — it lived only in
the repository history and the maintainers' heads. Two concrete problems came
out of that run:

- **`dev` was deleted** on the remote right after the `dev` → `main` merge. The
  two-branch model depends on `dev` as the permanent integration branch, so
  every later feature pull request lost its legal target; `dev` had to be
  recreated at `main`.
- **Tag placement** was ambiguous: the tag belongs on the **merge commit** on
  `main`, not on the release-preparation commit on `dev`.

The Git and Review Discipline section of the constitution already covered
branching, commits, pull requests and CI, but it stopped at the pull request; a
release had no documented steps.

## 2. What was added

- `docs/constitution.md` — a **Release flow** subsection under **Git and Review
  Discipline**, as an ordered list:
  1. choose the version (SemVer; pre-1.0: a backward-compatible fix or
     documentation change is a patch, a new or changed capability is a minor);
  2. prepare on `dev` (promote `CHANGELOG.md` `[Unreleased]`, update the
     `README.md` version block, do the housekeeping, commit
     `chore(release): vX.Y.Z`, run the Verification Workflow);
  3. open the `dev` → `main` pull request and let CI pass;
  4. merge into `main`;
  5. tag the **merge commit** on `main` with `vX.Y.Z` and push the tag;
  6. **keep `dev`** — the integration branch is permanent.
  The constitution is bumped **3.5.0 → 3.6.0** (a new procedural rule, so a
  minor amendment; the Governance section requires `AGENTS.md` to change in the
  same amendment).
- `AGENTS.md` — a matching **Release flow** section, and this plan indexed in
  the Context list.
- `CHANGELOG.md` — an `[Unreleased]` **Added** entry, consistent with how the
  earlier governance change was recorded.

No Go code changed, so the module and its public surface are untouched.

## 3. Verification

- `make verify` (test → gofmt → vet → golangci-lint): green, 0 lint issues.
- Documentation-only change; no test or fixture change.

## 4. References

- The release this documents: PR #3 (`dev` → `main`), merge commit `68c1dd6`,
  tag `v0.2.1`, preparation commit `f8fb433 chore(release): v0.2.1`.
- The governance rules it extends:
  `.opencode/plan/2026-11-22-git-and-code-quality-reglament.md`.
