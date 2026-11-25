# qi-json

A from-scratch JSON library for Go. No third-party code, no `encoding/json`, and
no `reflect` or `unsafe`: the decoder binds struct fields with compile-time
accessor closures, optionally generated from struct tags.

| Package | Import path | Role |
|---|---|---|
| `qijson` | `github.com/qi-lore/qi-json` | The one public package: the semantic layer plus the re-exported syntax surface (`Valid`, `Kind`, `Tokenizer`) |
| `internal/codec` | `github.com/qi-lore/qi-json/internal/codec` | Implementation of the semantic layer behind the facade (internal) |
| `internal/scanner` | `github.com/qi-lore/qi-json/internal/scanner` | Single-pass syntax engine behind the facade (internal) |
| `cmd/qi-json-gen` | `github.com/qi-lore/qi-json/cmd/qi-json-gen` | Optional code generator for struct tags |

Released as **v0.4.2** (2026-11-23). The decoder and encoder are complete and
benchmarked; as a pre-1.0 release the API may still change.

## Version

Current release: **v0.4.2** (2026-11-23).

v0.4.2 is a robustness release: four defects that turned a programming error into
a panic or a silently malformed result are now inspectable sentinels or precise
errors. `Schema.UnmarshalInto` and `Schema.Decode` return `errNilTarget` for a nil
target instead of panicking; `Object` returns `errNilSchema` for a nil nested
schema instead of dereferencing it; `Writer.WriteRaw` rejects an empty slice and
`Marshal`'s completeness guard now requires the root to emit a value; and
`Struct` enforces its documented unique-field-name requirement. The public API is
unchanged. Two code-level assessment records were added
(`docs/fn-comparison.md`, `docs/panics-issues.md`), and `main` and `dev` are now
protected by branch rulesets.

The previous release, v0.4.1, was a documentation release: the README gains an
**Installing the generator** section covering `go run`, the Go 1.24 `tool`
directive and `go install`, and the stale `qi-json/cmd/qi-json-gen` module path
in the codegen examples is corrected. It also folds in an internal Uber Go Style
Guide review pass; no public API, error message or runtime behavior changed, and
`go.mod` gains no dependency.

The previous release, v0.4.0, consolidated the syntax layer behind the facade:
the `scanner` subpackage is gone, the syntax engine lives in `internal/scanner`,
and `qijson` re-exports its whole surface — `Valid`, the `Kind` type with its
seven constants, and the tokenizing `Tokenizer` with `NewTokenizer`. There is now
a single public import path, `github.com/qi-lore/qi-json`. Names, signatures and
behavior are unchanged (`qijson.Kind` *is* the type `Reader.Peek` returns), so
the change is an import-path and qualifier rewrite; it is nonetheless breaking
for anyone who imported `github.com/qi-lore/qi-json/scanner`. See **Migrating
from 0.3.x** below.

The release before it, v0.3.1, was a documentation release (`docs/tasks.md` became
the single source of truth for task state; error messages and doc comments no
longer cite project documents). The release before that, v0.3.0, moved the
semantic engine behind the `qijson` facade into `internal/codec`, adopted the
canonical module path `github.com/qi-lore/qi-json`, and raised the Go floor to
**1.24** for the facade's generic type aliases — both breaking for importers; see
**Migrating from 0.2.x**. See [CHANGELOG.md](CHANGELOG.md) for the change history;
the git tags record the release series.

## Why another JSON library

- **Independent implementation.** The scanner and decoder are written from
  scratch against the contract the tests pin. `encoding/json` is not imported
  anywhere in the module, including tests and benchmarks.
- **No reflection.** `qijson` decodes into concrete Go types through accessor
  closures. That keeps struct decoding predictable and allocation-tight — and it
  is why the API is deliberately **not** `encoding/json`-compatible.
- **Fast syntax layer.** `Valid` is a single pass over the byte slice with an
  explicit container stack; it does no per-byte handler dispatch and allocates
  nothing for documents nesting up to 32 levels.
