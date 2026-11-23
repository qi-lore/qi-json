# qi-json — make the `Marshal` completeness guard hold against an empty `WriteRaw`

**Date**: 2026-11-23
**Related**: `docs/panics-issues.md` (Issue 2), `docs/tasks.md` (Q007),
`internal/codec/writer.go`, `internal/codec/encode.go`.

This file is a design record. Task state lives in `docs/tasks.md`.

---

## 1. Why (the defect)

`Marshal` is the only entry point that enforces "exactly one complete top-level
JSON value". It does so through `Writer.complete`:

```go
func (w *Writer) complete() bool {
	return w.err == nil && w.wroteRoot && len(w.stack) == 0
}
```

`wroteRoot` is set by `endValue`, which runs for every value write — including
`WriteRaw`, whose body appends the caller's bytes and may append **zero** bytes:

```go
func (w *Writer) WriteRaw(v []byte) error {
	if err := w.beginValue(); err != nil { return err }
	w.buf = append(w.buf, v...)   // may append nothing
	w.endValue()                  // depth 0 => wroteRoot = true
	return nil
}
```

So an `Encoder` that calls `WriteRaw(nil)` (or `WriteRaw([]byte{})`) marks the
document complete without writing a value, and `Marshal` returns `(nil, nil)` —
or, for a non-empty but invalid raw token such as `"1 2"`, hands back invalid
JSON with a nil error. The library's own `Raw` field never triggers this (it
writes `null` for an empty slice), but `WriteRaw` is a documented escape hatch
for hand-written encoders, which is exactly the path the guard exists to protect.

## 2. Decision

`docs/panics-issues.md` Issue 2 lists three candidate fixes and asks for a
decision. This task takes **option 3 (both)**:

1. **Harden the guard.** `complete` additionally requires the root value to have
   produced at least one byte (`len(w.buf) > 0`). At completion the frame stack
   is empty, so every byte in `w.buf` belongs to the root value; a non-empty
   buffer is exactly "a value was emitted". This makes the guard check the
   invariant it claims to enforce instead of assuming no zero-byte writer exists.
2. **Reject the misuse at its source.** `WriteRaw` rejects an empty slice with a
   new `errEmptyRaw` sentinel, mirroring the other misuse sentinels in the file
   (`errKeyOutsideObject`, `errMultipleRoots`, …). This also fixes the nested
   case the guard cannot see: an empty raw write inside an array or object
   produced `[1,]` / `{"a":}`.

The two are complementary, not redundant: (2) reports the programming error
where it happens and keeps the buffer well-formed, while (1) keeps the guard
sound on its own should another zero-byte write path ever appear.

The non-empty-but-invalid case (`WriteRaw([]byte("1 2"))`) is deliberately left
alone: `WriteRaw` is documented as trusted input, so validating arbitrary raw
bytes is out of scope. The doc comment now states the precondition explicitly.

## 3. Alternatives considered

- **Option 1 only** (harden the guard): satisfies the task title and the
  root-level reproduction, but leaves the nested `[1,]` corruption, and reports
  the misuse as `errIncomplete` (which reads as "the encoder forgot to write")
  rather than as the actual programming error.
- **Option 2 only** (reject empty raw): closes both the root and nested cases,
  but leaves the guard depending on the absence of zero-byte writers.

## 4. Verification

- `TestMarshalEmptyRaw` reproduces the defect through the public API: `Marshal`
  of an encoder that calls `WriteRaw(nil)` must not return `(nil, nil)`; it now
  returns `errEmptyRaw`, and `WriteRaw(nil)` / `WriteRaw([]byte{})` set the
  sticky error.
- `TestWriterCompleteRequiresValue` pins the guard directly: a writer with
  `wroteRoot` set and an empty buffer is not complete.
- `make verify` (tests, `gofmt`, `go vet`, `golangci-lint`).
