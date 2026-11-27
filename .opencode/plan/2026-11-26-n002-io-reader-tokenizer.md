# N002: `NewDecoder(io.Reader)` with a refillable tokenizer source

Design rationale and history only. Task status lives in `docs/tasks.md`; this
file records the design of the reader-backed tokenizer entry point.

Date: 2026-11-26.

## Summary

N002 adds `qijson.NewDecoder(io.Reader) *Tokenizer` — the reader-backed
counterpart of `NewTokenizer([]byte)`. It reuses the existing `Tokenizer` type;
no new exported type is introduced. Article VI sanctions only the `NewDecoder`
name for this surface.

## Design

- The refillable source (`readerSource`, 4096-byte chunks) lives in
  `internal/scanner`.
- The tokenizer buffer grows by `append` and is never compacted. This keeps
  absolute offsets stable, so the existing offset-based scanners work
  unchanged.
- The slice-backed path is untouched (`src == nil`).

## Aliasing contract difference

Slices returned by `ReadString`/`ReadNumber`/`ReadRaw` alias a growing internal
buffer and are valid only until the next read. This differs from
`NewTokenizer`, whose slices alias the caller's data for the tokenizer's
lifetime.

## Known limitation (deferred)

The buffer grows with the document. Bounded-memory streaming belongs to N003.

## Verification

Verified with `make verify`, `go test -race`, and a differential comparison
against `NewTokenizer` over the shared valid/invalid document tables at multiple
chunk sizes.
