package scanner

import (
	"errors"
	"fmt"
	"io"
)

// Kind identifies the JSON token a Tokenizer is positioned on.
type Kind uint8

const (
	// KindNull is the null literal.
	KindNull Kind = iota

	// KindBool is the true or false literal.
	KindBool

	// KindString is a string value or an object key.
	KindString

	// KindNumber is a number literal.
	KindNumber

	// KindObject is an object value, introduced by '{'.
	KindObject

	// KindArray is an array value, introduced by '['.
	KindArray

	// KindEnd is reported by Peek at the closing '}' or ']' of the current
	// object or array, and at the end of the document once the top-level value
	// and any trailing whitespace have been consumed.
	KindEnd
)

// String returns a short description of the kind, used in diagnostics.
func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindBool:
		return "boolean"
	case KindString:
		return "string"
	case KindNumber:
		return "number"
	case KindObject:
		return "object"
	case KindArray:
		return "array"
	case KindEnd:
		return "end of container"
	default:
		return "unknown"
	}
}

// Number is a JSON number literal, held as its raw bytes exactly as they appear
// in the input. Token returns a Number rather than a float64 so the literal is
// preserved with full precision; convert it with strconv when a numeric value is
// needed.
type Number string

// Delim is a JSON delimiter: one of '{', '}', '[' or ']'. Token returns it to
// mark the opening and closing of an object or array.
type Delim rune

// Token is one token of a JSON document, as returned by Tokenizer.Token: nil for
// null, a bool for true or false, a Number for a number, a string for a string
// value or object key, and a Delim for an object or array delimiter.
type Token any

// tokenizer holds the state shared by the Tokenizer methods. It carries its own
// frame stack so it never collides with the validator's nesting stack, but
// reuses the validator's data slice, scalar scanners and error contract.
type tokenizer struct {
	// v shares data and the scalar scanners with the validator. Only data,
	// failure and the scalar scanners are used; run is not.
	v validator

	// i is the index of the next byte to read. It always points just past the
	// last byte consumed by a read method, before any whitespace.
	i int

	// depth is the number of open containers.
	depth int

	// read records that the top-level value has been consumed, so any further
	// non-whitespace input is an "after top-level value" error.
	read bool

	// src is the refillable reader source for a NewDecoder tokenizer. It is nil
	// for a NewTokenizer tokenizer, whose input is the fixed slice in v.data.
	src *readerSource

	// inline holds the first nestingInline frames; overflow holds deeper ones.
	inline   [nestingInline]tokenFrame
	overflow []tokenFrame
}

// tokenFrame is one open container's state.
type tokenFrame struct {
	// kind is containerObject or containerArray.
	kind container

	// count is the number of members (objects) or elements (arrays) completed.
	count int

	// wantKey reports that an object is positioned at a member key rather than
	// a value. It is always false for arrays.
	wantKey bool

	// sep reports that a value has just completed, so the next token may be
	// preceded by a ',' separator.
	sep bool
}

// Tokenizer turns a JSON document into a stream of typed tokens. Construct one
// with NewTokenizer for a byte slice or NewDecoder for an io.Reader; the zero
// value is not ready to use.
//
// The methods are pull-based: Peek reports the kind of the next value token
// without consuming it, and the Read*/Begin*/End* methods consume it.
// Whitespace, commas and the ':' after an object key are consumed
// automatically, so inside an object the stream is key, value, key, value, ...
// and Peek reports KindEnd at the closing brace. Inside an array it is value,
// value, ... and Peek reports KindEnd at the closing bracket. At the top level a
// NewTokenizer tokenizer reports KindEnd once the single top-level value and its
// trailing whitespace are consumed, while a NewDecoder tokenizer reads
// successive top-level values from the stream and reports KindEnd at the end of
// the input.
//
// Byte slices returned by ReadString, ReadNumber and ReadRaw alias the input
// passed to NewTokenizer and must not be modified.
//
// A Tokenizer must not be copied after first use.
type Tokenizer struct {
	state tokenizer
}

// NewTokenizer returns a Tokenizer positioned at the start of data. The
// Tokenizer reads from data without copying: the caller must keep data alive and
// unmodified for the Tokenizer's lifetime, and every byte slice a read returns
// aliases data. A Tokenizer is not safe for concurrent use.
func NewTokenizer(data []byte) *Tokenizer {
	t := &Tokenizer{}
	t.state.v.data = data
	return t
}

// readChunk is the number of bytes a reader-backed Tokenizer asks the io.Reader
// for on each refill.
const readChunk = 4096

