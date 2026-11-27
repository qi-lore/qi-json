package codec

import (
	"bytes"
	"errors"
	"io"

	"github.com/qi-lore/qi-json/internal/scanner"
)

// Indent appends a pretty-printed form of the JSON text in src to dst. prefix is
// written at the start of every new line and indent is repeated once per level of
// nesting, so Indent(dst, src, "", "  ") indents each level by two spaces. Inside
// a container each element or member begins on its own line, a ':' is followed by
// a single space, and an empty object or array stays on one line as "{}" or "[]".
// No newline is written after the final token. The bytes inside JSON strings are
// preserved verbatim; only insignificant whitespace is normalised.
//
// On malformed input Indent appends nothing and returns a *Error with Kind
// KindSyntax that records the byte offset where the input became invalid. src is
// not retained after the call.
//
// Indent is a pure function of its arguments and holds no state, so it is safe
// for concurrent use. Because it appends to dst, callers must not share the same
// *bytes.Buffer across goroutines.
func Indent(dst *bytes.Buffer, src []byte, prefix, indent string) error {
	if err := validateSyntax(src); err != nil {
		return err
	}
	// Build the whole result before touching dst so a failure appends nothing.
	out := appendIndented(nil, src, prefix, indent)
	_, err := dst.Write(out)
	return err
}

// Compact appends the JSON text in src to dst with all insignificant whitespace
// removed, so the result is a single compact line. Whitespace inside JSON strings
// is part of the string and is preserved; only the whitespace between tokens is
// dropped.
//
// On malformed input Compact appends nothing and returns a *Error with Kind
// KindSyntax that records the byte offset where the input became invalid. src is
// not retained after the call.
//
// Compact is a pure function of its arguments and holds no state, so it is safe
// for concurrent use. Because it appends to dst, callers must not share the same
// *bytes.Buffer across goroutines.
func Compact(dst *bytes.Buffer, src []byte) error {
	if err := validateSyntax(src); err != nil {
		return err
	}
	out := appendCompact(nil, src)
	_, err := dst.Write(out)
	return err
}

// HTMLEscape appends the JSON text in src to dst with the HTML-sensitive bytes
// replaced: '<' becomes \u003c, '>' becomes \u003e, '&' becomes \u0026, U+2028
// becomes \u2028 and U+2029 becomes \u2029. Every other byte is copied unchanged.
// The replacements are the ones encoding/json applies so a document can be
// embedded in an HTML <script> element; they are made wherever the bytes occur,
// and src is otherwise assumed to be valid JSON.
//
// HTMLEscape never fails and has no error return: it copies its input rather than
// validating it, so it does not report malformed JSON. src is not retained after
// the call.
//
// HTMLEscape is a pure function of its arguments and holds no state, so it is
// safe for concurrent use. Because it appends to dst, callers must not share the
// same *bytes.Buffer across goroutines.
func HTMLEscape(dst *bytes.Buffer, src []byte) {
	_, _ = dst.Write(appendHTMLEscaped(nil, src))
}

