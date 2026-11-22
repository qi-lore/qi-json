# Deep Research — Reflection-Free JSON Unmarshalling

**Date**: 2026-11-21
**Status**: Research + proposal. Decisions taken 2026-11-21; the agreed API,
task list and session handoff live in `.opencode/plan/2026-11-21-json-decoder.md`.
**Superseded here**: §6.3, §6.4 and TL;DR caveat #4 (the scanner is now faster
than the standard library) and §7.3 (the state machine was deleted). The rest
still stands.
**Scope**: How to implement `Unmarshal` in `qi-json/marshal` without `reflect`.
**Related**: `docs/task.md`, `docs/constitution.md`, `AGENTS.md`,
`.opencode/plan/2026-11-21-json-decoder.md`.

---

## 1. TL;DR

1. **Reflection is only needed for one thing: binding JSON object members to
   struct fields.** Everything else — literals, strings, numbers, slices, maps,
   `any` — can be decoded with static code. That single insight collapses the
   problem from "avoid reflection in a JSON decoder" to "bind struct fields
   without reflection".
2. **Recommended binding mechanism: accessor closures.** A `Field[T]` carries a
   JSON member name plus a `func(*T) *Field` accessor. No reflection, no
   `unsafe`, no code generation, fully type-checked at compile time. Measured
   ~3.1× faster than cached-index reflection and ~13.7× faster than
   `FieldByName` for a 5-field bind.
3. **Recommended public API: a generic entry point with the pointer-receiver
   constraint trick**, verified to compile and infer correctly on go1.27.3:

   ```go
   func Unmarshal[T any, PT interface{ *T; Decoder }](data []byte) (T, error)
   ```

   Callers write `cfg, err := marshal.Unmarshal[Config](data)`; the compiler
   infers `PT = *Config` by constraint type inference. No reflection, no `any`
   boxing, no pointer argument.
4. **The honest caveat: reflection is *not* the main performance problem in this
   repository — the scanner is.** Head-to-head in one binary, our scanner is
   **1.8×–3.8× slower than the stdlib baseline** on identical inputs (§6.3).
   Two cheap fixes recover 25–35% of that; the rest needs bulk scanning.
   Reflection-free binding saves ~29 ns per small object while the scanner burns
   ~346 ns on the same object. **Remove reflection for allocation, safety and
   predictability — but the scanner is where the speed is.**
5. **Compatibility consequence:** `func Unmarshal(data []byte, v any) error` is
   *impossible* without reflection. The `marshal` package's stated goal of
   "encoding/json-compatible API" cannot be met for the decode direction. This
   needs an explicit amendment, not a quiet deviation.

---

## 2. Constraints inherited from the repository

| Constraint | Source | Impact on this design |
|---|---|---|
| Go ≥ 1.23, toolchain go1.27.3 | `docs/task.md` §3, `go.mod` | Generics, `errors.As`, `slices`/`maps` available. *Now ≥ 1.24 (v0.3.0).* |
| Zero external dependencies | `docs/task.md` §3 | No easyjson/jsoniter/goccy. Code generation must be our own. |
| Never use `encoding/json` | `docs/task.md` §3, constitution III | No reflection fallback via stdlib; no "compat" shim. |
| Fastest correct implementation | `docs/task.md` §3 | Benchmarks must justify every layer. |
| TDD mandatory | constitution II | Tests precede each phase. |
| Minimal public surface | constitution VI | Every new export needs an amendment. |
| Syntax-only validator | constitution IV | No UTF-8 or surrogate validation in the scanner. |

---

## 3. Baseline: what `encoding/json` does today

`encoding/json.Unmarshal(data, v)` in v1:

```
Unmarshal
  └─ reflect.ValueOf(v)                    // interface → reflect.Value
     ├─ kind check: Pointer? nil?          // InvalidUnmarshalError
     └─ decodeState.unmarshal(v)
          ├─ d.value(rv) → d.object(v)
          │     ├─ cachedTypeFields(t)      // sync.Map lookup + struct walk
          │     ├─ key: d.literalStore()    // string alloc per key
          │     ├─ v.Field(field.index...)  // per-field addressable Value
          │     └─ d.value(subv) → literalStore → v.SetString/SetInt/SetFloat/SetBool
          └─ d.savedError / errorContext
```