// errNilReader is reported by a Tokenizer built from a nil io.Reader.
var errNilReader = errors.New("qijson: nil io.Reader")

// readerSource feeds a Tokenizer from an io.Reader, refilling the Tokenizer's
// input buffer on demand so a value may straddle a read boundary and the reader
// may return short reads. It is not safe for concurrent use.
type readerSource struct {
	// r is the underlying reader. It is nil only when the caller passed nil,
	// in which case err is set and no read is attempted.
	r io.Reader

	// scratch is the destination of each Read.
	scratch []byte

	// eof records that r returned io.EOF; no further bytes will arrive.
	eof bool

	// err is the first non-EOF read error, wrapped with context. It is sticky.
	err error
}

// refill reads one chunk from the reader and appends it to dst. It returns the
// extended slice and whether any bytes were appended. Once the reader is
// exhausted, or after a read error, it returns dst unchanged and false.
func (s *readerSource) refill(dst []byte) ([]byte, bool) {
	if s.r == nil || s.eof || s.err != nil {
		return dst, false
	}
	if s.scratch == nil {
		s.scratch = make([]byte, readChunk)
	}

	for {
		n, err := s.r.Read(s.scratch)
		if n > 0 {
			dst = append(dst, s.scratch[:n]...)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.eof = true
			} else {
				s.err = fmt.Errorf("qijson: reading JSON input: %w", err)
			}
			return dst, n > 0
		}
		if n > 0 {
			return dst, true
		}
		// A zero-byte read with no error: retry, as io.ReadAll does.
	}
}

// NewDecoder returns a Tokenizer that reads one or more JSON documents from r.
// The Tokenizer pulls bytes from r on demand, refilling its internal buffer as
// it goes, so a value may straddle a read boundary and r may return short reads.
//
// Once a top-level value is complete, a following value that can begin here
// starts the next one, so several values may be read from one stream (for
// example newline-delimited JSON). Peek reports KindEnd at the end of the input;
// a byte that cannot begin a value after a completed value is an "after
// top-level value" error.
//
// A read error other than io.EOF is reported, wrapped, as the Tokenizer's
// sticky error; a nil r is reported as a nil-reader error rather than
// panicking.
//
// Byte slices returned by ReadString, ReadNumber and ReadRaw alias the
// Tokenizer's internal buffer, which grows as r is drained; a returned slice is
// valid only until the next read method, which may append to that buffer and
// move it. A Tokenizer is not safe for concurrent use.
func NewDecoder(r io.Reader) *Tokenizer {
	t := &Tokenizer{}
	t.state.src = &readerSource{r: r}
	if r == nil {
		t.state.src.err = errNilReader
	}
	return t
}

// Peek returns the kind of the next value token without consuming it. At the
// end of the current object, array or document it returns KindEnd.
func (t *Tokenizer) Peek() (Kind, error) { return t.state.peek() }

// ReadNull consumes a null literal.
func (t *Tokenizer) ReadNull() error { return t.state.readNull() }

// ReadBool consumes a true or false literal and returns its value.
func (t *Tokenizer) ReadBool() (bool, error) { return t.state.readBool() }

// ReadString consumes a string value or object key and returns the bytes
// between the quotes. Escape sequences are left intact; only the surrounding
// quotes are removed.
func (t *Tokenizer) ReadString() ([]byte, error) { return t.state.readString() }

// ReadNumber consumes a number literal and returns its raw bytes.
func (t *Tokenizer) ReadNumber() ([]byte, error) { return t.state.readNumber() }

// ReadRaw consumes the next value, composites included, and returns its raw
// bytes exactly as they appear in the input.
func (t *Tokenizer) ReadRaw() ([]byte, error) { return t.state.readRaw() }

// Token returns the next token of the stream: nil for null, a bool for true or
// false, a Number for a number, a string for a string value or an object key,
// and a Delim ('{', '}', '[' or ']') for a container delimiter. It returns
// io.EOF once the whole stream has been consumed, and otherwise the Tokenizer's
// sticky error.
//
// Token consumes the separators itself: the commas between members and elements
// and the ':' after an object key are never returned, so an object yields its
// opening Delim, then key and value tokens in order, then its closing Delim. A
// string value and an object key are both returned as a string; their position
// in the stream tells them apart.
//
// A returned string is the bytes between the quotes and a returned Number is the
// raw number literal: escape sequences are left intact and are not unescaped.
// Unlike ReadString and ReadNumber, which return a []byte that aliases the input,
// Token returns a string or Number value; the conversion copies the bytes, so a
// returned token is independent of the Tokenizer and safe to retain.
func (t *Tokenizer) Token() (Token, error) { return t.state.token() }

