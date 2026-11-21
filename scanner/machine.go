package scanner

import "strconv"

// opcode is a scanning event produced for one input byte.
type opcode int

const (
	// opContinue reports that the byte produced no significant event.
	opContinue opcode = iota

	// opLiteralStart marks the first byte of a string, number, true, false,
	// or null.
	opLiteralStart

	// opObjectStart marks a consumed '{'.
	opObjectStart

	// opObjectEnd marks a consumed '}'.
	opObjectEnd

	// opArrayStart marks a consumed '['.
	opArrayStart

	// opArrayEnd marks a consumed ']'.
	opArrayEnd

	// opDocumentEnd marks completion of the top-level value. It is reported
	// for the byte that proves completion, or by finish, and may repeat for
	// trailing whitespace.
	opDocumentEnd

	// opFailure marks a syntax error. machine.failure describes it.
	opFailure
)

// String returns a short name for the event, for example "objectStart".
func (o opcode) String() string {
	switch o {
	case opContinue:
		return "continue"
	case opLiteralStart:
		return "literalStart"
	case opObjectStart:
		return "objectStart"
	case opObjectEnd:
		return "objectEnd"
	case opArrayStart:
		return "arrayStart"
	case opArrayEnd:
		return "arrayEnd"
	case opDocumentEnd:
		return "documentEnd"
	case opFailure:
		return "failure"
	default:
		return "opcode(" + strconv.Itoa(int(o)) + ")"
	}
}

// parseError describes the first syntax error found by a machine.
type parseError struct {
	// at is the 1-based position of the offending byte in the machine's byte
	// stream. For end-of-input errors it is the number of consumed bytes,
	// which reset deliberately preserves.
	at int64

	// problem explains the error, for example
	// "invalid character '}' after object value".
	problem string
}

// Error implements the error interface.
func (e *parseError) Error() string { return e.problem }

// expectation is what the current container still expects to see.
type expectation uint8

const (
	expectKey expectation = iota
	expectValue
	expectElement
)

// handlerFunc handles one input byte and returns the resulting opcode.
type handlerFunc func(*machine, byte) opcode

// machine is a deterministic JSON syntax state machine. It consumes one byte
// at a time, must not be copied after first use, and is not safe for
// concurrent use.
type machine struct {
	handler  handlerFunc
	complete bool
	stack    []expectation
	consumed int64
	failure  *parseError
}

// reset prepares the machine for a new document. It does not clear consumed,
// so failure positions keep counting across documents.
func (m *machine) reset() {
	m.handler = beginValue
	m.complete = false
	m.failure = nil
	m.stack = m.stack[:0]
}

// step consumes c and returns the resulting opcode. A zero machine is ready
// to use.
func (m *machine) step(c byte) opcode {
	if m.handler == nil {
		m.reset()
	}
	m.consumed++
	return m.handler(m, c)
}

// finish ends the input. It forces a pending value or container to complete
// with a synthetic space that is not counted in consumed.
func (m *machine) finish() opcode {
	if m.handler == nil {
		m.reset()
	}
	if m.failure != nil {
		return opFailure
	}
	if m.complete {
		return opDocumentEnd
	}

	m.handler(m, ' ')

	if m.failure != nil {
		return opFailure
	}
	if m.complete {
		return opDocumentEnd
	}
	return m.failEnd()
}

// enter pushes an expectation onto the nesting stack.
func (m *machine) enter(e expectation) {
	m.stack = append(m.stack, e)
}

// leave pops the innermost expectation.
func (m *machine) leave() {
	m.stack = m.stack[:len(m.stack)-1]
}

// context returns the innermost expectation. It must not be called on an
// empty stack.
func (m *machine) context() expectation {
	return m.stack[len(m.stack)-1]
}

// replace swaps the innermost expectation without changing the depth.
func (m *machine) replace(e expectation) {
	m.stack[len(m.stack)-1] = e
}

