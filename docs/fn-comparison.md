# Generic and non-generic entry points: semantic comparison

This note records the semantic differences between the library's generic and
non-generic entry points, with the behavior pinned by the code, not by the
doc comments alone. Reviewed at `12e5c06` (v0.4.1); the code paths are unchanged
on `dev`.

The comparison covers two pairs:

- decode: `Unmarshal` / `UnmarshalInto` versus `Decode`
- encode: `Marshal` versus `Encode`

"Generic versus non-generic" is not the interesting part. The differences below
remain even after the type argument is put aside.

## Signatures

```go
func Unmarshal[T any, PT interface {
	*T
	Decoder
}](data []byte) (T, error)

func UnmarshalInto[T any, PT interface {
	*T
	Decoder
}](data []byte, v *T) error

func Decode(data []byte, v Decoder) error

func Marshal[T any, PT interface {
	*T
	Encoder
}](v T) ([]byte, error)

func Encode(w *Writer, v Encoder) error
```

`Decoder` is `DecodeJSON(*Reader) error`; `Encoder` is `EncodeJSON(*Writer) error`.

## Implementation layering

Decoding is a strict layering: each non-generic level delegates to the next.

```text
Unmarshal(data)
  └─ var v T
     └─ UnmarshalInto(data, &v)
        └─ nil check on *T
           └─ Decode(data, PT(v))
              └─ nil check on Decoder
                 └─ v.DecodeJSON(newReader(NewTokenizer(data)))
                    └─ trailing-content check (Peek)
```

Encoding is not layered the same way: `Marshal` does **not** call `Encode`.

```text
Marshal(v)
  └─ var w Writer
     └─ PT(&v).EncodeJSON(&w)
        └─ sticky writer error?
        └─ w.complete()?          // exactly one closed root value
        └─ return w.buf

Encode(w, v)
  └─ nil checks
     └─ v.EncodeJSON(w)
        └─ sticky writer error?   // no completeness check
```

This asymmetry explains most of the encode-side differences.

## Decoding: `Unmarshal` versus `Decode`

`Decode` is exactly `UnmarshalInto` without the type argument:
`UnmarshalInto` initializes nothing, checks its `*T`, and calls `Decode`.
`TestDecodeMatchesUnmarshalInto` pins equality for seven inputs, including
merge, wrong type, invalid JSON, trailing data, and a field type error.

Beyond the generics, the differences are:

| Aspect | `Unmarshal[T]` | `Decode(data, v)` |
|---|---|---|
| Starting value | fresh zero `T` | caller's existing value |
| Merge semantics | merge into a zero value, so absent members become zero values | merge into a live value, so absent members keep their old values |
| Result | returns a `T` by value | mutates `v`, returns only `error` |
| Nil target | impossible: the call always passes `&v` | untyped nil is caught; a typed nil is not |
| Nil check | `UnmarshalInto` rejects a nil `*T` with `errNilTarget` | `Decode` rejects only a nil interface |
| Binding | compile-time constraint `PT = *T`; the pointer method set must implement `Decoder` | dynamic dispatch on any `Decoder` implementation |
| Value on error | returns the partially decoded `T` together with the error | leaves partial writes in `v` |

### What is identical

When the `Decoder` behaves as documented and consumes one JSON value:

- the same `Reader`/`Tokenizer` is created per call; no pooling;
- syntax and type failures are `*Error` values carrying the JSON path and byte
  offset; all three entry points report the same message for the same input;
- a nil target is the plain sentinel `qijson: nil target`, not a `*Error`;
- trailing content after the consumed value is a syntax error;
- `data` is not retained and is not modified (`Raw` copies the captured bytes);
- the target is not usable concurrently with other decoding into it, and the
  per-call `Reader` is never shared.

### Merge versus fresh value

`Unmarshal` starts from `var v T`. For a `Schema`-backed type, that means
"member present → overwrite; member absent → zero value". `UnmarshalInto` and
`Decode` start from whatever the caller already holds, so absent members keep
their previous values:

```text
v := cfg{Name: "keep", Port: 1}
Decode(`{"port":9}`, &v)        → v == {Name: "keep", Port: 9}
```

`TestUnmarshalIntoMerge` and `TestDecodeMerge` pin this behavior.

