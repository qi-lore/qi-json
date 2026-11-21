package qijson

import (
	"errors"
	"math"
	"strconv"
	"unicode/utf8"
)

// Writer builds a JSON document in memory. The zero value is ready to use; call
// NewWriter only for a fresh, explicitly allocated writer. Write one JSON value
// with the Write* methods, framing objects and arrays with Begin/End and naming
// object members with Key:
//
//	var w qijson.Writer
//	w.BeginObject()
//	w.Key("name")
//	w.WriteString("qi-json")
//	w.Key("port")
//	w.WriteInt(8080)
//	w.EndObject()
//	data := w.Bytes()
//
// A mistake (a Key outside an object, an unbalanced End, a second top-level
// value) sets a sticky error: the remaining calls are no-ops and Bytes returns
// nil. Read the failure with Err. Encoding into a Writer never uses reflection.
type Writer struct {
	buf []byte
	err error

	// stack holds one frame per open container; the top frame counts the
	// members or elements written so far.
	stack []writerFrame

	// pendingKey is set between a Key and the value that must follow it.
	pendingKey bool

	// wroteRoot records that the single top-level value has been written.
	wroteRoot bool
}

// writerFrame is one open object or array.
type writerFrame struct {
	object bool
	count  int
}

// Errors reported for misuse of the Writer API. They are programming errors, not
// properties of input, so they are plain errors rather than *qijson.Error.
var (
	errKeyOutsideObject = errors.New("qijson: Key called outside an object")
	errKeyWithoutValue  = errors.New("qijson: object member has a Key but no value")
	errValueWithoutKey  = errors.New("qijson: object member needs a Key before its value")
	errEndWithoutBegin  = errors.New("qijson: End called without a matching Begin")
	errMultipleRoots    = errors.New("qijson: multiple top-level values")
	errNonFinite        = errors.New("qijson: cannot encode a non-finite number")
	errIncomplete       = errors.New("qijson: encoder did not write one complete JSON value")
)

// NewWriter returns a new, empty Writer.
func NewWriter() *Writer { return &Writer{} }

// Reset clears the buffer and any error so the Writer can be reused. It keeps
// the allocated buffer and frame stack.
func (w *Writer) Reset() {
	w.buf = w.buf[:0]
	w.err = nil
	w.stack = w.stack[:0]
	w.pendingKey = false
	w.wroteRoot = false
}

// Bytes returns the JSON written so far, or nil if the Writer has failed. The
// result is valid while the Writer is not used again.
func (w *Writer) Bytes() []byte {
	if w.err != nil {
		return nil
	}
	return w.buf
}

// Err returns the first error the Writer hit, or nil.
func (w *Writer) Err() error { return w.err }

// complete reports whether w holds exactly one complete top-level JSON value:
// the single root value has been written and every container it opened has been
// closed. It is how Marshal rejects an Encoder that returned nil without writing
// a whole value.
func (w *Writer) complete() bool {
	return w.err == nil && w.wroteRoot && len(w.stack) == 0
}

// fail records err as the sticky error, unless one is already set, and returns
// the sticky error.
func (w *Writer) fail(err error) error {
	if w.err == nil {
		w.err = err
	}
	return w.err
}

// beginValue writes the separator a value needs in its container and validates
// the position. A value inside an object must follow a Key.
func (w *Writer) beginValue() error {
	if w.err != nil {
		return w.err
	}
	if len(w.stack) == 0 {
		if w.wroteRoot {
			return w.fail(errMultipleRoots)
		}
		return nil
	}
	top := &w.stack[len(w.stack)-1]
	if top.object {
		if !w.pendingKey {
			return w.fail(errValueWithoutKey)
		}
		w.pendingKey = false
		return nil
	}
	if top.count > 0 {
		w.buf = append(w.buf, ',')
	}
	return nil
}

// endValue records that a value was written in its container.
func (w *Writer) endValue() {
	if len(w.stack) == 0 {
		w.wroteRoot = true
		return
	}
	w.stack[len(w.stack)-1].count++
}

// WriteNull writes a JSON null.
func (w *Writer) WriteNull() error {
	if err := w.beginValue(); err != nil {
		return err
	}
	w.buf = append(w.buf, "null"...)
	w.endValue()
	return nil
}

// WriteBool writes a JSON boolean.
func (w *Writer) WriteBool(v bool) error {
	if err := w.beginValue(); err != nil {
		return err
	}
	if v {
		w.buf = append(w.buf, "true"...)
	} else {
		w.buf = append(w.buf, "false"...)
	}
	w.endValue()
	return nil
}

// WriteString writes a JSON string. It escapes the characters JSON requires and
// leaves everything else, including multi-byte UTF-8, as-is. An invalid UTF-8
// byte is replaced with U+FFFD, so encoding never fails on a string. Unlike
// encoding/json it does not escape '<', '>', or '&'.
func (w *Writer) WriteString(v string) error {
	if err := w.beginValue(); err != nil {
		return err
	}
	w.buf = appendQuoted(w.buf, v)
	w.endValue()
	return nil
}

