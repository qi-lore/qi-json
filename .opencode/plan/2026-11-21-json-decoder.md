# qi-json Decoder — decisions, tasks, and handoff

**Date**: 2026-11-21
**Status**: decisions agreed; T0–T7 done; fuzz, differential testing and the bulk
fast paths are green.
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
  doc.go           NEW        Package documentation                  (done)
marshal/                      semantic layer (reflection-free)
  reader.go        NEW        Reader: typed access over the tokenizer + JSON path
  schema.go        NEW        Schema[T], Struct(fields...)
  field.go         NEW        String/Int/Float/Bool/Slice/Map/Array/Object/Any/Raw
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

Status legend: `[X]` — completed and verified; `[ ]` — not completed. The word
after each heading is the state: **done**, **next** (the task to start),
**pending**, or **deferred**. Details and verification evidence for completed
tasks are in §8.

### [X] T0 — Governance (this change; no code) — **done**
- Amend `docs/constitution.md`: Article VI + Additional Constraints, to allow the
  `scanner` tokenizer and the `marshal` surface, and to forbid `reflect`/`unsafe`
  in `marshal`.
- Update `AGENTS.md`; rewrite `marshal/doc.go`.
- **Acceptance**: docs landed; `make verify` still green.

### [X] T1 — Tokenizer in `scanner` (`scanner/token.go`, `scanner/token_test.go`) — **done**
Tests: `Peek` kind per value; `Read` raw string/number/raw bytes; `Begin`/`End`
object/array; `Skip` nested; `Offset`; errors match `Valid`'s message and
position; `Tokenizer` agrees with `Valid` over `TestValidDocuments` +
`TestInvalidDocuments`.
**Acceptance**: all pass; `Valid` benchmarks show no regression beyond noise.

### [X] T2 — `marshal` primitives and errors (`reader.go`, `errors.go`) — **done**
Tests: `\n \t \" \\ \/ \uXXXX` unescaping; int/float parse, overflow, `-0`,
exponents; bool/null; `$.a.b[2].c` path; `errors.As` unwrap.
**Acceptance**: zero allocations on the primitive path (`testing.AllocsPerRun`).

### [X] T3 — `Schema[T]` and struct binding (`schema.go`, `field.go`) — **done**
Tests: flat struct, all field types; unknown field ignored/rejected; duplicate
last-wins/strict; nested object; null into scalar and pointer;
`TestStructNoAllocs`; field lookup with 1/2/7/64 fields.
**Acceptance**: `make verify` green; decode benchmark recorded
(`BenchmarkStructDecode-8  412.0 ns/op  1048 B/op  4 allocs/op`).

### [X] T4 — Generic entry point and interface (`unmarshal.go`) — **done**
Tests: `Unmarshal[Config]`; `UnmarshalInto` merge; `Decoder` dispatch; invalid
target (nil pointer, wrong type).
**Acceptance**: `go doc ./marshal` shows only the intended surface.

### [X] T5 — Containers (`field.go`) — **done**
Tests: slice of primitives/structs, empty; map of strings/structs, empty; fixed
arrays overflow/underflow; `any` to map/array/string/float64/bool/nil; `Raw`.
**Acceptance**: `make verify` green.

### [X] T6 — Fuzz and differential testing — **done**
`FuzzUnmarshal` (never panics; every failure is an `*Error` with a JSON path
and an in-range offset; a successful decode is deterministic);
`FuzzUnmarshalMatchesValid` (success ⇒ `Valid`; a syntax error ⇒ `!Valid`).
**Acceptance**: 10s fuzz clean on both targets. See §8 for the edge case the
fuzzer surfaced.

### [X] T7 — Bulk fast paths (performance) — **done**
String/number/whitespace fast paths with a differential test against the
reference path.
**Acceptance**: ≥ 1 GB/s on string-heavy documents; 0 allocs; verify green.
Result: the decoder won the throughput; the scanner's existing SWAR string path
already meets the target, and the number/whitespace word paths were measured to
regress and are not shipped (see §8).

### [ ] T8 — Optional codegen (`cmd/qi-json-gen`) — **deferred**.
### [ ] T9 — Documentation — **deferred**.
### [ ] T10 — Non-generic entry point (API ergonomics) — **deferred**