// BeginObject consumes the '{' that starts an object and opens it.
func (t *Tokenizer) BeginObject() error { return t.state.begin(containerObject) }

// EndObject consumes the '}' that closes the current object.
func (t *Tokenizer) EndObject() error { return t.state.end(containerObject) }

// BeginArray consumes the '[' that starts an array and opens it.
func (t *Tokenizer) BeginArray() error { return t.state.begin(containerArray) }

// EndArray consumes the ']' that closes the current array.
func (t *Tokenizer) EndArray() error { return t.state.end(containerArray) }

// Skip consumes and discards the next value, composites included.
func (t *Tokenizer) Skip() error { return t.state.skip() }

// Offset returns the number of bytes consumed so far: the 0-based offset of the
// next byte to read. After NewTokenizer it is 0; after the whole document it is
// len(data).
func (t *Tokenizer) Offset() int64 { return int64(t.state.i) }

// Err returns the first error the Tokenizer encountered, or nil. The error is
// sticky: once set it is never cleared and every later call reports it.
func (t *Tokenizer) Err() error { return t.state.err() }

// peek reports the kind of the next token without consuming it, refilling from
// the reader as needed. It is the only method that interprets the separator
// (',') and the end of a container.
func (st *tokenizer) peek() (Kind, error) {
	for {
		k, err := st.peekOnce()
		if err != nil {
			if e := st.inputErr(); e != nil {
				return 0, e
			}
			if !st.needMore() {
				return 0, err
			}
			st.v.failure = nil
			st.refill()
			continue
		}
		// At the top level a KindEnd may mean only that the buffer is drained:
		// the reader may still hold trailing whitespace or a second value.
		if k == KindEnd && st.depth == 0 && st.read && st.refill() {
			continue
		}
		return k, nil
	}
}

// peekOnce classifies the next token over the bytes already buffered. It never
// refills, so a failure it reports at the buffer end may be a truncation that
// peek resolves by refilling.
func (st *tokenizer) peekOnce() (Kind, error) {
	if st.v.failure != nil {
		return 0, *st.v.failure
	}

	data := st.v.data
	n := len(data)
	i := skipSpace(data, st.i)
	st.i = i

	if st.depth == 0 {
		if st.read {
			if i >= n {
				return KindEnd, nil
			}
			// A reader-backed tokenizer reads several values from one stream
			// (NDJSON): once a top-level value is complete, a following value
			// that can begin here starts the next one. A slice-backed tokenizer
			// holds a single document, and in either case a byte that cannot
			// begin a value is an error.
			if st.src == nil || !canBeginValue(data[i]) {
				st.fail(i, "after top-level value")
				return 0, *st.v.failure
			}
			st.read = false
		}
		if i >= n {
			st.failEnd(n)
			return 0, *st.v.failure
		}
		return st.kindAt(i)
	}

	fr := st.frame(st.depth - 1)
	if fr.kind == containerObject && fr.wantKey {
		if fr.sep {
			if i >= n {
				st.failEnd(n)
				return 0, *st.v.failure
			}
			if data[i] == '}' {
				return KindEnd, nil
			}
			if data[i] != ',' {
				st.fail(i, "after object value")
				return 0, *st.v.failure
			}
			i = skipSpace(data, i+1)
			st.i = i
			fr.sep = false
		}
		if i >= n {
			st.failEnd(n)
			return 0, *st.v.failure
		}
		if data[i] == '"' {
			return KindString, nil
		}
		if data[i] == '}' && fr.count == 0 {
			return KindEnd, nil
		}
		st.fail(i, "looking for beginning of object key string")
		return 0, *st.v.failure
	}

	if fr.kind == containerObject {
		// A member value is required; no separator precedes it.
		if i >= n {
			st.failEnd(n)
			return 0, *st.v.failure
		}
		return st.kindAt(i)
	}

	// Array element position.
	if fr.sep {
		if i >= n {
			st.failEnd(n)
			return 0, *st.v.failure
		}
		if data[i] == ']' {
			return KindEnd, nil
		}
		if data[i] != ',' {
			st.fail(i, "after array value")
			return 0, *st.v.failure
		}
		i = skipSpace(data, i+1)
		st.i = i
		fr.sep = false
	}
	if i >= n {
		st.failEnd(n)
		return 0, *st.v.failure
	}
	if data[i] == ']' {
		if fr.count == 0 {
			return KindEnd, nil
		}
		st.fail(i, "looking for beginning of value")
		return 0, *st.v.failure
	}
	return st.kindAt(i)
}

