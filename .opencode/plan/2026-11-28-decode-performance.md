# Decode performance roadmap (Phase 0 + Phase 1)

Date: 2026-11-28
Status: active
Tasks: `Q015` (benchmark harness integrity) and `D013` (decoder allocation
elimination — established as Reader/tokenizer reuse, not container
pre-sizing). Later phases are described but not yet tasked.

## Goal

Make qi-json competitive on raw speed: match or beat `encoding/json` on both
encode and decode, then close on `easyjson`. This supersedes the earlier
"parity is not the pitch" framing — speed is now an explicit project goal,
decided with the maintainer on 2026-11-28.

## Where we stand

Measured 2026-11-28, Apple M1, Go 1.27.3, `-benchtime=150ms`, single run
(indicative, not a guarantee):

| Case | `encoding/json` | `easyjson` | qi-json |
|---|---|---|---|
| Marshal ×1 | 522 ns / 3 allocs | 390 ns / 6 | 442 ns / 6 |
| Unmarshal ×1 | 947 ns / 6 | 448 ns / 8 | 1080 ns / 5 |
| Marshal ×100 | 37.6 µs / 3 | 18.8 µs / 12 | 30.9 µs / 7 |
| Unmarshal ×100 | 88.1 µs / 509 | 51.6 µs / 808 | 93.3 µs / 212 |

Encode already beats the standard library; decode trails both, and `easyjson`
leads decode by a wide margin.

## Findings (`pprof`, `Unmarshal100/qijson`)

Allocation profile (`alloc_space`), by share of allocated bytes:

- **61.5%** — `bench.(*Records).DecodeJSON.func1`: the benchmark's top-level
  array decoder grows its slice with `append` and never pre-sizes.
- **27.4%** — `codec.Decode`: `string(data)` copies the **entire document** on
  every call (`internal/codec/decode.go`).
- **9.0%** — `codec.Slice.func1`: the per-record `tags []string` growth.

The CPU profile was noisy (`runtime.kevent` ~71% — the machine was not idle
during the run), so CPU attribution is not yet trusted; the allocation profile
is deterministic and is the basis for Phase 1.

**Key observation.** The library's own `Slice` field codec already pre-sizes via
`arrayLenHint` (`internal/codec/field.go`), but a hand-written top-level
`DecodeJSON` has **no public way** to pre-size. This is simultaneously a
measurement artifact and a genuine ergonomic gap.

## Plan

- **Phase 0 — `Q015` benchmark integrity.** Broader shapes, a reproducible
  methodology, an honest binding, and a written record of the top-level
  pre-size gap.
- **Phase 1 — `D013` decoder allocation elimination.** The per-call fixed cost
  proved to be the `*Reader` and its `*Tokenizer`; the change holds the
  tokenizer by value inside `Reader` and recycles the `Reader` through a
  `sync.Pool`. It did **not** pre-size containers or cut the whole-document
  copy: the top-level container slice growth is bench-side and needs a public
  length hint (an **Article VI amendment**), and the whole-document
  `string(data)` copy is a deliberate zero-copy optimisation that is retained.
  Measured (Apple M1, Go 1.27.3, n=10): `Unmarshal1/qijson` 1536 B / 5 allocs →
  352 B / 3 allocs; `Unmarshal100/qijson` 66.09 KiB / 212 → 64.95 KiB / 210
  allocs.
- **Phase 2 — decode CPU hot path.** Drop the per-value `cachedKind` offset/err
  checks, flatten the tokenizer↔Reader boundary, inline scalar dispatch,
  length-first key matching. *(not yet tasked)*
- **Phase 3 — encoder tightening.** Fewer allocations, inlined writes.
  *(not yet tasked)*
- **Phase 4 — generated fast path.** `qi-json-gen` emits straight-line
  decode/encode that bypasses `Reader`/`Writer` — the only route to
  `easyjson`-class speed. *(not yet tasked)*

## Decisions

- Speed is the explicit goal; **measure before optimizing** (Phase 0 precedes
  Phase 1).
- Phase 1 data (2026-11-28) shows internal pre-sizing alone does **not** close
  the decode gap: ~61% of `Unmarshal100` bytes is bench-side top-level slice
  growth that only a public length hint (an **Article VI amendment**) can
  remove, and the ~27% whole-document copy is retained by design. The
  amendment decision therefore remains open.
- Rough estimate: **~1 week** for Phases 0–1; **~1–2 months** for
  `easyjson`-class.

## Open questions

- Closing the decode gap for a hand-written top-level container needs a public
  length hint (Article VI amendment); nested containers already pre-size
  internally. The amendment decision is open.
- Is the Go standard library's `encoding/json/v2` (in the toolchain) a moving
  target we accept, and does it change the Phase 4 calculus?
