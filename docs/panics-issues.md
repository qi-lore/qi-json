# Panics and robustness issues

A code-level audit of the `qi-json` engine, looking for defects that the
behavioral note in `docs/fn-comparison.md` does not cover: unrecoverable
failures (panics), guards that can be defeated, and silent misbehavior.

This is an assessment record. The companion note `docs/fn-comparison.md`
documents the semantic differences between the generic and non-generic entry
points and lists the *documented* footguns; this file goes behind those
descriptions to the code and records what is an actual defect, what is a
deliberate (documented) trade-off, and what merely looks risky but is correct.

- Reviewed at: `12e5c06` (v0.4.1), on branch `chore/fn-comparison-doc`.
- Method: full read of `internal/codec/*.go`, `internal/scanner/*.go` and
  `qijson.go`; `go test ./...`, `go test -race ./...`, `gofmt -l .`;
  `FuzzValid` (~3.4M execs) and `FuzzMarshalRoundTrip` (~0.7M execs); and a
  temporary external program importing `github.com/qi-lore/qi-json` to
  reproduce each candidate defect. The probe program was deleted after the run;
  its output is quoted verbatim below.

## Summary

| # | Issue | Class | Severity | Status |
|---|---|---|---|---|
| 1 | `Schema.UnmarshalInto` / `Schema.Decode` panic on a nil target | Nil deref (panic) | High | Resolved (Q006) |
| 2 | `Marshal` completeness guard defeated by `WriteRaw` | Defeated invariant | Medium | Resolved (Q007) |
| 3 | `Object` / nested `Schema` panic on a nil schema | Nil deref (panic) | Medium | Resolved (Q008) |
| 4 | `Struct` does not enforce unique field names | Silent misconfiguration | Medium | Unresolved |
| 5 | Typed-nil receiver panics in `Decode` / `Encode` | Nil deref (panic) | Medium | Documented, unresolved |
| 6 | Under-consuming `DecodeJSON` makes `Decode` accept invalid JSON | Defeated invariant | Medium | Documented, unresolved |
| 7 | Value-receiver `DecodeJSON` silently no-ops | Silent misbehavior | Low | Documented, unresolved |
| 8 | `Encode` does not enforce completeness | Missing guard | Low | Documented, unresolved |
| 9 | `errIncomplete` is unexported, so `errors.Is` cannot match it | API gap | Low | Documented, unresolved |

Issues 1–4 are new defects not recorded in `docs/fn-comparison.md`. Issues 5–9
are the documented trade-offs that the same note already lists; they are
restated here, confirmed, so the record is self-contained. The audit itself
changed no code; fixes land as tasks (see `docs/tasks.md`) and are reflected in
the Summary table and the Resolution indicator, which track state after the
review.

---

## Issue 1 — `Schema.UnmarshalInto` / `Schema.Decode` panic on a nil target

**Class:** unrecoverable failure (nil-pointer dereference).
**Severity:** high — a public entry point panics where every sibling returns a
sentinel.
**Location:** `internal/codec/schema.go`.

`Schema.UnmarshalInto` does not check its `v *T` argument before decoding into
it:

```go
// internal/codec/schema.go, Schema.UnmarshalInto
func (s *Schema[T]) UnmarshalInto(data []byte, v *T) error {
	r := newReader(scanner.NewTokenizer(data))
	if err := s.Decode(r, v); err != nil {   // v may be nil
		return err
	}
	if _, err := r.Peek(); err != nil {
		return err
	}
	return nil
}
```

`Decode` is the same (`internal/codec/schema.go`):

```go
func (s *Schema[T]) Decode(r *Reader, v *T) error {
	...
	return r.objectRaw(func(raw []byte) error {
		idx, f := s.lookup(raw)
		...
		err := f.Decode(r, v)   // v flows into the field closure
		...
	})
}
```

Every field constructor writes through the accessor closure, for example
`internal/codec/field.go`:

```go
func scalar[T, E any](name string, set func(*T) *E, ...) Field[T] {
	return Field[T]{
		Decode: func(r *Reader, v *T) error {
			...
			*set(v) = e   // set(v) is &v.Field; v nil => write to a nil base
			return nil
		},
		...
	}
}
```

With `v == nil`, `set(v)` evaluates `&v.Field` on a nil `*T`, and the
subsequent assignment dereferences it.

**Reproduction (external probe importing the public facade):**

```text
Schema.UnmarshalInto(data, nil)               PANIC: runtime error: invalid memory address or nil pointer dereference
Decode(data, (*cfg)(nil))                     PANIC: runtime error: invalid memory address or nil pointer dereference
```

The first lines are `cfgSchema.UnmarshalInto([]byte(`{"name":"x"}`), nil)` and
`qijson.Decode(..., (*cfg)(nil))`.

**Why this is a defect rather than a documented precondition.** The method's
doc says *"which must be non-nil"*, but the rest of the library treats a nil
target as a recoverable misuse and reports the `errNilTarget` sentinel:

- `codec.UnmarshalInto` (`internal/codec/decode.go`) checks `if v == nil` and
  returns `errNilTarget`.
- `Decode` (`internal/codec/decode.go`) checks `if v == nil` and returns
  `errNilTarget` for a nil interface.
- `Encode` (`internal/codec/encode.go`) checks both the writer and the encoder
  and returns `errNilWriter` / `errNilTarget`.

So the same misuse produces a sentinel through three entry points and a crash
through two. This violates the stated rule "no panics in library code for
recoverable conditions" (`AGENTS.md`, Code quality) and the intent already set
by the generic sibling.

**Suggested fix.** Add the same guard at the top of `Schema.UnmarshalInto` and
`Schema.Decode`:

```go
if v == nil {
	return errNilTarget
}
```

`errNilTarget` already lives in `internal/codec/decode.go` and is reachable
from `schema.go` (same package). A test can pin it alongside
`TestUnmarshalInvalidTarget`'s "nil pointer" subtest, and the doc comment should
drop the bare "must be non-nil" precondition in favor of the sentinel contract.

**Resolution (Q006).** `Schema.Decode` and `Schema.UnmarshalInto` now guard
`v == nil` and return `errNilTarget`; both doc comments state the sentinel
contract instead of the "must be non-nil" precondition. `TestSchemaNilTarget`
pins the behavior for both methods. `Schema.Encode` was left unchanged: it is
not part of this issue, and it is reached through `Marshal`, which always passes
a non-nil target.

---

## Issue 2 — `Marshal` completeness guard defeated by `WriteRaw`

**Class:** defeated invariant.
**Severity:** medium — `Marshal` can return `(nil, nil)` for a non-value, or
hand back invalid JSON, while claiming "exactly one complete JSON value".
**Location:** `internal/codec/writer.go` (`Writer.complete`, `Writer.WriteRaw`),
`internal/codec/encode.go` (`Marshal`).

`Marshal` is documented as the entry point that enforces one complete top-level
value, and it is the only place that does so:

```go
// internal/codec/encode.go, Marshal
func Marshal[T any, PT ...](v T) ([]byte, error) {
	var w Writer
	if err := PT(&v).EncodeJSON(&w); err != nil { return nil, err }
	if w.err != nil { return nil, w.err }
	if !w.complete() { return nil, errIncomplete }
	return w.buf, nil
}
```

`complete` only inspects the writer's frame state, not the bytes it produced:

```go
// internal/codec/writer.go
func (w *Writer) complete() bool {
	return w.err == nil && w.wroteRoot && len(w.stack) == 0
}
```

`wroteRoot` is set by `endValue`, which runs for *every* value write including a
raw write that appends nothing:

```go
// internal/codec/writer.go
func (w *Writer) endValue() {
	if len(w.stack) == 0 {
		w.wroteRoot = true
		return
	}
	w.stack[len(w.stack)-1].count++
}

func (w *Writer) WriteRaw(v []byte) error {
	if err := w.beginValue(); err != nil { return err }
	w.buf = append(w.buf, v...)   // may append zero bytes
	w.endValue()                  // depth 0 => wroteRoot = true
	return nil
}
```

So an encoder that writes `WriteRaw(nil)` (or whitespace, or arbitrary invalid
bytes) marks the document complete without writing a value.

**Reproduction (external probe):**

```text
=== Marshal completeness defeated by WriteRaw ===
    -> data="" (len=0) err=<nil>
Marshal[rawNilEnc] (WriteRaw(nil))            err=<nil>
    -> data="1 2" valid=false err=<nil>
Marshal[rawBadEnc] (WriteRaw("1 2"))          err=<nil>
```

`Marshal[rawNilEnc]` returns an empty `[]byte` and a nil error; `Marshal[rawBadEnc]`
returns the invalid text `1 2` and a nil error. In both cases the completeness
guard passed because `wroteRoot` was set by the empty raw write.

**Scope.** The library's own `Raw` field avoids the empty case — it writes
`null` for an empty or nil slice (`internal/codec/field.go`):

```go
Encode: func(w *Writer, v *T) error {
	b := *field(v)
	if len(b) == 0 {
		return w.WriteNull()
	}
	return w.WriteRaw(b)
},
```

So only a hand-written `Encoder` or `Field.Encode` that uses the documented
`WriteRaw` escape hatch is affected. But that is exactly the path the guard
exists to protect, and `WriteRaw` is documented as "without validation", which
invites tokens like `nil`.

**Suggested fix (pick one, needs a decision).**

1. Make `complete` require a produced value, e.g. also require `len(w.buf) > 0`
   at the root, or track whether a value's first byte was actually emitted.
2. Make `WriteRaw` reject an empty slice with a sentinel (a programming error),
   mirroring the other misuse sentinels in the file.
3. Both: reject an empty raw write *and* keep the state-based check.

Option 1 alone does not fix the `"1 2"` case (invalid but non-empty); that case
is arguably "raw is trusted", so the practical fix is option 2 plus a doc note
that `WriteRaw` must receive exactly one valid JSON value.

**Resolution (Q007).** The fix takes option 3 (both). `Writer.WriteRaw` now
rejects an empty slice with the `errEmptyRaw` sentinel, so the misuse is reported
where it happens and a nested empty raw write can no longer corrupt the document
(`[1,]`, `{"a":}`). `Writer.complete` additionally requires the root value to
have produced at least one byte (`len(w.buf) > 0`), so the guard checks the
invariant it claims to enforce rather than assuming no zero-byte writer exists.
`Marshal` therefore returns `errEmptyRaw` instead of `(nil, nil)` for an encoder
that calls `WriteRaw(nil)`. `TestMarshalEmptyRaw` pins the behavior through
`Marshal` and `WriteRaw`, and `TestWriterCompleteRequiresValue` pins the guard
directly. The `"1 2"` case is unchanged: `WriteRaw` is documented as trusted, so a
non-empty but invalid raw value remains the caller's responsibility.

---

## Issue 3 — `Object` / nested `Schema` panic on a nil schema

**Class:** unrecoverable failure (nil-pointer dereference on a nil `*Schema`).
**Severity:** medium — a programming error is a crash, not an inspectable error.
**Location:** `internal/codec/field.go` (`Object`).

```go
// internal/codec/field.go
func Object[T, S any](name string, field func(*T) *S, schema *Schema[S]) Field[T] {
	return Field[T]{
		Name: name,
		Decode: func(r *Reader, v *T) error {
			...
			return schema.Decode(r, field(v))   // schema may be nil
		},
		Encode: func(w *Writer, v *T) error {
			return schema.Encode(w, field(v))   // schema may be nil
		},
	}
}
```

