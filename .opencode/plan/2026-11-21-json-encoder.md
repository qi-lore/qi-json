# qi-json Encoder — plan and decisions

**Date**: 2026-11-21
**Status**: **done** — all §8 decisions signed off and the amendment approved
(2026-11-21); implemented as T-E1…T-E7 with `make verify` and `-race` green and a
round-trip fuzz target passing. See §9. **Locked decisions**:
D3.5 **Option A** (`Codec[E]`, breaking the container constructors); D3.6
**replace invalid UTF-8 with U+FFFD**; D3.4 **declaration order**; D3.7
**support `omitempty` in v1** (an exception to the "non-goals" note — the user
chose to support it now); D3.8 **plain errors**; amendment **approved as
proposed**.
**Related**: `docs/constitution.md`, `AGENTS.md`,
`.opencode/plan/2026-11-21-post-decoder-backlog.md` (§6),
`.opencode/plan/2026-11-21-json-decoder.md`,
`.opencode/plan/2026-11-21-json-codegen.md`, `README.md`.

---

## 1. Goal

Add an **encoder** to `qi-json`: turn Go values into JSON text, reflection-free,
so the library covers both directions under the decoder's constraints. The
encoder is the mirror image of the decoder:

- `marshal` — gains an `Encoder` interface, a `Writer`, and encode methods on
  `Schema`/`Field` (string escaping, number formatting, container framing).
- `cmd/qi-json-gen` — emits `EncodeJSON` alongside `DecodeJSON`, so a tagged
  struct gets both methods from one `//go:generate` line.
- `scanner` — **unchanged**. Encoding needs no validator or tokenizer change.

## 2. Non-goals (v1)

- No `encoding/json` API compatibility (same deliberate deviation as the
  decoder), and no import of it anywhere.
- No reflection, no `unsafe` (constitution Article III/VI).
- No `io.Writer` streaming: encode into an in-memory `[]byte` buffer. An `io`
  adapter is a later, additive change.
- No pretty-printing: compact output only. An `Indent` option is later.
- `omitempty` **is** honoured in v1 (decided 2026-11-21; see D3.7).

## 3. Decisions (proposed)

### D3.1 — Reflection-free, symmetric with the decoder

Encoding arbitrary Go types without reflection needs a compile-time mapping,
exactly as decoding does. So the encoder is **schema/closure based**, mirroring
the decoder:

```go
type Encoder interface { EncodeJSON(*Writer) error }
```

A type implements it by handing its `Schema` to a writer, or by writing fields
imperatively. `cmd/qi-json-gen` generates the method from struct tags, exactly
as it already generates `DecodeJSON`.

### D3.2 — `Writer`

```go
type Writer struct { /* buf []byte; stack []frame; err error */ }
func NewWriter() *Writer
func (w *Writer) Reset()
func (w *Writer) Bytes() []byte            // valid JSON; nil after an error
func (w *Writer) Err() error               // sticky, first failure wins
func (w *Writer) WriteNull() error
func (w *Writer) WriteBool(v bool) error
func (w *Writer) WriteString(v string) error
func (w *Writer) WriteInt(v int64) error
func (w *Writer) WriteUint(v uint64) error
func (w *Writer) WriteFloat(v float64) error
func (w *Writer) WriteRaw(v []byte) error  // verbatim, trusted JSON
func (w *Writer) BeginObject() error
func (w *Writer) EndObject() error
func (w *Writer) BeginArray() error
func (w *Writer) EndArray() error
func (w *Writer) Key(name string) error    // writes "name": inside an object
```

- **Framing.** A small `frame` stack tracks, per open container, whether it is an
  object and how many members/elements it holds. A `needComma` flag on the writer
  makes value writers emit a leading `,` when required; `Key` emits the separator
  plus `"name":` and clears the flag for the value that follows. `BeginObject`
  and `BeginArray` push a frame and clear the flag; `End*` pop and set it. The
  top level has no container, so no leading comma.
