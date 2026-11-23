# qi-json — enforce unique field names in `Struct`

**Date**: 2026-11-23
**Related**: `docs/panics-issues.md` (Issue 4), `docs/tasks.md` (Q009),
`internal/codec/schema.go`, `internal/codec/sentinel_test.go`.

This file is a design record. Task state lives in `docs/tasks.md`.

---

## 1. Why (the defect)

`Struct`'s doc states that field names must be unique, but the constructor never
checks it. With two fields named `"x"` the sorted `fields` slice makes `lookup`
bind a member to whichever duplicate the binary search lands on, and `Encode`
walks `decl` and writes the key twice:

```text
Marshal => {"x":1,"x":2}
Decode {"x":7} => second field only
```

The result is silent misconfiguration: output no reasonable consumer expects.

## 2. Decision

Detect duplicates in `Struct` and report them through an inspectable sentinel,
without panicking and without changing the constructor's signature.

- `Struct` sorts `fields` and then scans adjacent names. On the first repeated
  name it records a build error on the `Schema`:
  `fmt.Errorf("%w %q", errDuplicateFieldName, name)`.
- `Decode` and `Encode` return that build error before touching the reader or
  writer. Every other entry point funnels through one of them — `Marshal` calls
  `Encode`; `Unmarshal` calls `UnmarshalInto`, which calls `Decode` — so all six
  surface it.
- The sentinel `errDuplicateFieldName` lives unexported in `internal/codec`,
  matching `errNilElementDecoder`, `errNilSchema` and `errNoFieldEncoder`. It is
  inspectable with `errors.Is` from inside the package and carries the offending
  name in its message for external callers.

The build error is stored on the `Schema` (a new unexported `buildErr` field) so
the constructor can keep returning `*Schema[T]`. This preserves the public
signature that the generator and every fixture rely on, and keeps the library
free of a panic for a static programming error — the same direction as the Q006,
Q007 and Q008 fixes.

## 3. Alternatives considered

- **Panic in `Struct`.** Loud and immediate, and the audit calls it defensible,
  but it reintroduces exactly the crash class that Q006–Q008 removed and breaks
  the package's uniform sentinel treatment of constructor misuse.
- **Change `Struct` to return `(*Schema[T], error)`.** Cleanest static check,
  but a breaking signature change that ripples through generated code, the
  fixtures and every example for a pre-1.0 library that has otherwise kept the
  constructor ergonomic.

## 4. Verification

- `TestStructDuplicateFieldName` builds a `Struct` with two `"x"` fields and
  expects `errors.Is(err, errDuplicateFieldName)` from both `Unmarshal` and
  `Marshal`.
- `make verify` (tests, `gofmt`, `go vet`, `golangci-lint`).