# qi-json — distributing the code generator (`cmd/qi-json-gen`)

**Date**: 2026-11-22
**Related**: `docs/constitution.md`, `AGENTS.md`, `README.md`,
`.opencode/plan/2026-11-21-json-codegen.md`,
`.opencode/plan/user-scenarios.md` §6.

This file is a design record. Task state lives in `docs/tasks.md`.

---

## 1. Why

A Go developer who imports `qi-json` can hand-write the accessor closures, but
the intended path for a struct with many fields is `cmd/qi-json-gen`. The
library cannot `go get` the generator into the environment: it is a `main`
package, so importing it does nothing. The user's ask is concrete — *"I want the
code generator command to live in my PATH"* — and until now the README only
showed `go run <module>/cmd/qi-json-gen`, with no install path and with a stale
module path from before the rename.

The gap is a **distribution** gap: the generator exists, is tested and is
drift-guarded, but there is no documented, minimal way to obtain the command,
and no prebuilt artifact for a user without a Go toolchain.

The scenario this closes is recorded as Scenario 3 (the generator on `PATH`) in
`.opencode/plan/user-scenarios.md`.

## 2. Options considered

Four ways to get `qi-json-gen` onto a machine, from "nothing to install" to "a
bare command on `PATH`":

| # | Mechanism | Version-pinned to the import? | PATH binary | Maintainer cost |
|---|---|---|---|---|
| A | `go:generate` + `go run <module>/cmd/qi-json-gen` | Yes, automatically (module graph) | No | Documentation only |
| B | Go 1.24 `tool` directive (`go get -tool`, `go tool`) | Yes (recorded in `go.mod`) | No | Documentation only |
| C | `go install <module>/cmd/qi-json-gen@vX.Y.Z` | Yes if pinned with `@vX.Y.Z` | Yes (`$GOBIN`) | Documentation only |
| D | Prebuilt release binaries + Homebrew cask | Chosen at install time | Yes | goreleaser config, release CI, a tap repo |

## 3. Decisions (2026-11-22)

- **D1 — Document A, B and C, and ship D.** A and B are the recommended default
  because they keep the generator version locked to the `qi-json` version in the
  consumer's `go.mod`, which is the version the generated code targets. C is the
  one-liner for a literal `PATH` binary. D serves a user with no Go toolchain
  and is what "out of the box" means for the bare command.
- **D2 — A prebuilt binary is published by GoReleaser, not by the module.**
  `.goreleaser.yaml` builds `./cmd/qi-json-gen` only (the library is consumed as
  a module) for linux, macOS and Windows on amd64 and arm64, with
  `CGO_ENABLED=0`, `-trimpath` and `-s -w`, into `tar.gz` (zip on Windows), plus
  a `checksums.txt`. `project_name` is `qi-json-gen` so artifact names are
  unambiguous.
- **D3 — The tap is a Homebrew cask, not a formula.** GoReleaser deprecated the
  `brews:` (formula) section in v2.10 in favour of `homebrew_casks:`, and a cask
  carries a prebuilt binary without a build-from-source recipe. The cask is
  pushed to `qi-lore/homebrew-tap` (`Casks/`), so `brew install
  qi-lore/tap/qi-json-gen` puts the command on `PATH`.
- **D4 — Release-time only; no runtime or module change.** GoReleaser and the
  release workflow are build tooling, like the existing golangci-lint action.
  They are not a Go dependency: `go.mod` is unchanged, Article III is untouched
  (the module still imports no third-party code), and the generated output is
  identical.
- **D5 — Trigger on the existing release tag.** `.github/workflows/release.yml`
  runs on the `v*` tag that the release flow already pushes onto `main`, so the
  binary version always equals the library version, with no new release step for
  the maintainer beyond the existing tag.

## 4. Files

- `.goreleaser.yaml` — the build, archive, checksum and cask configuration.
- `.github/workflows/release.yml` — tag-triggered GoReleaser run; creates the
  tap repository if it is missing.
- `Makefile` — `release-check` runs `goreleaser check` locally.
- `README.md` — an **Installing the generator** section (options A–D) and a fix
  of the stale `qi-json/cmd/qi-json-gen` module path in the `go:generate`
  directive and the direct-run examples.

## 5. Bootstrap and external steps

Everything but one credential is automated:

- **Automatic.** The release workflow's `ensure tap repository` step runs
  `gh repo create qi-lore/homebrew-tap` when the tap does not exist (GoReleaser
  pushes casks but does not create the tap), so the maintainer does not create
  the repository by hand; on later releases the step is a no-op.
- **Manual, once.** Add a repository secret `HOMEBREW_TAP_GITHUB_TOKEN` to
  `qi-lore/qi-json`: a fine-grained token scoped to `qi-lore/homebrew-tap` with
  Contents and Administration write. `GITHUB_TOKEN` is scoped to this repository
  and cannot push to another, and GitHub does not let a workflow create its own
  secrets, so this one credential cannot be bootstrapped from the repository.
- After the secret exists, a `v*` tag needs no manual step.

The fully self-hosted alternative — write the cask into this repository and rely
on the two-argument `brew tap <user>/<repo> <URL>` — needs no new secret but is
rejected: Homebrew's one-argument `brew tap` requires a `homebrew-` repository
name, so users would have to pass the URL, and the cask would have to land on
the protected default branch.

## 6. Risks

| Risk | Mitigation |
|---|---|
| GoReleaser schema drift between major versions | Config pins `version: 2`; the workflow pins `~> v2`; `make release-check` validates locally. |
| The tap steps fail when the token is missing | The secret is required and the workflow comment says so; the archives still upload, then the run fails at the tap. Prefer a fine-grained token scoped to the tap. |
| `@latest` in `go install` drifts from the imported version | The README recommends an explicit `@vX.Y.Z`, and A/B are the recommended default. |
| Homebrew may quarantine the downloaded binary on macOS | A known Homebrew behaviour for prebuilt CLI binaries; the `go install` and `go run` paths are unaffected. |
| A release tag already used before this workflow existed still triggers it | GoReleaser attaches artifacts to the existing GitHub release; re-running is idempotent with `--clean`. |
