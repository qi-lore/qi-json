package codec

import (
	"errors"
	"io"
	"math"
	"strconv"
	"sync"
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
// nil. Read the failure with Err. Encoding into a Writer never uses reflection. A
// Writer is not safe for concurrent use; give each goroutine its own.
//
// By default the document is compact and HTML bytes are not escaped. SetIndent
// selects an indented form and SetEscapeHTML turns on HTML escaping; both are
// sticky across Reset.
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

	// prefix and indent configure pretty-printing: prefix begins each new line
	// and indent is repeated once per level of nesting. Indentation is off when
	// both are empty, which is the default. They persist across Reset.
	prefix string
	indent string

	// escapeHTML makes WriteString and Key escape the HTML-sensitive bytes.
	// It is off by default. It persists across Reset.
	escapeHTML bool
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
	errEmptyRaw         = errors.New("qijson: WriteRaw called with an empty value")
	errIncomplete       = errors.New("qijson: encoder did not write one complete JSON value")
	errNilDumpTarget    = errors.New("qijson: nil dump target")
)

// Writer buffers are recycled through a pool so a short-lived encode can start
// from a warmed allocation instead of growing a slice from nil. pooledBufCap
// sizes a fresh buffer for a typical record; pooledBufMax caps what the pool
// retains, so one large document does not pin memory there.
const (
	pooledBufCap = 1024
	pooledBufMax = 64 << 10
)

// bufPool recycles the byte slices behind Writers. It stores *[]byte so a
// Get/Put round-trip does not box a slice header.
var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, pooledBufCap)
		return &b
	},
}

// acquireBuf returns an empty buffer taken from the pool.
func acquireBuf() []byte { return (*bufPool.Get().(*[]byte))[:0] }

// releaseBuf returns b to the pool unless it is empty or grew past pooledBufMax.
// The caller must not use b afterwards.
func releaseBuf(b []byte) {
	if cap(b) == 0 || cap(b) > pooledBufMax {
		return
	}
	b = b[:0]
	bufPool.Put(&b)
}

// newPooledWriter returns an empty Writer whose buffer comes from the pool. Call
// release when done so the buffer returns for reuse.
func newPooledWriter() *Writer { return &Writer{buf: acquireBuf()} }

// release returns the Writer's buffer to the pool and clears the Writer. Call it
// only after the buffer's contents have been copied out, because the buffer may
// be handed to another Writer.
func (w *Writer) release() {
	buf := w.buf
	w.Reset()
	w.buf = nil
	releaseBuf(buf)
}

// NewWriter returns a new, empty Writer.
func NewWriter() *Writer { return &Writer{} }

// Reset clears the buffer and any error so the Writer can be reused. It keeps
// the allocated buffer and frame stack, and it keeps the indentation and HTML
// escaping configuration set by SetIndent and SetEscapeHTML.
func (w *Writer) Reset() {
	w.buf = w.buf[:0]
	w.err = nil
	w.stack = w.stack[:0]
	w.pendingKey = false
	w.wroteRoot = false
}

// SetIndent configures the Writer to emit an indented document. prefix begins
// every new line and indent is repeated once per level of nesting, so
// SetIndent("", "  ") indents each level by two spaces. An empty object or array
// stays on one line as "{}" or "[]", a ':' is followed by one space, and no
// newline follows the final token. SetIndent("", "") disables indentation and
// restores the compact default.
//
// The setting is sticky: it survives Reset and the implicit reset that DumpTo and
// ReadCloser perform, so one Writer keeps its style document after document. Only
// another SetIndent call changes it. SetIndent must not be called concurrently
// with any other Writer method.
func (w *Writer) SetIndent(prefix, indent string) {
	w.prefix = prefix
	w.indent = indent
}

// SetEscapeHTML controls whether WriteString and Key escape the HTML-sensitive
// bytes: '<', '>', '&' and the characters U+2028 and U+2029 become \u escapes.
// The default is off, so those bytes pass through literally; turning it on matches
// the escaping encoding/json applies when a document is embedded in a web page.
//
// The setting is sticky: it survives Reset and the implicit reset that DumpTo and
// ReadCloser perform. Only another SetEscapeHTML call changes it. SetEscapeHTML
// must not be called concurrently with any other Writer method.
func (w *Writer) SetEscapeHTML(on bool) {
	w.escapeHTML = on
}

