# Scanner implementation

## 1. Purpose

Implement, in **Go**, a single-pass JSON syntax scanner that validates a whole
`[]byte` in one sweep, scanning runs of ordinary bytes in bulk instead of
dispatching once per byte.

## 2. Role

You are an expert backend and systems engineer with wide low-level programming
experience. Your preferred language is Go.

## 3. Core principles

- Use Go >= 1.24.0 with no external dependencies.
- NEVER use `encoding/json`. Our goal is to write our own implementation —
  ideally a faster one.
- When choosing solutions, ALWAYS consider performance. If one implementation
  runs faster than another, choose the faster one.
- ALWAYS follow `https://github.com/uber-go/guide`.
- Write clean, human-readable, unambiguous, fault-tolerant code.
- ALWAYS use a TDD approach: tests MUST be written before production code. The
  red-green-refactor cycle is mandatory:
  - Write or update a failing test that expresses the requirement.
  - Confirm the test fails for the expected reason.
  - Implement the minimum code required to make the test pass.
  - Refactor without changing behavior.
  - Re-run all affected tests.

### 3.1 The scanner must

- consume a whole byte slice in a single pass,
- validate JSON syntax,
- report a precise, human-readable error for the first failure,
- maintain a stack of open containers,
- treat a scalar as complete only when a delimiter or the end of input proves
  it.

### 3.2 The scanner must not

- dispatch once per byte through function pointers, closures or a handler table,
- expose parsing events, opcodes or a streaming `step`/`finish` interface,
- hold a `sync.Pool` or otherwise allocate on the success path.

The previous revision of this document specified a byte-at-a-time state machine
with opcodes. That architecture was measured at 2.3×–5.1× slower than
`encoding/json.Valid` on identical inputs and has been replaced. The rationale
is recorded in `.opencode/plan/2026-11-21-scanner-rewrite.md`.

## 4. Public surface

The scanner validates through one headline export, with the same name and
signature as `encoding/json.Valid`:

```go
// Valid reports whether data is a syntactically valid JSON text.
func Valid(data []byte) bool
```

It is a thin wrapper over an unexported function that returns the failure:

```go
func Valid(data []byte) bool {
	_, failed := validate(data)
	return !failed
}

// validate returns the first syntax error and whether data is invalid. The
// error is a value type so the validator's inline stack stays on the stack.
func validate(data []byte) (parseError, bool)
```

`validate` exists so that diagnostics — the error text and the 1-based position
— remain a testable contract without widening the exported surface.

`Valid` is the scanner's syntax validator. The package additionally exposes a
pull tokenizer — `Kind` and `Tokenizer`, in `scanner/token.go` — so the semantic
layer in `qijson` consumes the same engine instead of re-scanning. That export,
and the `qijson` decoder, are sanctioned by `docs/constitution.md` Article VI
and specified in `.opencode/plan/2026-11-21-json-decoder.md`; this document
specifies the syntax validator only.

## 5. Grammar

```text
json    = ws value ws
value   = object | array | string | number | "true" | "false" | "null"

object  = '{' ws ( member ( ws ',' ws member )* )? ws '}'
member  = string ws ':' ws value

array   = '[' ws ( value ( ws ',' ws value )* )? ws ']'

string  = '"' ( char | escape )* '"'
char    = any byte except '"', '\\' and the control bytes 0x00–0x1F
escape  = '\\' ( '"' | '\\' | '/' | 'b' | 'f' | 'n' | 'r' | 't' | 'u' hex hex hex hex )

number  = [ '-' ] int [ frac ] [ exp ]
int     = '0' | [1-9] digit*
frac    = '.' digit+
exp     = ( 'e' | 'E' ) [ '+' | '-' ] digit+

ws      = ( ' ' | '\t' | '\n' | '\r' )*
```

Two grammar rules are deliberately strict and must be preserved:

- A `0` integer part may not be followed by another digit, so `01` is invalid.
- At most one fraction and one exponent are allowed, so `1.2.3` and `1e1e2` are
  invalid. The offending byte is reported by the *delimiter* rule below, not by
  the number scanner.

