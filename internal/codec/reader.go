package codec

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/qi-lore/qi-json/internal/scanner"
)

// compsInline is the number of JSON path components held inline in a Reader
// before the component stack grows onto the heap. It bounds nesting depth, not
// the number of members or elements, so decoding a flat array or object never
// grows it.
const compsInline = 8

// Reader decodes typed values from a JSON document, tracking the JSON path of
// the value being decoded so that errors point at the offending member. It reads
// from the pull tokenizer: whitespace,
// separators and object keys are handled by the tokenizer and the Object and
// Array iterators.
//
// The JSON path is built lazily: descending into a container records a component
// and the path string is only rendered when Path (or an error that carries it)
// needs it, so a successful decode never formats the path.
//
// A Reader is created by the decoder entry points and passed to Decoder
// implementations. It is not safe for concurrent use and must not be copied
// after first use (its component stack points into itself).
type Reader struct {
	tok *scanner.Tokenizer

	// comps is the stack of path components from the document root to the value
	// being decoded. inline backs the common case so descending does not
	// allocate.
	comps  []pathComp
	inline [compsInline]pathComp

	// peekKind, peekOff and peekTok memoize the kind of the token at the
	// tokenizer's current offset. A Peek followed by a read would otherwise
	// peek twice; the cache makes the read reuse the Peek. It stays valid only
	// while the same tokenizer has neither advanced nor failed since the kind
	// was recorded.
	peekKind scanner.Kind
	peekOff  int64
	peekTok  *scanner.Tokenizer
}

// pathComp is one step in the JSON path: a member name or an array index.
type pathComp struct {
	// name is the member name; it is meaningful when index is negative.
	name string

	// index is the array index, or -1 for a member.
	index int
}

// newReader returns a Reader that decodes from tok.
func newReader(tok *scanner.Tokenizer) *Reader {
	r := &Reader{tok: tok}
	r.comps = r.inline[:0]
	return r
}

// pushKey records that the value about to be read is the member name of the
// current object.
func (r *Reader) pushKey(name string) {
	r.comps = append(r.comps, pathComp{name: name, index: -1})
}

// pushIndex records that the value about to be read is element i of the current
// array.
func (r *Reader) pushIndex(i int) {
	r.comps = append(r.comps, pathComp{index: i})
}

// popComp discards the innermost path component.
func (r *Reader) popComp() {
	r.comps = r.comps[:len(r.comps)-1]
}

// Peek returns the kind of the next value without consuming it, so a Decoder can
// dispatch on it.
func (r *Reader) Peek() (scanner.Kind, error) {
	if k, ok := r.cachedKind(); ok {
		return k, nil
	}
	k, err := r.tok.Peek()
	if err != nil {
		return k, r.syntaxError(err)
	}
	r.peekKind, r.peekOff, r.peekTok = k, r.tok.Offset(), r.tok
	return k, nil
}

// cachedKind returns the memorized kind of the token at the tokenizer's current
// offset, or false when there is none. The entry is used only while the same
// tokenizer has neither advanced nor failed since it was recorded, so it never
// hides a consumed token or a sticky error.
func (r *Reader) cachedKind() (scanner.Kind, bool) {
	if r.peekTok == r.tok && r.peekTok != nil && r.tok.Offset() == r.peekOff && r.tok.Err() == nil {
		return r.peekKind, true
	}
	return 0, false
}

// nextKind returns the kind of the next value, reusing a just-made Peek.
func (r *Reader) nextKind() (scanner.Kind, error) {
	if k, ok := r.cachedKind(); ok {
		return k, nil
	}
	return r.Peek()
}

// ReadNull consumes a null value.
func (r *Reader) ReadNull() error {
	k, err := r.nextKind()
	if err != nil {
		return err
	}
	if k != scanner.KindNull {
		return r.mismatch("null", k)
	}
	if err := r.tok.ReadNull(); err != nil {
		return r.syntaxError(err)
	}
	return nil
}

// ReadBool consumes a true or false value.
func (r *Reader) ReadBool() (bool, error) {
	k, err := r.nextKind()
	if err != nil {
		return false, err
	}
	if k != scanner.KindBool {
		return false, r.mismatch("boolean", k)
	}
	v, err := r.tok.ReadBool()
	if err != nil {
		return false, r.syntaxError(err)
	}
	return v, nil
}

