package marshal

import (
	"bytes"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"

	"qi-json/scanner"
)

// pathInline is the number of bytes of JSON path held inline in a Reader before
// the path grows onto the heap.
const pathInline = 64

// Reader decodes typed values from a JSON document, tracking the JSON path of
// the value being decoded so that errors point at the offending member. It reads
// from the pull tokenizer in qi-json/scanner: whitespace, separators and object
// keys are handled by the tokenizer and the Object and Array iterators.
//
// A Reader is created by the decoder entry points and passed to Decoder
// implementations. It is not safe for concurrent use and must not be copied
// after first use (its path buffer points into itself).
type Reader struct {
	tok *scanner.Tokenizer

	// path holds the JSON path of the value about to be read, in the form
	// ".a.b[2]", relative to the document root. buf backs path so the common
	// case does not allocate.
	path []byte
	buf  [pathInline]byte
}

// newReader returns a Reader that decodes from tok.
func newReader(tok *scanner.Tokenizer) *Reader {
	r := &Reader{tok: tok}
	r.path = r.buf[:0]
	return r
}

// Peek returns the kind of the next value without consuming it, so a Decoder can
// dispatch on it.
func (r *Reader) Peek() (scanner.Kind, error) {
	k, err := r.tok.Peek()
	if err != nil {
		return k, r.syntaxError(err)
	}
	return k, nil
}

// ReadNull consumes a null value.
func (r *Reader) ReadNull() error {
	k, err := r.tok.Peek()
	if err != nil {
		return r.syntaxError(err)
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
	k, err := r.tok.Peek()
	if err != nil {
		return false, r.syntaxError(err)
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
	k, err := r.tok.Peek()
	if err != nil {
		return "", r.syntaxError(err)
	}
	if k != scanner.KindString {
		return "", r.mismatch("string", k)
	}
	raw, err := r.tok.ReadString()
	if err != nil {
		return "", r.syntaxError(err)
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
	k, err := r.tok.Peek()
	if err != nil {
		return nil, r.syntaxError(err)
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
		k, err := r.tok.Peek()
		if err != nil {
			return r.syntaxError(err)
		}
		if k == scanner.KindEnd {
			break
		}
		raw, err := r.tok.ReadString()
		if err != nil {
			return r.syntaxError(err)
		}
		key, err := unescape(raw)
		if err != nil {
			return r.syntaxError(err)
		}

		before := len(r.path)
		r.appendKey(key)
		err = fn(key)
		r.path = r.path[:before]
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
		k, err := r.tok.Peek()
		if err != nil {
			return r.syntaxError(err)
		}
		if k == scanner.KindEnd {
			break
		}
		raw, err := r.tok.ReadString()
		if err != nil {
			return r.syntaxError(err)
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
		k, err := r.tok.Peek()
		if err != nil {
			return r.syntaxError(err)
		}
		if k == scanner.KindEnd {
			break
		}

		before := len(r.path)
		r.appendIndex(i)
		err = fn(i)
		r.path = r.path[:before]
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

// Path returns the JSON path of the value being decoded, such as "$.a.b[2].c".
// At the document root it is "$".
func (r *Reader) Path() string {
	if len(r.path) == 0 {
		return "$"
	}
	return "$" + string(r.path)
}

// Offset returns the number of bytes of the input consumed so far.
func (r *Reader) Offset() int64 { return r.tok.Offset() }

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
	before := len(r.path)
	r.appendKey(key)
	p := r.Path()
	r.path = r.path[:before]
	return p
}

// appendKey extends the current path with a member name.
func (r *Reader) appendKey(key string) {
	if isPlainKey(key) {
		r.path = append(r.path, '.')
		r.path = append(r.path, key...)
		return
	}
	r.path = append(r.path, '[')
	r.appendQuoted(key)
	r.path = append(r.path, ']')
}

// appendIndex extends the current path with an array index.
func (r *Reader) appendIndex(i int) {
	r.path = append(r.path, '[')
	r.path = strconv.AppendInt(r.path, int64(i), 10)
	r.path = append(r.path, ']')
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

// appendQuoted appends key to the path as a double-quoted JSON string.
func (r *Reader) appendQuoted(key string) {
	r.path = append(r.path, '"')
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c == '"' || c == '\\' {
			r.path = append(r.path, '\\')
		}
		r.path = append(r.path, c)
	}
	r.path = append(r.path, '"')
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