Cost centres, in order of typical impact:

1. **Allocation per key** — `literalStore` copies the key bytes into a `string`.
2. **Allocation per string value** — `v.SetString(string(s))` copies.
3. **`reflect.Value` boxing and addressable copies** — each `d.value(subv)`
   passes a `reflect.Value` by value; `SetString`/`SetInt` cross an interface
   and a type switch.
4. **`cachedTypeFields`** — one `sync.Map` load per `Unmarshal` call, plus a
   struct walk on first use. Cached, but contended under concurrency.
5. **Interface dispatch** — `Unmarshaler`/`TextUnmarshaler` checks per value.

The `encoding/json/v2` redesign confirms the diagnosis. It keeps reflection for
the general case but adds an explicit escape hatch:

```go
// go doc encoding/json/v2.UnmarshalerFrom
type UnmarshalerFrom interface {
    UnmarshalJSONFrom(*jsontext.Decoder) error
}
```

and moves syntax into a separate, reflection-free package with an explicit
token/value API:

```go
// go doc encoding/json/jsontext.Decoder
func (d *Decoder) PeekKind() (Kind, error)
func (d *Decoder) ReadToken() (Token, error)
func (d *Decoder) ReadValue() (Value, error)
func (d *Decoder) SkipValue() error
func (d *Decoder) InputOffset() int64
func (d *Decoder) StackDepth() int
func (d *Decoder) UnreadBuffer() []byte
```