// Bytes returns the JSON written so far, or nil if the Writer has failed. The
// slice aliases the Writer's internal buffer: the caller must not modify it, and
// it is valid only while the Writer is not written to or reset again. Use
// slices.Clone for a copy that outlives the Writer.
func (w *Writer) Bytes() []byte {
	if w.err != nil {
		return nil
	}
	return w.buf
}

// BuildBytes returns the JSON written so far, or nil if the Writer has failed.
// Unlike Bytes it never hands out the Writer's internal buffer: the document is
// appended to the optional caller-supplied buffer, reusing that buffer's
// capacity, or with no buffer to a freshly allocated one. The result is owned by
// the caller — it may be kept and modified — and the Writer stays valid and
// reusable, so one Writer can build document after document across a Reset.
//
// Pass a scratch buffer to amortise allocation across calls. To append the
// document at the start of the caller's buffer, pass it truncated, buf[:0]:
//
//	var w qijson.Writer
//	// ... write one value ...
//	doc := w.BuildBytes(buf[:0])
//
// At most one reuse buffer is used; any further arguments are ignored.
func (w *Writer) BuildBytes(reuse ...[]byte) []byte {
	if w.err != nil {
		return nil
	}
	var dst []byte
	if len(reuse) > 0 {
		dst = reuse[0]
	}
	return append(dst, w.buf...)
}

// Err returns the first error the Writer hit, or nil.
func (w *Writer) Err() error { return w.err }

// DumpTo writes the JSON built so far to out and, on success, resets the Writer
// so the next value can be built and dumped into the same stream:
//
//	for _, v := range values {
//	    // ... write one value into w ...
//	    if _, err := w.DumpTo(conn); err != nil { /* ... */ }
//	}
//
// It writes the buffered bytes directly to out without materialising a second
// copy, and returns the number of bytes written. On a write error the Writer is
// left untouched, so the document is not lost and the dump can be retried. A
// Writer that already holds a sticky error dumps nothing and returns that error,
// matching Bytes and BuildBytes. A nil out is reported as errNilDumpTarget
// rather than panicking.
//
// DumpTo mutates w: it must not be called concurrently with any other Writer
// method.
func (w *Writer) DumpTo(out io.Writer) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if out == nil {
		return 0, errNilDumpTarget
	}
	n, err := out.Write(w.buf)
	if err != nil {
		return n, err
	}
	w.Reset()
	return n, nil
}

// ReadCloser returns an io.ReadCloser that reads the JSON built so far and, on
// success, resets the Writer so the next value can be built immediately. It is
// the read side of the streaming surface: hand it to a consumer that wants an
// io.Reader (an HTTP body, io.Copy) instead of the raw bytes:
//
//	rc, err := w.ReadCloser()
//	if err != nil { /* ... */ }
//	defer rc.Close()
//	io.Copy(conn, rc)
//
// The reader owns the bytes it reads: ReadCloser detaches the Writer's buffer,
// so the reader does not alias the Writer and a later write to the Writer does
// not corrupt the reader's data. A Writer that already holds a sticky error
// returns (nil, err) rather than a reader. Close is a no-op that returns nil and
// is safe to call more than once; it does not touch the Writer.
func (w *Writer) ReadCloser() (io.ReadCloser, error) {
	if w.err != nil {
		return nil, w.err
	}
	buf := w.buf
	w.Reset()
	w.buf = nil
	return &writerReader{buf: buf}, nil
}

// writerReader reads a buffer once, front to back. It owns the buffer handed to
// it by ReadCloser, which is detached from the Writer.
type writerReader struct {
	buf []byte
	off int
}

// *writerReader implements io.ReadCloser as part of its contract; this fails to
// compile if it ever stops doing so.
var _ io.ReadCloser = (*writerReader)(nil)

// Read copies from the buffer until it is exhausted, then reports io.EOF.
func (r *writerReader) Read(p []byte) (int, error) {
	if r.off >= len(r.buf) {
		return 0, io.EOF
	}
	n := copy(p, r.buf[r.off:])
	r.off += n
	return n, nil
}

