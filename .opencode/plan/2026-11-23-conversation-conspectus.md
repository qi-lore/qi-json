# Conversation conspectus — 2026-11-23

Design rationale and history only. Task state lives in `docs/tasks.md`; the
merged plan lives in `2026-11-23-roadmap-network-perf-parity.md`. This file
records what the working session covered, what it concluded, and what it left
open.

## Scope

The session compared qi-json with `encoding/json` and `mailru/easyjson`,
examined the library's streaming/network boundaries, benchmarked all three, and
merged the result into one feature+performance roadmap.

## Thread 1 — public-surface comparison (Encoding/Decoding families)

- qi-json and `encoding/json` share verbs (`Marshal`/`Unmarshal`/`Valid`) but
  qi-json is generic (`[T, PT *T Encoder/Decoder]`) instead of `any`, and
  reflection-free via `Schema`/`Field`/`Codec` closures instead of tags.
- Names are inverted: `qijson.Decoder`/`Encoder` are the *self-serialization*
  interfaces (≈ `Unmarshaler`/`Marshaler`); `qijson.Reader`/`Writer` are the
  stream cursors (≈ `json.Decoder`/`Encoder`).
- `qijson` adds `UnmarshalInto`/`Decode` (merge) alongside fresh `Unmarshal`;
  `encoding/json` has no such split.

## Thread 2 — single-document, in-memory boundaries

Enforced boundaries, with code evidence:

- Input is one complete `[]byte`; no `io` import anywhere.
- Exactly one top-level value: `Decode` does a trailing `Peek`; the tokenizer
  returns `after top-level value` at depth 0 (`token.go`).
- `Writer` allows one root (`wroteRoot`/`errMultipleRoots`); `Marshal` guards
  completeness (`errIncomplete`).
- No incremental output, no formatting utilities (`Indent`/`Compact`/
  `HTMLEscape`/`SetIndent`/`SetEscapeHTML`), entry points do not report consumed
  bytes, cursors are single-goroutine.

## Thread 3 — HTTP

- qi-json handles JSON bodies only by buffering: `io.ReadAll` → `Unmarshal`/
  `Decode`; `Marshal` → `w.Write`. A compiled, vetted handler was produced.
- No streaming/SSE/NDJSON; cap size with `MaxBytesReader`/`LimitReader`; map
  `*qijson.Error.Kind` to status codes.
- Documented as an explicit non-goal ("an `io` adapter is a later, additive
  change").

## Thread 4 — reformulated goal

> "how to fix all semantic and conceptual gaps to make it production ready with
> full network compatibility, with main syntax identical to `encoding/json`,
> while keeping a performance advantage over stdlib?"

Core conclusion: **identical `encoding/json` syntax requires runtime type
inspection, so it is mutually exclusive with the current no-`reflect` contract.**
Any literal-parity path starts with an Article III/VI amendment. This is the one
unavoidable fork; the thread proposed Track A (reflection-free, additive,
network + perf) vs Track B (parity, amendment).

## Thread 5 — three-way comparison

- `encoding/json` v1: reflection; the only fully bidirectional streamer; rich
  concrete error types.
- easyjson v0.9.2: codegen + `sync.Pool` + `unsafe` + `josharian/intern`;
  imports `encoding/json`; streams output only (input is `ReadAll`).
- qi-json: zero deps, no `reflect`/`unsafe`; strictest; gaps = no `io`/multi-value
  streaming, no stdlib syntax, no formatting utilities.

Borrow-selective list: pooling + `DumpTo` + unknown-fields proxy + omit hooks
(easyjson); `io.Reader` decode + multi-value + `Token()` + formatting
(`encoding/json`); keep scanner, path-tracked `*Error`, reflective-free `Schema`.

## Thread 6 — benchmark (external module `/private/tmp/opencode/bench3`)

Apple M1, Go 1.27.3, median of 3, `-benchmem -benchtime=1s`. Same bytes for all
decoders; qi-json uses `UnmarshalInto` to match merge semantics.

| case | v1 | v2 | easyjson | qijson |
|---|---|---|---|---|
| marshal 1 (ns) | 750.9 | 848.1 | 383.1 | 517.9 |
| marshal 1 (B) | 1216 | 1216 | 1008 | 744 |
| unmarshal 1 (ns) | 1069 | 931.7 | 527.2 | 1116 |
| unmarshal 1 (B) | 280 | 280 | 168 | 1480 |
| marshal 100 (ns) | 45172 | 53849 | 20130 | 38850 |
| marshal 100 (B) | 39229 | 39229 | 20057 | 84944 |
| unmarshal 100 (ns) | 100675 | 87536 | 58667 | 101300 |

Findings: easyjson wins all cases; qi-json marshal is fast and most compact per
record, but unpooled (slice marshal ~2x bytes); qi-json decode ties v1 but loses
to v2/easyjson because it copies strings and enforces strict UTF-8.

## Thread 7 — merged roadmap

The "borrow-selective" list and the "perf levers" are the same item set; e.g.
pooling (A1) == easyjson borrow E1 == perf lever L1. Merged into:

- **Track A (reflection-free, surface amendment only):** A1 pooling, A2 writer
  reuse, A3 decode string/UTF-8 fast path, A4 capacity hint, A5 `DumpTo`, A6
  `NewDecoder`+multi-value, A7 `Token()` streaming, A8 formatting, A9 omit hooks,
  A10 unknown-fields proxy. ~6–10 weeks.
- **Track B (parity, Article III/VI amendment):** `any`-based entry points + tag
  binding + `Marshaler`/`Unmarshaler` + error types. ~3–5 months.

## Decisions / positions taken

- Do **not** chase literal `encoding/json` syntax unless a surface amendment is
  explicitly approved; Track A delivers network+production without it.
- The perf gap is closable without `unsafe`; the residual easyjson edge is
  zero-copy, which the constitution forbids.
- Recommended order: A-I/A-II (perf) → A-III (output stream) → A-VI (hooks) →
  A-IV (input stream) → A-V (token/format) → re-evaluate B.

## Open decision points

1. Approve the Article VI surface amendment for the Track A additions (M009)?
2. Amend for parity (Track B) or decline (M010)? — **Resolved 2026-11-26: declined
   (M010)**; see the roadmap's §8 "Decision (M010): Track B declined".
3. Prototype A-I (pooled writer) with a before/after benchmark first?

## Artifacts

- `.opencode/plan/2026-11-23-roadmap-network-perf-parity.md` — merged plan table.
- `/private/tmp/opencode/bench3/` — external benchmark harness (out of repo;
  Article III forbids `encoding/json`/third-party imports in the module).
- `docs/tasks.md` — planned tasks (see below).

## Registry entries proposed

`E002`, `E003`, `D011`, `D012`, `N001`–`N005`, `H001`–`H002`, `M009`, `M010`,
`A001` (gated on M010), `Q010`.
