# Roadmap: borrow-selective (features) x performance levers

Design rationale and history only. Task status lives in `docs/tasks.md`; this
file records the merged analysis that joined two earlier artifacts:

- the three-way comparison (`qijson` vs `encoding/json` vs `easyjson`), and
- the comparative benchmark of the three libraries.

Date: 2026-11-23.

## 1. Inputs

Three sources were combined:

- **Borrow selectively** — features worth taking from `encoding/json` and
  `easyjson` while keeping the qi-json edge.
- **Performance levers** — the concrete fixes the benchmark showed are needed.
- **Benchmark baseline** (Apple M1, Go 1.27.3, median of 3, `-benchtime=1s`):

  | case | v1 | v2 | easyjson | qijson |
  |---|---|---|---|---|
  | marshal 1 record (ns/op) | 750.9 | 848.1 | 383.1 | 517.9 |
  | marshal 1 record (B/op) | 1216 | 1216 | 1008 | 744 |
  | marshal 1 record (allocs) | 7 | 7 | 6 | 10 |
  | unmarshal 1 record (ns/op) | 1069 | 931.7 | 527.2 | 1116 |
  | unmarshal 1 record (B/op) | 280 | 280 | 168 | 1480 |
  | unmarshal 1 record (allocs) | 9 | 9 | 8 | 13 |
  | marshal 100 (ns/op) | 45172 | 53849 | 20130 | 38850 |
  | marshal 100 (B/op) | 39229 | 39229 | 20057 | 84944 |
  | unmarshal 100 (ns/op) | 100675 | 87536 | 58667 | 101300 |
  | unmarshal 100 (B/op) | 48936 | 48936 | 48896 | 66072 |

Findings that drive the plan:

- easyjson wins every case (buffer pool + `unsafe` + codegen).
- qi-json marshal is fast and the most compact per record (744 B; no HTML
  escaping, compact writer), but is not pooled: `Marshal` returns a fresh
  `[]byte` and grows by `append`, so slice marshal is ~2x the stdlib bytes and
  marshal allocs are the highest.
- qi-json decode copies every string and runs strict UTF-8 validation, so it
  ties v1 but loses to v2 and easyjson on both time and bytes.

## 2. Constraint recap

Two distinct barriers, not one:

- **No `reflect` / no `unsafe` / no `encoding/json` / stdlib-only**
  (constitution Articles III and VI). Items that only touch the syntax/streaming
  layers, buffer management or string handling do NOT cross this line.