// Close is a no-op that returns nil. It is safe to call more than once and never
// fails.
func (r *writerReader) Close() error { return nil }

// complete reports whether w holds exactly one complete top-level JSON value:
// the single root value has been written, every container it opened has been
// closed, and the value produced at least one byte. The byte check rejects an
// Encoder that marked the root written with an empty WriteRaw, which would
// otherwise pass with no value at all. It is how Marshal rejects an Encoder that
// returned nil without writing a whole value.
func (w *Writer) complete() bool {
	return w.err == nil && w.wroteRoot && len(w.stack) == 0 && len(w.buf) > 0
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
	if w.indenting() {
		w.writeNewlineIndent(len(w.stack))
	}
	return nil
}

// indenting reports whether the Writer is configured to emit an indented
// document. Indentation is off only when both prefix and indent are empty.
func (w *Writer) indenting() bool {
	return w.prefix != "" || w.indent != ""
}

// writeNewlineIndent appends the line break that precedes a token at the given
// depth: a newline, then the prefix, then the indent repeated depth times.
func (w *Writer) writeNewlineIndent(depth int) {
	w.buf = append(w.buf, '\n')
	w.buf = append(w.buf, w.prefix...)
	for ; depth > 0; depth-- {
		w.buf = append(w.buf, w.indent...)
	}
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
// byte is replaced with U+FFFD, so encoding never fails on a string. By default
// it does not escape '<', '>', or '&'; SetEscapeHTML(true) turns that escaping
// on.
func (w *Writer) WriteString(v string) error {
	if err := w.beginValue(); err != nil {
		return err
	}
	w.buf = appendQuoted(w.buf, v, w.escapeHTML)
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
// captured while decoding. v must hold exactly one complete JSON value; an empty
// slice is rejected with errEmptyRaw, since it would mark the document complete
// without writing a value. The bytes are trusted: a non-empty v that is not valid
// JSON is written as-is.
func (w *Writer) WriteRaw(v []byte) error {
	if len(v) == 0 {
		return w.fail(errEmptyRaw)
	}
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
	count := w.stack[len(w.stack)-1].count
	w.stack = w.stack[:len(w.stack)-1]
	if w.indenting() && count > 0 {
		w.writeNewlineIndent(len(w.stack))
	}
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
	w.stack = append(w.stack, writerFrame{})
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
	count := w.stack[len(w.stack)-1].count
	w.stack = w.stack[:len(w.stack)-1]
	if w.indenting() && count > 0 {
		w.writeNewlineIndent(len(w.stack))
	}
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
	if w.indenting() {
		w.writeNewlineIndent(len(w.stack))
	}
	w.buf = appendQuoted(w.buf, name, w.escapeHTML)
	if w.indenting() {
		w.buf = append(w.buf, ':', ' ')
	} else {
		w.buf = append(w.buf, ':')
	}
	w.pendingKey = true
	return nil
}

// hexDigits is the lower-case hex alphabet used for \u escapes.
const hexDigits = "0123456789abcdef"

// appendQuoted appends s as a JSON string literal. It escapes '"', '\\' and the
// control bytes using the short escapes where JSON defines one, and replaces
// each invalid UTF-8 byte with U+FFFD. When escapeHTML is true it also escapes
// '<', '>', '&' and the characters U+2028 and U+2029 as \u escapes; the ASCII
// ones fall through to appendEscapedByte's generic \u00XX form.
func appendQuoted(dst []byte, s string, escapeHTML bool) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		b := s[i]
		if b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' && (!escapeHTML || !htmlSensitive(b)) {
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
		if escapeHTML && (r == '\u2028' || r == '\u2029') {
			dst = append(dst, s[start:i]...)
			if r == '\u2028' {
				dst = append(dst, `\u2028`...)
			} else {
				dst = append(dst, `\u2029`...)
			}
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// htmlSensitive reports whether b is one of the ASCII bytes the encoder escapes
// when HTML escaping is on: '<', '>' or '&'.
func htmlSensitive(b byte) bool {
	return b == '<' || b == '>' || b == '&'
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