- **Precise errors.** Every decode failure is a `*qijson.Error` carrying the
  JSON path (`$.items[2].name`), the byte offset, and a failure kind, wrapping a
  `*qijson.TypeError` for type mismatches.
- **Boilerplate is optional.** Hand-write the field mapping, or let
  `qi-json-gen` emit it from `json` struct tags.

## Using it

The canonical module path is `github.com/qi-lore/qi-json`:

```
go get github.com/qi-lore/qi-json
```

While working from a local checkout, point a `replace` directive at it instead:

```
require github.com/qi-lore/qi-json v0.0.0
replace github.com/qi-lore/qi-json => ../qi-json
```

The module requires **Go 1.24 or newer**. Its `go.mod` sets `go 1.24.0` (the
language floor, because the `qijson` facade re-exports generic types through
generic type aliases) and adds a `toolchain go1.27.3` line, which merely names
the toolchain used to develop the project — a dependency's `toolchain` line is
ignored, so it does not raise the floor for a consumer.

### Migrating from 0.3.x

The syntax layer moved behind the facade. The `scanner` subpackage is no longer
importable; its whole surface is re-exported from `qijson`. One import path and a
qualifier change:

```diff
-import "github.com/qi-lore/qi-json/scanner"
+import "github.com/qi-lore/qi-json"

-ok := scanner.Valid(data)
-tok := scanner.NewTokenizer(data)
-if k == scanner.KindNull { ... }
+ok := qijson.Valid(data)
+tok := qijson.NewTokenizer(data)
+if k == qijson.KindNull { ... }
```

The names, signatures and behavior are identical — `qijson.Kind` *is* the token
kind `Reader.Peek` returns — so only the qualifier and the import line change.
Regenerate any `qi-json-gen` output: it no longer imports `scanner` and compares
against `qijson.KindNull`.

### Migrating from 0.2.x

Three edits are required:

1. **Rewrite the import path.** `qi-json` → `github.com/qi-lore/qi-json`, and
   `qi-json/scanner` → `github.com/qi-lore/qi-json/scanner`. The package name is
   still `qijson`, so no identifier changes.
2. **Raise the `go` directive in your `go.mod` to 1.24.** The facade re-exports
   generic types through generic type aliases.
3. **Regenerate any `qi-json-gen` output.** The emitted import path changed with
   the module path, so checked-in generated files still name the old path.

Nothing else changes: `qijson.Reader` and `qijson.Writer` are the same types as
before, so `errors.As`, generated `Field` literals and existing call sites
compile unchanged once the imports are updated. (v0.4.0 then folds the
`scanner` subpackage into the facade — see **Migrating from 0.3.x**.)

## Quick start

Map JSON members to struct fields with a `Schema`, then implement `DecodeJSON`:

```go
package example

import "github.com/qi-lore/qi-json"

type Config struct {
	Name string
	Port int64
}

var configSchema = qijson.Struct(
	qijson.String[Config]("name", func(c *Config) *string { return &c.Name }),
	qijson.Int[Config]("port", func(c *Config) *int64 { return &c.Port }),
)

func (c *Config) DecodeJSON(r *qijson.Reader) error {
	return configSchema.Decode(r, c)
}

func (c *Config) EncodeJSON(w *qijson.Writer) error {
	return configSchema.Encode(w, c)
}

func Load(data []byte) (Config, error) {
	return qijson.Unmarshal[Config](data)   // fresh value
}

func Merge(data []byte, cfg *Config) error {
	return qijson.Decode(data, cfg)         // no type argument; in place
}

func Save(cfg Config) ([]byte, error) {
	return qijson.Marshal[Config](cfg)      // encode to JSON
}
```

### Generic and non-generic entry points

There are three package-level entry points for decoding and two for encoding. All
of them handle exactly one JSON value, and a decoder rejects trailing input with
the same errors.