A nil `*Schema` receiver dereference inside `Schema.Decode`/`Encode`
(`s.rejectDuplicate`, `s.decl`, …) panics.

**Reproduction (external probe):**

```text
Object(nil schema).Decode                     PANIC: runtime error: invalid memory address or nil pointer dereference
```

**Why this is a defect.** The container constructors already turn their misuse
into inspectable sentinels instead of panics:
`errNilElementDecoder` / `errNilElementEncoder` (`internal/codec/field.go`) are
returned by `Slice`/`Map`/`Array` when the element `Codec` is missing a
direction, and `errNoFieldEncoder` (`internal/codec/schema.go`) is returned by
`Schema.Encode` for a field with no encoder. All of these are tested
(`internal/codec/sentinel_test.go`). `Object` (and `ObjectDecoder`,
`ObjectDecoderPtr`, `DecoderCodec` for a nil receiver via a typed-nil field)
does not follow that pattern.

**Suggested fix.** Either validate at construction time (return cleanly or
panic with a clear message in a "must not be nil" constructor contract), or
check `schema == nil` in the returned closures and return a new sentinel such as
`errNilSchema` so the failure is inspectable with `errors.Is`, consistent with
the existing sentinels.

**Resolution (Q008).** `Object` now checks `schema == nil` in both closures and
returns the new `errNilSchema` sentinel, so the nil pointer is never
dereferenced and the misuse is inspectable with `errors.Is`. The check follows
the pattern of `errNilElementDecoder` / `errNilElementEncoder` and
`errNoFieldEncoder`. On decode it runs after the JSON-null branch, so a null
member still decodes without needing the schema, matching how a null container
decodes without needing its element codec. `TestObjectNilSchema` pins the decode
and encode errors and the null case.

---

## Issue 4 — `Struct` does not enforce unique field names

**Class:** silent misconfiguration.
**Severity:** medium — misconfiguration yields duplicate JSON keys and
non-deterministic field binding.
**Location:** `internal/codec/schema.go` (`Struct`, `lookup`).

`Struct`'s doc states the requirement but the constructor does not check it:

```go
// internal/codec/schema.go
// Struct returns a Schema ... Field names must be unique; the order of the
// arguments does not matter ...
func Struct[T any](fields ...Field[T]) *Schema[T] {
	s := &Schema[T]{
		fields: slices.Clone(fields),
		decl:   slices.Clone(fields),
	}
	slices.SortFunc(s.fields, ...) // no uniqueness check
	return s
}
```

`lookup` binary-searches the sorted slice, so with two fields named `"x"` it
returns whichever the search lands on, and `Encode` walks `decl` and emits both.

**Reproduction (external probe):**

```text
=== duplicate field names in Struct ===
    -> Marshal={"x":1,"x":2} err=<nil>
    -> Decode {"x":7} => {A:0 B:7} err=<nil>
```

The encode emits a JSON object with the duplicate key `x` (syntactically valid
but semantically ambiguous), and the decode binds `{"x":7}` to the second field
only.

**Why this is a defect.** The API states a hard requirement and silently
violates it, producing output that no reasonable consumer expects. Every other
`Struct` precondition that matters is enforced by construction (types) or by a
sentinel (containers).

**Suggested fix.** Validate in `Struct` (panic with a clear message, or accept
an error and provide a checked variant), or at least document that duplicate
names are undefined behavior. A panic at construction is defensible here
because it is a static programming error caught before any data flows, and it is
consistent with Go's "resolve programming errors loudly" convention; a returned
error would be more in line with the constructor-sentinel pattern already used
elsewhere.

---

## Issue 5 — Typed-nil receiver panics in `Decode` / `Encode` (documented)

**Class:** unrecoverable failure; inherent to the reflection-free design.
**Severity:** medium.
**Location:** `internal/codec/decode.go` (`Decode`), `internal/codec/encode.go`
(`Encode`).

`Decode` and `Encode` reject an untyped nil (`v == nil`) with `errNilTarget`, but
they cannot see a nil pointer *inside* a non-nil interface without reflection,
which the library forbids:

```text
Decode(data, (*cfg)(nil))                     PANIC: runtime error: invalid memory address or nil pointer dereference
```

The method is invoked on a nil receiver; whether it panics is up to the
`DecodeJSON`/`EncodeJSON` body. This is documented in both the code
(`internal/codec/decode.go`, `internal/codec/encode.go`) and the facade
(`qijson.go`), and in `docs/fn-comparison.md` ("Nil targets"). It is restated
here because it is a real crash a caller can hit; it is **not fixable** within
the no-reflection constraint, so the only remedy is clearer documentation and
generated methods that tolerate a nil receiver where that is meaningful.

## Issue 6 — Under-consuming `DecodeJSON` makes `Decode` accept invalid JSON (documented)

**Class:** defeated invariant.
**Severity:** medium.
**Location:** `internal/codec/decode.go` (`Decode`),
`internal/codec/scanner` (`Tokenizer.Peek`).

The trailing-content check runs only *after* the decoder returns, and `Peek`
classifies only the next byte:

```text
Decode('nope', underDecoder)                  err=<nil>
Decode('{}x', underDecoder)                   err=<nil>
```

A `DecodeJSON` that reads nothing and returns nil yields success on `nope`
(invalid JSON) and on `{}x` (trailing data). "Exactly one JSON value" is a
contract on the `Decoder`; the entry point enforces only the trailing half.
Library-provided decoders (Schema and the field readers) always consume the
value, so this affects hand-written decoders. Documented in
`docs/fn-comparison.md` ("Trailing input and the decoder's obligation", gap #4).

## Issue 7 — Value-receiver `DecodeJSON` silently no-ops (documented)

**Class:** silent misbehavior.
**Severity:** low.
**Location:** `internal/codec/decode.go` (`Decode`).

A type whose `DecodeJSON` has a value receiver compiles with
`Decode(data, value)`, reads into a copy, and returns nil:

```text
    -> caller copy N=0 (want 0 if no-op)
Decode(valueReceiverDecode)                   err=<nil>
```

`Unmarshal` cannot express this because its `PT` is `*T`. Documented in
`docs/fn-comparison.md` ("Nil targets", last paragraph). The remedy is a doc
note on the interface ("implement `DecodeJSON` on the pointer receiver") rather
than a code change.

## Issue 8 — `Encode` does not enforce completeness (documented)

**Class:** missing guard (by design).
**Severity:** low.
**Location:** `internal/codec/encode.go` (`Encode`).

```text
Encode(partial, noop)                         err=<nil>
    -> Bytes()="" complete-ok? err=<nil>
```

`Encode` returns the sticky writer error but never checks `w.complete()`, so an
encoder that returns nil without writing a complete value succeeds and leaves
unbalanced bytes readable through `Bytes()`. Documented in
`docs/fn-comparison.md` ("Completeness", gap #2); the note advises using
`Marshal` when the guarantee matters.

## Issue 9 — `errIncomplete` is unexported, so `errors.Is` cannot match it (documented)

**Class:** API gap.
**Severity:** low.
**Location:** `internal/codec/writer.go` (`errIncomplete`).

```text
    -> err=qijson: encoder did not write one complete JSON value isIncomplete=false
errors.Is(errIncomplete) observable?          err=qijson: encoder did not write one complete JSON value
```

The sentinel is not exported, so a caller can only match the message, which
`docs/fn-comparison.md` already records as gap #1. If the completeness failure
is meant to be programmatically distinguishable (as the other encode sentinels
are — `errNonFinite`, `errNilWriter`, `errNilTarget` are all reachable with
`errors.Is` through exported behavior), exporting it (or an `Is`-friendly
wrapper) would close the gap.

---

## Confirmed *not* defective (checked, looked risky)

To keep this record balanced, the following were inspected and found correct;
they are recorded so a later reader does not re-litigate them:

- **Scanner SWAR word-skip** (`internal/scanner/scan.go`, `stringStopWord`): the
  control-byte lane test (`zero(w & 0xe0e0...)`) isolates the top three bits, so
  a lane is zero exactly when the byte is `< 0x20`. The fast path is pinned
  differentially against a byte-at-a-time reference by
  `internal/scanner/fastpath_test.go` (`scanStringRef`), including control bytes
  placed past the eight-byte warm-up. No false-negative path that would skip a
  real stop byte.
- **`scanNumber` / tokenizer delimiters**: `1.2.3`, `{}x`, trailing comma,
  empty containers, and the top-level "after top-level value" contract match
  `docs/fn-comparison.md` and the tests.
- **`ReadFloat` underflow**: `strconv.ParseFloat("1e-400", 64)` returns
  `(0, nil)` — no `ErrRange` — so `numberMismatch` does *not* turn an underflow
  into a type error, and the doc comment ("a magnitude that underflows becomes
  zero") is accurate. Verified directly:
  `ParseFloat("1e-400") = 0, err=<nil>, ErrRange=false`.
- **UTF-8 policy**: the decoder rejects invalid UTF-8 in strings and keys as a
  `KindSyntax` error (`errInvalidUTF8`), while `Valid` (syntax-only) accepts it —
  a documented, intended split.
- **`Raw` copying**: `Raw.Decode` copies the captured bytes, so it does not
  alias the input; the tokenizer's own `ReadString`/`ReadNumber`/`ReadRaw`
  aliasing is documented and not leaked through `Raw`.
- **Determinism**: `Map` and `encodeAny` sort keys; `FuzzMarshalRoundTrip`
  confirms decode→encode→decode preserves the value.

## How the verification was run

- `go test ./...` — pass.
- `go test -race ./...` — pass.
- `gofmt -l .` — prints nothing.
- `go test -run=^$ -fuzz=FuzzValid -fuzztime=20s ./internal/scanner` — pass,
  ~3.4M executions.
- `go test -run=^$ -fuzz=FuzzMarshalRoundTrip -fuzztime=25s ./internal/codec` —
  pass, ~0.7M executions.
- A temporary external module (`replace github.com/qi-lore/qi-json => <repo>`)
  imported the public facade, reproduced every panic and every defeated guard
  quoted above, and was deleted afterward. It used only exported API, so it is
  reproducible by any consumer.

---

## Resolution indicator

> **Status: IN PROGRESS (3 of 9 issues fixed).**
>
> Issues 1–3 were fixed by tasks Q006, Q007 and Q008: the nil-target and
> nil-schema misuses now return sentinels instead of panicking, and `Marshal`
> rejects an empty raw write that used to defeat its completeness guard. The
> remaining new defect (4) and the five documented trade-offs (5–9) are still
> open. The canonical task state lives in `docs/tasks.md`; this indicator is
> informational and must be updated when further fixes land.
>
> Per-issue status:
>
> | # | Issue | Status |
> |---|---|---|
> | 1 | `Schema.UnmarshalInto`/`Decode` nil-target panic | RESOLVED (Q006) |
> | 2 | `Marshal` completeness defeated by `WriteRaw` | RESOLVED (Q007) |
> | 3 | `Object`/nested `Schema` nil-schema panic | RESOLVED (Q008) |
> | 4 | `Struct` unique-field-name enforcement | UNRESOLVED |
> | 5 | Typed-nil receiver panic (`Decode`/`Encode`) | UNRESOLVED (documented) |
> | 6 | Under-consuming `DecodeJSON` accepts invalid JSON | UNRESOLVED (documented) |
> | 7 | Value-receiver `DecodeJSON` no-op | UNRESOLVED (documented) |
> | 8 | `Encode` completeness not checked | UNRESOLVED (documented) |
> | 9 | `errIncomplete` unexported / not `errors.Is`-matchable | UNRESOLVED (documented) |
