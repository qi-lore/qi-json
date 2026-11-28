[![CI](https://github.com/qi-lore/qi-json/actions/workflows/ci.yml/badge.svg)](https://github.com/qi-lore/qi-json/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/qi-lore/qi-json.svg)](https://pkg.go.dev/github.com/qi-lore/qi-json)
[![Go Version](https://img.shields.io/github/go-mod/go-version/qi-lore/qi-json)](go.mod)
[![Coverage](https://img.shields.io/badge/coverage-98.2%25-brightgreen)](https://github.com/qi-lore/qi-json/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/qi-lore/qi-json)](LICENSE)

# qi-json

A **JSON library for Go**, written from scratch. No `encoding/json`, no `reflection`, no `unsafe`.

```go
import "github.com/qi-lore/qi-json"
```

Requires Go 1.24+.

## Version `v0.5.5`

## Why

Most Go JSON libraries either wrap `encoding/json` or rely on **reflection**. `qi-json` does neither.

- **No reflection**. Struct fields are bound through compile-time accessor closures. Decoding is predictable and allocation-tight.
- **Fast syntax layer**. `Valid` is a single pass over the byte slice with an explicit container stack. It allocates nothing for documents nested up to 32 levels.
- **Precise errors**. Every failure is a `*qijson.Error` with the JSON path, byte offset, and failure kind — not a generic syntax error.
- **Optional code generation**. Write the field mapping by hand, or let `qi-json-gen` generate it from struct tags.

The trade-off is intentional: the API is deliberately not `encoding/json`-compatible, so you opt into the mapping explicitly. See [Differences from `encoding/json`](#differences-from-encodingjson).

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

To decode, `T` must implement `DecodeJSON` — either by hand or via generated code.

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

## Code generation (optional)

The library works without the generator. If you'd rather not write field mappings, mark a struct and run `qi-json-gen`:

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

Generated code is deterministic and checked in. `go generate ./...` keeps it in sync.

## Differences from `encoding/json`

qi-json speaks **standard JSON (RFC 8259)** — the same wire format — but the Go API differs deliberately (see [Why](#why)). The compatibility boundary:

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

There is no compatibility shim; if you need `encoding/json`'s exact API, keep using it.

### Nesting depth limit

Decoding caps container nesting at **10000 levels**. Input that nests deeper fails with a `*qijson.Error` whose kind is `KindDepth`:

```go
var e *qijson.Error
if errors.As(err, &e) && e.Kind == qijson.KindDepth {
    // qijson: $.a.b.c: nesting depth exceeds 10000 (at offset 30004)
}
```

The 10000 is not arbitrary — it is the same number the Go standard library uses. `encoding/json` defines `maxNestingDepth = 10000` in `encoding/json/scanner.go` and rejects deeper input with `exceeded max depth`. RFC 7159 §9 (carried into RFC 8259 §9) permits an implementation to set such a limit, so neither library is guessing.

One deliberate difference: `encoding/json` applies the cap to `Valid` as well as `Unmarshal`, because both run through the same scanner. qi-json caps the **decoder** and leaves `Valid` **unlimited** — the validator is a single allocation-free pass over an explicit stack, so deep nesting cannot overflow the Go stack. To reject pathological nesting, use the decoder (or the `Tokenizer`).

## Exposed API

Everything is reached through the single import path `github.com/qi-lore/qi-json` (package `qijson`), a thin facade over two internal engines:

- **Entry points** — `Unmarshal[T]`, `UnmarshalInto[T]`, `Decode`, `Marshal[T]`, `Encode`
- **Struct binding** — `Schema[T]`, `Struct`, `Field[T]`, `Codec[E]`; field constructors `String`, `Int`, `Float`, `Bool`, `Object`, `Slice`, `Map`, `Array`, `Any`, `Raw` and their `…Ptr` variants; `ObjectDecoder`, `ObjectDecoderPtr`, `DecoderCodec`; hooks `Optional`, `IsZero`, `UnmarshalUnknown`, `MarshalUnknowns`
- **Codec interfaces** — `Decoder` (`DecodeJSON(*Reader)`) with `Reader`; `Encoder` (`EncodeJSON(*Writer)`) with `Writer`
- **Syntax layer** — `Valid`; `Tokenizer` via `NewTokenizer` / `NewDecoder`, with `Kind`, `Token`, `Number`, `Delim`
- **Formatting & streaming** — `MarshalIndent`, `Indent`, `Compact`, `HTMLEscape`, `SetIndent`, `SetEscapeHTML`; `Writer.DumpTo`, `Writer.ReadCloser`
- **Errors** — `Error` (JSON path + byte offset + `Kind`), `TypeError`, `ErrorKind`

Run `go doc github.com/qi-lore/qi-json` for the full reference. The public surface is fixed by the project constitution and only grows by amendment.

## Performance

Ballpark numbers on an **Apple M1, Go 1.27.3** (see `make bench`). Numbers are machine-specific — reproduce on your own hardware.

| Benchmark | Result |
|-----------|-----------|
| Valid (object) | 829 MB/s, 0 allocs |
| Valid (numbers) | 1044 MB/s, 0 allocs |
| Valid (string) | 2578 MB/s, 0 allocs |
| Struct decode | ~447 ns/op, 4 allocs |
| Struct encode | ~224 ns/op, 7 allocs |

### Against `encoding/json` and `easyjson`

The separate, non-shipped [`bench/`](bench/README.md) module compares qi-json with `encoding/json` (reflection binding) and [`mailru/easyjson`](https://github.com/mailru/easyjson) (generated marshaller) on the same documents and record type:

```bash
cd bench
go test -run=^$ -bench=. -benchmem
```

| Case | `encoding/json` | `easyjson` | qi-json |
|---|---|---|---|
| `Marshal` ×1      | 511 ns/op, 3 allocs | 365 ns/op, 6 allocs | 431 ns/op, 6 allocs |
| `Unmarshal` ×1    | 946 ns/op, 6 allocs | 443 ns/op, 8 allocs | 1042 ns/op, 5 allocs |
| `Marshal` ×100    | 37.0 µs/op, 3 allocs | 18.1 µs/op, 12 allocs | 30.4 µs/op, 7 allocs |
| `Unmarshal` ×100  | 86.8 µs/op, 509 allocs | 49.7 µs/op, 808 allocs | 91.4 µs/op, 212 allocs |

Throughput is in the same class as `encoding/json` — faster on encode, broadly comparable on decode; `easyjson` is the fastest here. qi-json's edge is structural, not raw speed: no reflection, no `unsafe`, and a flatter allocation profile on large documents (212 allocations to unmarshal 100 records versus 509 and 808).

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