// fail records c as the cause of a syntax error in the given context. Only
// the first error is kept.
func (m *machine) fail(c byte, context string) opcode {
	m.handler = errorState
	if m.failure == nil {
		m.failure = &parseError{
			at:      m.consumed,
			problem: "invalid character " + quoteByte(c) + " " + context,
		}
	}
	return opFailure
}

// failEnd records an error for input that ended inside a value.
func (m *machine) failEnd() opcode {
	m.handler = errorState
	m.failure = &parseError{at: m.consumed, problem: "unexpected end of JSON input"}
	return opFailure
}

// quoteByte formats c for a diagnostic, for example '\t' or '\xff'.
func quoteByte(c byte) string {
	// Special cases - different from quoted strings.
	if c == '\'' {
		return `'\''`
	}
	if c == '"' {
		return `'"'`
	}

	q := strconv.Quote(string(c))
	return "'" + q[1:len(q)-1] + "'"
}

// isWhitespace reports whether c is JSON whitespace.
func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// isDigit reports whether c is an ASCII decimal digit.
func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

// isHexDigit reports whether c is an ASCII hexadecimal digit.
func isHexDigit(c byte) bool {
	switch {
	case '0' <= c && c <= '9':
		return true
	case 'a' <= c && c <= 'f':
		return true
	case 'A' <= c && c <= 'F':
		return true
	default:
		return false
	}
}

// errorState is the state after a syntax error; it never recovers.
func errorState(*machine, byte) opcode { return opFailure }

// afterDocument is the state after a complete top-level value. Only
// whitespace may follow.
func afterDocument(m *machine, c byte) opcode {
	if !isWhitespace(c) {
		return m.fail(c, "after top-level value")
	}
	return opDocumentEnd
}

// afterValue processes the byte that follows a completed value.
func afterValue(m *machine, c byte) opcode {
	if len(m.stack) == 0 {
		// The top-level value just completed before byte c.
		m.complete = true
		m.handler = afterDocument
		return afterDocument(m, c)
	}

	if isWhitespace(c) {
		m.handler = afterValue
		return opContinue
	}

	switch m.context() {
	case expectKey:
		if c == ':' {
			m.replace(expectValue)
			m.handler = beginValue
			return opContinue
		}
		return m.fail(c, "after object key")

	case expectValue:
		if c == ',' {
			m.replace(expectKey)
			m.handler = beginKey
			return opContinue
		}
		if c == '}' {
			m.leave()
			m.handler = afterValue
			return opObjectEnd
		}
		return m.fail(c, "after object value")

	default: // expectElement
		if c == ',' {
			m.handler = beginValue
			return opContinue
		}
		if c == ']' {
			m.leave()
			m.handler = afterValue
			return opArrayEnd
		}
		return m.fail(c, "after array value")
	}
}

// beginValue is the state at the start of a JSON value.
func beginValue(m *machine, c byte) opcode {
	if isWhitespace(c) {
		m.handler = beginValue
		return opContinue
	}

	switch {
	case c == '{':
		m.enter(expectKey)
		m.handler = beginKeyOrEnd
		return opObjectStart
	case c == '[':
		m.enter(expectElement)
		m.handler = beginValueOrEnd
		return opArrayStart
	case c == '"':
		m.handler = inString
		return opLiteralStart
	case c == 't':
		m.handler = trueR
		return opLiteralStart
	case c == 'f':
		m.handler = falseA
		return opLiteralStart
	case c == 'n':
		m.handler = nullU
		return opLiteralStart
	case c == '-':
		m.handler = numberSign
		return opLiteralStart
	case c == '0':
		m.handler = numberZero
		return opLiteralStart
	case '1' <= c && c <= '9':
		m.handler = numberDigits
		return opLiteralStart
	default:
		return m.fail(c, "looking for beginning of value")
	}
}

// beginValueOrEnd is the state after '[': the array may be empty.
func beginValueOrEnd(m *machine, c byte) opcode {
	if isWhitespace(c) {
		m.handler = beginValueOrEnd
		return opContinue
	}
	if c == ']' {
		m.leave()
		m.handler = afterValue
		return opArrayEnd
	}
	return beginValue(m, c)
}