// WriteInt writes a JSON number from an int64.
func (w *Writer) WriteInt(v int64) error {
	if err := w.beginValue(); err != nil {
		return err
	}
	w.buf = strconv.AppendInt(w.buf, v, 10)
	w.endValue()
	return nil
}

// WriteUint writes a JSON number from a uint64.
func (w *Writer) WriteUint(v uint64) error {
	if err := w.beginValue(); err != nil {
		return err
	}
	w.buf = strconv.AppendUint(w.buf, v, 10)
	w.endValue()
	return nil
}

// WriteFloat writes a JSON number from a float64, using the shortest form that
// round-trips. A NaN or infinite value has no JSON representation and is an
// error.
func (w *Writer) WriteFloat(v float64) error {
	if w.err != nil {
		return w.err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return w.fail(errNonFinite)
	}
	if err := w.beginValue(); err != nil {
		return err
	}
	w.buf = strconv.AppendFloat(w.buf, v, 'g', -1, 64)
	w.endValue()
	return nil
}

// WriteRaw writes raw JSON bytes verbatim, without validation. It is used for a
// value that was already known to be valid JSON, such as the bytes a Raw field
// captured while decoding.
func (w *Writer) WriteRaw(v []byte) error {
	if err := w.beginValue(); err != nil {
		return err
	}
	w.buf = append(w.buf, v...)
	w.endValue()
	return nil
}

// BeginObject starts a JSON object. End it with EndObject, writing each member
// as Key followed by a value.
func (w *Writer) BeginObject() error {
	if err := w.beginValue(); err != nil {
		return err
	}
	w.buf = append(w.buf, '{')
	w.endValue()
	w.stack = append(w.stack, writerFrame{object: true})
	return nil
}

// EndObject closes the innermost object.
func (w *Writer) EndObject() error {
	if w.err != nil {
		return w.err
	}
	if len(w.stack) == 0 || !w.stack[len(w.stack)-1].object {
		return w.fail(errEndWithoutBegin)
	}
	if w.pendingKey {
		return w.fail(errKeyWithoutValue)
	}
	w.stack = w.stack[:len(w.stack)-1]
	w.buf = append(w.buf, '}')
	return nil
}

// BeginArray starts a JSON array. End it with EndArray.
func (w *Writer) BeginArray() error {
	if err := w.beginValue(); err != nil {
		return err
	}
	w.buf = append(w.buf, '[')
	w.endValue()
	w.stack = append(w.stack, writerFrame{object: false})
	return nil
}

// EndArray closes the innermost array.
func (w *Writer) EndArray() error {
	if w.err != nil {
		return w.err
	}
	if len(w.stack) == 0 || w.stack[len(w.stack)-1].object {
		return w.fail(errEndWithoutBegin)
	}
	w.stack = w.stack[:len(w.stack)-1]
	w.buf = append(w.buf, ']')
	return nil
}

// Key writes an object member name and the ':' that must precede its value. It
// must be called inside an object, immediately before the member's value.
func (w *Writer) Key(name string) error {
	if w.err != nil {
		return w.err
	}
	if len(w.stack) == 0 || !w.stack[len(w.stack)-1].object {
		return w.fail(errKeyOutsideObject)
	}
	if w.pendingKey {
		return w.fail(errKeyWithoutValue)
	}
	top := &w.stack[len(w.stack)-1]
	if top.count > 0 {
		w.buf = append(w.buf, ',')
	}
	w.buf = appendQuoted(w.buf, name)
	w.buf = append(w.buf, ':')
	w.pendingKey = true
	return nil
}

// hexDigits is the lower-case hex alphabet used for \u escapes.
const hexDigits = "0123456789abcdef"

// appendQuoted appends s as a JSON string literal. It escapes '"', '\\' and the
// control bytes using the short escapes where JSON defines one, and replaces
// each invalid UTF-8 byte with U+FFFD. It does not HTML-escape.
func appendQuoted(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		b := s[i]
		if b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			dst = appendEscapedByte(dst, b)
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, "\uFFFD"...)
			i++
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// appendEscapedByte appends one ASCII byte that must be escaped.
func appendEscapedByte(dst []byte, b byte) []byte {
	switch b {
	case '"':
		return append(dst, '\\', '"')
	case '\\':
		return append(dst, '\\', '\\')
	case '\n':
		return append(dst, '\\', 'n')
	case '\r':
		return append(dst, '\\', 'r')
	case '\t':
		return append(dst, '\\', 't')
	case '\b':
		return append(dst, '\\', 'b')
	case '\f':
		return append(dst, '\\', 'f')
	default:
		return append(dst, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xf])
	}
}
