# qi-json comparative benchmark

A **separate, non-shipped Go module** that benchmarks qi-json against
`encoding/json` (the standard library) and [`mailru/easyjson`](https://github.com/mailru/easyjson)
on the same documents, so the three-way numbers are reproducible.

It is the sole carve-out to the repository's "standard library only" rule: a
comparative harness must import `encoding/json` and a third-party codec, which
the shipped `github.com/qi-lore/qi-json` module may not. Nothing here is part of
the published module — the shipped packages do not import it, and the main
module's `make verify` gate does not traverse into it (it has its own `go.mod`).

## What it measures

One representative record type, `Record` (scalars, a string slice, and a
timestamp string), bound three ways:

| codec | binding |
|---|---|
| `encoding/json` | reflection over the struct's `json` tags |
| `easyjson` | the committed `model_easyjson.go`, generated from the same tags |
| `qijson` | the hand-written `recordSchema` in `qijson_bind.go` |

Four cases, each reported for all three codecs:

- `BenchmarkMarshal1` / `BenchmarkUnmarshal1` — a single `Record`;
- `BenchmarkMarshal100` / `BenchmarkUnmarshal100` — a `Records` slice of 100.

The `qijson` decodes use `UnmarshalInto` to match the merge semantics of the
other two. The input bytes (`jsonRecord`, `jsonRecords`) are built once with
`encoding/json` outside every timed loop, so all three decoders read the
identical bytes. The `easyjson` marshallers are generated with
`-no_std_marshalers`, so `encoding/json` does **not** pick up `MarshalJSON` /
`UnmarshalJSON` from them and its benchmarks stay reflection-based.

## Running it

```
cd bench
go test -run=^$ -bench=. -benchmem
```

A quick pass with a shorter per-benchmark budget:

```
cd bench
go test -run=^$ -bench=. -benchmem -benchtime=100ms
```

The output is one line per sub-benchmark, e.g.:

```
BenchmarkMarshal1/qijson-8     ... ns/op    ... B/op    ... allocs/op
```

## Regenerating the easyjson marshaller

`model_easyjson.go` is committed so the harness builds without a codegen step.
To regenerate it after editing `model.go`:

```
cd bench
go mod download github.com/mailru/easyjson github.com/josharian/intern
go run github.com/mailru/easyjson/easyjson@v0.9.2 -no_std_marshalers model.go
```

## Regenerating the qi-json binding

`qijson_bind.go` is hand-written. The repository's `qi-json-gen` emits an
equivalent `Schema` plus `DecodeJSON`/`EncodeJSON` from the same tags, as an
alternative to the hand-written file — use one or the other, not both, because
both define those two methods:

```
cd bench
go run github.com/qi-lore/qi-json/cmd/qi-json-gen -type=Record,Records .
```
