# qi-json Decoder — decisions, tasks, and handoff

**Date**: 2026-11-21
**Status**: decisions agreed; decoder not started.
**Supersedes (in part)**: `.opencode/plan/2026-11-21-json-unmarshal-research.md`.
**Related**: `docs/task.md`, `docs/constitution.md`, `AGENTS.md`,
`.opencode/plan/2026-11-21-scanner-rewrite.md`.

---

## 1. Goal

`qi-json` is a from-scratch JSON library, **independent of `encoding/json`** and
**faster than the standard library**. The deliverable is a **decoder /
unmarshaller** that turns JSON text into Go values. The packages split by job:

- `scanner/` — the **syntax layer**. One pass over `[]byte`; today it exposes
  only `Valid`. It is the means, not the product.
- `marshal/` — the **semantic layer**, the decoder. **This is the goal.**

The scanner already beats `encoding/json.Valid` on all four benchmark documents
(1.10×–1.77×, 0 allocations), so the syntax core is no longer a blocker.

## 2. Decisions (agreed 2026-11-21)

- **D1 — API and reflection: reflection-free generic API.**
  ```go
  func Unmarshal[T any, PT interface{ *T; Decoder }](data []byte) (T, error)
  func UnmarshalInto[T any, PT interface{ *T; Decoder }](data []byte, v *T) error
  ```
  Struct binding uses compile-time **accessor closures** (`Field[T]`); no
  `reflect`, no `unsafe`. The API is deliberately **not**
  `encoding/json`-compatible — a documented choice, not a silent deviation.
- **D2 — The tokenizer lives in `scanner` (research option S1).** `scanner`
  gains an exported tokenizer alongside `Valid`. One engine, least churn,
  mirrors `encoding/json/jsontext`.
- **D3 — Sequencing: correctness first.** Build the decoder correct and
  allocation-tight; add bulk fast paths in a later, dedicated phase.

## 3. What changed since the research doc

`2026-11-21-json-unmarshal-research.md` predates the scanner rewrite. Two of its
load-bearing claims are now false or stale:

- Research §6.3, §6.4 and TL;DR caveat #4 ("the scanner is 2.3×–5.1× slower than
  the baseline; the scanner is the 10× lever") — **superseded.** The scanner now
  beats `encoding/json.Valid` on every benchmark document.
- Research §7.3 ("add span fields to the existing `machine`") — **stale.**
  `scanner/machine.go` is deleted. The scanner is a whole-slice validator that
  exposes no tokens or spans.

Everything else in the research (layering, accessor closures, error model,
semantic corner cases) still stands.

## 4. Architecture

```
scanner/                      syntax layer
  valid.go, scan.go           Valid + the single-pass scanner        (done)
  token.go         NEW        Kind + Tokenizer: Peek, Read*, Begin/End, Skip, Offset, Err
marshal/                      semantic layer (reflection-free)
  reader.go        NEW        Reader: typed access over the tokenizer + JSON path
  schema.go        NEW        Schema[T], Struct(fields...)
  field.go         NEW        String/Int/Float/Bool/Slice/Map/Object/Any/Raw
  unmarshal.go     NEW        Unmarshal[T], UnmarshalInto[T], Decoder interface
  errors.go        NEW        Error, TypeError, path formatting, errors.As
  doc.go                      updated package doc
```

`marshal` imports `scanner`. No cycles, no third-party code, no `encoding/json`,
no `reflect`, no `unsafe`.

### 4.1 Tokenizer design note — the T1 crux

The current scanner is a whole-slice validator; it does not emit spans. The
tokenizer must reuse the scalar scanners (`scanString`, `scanNumber`,
`scanLiteral`, `skipSpace`) to produce token boundaries and offsets while keeping
`Valid` unchanged and allocation-free. Evaluate two options in T1:

- **T1a (preferred)** — a separate `Tokenizer` type that walks the slice with the
  same helpers and records `kind`, `start`, `end`, reusing the scanner's error
  contract. Does not touch the `Valid` hot path.
- **T1b** — add span fields to `validator` and expose them. Cheaper sharing, but
  risks perturbing the `Valid` benchmarks that constitution Article V guards.

Prefer T1a; fall back to T1b only if T1a duplicates too much of `run`.

## 5. Tasks

Each task is TDD (failing test first), ends with the mandated verification
(`make verify`, plus `-race` and fuzz where relevant), and does not start until
the previous task is green.

### T0 — Governance (this change; no code)
- Amend `docs/constitution.md`: Article VI + Additional Constraints, to allow the
  `scanner` tokenizer and the `marshal` surface, and to forbid `reflect`/`unsafe`
  in `marshal`.
- Update `AGENTS.md`; rewrite `marshal/doc.go`.
- **Acceptance**: docs landed; `make verify` still green.

### T1 — Tokenizer in `scanner` (`scanner/token.go`, `scanner/token_test.go`)
Tests: `Peek` kind per value; `Read` raw string/number/raw bytes; `Begin`/`End`
object/array; `Skip` nested; `Offset`; errors match `Valid`'s message and
position; `Tokenizer` agrees with `Valid` over `TestValidDocuments` +
`TestInvalidDocuments`.
**Acceptance**: all pass; `Valid` benchmarks show no regression beyond noise.

### T2 — `marshal` primitives and errors (`reader.go`, `errors.go`)
Tests: `\n \t \" \\ \/ \uXXXX` unescaping; int/float parse, overflow, `-0`,
exponents; bool/null; `$.a.b[2].c` path; `errors.As` unwrap.
**Acceptance**: zero allocations on the primitive path (`testing.AllocsPerRun`).