// beginKeyOrEnd is the state after '{': the object may be empty.
func beginKeyOrEnd(m *machine, c byte) opcode {
	if isWhitespace(c) {
		m.handler = beginKeyOrEnd
		return opContinue
	}
	if c == '}' {
		m.leave()
		m.handler = afterValue
		return opObjectEnd
	}
	if c == '"' {
		m.handler = inString
		return opLiteralStart
	}
	return m.fail(c, "looking for beginning of object key string")
}

// beginKey is the state after ',' in an object: another key is required
// (trailing commas are not allowed).
func beginKey(m *machine, c byte) opcode {
	if isWhitespace(c) {
		m.handler = beginKey
		return opContinue
	}
	if c == '"' {
		m.handler = inString
		return opLiteralStart
	}
	return m.fail(c, "looking for beginning of object key string")
}

// inString scans string contents until the closing quote.
func inString(m *machine, c byte) opcode {
	switch {
	case c == '"':
		m.handler = afterValue
		return opContinue
	case c == '\\':
		m.handler = inEscape
		return opContinue
	case c < 0x20:
		return m.fail(c, "in string literal")
	default:
		m.handler = inString
		return opContinue
	}
}

// inEscape scans the byte after a backslash.
func inEscape(m *machine, c byte) opcode {
	switch c {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		m.handler = inString
		return opContinue
	case 'u':
		m.handler = unicodeEscape1
		return opContinue
	default:
		return m.fail(c, "in string escape code")
	}
}

// unicodeEscape1 scans the first hexadecimal digit of a \uXXXX escape.
func unicodeEscape1(m *machine, c byte) opcode {
	if isHexDigit(c) {
		m.handler = unicodeEscape2
		return opContinue
	}
	return m.fail(c, `in \u hexadecimal character escape`)
}

// unicodeEscape2 scans the second hexadecimal digit of a \uXXXX escape.
func unicodeEscape2(m *machine, c byte) opcode {
	if isHexDigit(c) {
		m.handler = unicodeEscape3
		return opContinue
	}
	return m.fail(c, `in \u hexadecimal character escape`)
}

// unicodeEscape3 scans the third hexadecimal digit of a \uXXXX escape.
func unicodeEscape3(m *machine, c byte) opcode {
	if isHexDigit(c) {
		m.handler = unicodeEscape4
		return opContinue
	}
	return m.fail(c, `in \u hexadecimal character escape`)
}

// unicodeEscape4 scans the fourth hexadecimal digit of a \uXXXX escape.
func unicodeEscape4(m *machine, c byte) opcode {
	if isHexDigit(c) {
		m.handler = inString
		return opContinue
	}
	return m.fail(c, `in \u hexadecimal character escape`)
}

// trueR is the state after reading 't'.
func trueR(m *machine, c byte) opcode {
	if c == 'r' {
		m.handler = trueRU
		return opContinue
	}
	return m.fail(c, "in literal true (expecting 'r')")
}

// trueRU is the state after reading "tr".
func trueRU(m *machine, c byte) opcode {
	if c == 'u' {
		m.handler = trueRUE
		return opContinue
	}
	return m.fail(c, "in literal true (expecting 'u')")
}

// trueRUE is the state after reading "tru".
func trueRUE(m *machine, c byte) opcode {
	if c == 'e' {
		m.handler = afterValue
		return opContinue
	}
	return m.fail(c, "in literal true (expecting 'e')")
}

// falseA is the state after reading 'f'.
func falseA(m *machine, c byte) opcode {
	if c == 'a' {
		m.handler = falseAL
		return opContinue
	}
	return m.fail(c, "in literal false (expecting 'a')")
}

// falseAL is the state after reading "fa".
func falseAL(m *machine, c byte) opcode {
	if c == 'l' {
		m.handler = falseALS
		return opContinue
	}
	return m.fail(c, "in literal false (expecting 'l')")
}

// falseALS is the state after reading "fal".
func falseALS(m *machine, c byte) opcode {
	if c == 's' {
		m.handler = falseALSE
		return opContinue
	}
	return m.fail(c, "in literal false (expecting 's')")
}