- **Public-surface freeze** (Article VI: "do not export anything else without a
  new amendment"). Any new exported name — even a reflection-free one — needs a
  surface amendment.

So the merged plan splits into two tracks:

- **Track A — reflection-free, network + production.** Closes the streaming,
  formatting, allocation and pooling gaps on the current closure-based API. Needs
  a surface amendment only.
- **Track B — `encoding/json` parity.** `any`-based entry points and tag binding
  in the stdlib shape. Needs the Article III/VI amendment, because runtime struct
  binding without reflection is impossible.

## 3. Unified table

Each row merges a borrow (feature) with the benchmark lever (performance) it
also satisfies.

| # | Item | Borrowed from | Gap closed | Constitution |
|---|---|---|---|---|
| A1 | `sync.Pool` buffer in `Writer`; `BuildBytes(reuse ...[]byte)` | easyjson (E1/E5) | perf: slice marshal 84 KB → ~20 KB, allocs 23 → ~3 | surface |
| A2 | Reuse writer in `Marshal` (`Reset` + pooled buffer) | easyjson, perf lever L2 | perf: marshal allocs 10 → ~3, no behaviour change | none |
| A3 | Decode fast path: skip re-copy of unescaped strings + bounded UTF-8 check | perf lever L3 | perf: decode 1480 B → ~400–600 B | none |
| A4 | Preallocate slice/map capacity from a length hint | perf lever L4 | perf: decode allocs 1111 → ~900 | none |
| A5 | `Writer.DumpTo(io.Writer)` / `ReadCloser()` | easyjson (E2) | feature: encode to a stream; no full-buffer materialize | surface |
| A6 | `NewDecoder(io.Reader)` + multi-value loop (`Decode` repeatedly) | encoding/json (J1/J2) | feature: network input streaming, NDJSON | surface |
| A7 | `Token()` streaming + `Number`/`Delim`/`Token` types | encoding/json (J3) | feature: streaming analysis / `UseNumber` | surface |
| A8 | `MarshalIndent`/`Indent`/`Compact`/`HTMLEscape`, `SetIndent`/`SetEscapeHTML` | encoding/json (J4/J5) | feature: formatting + escaping parity | surface |
| A9 | `Optional { IsDefined() bool }` / `IsZero { IsZero() bool }` hooks | easyjson (E4) | feature: user `omitempty`/`omitzero` logic | surface |
| A10 | Unknown-fields proxy (`UnmarshalUnknown`/`MarshalUnknowns`) | easyjson (E3) | feature: round-trip unknown members; complements `RejectUnknown` | surface |
| B1 | `Marshal(v any)` / `Unmarshal(data, v any)` stdlib-shaped entry points | encoding/json | parity: familiar syntax | **III/VI amnesty** |
| B2 | Tag-driven struct binding via cached per-type plan, deferring to `Schema` | encoding/json/v2 | parity + perf: reflection once per type, never per call | **III/VI amnesty** |
| B3 | `Marshaler`/`Unmarshaler` (`MarshalJSON`/`UnmarshalJSON`) interfaces | encoding/json | parity: drop-in with stdlib-shaped types | **III/VI amnesty** |
| B4 | Concrete error types (`SyntaxError`, `UnmarshalTypeError`, ...) | encoding/json | parity: `errors.As` targets | **III/VI amnesty** |
| K1 | Keep single-pass, allocation-free `Valid` scanner | (existing) | perf: the edge vs both stdlibs | none |
| K2 | Keep path-tracked `*Error` (`Path`, `Offset`, `Kind`) | (existing) | feature: better diagnostics than both | none |
| K3 | Keep reflection-free `Schema` as the fast path B2 defers to | (existing) | perf: guarantees beating stdlib for hot types | none |

Note: A1 == the benchmark lever L1 and the easyjson borrow E1; they were the same
item seen twice, and are merged.

## 4. Track A — phases, cost, expected delta

| Phase | Items | Dev cost | Expected effect |
|---|---|---|---|
| A-I Pooling + reuse | A1, A2 | 1–2 weeks | slice marshal 84 KB → ~20 KB; marshal allocs 10/23 → ~3/5 |
| A-II Decode cost | A3, A4 | 1–2 weeks | decode 1480 B → ~400–600 B; decode time toward v1/v2 |
| A-III Output stream | A5 | 3–5 days | encode to `io.Writer`; removes full-buffer materialize |
| A-IV Input stream | A6 | 2–4 weeks | `io.Reader` decode + multi-value; the real streaming work |
| A-V Token + format | A7, A8 | 1–2 weeks | `Token()` streaming, indent/compact/escape |
| A-VI Extension hooks | A9, A10 | 1 week | `Optional`/`IsZero`, unknown-field round-trip |

Track A total: **~6–10 weeks**. Outcome: full network compatibility
(stream both directions), most stdlib formatting/parity ergonomics, and a
performance profile close to easyjson — landing qi-json **at/above easyjson on
marshal** and **between stdlib and easyjson on decode**, with only the `unsafe`
zero-copy residual left unreachable.

Syntax note: Track A keeps the generic closure-based API; `Marshal`/`Unmarshal`
stay `[T, PT]`-shaped, not `any`. This is the "no reflection, still production
ready" answer.

## 5. Track B — parity (needs amendment)

| Item | Cost | Blocked by |
|---|---|---|
| B1–B4 | 3–5 months | Article III/VI amendment; major-version bump |

Track B is the earlier "re-implement `encoding/json/v2`" effort (~4–7
engineer-months total). It is the only way to get literal stdlib syntax, and it
erases the no-reflect identity. Recommend deferring until Track A ships and the
parity demand is proven.

## 6. Recommended sequence

1. **A-I + A-II** (perf, no amendment) — cheapest wins, immediately visible in
   the benchmark.
2. **A-III** (output stream) — small, enables HTTP response streaming.
3. **A-VI** (hooks) — small, closes easyjson feature parity cheaply.
4. **A-IV** (input stream) — the substantive streaming work.
5. **A-V** (token + format) — ergonomics.
6. Re-evaluate **Track B** only if stdlib-shaped syntax is a hard requirement
   (declined in M010 — see §8).

## 7. Risks

- A3 (UTF-8 fast path) must stay exact: the decoder is deliberately stricter
  than `Valid` and rejects invalid UTF-8; optimising must not weaken that.
- A4 (capacity hint) changes allocation, not semantics; guard with
  `TestValidDoesNotAllocate`-style allocation assertions.
- Surface additions (A5–A10) widen the public API; each needs its Article VI
  amendment and facade docs (ownership/concurrency/lifecycle, per the existing
  contract style).
- Any Track B step reverses a core principle and must be an explicit governance
  decision, not a byproduct.

## 8. Decision (M010): Track B declined

**Date:** 2026-11-26. **Status:** decided — Track B (B1–B4) is declined for now;
the no-reflect identity is preserved and Article III/VI are unchanged (a decline
is not an amendment).

**Rationale:**

- Track A delivers network and production readiness without reflection, so the
  "production ready" goal does not require stdlib-shaped syntax.
- Demand for literal `encoding/json` syntax is unproven; no consumer has made it
  a hard requirement.
- Track B reverses a core principle (no reflection), costs ~3–5 engineer-months,
  and needs an Article III/VI amendment plus a major-version bump — too much for
  an unproven benefit.

**Error inspectability (B4):** the library already ships `errors.As`/`errors.Is`-
friendly types — `*qijson.Error` (Path, Offset, Kind, Unwrap) and
`*qijson.TypeError` (Want, Got), plus `ErrorKind`, re-exported through the facade
(`qijson.go`) and exercised with `errors.As` in tests. Declining B4 therefore
costs only `encoding/json`-identical type names and shapes, not error
inspectability.

**Revisit trigger:** reopen Track B only if stdlib-shaped syntax becomes a hard
requirement for a real consumer.