| Function | Generic | Starts from | Result |
|---|---|---|---|
| `Unmarshal[T](data)` | yes | a zero `T` | a fresh `T`, returned |
| `UnmarshalInto[T](data, v)` | yes | the existing `v` | fills `v` in place (merge) |
| `Decode(data, v)` | no | the existing `v` | fills `v` in place (merge) |
| `Marshal[T](v)` | yes | a value `v` | a fresh `[]byte`, returned |
| `Encode(w, v)` | no | a `*Writer` and an `Encoder` | writes into `w` |

Their signatures:

```go
func Unmarshal[T any, PT interface{ *T; Decoder }](data []byte) (T, error)
func UnmarshalInto[T any, PT interface{ *T; Decoder }](data []byte, v *T) error
func Decode(data []byte, v Decoder) error
func Marshal[T any, PT interface{ *T; Encoder }](v T) ([]byte, error)
func Encode(w *Writer, v Encoder) error
```

The generic pair takes a type argument and the pointer-receiver constraint
`PT interface{ *T; Decoder }`, so naming `T` alone selects the decoder: `*T`
must have a `DecodeJSON` method (from a hand-written `Schema`, or generated by
`qi-json-gen`). Use `Unmarshal` for a new value and `UnmarshalInto` to merge
into one that already exists.

