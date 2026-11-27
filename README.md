# qi-json

A **JSON library for Go**, written from scratch. No `encoding/json`, no `reflection`, no `unsafe`.

```go
import "github.com/qi-lore/qi-json"
```

Requires Go 1.24+.

## Version `v0.5.3`

## Why

Most Go JSON libraries either wrap `encoding/json` or rely on **reflection**. `qi-json` does neither.

 - **No reflection**. Struct fields are bound through compile-time accessor closures. Decoding is predictable and allocation-tight.
 - **Fast syntax layer**. Valid is a single pass over the byte slice with an explicit container stack. It allocates nothing for documents nested up to 32 levels.
 - **Precise errors**. Every failure is a *qijson.Error with the JSON path, byte offset, and failure kind — not a generic syntax error.
 - **Optional code generation**. Write the field mapping by hand, or let qi-json-gen generate it from struct tags.

**The trade-off**: the API is deliberately not encoding/json-compatible. You opt into the mapping explicitly.

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

## Development

```bash
make verify   # test, gofmt, vet, lint
make race     # race detector
make bench    # benchmarks
```

## Status
Current release: `v0.5.3`. The decoder and encoder are complete and benchmarked, but the API may still change before `1.0`.

## License

See `LICENSE`.


