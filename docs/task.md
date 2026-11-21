# Scanner implementation

## 1. Purpose

    Implement on **Golang**  a byte-at-a-time JSON syntax scanner as a deterministic state machine.

## 2. Role

You are expert backend and system engineer with wide lowlevel-programming experience. Your mostly preffered language is Golang&

## 3. Core Principles

- use Go >= 1.23.0 with no external dependencies
- NEVER use `encoding/json`. Our goal - write own implementation — ideally, a faster one
- when choosing solutions, ALWAYS consider performance. If one implementation runs faster than the others, ALWAYS choose the fastest one.
- ALWAYS use `https://github.com/uber-go/guide`
- write clean, human-readable, unambiguous, fault-tolerant code.
- ALWAYS use a TDD approach: tests MUST be written before production code. The red-green-refactor cycle is
 mandatory:
   - Write or update a failing test that expresses the requirement.
   - Confirm the test fails for the expected reason.
   - Implement the minimum code required to make the test pass.
   - Refactor without changing behavior.
   - Re-run all affected tests.

### The scanner must:

- accept bytes one at a time,
- validate JSON syntax,
- report significant parsing events through integer opcodes,
- maintain a stack of nested contexts,
- delay the “top-level value complete” event until the byte after the value is known.

**The caller drives the scanner**:

```text
    reset scanner
    for each input byte c:
        op := scanner.step(scanner, c)
        handle op; if op == scanError, stop
    after all bytes:
        op := scanner.eof()
        handle op
```

## 4. Scanner state

```text
   scanner {
     step: function(scanner, byte) -> int   // current state handler
     endTop: bool                            // top-level value completed
     parseState: stack of int                // nested contexts
     err: error                              // first syntax error
     bytes: int64                            // total bytes consumed
   }
```

**Context constants for parseState**:

```text
  parseObjectKey   // inside object, expecting a key
  parseObjectValue // inside object, expecting a value after ':'
  parseArrayValue  // inside array, expecting a value
```

**reset() must set**:

```text
  step      = stateBeginValue
  endTop    = false
  parseState = empty
  err       = nil
  // do NOT reset bytes
```

## 5. Opcodes

```text
   scanContinue     // no significant event; continue scanning
   scanBeginLiteral // beginning of string, number, true, false, null
   scanBeginObject  // consumed '{'
   scanBeginArray   // consumed '['
   scanEndObject    // consumed '}'
   scanEndArray     // consumed ']'
   scanEnd          // a complete top-level JSON value was completed
   scanError        // syntax error; scanner.err is set
```

## 6. Common helpers

```text
    isSpace(c) =
        c == ' ' or c == '\t' or c == '\n' or c == '\r'

    error(s, c, msg):
        if s.err == nil:
            s.err = syntax error including c and msg
    s.step = stateError
    return scanError

    push(s, context):
        append context to s.parseState

    pop(s):
        remove last element from s.parseState

    top(s):
        return last element of s.parseState
```

`stateError` always returns `scanError` and never changes state.

## 7. Central value-completion handler: stateEndValue

**This handler processes the byte that follows a completed value**.

```text
stateEndValue(s, c) -> opcode:
    if s.parseState is empty:
        // Top-level value just completed before byte c.
        s.endTop = true
        s.step = stateEndTop
        return stateEndTop(s, c)

    if isSpace(c):
        s.step = stateEndValue
        return scanContinue

    ctx = top(s)

    if ctx == parseObjectKey:
        if c == ':':
            top(s) = parseObjectValue
            s.step = stateBeginValue
            return scanContinue
        else:
            return error(s, c, "after object key")

    if ctx == parseObjectValue:
        if c == ',':
            top(s) = parseObjectKey
            s.step = stateBeginString
            return scanContinue
        if c == '}':
            pop(s)
            s.step = stateEndValue
            return scanEndObject
        else:
            return error(s, c, "after object value")

    if ctx == parseArrayValue:
        if c == ',':
            s.step = stateBeginValue
            return scanContinue
        if c == ']':
            pop(s)
            s.step = stateEndValue
            return scanEndArray
        else:
            return error(s, c, "after array value")
```

**`stateEndTop`**:

```text
stateEndTop(s, c) -> opcode:
    if not isSpace(c):
        return error(s, c, "after top-level value")
    return scanEnd
```

## 8. Beginning a value: stateBeginValue