// ReadString consumes a string value and returns it decoded: escape sequences
// are resolved and a lone surrogate becomes U+FFFD.
func (r *Reader) ReadString() (string, error) {
	k, err := r.nextKind()
	if err != nil {
		return "", err
	}
	if k != scanner.KindString {
		return "", r.mismatch("string", k)
	}
	// After the kind check the tokenizer is positioned at the opening quote, so
	// the content starts one byte later. ReadString consumes the closing quote,
	// and for a key it also consumes the ':', which makes Offset() unreliable as
	// a content anchor.
	start := r.tok.Offset()
	raw, err := r.tok.ReadString()
	if err != nil {
		return "", r.syntaxError(err)
	}
	if err := r.checkString(raw, start+1); err != nil {
		return "", err
	}
	s, err := unescape(raw)
	if err != nil {
		return "", r.syntaxError(err)
	}
	return s, nil
}

// ReadInt consumes a number and returns it as an int64. The literal must be an
// integer (no fraction or exponent) and must fit in an int64; both violations
// are reported as type errors.
func (r *Reader) ReadInt() (int64, error) {
	raw, err := r.readNumber("integer")
	if err != nil {
		return 0, err
	}
	v, perr := strconv.ParseInt(string(raw), 10, 64)
	if perr != nil {
		return 0, r.numberMismatch("integer", "int64", raw, perr)
	}
	return v, nil
}

// ReadFloat consumes a number and returns it as a float64. A magnitude that
// overflows a float64 is a type error; a magnitude that underflows becomes zero.
func (r *Reader) ReadFloat() (float64, error) {
	raw, err := r.readNumber("number")
	if err != nil {
		return 0, err
	}
	v, perr := strconv.ParseFloat(string(raw), 64)
	if perr != nil {
		return 0, r.numberMismatch("number", "float64", raw, perr)
	}
	return v, nil
}

// readNumber peeks at a number token and returns its raw bytes, describing the
// expected JSON type as want for a type error.
func (r *Reader) readNumber(want string) ([]byte, error) {
	k, err := r.nextKind()
	if err != nil {
		return nil, err
	}
	if k != scanner.KindNumber {
		return nil, r.mismatch(want, k)
	}
	raw, err := r.tok.ReadNumber()
	if err != nil {
		return nil, r.syntaxError(err)
	}
	return raw, nil
}

// numberMismatch describes a number that Peek accepted but parsing rejected, as
// a type error. want is the JSON type expected and target names the Go kind,
// such as "int64".
func (r *Reader) numberMismatch(want, target string, raw []byte, perr error) error {
	got := "number " + string(raw)
	if ne, ok := perr.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
		return r.typeMismatch(want, got+" out of range for "+target)
	}
	if want == "integer" {
		return r.typeMismatch(want, got+" is not an integer")
	}
	return r.typeMismatch(want, got+" is not a number")
}

// beginObject consumes the '{' that opens an object. A next value that is not an
// object is a type error, not a syntax error: the JSON is well-formed but does
// not match the target.
func (r *Reader) beginObject() error {
	k, err := r.Peek()
	if err != nil {
		return err
	}
	if k != scanner.KindObject {
		return r.mismatch("object", k)
	}
	if err := r.tok.BeginObject(); err != nil {
		return r.syntaxError(err)
	}
	return nil
}

// beginArray consumes the '[' that opens an array, reporting a type error if the
// next value is not an array.
func (r *Reader) beginArray() error {
	k, err := r.Peek()
	if err != nil {
		return err
	}
	if k != scanner.KindArray {
		return r.mismatch("array", k)
	}
	if err := r.tok.BeginArray(); err != nil {
		return r.syntaxError(err)
	}
	return nil
}

// Object consumes an object, calling fn with each member key. fn is responsible
// for consuming the member value. While fn runs, the JSON path is extended with
// the member name.
func (r *Reader) Object(fn func(key string) error) error {
	if err := r.beginObject(); err != nil {
		return err
	}
	for {
		k, err := r.Peek()
		if err != nil {
			return err
		}
		if k == scanner.KindEnd {
			break
		}
		start := r.tok.Offset()
		raw, err := r.tok.ReadString()
		if err != nil {
			return r.syntaxError(err)
		}
		if err := r.checkString(raw, start+1); err != nil {
			return err
		}
		key, err := unescape(raw)
		if err != nil {
			return r.syntaxError(err)
		}

		before := len(r.comps)
		r.pushKey(key)
		err = fn(key)
		r.comps = r.comps[:before]
		if err != nil {
			return err
		}
	}
	if err := r.tok.EndObject(); err != nil {
		return r.syntaxError(err)
	}
	return nil
}

// objectRaw consumes an object, calling fn with each member's raw key bytes
// (escape sequences intact). fn is responsible for consuming the member value.
// Unlike Object, it does not extend the JSON path or decode the key, so a caller
// that matches keys itself can do so without allocating; it extends the path
// itself when it needs one.
func (r *Reader) objectRaw(fn func(raw []byte) error) error {
	if err := r.beginObject(); err != nil {
		return err
	}
	for {
		k, err := r.Peek()
		if err != nil {
			return err
		}
		if k == scanner.KindEnd {
			break
		}
		start := r.tok.Offset()
		raw, err := r.tok.ReadString()
		if err != nil {
			return r.syntaxError(err)
		}
		if err := r.checkString(raw, start+1); err != nil {
			return err
		}
		if err := fn(raw); err != nil {
			return err
		}
	}
	if err := r.tok.EndObject(); err != nil {
		return r.syntaxError(err)
	}
	return nil
}

