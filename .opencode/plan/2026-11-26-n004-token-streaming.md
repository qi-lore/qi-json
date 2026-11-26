# N004: `Token()` streaming with `Number`/`Delim`/`Token`

Design rationale and history only. Task status lives in `docs/tasks.md`.

Date: 2026-11-26.

## Summary

N004 adds `Tokenizer.Token() (Token, error)` and the `Token`, `Number` and
`Delim` types, mirroring the shape of `encoding/json.Decoder.Token()` while
staying deliberately not `encoding/json`-compatible. Sanctioned by Article VI
("network, formatting and hooks surface", item A7); no new amendment.

## Design

- `Token()` reuses the pull primitives (`peek` for classification; typed reads /
  `Begin*` / `End*` for consumption), so the token stream and the `Peek`/`Read*`
  stream are consistent by construction; tests assert this differentially over
  the shared document tables.
- Union: `nil` (null), `bool`, `Number` (raw literal), `string` (string value or
  object key), `Delim` for `{ } [ ]`.
- End of stream: `io.EOF` once the top-level value (and, for `NewDecoder`, every
  following value) is consumed; a closing `}`/`]` is a `Delim`.
- Numbers stay `Number`, never `float64`; precision preserved.
- Strings keep escapes intact (the syntax layer does not unescape) — a documented
  deviation from `encoding/json`, whose `Token` unescapes.
- Separators `,` and the `:` after a key are consumed internally.
- Errors reuse the sticky contract and the validator's exact messages/positions.

## Ownership and aliasing

`Token` returns a `string`/`Number` value, not a `[]byte`; the conversion copies,
so a returned token is independent of the `Tokenizer` and safe to retain —
stronger than the `ReadString`/`ReadNumber` aliasing contract.

## Verification

`make verify` and `go test -race ./...`, plus a differential comparison against
the `Peek`/`Read*` stream over the shared valid/invalid tables and against the
slice-backed tokenizer at several reader chunk sizes.