// canBeginValue reports whether c can be the first byte of a JSON value: the
// opening byte of a container, a string quote, the first byte of a literal, or
// the start of a number.
func canBeginValue(c byte) bool {
	switch c {
	case '{', '[', '"', 't', 'f', 'n', '-':
		return true
	default:
		return isDigit(c)
	}
}

// kindAt returns the kind of the value whose first byte is at i, recording
// "looking for beginning of value" when the byte cannot begin one.
func (st *tokenizer) kindAt(i int) (Kind, error) {
	c := st.v.data[i]
	if !canBeginValue(c) {
		st.fail(i, "looking for beginning of value")
		return 0, *st.v.failure
	}
	switch c {
	case '{':
		return KindObject, nil
	case '[':
		return KindArray, nil
	case '"':
		return KindString, nil
	case 't', 'f':
		return KindBool, nil
	case 'n':
		return KindNull, nil
	default: // '-' or a digit
		return KindNumber, nil
	}
}

// begin consumes the opening byte of a container and pushes its frame.
func (st *tokenizer) begin(kind container) error {
	k, err := st.peek()
	if err != nil {
		return err
	}
	want := KindObject
	if kind == containerArray {
		want = KindArray
	}
	if k != want {
		return st.mismatch(k, kindNoun(kind))
	}
	st.i++
	st.push(kind)
	return nil
}

// end consumes the closing byte of the current container.
func (st *tokenizer) end(kind container) error {
	if st.v.failure != nil {
		return *st.v.failure
	}
	if st.depth == 0 || st.frame(st.depth-1).kind != kind {
		return st.misuse("expected " + kindNoun(kind) + " end")
	}

	k, err := st.peek()
	if err != nil {
		return err
	}
	if k != KindEnd {
		return st.mismatch(k, kindNoun(kind)+" end")
	}
	st.i++
	st.depth--
	st.consumedValue()
	return nil
}

// readNull consumes a null literal.
func (st *tokenizer) readNull() error {
	k, err := st.peek()
	if err != nil {
		return err
	}
	if k != KindNull {
		return st.mismatch(k, "null")
	}
	i, err := st.scanLiteralRefill(st.i, "null")
	if err != nil {
		return err
	}
	st.i = i
	st.consumedValue()
	return nil
}

// readBool consumes a true or false literal.
func (st *tokenizer) readBool() (bool, error) {
	k, err := st.peek()
	if err != nil {
		return false, err
	}
	if k != KindBool {
		return false, st.mismatch(k, "boolean")
	}

	lit := "false"
	if st.v.data[st.i] == 't' {
		lit = "true"
	}
	i, err := st.scanLiteralRefill(st.i, lit)
	if err != nil {
		return false, err
	}
	st.i = i
	st.consumedValue()
	return lit == "true", nil
}

// readString consumes a string value or object key and returns its content.
func (st *tokenizer) readString() ([]byte, error) {
	k, err := st.peek()
	if err != nil {
		return nil, err
	}
	if k != KindString {
		return nil, st.mismatch(k, "string")
	}

	key := st.atKey()
	start := st.i
	var end int
	for {
		end = st.v.scanString(start)
		if st.v.failure == nil {
			break
		}
		if e := st.inputErr(); e != nil {
			return nil, e
		}
		if !st.needMore() {
			return nil, *st.v.failure
		}
		st.v.failure = nil
		st.refill()
	}
	st.i = end

	if key {
		// The ':' after a key may not be buffered yet.
		for {
			j := skipSpace(st.v.data, st.i)
			if j < len(st.v.data) {
				if st.v.data[j] != ':' {
					st.fail(j, "after object key")
					return nil, *st.v.failure
				}
				st.i = j + 1
				break
			}
			if e := st.inputErr(); e != nil {
				return nil, e
			}
			if !st.refill() {
				st.failEnd(len(st.v.data))
				return nil, *st.v.failure
			}
		}
		st.frame(st.depth - 1).wantKey = false
	} else {
		st.consumedValue()
	}
	return st.v.data[start+1 : end-1], nil
}

