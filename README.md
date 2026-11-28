# qi-json

[![CI](https://github.com/qi-lore/qi-json/actions/workflows/ci.yml/badge.svg)](https://github.com/qi-lore/qi-json/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/qi-lore/qi-json.svg)](https://pkg.go.dev/github.com/qi-lore/qi-json)
[![Go Version](https://img.shields.io/github/go-mod/go-version/qi-lore/qi-json)](go.mod)
[![Coverage](https://img.shields.io/badge/coverage-98.2%25-brightgreen)](https://github.com/qi-lore/qi-json/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/qi-lore/qi-json)](LICENSE)

A **JSON library for Go**, written from scratch. No `encoding/json`, no `reflection`, no `unsafe`.

```go
import "github.com/qi-lore/qi-json"
```

Requires Go 1.24+.

## Version `v0.5.5`

## Why

Most Go JSON libraries either wrap `encoding/json` or rely on **reflection**. `qi-json` does neither.

 - **No reflection**. Struct fields are bound through compile-time accessor closures. Decoding is predictable and allocation-tight.
 - **Fast syntax layer**. Valid is a single pass over the byte slice with an explicit container stack. It allocates nothing for documents nested up to 32 levels.
 - **Precise errors**. Every failure is a *qijson.Error with the JSON path, byte offset, and failure kind — not a generic syntax error.
 - **Optional code generation**. Write the field mapping by hand, or let qi-json-gen generate it from struct tags.

**The trade-off**: the API is deliberately not encoding/json-compatible. You opt into the mapping explicitly.

## Differences from `encoding/json`

qi-json speaks **standard JSON (RFC 8259)** — the same wire format — but its
**Go API is deliberately not `encoding/json`-compatible**. The compatibility
boundary is:

| | `encoding/json` | qi-json |
|---|---|---|
| Struct binding | reflection over `json` tags | compile-time accessor closures (`Field[T]`) or generated code |
| Entry points | `Marshal` / `Unmarshal` | `Marshal[T]` / `Unmarshal[T]`, non-generic `Decode` / `Encode` |
| Custom codec interfaces | `MarshalJSON` / `UnmarshalJSON` | `EncodeJSON` / `DecodeJSON` (`Encoder` / `Decoder`) |
| Reflection / `unsafe` | uses reflection | **never** |
| Invalid UTF-8 on decode | replaced with `U+FFFD` | **rejected** as a syntax error (stricter) |
| Invalid UTF-8 on encode | replaced with `U+FFFD` | replaced with `U+FFFD` (same) |
| Error type | `*json.SyntaxError`, `*json.UnmarshalTypeError` | `*qijson.Error` with JSON path, byte offset and `Kind` |
| Decoding into an existing value | merges into structs | merges (`UnmarshalInto`, `Decode`) |
| Nesting depth | capped at 10000 (`maxNestingDepth`) | decoder capped at 10000 (`KindDepth`); `Valid` has no limit |

The trade-off is intentional: dropping reflection buys predictability and an
allocation-tight hot path, at the cost of opting into the mapping explicitly —
by hand or via [`qi-json-gen`](#code-generation-optional). There is no
compatibility shim; if you need `encoding/json`'s exact API, keep using it.

### Nesting depth limit

Decoding caps container nesting at **10000 levels**. Input that nests deeper
fails with a `*qijson.Error` whose kind is `KindDepth`:

```go
var e *qijson.Error
if errors.As(err, &e) && e.Kind == qijson.KindDepth {
    // qijson: $.a.b.c: nesting depth exceeds 10000 (at offset 30004)
}
```

**The 10000 is not arbitrary — it is the number the Go standard library uses.**
`encoding/json` defines the same cap in `encoding/json/scanner.go`:

```go
// This limits the max nesting depth to prevent stack overflow.
// This is permitted by https://tools.ietf.org/html/rfc7159#section-9
const maxNestingDepth = 10000
```

and rejects deeper input with the message `exceeded max depth`. RFC 7159 §9
(carried into RFC 8259 §9) states that "an implementation may set limits on the
maximum depth of nesting", so neither library is guessing — they agree on the
same value.

**One deliberate difference from `encoding/json`.** In the standard library the
10000 cap applies to `Valid` as well as to `Unmarshal`, because both run through
the same scanner. qi-json applies the cap to the **decoder** and leaves
`Valid` **unlimited**: the syntax validator is a single allocation-free pass over
an explicit stack, so a deeply nested document cannot overflow the Go stack and
needs no ceiling. If you rely on `encoding/json.Valid` rejecting pathological
nesting, use the decoder (or the `Tokenizer`) for that check instead.

## Install

```bash
go get github.com/qi-lore/qi-json
```

## Usage

**Decode into a fresh value**:

```go
cfg, err := qijson.Unmarshal[Config](data)
```

**Decode into an existing value (merge)**:

```go
var cfg Config
err := qijson.UnmarshalInto(data, &cfg)
```

To decode, T must implement DecodeJSON — either by hand or via generated code.

**Or use the non-generic form when the type is already known**:

```go
var cfg Config
err := qijson.Decode(data, &cfg)
```

**Encoding mirrors decoding**:

```go
b, err := qijson.Marshal(cfg)
```

## Example

A runnable end-to-end example lives in `example/`. It validates the input with
`Valid`, decodes it into a struct with `Unmarshal`, and re-encodes the result
with `Marshal`. Run it with:

```bash
go run ./example
```

## Syntax-only checks

```go
ok := qijson.Valid(data)          // yes/no, no allocation
tok := qijson.NewTokenizer(data)  // pull-based tokenizer
```

Most callers should use a Schema, not the tokenizer directly.

## Exposed API

Everything is reached through the single import path
`github.com/qi-lore/qi-json` (package `qijson`). The module root is a thin facade
over two internal engines; the public surface is:

**Decode / encode entry points**

- `Unmarshal[T]`, `UnmarshalInto[T]`, `Decode` — decode into a fresh or existing value
- `Marshal[T]`, `Encode` — encode a value

**Struct binding**

- `Schema[T]`, `Struct`, `Field[T]`, `Codec[E]`
- Field constructors: `String`, `Int`, `Float`, `Bool`, `Object`, `Slice`, `Map`,
  `Array`, `Any`, `Raw` and their `…Ptr` variants
- Foreign-type delegation: `ObjectDecoder`, `ObjectDecoderPtr`, `DecoderCodec`
- Hooks: `Optional`, `IsZero`, `UnmarshalUnknown`, `MarshalUnknowns`

**Custom codec interfaces**

- `Decoder` (`DecodeJSON(*Reader)`) with `Reader`
- `Encoder` (`EncodeJSON(*Writer)`) with `Writer`

**Syntax layer**

- `Valid` — allocation-free syntax check
- `Tokenizer` via `NewTokenizer(data)` / `NewDecoder(io.Reader)`, with `Kind`,
  `Token`, `Number`, `Delim`

**Formatting & streaming**

- `MarshalIndent`, `Indent`, `Compact`, `HTMLEscape`, `SetIndent`, `SetEscapeHTML`
- `Writer.DumpTo`, `Writer.ReadCloser`

**Errors**

- `Error` (JSON path + byte offset + `Kind`), `TypeError`, `ErrorKind`
  (`KindSyntax`, `KindType`, `KindUnknownField`, `KindDuplicateField`, `KindDepth`)

Run `go doc github.com/qi-lore/qi-json` for the full reference. The public
surface is fixed by the project constitution and only grows by amendment.

## Code generation (optional)

The library works without the generator. If you'd rather not write field mappings, mark a struct and run qi-json-gen:

```go
//go:generate go run github.com/qi-lore/qi-json/cmd/qi-json-gen -type=Config
type Config struct {
    Name    string `json:"name"`
    Timeout int    `json:"timeout"`
}
```

Or pin it as a tool (Go 1.24+):

```bash
go get -tool github.com/qi-lore/qi-json/cmd/qi-json-gen
go tool qi-json-gen -type=Config
```

Generated code is deterministic and checked in. go generate ./... keeps it in sync.

## Performance

Ballpark numbers on an **Apple M1 8 cores 16GB RAM** (see `make bench`):

| Benchmark | Result |
|-----------|-----------|
| Valid (object) | 829 MB/s, 0 allocs |
| Valid (numbers) | 1044 MB/s, 0 allocs |
| Valid (string) | 2578 MB/s, 0 allocs |
| Struct decode | ~447 ns/op, 4 allocs |
| Struct encode | ~224 ns/op, 7 allocs |

Numbers are not guarantees. Run the benchmarks on your own hardware.

## Comparison with `encoding/json`

qi-json is benchmarked against `encoding/json` (the standard library) and
[`mailru/easyjson`](https://github.com/mailru/easyjson) on the **same documents
and the same record type**, in the separate, non-shipped `bench/` module. All
three codecs read identical input bytes: `encoding/json` binds by reflection,
`easyjson` via its generated marshaller, and qi-json via a hand-written schema.
The easyjson marshallers are generated with `-no_std_marshalers`, so
`encoding/json` stays reflection-based and does not delegate to them.

### How to run them

```bash
cd bench
go test -run=^$ -bench=. -benchmem                        # full run
go test -run=^$ -bench=. -benchmem -benchtime=100ms       # quick pass
```

The output is one line per case and codec, e.g.:

```
BenchmarkUnmarshal100/qijson-8    2637    91379 ns/op    67672 B/op    212 allocs/op
```

See [`bench/README.md`](bench/README.md) for the harness details and how to
regenerate the easyjson and qi-json bindings.

### Indicative numbers

Apple M1 (8 cores), Go 1.27.3, short benchmark budget — **single run, not a
guarantee**. Reproduce on your own hardware before drawing conclusions.

| Case | `encoding/json` | `easyjson` | qi-json |
|---|---|---|---|
| `Marshal` ×1      | 511 ns/op, 3 allocs | 365 ns/op, 6 allocs | 431 ns/op, 6 allocs |
| `Unmarshal` ×1    | 946 ns/op, 6 allocs | 443 ns/op, 8 allocs | 1042 ns/op, 5 allocs |
| `Marshal` ×100    | 37.0 µs/op, 3 allocs | 18.1 µs/op, 12 allocs | 30.4 µs/op, 7 allocs |
| `Unmarshal` ×100  | 86.8 µs/op, 509 allocs | 49.7 µs/op, 808 allocs | 91.4 µs/op, 212 allocs |

### What the numbers say

- **Throughput is in the same class as `encoding/json`** — faster on encode,
  broadly comparable on decode. qi-json is **not** marketed as the fastest codec;
  `easyjson` is typically the fastest here.
- **qi-json's differentiators are structural, not raw speed**: no reflection, no
  `unsafe`, and a flatter allocation profile on large documents — 212 allocations
  to unmarshal 100 records versus 509 (`encoding/json`) and 808 (`easyjson`).
- **The syntax layer is where the speed story is clearest**: `Valid` runs a single
  allocation-free pass (see [Performance](#performance)), so validation and
  streaming tokenization avoid the decode machinery entirely.

Numbers are machine-specific. Re-run `make bench` (root module) and
`cd bench && go test -run=^$ -bench=. -benchmem` on your own hardware.

## Development

```bash
make verify   # test, gofmt, vet, lint
make race     # race detector
make bench    # benchmarks
```

## Status
Current release: `v0.5.5`. The decoder and encoder are complete and benchmarked, but the API may still change before `1.0`.

## License

See `LICENSE`.


