// Package marshal is the semantic layer of qi-json: a reflection-free JSON
// decoder built on the syntax engine in qi-json/scanner.
//
// The API is deliberately not compatible with encoding/json. Decoding arbitrary
// targets without reflection needs a compile-time field mapping, so the entry
// point is generic — Unmarshal[T] / UnmarshalInto[T] — and struct binding uses
// accessor closures (Field[T]) rather than reflect or unsafe. See
// .opencode/plan/2026-11-21-json-decoder.md for the design and task list.
//
// This package is currently a placeholder: it exports nothing until the first
// decoder feature is implemented.
package marshal