## 6. Scanning model

The scanner walks the slice with an index `i`. The nesting stack holds one
`container` per open `{` or `[`; its depth is the number of open containers.

```text
container is one of:
    containerObject   // opened by '{'
    containerArray    // opened by '['
```

The nesting stack is a small fixed array on the caller's stack plus an explicit
depth counter:

```text
const nestingInline = 32

validator {
    data    []byte        // the input
    depth   int           // number of open containers
    stack   []container   // len == nestingInline until it grows
    failure *parseError   // first syntax error, or nil
}
```

`stack` starts as a slice of a local `[nestingInline]container`. The scanner
never calls `append` on the hot path; `push` writes `stack[depth]` and increments
`depth`, and `grow` copies into a heap slice of twice the length only when
`depth == len(stack)`. This keeps the fixed array on the stack and makes the
success path allocation-free.

### 6.1 Dispatch

`run` is one function built from two nested loops. There are no labels and no
`goto`. The outer loop scans exactly one value; the inner loop runs once a value
is complete, consumes the delimiter that follows it, closes every container that
ends there, and decides where the next value begins.

The outer loop's switch names what the next non-whitespace byte must be:

```text
'{'          open an object; when non-empty, scan its first member
'['          open an array; when non-empty, scan its first element
'"'          scan a string value
't' 'f' 'n'  scan the true / false / null literal
'-' '0'..'9' scan a number value
other        fail with "looking for beginning of value"
```

The inner loop, entered after a completed value, does:

```text
depth == 0   only whitespace may follow, else "after top-level value"
object top   ',' -> scan the next member (key, ':', then value)
             '}' -> close the object and repeat the inner loop
             else "after object value"
array top    ',' -> scan the next element in the outer loop
             ']' -> close the array and repeat the inner loop
             else "after array value"
```

There are no function pointers and no per-byte handler table. Measured on the
same documents, the two-loop form is within about 2% of a former labelled-jump
form. The "scan an object key and the `:` that follows" block is written twice —
just after `{` and just after an object `,` — on purpose: a shared helper cannot
be inlined (it calls `scanString`) and costs roughly 8% on object-heavy
documents. Keep the two copies in step.

## 7. Pseudocode

```text
run():
    data = v.data
    n    = len(data)
    i    = 0

    outer:                                 // scan exactly one value
        i = skipSpace(data, i)
        if i == n: return failEnd(n)

        switch c = data[i]:
        case '{':
            i = i + 1
            push(containerObject)
            i = skipSpace(data, i)
            if i == n: return failEnd(n)
            if data[i] != '}':
                i = memberStart(i)         // scan the first key and ':'
                if failure != nil: return false
                continue outer
            i = i + 1; pop()

        case '[':
            i = i + 1
            push(containerArray)
            i = skipSpace(data, i)
            if i == n: return failEnd(n)
            if data[i] != ']': continue outer
            i = i + 1; pop()

        case '"':
            i = scanString(i)
            if failure != nil: return false

        case 't': i = scanLiteral(i, "true");  if failure != nil: return false
        case 'f': i = scanLiteral(i, "false"); if failure != nil: return false
        case 'n': i = scanLiteral(i, "null");  if failure != nil: return false

        case '-', '0'..'9':
            i = scanNumber(i)
            if failure != nil: return false

        default:
            return fail(i, "looking for beginning of value")

        inner:                             // the value is complete
            if depth == 0:
                // The top-level value is complete before byte i.
                i = skipSpace(data, i)
                if i != n: return fail(i, "after top-level value")
                return true

            i = skipSpace(data, i)
            if i == n: return failEnd(n)

            if top() == containerObject:
                if data[i] == ',':
                    i = i + 1
                    i = memberStart(i)     // scan the next key and ':'
                    if failure != nil: return false
                    break inner            // scan the member value next
                if data[i] == '}':
                    i = i + 1; pop(); continue inner
                return fail(i, "after object value")

            if data[i] == ',':
                i = i + 1; break inner
            if data[i] == ']':
                i = i + 1; pop(); continue inner
            return fail(i, "after array value")

// memberStart is written inline at both call sites in scan.go; shown once here.
memberStart(i):
    i = skipSpace(data, i)
    if i == n: failEnd(n); return
    if data[i] != '"':
        fail(i, "looking for beginning of object key string"); return
    i = scanString(i)
    if failure != nil: return
    i = skipSpace(data, i)
    if i == n: failEnd(n); return
    if data[i] != ':': fail(i, "after object key"); return
    return i + 1
```