- **Misuse is an error, not a panic.** `Key` outside an object, `EndObject` on an
  array, a value where a key is expected, or an unbalanced end sets the sticky
  error. (A `panic` is also defensible; the sticky error keeps the API total and
  mirrors the decoder's error-returning style.)
- **Sticky error.** Once `err` is set, every method is a no-op returning `err`;
  `Bytes` returns `nil`. `Marshal` returns `(nil, err)`.
- **Zero value.** The zero `Writer` is usable (like `bytes.Buffer`), so
  `var w marshal.Writer` works without `NewWriter`. `NewWriter` is sugar.

### D3.3 — Top-level API (mirrors `Unmarshal`/`Decode`)

```go
func Marshal[T any, PT interface{ *T; Encoder }](v T) ([]byte, error)
func Encode(w *Writer, v Encoder) error
```

- `Marshal` creates a writer, calls `PT(&v).EncodeJSON(w)`, and returns
  `w.Bytes(), w.Err()`. It takes `v T` so a value works; the `PT` constraint
  makes the compiler select the encoder, exactly like `Unmarshal`.
- `Encode` is the non-generic counterpart for a caller that already holds an
  `Encoder` or wants to reuse a pre-grown `Writer`:
  `Encode(w, &cfg)`.
- A nil `Encoder` interface is reported as `errNilTarget` (reusing the decoder's
  sentinel), matching `Decode`.

### D3.4 — `Field`/`Schema` encode

- `Field[T]` gains an `Encode func(*Writer, *T) error` field. The scalar
  constructors (`String`, `Int`, `Float`, `Bool` and the `*Ptr` variants) set
  both `Decode` and `Encode`, so existing scalar call sites get encoding for
  free.
- `Schema[T]` gains:

  ```go
  func (s *Schema[T]) Encode(w *Writer, v *T) error
  func (s *Schema[T]) Marshal(v T) ([]byte, error)
  ```

  `Encode` writes `{`, each field as `Key(name)` + `field.Encode`, then `}`.
- **Field order on encode.** `Struct` currently sorts `fields` by name for
  binary-search lookup. JSON object member order is not significant, but output
  should be stable and natural. Proposal: keep a second slice holding the
  fields in **declaration order** and encode in that order (like
  `encoding/json`); the sorted slice stays for decode lookup. If declaration
  order is undesirable, encoding in sorted order is a one-line change and is
  also deterministic.

### D3.5 — Element encoders for containers (**breaking, needs sign-off**)

`Object[T,S](name, field, schema)` is fine: the `Schema[S]` already carries both
directions. `Slice`/`Map`/`Array` are not: their last parameter is a bare
`func(*Reader) (E, error)` (decode only). Encoding an element needs its
counterpart `func(*Writer, E) error`. Three options:

- **Option A (CHOSEN, 2026-11-21) — a `Codec[E]` value carrying both
  directions:**

  ```go
  type Codec[E any] struct {
      Decode func(*Reader) (E, error)
      Encode func(*Writer, E) error
  }
  func NewCodec[E any](dec func(*Reader) (E, error), enc func(*Writer, E) error) Codec[E]

  func Slice[T, E any](name string, field func(*T) *[]E, elem Codec[E]) Field[T]
  func Map[T, V any](name string, field func(*T) *map[string]V, elem Codec[V]) Field[T]
  func Array[T, E any](name string, field func(*T) []E, elem Codec[E]) Field[T]
  ```

  This is the coherent shape (one value, both directions, composable for nested
  containers) but it is a **breaking change** to the three container
  constructors and to every existing call site (the generator's `Slice`/`Map`/
  `Array` emissions and the decoder tests). Pre-1.0 and explicitly
  non-`encoding/json`-compatible, so acceptable if agreed.

- **Option B (additive) — keep the constructors and add a fluent setter:**
  `Field[T].WithEncode(func(*Writer, *T) error)`. The generator emits the whole
  encode closure (using element-encode helpers it also emits), so generated code
  is fully covered; hand-written nested containers must supply their own encode
  closure. No existing call site changes. Less elegant for hand-written code.

- **Option C (additive, parallel constructors):** `SliceEnc`/`MapEnc`/`ArrayEnc`
  taking an extra encode function. Purely additive but leaves two near-identical
  families to learn.

**Decision: Option A (chosen).** It is the smallest *conceptual* surface and
keeps one codec per element type; the breakage is mechanical and confined to
`field.go`, the generator, and decoder tests.

### D3.6 — Scalars: numbers, strings, `any`, raw

- **Integers.** `WriteInt` writes via `strconv.AppendInt`; `WriteUint` via
  `strconv.AppendUint`. Generated code range-checks exactly as it does for
  decode, then widens to `int64`/`uint64`.
- **Floats.** `WriteFloat` uses `strconv.AppendFloat(buf, v, 'g', -1, 64)` (the
  shortest round-tripping form). `NaN` and `±Inf` are an error (JSON has no
  representation). `'g'` output such as `1e+06` is valid JSON.
- **Strings.** Escape `"`, `\`, and control bytes `< 0x20` (using `\b \f \n \r
  \t` where available, else `\u00XX`). **Do not** HTML-escape `<`, `>`, `&`
  (a deliberate, documented deviation from `encoding/json`'s default: smaller
  output, and this library is not for HTML embedding). Valid multi-byte UTF-8 is
  emitted as-is.
- **Invalid UTF-8 in a Go string.** A Go `string` may hold arbitrary bytes.
  **Decision:** replace each invalid byte with `U+FFFD` (the `encoding/json`
  behaviour), so encoding is total and never fails on a string.
- **`any`.** `Any` encodes the dynamic value: `nil`, `bool`, `string`,
  `float64`/`float32`, every `int`/`uint` width, `[]any`, `map[string]any`, and
  a nested `Encoder`. Map keys are sorted for determinism. Any other dynamic
  type is an error naming the type.
- **`Raw`.** Writes the stored bytes verbatim via `WriteRaw`; they were produced
  by decoding, so they are trusted. (Validation on encode is a possible later
  hardening.)

### D3.7 — `omitempty` (supported in v1)

The decoder ignores `omitempty` (it is an encode concern; a decoded document is
unaffected). The encoder **honours** `json:",omitempty"`: a field whose value is
"empty" is omitted from the object. `Field[T]` gains an exported
`Empty func(*T) bool` (nil = never empty) set by the constructors, plus an
unexported `omitEmpty` flag set by the exported method
`func (f Field[T]) OmitEmpty() Field[T]`; the generator emits
`... .OmitEmpty()` for tagged fields. Emptiness per kind: value scalars zero,
pointer scalars nil, slices/maps `len == 0`, `any` nil, `Raw` `len == 0`; a
nested struct and a fixed-size array are never empty (matching `encoding/json`,
where structs and arrays are not omitted).

### D3.8 — Encode errors

Encode failures are programming errors (misusing `Writer`, an unsupported
dynamic `any`, `NaN`/`Inf`), not properties of input text. **Decision:** use
plain `errors.New`/`fmt.Errorf` values, **not** the position-oriented
`*marshal.Error` (which is a decode diagnostic). This adds **no** new exported
error type, keeping the amendment small.

### D3.9 — Codegen

- Emit `func (v *T) EncodeJSON(w *marshal.Writer) error { return <schema>.Encode(w, v) }`
  next to each generated `DecodeJSON`.
- Every generated `Field` carries both `Decode` and `Encode`. Scalars use the
  `marshal.String`/`Int`/... constructors (which now set both); named/array/map
  fields use a `Field` literal with both closures, and emit a paired element
  helper (`qiDecodeX` and `qiEncodeX`) for slices and maps.
- Generation stays deterministic (sorted collections) so the drift guard passes.
- `-strict` is decode-only and does not change `EncodeJSON`.
- Cross-package named scalars (T2) encode through the same qualified type.

## 4. Constitution amendment (proposed)

Article VI gains a fourth sanctioned surface:

> **`marshal` encoder** — the reflection-free encode layer: the `Encoder`
> interface (`EncodeJSON(*Writer) error`), `Writer`, the generic `Marshal` and
> non-generic `Encode`, `Schema.Encode`/`Schema.Marshal`, `Field.Encode`, and the
> element `Codec` (if D3.5 Option A is adopted). `marshal` MUST NOT import
> `encoding/json` and MUST NOT use `reflect` or `unsafe`, in the encoder as in
> the decoder.

Also amend "Additional Constraints": `marshal/` becomes "the reflection-free
semantic layer (decode and encode)". Bump **Version** to 3.2.0 and update
`AGENTS.md` in the same change (governance rule).

## 5. Task breakdown (TDD; failing test first)

1. **T-E1 — `Writer` framing + scalars.** Red: tests for `WriteNull`/`WriteBool`/
   `WriteString` (escaping, invalid UTF-8 policy)/`WriteInt`/`WriteUint`/
   `WriteFloat` (incl. `NaN`/`Inf` errors)/object & array framing/`Key` misuse.
   Green: `writer.go`.
2. **T-E2 — `Encoder` + top-level API.** `Encoder` interface; `Marshal[T,PT]`;
   `Encode(w, v)`; nil-encoder handling. `encode.go`.
3. **T-E3 — `Field`/`Schema` encode.** Add `Field.Encode`; scalar constructors
   set it; `Schema.Encode`/`Marshal`; field order (D3.4); round-trip tests
   (encode then `UnmarshalInto` equals the original) for every field family.
4. **T-E4 — element codecs.** Adopt D3.5's chosen option; wire `Slice`/`Map`/
   `Array`; update decoder tests and generator call sites.
5. **T-E5 — `Any`/`Raw` encode.** Dynamic value encoding (sorted map keys) and
   verbatim raw.
6. **T-E6 — codegen.** Emit `EncodeJSON` + element-encode helpers; extend the
   `gentest` fixture with a round-trip test; regenerate; keep the drift guard
   green.
7. **T-E7 — docs + amendment.** `marshal/doc.go`, README (new "Encoding"
   section + API table + performance row), CHANGELOG, constitution + AGENTS.md,
   and this plan's status.

Each task ends with the mandated verification (`go test ./...` → `gofmt -l .` →
`go vet ./...` → `golangci-lint run`; i.e. `make verify`), plus `go test -race
./...`. Fuzz: extend `FuzzUnmarshal`-style coverage with an encode round-trip
invariant (`Unmarshal(Marshal(v)) == v`) where meaningful.

## 6. Verification

- `make verify` green (0 lint issues) and `go test -race ./...` green after every
  task.
- Round-trip property: for the fixture types, `Marshal` then `Unmarshal` yields an
  equal value; for arbitrary decoded documents, re-encoding then re-decoding is
  stable.
- The scanner and its benchmarks are untouched.

## 7. Risks

- **D3.5 breakage** touches the decoder's public container constructors; it must
  land atomically with the generator and test updates.
- **Float formatting** differences from `encoding/json` are intentional but must
  be documented and tested (e.g. `1e+06`).
- **Invalid-UTF-8 policy** (D3.6) must be decided before T-E1.
- **Amendment scope**: adding `Codec` vs. keeping it unexported affects Article
  VI wording.

## 8. Sign-off items (decided 2026-11-21)

1. **D3.5** — element encoder: **Option A** (`Codec[E]`, breaking change to
   `Slice`/`Map`/`Array`).
2. **D3.6** — invalid UTF-8 on encode: **replace with `U+FFFD`**.
3. **D3.4** — encode field order: **declaration order**.
4. **D3.7** — `omitempty`: **supported in v1**.
5. **D3.8** — encode errors: **plain errors**.
6. **Amendment** — **approved as proposed** (Article VI fourth surface, version
   3.2.0, `AGENTS.md` updated in the same change).

---

## 9. Results (done, 2026-11-21)

**Shipped**

- `marshal/writer.go` — `Writer` (zero value usable, `NewWriter`/`Reset`/`Bytes`/
  `Err`), `WriteNull`/`WriteBool`/`WriteString`/`WriteInt`/`WriteUint`/
  `WriteFloat`/`WriteRaw`, `Begin*`/`End*`, `Key`, comma/framing state machine, a
  sticky error, and `appendQuoted` (JSON escaping; invalid UTF-8 → `U+FFFD`; no
  HTML escaping).
- `marshal/encode.go` — the `Encoder` interface, `Marshal[T, PT]`, `Encode`, the
  `Codec[E]` type and `NewCodec`.
- `marshal/field.go` — `Field.Encode`/`Field.Empty`/`Field.OmitEmpty`,
  `Slice`/`Map`/`Array` now take `Codec[E]`, `encodeAny`, and per-constructor
  encoders and empty tests.
- `marshal/schema.go` — a declaration-order slice, `Schema.Encode` and
  `Schema.Marshal`, and omitempty handling.
- `cmd/qi-json-gen` — every target now gets `EncodeJSON`; the generator emits
  paired `qiDecode*`/`qiEncode*` element helpers and `marshal.NewCodec(...)`, adds
  `Encode`/`Empty` to the numeric and cross-package field literals, and parses
  `json:",omitempty"`. Cross-package element types get paired encode helpers.
- Docs: `marshal/doc.go`, `README.md` (new *Encoding* section, entry-point table,
  field-constructor note, codegen section, performance row), `CHANGELOG.md`, the
  constitution (3.2.0) and `AGENTS.md`.

**Tests**

- `marshal/writer_test.go`, `marshal/encode_test.go`,
  `marshal/encode_fields_test.go` (round trip, declaration order, omitempty, nil
  and empty containers, `any` cases), an updated `marshal/field_test.go`, and
  `FuzzMarshalRoundTrip` (decode → encode → decode must be stable).
- `cmd/qi-json-gen/gen_test.go` gains `TestGenerateOmitEmpty` and updated
  container expectations; the `gentest` fixture gains an `omitempty` field and
  `TestGeneratedEncodeConfig`.

**Verification**: `make verify` green (0 lint issues); `go test -race ./...`
green; `FuzzMarshalRoundTrip` ran clean for 12 s (~800k execs). Benchmarks (M1,
go1.27.3): `BenchmarkStructEncode` 224 ns/op, 264 B/op, 7 allocs/op (decode of the
same shape: 447 ns/op).

**Note**: the container-constructor signature change (`func(*Reader)(E,error)` →
`Codec[E]`) is the one deliberate breaking change; the decoder tests and the
checked-in fixture were updated in the same change.

---

## 10. Independent review follow-up (2026-11-21)

A read-only reviewer (`general`) audited the encoder and found no hard-constraint
violations (no `encoding/json`/`reflect`/`unsafe`; `scanner/` untouched), correct
writer framing/escaping/floats, deterministic codegen, and sound round trips. The
following were fixed in the same change:

- **Nil element encoder panicked.** `Slice`/`Map`/`Array` encode now return
  `marshal: nil element encoder` instead of calling a nil `Codec.Encode`, matching
  the decode side's nil-`Decode` guard. A nil slice or map still encodes to
  `null` without needing an encoder.
- **Empty `Raw` emitted invalid JSON.** A nil *or empty* `[]byte` Raw field now
  encodes to `null` (previously an empty non-nil slice wrote nothing, producing
  `{"b":}` with a nil error).
- **`Marshal`/`Schema.Marshal` accepted an incomplete value.** A new
  `Writer.complete()` plus `errIncomplete` make both reject an `Encoder` that
  returned nil without writing exactly one whole JSON value.
- **`Encode` ignored a sticky writer error.** It now returns `w.err` after the
  call, like `Marshal`.
- **Codegen missed a pre-existing `EncodeJSON`.** `collect`/`selectTargets` now
  reject a type that already declares `EncodeJSON`, mirroring the `DecodeJSON`
  guard (previously it emitted a duplicate method).
- **Doc/typed-nil.** `Encode`'s doc now states that a typed-nil Encoder is not
  detected (needs reflection), and the encoder bullet in Article VI names
  `NewWriter`/`NewCodec`.

Not changed (documented instead): the README's pre-existing over-claim that
same-package named `string`/`bool` types are generated was narrowed to *named
types whose underlying type is a numeric scalar*, which is what `resolveBasic`
supports (recorded as a candidate in the codegen plan §8).

New tests: `marshal/encode_guard_test.go`
(`TestEncodeNilElementEncoder`, `TestEncodeEmptyRaw`, `TestMarshalIncomplete`,
`TestEncodeSurfacesWriterError`) and a `gen_test.go` case for a duplicate
`EncodeJSON`. `make verify`, `-race`, and `FuzzMarshalRoundTrip` are green again.
