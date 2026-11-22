package scanner

import (
	"encoding/binary"
	"strconv"
)

// nestingInline is the number of container nesting levels held in the caller's
// local array. Documents that nest deeper grow the stack onto the heap.
const nestingInline = 32

// container is the kind of an open container.
type container uint8

const (
	// containerObject is an open '{'.
	containerObject container = iota

	// containerArray is an open '['.
	containerArray
)

// parseError describes the first syntax error found while scanning. It is a
// value type so that validate can return it without making the validator escape
// to the heap, and it implements error so the qijson layer can surface
// the same diagnostic.
type parseError struct {
	// at is the 1-based position of the offending byte. For end-of-input
	// errors it is the number of bytes in the input.
	at int64

	// problem explains the error, for example
	// "invalid character '}' after object value".
	problem string
}

// parseError implements error as part of its contract.
var _ error = parseError{}

// Error implements the error interface.
func (e parseError) Error() string { return e.problem }

// validator scans one JSON document held in data. The zero value is not ready
// to use; stack must hold at least one element.
type validator struct {
	data    []byte
	depth   int
	stack   []container
	failure *parseError
}

// run scans the document. It reports whether the document is valid and records
// the first syntax error in v.failure.
//
// Dispatch is two nested structured loops, not a switch over a position
// variable and not labelled jumps. The outer loop scans one value; the inner
// loop consumes the delimiter that follows a completed value, closes every
// container that ends there, and decides where the next value starts.
//
// The "scan an object key and the ':' that follows" block appears twice: once
// just after '{' and once just after an object ','. That duplication is
// deliberate. A shared helper cannot be inlined (it calls scanString), and the
// call costs roughly 8% on object-heavy documents; inlining keeps the key path
// within ~2% of the retired label-and-goto form. Keep the two copies in step.
func (v *validator) run() bool {
	data := v.data
	n := len(data)
	i := 0

	for {
		// Scan one value. A container is pushed and, when empty, closed at
		// once; otherwise its first element or member is scanned next.
		i = skipSpace(data, i)
		if i >= n {
			return v.failEnd(n)
		}
		switch c := data[i]; {
		case c == '{':
			i++
			v.push(containerObject)
			i = skipSpace(data, i)
			if i >= n {
				return v.failEnd(n)
			}
			if data[i] != '}' {
				// First member key and ':'; see run's note on the copy.
				if data[i] != '"' {
					return v.fail(i, "looking for beginning of object key string")
				}
				i = v.scanString(i)
				if v.failure != nil {
					return false
				}
				i = skipSpace(data, i)
				if i >= n {
					return v.failEnd(n)
				}
				if data[i] != ':' {
					return v.fail(i, "after object key")
				}
				i++
				continue
			}
			i++
			v.pop()
		case c == '[':
			i++
			v.push(containerArray)
			i = skipSpace(data, i)
			if i >= n {
				return v.failEnd(n)
			}
			if data[i] != ']' {
				continue
			}
			i++
			v.pop()
		case c == '"':
			i = v.scanString(i)
			if v.failure != nil {
				return false
			}
		case c == 't':
			i = v.scanLiteral(i, "true")
			if v.failure != nil {
				return false
			}
		case c == 'f':
			i = v.scanLiteral(i, "false")
			if v.failure != nil {
				return false
			}
		case c == 'n':
			i = v.scanLiteral(i, "null")
			if v.failure != nil {
				return false
			}
		case c == '-' || isDigit(c):
			i = v.scanNumber(i)
			if v.failure != nil {
				return false
			}
		default:
			return v.fail(i, "looking for beginning of value")
		}

		// A value is complete. Close every container that ends here and find
		// where the next value starts, or finish at the top level.
		for {
			if v.depth == 0 {
				i = skipSpace(data, i)
				if i < n {
					return v.fail(i, "after top-level value")
				}
				return true
			}
			i = skipSpace(data, i)
			if i >= n {
				return v.failEnd(n)
			}
			if v.top() == containerObject {
				if data[i] == ',' {
					i++
					i = skipSpace(data, i)
					if i >= n {
						return v.failEnd(n)
					}
					// Member key and ':'; see run's note on the copy.
					if data[i] != '"' {
						return v.fail(i, "looking for beginning of object key string")
					}
					i = v.scanString(i)
					if v.failure != nil {
						return false
					}
					i = skipSpace(data, i)
					if i >= n {
						return v.failEnd(n)
					}
					if data[i] != ':' {
						return v.fail(i, "after object key")
					}
					i++
					break
				}
				if data[i] == '}' {
					i++
					v.pop()
					continue
				}
				return v.fail(i, "after object value")
			}
			if data[i] == ',' {
				i++
				break
			}
			if data[i] == ']' {
				i++
				v.pop()
				continue
			}
			return v.fail(i, "after array value")
		}
	}
}