Today every decoder entry point is generic, so a caller must name an
instantiation: `marshal.Unmarshal[Config](data)`,
`marshal.String[Config](…)`. A caller can already write a non-generic wrapper
(`func decodeConfig(data []byte) (Config, error) { return
marshal.Unmarshal[Config](data) }`), but that is boilerplate repeated per type.

Add a package-level non-generic entry point so the common case needs no type
arguments. It **cannot** return a fresh value — that needs reflection, which
`marshal` forbids — so the reflection-free shape is **decode-into**, and the
name cannot be `Unmarshal`/`UnmarshalInto` (Go has no overloading). Candidate:

```go
func Decode(data []byte, v Decoder) error   // v is e.g. &cfg
```

A target already implements `Decoder` when it has a `DecodeJSON` method, so no
new machinery is needed: it is a thin wrapper over `newReader` + `DecodeJSON` +
the same trailing-value check `UnmarshalInto` performs.

**Blocked on governance**: this is a new exported identifier, which
`docs/constitution.md` (Article VI; Additional Constraints) says requires an
amendment. Amend Article VI and `AGENTS.md` together before shipping.

**Acceptance**: `marshal.Decode(data, &cfg)` decodes and reports trailing data
and syntax/type errors exactly like `UnmarshalInto`; the generic API is
unchanged; amendment recorded; `make verify`, `-race`, fuzz green.

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

- **Repo**: git repository, branch `main`. T4 is commit `bccd46e`
  `feat(marshal): add generic Unmarshal entry point and Decoder interface`; T5
  (`marshal/field.go`, `marshal/field_test.go`, `marshal/reader.go`,
  `marshal/doc.go`) is committed on top.
- **Done**: the scanner — `scanner/scan.go` + `valid.go`: single pass, no `goto`,
  faster than `encoding/json.Valid`, 0 allocs. **T0** (governance: constitution
  Article VI + Additional Constraints, `AGENTS.md`, `marshal/doc.go`). **T1**
  (`scanner/token.go` + `scanner/token_test.go`): the tokenizer — `Kind`,
  `Tokenizer`, `NewTokenizer`; the T1a design (a separate `tokenizer` state that
  reuses the validator's scalar scanners and error contract, leaving `Valid`
  untouched). Tests cover `Peek` per value, raw `ReadString`/`ReadNumber`/
  `ReadRaw`, `Begin`/`End` for objects and arrays, nested `Skip`, `Offset`,
  sticky `Err`, misuse errors, and full agreement with `validate` over
  `validDocuments` and `invalidDocuments`. `make verify`, `-race`, a 10s
  `FuzzValid` and the benchmarks are green; `Valid` still allocates 0.
- **T2** (`marshal/reader.go` + `marshal/errors.go` + `marshal/reader_test.go`):
  the `Reader` over `scanner.Tokenizer` and the error types.
  - `Reader`: `Peek`, `ReadNull`, `ReadBool`, `ReadString` (decoded: escapes
    resolved, lone surrogate → U+FFFD), `ReadInt` (`int64`, integer literal only,
    range-checked), `ReadFloat` (`float64`; overflow is an error, underflow → 0),
    `Object`/`Array` iterators with callbacks, `Skip`, `Path`, `Offset`.
  - Errors: `Error` (`Path`, `Offset`, `Kind`, `Err`; `Unwrap`), `ErrorKind`
    (`KindSyntax`, `KindType`, `KindUnknownField`, `KindDuplicateField`,
    `KindDepth`), and `TypeError` (`Want`, `Got`) reachable through
    `errors.As`/`errors.Unwrap`. No scanner or constitution change was needed:
    `Reader` and the error types are already sanctioned by Article VI.
  - Path tracking uses an inline 64-byte buffer (mirroring the scanner's
    `nestingInline` idea), so primitives do not allocate; `Path` renders keys
    plainly (`$.a.b`) or quoted (`$["a b"]`) and indices as `[2]`.
  - `ReadInt`/`ReadFloat` use `strconv.ParseInt`/`ParseFloat(string(raw))`; the
    byte-to-string conversion is stack-allocated by the compiler (measured 0
    allocs), so the primitive read path does not allocate.
  - Tests: unescaping (`\n \t \" \\ \/ \uXXXX`, surrogate pair, lone surrogate);
    int/float range, overflow, `-0`, exponents; bool/null and type mismatches;
    `$.a.b[2].c` path and offset exactness; quoted-key path; `errors.As`/
    `errors.Unwrap` for type and syntax errors; and `TestPrimitiveNoAllocs`
    (differential against tokenizer construction). `make verify`, `-race`, the
    scanner benchmarks (no change) are green.