// falseALSE is the state after reading "fals".
func falseALSE(m *machine, c byte) opcode {
	if c == 'e' {
		m.handler = afterValue
		return opContinue
	}
	return m.fail(c, "in literal false (expecting 'e')")
}

// nullU is the state after reading 'n'.
func nullU(m *machine, c byte) opcode {
	if c == 'u' {
		m.handler = nullUL
		return opContinue
	}
	return m.fail(c, "in literal null (expecting 'u')")
}

// nullUL is the state after reading "nu".
func nullUL(m *machine, c byte) opcode {
	if c == 'l' {
		m.handler = nullULL
		return opContinue
	}
	return m.fail(c, "in literal null (expecting 'l')")
}

// nullULL is the state after reading "nul".
func nullULL(m *machine, c byte) opcode {
	if c == 'l' {
		m.handler = afterValue
		return opContinue
	}
	return m.fail(c, "in literal null (expecting 'l')")
}

// numberSign is the state after reading '-' in a number.
func numberSign(m *machine, c byte) opcode {
	if c == '0' {
		m.handler = numberZero
		return opContinue
	}
	if '1' <= c && c <= '9' {
		m.handler = numberDigits
		return opContinue
	}
	return m.fail(c, "in numeric literal")
}

// numberZero is the state after reading '0', which may not be followed by
// another digit.
func numberZero(m *machine, c byte) opcode {
	if isDigit(c) {
		return m.fail(c, "in numeric literal")
	}
	return extendNumber(m, c)
}

// numberDigits is the state after the first digit of a non-zero integer part.
func numberDigits(m *machine, c byte) opcode {
	if isDigit(c) {
		m.handler = numberDigits
		return opContinue
	}
	return extendNumber(m, c)
}

// numberPoint is the state after reading '.' in a number.
func numberPoint(m *machine, c byte) opcode {
	if isDigit(c) {
		m.handler = numberFraction
		return opContinue
	}
	return m.fail(c, "after decimal point in numeric literal")
}

// numberFraction is the state after the first fraction digit.
func numberFraction(m *machine, c byte) opcode {
	if isDigit(c) {
		m.handler = numberFraction
		return opContinue
	}
	if c == 'e' || c == 'E' {
		m.handler = numberExponent
		return opContinue
	}
	return finishNumber(m, c)
}

// numberExponent is the state after reading 'e' or 'E' in a number.
func numberExponent(m *machine, c byte) opcode {
	if c == '+' || c == '-' {
		m.handler = numberExponentSign
		return opContinue
	}
	if isDigit(c) {
		m.handler = numberExponentDigits
		return opContinue
	}
	return m.fail(c, "in exponent of numeric literal")
}

// numberExponentSign is the state after an exponent sign.
func numberExponentSign(m *machine, c byte) opcode {
	if isDigit(c) {
		m.handler = numberExponentDigits
		return opContinue
	}
	return m.fail(c, "in exponent of numeric literal")
}

// numberExponentDigits is the state after the first exponent digit.
func numberExponentDigits(m *machine, c byte) opcode {
	if isDigit(c) {
		m.handler = numberExponentDigits
		return opContinue
	}
	return finishNumber(m, c)
}

// extendNumber extends a valid integer part with an optional fraction or
// exponent, or treats c as the delimiter that follows the completed number.
//
// The JSON grammar allows at most one fraction and one exponent, so the
// fraction and exponent states call finishNumber instead: a second '.' or
// exponent marker is rejected by afterValue.
func extendNumber(m *machine, c byte) opcode {
	if c == '.' {
		m.handler = numberPoint
		return opContinue
	}
	if c == 'e' || c == 'E' {
		m.handler = numberExponent
		return opContinue
	}
	return finishNumber(m, c)
}

// finishNumber treats c as the delimiter that follows a complete number and
// lets afterValue process it.
func finishNumber(m *machine, c byte) opcode {
	m.handler = afterValue
	return afterValue(m, c)
}