// push opens a container.
func (v *validator) push(c container) {
	if v.depth == len(v.stack) {
		v.grow()
	}
	v.stack[v.depth] = c
	v.depth++
}

// pop closes the innermost container.
func (v *validator) pop() {
	v.depth--
}

// top returns the innermost open container. It must not be called when no
// container is open.
func (v *validator) top() container {
	return v.stack[v.depth-1]
}

// grow doubles the nesting stack, or restores it to nestingInline when it was
// empty. The new stack is on the heap; the old one is dropped.
func (v *validator) grow() {
	size := 2 * len(v.stack)
	if size < nestingInline {
		size = nestingInline
	}
	next := make([]container, size)
	copy(next, v.stack[:v.depth])
	v.stack = next
}

// fail records a syntax error at the 1-based position of data[i]. Only the first
// error is kept, and the return value is always false.
func (v *validator) fail(i int, problem string) bool {
	if v.failure == nil {
		v.failure = &parseError{
			at:      int64(i) + 1,
			problem: "invalid character " + quoteByte(v.data[i]) + " " + problem,
		}
	}
	return false
}

// failAt records a syntax error with an explicit position and message. Only the
// first error is kept, and the return value is always false.
func (v *validator) failAt(at int64, problem string) bool {
	if v.failure == nil {
		v.failure = &parseError{at: at, problem: problem}
	}
	return false
}

// failEnd records an error for input that ended inside a value. The position is
// the number of bytes in the input.
func (v *validator) failEnd(n int) bool {
	return v.failAt(int64(n), "unexpected end of JSON input")
}

// spaceTable marks the four JSON whitespace bytes. A table lookup keeps the
// whitespace skip to one load and one branch per byte, and it is inlined at
// every call site in run.
var spaceTable = buildSpaceTable()

// buildSpaceTable fills the spaceTable.
func buildSpaceTable() [256]bool {
	var table [256]bool
	table[' '] = true
	table['\t'] = true
	table['\n'] = true
	table['\r'] = true
	return table
}

// skipSpace returns the index of the first byte at or after from that is not
// JSON whitespace, or len(data) when only whitespace remains.
//
// The loop is deliberately byte at a time. A word-at-a-time (eight-byte) skip
// was measured against it on pretty-printed input with runs of 4, 16 and 64
// spaces and on documents whose only whitespace runs were newline plus sixteen
// spaces: it lost 9-22% every time. Inlined at each of the run loop's call
// sites the word loop bloats `run`, and the byte loop is already cheap enough
// that the extra work per call outweighs the bytes skipped.
func skipSpace(data []byte, from int) int {
	for from < len(data) && spaceTable[data[from]] {
		from++
	}
	return from
}

// stringStop marks the bytes that end a run of plain string content: the
// closing quote, the escape introducer, and the control bytes JSON forbids. A
// table lookup keeps the string body scan to one load and one branch per byte.
var stringStop = buildStringStop()

// buildStringStop fills the stringStop table.
func buildStringStop() [256]bool {
	var table [256]bool
	for c := 0; c < 0x20; c++ {
		table[c] = true
	}
	table['"'] = true
	table['\\'] = true
	return table
}

// stringStopWord reports whether w contains a byte that ends a plain string run:
// a quote, a backslash, or a control byte below 0x20. The constants are byte
// replicated, so endianness does not matter.
func stringStopWord(w uint64) bool {
	const (
		quote = 0x2222222222222222
		slash = 0x5c5c5c5c5c5c5c5c
		ctl   = 0xe0e0e0e0e0e0e0e0
		one   = 0x0101010101010101
		high  = 0x8080808080808080
	)
	// A byte is zero when (v-one)&^v&high is non-zero for that byte.
	zero := func(v uint64) bool { return (v-one)&^v&high != 0 }
	return zero(w^quote) || zero(w^slash) || zero(w&ctl)
}