// readNumber consumes a number literal and returns its raw bytes.
func (st *tokenizer) readNumber() ([]byte, error) {
	k, err := st.peek()
	if err != nil {
		return nil, err
	}
	if k != KindNumber {
		return nil, st.mismatch(k, "number")
	}

	start := st.i
	var end int
	for {
		end = st.v.scanNumber(start)
		if st.v.failure != nil {
			if e := st.inputErr(); e != nil {
				return nil, e
			}
			if !st.needMore() {
				return nil, *st.v.failure
			}
			st.v.failure = nil
			st.refill()
			continue
		}
		// A number scanner stops at the buffer end exactly as it would at a
		// delimiter, so a number that reaches the end of the buffered input may
		// continue: refill and rescan until it ends inside the buffer or the
		// reader is exhausted.
		if end < len(st.v.data) || !st.moreInput() {
			break
		}
		if !st.refill() {
			break
		}
	}
	st.i = end
	st.consumedValue()
	return st.v.data[start:end], nil
}

// readRaw consumes the next value, composites included, and returns its bytes.
func (st *tokenizer) readRaw() ([]byte, error) {
	k, err := st.peek()
	if err != nil {
		return nil, err
	}
	if k == KindEnd {
		return nil, st.misuse("expected a value, found end of container")
	}
	start := st.i
	if err := st.skip(); err != nil {
		return nil, err
	}
	return st.v.data[start:st.i], nil
}

// token returns the next token of the stream. It reuses peek for classification
// and the typed reads, Begin* and End* for consumption, so the token stream and
// the Peek/Read* stream stay consistent by construction. It reports io.EOF once
// the top-level value (and, for a reader-backed tokenizer, every following value)
// has been consumed.
func (st *tokenizer) token() (Token, error) {
	k, err := st.peek()
	if err != nil {
		return nil, err
	}

	switch k {
	case KindNull:
		if err := st.readNull(); err != nil {
			return nil, err
		}
		return nil, nil
	case KindBool:
		b, err := st.readBool()
		if err != nil {
			return nil, err
		}
		return b, nil
	case KindString:
		s, err := st.readString()
		if err != nil {
			return nil, err
		}
		return string(s), nil
	case KindNumber:
		n, err := st.readNumber()
		if err != nil {
			return nil, err
		}
		return Number(n), nil
	case KindObject:
		if err := st.begin(containerObject); err != nil {
			return nil, err
		}
		return Delim('{'), nil
	case KindArray:
		if err := st.begin(containerArray); err != nil {
			return nil, err
		}
		return Delim('['), nil
	default: // KindEnd
		if st.depth == 0 {
			return nil, io.EOF
		}
		kind := st.frame(st.depth - 1).kind
		if err := st.end(kind); err != nil {
			return nil, err
		}
		if kind == containerObject {
			return Delim('}'), nil
		}
		return Delim(']'), nil
	}
}

// skip consumes and discards the next value, composites included.
func (st *tokenizer) skip() error {
	if st.atKey() {
		return st.misuse("expected a value, found object key")
	}

	k, err := st.peek()
	if err != nil {
		return err
	}

	switch k {
	case KindNull:
		return st.readNull()
	case KindBool, KindString, KindNumber:
		return st.consumeScalar(k)
	case KindObject, KindArray:
		base := st.depth
		st.i++
		if k == KindObject {
			st.push(containerObject)
		} else {
			st.push(containerArray)
		}
		for st.depth > base {
			kk, err := st.peek()
			if err != nil {
				return err
			}
			if err := st.consumeAny(kk); err != nil {
				return err
			}
		}
		return nil
	default: // KindEnd
		return st.misuse("expected a value, found end of container")
	}
}

// consumeAny consumes one already-peeked token while skipping a container. A
// KindEnd closes the innermost container.
func (st *tokenizer) consumeAny(k Kind) error {
	switch k {
	case KindNull:
		return st.readNull()
	case KindBool, KindString, KindNumber:
		return st.consumeScalar(k)
	case KindObject:
		st.i++
		st.push(containerObject)
		return nil
	case KindArray:
		st.i++
		st.push(containerArray)
		return nil
	default: // KindEnd
		return st.end(st.frame(st.depth - 1).kind)
	}
}

// consumeScalar consumes a scalar token that Peek already classified as k.
func (st *tokenizer) consumeScalar(k Kind) error {
	switch k {
	case KindBool:
		_, err := st.readBool()
		return err
	case KindString:
		_, err := st.readString()
		return err
	default: // KindNumber
		_, err := st.readNumber()
		return err
	}
}