### T3 — `Schema[T]` and struct binding (`schema.go`, `field.go`)
Tests: flat struct, all field types; unknown field ignored/rejected; duplicate
last-wins/strict; nested object; null into scalar and pointer;
`TestStructNoAllocs`; field lookup with 1/2/7/64 fields.
**Acceptance**: `make verify` green; decode benchmark recorded.

### T4 — Generic entry point and interface (`unmarshal.go`)
Tests: `Unmarshal[Config]`; `UnmarshalInto` merge; `Decoder` dispatch; invalid
target (nil pointer, wrong type).
**Acceptance**: `go doc ./marshal` shows only the intended surface.

### T5 — Containers (`field.go`)
Tests: slice of primitives/structs, empty; map of strings/structs, empty; fixed
arrays overflow/underflow; `any` to map/array/string/float64/bool/nil; `Raw`.
**Acceptance**: `make verify` green.

### T6 — Fuzz and differential testing
`FuzzUnmarshal` (never panics; error offset in range);
`FuzzUnmarshalMatchesValid`.
**Acceptance**: 10s fuzz clean.

### T7 — Bulk fast paths (performance)
String/number/whitespace fast paths with a differential test against the
reference path.
**Acceptance**: ≥ 1 GB/s on string-heavy documents; 0 allocs; verify green.

### T8 — Optional codegen (`cmd/qi-json-gen`) — deferred.
### T9 — Documentation — deferred.

## 6. Open questions still to settle (from research §10)

- **Semantic defaults** at T3/T5: unknown members (recommend *ignored*, strict
  opt-in); duplicate members (recommend *last wins*, strict opt-in); `null` into
  a non-pointer scalar (recommend *leave zero*); number overflow (error, not
  truncation).
- **UTF-8 policy** in the decoder: reject invalid UTF-8 in strings, or pass
  through as the scanner does? Recommend reject in the decoder; the scanner stays
  syntax-only.
- **Encoder** (`Marshal`): out of scope until the decoder is done. Encoding
  arbitrary Go types likely needs reflection or codegen — a separate amendment.

## 7. Target API (sketch)

```go
// scanner (syntax)
type Kind uint8
const ( KindNull Kind = iota; KindBool; KindString; KindNumber; KindObject; KindArray )

type Tokenizer struct { /* ... */ }
func NewTokenizer(data []byte) *Tokenizer
func (t *Tokenizer) Peek() (Kind, error)
func (t *Tokenizer) ReadNull() error
func (t *Tokenizer) ReadBool() (bool, error)
func (t *Tokenizer) ReadString() ([]byte, error)   // raw, still escaped
func (t *Tokenizer) ReadNumber() ([]byte, error)   // raw literal
func (t *Tokenizer) ReadRaw() ([]byte, error)      // whole value, composites included
func (t *Tokenizer) BeginObject() error
func (t *Tokenizer) EndObject() error
func (t *Tokenizer) BeginArray() error
func (t *Tokenizer) EndArray() error
func (t *Tokenizer) Skip() error
func (t *Tokenizer) Offset() int64
func (t *Tokenizer) Err() error

// marshal (semantics)
type Decoder interface { DecodeJSON(*Reader) error }

func Unmarshal[T any, PT interface{ *T; Decoder }](data []byte) (T, error)
func UnmarshalInto[T any, PT interface{ *T; Decoder }](data []byte, v *T) error

type Schema[T any] struct { /* ... */ }
func Struct[T any](fields ...Field[T]) *Schema[T]
func (s *Schema[T]) Decode(r *Reader, v *T) error
func (s *Schema[T]) Unmarshal(data []byte) (T, error)

type Field[T any] struct {
    Name   string
    Decode func(*Reader, *T) error
}
func String[T any](name string, field func(*T) *string) Field[T]   // and Int/Float/Bool/Slice/Map/Object/Any/Raw

type Reader struct { /* ... */ }
func (r *Reader) Object(fn func(key string) error) error
func (r *Reader) Array(fn func(index int) error) error
func (r *Reader) Skip() error
func (r *Reader) Path() string
func (r *Reader) Offset() int64
```

Usage target:

```go
var configSchema = marshal.Struct(
    marshal.String[Config]("name", func(c *Config) *string { return &c.Name }),
    marshal.Int[Config]("port", func(c *Config) *int64 { return &c.Port }),
)

cfg, err := configSchema.Unmarshal(data)   // or marshal.Unmarshal[Config](data)
```

Names may be refined while implementing; the mechanism (accessor closures,
generic entry point, tokenizer in `scanner`) is fixed by D1–D3.

## 8. Session state (handoff) — resume here

- **Repo**: git repository, branch `main`. Last commit `3d47c70`
  `perf(scanner): replace state machine with goto-free scanner`. Tree clean
  before this docs change.
- **Done**: the scanner — `scanner/scan.go` + `valid.go`: single pass, no `goto`,
  faster than `encoding/json.Valid`, 0 allocs.
- **Not started**: everything under `marshal/`, and `scanner/token.go`.
- **Next action**: T0 is landed by this doc (plus the constitution, `AGENTS.md`
  and `marshal/doc.go`). Then start **T1**: write `scanner/token_test.go` (red)
  for the tokenizer API in §7, then implement `scanner/token.go` (green).
- **Honor**: D1 (reflection-free generic API), D2 (tokenizer in `scanner`),
  D3 (correctness first).
- **Do not**: import `encoding/json`; use `reflect` or `unsafe` in `marshal`; add
  exports beyond the amended constitution without a new amendment; reintroduce
  `goto`/labels or per-byte dispatch in the scanner.
- **Commands**:
  - `make verify` — test → gofmt → vet → golangci-lint.
  - `go test -race ./...`
  - `go test -run=^$ -fuzz=FuzzValid -fuzztime=10s ./scanner`
  - `go test -run=^$ -bench=. -benchmem ./...`