// Array consumes an array, calling fn with each element index. fn is responsible
// for consuming the element value. While fn runs, the JSON path is extended with
// the element index.
func (r *Reader) Array(fn func(index int) error) error {
	if err := r.beginArray(); err != nil {
		return err
	}
	for i := 0; ; i++ {
		k, err := r.Peek()
		if err != nil {
			return err
		}
		if k == scanner.KindEnd {
			break
		}

		before := len(r.comps)
		r.pushIndex(i)
		err = fn(i)
		r.comps = r.comps[:before]
		if err != nil {
			return err
		}
	}
	if err := r.tok.EndArray(); err != nil {
		return r.syntaxError(err)
	}
	return nil
}

// Skip consumes and discards the next value, composites included.
func (r *Reader) Skip() error {
	if err := r.tok.Skip(); err != nil {
		return r.syntaxError(err)
	}
	return nil
}

// raw consumes the next value, composites included, and returns its raw bytes
// exactly as they appear in the input. The returned slice aliases the input, so
// callers that keep it must copy.
func (r *Reader) raw() ([]byte, error) {
	b, err := r.tok.ReadRaw()
	if err != nil {
		return nil, r.syntaxError(err)
	}
	return b, nil
}

// Path returns the JSON path of the value being decoded, such as "$.a.b[2].c".
// At the document root it is "$". The path is rendered on demand from the
// recorded path components, so it is only built when an error needs it.
func (r *Reader) Path() string {
	if len(r.comps) == 0 {
		return "$"
	}
	var dst []byte
	for i := range r.comps {
		if c := &r.comps[i]; c.index >= 0 {
			dst = appendIndexTo(dst, c.index)
		} else {
			dst = appendKeyTo(dst, c.name)
		}
	}
	return "$" + string(dst)
}

// Offset returns the number of bytes of the input consumed so far.
func (r *Reader) Offset() int64 { return r.tok.Offset() }

// checkString rejects a JSON string whose bytes are not valid UTF-8. raw is the
// content between the quotes with escape sequences intact and contentStart is the
// byte offset of that content in the input. Escapes are ASCII and \u decoding
// always yields valid UTF-8 (a lone surrogate becomes U+FFFD), so the decoded
// string is valid exactly when raw is. Pure ASCII, the common case, pays only a
// high-bit scan.
func (r *Reader) checkString(raw []byte, contentStart int64) error {
	if !hasHighBit(raw) {
		return nil
	}
	i := firstInvalidUTF8(raw)
	if i < 0 {
		return nil
	}
	return &Error{
		Path:   r.Path(),
		Offset: contentStart + int64(i),
		Kind:   KindSyntax,
		Err:    errInvalidUTF8,
	}
}

// hasHighBit reports whether b contains a byte with the high bit set. A sequence
// of bytes all below 0x80 is always valid UTF-8, so the expensive validator only
// runs when this returns true.
func hasHighBit(b []byte) bool {
	const high = 0x8080808080808080
	for len(b) >= 8 {
		if binary.LittleEndian.Uint64(b)&high != 0 {
			return true
		}
		b = b[8:]
	}
	for _, c := range b {
		if c&0x80 != 0 {
			return true
		}
	}
	return false
}

// firstInvalidUTF8 returns the index of the first byte of b that does not start a
// valid UTF-8 sequence, or -1 when b is valid. b is assumed to contain a high
// bit, so this runs only on the slow path.
func firstInvalidUTF8(b []byte) int {
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return -1
}

// syntaxError wraps a tokenizer error as a syntax Error.
func (r *Reader) syntaxError(err error) error {
	if err == nil {
		return nil
	}
	return &Error{
		Path:   r.Path(),
		Offset: r.tok.Offset(),
		Kind:   KindSyntax,
		Err:    err,
	}
}

// mismatch reports that the next value of kind got does not match want.
func (r *Reader) mismatch(want string, got scanner.Kind) error {
	return r.typeMismatch(want, got.String())
}

// typeMismatch builds a type error whose found-value description is got.
func (r *Reader) typeMismatch(want, got string) error {
	return &Error{
		Path:   r.Path(),
		Offset: r.tok.Offset(),
		Kind:   KindType,
		Err:    &TypeError{Want: want, Got: got},
	}
}

