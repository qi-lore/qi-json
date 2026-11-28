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

Five shapes, each bound three ways and reported for both directions, Marshal and
Unmarshal:

| Benchmarks | Shape | What it exercises |
|---|---|---|
| `BenchmarkMarshal1` / `BenchmarkUnmarshal1` | flat `Record` | scalars, a `[]string`, a timestamp string |
| `BenchmarkMarshal100` / `BenchmarkUnmarshal100` | `Records` (100 × `Record`) | a top-level array of objects; slice growth |
| `BenchmarkMarshalNested` / `BenchmarkUnmarshalNested` | `Nested` | three levels of nested objects (`Nested` → `Company` → `Address` → `Geo`) |
| `BenchmarkMarshalMap` / `BenchmarkUnmarshalMap` | `MapDoc` | `map[string]string` and `map[string]int64` fields |
| `BenchmarkMarshalUnicode` / `BenchmarkUnmarshalUnicode` | `UnicodeDoc` | non-ASCII runes and `\n` / `\t` / `\u` escape sequences |

The types live in `model.go`; the shapes are deliberately small and deterministic
so a single number is stable across runs.

The three codecs and their bindings:

| codec | binding |
|---|---|
| `encoding/json` | reflection over the struct's `json` tags |
| `easyjson` | the committed `model_easyjson.go`, generated from the same tags |
| `qijson` | the hand-written schemas in `qijson_bind.go` |

Every shape is measured with sub-benchmarks named `stdlib`, `easyjson` and
`qijson`, so one `go test -bench` run reports all three side by side.

### Fairness invariants

- The JSON input for every decoder is built **once**, outside every timed loop,
  with `encoding/json` in `init()` (`jsonRecord`, `jsonRecords`, `jsonNested`,
  `jsonMapDoc`, `jsonUnicode`). All three decoders therefore read the identical
  bytes; no codec pays for constructing its input inside the loop.
- The marshal benchmarks encode one value reused across iterations, so all three
  encoders write the same data.
- `TestCodecsAgree` (in `agree_test.go`) decodes the shared bytes with all three
  codecs and asserts they produce the same value, so a codec that silently
  diverges fails the harness instead of skewing the comparison.
- The `qijson` decodes use `UnmarshalInto` / `Decode` to match the merge
  semantics of the other two.
- The `easyjson` marshallers are generated with `-no_std_marshalers`, so
  `encoding/json` does **not** pick up `MarshalJSON` / `UnmarshalJSON` from them
  and its benchmarks stay reflection-based.

## Running it

A quick pass (short per-benchmark budget):

```
cd bench
go test -run=^$ -bench=. -benchmem
```

The output is one line per sub-benchmark, e.g.:

```
BenchmarkMarshal1/qijson-8     ... ns/op    ... B/op    ... allocs/op
```

### Reproducible procedure

Micro-benchmark timing is sensitive to how many OS threads the Go scheduler uses
and to anything else running on the machine, so the published numbers come from a
fixed procedure:

1. Run on an otherwise **idle** machine — no build, no test job, no editor
   indexing. An earlier run on a busy host was dominated by `runtime.kevent`
   (~71% of samples) in the CPU profile, which made the CPU attribution useless;
   the allocation profile stayed deterministic, but timing did not. Keep the host
   quiet so the wall-clock numbers mean something.
2. Pin the scheduler: set `GOMAXPROCS` explicitly rather than letting it track
   the host, so a run on a machine with a different core count is comparable.
3. Take several samples and aggregate with `benchstat` rather than trusting a
   single run:

```
cd bench
GOMAXPROCS=8 go test -run=^$ -bench=. -benchmem -count=5 | tee bench-count5.txt
go run golang.org/x/perf/cmd/benchstat@latest bench-count5.txt
```

`benchstat` prints the mean of the samples per benchmark and flags statistically
significant differences when comparing two files (before/after). With `-count=5`
its confidence-interval column reads `± ∞`; it needs **at least 6 samples** for a
95% interval, so use `-count=6` or more when comparing two revisions and a real
interval matters.