- **T3** (`marshal/schema.go` + `marshal/field.go` + `marshal/schema_test.go`):
  `Schema[T]` and struct binding.
  - `Field[T]` (`Name`, `Decode func(*Reader, *T) error`) plus the constructors
    `String`/`Int`/`Float`/`Bool` (value fields) and `StringPtr`/`IntPtr`/
    `FloatPtr`/`BoolPtr` (pointer fields), and `Object[T, S]` for a nested
    `*Schema[S]`. All are sanctioned by Article VI ("the `Field[T]`
    constructors"); no new amendment.
  - `Schema[T]` with `Struct[T](fields ...Field[T])` (sorts fields by name),
    `Decode`, `Unmarshal`, `UnmarshalInto`, and the strict opt-ins
    `RejectUnknown` / `RejectDuplicate`.
  - **Key allocation resolved** (the T3 design pressure): a new unexported
    `Reader.objectRaw(fn func(raw []byte) error)` iterates members with the raw
    (still escaped) key bytes and does not touch the path; `Schema` matches the
    key against field names with a byte-wise binary search (`compareKey`), so the
    hot path never builds a key string. A key containing `\` is unescaped and
    re-matched (rare path).
  - **Pointer fields**: the union-constraint trick (`E string | *string`) was
    rejected — boxing `*E` to `any` for the type switch allocates (measured 1
    alloc/field). Separate `*Ptr` constructors keep the scalar path at 0 allocs.
  - **Null semantics**: a JSON `null` leaves a value field unchanged (zero for a
    fresh value) and sets a pointer field to nil. Documented in `doc.go`.
  - **Type vs syntax**: a non-object where an object is expected is now a
    `KindType` error, not `KindSyntax`. New `Reader.beginObject`/`beginArray`
    helpers peek and report a type mismatch; `Object`, `Array` and `objectRaw`
    all use them (previously `BeginObject` surfaced a syntax error).
  - Tests: flat struct (all field types), key-order independence, escaped key,
    unknown ignored/rejected, duplicate last-wins/rejected, nested object and
    nested error path (`$.origin.x`), null into scalar/pointer (fresh and merge),
    pointer fields, non-object input, trailing data, empty object,
    `TestStructFieldLookup` (1/2/7/64 fields), and `TestStructNoAllocs` (the
    decode machinery allocates 0; the target is hoisted so `&v` is not counted).
  - **Benchmark recorded** (Apple M1, go1.27.3):
    `BenchmarkStructDecode-8  412.0 ns/op  1048 B/op  4 allocs/op` — the four
    allocations are the tokenizer, the reader, the escaped result value, and the
    decoded string.
- **T4** (`marshal/unmarshal.go` + `marshal/unmarshal_test.go` + `doc.go` +
  Article VI): the generic entry point and interface.
  - `Decoder` (`DecodeJSON(*Reader) error`), `Unmarshal[T any, PT interface{ *T;
    Decoder }]`, and `UnmarshalInto[T, PT]`. The generic form uses the
    pointer-receiver constraint trick so the type argument alone selects the
    decoder; `PT(v)` converts the target to the constraint type to call the
    method. `Unmarshal` starts from a zero T; `UnmarshalInto` decodes in place
    (merge). Both verify the document holds exactly one value (a trailing `Peek`),
    matching `Schema.UnmarshalInto`.
  - **Invalid target**: a nil `*T` returns the unexported sentinel `errNilTarget`
    (a programming error, not an `*Error`). It adds no exported identifier, so no
    amendment beyond naming `Decoder`. A JSON type that does not match T still
    reports `KindType`; invalid JSON reports `KindSyntax`; trailing data reports
    `KindSyntax`.
  - **Amendment**: Article VI now names the `Decoder` interface in the sanctioned
    `marshal` surface (it was implicit in the T0 wording). `AGENTS.md` refers to
    Article VI generically, so it needed no change.
  - Tests: `TestUnmarshalGeneric`, `TestUnmarshalIntoMerge`,
    `TestUnmarshalDecoderDispatch` (a type implementing `DecodeJSON` directly,
    proving dispatch rather than Schema), `TestUnmarshalInvalidTarget`
    (nil pointer / wrong type / invalid JSON / trailing data),
    `TestUnmarshalIntoPreservesTargetOnError`.
  - `go doc ./marshal` lists exactly the intended surface; `errNilTarget` is
    unexported and absent.
  - **Benchmark recorded** (Apple M1, go1.27.3):
    `BenchmarkUnmarshalGeneric-8  285.2 ns/op  1024 B/op  4 allocs/op` (same four
    allocations as T3). `Valid` benchmarks unchanged.
- **T5** (`marshal/field.go` + `marshal/field_test.go`): the container fields.
  - `Slice[T, E]` (JSON array → `[]E`) and `Map[T, V]` (JSON object →
    `map[string]V`), each taking an element decoder `func(*Reader) (E, error)`;
    a JSON `null` leaves the field unchanged and an empty array/object yields a
    non-nil empty slice/map. Both build into a temporary and assign only on
    success, so a failed decode leaves the target unchanged.
  - `Array[T, E]` (fixed-size array): the accessor returns a slice view of the
    array (`func(c *C) []E { return c.V[:] }`), so the decoder writes through;
    the JSON array must hold exactly `len(field(v))` elements, with underflow and
    overflow reported as `KindType` errors (`arrayLengthError`, offset at the
    `[`). This is the only reflection-free way to support a real Go array without
    `unsafe` (Go has no const type parameters); the view keeps the element type
    known at compile time.
  - `Any[T]` (JSON value → `any`): `decodeAny` recurses without reflection —
    object → `map[string]any`, array → `[]any`, string → `string`, number →
    `float64`, boolean → `bool`, null → `nil`.
  - `Raw[T]` (JSON value → `[]byte`): reads the exact literal bytes via a new
    unexported `Reader.raw()` over `Tokenizer.ReadRaw`, and copies them so the
    field does not alias the input.
  - No new amendment needed: these are `Field[T]` constructors, sanctioned by
    Article VI.
  - Tests: `TestSliceOfPrimitives`, `TestSliceOfStructs`, `TestEmptySlice`,
    `TestSliceNullLeavesFieldUnchanged`, `TestSliceTypeError`, `TestMapOfStrings`,
    `TestMapOfStructs`, `TestEmptyMap`, `TestMapTypeError`,
    `TestArrayFixedSize` (exact/overflow/underflow), `TestAny`
    (object/array/string/float64/bool/null), `TestRaw`, and
    `TestRawDoesNotAliasInput`. `make verify`, `-race`, `go doc ./marshal` green.
- **T6** (`marshal/fuzz_test.go`): fuzz and differential testing.
  - `fuzzAny` is a Schema-less `Decoder` that decodes any value at the document
    root with `decodeAny`, so one target exercises the whole semantic layer over
    arbitrary input.
  - `FuzzUnmarshal` asserts no panic, that every failure is an `*Error` with a
    non-empty path and an offset in `0..len(data)`, and that a successful decode
    is deterministic (checked with a reflection-free `anyEqual`, since `marshal`
    may not use `reflect`).
  - `FuzzUnmarshalMatchesValid` asserts the two sound directions: a successful
    decode proves the document is valid (`Valid`), and a reported syntax error
    proves it is not. The converse does not hold and is not asserted.
  - **Edge case the fuzzer surfaced** (`1e700A`): a number that overflows
    `float64` is rejected as a **type** error before the trailing garbage is
    reached, so invalid JSON does not always yield a syntax error. The test was
    corrected to assert only the sound directions; `1e700A`/`1e999x` are kept as
    seeds. This matches `encoding/json`, which likewise converts the number
    before checking for trailing data.
  - `make verify`, `-race`, and 10s fuzz on both targets are green.
- **T7** (`scanner/fastpath_test.go`, `marshal/reader.go`, `marshal/schema.go`,
  `marshal/bench_test.go`): bulk fast paths and their differential tests.
  - **Differential tests** (`scanner/fastpath_test.go`): plain byte-at-a-time
    reference scanners (`skipSpaceRef`, `scanStringRef`, `scanEscapeRef`,
    `scanNumberRef`) are compared against the production `skipSpace`,
    `scanString`/`scanEscape`, and `scanNumber` over the fixed corpora plus
    seeded random fragments, asserting the same return index and the same (or
    no) failure down to the 1-based position and the message. This pins the
    scanner's SWAR string path (and the scalar scanners generally) so a future
    fast path cannot silently change an index or an error.
  - **Scanner fast paths — measured, not shipped.** The string fast path
    (`stringStopWord` SWAR, warm-up gated) already exists and is what the
    differential tests validate. Two further word-at-a-time paths were built and
    benchmarked, both with the byte-at-a-time form as the reference:
    - *Number* (SWAR digit-run skip, warm-up gated at eight digits): regressed
      every benchmark — `BenchmarkValidNumbers` 1048 → 885 MB/s and the object
      and array documents ~2%. The extra work on the common one-to-four-digit
      number costs more than the rare eight-plus-digit run saves, even with the
      helper inlined (the helper is too large to inline, so calling it per digit
      run is what regressed at first).
    - *Whitespace* (eight-byte skip of space runs): regressed pretty-printed
      input at every indent tried — 4, 16 and 64 spaces and a document whose
      only whitespace runs were newline plus sixteen spaces — by 9–22%. Inlined
      at each `run` call site the word loop bloats `run`, and the byte loop is
      already cheap enough that the per-call overhead outweighs the bytes
      skipped. `skipSpace` is therefore unchanged save for a comment recording
      this.
  - **Decoder fast path — the throughput win.** Two changes to `Reader`:
    - *Lazy JSON path*: instead of appending formatted path text (`.key`,
      `[i]`) while descending, `Reader` keeps a stack of `pathComp` (member
      name or index) in an inline `[8]pathComp` slice, and `Path` renders the
      string only when an error needs it. A successful decode formats nothing;
      removing the per-element `strconv.AppendInt` was worth ~16% on the
      many-string document.
    - *Memoized `Peek`*: a `Peek` followed by a read peeked twice. `Reader`
      memoizes the kind keyed on the tokenizer pointer, its offset and a nil
      sticky error, so a read reuses the just-made `Peek`; the entry can never
      hide a consumed token or a sticky error. Worth ~4–5% alone.
  - **Benchmarks recorded** (Apple M1, go1.27.3; `BenchmarkDecodeStrings*` in
    `marshal/bench_test.go`), before → after: many medium strings (10 000 × 64 B)
    981 → **~1120 MB/s** (acceptance met), long strings (2 000 × 512 B)
    ~2 685 → ~2 780 MB/s, short strings (20 000 × 16 B) ~296 → ~350 MB/s (the
    short case is allocation-bound, not scanner-bound), one 64 KiB string
    ~4 162 → ~4 100 MB/s (within noise). `BenchmarkStructDecode` stayed
    ~415 ns/op, 1208 B/op, 4 allocs; `BenchmarkUnmarshalGeneric` ~294 ns/op.
  - **Scanner benchmarks** are unchanged from T6 within noise:
    `ValidObject` ~831 MB/s, `ValidArrayOfObjects` ~821, `ValidNumbers` ~1039,
    `ValidString` ~2530–2575 MB/s — all `0 B/op`, `0 allocs/op`.
  - `make verify`, `-race`, and 12s fuzz on all three targets are green.
- **Not started**: the remaining tasks (T8–T10), all deferred. T8 (codegen) and
  T9 (documentation) are optional. **T10** (a non-generic entry point such as
  `marshal.Decode(data, &cfg)`) is a small, high-value ergonomics task but is
  **blocked on a constitution amendment**, because it adds an exported
  identifier; the design is in §5 T10.
- **Next action**: none required for the decoder. If resumed, T9 (package docs,
  README, and folding §8's results into `doc.go`) and then T10 (amendment
  first).
- **Honor**: D1 (reflection-free generic API), D2 (tokenizer in `scanner`),
  D3 (correctness first).
- **Do not**: import `encoding/json`; use `reflect` or `unsafe` in `marshal`; add
  exports beyond the amended constitution without a new amendment; reintroduce
  `goto`/labels or per-byte dispatch in the scanner. The tokenizer must not be
  copied after first use (its frame stack points into itself).
- **Commands**:
  - `make verify` — test → gofmt → vet → golangci-lint.
  - `go test -race ./...`
  - `go test -run=^$ -fuzz=FuzzValid -fuzztime=10s ./scanner`
  - `go test -run=^$ -bench=. -benchmem ./...`