### Nil targets

```text
UnmarshalInto[cfg](data, (*cfg)(nil))  → qijson: nil target
Decode(data, (*cfg)(nil))             → DecodeJSON called with a nil receiver
Decode(data, nil)                     → qijson: nil target
```

`Decode` cannot distinguish an interface holding a typed nil from a non-nil
receiver without reflection, which the library forbids. `UnmarshalInto`
compares the concrete `*T` before converting it to the interface, so it catches
the nil pointer. `Unmarshal` cannot receive a nil target at all.

A related consequence of the interface form: a type whose `DecodeJSON` has a
**value receiver** compiles with `Decode(data, value)`, reads the JSON into a
copy, and returns nil without changing the caller's value. `Unmarshal` cannot
express this because `PT` is `*T`.

### Partial writes on error

Writes happen per field, as each member succeeds. A failure therefore does not
roll back earlier members:

```text
v := cfg{Name: "keep", Port: 5}
Decode(`{"name":"new","port":"nope"}`, &v)
  → error at $.port
  → v == {Name: "new", Port: 5}

Unmarshal[cfg](`{"name":"new","port":"nope"}`)
  → returns {Name: "new", Port: 0} plus the error
```

`TestUnmarshalIntoPreservesTargetOnError` covers only a document whose first
written member fails, so its comment ("a failed decode leaves the target as it
was before the call") describes that case, not a general atomicity guarantee.
The public doc comments promise merging, not rollback.

### Trailing input and the decoder's obligation

Neither `Unmarshal` nor `Decode` verifies that `DecodeJSON` consumed a value.
The trailing check runs after `DecodeJSON` returns, so it only reports input
left **after** the tokenizer has passed a value:

```text
Decode(`null x`, decoder that reads null)  → after top-level value (offset 5)
Decode(`null`,   decoder that reads null)  → nil
Decode(`nope`,   decoder that reads nothing) → nil   // no read, no syntax check
Decode(`{}`,     decoder that reads nothing) → nil
```

Library-provided decoders (Schema and the field readers) always consume the
value, so this only affects a hand-written `DecodeJSON` that returns early.
"Exactly one JSON value" is a contract on the `Decoder`; the entry point only
enforces the trailing half of it.

## Encoding: `Marshal` versus `Encode`

| Aspect | `Marshal[T]` | `Encode(w, v)` |
|---|---|---|
| Output | owns a fresh `Writer`; returns its buffer | writes into the caller's `Writer` |
| Output lifetime | caller-owned, independent of any `Writer` | aliases `w.buf`; valid until the next write or `Reset`; `Bytes` returns nil after a sticky error |
| Completeness | enforces exactly one complete closed value; otherwise `errIncomplete` | does not check completeness; only the sticky writer error |
| Encoder input | `PT(&v)`: the address of a **copy** of `v` | the caller's dynamic value; a pointer receiver can mutate it |
| Addressability | `Marshal[Config](cfg)` works when `EncodeJSON` is on `*Config` | the argument must already implement `Encoder`, so pass `&cfg` |
| Nil checks | none needed for the encoder; no writer argument | nil writer → `qijson: nil Writer`; untyped nil encoder → `qijson: nil target`; typed nil is not detected |
| Composition | always a whole document | can write into an in-progress document |
| On error | returns `nil, err`; partial output is discarded | returns the error but leaves partial output in `w` |

### Completeness

`Marshal` rejects an encoder that returns nil without writing exactly one
complete value. `Encode` accepts it:

```text
Encode(&w, noopEncoder)      → nil, Bytes() == ""       // nothing written
Marshal[noopEnc]({})         → qijson: encoder did not write one complete JSON value

Encode(&w, partialEncoder)   → nil, Bytes() == "{"      // object never closed
Marshal[partialEnc]({})      → qijson: encoder did not write one complete JSON value
```

`TestMarshalIncomplete` pins the `Marshal` guard. There is no equivalent test
for `Encode` because there is no guard: `Encode` returns success and leaves the
unbalanced bytes readable through `w.Bytes()` (the sticky error is nil, so
`Bytes` does not suppress them).

`errIncomplete` is unexported. A caller cannot use `errors.Is` to recognize it;
the only observable form is the message.

### Copy versus original value

`Marshal` takes `v` by value and calls `PT(&v)`, so the encoder sees a copy:

```text
c := &countingEnc{}
Marshal[countingEnc](*c)   → encoder increments the copy; c.n stays 0
Encode(&w, c)              → encoder increments the caller's value; c.n becomes 1
```

This is a real semantic difference, not just an allocation difference: an
`EncodeJSON` method that caches, normalizes, or otherwise mutates its receiver
affects the caller only through `Encode`.

### Composition into an existing document

`Encode` can target a `Writer` that is already inside a container, so an
`Encoder` can be used as an object member or array element:

```text
w.BeginObject(); w.Key("x"); Encode(&w, enc{ /* writes 7 */ }); w.EndObject()
  → {"x":7}
```

`Marshal` always produces a standalone document. `Encode` into a writer that
already has a top-level value is rejected by the writer itself with
`qijson: multiple top-level values`, after which `Bytes` returns nil.

### Error precedence

Both entry points return the error from `EncodeJSON` as-is. If the encoder
returned nil but left a sticky error on the `Writer`, both return that sticky
error. `Marshal` then applies the completeness check; `Encode` does not.
`Schema.Marshal` / `Schema.Encode` mirror the same split: only the `Marshal`
form checks completeness.

## Documentation gaps worth noting

These are observations from the code, not current failures:

1. The public `Marshal` doc comments do not mention `errIncomplete`, and the
   sentinel is unexported, so the failure mode is only visible as a message.
2. The public `Encode` doc says an error left on the `Writer` is surfaced, but
   does not say that completeness is **not** checked. Given the symmetry with
   `Marshal`, this is easy to miss.
3. `TestUnmarshalIntoPreservesTargetOnError`'s comment overstates the guarantee:
   earlier successful members are not rolled back on a later failure.
4. The decode entry points validate trailing content only; an under-consuming
   `DecodeJSON` can return success without reading anything, including on
   syntactically invalid input.

## Conclusions

- **Decode:** `Unmarshal` and `Decode` run the same algorithm; `Decode` is
  `UnmarshalInto` without the type argument. The only semantic differences are
  the starting value (fresh zero versus merge), the result (returned `T` versus
  in-place mutation), and nil handling (`UnmarshalInto` catches a nil `*T`;
  `Decode` cannot catch a typed nil).
- **Encode:** `Marshal` and `Encode` differ in ownership of the output, whether
  the encoder sees a copy or the original value, whether the call can compose
  into an in-progress document, and — the largest gap — whether one complete
  JSON value is enforced. `Marshal` is not "`Encode` plus allocation"; it is
  `EncodeJSON` plus the completeness guard.
- **Failures are not transactional.** All entry points leave prior successful
  writes in place. Only `Schema`-level field assignment is per-member atomic.
- **Practical choice:** use `Unmarshal` for a fresh value, `Decode` for an
  in-place merge when the concrete type is already known, `Marshal` for a
  standalone document, and `Encode` when composing into an existing `Writer`.
  If the completeness guarantee matters, use `Marshal` (or check the result
  yourself after `Encode`, since `Encode` will not).

## Sources

- `internal/codec/decode.go`: `Decoder`, `Unmarshal`, `UnmarshalInto`, `Decode`,
  `errNilTarget`.
- `internal/codec/encode.go`: `Encoder`, `Marshal`, `Encode`, `errNilWriter`.
- `internal/codec/schema.go`: `Schema.Decode`, `Schema.Encode`,
  `Schema.Marshal`, `Schema.Unmarshal`, `Schema.UnmarshalInto`.
- `internal/codec/writer.go`: `Writer.complete`, `Writer.Bytes`, sticky errors,
  `errIncomplete`.
- Tests: `TestUnmarshalGeneric`, `TestUnmarshalIntoMerge`,
  `TestDecodeMerge`, `TestDecodeMatchesUnmarshalInto`, `TestDecodeErrorKinds`,
  `TestUnmarshalIntoPreservesTargetOnError`, `TestMarshalIncomplete`,
  `TestEncodeNil`, `TestEncodeSurfacesWriterError`.

Behavior was additionally verified with a temporary external program importing
`github.com/qi-lore/qi-json`; the program was deleted after the run.