// consumedValue records that a scalar value or a closed container completed: at
// the top level it marks the document's value read; inside an object it
// advances from key to value or from value to the next key; inside an array it
// counts the element.
func (st *tokenizer) consumedValue() {
	if st.depth == 0 {
		st.read = true
		return
	}
	fr := st.frame(st.depth - 1)
	if fr.kind == containerObject {
		if fr.wantKey {
			fr.wantKey = false
			return
		}
		fr.count++
		fr.wantKey = true
		fr.sep = true
		return
	}
	fr.count++
	fr.sep = true
}

// atKey reports whether the next token is an object member key.
func (st *tokenizer) atKey() bool {
	if st.depth == 0 {
		return false
	}
	fr := st.frame(st.depth - 1)
	return fr.kind == containerObject && fr.wantKey
}

// frame returns the frame at depth d.
func (st *tokenizer) frame(d int) *tokenFrame {
	if d < nestingInline {
		return &st.inline[d]
	}
	return &st.overflow[d-nestingInline]
}

// push opens a container.
func (st *tokenizer) push(kind container) {
	if st.depth >= nestingInline {
		deep := st.depth - nestingInline
		switch {
		case st.overflow == nil:
			st.overflow = make([]tokenFrame, nestingInline)
		case deep >= len(st.overflow):
			next := make([]tokenFrame, 2*len(st.overflow))
			copy(next, st.overflow)
			st.overflow = next
		}
	}
	*st.frame(st.depth) = tokenFrame{kind: kind, wantKey: kind == containerObject}
	st.depth++
}

// refill appends more input from the reader, if any. It reports whether the
// buffer grew. A slice-backed Tokenizer (src == nil) never grows.
func (st *tokenizer) refill() bool {
	if st.src == nil {
		return false
	}
	data, added := st.src.refill(st.v.data)
	st.v.data = data
	return added
}

// moreInput reports whether the reader may still supply bytes: a slice-backed
// Tokenizer never does, and an exhausted or failed reader cannot.
func (st *tokenizer) moreInput() bool {
	return st.src != nil && st.src.err == nil && !st.src.eof
}

// needMore reports whether the recorded failure sits at the end of the buffered
// input while more bytes may still arrive, so the failure is a truncation a
// refill can resolve rather than a genuine error.
func (st *tokenizer) needMore() bool {
	return st.moreInput() && st.v.failure != nil && st.v.failure.at == int64(len(st.v.data))
}

// inputErr returns the sticky reader error, if any.
func (st *tokenizer) inputErr() error {
	if st.src == nil {
		return nil
	}
	return st.src.err
}

// scanLiteralRefill scans the literal lit whose first byte is at start,
// refilling from the reader when the literal is cut off by the buffer end.
func (st *tokenizer) scanLiteralRefill(start int, lit string) (int, error) {
	for {
		end := st.v.scanLiteral(start, lit)
		if st.v.failure == nil {
			return end, nil
		}
		if e := st.inputErr(); e != nil {
			return 0, e
		}
		if !st.needMore() {
			return 0, *st.v.failure
		}
		st.v.failure = nil
		st.refill()
	}
}

// err returns the sticky error, if any: a reader error takes precedence over a
// syntax failure, because it explains why the input stopped.
func (st *tokenizer) err() error {
	if st.src != nil && st.src.err != nil {
		return st.src.err
	}
	if st.v.failure == nil {
		return nil
	}
	return *st.v.failure
}

// fail records "invalid character ... <problem>" at data[i], via the validator
// so the message and 1-based position match validate exactly.
func (st *tokenizer) fail(i int, problem string) { st.v.fail(i, problem) }

// failEnd records an end-of-input error at n, matching validate.
func (st *tokenizer) failEnd(n int) { st.v.failEnd(n) }

// failAt records problem verbatim at pos, matching validate's failAt.
func (st *tokenizer) failAt(pos int64, problem string) { st.v.failAt(pos, problem) }

// misuse records a tokenizer misuse error at the current position, through the
// validator so it is sticky like every other error.
func (st *tokenizer) misuse(problem string) error {
	at := int64(st.i)
	if at < int64(len(st.v.data)) {
		at++
	}
	st.failAt(at, problem)
	return *st.v.failure
}

// mismatch records that a token of kind got was found where want was required.
func (st *tokenizer) mismatch(got Kind, want string) error {
	return st.misuse("expected " + want + ", found " + got.String())
}

// kindNoun names the container kind for diagnostics.
func kindNoun(kind container) string {
	if kind == containerObject {
		return "object"
	}
	return "array"
}