The encoder mirrors them: `Marshal[T]` requires `*T` to have an `EncodeJSON`
method, and `Encode(w, &cfg)` is the non-generic form writing into a reused
`*Writer`. See [Encoding](#encoding).

`Decode` is the non-generic counterpart to `UnmarshalInto` for the common case
where the concrete type is already known — pass `&cfg` and skip the type
argument:

```go
var cfg Config
err := qijson.Decode(data, &cfg)
```

`UnmarshalInto[Config](data, &cfg)` is the same operation spelled generically;
`Decode` is a thin wrapper over it. All three report the same failures: a syntax
error, a JSON type that does not match the target, or trailing input. `Decode`
with a nil interface returns `qijson: nil target`, a programming error rather
than a `*qijson.Error`; a typed-nil target is not detected because `qijson`
may not use reflection.

### Field constructors

| Go field | Constructor |
|---|---|
| `string`, `bool`, `int64`, `float64` | `String`, `Bool`, `Int`, `Float` |
| `*string`, `*bool`, `*int64`, `*float64` | `StringPtr`, `BoolPtr`, `IntPtr`, `FloatPtr` |
| nested struct bound by another `*Schema[S]` | `Object[T, S]` |
| nested struct whose type implements `DecodeJSON`/`EncodeJSON` | `ObjectDecoder[T, S]`, `ObjectDecoderPtr[T, S]`; element `DecoderCodec[S]` |
| `[]E`, `map[string]V`, `[N]E` | `Slice`, `Map`, `Array` (with an element `Codec[E]`) |
| `any` | `Any` |
| `[]byte` | `Raw` (the value's exact literal bytes, quotes included for a string) |

Constructors are generic in the target type and take an accessor, so the field
set is fixed at compile time. `Array` takes a slice view of the array
(`func(c *C) []E { return c.V[:] }`) and requires the exact element count. A
container constructor takes a `Codec[E]` — a pair of the element's decode and
encode functions, built with `qijson.NewCodec(decode, encode)`.

### Decoding rules

- The document must hold exactly one JSON value; anything after it is a syntax
  error.
- A JSON `null` leaves a value field unchanged and sets a pointer field to `nil`.
- An object member that no field matches is ignored unless `RejectUnknown` is
  set. A repeated member is overwritten (last one wins) unless
  `RejectDuplicate` is set. Both are methods on `*Schema[T]`.
- `ReadInt`/`Int` accept integer literals only: `1.0` and `1e3` are type errors,
  as is a value outside `int64`. `ReadFloat`/`Float` reject overflow and turn
  underflow into `0`.
- Strings are unescaped (`\" \\ \/ \b \f \n \r \t \uXXXX`, including surrogate
  pairs); a lone surrogate becomes `U+FFFD`. A decoded string value or member
  name must be valid UTF-8 — invalid UTF-8 is a syntax error — so the decoder is
  stricter here than `qijson.Valid`, which checks syntax only.
- `Slice`, `Map`, and `Array` fields build into a temporary and assign only on
  success, so a failed decode leaves that field unchanged. An `Object` field
  decodes into the nested struct directly, so a failure partway through can
  leave its earlier members set.

### Encoding

`EncodeJSON` is the mirror of `DecodeJSON`: a type writes itself to a `*Writer`,
usually by delegating to its `Schema`. `Marshal[T](v)` returns the bytes;
`Encode(w, &v)` writes into a reused `Writer`:

```go
data, err := qijson.Marshal[Config](cfg)   // generic, value in
var w qijson.Writer
err = qijson.Encode(&w, &cfg)              // non-generic, into a Writer
```

Encoding rules:

- `Schema.Encode` writes object members in **declaration order** — the order the
  fields were passed to `Struct`.
- A field tagged `json:",omitempty"` is skipped when it is empty: a zero-value
  scalar, a nil pointer, a nil or empty slice or map, a nil `any`, or empty
  `Raw` bytes. A nested struct and a fixed-size array are never empty, matching
  `encoding/json`.
- A nil slice or map is written as `null`; an empty (non-nil) one as `[]`/`{}`.
  Map keys are written in sorted order for a stable result.
- Strings escape `"`, `\` and the control bytes; unlike `encoding/json` the
  encoder does not escape `<`, `>`, or `&`. An invalid UTF-8 byte in a Go string
  becomes `U+FFFD` (the decoder, in contrast, rejects invalid UTF-8 on input).
- A `float64` is written in the shortest form that round-trips; `NaN` and `±Inf`
  are an error. An `any` value is written from its dynamic type — nil, `bool`,
  `string`, the integer and float kinds, `[]any`, `map[string]any`, or a nested
  `Encoder` — and any other dynamic type is an error, since `qijson` may not use
  reflection.
- Encode failures are plain `error` values, not `*qijson.Error` (which is a
  decode diagnostic carrying a path and offset).

### Inspecting errors

```go
var e *qijson.Error
if errors.As(err, &e) {
	// e.Path   "$.port"
	// e.Offset 8
	// e.Kind   qijson.KindType
	// e.Err    *qijson.TypeError{Want: "integer", Got: "number 1.5 is not an integer"}
}
```

`Error` wraps its cause, so `errors.As`/`errors.Unwrap` also reach the
`*TypeError` or the scanner's error. `ErrorKind` is one of `KindSyntax`,
`KindType`, `KindUnknownField`, `KindDuplicateField` (and the reserved
`KindDepth`; the decoder applies no nesting limit today).

## Code generation

Instead of writing the closures by hand, tag the struct and run the generator:

```go
//go:generate go run github.com/qi-lore/qi-json/cmd/qi-json-gen -type=Config

type Config struct {
	Name string   `json:"name"`
	Port int64    `json:"port"`
	Tags []string `json:"tags"`
	Blob []byte   `json:"blob"`
}
```

`go generate ./...` emits `zz_qi_json_gen.go` in the same package with a
`configSchema`, a `DecodeJSON` method, and an `EncodeJSON` method, after which
`qijson.Unmarshal[Config](data)` and `qijson.Marshal[Config](cfg)` both work
unchanged.

The generator supports `string`, `bool`, `float32`/`float64`, every signed and
unsigned integer width (plus pointers), same-package named types whose underlying
type is a basic scalar (string, bool, or a numeric scalar), `[]E`,
`map[string]V`, `[N]E`, `[]byte`, `any`, and nested same-package structs that are
generated too. It also resolves **cross-package named types whose underlying type
is a basic scalar** (`time.Duration`, `net/http.Dir`, or any named string, bool,
integer or float from another package), both as a field and as a slice or map
element type, using the standard library's type checker. It honours
`json:"name"`, `json:"-"`, `json:"-,"`, and
`json:",omitempty"` (which makes the encoder skip an empty field); an untagged
field uses the field name.

A field whose type is a **struct from another package** is supported too:

- If the foreign type implements both `DecodeJSON` and `EncodeJSON`, the
  generator binds it by delegation (`ObjectDecoder`/`ObjectDecoderPtr`, or
  `DecoderCodec` as a container element), so the other package's mapping is used
  exactly.
- Otherwise, for a plain foreign struct, the generator synthesizes a
  `<pkg><Type>Schema` var in your package from the struct's exported fields,
  recursing into nested foreign structs and emitting a type used on several paths
  once. Only exported fields round-trip; unexported ones are skipped (the
  generated file notes this), and an embedded foreign field is an error.
- For a type the generator cannot introspect, pass
  `-foreign=pkg.T=ident` to bind it to a `*qijson.Schema[pkg.T]` you own (repeat
  the flag per type). A malformed or unused entry is an error.

Still reported as errors: a cross-package type whose underlying type is not a
basic scalar or struct (such as `uuid.UUID`, an array), an unexported foreign
type, a foreign type implementing only one of `DecodeJSON`/`EncodeJSON`,
same-package pointer-to-struct fields, embedded fields, non-string map keys, and
numbers above `2^63-1` in `uint64` fields.

### Installing the generator

The generator is optional — the library works without it. There are three ways to
get it, from "nothing to install" to "a bare command on your `PATH`":

```
# 1. No install: uses the qi-json version already in your go.mod.
//go:generate go run github.com/qi-lore/qi-json/cmd/qi-json-gen -type=Config
go generate ./...

# 2. Pin it as a module tool (Go 1.24+): recorded in go.mod, run with `go tool`.
go get -tool github.com/qi-lore/qi-json/cmd/qi-json-gen
go tool qi-json-gen -type=Config

# 3. Install a bare command on PATH (match your imported version).
go install github.com/qi-lore/qi-json/cmd/qi-json-gen@v0.4.2
qi-json-gen -type=Config
```

Options 1 and 2 keep the generator version locked to the `qi-json` version in
your `go.mod`, which is the version generated code targets, so they are the
recommended default. Option 3 puts the binary in `$(go env GOBIN)` or
`$(go env GOPATH)/bin`; make sure that directory is on `PATH`, and prefer an
explicit `@vX.Y.Z` over `@latest` so the generator matches the library you
import.

### Invoking the generator

```
qi-json-gen -type=Config[,Other...] [-foreign=pkg.T=ident] [-strict] [-o file] [dir]
qi-json-gen [dir]   # every type marked //qi-json:generate
```

With `-strict`, every generated schema chains `RejectUnknown` and
`RejectDuplicate`, so decoding rejects unknown and duplicate object members
(see [Decoding rules](#decoding-rules)). The output is deterministic and checked
in; `TestGeneratedFixtureUpToDate` fails if it drifts from the generator.

## The syntax layer

```go
ok := qijson.Valid(data)                          // syntax only

tok := qijson.NewTokenizer(data)
kind, err := tok.Peek()                           // KindNull ... KindEnd
raw, err := tok.ReadString()                      // aliases data, still escaped
```

`Valid` accepts any JSON value at the top level, applies no nesting depth limit,
and does not check UTF-8 or surrogate pairing. `Tokenizer` is pull-based and
shares the same scalar scanners and error contract, so a decode failure reports
the same message and 1-based position as `Valid` would. It is the engine behind
the semantic layer; most callers should use a `Schema` instead. Byte slices
returned by `ReadString`, `ReadNumber`, and `ReadRaw` alias the input. `Valid` is
safe for concurrent use; a `Tokenizer` must not be copied or shared.

## Design constraints

- Standard library only; no `encoding/json` anywhere, including tests.
- `qijson` and `cmd/qi-json-gen` use neither `reflect` nor `unsafe`.
- The scanner's `Valid` hot path stays allocation-free for nesting up to 32
  levels, with no `goto`, labels, or per-byte handler dispatch.
- The decoder and encoder APIs are intentionally not drop-in replacements for
  `encoding/json`; see `docs/constitution.md` Article VI for the sanctioned
  surface.
- Tests are hand-built tables; TDD is mandatory.

## Performance

Measured on an Apple M1 with go1.27.3 (`make bench`). Treat them as ballpark
numbers, not guarantees.

| Benchmark | Result |
|---|---|
| `BenchmarkValidObject` | 829 MB/s, 0 allocs |
| `BenchmarkValidArrayOfObjects` | 820 MB/s, 0 allocs |
| `BenchmarkValidNumbers` | 1044 MB/s, 0 allocs |
| `BenchmarkValidString` | 2578 MB/s, 0 allocs |
| `BenchmarkDecodeStringsShort` (20k × 16 B) | 338 MB/s |
| `BenchmarkDecodeStringsMedium` (10k × 64 B) | 1033 MB/s |
| `BenchmarkDecodeStringsLong` (2k × 512 B) | 2350 MB/s |
| `BenchmarkDecodeBigString` (64 KiB) | 3172 MB/s |
| `BenchmarkStructDecode` | 447 ns/op, 1208 B/op, 4 allocs/op |
| `BenchmarkDecodeNonGeneric` | 301 ns/op, 1160 B/op, 3 allocs/op |
| `BenchmarkStructEncode` | 224 ns/op, 264 B/op, 7 allocs/op |

The decoder validates UTF-8 while decoding each string value and member name
(the syntax-only scanner does not), which adds a pass over the string bytes. On
string-heavy documents that costs roughly 8–25% versus a build without the check;
the medium-string case still exceeds 1 GB/s. The 16-byte case is allocation-bound,
not scan-bound. The encoder is a single append-only pass into one buffer, so it is
cheaper than the decoder for the same shape.

During development `Valid` was 1.10×–1.77× faster than `encoding/json.Valid` on
the four benchmark documents. That comparison is not part of the suite because
the module may not import `encoding/json`.

## Development

Every change must pass the repository's verification sequence, in order:

```
make verify    # go test ./... -> gofmt -l . -> go vet ./... -> golangci-lint run
```

Other useful targets:

```
make race      # go test -race ./...
make fuzz      # 10s FuzzValid on the scanner
make bench     # benchmarks with memory stats
make generate  # regenerate the codegen fixture
make cover     # coverage profile and per-function output
```

## Contributing

`main` is the production mainline and `dev` is the integration branch. Create a
feature or task branch from the current `main` (after `git fetch` and a
fast-forward update), name it per
[Conventional Branch](https://conventionalbranch.org/), and open a pull request
into `dev`. Only the `dev` → `main` pull request merges into `main`.

Commit messages follow
[Conventional Commits](https://www.conventionalcommits.org/) and carry a full
body. A pull request that closes an issue starts its description with
`Closes: #<number>` and states the problem and the solution. Confirm suggested
labels with a maintainer before applying them.

All Go code follows the
[Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
without exception, plus the repository's additional rules: wrap errors with
`%w`, never return a bare error from a deeper layer, define a sentinel in the
owning package where a caller must inspect it, document the ownership,
concurrency safety and lifecycle of public APIs, and never panic in library
code for a recoverable condition. The full rules are in
`docs/constitution.md` ("Git and Review Discipline" and "Rigorous Code
Quality").

CI (`.github/workflows/ci.yml`) runs the verification sequence and checks the
branch name, the target branch and the commit messages of each pull request.

## Repository layout

```
.                        public facade: type aliases, constants and wrappers (package qijson) + the facade contract test
internal/codec/          the engine: Reader, Writer, Schema, field constructors, entry points, errors, fuzz tests (package codec)
internal/scanner/        Valid, the tokenizer, benchmarks and differential tests
cmd/qi-json-gen/         the code generator and its end-to-end fixture
docs/tasks.md            the task registry: single source of truth for task state
docs/constitution.md     governing rules and the sanctioned public surface
.opencode/plan/          design rationale and history (no task state)
```

The decoder plan (`.opencode/plan/2026-11-21-json-decoder.md`) and the encoder
plan (`.opencode/plan/2026-11-21-json-encoder.md`) are the design records for the
semantic layer; the codegen plan
(`.opencode/plan/2026-11-21-json-codegen.md`) covers `qi-json-gen`. Task state
lives in [docs/tasks.md](docs/tasks.md).