```text
stateBeginValue(s, c) -> opcode:
    if isSpace(c):
        s.step = stateBeginValue
        return scanContinue

    if c == '{':
        push(s, parseObjectKey)
        s.step = stateBeginStringOrEmpty
        return scanBeginObject

    if c == '[':
        push(s, parseArrayValue)
        s.step = stateBeginValueOrEmpty
        return scanBeginArray

    if c == '"':
        s.step = stateInString
        return scanBeginLiteral

    if c == 't':
        s.step = stateLiteralT
        return scanBeginLiteral

    if c == 'f':
        s.step = stateLiteralF
        return scanBeginLiteral

    if c == 'n':
        s.step = stateLiteralN
        return scanBeginLiteral

    if c == '-':
        s.step = stateNumberNeg
        return scanBeginLiteral

    if c == '0':
        s.step = stateNumberZero
        return scanBeginLiteral

    if c >= '1' and c <= '9':
        s.step = stateNumberInt
        return scanBeginLiteral

    return error(s, c, "looking for beginning of value")
```

## 9. Empty object / empty array handling

**After `{`, an object may be empty**:

```text
stateBeginStringOrEmpty(s, c) -> opcode:
    if isSpace(c):
        s.step = stateBeginStringOrEmpty
        return scanContinue

    if c == '}':
        pop(s)
        s.step = stateEndValue
        return scanEndObject

    if c == '"':
        s.step = stateInString
        return scanBeginLiteral

    return error(s, c, "looking for beginning of object key string")
```