// unknownField reports an object member that no field matched, naming it in the
// JSON path.
func (r *Reader) unknownField(raw []byte) error {
	key, err := unescape(raw)
	if err != nil {
		key = string(raw)
	}
	return &Error{
		Path:   r.pathWithKey(key),
		Offset: r.tok.Offset(),
		Kind:   KindUnknownField,
	}
}

// duplicateField reports an object member that a field already consumed.
func (r *Reader) duplicateField(name string) error {
	return &Error{
		Path:   r.pathWithKey(name),
		Offset: r.tok.Offset(),
		Kind:   KindDuplicateField,
	}
}

// pathWithKey returns the current JSON path extended with a member name, leaving
// the reader's path unchanged.
func (r *Reader) pathWithKey(key string) string {
	r.pushKey(key)
	p := r.Path()
	r.popComp()
	return p
}

// appendKeyTo appends key to dst as a JSON path step, quoting it when it is not a
// plain identifier.
func appendKeyTo(dst []byte, key string) []byte {
	if isPlainKey(key) {
		dst = append(dst, '.')
		return append(dst, key...)
	}
	dst = append(dst, '[')
	dst = appendQuotedTo(dst, key)
	return append(dst, ']')
}

// appendIndexTo appends an array index to dst as "[i]".
func appendIndexTo(dst []byte, i int) []byte {
	dst = append(dst, '[')
	dst = strconv.AppendInt(dst, int64(i), 10)
	return append(dst, ']')
}

// isPlainKey reports whether key can follow a '.' in a JSON path without
// quoting.
func isPlainKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case 'a' <= c && c <= 'z':
		case 'A' <= c && c <= 'Z':
		case '0' <= c && c <= '9':
		case c == '_' || c == '$':
		default:
			return false
		}
	}
	return true
}

// appendQuotedTo appends key to dst as a double-quoted JSON string.
func appendQuotedTo(dst []byte, key string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c == '"' || c == '\\' {
			dst = append(dst, '\\')
		}
		dst = append(dst, c)
	}
	return append(dst, '"')
}

// unescape decodes the JSON escape sequences in raw and returns the decoded
// string. raw is the content between the quotes with escapes intact, as returned
// by the tokenizer. A fast path avoids a second copy when raw has no escapes.
func unescape(raw []byte) (string, error) {
	if bytes.IndexByte(raw, '\\') < 0 {
		return string(raw), nil
	}

	buf := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); {
		c := raw[i]
		if c != '\\' {
			buf = append(buf, c)
			i++
			continue
		}
		if i+1 >= len(raw) {
			return "", errBadEscape
		}
		switch raw[i+1] {
		case '"':
			buf = append(buf, '"')
			i += 2
		case '\\':
			buf = append(buf, '\\')
			i += 2
		case '/':
			buf = append(buf, '/')
			i += 2
		case 'b':
			buf = append(buf, '\b')
			i += 2
		case 'f':
			buf = append(buf, '\f')
			i += 2
		case 'n':
			buf = append(buf, '\n')
			i += 2
		case 'r':
			buf = append(buf, '\r')
			i += 2
		case 't':
			buf = append(buf, '\t')
			i += 2
		case 'u':
			rr, size, ok := unescapeUnicode(raw[i:])
			if !ok {
				return "", errBadEscape
			}
			buf = utf8.AppendRune(buf, rr)
			i += size
		default:
			return "", errBadEscape
		}
	}
	return string(buf), nil
}

// unescapeUnicode decodes the \u escape at the start of raw, combining a
// surrogate pair when one follows. It returns the rune, the number of bytes
// consumed, and whether the escape is well-formed. A lone surrogate is replaced
// with U+FFFD.
func unescapeUnicode(raw []byte) (rune, int, bool) {
	if len(raw) < 6 || raw[0] != '\\' || raw[1] != 'u' {
		return 0, 0, false
	}
	r1, ok := hex4(raw[2:6])
	if !ok {
		return 0, 0, false
	}
	if !utf16.IsSurrogate(rune(r1)) {
		return rune(r1), 6, true
	}
	if len(raw) >= 12 && raw[6] == '\\' && raw[7] == 'u' {
		if r2, ok := hex4(raw[8:12]); ok {
			if rr := utf16.DecodeRune(rune(r1), rune(r2)); rr != utf8.RuneError {
				return rr, 12, true
			}
		}
	}
	return utf8.RuneError, 6, true
}

// hex4 decodes four hexadecimal digits.
func hex4(b []byte) (uint32, bool) {
	if len(b) < 4 {
		return 0, false
	}
	var v uint32
	for _, c := range b[:4] {
		d, ok := hexVal(c)
		if !ok {
			return 0, false
		}
		v = v<<4 | uint32(d)
	}
	return v, true
}

// hexVal returns the value of the ASCII hexadecimal digit c.
func hexVal(c byte) (byte, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}