// scanString scans the string whose opening quote is at i and returns the
// position just past the closing quote. On failure it records the error and
// returns len(data).
func (v *validator) scanString(i int) int {
	data := v.data
	n := len(data)

	// Keys and short values end within a few bytes. A string that reaches an
	// escape is long, so its remaining runs scan words without the byte warm-up.
	long := false
	for i++; ; {
		if long {
			for i+8 <= n && !stringStopWord(binary.LittleEndian.Uint64(data[i:])) {
				i += 8
			}
			for i < n && !stringStop[data[i]] {
				i++
			}
		} else {
			short := i + 8
			if short > n {
				short = n
			}
			for i < short && !stringStop[data[i]] {
				i++
			}
			if i == short {
				for i+8 <= n && !stringStopWord(binary.LittleEndian.Uint64(data[i:])) {
					i += 8
				}
				for i < n && !stringStop[data[i]] {
					i++
				}
			}
		}
		if i >= n {
			v.failEnd(n)
			return n
		}
		switch data[i] {
		case '"':
			return i + 1
		case '\\':
			i = v.scanEscape(i)
			if v.failure != nil {
				return n
			}
			long = true
		default:
			v.fail(i, "in string literal")
			return n
		}
	}
}

// scanEscape scans the escape whose backslash is at i. It returns the index just
// past the escape, or len(data) after recording a failure.
func (v *validator) scanEscape(i int) int {
	n := len(v.data)

	i++ // the backslash
	if i >= n {
		v.failAt(int64(n), "in string escape code")
		return n
	}

	switch v.data[i] {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		return i + 1
	case 'u':
		for k := 1; k <= 4; k++ {
			if i+k >= n {
				v.failAt(int64(n), `in \u hexadecimal character escape`)
				return n
			}
			if !isHexDigit(v.data[i+k]) {
				v.fail(i+k, `in \u hexadecimal character escape`)
				return n
			}
		}
		return i + 5
	default:
		v.fail(i, "in string escape code")
		return n
	}
}

// scanLiteral scans the literal lit whose first byte is at i. It returns the
// index just past the literal, or len(data) after recording a failure.
func (v *validator) scanLiteral(i int, lit string) int {
	n := len(v.data)

	for k := 1; k < len(lit); k++ {
		if i+k >= n {
			v.failAt(int64(n), "in literal "+lit+" (expecting '"+lit[k:k+1]+"')")
			return n
		}
		if v.data[i+k] != lit[k] {
			v.fail(i+k, "in literal "+lit+" (expecting '"+lit[k:k+1]+"')")
			return n
		}
	}
	return i + len(lit)
}

// scanNumber scans the number whose first byte is at i. It returns the index of
// the first byte that cannot extend the number, or len(data) after recording a
// failure. The delimiter is never consumed: the caller decides what follows.
func (v *validator) scanNumber(i int) int {
	data := v.data
	n := len(data)

	if data[i] == '-' {
		i++
		if i >= n {
			v.failAt(int64(n), "in numeric literal")
			return n
		}
	}

	if data[i] == '0' {
		i++
		if i < n && isDigit(data[i]) {
			v.fail(i, "in numeric literal")
			return n
		}
	} else if isDigit(data[i]) {
		for i++; i < n && isDigit(data[i]); i++ {
		}
	} else {
		v.fail(i, "in numeric literal")
		return n
	}

	if i < n && data[i] == '.' {
		i++
		if i >= n {
			v.failAt(int64(n), "after decimal point in numeric literal")
			return n
		}
		if !isDigit(data[i]) {
			v.fail(i, "after decimal point in numeric literal")
			return n
		}
		for i++; i < n && isDigit(data[i]); i++ {
		}
	}

	if i < n && (data[i] == 'e' || data[i] == 'E') {
		i++
		if i < n && (data[i] == '+' || data[i] == '-') {
			i++
		}
		if i >= n {
			v.failAt(int64(n), "in exponent of numeric literal")
			return n
		}
		if !isDigit(data[i]) {
			v.fail(i, "in exponent of numeric literal")
			return n
		}
		for i++; i < n && isDigit(data[i]); i++ {
		}
	}

	return i
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