To compare a change against a baseline, capture the baseline with the same
command and run `benchstat baseline.txt candidate.txt`.

## Current numbers

Measured 2026-11-28 on an Apple M1, Go 1.27.3, `GOMAXPROCS=8`, `-count=5`,
machine otherwise idle; means as reported by `benchstat` (lower is better):

| Benchmark | `encoding/json` | `easyjson` | qi-json |
|---|---|---|---|
| Marshal1 | 516.7 ns / 384 B / 3 | 365.8 ns / 976 B / 6 | 434.3 ns / 472 B / 6 |
| Unmarshal1 | 967.4 ns / 236 B / 6 | 453.2 ns / 168 B / 8 | 1060 ns / 1.5 KiB / 5 |
| Marshal100 | 38.44 µs / 18.05 KiB / 3 | 18.41 µs / 18.98 KiB / 12 | 30.83 µs / 18.28 KiB / 7 |
| Unmarshal100 | 87.17 µs / 44.99 KiB / 509 | 50.25 µs / 47.69 KiB / 808 | 92.80 µs / 66.09 KiB / 212 |
| MarshalNested | 551.2 ns / 304 B / 3 | 364.7 ns / 912 B / 6 | 440.8 ns / 472 B / 7 |
| UnmarshalNested | 932.0 ns / 80 B / 1 | 402.7 ns / 48 B / 3 | 1003 ns / 1.375 KiB / 4 |
| MarshalMap | 913.7 ns / 304 B / 13 | 406.6 ns / 840 B / 6 | 672.7 ns / 736 B / 20 |
| UnmarshalMap | 1156 ns / 672 B / 9 | 585.9 ns / 656 B / 13 | 1143 ns / 1.883 KiB / 8 |
| MarshalUnicode | 421.1 ns / 304 B / 3 | 401.5 ns / 928 B / 6 | 324.7 ns / 376 B / 5 |
| UnmarshalUnicode | 996.6 ns / 176 B / 3 | 363.4 ns / 160 B / 3 | 890.0 ns / 1.570 KiB / 8 |

Each cell is `ns/op` (or `µs/op`) / `B/op` / `allocs/op`.

Reading it: qi-json encode is already competitive with the standard library and
beats it on the unicode and 100-record shapes; qi-json decode trails both
`encoding/json` and `easyjson`, and `easyjson` leads decode by a wide margin.
The decode gap is the target of the decode-performance work; see the roadmap
record referenced below.

## Known caveat: the qi-json top-level array decoder does not pre-size

A `pprof` allocation profile of `BenchmarkUnmarshal100/qijson` attributes the
bulk of the allocated bytes to two things the harness cannot fix:

- ~61.5% — `bench.(*Records).DecodeJSON`: the hand-written top-level array
  decoder in `qijson_bind.go` grows its `[]Record` with `append` and never
  pre-sizes it.
- ~27.4% — `codec.Decode`: `string(data)` copies the whole document on every
  call, for the input slice the decoder is handed.
- ~9.0% — the per-record `tags []string` growth.

The library's own `Slice` field codec already pre-sizes from an internal array
length hint, but a **hand-written top-level `DecodeJSON` has no public way to
pre-size** — the hint is unexported and there is no exported top-level container
codec. This is simultaneously an artifact of the harness and a genuine ergonomic
gap.

It is deliberately **not** "fixed" here: the harness must measure the library as
it stands, and inventing a private fast path in `bench/` would hide the gap. The
caveat is recorded so the decode-performance phase can decide whether to close it
with internal pre-sizing alone or to add a top-level container codec (which would
need a constitution Article VI amendment). The `DecodeJSON` comment in
`qijson_bind.go` repeats this warning at the point of the code.

## Regenerating the easyjson marshaller

`model_easyjson.go` is committed so the harness builds without a codegen step.
After editing `model.go`, regenerate it (every type carries `//easyjson:json`):

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
go run github.com/qi-lore/qi-json/cmd/qi-json-gen -type=Record,Records,Nested,Company,Address,Geo,MapDoc,UnicodeDoc .
```