`skipSpace(data, i)` returns the index of the first byte at or after `i` that is
not `' '`, `'\t'`, `'\n'` or `'\r'`, or `len(data)` when only whitespace
remains.

## 8. Scalar scanners

Each scanner takes the index of the literal's first byte and returns the index
just past the scalar. A scanner that fails records the error in `v.failure` and
returns `len(data)`; the caller checks `v.failure` after every call.

### 8.1 Strings

```text
scanString(i):
    // data[i] == '"'
    long = false
    for i = i + 1; ; :
        if long:
            while i + 8 <= n and not stringStopWord(word(data[i:])):
                i = i + 8
            while i < n and not stringStop[data[i]]: i = i + 1
        else:
            // Warm up byte by byte: keys and short values end within a few
            // bytes, so they never pay for the word setup.
            short = min(i + 8, n)
            while i < short and not stringStop[data[i]]: i = i + 1
            if i == short:
                long = true
                while i + 8 <= n and not stringStopWord(word(data[i:])):
                    i = i + 8
                while i < n and not stringStop[data[i]]: i = i + 1
        if i == n:
            return failEnd(n)                 // unterminated string
        switch data[i]:
        case '"':  return i + 1
        case '\\': i = scanEscape(i)
                   if failure != nil: return n
                   long = true            // an escape means the string is long
        default:   fail(i, "in string literal"); return n

scanEscape(i):
    // data[i] == '\\'
    i = i + 1
    if i == n:
        failAt(n, "in string escape code"); return n
    switch data[i]:
    case '"', '\\', '/', 'b', 'f', 'n', 'r', 't': return i + 1
    case 'u':
        for k = 1 .. 4:
            if i + k == n:
                failAt(n, "in \u hexadecimal character escape"); return n
            if not isHexDigit(data[i + k]):
                fail(i + k, "in \u hexadecimal character escape"); return n
        return i + 5
    default:
        fail(i, "in string escape code"); return n
```

`stringStop` is a 256-entry table marking the bytes that end a plain run: `'"'`,
`'\\'` and every byte below `0x20`. `stringStopWord` applies the same test to a
64-bit word using SWAR: a byte is zero when `(v-1) &^ v & 0x80…80` is non-zero,
so XOR with the replicated quote or backslash byte, or mask with `0xe0…e0` for
control bytes, finds any terminator in eight bytes at once. Endianness does not
matter because the constants are byte-replicated. Word-at-a-time scanning lifts
long strings from about 1.2 GB/s to about 2.6 GB/s; the byte warm-up keeps short
keys cheap and the `long` flag keeps long strings in word mode after the first
run or escape.

Only syntax is checked. Raw UTF-8 bytes pass through unvalidated; surrogate
pairs are not paired or rejected.

### 8.2 Literals

```text
scanLiteral(i, lit):
    // data[i] == lit[0]
    for k = 1 .. len(lit) - 1:
        if i + k == n:
            failAt(n, "in literal " + lit + " (expecting '" + lit[k] + "')")
            return n
        if data[i + k] != lit[k]:
            fail(i + k, "in literal " + lit + " (expecting '" + lit[k] + "')")
            return n
    return i + len(lit)
```

### 8.3 Numbers