**After ``[``, an array may be empty**:

```text
stateBeginValueOrEmpty(s, c) -> opcode:
    if isSpace(c):
        s.step = stateBeginValueOrEmpty
        return scanContinue

    if c == ']':
        pop(s)
        s.step = stateEndValue
        return scanEndArray

    return stateBeginValue(s, c)
```

## 10. String scanning

```text
stateInString(s, c) -> opcode:
    if c == '"':
        s.step = stateEndValue
        return scanContinue
    if c == '\\':
        s.step = stateInStringEsc
        return scanContinue
    if c < 0x20:
        return error(s, c, "in string literal")
    s.step = stateInString
    return scanContinue
```

**Escape handling**:

```text
stateInStringEsc(s, c) -> opcode:
    if c in {'"', '\\', '/', 'b', 'f', 'n', 'r', 't'}:
        s.step = stateInString
        return scanContinue
    if c == 'u':
        s.step = stateInStringEscU
        return scanContinue
    return error(s, c, "in string escape code")
```

**Unicode escape \uXXXX requires exactly four hexadecimal digits**:

```text
stateInStringEscU(s, c):
    if isHex(c):
        s.step = stateInStringEscU1
        return scanContinue
    return error(s, c, "in \\u hexadecimal character escape")

stateInStringEscU1(s, c):
    if isHex(c):
        s.step = stateInStringEscU2
        return scanContinue
    return error(...)

stateInStringEscU2(s, c):
    if isHex(c):
        s.step = stateInStringEscU3
        return scanContinue
    return error(...)

stateInStringEscU3(s, c):
    if isHex(c):
        s.step = stateInString
        return scanContinue
    return error(...)
```

## 11. Literals: true, false, null

**Each literal is consumed character by character. 
The first character returns scanBeginLiteral; 
subsequent characters return scanContinue; 
after the last character, set step = stateEndValue.**

```text
stateLiteralT(s, c):
    if c == 'r': s.step = stateLiteralTr; return scanContinue
    return error(...)

stateLiteralTr(s, c):
    if c == 'u': s.step = stateLiteralTru; return scanContinue
    return error(...)

stateLiteralTru(s, c):
    if c == 'e':
        s.step = stateEndValue
        return scanContinue
    return error(...)
```

**Analogous chains**:

```text
f -> a -> l -> s -> e -> stateEndValue
n -> u -> l -> l -> stateEndValue
```

## 12. Number scanning

### JSON number grammar:

```text
number = [ '-' ] int [ frac ] [ exp ]
int    = '0' | [1-9] digit*
frac   = '.' digit+
exp    = ('e'|'E') [ '+' | '-' ] digit+
```
#### States:

```text
stateNumberNeg(s, c):
    if c == '0': s.step = stateNumberZero; return scanContinue
    if '1' <= c <= '9': s.step = stateNumberInt; return scanContinue
    return error(s, c, "in numeric literal")

stateNumberZero(s, c):
    // '0' cannot be followed by another digit
    if '0' <= c <= '9':
        return error(s, c, "in numeric literal")
    return handleNumberContinuation(s, c)

stateNumberInt(s, c):
    if '0' <= c <= '9':
        s.step = stateNumberInt
        return scanContinue
    return handleNumberContinuation(s, c)
```

#### Fraction:

```text
stateNumberDot(s, c):
    if '0' <= c <= '9':
        s.step = stateNumberFrac
        return scanContinue
    return error(s, c, "after decimal point in numeric literal")

stateNumberFrac(s, c):
    if '0' <= c <= '9':
        s.step = stateNumberFrac
        return scanContinue
    if c == 'e' or c == 'E':
        s.step = stateNumberExp
        return scanContinue
    return handleNumberContinuation(s, c)
```

#### Exponent:

```text
stateNumberExp(s, c):
    if c == '+' or c == '-':
        s.step = stateNumberExpSign
        return scanContinue
    if '0' <= c <= '9':
        s.step = stateNumberExpDigits
        return scanContinue
    return error(s, c, "in exponent of numeric literal")

stateNumberExpSign(s, c):
    if '0' <= c <= '9':
        s.step = stateNumberExpDigits
        return scanContinue
    return error(s, c, "in exponent of numeric literal")

stateNumberExpDigits(s, c):
    if '0' <= c <= '9':
        s.step = stateNumberExpDigits
        return scanContinue
    return handleNumberContinuation(s, c)
```

#### Continuation after a valid number:

```text
handleNumberContinuation(s, c):
    if c == '.':
        s.step = stateNumberDot
        return scanContinue
    if c == 'e' or c == 'E':
        s.step = stateNumberExp
        return scanContinue
    // The number is complete before byte c; let stateEndValue
    // process c as the delimiter after the value.
    s.step = stateEndValue
    return stateEndValue(s, c)
```

**Important**: the delimiter byte is processed by stateEndValue, not consumed separately.

## 13. EOF handling

**EOF must force completion of any pending value, especially numbers and literals.**

```text
eof(s) -> opcode:
    if s.err != nil:
        return scanError

    if s.endTop:
        return scanEnd

    // Feed a synthetic space to finish the current state.
    op := s.step(s, ' ')

    if s.err != nil:
        return scanError

    if s.endTop:
        return scanEnd

    return error(s, 0, "unexpected end of JSON input")
```

**The synthetic space is not a real input byte; it is only used to force delimiter handling.**

## 14. Main loop contract

```text
scan(s, data):
    s.reset()

    for each byte c in data:
        s.bytes++
        op := s.step(s, c)

        if op == scanError:
            return s.err

        // The caller may react to scanBeginObject, scanEndArray,
        // scanBeginLiteral, scanEnd, etc.

    op := s.eof()
    if op == scanError:
        return s.err

    return nil
```

### 15. Critical invariant: delayed scanEnd

`scanEnd means`:

> A complete top-level JSON value has been completed before the byte just passed to the scanner.

**Do not return scanEnd immediately after
reading the last digit of a number or the last character of a literal.
Wait until the next byte — or EOF — proves that the value has ended.**

Example:

```text
input: 123
```

After reading 1, 2, 3, the scanner cannot yet know whether 123 is complete or whether the next byte is 4 making 1234. Only when the next byte is a space, EOF, ], }, or , can the scanner decide that 123 is complete.

Therefore:

```text
number state sees delimiter c
-> set step = stateEndValue
-> return stateEndValue(s, c)
```

If this is the top level, `stateEndValue` sets `endTop` = true, moves to `stateEndTop`, and returns `scanEnd` for the delimiter byte.

## 16. Summary of state transition rules

```text
Whitespace:
    allowed between tokens; ignored in begin/end states.

Object:
    '{' push parseObjectKey -> beginStringOrEmpty
    key string -> endValue -> ':' -> parseObjectValue
    object value -> ',' -> parseObjectKey
                  -> '}' -> pop -> endValue

Array:
    '[' push parseArrayValue -> beginValueOrEmpty
    array value -> ',' -> beginValue
                -> ']' -> pop -> endValue

String:
    '"' -> inString -> escapes -> '"' -> endValue

Literal:
    exact match true/false/null -> endValue

Number:
    optional '-' -> int -> optional fraction -> optional exponent
    on first non-number byte -> endValue(delimiter)

Top level:
    after top-level value -> endTop = true
    stateEndTop accepts only whitespace
    EOF after complete value -> scanEnd
```