// MarshalIndent encodes v as JSON and appends a pretty-printed form to a new
// buffer, using prefix and indent exactly as Indent does. The pointer type *T must
// implement Encoder, as with Marshal, so the type argument alone selects the
// encoder:
//
//	data, err := qijson.MarshalIndent[Config](cfg, "", "  ")
//
// HTML escaping stays off, matching Marshal: '<', '>' and '&' are written
// literally.
//
// The returned []byte is freshly allocated and owned by the caller; it does not
// alias v. On an encoding failure MarshalIndent returns (nil, err) and writes
// nothing. It is safe for concurrent use.
func MarshalIndent[T any, PT interface {
	*T
	Encoder
}](v T, prefix, indent string) ([]byte, error) {
	data, err := Marshal[T, PT](v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := Indent(&buf, data, prefix, indent); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// validateSyntax walks src with the tokenizer and returns a *Error of Kind
// KindSyntax at the first malformed token, or nil when src holds exactly one
// complete JSON value. It is the syntax gate Indent and Compact share with the
// decoder, so their error offsets and messages match the rest of the package.
func validateSyntax(src []byte) error {
	tok := scanner.NewTokenizer(src)
	for {
		_, err := tok.Token()
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return &Error{
			Offset: tok.Offset(),
			Kind:   KindSyntax,
			Err:    err,
		}
	}
}

// appendIndented appends the pretty-printed form of the valid JSON in src to dst.
func appendIndented(dst, src []byte, prefix, indent string) []byte {
	depth := 0
	for i := 0; i < len(src); {
		c := src[i]
		switch c {
		case ' ', '\t', '\n', '\r':
			i++
		case '"':
			j := jsonStringEnd(src, i)
			dst = append(dst, src[i:j]...)
			i = j
		case '{', '[':
			dst = append(dst, c)
			i++
			closing := byte('}')
			if c == '[' {
				closing = ']'
			}
			if j := skipJSONSpace(src, i); j < len(src) && src[j] == closing {
				// An empty container stays on one line.
				dst = append(dst, closing)
				i = j + 1
				continue
			}
			depth++
			dst = appendNewlineIndent(dst, prefix, indent, depth)
		case '}', ']':
			depth--
			dst = appendNewlineIndent(dst, prefix, indent, depth)
			dst = append(dst, c)
			i++
		case ',':
			dst = append(dst, ',')
			i++
			dst = appendNewlineIndent(dst, prefix, indent, depth)
		case ':':
			dst = append(dst, ':', ' ')
			i++
		default:
			dst = append(dst, c)
			i++
		}
	}
	return dst
}

// appendCompact appends the compact form of the valid JSON in src to dst: every
// byte outside a string is copied, and whitespace outside a string is dropped.
func appendCompact(dst, src []byte) []byte {
	for i := 0; i < len(src); {
		c := src[i]
		switch c {
		case ' ', '\t', '\n', '\r':
			i++
		case '"':
			j := jsonStringEnd(src, i)
			dst = append(dst, src[i:j]...)
			i = j
		default:
			dst = append(dst, c)
			i++
		}
	}
	return dst
}

// appendNewlineIndent appends the line break that precedes a token at the given
// depth: a newline, then prefix, then indent repeated depth times.
func appendNewlineIndent(dst []byte, prefix, indent string, depth int) []byte {
	dst = append(dst, '\n')
	dst = append(dst, prefix...)
	for ; depth > 0; depth-- {
		dst = append(dst, indent...)
	}
	return dst
}

// jsonStringEnd returns the index just past the closing quote of the JSON string
// whose opening quote is at src[i]. A backslash escapes the byte after it, so a
// quote preceded by an odd number of backslashes does not end the string. The
// input is valid JSON, so the closing quote is always found.
func jsonStringEnd(src []byte, i int) int {
	j := i + 1
	for j < len(src) {
		switch src[j] {
		case '\\':
			j += 2
		case '"':
			return j + 1
		default:
			j++
		}
	}
	return j
}

// skipJSONSpace returns the index of the first byte at or after i that is not
// JSON whitespace.
func skipJSONSpace(src []byte, i int) int {
	for i < len(src) {
		switch src[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

// appendHTMLEscaped appends src to dst with the HTML-sensitive bytes replaced by
// their \u escapes: '<', '>', '&' and the three-byte encodings of U+2028 and
// U+2029. Every other byte is copied unchanged.
func appendHTMLEscaped(dst, src []byte) []byte {
	start := 0
	for i := 0; i < len(src); i++ {
		var esc string
		switch src[i] {
		case '<':
			esc = `\u003c`
		case '>':
			esc = `\u003e`
		case '&':
			esc = `\u0026`
		case 0xE2:
			// U+2028 (E2 80 A8) and U+2029 (E2 80 A9) are the only non-ASCII
			// bytes encoding/json escapes.
			if i+2 < len(src) && src[i+1] == 0x80 {
				switch src[i+2] {
				case 0xA8:
					esc = `\u2028`
				case 0xA9:
					esc = `\u2029`
				}
			}
		}
		if esc == "" {
			continue
		}
		dst = append(dst, src[start:i]...)
		dst = append(dst, esc...)
		if src[i] == 0xE2 {
			i += 2 // consume the two continuation bytes as well
		}
		start = i + 1
	}
	return append(dst, src[start:]...)
}