```text
scanNumber(i):
    if data[i] == '-':
        i = i + 1
        if i == n: failAt(n, "in numeric literal"); return n

    switch data[i]:
    case '0':
        i = i + 1
        if i < n and isDigit(data[i]):
            fail(i, "in numeric literal"); return n
    case '1'..'9':
        i = i + 1
        while i < n and isDigit(data[i]): i = i + 1
    default:
        fail(i, "in numeric literal"); return n

    if i < n and data[i] == '.':
        i = i + 1
        if i == n: failAt(n, "after decimal point in numeric literal"); return n
        if not isDigit(data[i]):
            fail(i, "after decimal point in numeric literal"); return n
        i = i + 1
        while i < n and isDigit(data[i]): i = i + 1

    if i < n and data[i] in {'e', 'E'}:
        i = i + 1
        if i < n and data[i] in {'+', '-'}: i = i + 1
        if i == n: failAt(n, "in exponent of numeric literal"); return n
        if not isDigit(data[i]):
            fail(i, "in exponent of numeric literal"); return n
        i = i + 1
        while i < n and isDigit(data[i]): i = i + 1

    return i
```

`scanNumber` returns the index of the first byte that cannot extend the number.
That byte is not consumed: the inner delimiter loop re-reads it, which is what
makes `1.2.3` fail with `after top-level value` at offset 4 and `[1.2.3]` fail
with `after array value`.

## 9. Error contract

`fail(i, context)` records

```text
invalid character '<c>' <context>
```

at the 1-based position `i + 1`, where `<c>` is `data[i]` rendered by
`quoteByte` (a single-quoted Go literal). Only the first error is kept; `run`
stops at it immediately.

`failEnd(n)` records `unexpected end of JSON input` at position `n`.

`failAt(pos, problem)` records `problem` verbatim at position `pos`.

Positions are 1-based offsets into `data`. An end-of-input error is reported at
`len(data)`, so an empty document fails at position 0.

The exact strings asserted by the test suite must be preserved:

```text
looking for beginning of value
looking for beginning of object key string
after object key
after object value
after array value
after top-level value
in string literal
in string escape code
in \u hexadecimal character escape
in literal true (expecting 'r')
in literal true (expecting 'u')
in literal true (expecting 'e')
in literal false (expecting 'a')
in literal false (expecting 'l')
in literal false (expecting 's')
in literal false (expecting 'e')
in literal null (expecting 'u')
in literal null (expecting 'l')
in numeric literal
after decimal point in numeric literal
in exponent of numeric literal
unexpected end of JSON input
```

## 10. Invariants

1. **Delayed completion.** A number or literal is complete only once a delimiter
   or the end of input follows it. `scanNumber` and `scanLiteral` never consume
   the delimiter; the inner delimiter loop judges it.
2. **Sticky first failure.** `run` returns at the first error and never
   overwrites `failure`.
3. **Top level.** Any JSON value is a valid document. Only JSON whitespace may
   follow it; anything else fails with `after top-level value`.
4. **Syntax only.** No UTF-8 validation, no surrogate pairing, no nesting depth
   limit.
5. **Allocation-free success path.** For documents nesting at most
   `nestingInline` levels, `Valid` allocates nothing. Deeper documents allocate
   the nesting stack once and grow it geometrically.
6. **Deterministic.** The same input always produces the same verdict.

## 11. File layout

```text
scanner/
  valid.go   // package doc, Valid, validate
  scan.go    // validator, dispatch, scanners, tables, parseError
  token.go   // Kind and the tokenizer used by qijson
  *_test.go  // tests and benchmarks
```

The semantic layer itself (the reflection-free decoder and encoder) is the
`qijson` package at the module root: a facade of type aliases, constants and
wrappers over the engine in `internal/codec`. It is specified by its own plans;
this document specifies the syntax validator only.

## 12. Verification

Every task finishes by running, in order: `go test ./...`, `gofmt -l .`,
`go vet ./...`, `golangci-lint run`. Shortcut: `make verify`.

Additional commands:

```text
go test -race ./...
go test ./scanner -run '^TestInvalidDocuments$'
go test -run=^$ -fuzz=FuzzValid -fuzztime=10s ./scanner
go test -run=^$ -bench=. -benchmem ./scanner
```

Performance is a feature: `BenchmarkValid*` must not regress, and the success
path must stay allocation-free (`TestValidDoesNotAllocate`).