The v2 docs also state the reason for the split: syntax ("processing JSON
text") is separable from semantics ("mapping to Go values"), and only the
semantic layer needs reflection. This is the strongest available precedent for
our own layering, and it is verifiable locally with `go doc` — no web access
needed.

**Prior art worth knowing (qualitative, not re-benchmarked here):**

- `easyjson`, `ffjson` — per-type code generation; `UnmarshalJSON` emitted at
  build time. Fast, but codegen debt and unmaintained in `ffjson`'s case.
- `jsoniter` — reflection by default, optional "static" codegen mode.
- `goccy/go-json`, `sonic` — drop-in replacements using `unsafe` and assembly.
- `buger/jsonparser` — zero-allocation *read-only* extraction; no binding.

None of these is available to us (zero dependencies), but they establish that
both viable paths — codegen and hand-written interfaces — are well-trodden.

---

## 4. What "without reflection" actually means

A JSON decoder has two independent jobs:

| Job | Needs reflection? | Why |
|---|---|---|
| Recognise syntax, produce tokens | No | Byte scanning. |
| Convert tokens to primitives | No | `strconv.ParseInt`, `ParseFloat`, `Unquote`. |
| Fill slices, maps, `any` | No | `append`, `map[k]=v`, interface boxing. |
| **Fill struct fields** | **Yes, in the general case** | Field set is a *compile-time* property; the runtime needs a name→offset mapping. |

So the entire problem reduces to: **how do we obtain a name → field mapping
without asking `reflect` at runtime?** There are exactly five answers:

1. **Generate the mapping at build time** (codegen) — the mapping is source code.
2. **Hand-write the mapping** — the mapping is source code.
3. **Use `unsafe` offsets** — the mapping is a number; still needs a source.
4. **Keep reflection** — the mapping is computed at runtime.
5. **Don't have structs** — expose a token API and let callers bind themselves.

Options 1 and 2 are the same mechanism at different levels of automation; option
3 is 1/2 plus `unsafe`; option 4 is what we are asked to avoid; option 5 is a
fallback, not a product.

---

## 5. Design space

### A1 — Code generation

Emit `UnmarshalJSON` per annotated type.

```go
//go:generate qi-json-gen -type=Config
func (c *Config) UnmarshalJSON(data []byte) error { /* generated */ }
```

- Reflection-free: yes. Codegen: mandatory. `unsafe`: no.
- Fastest possible binding; unknown-field handling resolved at build time.
- Costs: generator to write and maintain, build-step friction, poor fit for
  third-party or dynamically-typed types, binary bloat.
- Verdict: **good optional accelerator, bad foundation.**

### A2 — Hand-written interface

```go
type Decoder interface { DecodeJSON(*Reader) error }
```

- Reflection-free: yes. Codegen: no. `unsafe`: no.
- Costs: boilerplate per type; the user writes the field mapping.
- Verdict: **correct foundation, but the boilerplate is the product defect.**

### A3 — Generics + interface

Same as A2, with a generic entry point so callers get a typed result:

```go
func Unmarshal[T any, PT interface{ *T; Decoder }](data []byte) (T, error)
```

- Verified: compiles and infers on go1.27.3 (see §6.1).
- Verdict: **adopt — it is A2 with a better signature.**

### A4 — `unsafe` offsets

```go
type Field struct { name string; offset uintptr; kind kind }
// set via unsafe.Add(base, offset)
```

- Reflection-free: yes. Codegen: still needed to produce offsets. `unsafe`: yes.
- Costs: memory-safety escapes the type system; `go vet`/`-race` cannot protect
  field writes; GC pointer rules become the author's problem; violating the
  spirit of "clean, fault-tolerant code" (constitution).
- Verdict: **reject.** It buys ~5 ns/field over A5 and costs all static safety.

### A5 — Accessor-closure descriptors (recommended core)

```go
type Field[T any] struct {
    Name   string
    Decode func(*Reader, *T) error
}

func String[T any](name string, field func(*T) *string) Field[T]
```

- Reflection-free: yes. Codegen: no. `unsafe`: no. Type-safe: yes.
- The accessor `func(*T) *string` is a compile-time pointer to the field; the
  closure captures nothing, so it does not allocate.
- Cost: one indirect call per field assignment.
- Verdict: **recommended.** It is A2's safety with A1's ergonomics, and A1 can
  generate A5 descriptors later without changing the runtime.

### A6 — Token API only ("bring your own binding")

Expose `scanner`'s tokenizer and stop there.

- Reflection-free: trivially. Work for the user: all of it.
- Verdict: **ship it as a by-product**, not as the answer.

### A7 — Reflection fallback for `any`

Keep `Unmarshal(data, v any)` for dynamic callers and use A5 internally.

- Reflection-free: **no** — violates the task.
- Verdict: **reject**, but document the trade-off so the deviation is explicit.

### Comparison matrix

| | Reflect-free | Codegen | `unsafe` | Type-safe | Ergonomics | Perf |
|---|---|---|---|---|---|---|
| A1 codegen | ✅ | required | ❌ | ✅ | medium | ★★★★★ |
| A2 hand-written | ✅ | ❌ | ❌ | ✅ | low | ★★★★ |
| A3 generics + A2 | ✅ | ❌ | ❌ | ✅ | high | ★★★★ |
| A4 unsafe offsets | ✅ | required | ✅ | ❌ | medium | ★★★★★ |
| **A5 accessors** | ✅ | ❌ | ❌ | ✅ | **high** | ★★★★ |
| A6 tokens only | ✅ | ❌ | ❌ | ✅ | low | ★★★★★ |
| A7 reflect fallback | ❌ | ❌ | ❌ | ✅ | highest | ★★ |

---

## 6. Empirical findings

All measurements on Apple M1, go1.27.3, `-benchtime=300ms`. Experiments live
outside the repository in `/private/tmp/opencode/research/`; no repository code
was modified.

### 6.1 Generics inference — verified working

`/private/tmp/opencode/research/ginfer/main.go` compiles and runs:

```go
func Unmarshal[T any, PT interface {
    *T
    Decoder
}](data []byte) (T, error)
```

Calling `Unmarshal[Config](data)` type-checks: the compiler infers
`PT = *Config` by constraint type inference. Output:

```
inferred call ok: {Name:cfg Port:1} err=<nil>
```

This removes the usual objection to interface-based APIs — no `*Config` argument
and no `any` boxing at the call site.

### 6.2 Binding cost: reflection vs accessors

`/private/tmp/opencode/research/bind/bench_test.go`, 5-field struct, one bind
per iteration:

| Strategy | ns/op | B/op | allocs/op |
|---|---|---|---|
| `reflect.Value.FieldByName` per field | 191.4 | 56 | 2 |
| Reflection, cached field indices | 43.2 | 56 | 2 |
| **Accessor closures** | **13.9** | 32 | 1 |
| Direct field writes (codegen ceiling) | 8.4 | 32 | 1 |

(The single allocation in the last two rows is the `[]string{"a","b"}` literal
in the benchmark, not the binding.)

Reading: cached-index reflection costs **~29 ns more** than accessor closures for
five fields, i.e. ~5.9 ns per field of pure reflection overhead. Against a
87-byte document that the scanner validates in 346 ns, that is ~8% of total
decode time. Real, but not dominant.

### 6.3 The real bottleneck: per-byte dispatch

`/private/tmp/opencode/research/dispatch/bench_test.go`, 64 KiB string body:

| Strategy | Throughput | ns/byte |
|---|---|---|
| Per-byte function-pointer dispatch (current scanner model) | 482 MB/s | 2.07 |
| `bytes.IndexByte` bulk scan | 65.2 GB/s | 0.015 |

The 135× gap is a **ceiling for the string sub-task**, not an end-to-end
number: bulk scanning only applies to runs of plain bytes. The honest end-to-end
measurement is head-to-head against the baseline.

#### Head-to-head, same binary, same documents

`/private/tmp/opencode/compare/compare_test.go` imports both this repository's
scanner and `encoding/json`, and validates identical byte slices.

| Document | This scanner | Stdlib `json.Valid` | Gap |
|---|---|---|---|
| 87-byte object | 251.3 MB/s | 570.5 MB/s | 2.27× |
| 1 000-object array | 262.3 MB/s | 749.2 MB/s | 2.86× |
| 10 000 numbers | 265.6 MB/s | 692.1 MB/s | 2.61× |
| 64 KiB string | 286.9 MB/s | 1 465.8 MB/s | 5.11× |

**The scanner is 2.3–5.1× slower than the baseline it is supposed to beat.**
That is the single most important finding in this document.

#### Why: the baseline is no longer a byte-at-a-time state machine

A CPU profile of the stdlib run shows the work happening in
`encoding/json/internal/jsonwire.ConsumeStringResumable` and
`jsontext.(*decoderState).consumeString` — i.e. the `encoding/json/v2` path is
active by default on go1.27.3, and it **bulk-scans string bodies** rather than
stepping one byte at a time. Our profile shows the opposite: 48% of samples in
`Valid`'s dispatch loop and 42% in `inString`, one indirect call per byte.

So the comparison is not "same design, different tuning". The state of the art
moved to bulk scanning and we did not.

#### Two cheap fixes, measured

Applied to a disposable copy of the package
(`/private/tmp/opencode/scannerexp/`), with the original test suite passing:

1. Remove 11 redundant `m.handler = <same state>` self-assignments in hot
   handlers — a handler that stays in its own state does not need to rewrite the
   function pointer. **+3–9%.**
2. Remove the `machine.step` wrapper from `Valid`'s hot loop and call
   `m.handler(m, c)` directly. **+20–25%.**

| Document | Original | Both fixes | Gain |
|---|---|---|---|
| object | 251.3 MB/s | 313.5 MB/s | +24.7% |
| array of objects | 262.3 MB/s | 323.0 MB/s | +23.2% |
| numbers | 265.6 MB/s | 331.2 MB/s | +24.7% |
| string | 286.9 MB/s | 388.1 MB/s | +35.3% |

After both fixes the gap to the baseline narrows to **1.8×–3.8×**. The remaining
gap is architectural: bulk scanning versus per-byte dispatch.

Implication for this plan: a decoder built on the current per-byte machine will
inherit ~3.5 ns/byte. Reflection-free binding saves ~29 ns per object; the
scanner spends ~350 ns on the same object. **Optimising the scanner is the 10×
lever; optimising binding is a 1.08× lever.** The plan therefore includes an
explicit fast-path phase, and the research should not be sold as a speed win.

### 6.4 Current scanner baseline (for later comparison)

```
BenchmarkValidObject-8            346.2 ns/op   251.29 MB/s   0 B/op   0 allocs/op
BenchmarkValidArrayOfObjects-8    133.0 µs/op   262.28 MB/s   0 B/op   0 allocs/op
BenchmarkValidNumbers-8           372.4 µs/op   265.56 MB/s   0 B/op   0 allocs/op
BenchmarkValidString-8            249.9 µs/op   286.86 MB/s   0 B/op   0 allocs/op
```

These are the numbers to beat. Note that they are **not** competitive with the
stdlib on this toolchain, which contradicts the project's stated goal of being
"ideally faster".

---

## 7. Recommended architecture

### 7.1 Layering

```
scanner/                     syntax layer (existing, extended)
  machine.go                 state machine — unchanged semantics
  token.go        NEW        pull tokenizer: Kind, Peek, Read*, Skip, Offset
  valid.go                   Valid([]byte) bool — unchanged

marshal/                     semantic layer (new)
  reader.go       NEW        Reader: typed accessors over scanner's tokenizer
  schema.go       NEW        Schema[T], Field[T], Struct()
  field.go        NEW        String/Int/Float/Bool/Slice/Map/Object/Any/Raw helpers
  unmarshal.go    NEW        Unmarshal[T], UnmarshalInto[T], Decoder interface
  errors.go       NEW        Error, TypeError, path formatting
  doc.go                     updated package documentation
```

`marshal` imports `scanner`. No cycles. No third-party code.

### 7.2 Why the tokenizer lives in `scanner`

`marshal` is a separate package, so it can only use *exported* identifiers. There
are exactly two ways to share the state machine:

- **S1 (recommended):** `scanner` exports a tokenizer. The machine stays where it
  is; one engine; matches the `encoding/json/jsontext` precedent. Public surface
  grows by one type and a `Kind` enum.
- **S2 (alternative):** move the engine to `internal/json`, leaving `scanner` as
  a thin `Valid` facade. Keeps the public surface minimal, but contradicts the
  repository's stated identity ("`scanner/` is the state machine") and moves all
  contract tests.

S1 is recommended: least churn, one engine, and a public tokenizer is genuinely
useful for streaming documents too large to materialise.

### 7.3 Tokenizer design

The machine already produces opcodes but **not spans**. It cannot answer "where
did this string start and end?" — a string's closing quote returns
`opContinue`, indistinguishable from its interior. Decoding therefore needs span
tracking. Three ways:

1. **Add span fields to `machine`** (`tokenStart`, `tokenEnd`, `tokenKind`),
   set at the six completion points (`inString`, `trueRUE`, `falseALSE`,
   `nullULL`, `finishNumber`, `extendNumber`). No opcode changes, so no existing
   test changes. The subtle part is the number end: with a real delimiter the
   number ends at `consumed-1`; after `finish()`'s synthetic space it ends at
   `consumed`. This is exactly the invariant already documented in `AGENTS.md`.
2. **Watch handler transitions** from the tokenizer. Fragile, implicit coupling
   to state internals — reject.
3. **Write a second state machine for decoding.** Two sources of truth — reject
   unless the fast path in §9.3 makes it unavoidable.

Adopt (1). The cost is two stores per completed value, measured against the
`Valid` benchmarks to confirm no regression.

### 7.4 Public API sketch

```go
// scanner
type Kind uint8
const (
    KindNull Kind = iota
    KindBool
    KindString
    KindNumber
    KindObject
    KindArray
)

type Decoder struct { /* ... */ }
func NewDecoder(data []byte) *Decoder
func (d *Decoder) Peek() (Kind, error)
func (d *Decoder) ReadNull() error
func (d *Decoder) ReadBool() (bool, error)
func (d *Decoder) ReadString() ([]byte, error)   // raw, still escaped
func (d *Decoder) ReadNumber() ([]byte, error)   // raw literal
func (d *Decoder) ReadRaw() ([]byte, error)      // whole value, composites included
func (d *Decoder) BeginObject() error
func (d *Decoder) EndObject() error
func (d *Decoder) BeginArray() error
func (d *Decoder) EndArray() error
func (d *Decoder) Skip() error
func (d *Decoder) Offset() int64
func (d *Decoder) Err() error
```

```go
// marshal
type Decoder interface { DecodeJSON(*Reader) error }

func Unmarshal[T any, PT interface{ *T; Decoder }](data []byte) (T, error)
func UnmarshalInto[T any, PT interface{ *T; Decoder }](data []byte, v *T) error

type Schema[T any] struct { /* ... */ }
func Struct[T any](fields ...Field[T]) *Schema[T]
func (s *Schema[T]) Decode(r *Reader, v *T) error
func (s *Schema[T]) Unmarshal(data []byte) (T, error)
func (s *Schema[T]) UnmarshalInto(data []byte, v *T) error

type Field[T any] struct {
    Name   string
    Decode func(*Reader, *T) error
}

func String[T any](name string, field func(*T) *string) Field[T]
func Int[T any](name string, field func(*T) *int64) Field[T]
func Float[T any](name string, field func(*T) *float64) Field[T]
func Bool[T any](name string, field func(*T) *bool) Field[T]
func Slice[T, E any](name string, field func(*T) *[]E, elem func(*Reader) (E, error)) Field[T]
func Map[T, V any](name string, field func(*T) *map[string]V, elem func(*Reader) (V, error)) Field[T]
func Object[T, S any](name string, field func(*T) *S, schema *Schema[S]) Field[T]
func Any[T any](name string, field func(*T) *any) Field[T]
func Raw[T any](name string, field func(*T) *[]byte) Field[T]
```

Usage:

```go
type Config struct {
    Name  string
    Port  int64
    Tags  []string
    Debug bool
}

var configSchema = marshal.Struct(
    marshal.String[Config]("name", func(c *Config) *string { return &c.Name }),
    marshal.Int[Config]("port", func(c *Config) *int64 { return &c.Port }),
    marshal.Slice[Config]("tags", func(c *Config) *[]string { return &c.Tags },
        func(r *marshal.Reader) (string, error) { return r.ReadString() }),
    marshal.Bool[Config]("debug", func(c *Config) *bool { return &c.Debug }),
)

cfg, err := configSchema.Unmarshal(data)
```

If the type also implements `Decoder`, the shorter form works:

```go
cfg, err := marshal.Unmarshal[Config](data)
```

### 7.5 Reader design

`Reader` wraps `scanner.Decoder` and adds typed conversion plus JSON-path
tracking for errors. Object and array iteration use callbacks rather than
iterator objects, so the hot path allocates nothing:

```go
func (r *Reader) Object(fn func(key string) error) error
func (r *Reader) Array(fn func(index int) error) error
func (r *Reader) Skip() error
func (r *Reader) Path() string      // "$.items[2].name"
func (r *Reader) Offset() int64
```

`Schema.Decode` builds a name→`Field` lookup once (a small sorted slice, binary
search — no map allocation per document) and calls `r.Object`.

### 7.6 Error model

```go
type Error struct {
    Path   string // "$.items[2].name"
    Offset int64  // byte offset into the input
    Kind   ErrorKind
    Err    error
}
```

Kinds: syntax, type mismatch, unknown field (strict mode), duplicate field
(strict mode), depth limit. `errors.As` support; `Unwrap` to the underlying
`scanner` error. Offsets come from the tokenizer, so they are exact.

### 7.7 Semantics to pin down (and test)

- Unknown members: ignored by default, error in strict mode (v2 calls this
  `RejectUnknownMembers`).
- Duplicate members: last wins by default; error in strict mode.
- Case sensitivity: exact by default; opt-in case-insensitive.
- `null` into a non-pointer: leave zero / error? Recommend *leave zero* for
  scalars (matches v1's "null into non-pointer is a no-op") and document it.
- Number overflow: `int64` overflow is an error, not silent truncation.
- Strings: `\uXXXX` unescaping, surrogate pairs rejected or replaced — decide
  once and test. Note the scanner deliberately does not validate UTF-8, so the
  decoder must decide its own policy.
- Merge semantics: `UnmarshalInto` merges into the existing value; `Unmarshal`
  starts from zero. This mirrors v2's `Unmarshal` behaviour.

---

## 8. Phased implementation plan (TDD)

Each phase: failing tests first → minimal implementation → refactor → run the
mandated verification sequence. Do not start a phase before the previous one
passes `make verify`.

### Phase 0 — Governance (no code)

- Amend `docs/constitution.md` to v1.3.0:
  - Article VI: allow `scanner`'s tokenizer and `marshal`'s `Unmarshal` surface.
  - Article III: restate "no `encoding/json`" as applying to the whole module.
  - Additional constraints: document that `marshal` is *not* API-compatible with
    `encoding/json.Unmarshal` and why.
- Update `AGENTS.md` (layout, commands, new contract details).
- Update `marshal/doc.go`: drop the "API compatible with encoding/json" claim.
- **Acceptance**: documents reviewed; no code change; `make verify` still green.

### Phase 1 — Tokenizer spans in `scanner`

Tests first (`scanner/token_test.go`):
- `TestDecoderPeekKind` — kind for each JSON value type.
- `TestDecoderReadString` — raw bytes, escapes preserved, empty string.
- `TestDecoderReadNumber` — integer, fraction, exponent, negative.
- `TestDecoderReadRaw` — whole composite value byte-for-byte.
- `TestDecoderSkip` — nested composites, offset after skip.
- `TestDecoderOffset` — offsets match the machine's `consumed`.
- `TestDecoderErrors` — syntax error surfaces with the same message and position
  as `Valid`.
- `TestDecoderMatchesValid` — for every corpus entry in `TestValidDocuments`
  and `TestInvalidDocuments`, `Decoder` agrees with `Valid`.

Implementation:
- Add `tokenStart`, `tokenEnd`, `tokenKind` to `machine`; set at the six
  completion points. No opcode changes.
- Add `scanner/token.go` with `Kind`, `Decoder`, and the read methods.
- **Acceptance**: all tests pass; `Valid` benchmarks show no regression beyond
  noise; `make verify` green.

### Phase 2 — `marshal` primitives and errors

Tests first:
- `TestReadStringUnescapes` — `\n`, `\t`, `\"`, `\\`, `\/`, `\uXXXX`.
- `TestReadInt` / `TestReadFloat` — range, overflow, `-0`, exponents.
- `TestReadBool`, `TestReadNull`.
- `TestErrorPath` — `$.a.b[2].c` formatting; offset exactness.
- `TestErrorUnwrap` — `errors.As` reaches the scanner error.

Implementation: `marshal/reader.go`, `marshal/errors.go`.
- **Acceptance**: zero allocations on the primitive read path (assert with
  `testing.AllocsPerRun`); `make verify` green.

### Phase 3 — `Schema[T]` and struct binding

Tests first:
- `TestStructDecode` — flat struct, all field types.
- `TestStructUnknownFieldIgnored` / `TestStructUnknownFieldRejected`.
- `TestStructDuplicateField` — last wins; strict mode errors.
- `TestStructNested` — object inside object.
- `TestStructNullField` — null into scalar and into pointer.
- `TestStructNoAllocs` — steady-state decode allocates only for the result.
- `TestStructFieldLookup` — binary search correctness with 1, 2, 7, 64 fields.

Implementation: `marshal/schema.go`, `marshal/field.go`.
- **Acceptance**: `make verify` green; decode benchmark recorded.

### Phase 4 — Generic entry point and interface

Tests first:
- `TestUnmarshalGeneric` — `Unmarshal[Config](data)` returns the value.
- `TestUnmarshalInto` — merge semantics on a pre-populated value.
- `TestUnmarshalInterface` — `Decoder` interface dispatch.
- `TestUnmarshalInvalidTarget` — nil pointer, wrong type.

Implementation: `marshal/unmarshal.go`.
- **Acceptance**: `make verify` green; `go doc ./marshal` shows the intended
  surface only.

### Phase 5 — Containers

Tests first:
- `TestSliceOfPrimitives`, `TestSliceOfStructs`, `TestEmptySlice`.
- `TestMapOfStrings`, `TestMapOfStructs`, `TestEmptyMap`.
- `TestArrayFixedSize` — fixed-length arrays, overflow and underflow.
- `TestAny` — `any` decoding to `map[string]any`, `[]any`, `string`, `float64`,
  `bool`, `nil`; verify no reflection is used.
- `TestRaw` — `[]byte` receives the exact literal bytes.

Implementation: extend `field.go`.
- **Acceptance**: `make verify` green.

### Phase 6 — Fuzz and differential testing

- `FuzzUnmarshal` — random input must either decode or return an error; never
  panic; error offsets within `[0, len(data)]`.
- `FuzzUnmarshalMatchesValid` — if `Unmarshal` succeeds, `Valid` must agree.
- `FuzzSchemaRoundTrip` — decode → re-encode with the future `Marshal` → decode
  must be stable (deferred until encoding exists).
- **Acceptance**: 10s fuzz run clean; `make verify` green.

### Phase 7 — Performance: bulk fast paths

This is where the real speed lives (§6.3).

- Add a string fast path: `bytes.IndexByte` to find the closing quote, then scan
  only for `\` and control bytes; fall back to the machine on escapes.
- Add a number fast path: digit-run loop, then delegate the delimiter to the
  machine.
- Add a whitespace fast path: skip runs of spaces/tabs/newlines.
- Keep the machine as the reference implementation; add a differential test that
  every fast-path input also passes the machine.
- **Acceptance**: `Valid` and `Unmarshal` benchmarks ≥ 1 GB/s on string-heavy
  documents; `0 B/op`, `0 allocs/op`; all differential tests pass; `make verify`
  green.

### Phase 8 — Optional codegen

- A `cmd/qi-json-gen` tool (stdlib only: `go/ast`, `go/parser`, `go/types`,
  `text/template`) that emits `Schema` descriptors or `DecodeJSON` methods from
  struct tags.
- **Acceptance**: generated code passes the same tests; `make verify` green.

### Phase 9 — Documentation

- `marshal/doc.go` with examples; `README`-level usage; `AGENTS.md` contract
  notes; plan document updated with verification results.

---

## 9. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Span tracking perturbs the `Valid` hot path | Slower validator | Measure each change; keep stores out of per-byte handlers; revert if regression > 2%. |
| Two sources of truth (machine vs fast path) | Silent divergence | Differential fuzz; keep the machine as the reference and never delete its tests. |
| `Unmarshal` API incompatibility surprises users | Broken expectations | Document explicitly in `doc.go` and the constitution; provide `UnmarshalInto` for merge semantics. |
| Accessor closures per field add an indirect call | Slight slowdown | Measure; if hot, codegen (Phase 8) removes the call. |
| `any` decoding requires type assertions | Panic risk | Table-driven tests for every JSON kind; return typed errors. |
| No UTF-8 validation in the scanner | Invalid UTF-8 accepted | Decide decoder policy explicitly (reject or replace) and test it. |
| Constitution says "one exported identifier" | Governance violation | Phase 0 amendment before any export. |

---

## 10. Open questions for the user

1. **API shape**: `Unmarshal[T](data) (T, error)` (copy, no pointer argument) or
   `UnmarshalInto[T](data, *T) error` (merge, no copy) — or both? Recommendation:
   both, with `Unmarshal` as the headline.
2. **Tokenizer exposure**: accept `scanner` exporting a tokenizer (S1), or move
   the engine to `internal/json` (S2)?
3. **Codegen**: is Phase 8 in scope now, or deferred until the hand-written path
   is proven?
4. **Strictness defaults**: unknown members ignored or rejected by default?
   Recommendation: ignored (v1-compatible), strict opt-in.
5. **UTF-8 policy**: reject invalid UTF-8 in strings, or pass through as the
   scanner does? Recommendation: reject in the decoder, keep the scanner
   syntax-only.
6. **Performance target**: is the Phase 7 fast path required now, or is
   correctness-first acceptable with the scanner as-is?

---

## 11. Verification

This research phase changed no repository code. To confirm the repository is
still in its verified state:

```
go test ./...
gofmt -l .
go vet ./...
golangci-lint run
```

Experiments (outside the repository, disposable):

```
/private/tmp/opencode/research/ginfer    — generics inference check
/private/tmp/opencode/research/bind      — reflection vs accessor binding
/private/tmp/opencode/research/dispatch  — per-byte dispatch vs bulk scan
```

---

## 12. Summary of the recommendation

Build `marshal` as a **descriptor-driven, reflection-free decoder** on top of a
**span-aware tokenizer** in `scanner`:

- **Binding**: accessor closures (`Field[T]`), no reflection, no `unsafe`.
- **Entry point**: `Unmarshal[T]` with the pointer-receiver constraint trick,
  verified to infer on go1.27.3.
- **Layering**: mirrors `encoding/json/v2` + `jsontext`, which is the stdlib's
  own answer to the same problem.
- **Honesty**: the API cannot be `encoding/json`-compatible; that must be an
  amendment, not a silent deviation.
- **Performance**: reflection-free binding is an allocation and safety win, not
  a speed win. The 10× lever is the scanner's per-byte dispatch, which deserves
  its own phase.
