package scanner

import (
	"errors"
	"io"
	"strconv"
	"testing"
)

// tokenDesc renders a Token as a printable descriptor that keeps the token's
// dynamic type visible, so the tests tell a string value apart from a Number and
// both apart from a Delim.
func tokenDesc(tok Token) string {
	switch v := tok.(type) {
	case nil:
		return "null"
	case bool:
		return "bool:" + strconv.FormatBool(v)
	case Delim:
		return "delim:" + string(rune(v))
	case Number:
		return "num:" + string(v)
	case string:
		return "str:" + v
	default:
		return "?"
	}
}

// tokenStream drains a tokenizer with Token, recording each token's descriptor,
// and stops at io.EOF.
func tokenStream(t *Tokenizer) ([]string, error) {
	var out []string
	for {
		tok, err := t.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, tokenDesc(tok))
	}
}

// peekStream walks a document with Peek and the typed readers, recording the
// same descriptors tokenStream records, so the two streams can be compared over
// the shared document tables.
func peekStream(t *Tokenizer) ([]string, error) {
	var out []string
	var walk func() error
	walk = func() error {
		k, err := t.Peek()
		if err != nil {
			return err
		}
		switch k {
		case KindNull:
			if err := t.ReadNull(); err != nil {
				return err
			}
			out = append(out, "null")
		case KindBool:
			b, err := t.ReadBool()
			if err != nil {
				return err
			}
			out = append(out, "bool:"+strconv.FormatBool(b))
		case KindString:
			s, err := t.ReadString()
			if err != nil {
				return err
			}
			out = append(out, "str:"+string(s))
		case KindNumber:
			n, err := t.ReadNumber()
			if err != nil {
				return err
			}
			out = append(out, "num:"+string(n))
		case KindObject:
			if err := t.BeginObject(); err != nil {
				return err
			}
			out = append(out, "delim:{")
			for {
				kk, err := t.Peek()
				if err != nil {
					return err
				}
				if kk == KindEnd {
					if err := t.EndObject(); err != nil {
						return err
					}
					out = append(out, "delim:}")
					return nil
				}
				if err := walk(); err != nil { // the key
					return err
				}
				if err := walk(); err != nil { // the value
					return err
				}
			}
		case KindArray:
			if err := t.BeginArray(); err != nil {
				return err
			}
			out = append(out, "delim:[")
			for {
				kk, err := t.Peek()
				if err != nil {
					return err
				}
				if kk == KindEnd {
					if err := t.EndArray(); err != nil {
						return err
					}
					out = append(out, "delim:]")
					return nil
				}
				if err := walk(); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(); err != nil {
		return out, err
	}
	return out, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTokenScalarValues(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`null`, "null"},
		{`true`, "bool:true"},
		{`false`, "bool:false"},
		{`"x"`, "str:x"},
		{`""`, "str:"},
		{`"a\"b"`, `str:a\"b`}, // escape sequences are left intact
		{`"\n\t"`, `str:\n\t`}, // ... including the control escapes
		{`"\u00e9"`, `str:\u00e9`},
		{`0`, "num:0"},
		{`-0`, "num:-0"},
		{`-1.5e3`, "num:-1.5e3"},
		{`123.456e-789`, "num:123.456e-789"},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			tok := NewTokenizer([]byte(tt.in))
			got, err := tok.Token()
			if err != nil {
				t.Fatalf("Token() error = %v, want nil", err)
			}
			if d := tokenDesc(got); d != tt.want {
				t.Fatalf("Token() = %s, want %s", d, tt.want)
			}
			if _, err := tok.Token(); !errors.Is(err, io.EOF) {
				t.Fatalf("second Token() error = %v, want io.EOF", err)
			}
		})
	}
}

// TestTokenStringValueIsNotNumber guards the type union: a JSON string is a
// string and a JSON number is a Number, never the other way round.
func TestTokenStringValueIsNotNumber(t *testing.T) {
	tok := NewTokenizer([]byte(`"1"`))
	got, err := tok.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if _, ok := got.(string); !ok {
		t.Fatalf("Token() of a string = %T, want string", got)
	}

	tok = NewTokenizer([]byte(`1`))
	got, err = tok.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if _, ok := got.(Number); !ok {
		t.Fatalf("Token() of a number = %T, want Number", got)
	}
}

func TestTokenObjectKeysAndValues(t *testing.T) {
	const in = `{"a":1,"b":"two"}`
	want := []string{"delim:{", "str:a", "num:1", "str:b", "str:two", "delim:}"}

	tok := NewTokenizer([]byte(in))
	got, err := tokenStream(tok)
	if err != nil {
		t.Fatalf("tokenStream(%q) = %v", in, err)
	}
	if !equalStrings(got, want) {
		t.Fatalf("tokenStream(%q):\n got %v\nwant %v", in, got, want)
	}
}

func TestTokenNested(t *testing.T) {
	const in = `{"a":[1,{"b":[true,null]}],"c":{}}`
	want := []string{
		"delim:{", "str:a",
		"delim:[", "num:1",
		"delim:{", "str:b",
		"delim:[", "bool:true", "null", "delim:]",
		"delim:}", "delim:]",
		"str:c", "delim:{", "delim:}",
		"delim:}",
	}

	tok := NewTokenizer([]byte(in))
	got, err := tokenStream(tok)
	if err != nil {
		t.Fatalf("tokenStream(%q) = %v", in, err)
	}
	if !equalStrings(got, want) {
		t.Fatalf("tokenStream(%q):\n got %v\nwant %v", in, got, want)
	}
}

func TestTokenEOF(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{`null`, []string{"null"}},
		{`{}`, []string{"delim:{", "delim:}"}},
		{`[]`, []string{"delim:[", "delim:]"}},
		{`[[]]`, []string{"delim:[", "delim:[", "delim:]", "delim:]"}},
		{"  \n42 ", []string{"num:42"}},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			tok := NewTokenizer([]byte(tt.in))
			got, err := tokenStream(tok)
			if err != nil {
				t.Fatalf("tokenStream(%q) = %v", tt.in, err)
			}
			if !equalStrings(got, tt.want) {
				t.Fatalf("tokenStream(%q) = %v, want %v", tt.in, got, tt.want)
			}
			// Once at the end, Token keeps reporting io.EOF.
			if _, err := tok.Token(); !errors.Is(err, io.EOF) {
				t.Fatalf("Token() at end of %q = %v, want io.EOF", tt.in, err)
			}
		})
	}
}

// TestTokenMatchesPeekStream is the differential check: over every valid
// document, the Token stream and the Peek/Read* stream agree token for token.
func TestTokenMatchesPeekStream(t *testing.T) {
	for _, in := range validDocuments {
		t.Run(strconv.Quote(in), func(t *testing.T) {
			want, err := peekStream(NewTokenizer([]byte(in)))
			if err != nil {
				t.Fatalf("peekStream(%q) = %v", in, err)
			}
			got, err := tokenStream(NewTokenizer([]byte(in)))
			if err != nil {
				t.Fatalf("tokenStream(%q) = %v", in, err)
			}
			if !equalStrings(got, want) {
				t.Fatalf("tokenStream(%q):\n got %v\nwant %v", in, got, want)
			}
		})
	}
}

// TestTokenInvalidMatchesValid requires the Token stream to fail with the same
// message and 1-based position as validate reports for every invalid document.
func TestTokenInvalidMatchesValid(t *testing.T) {
	for _, tt := range invalidDocuments {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			tok := NewTokenizer([]byte(tt.in))
			_, err := tokenStream(tok)
			if err == nil {
				t.Fatalf("tokenStream(%q) = nil, want error %q at %d", tt.in, tt.problem, tt.at)
			}
			pe, ok := err.(parseError)
			if !ok {
				t.Fatalf("tokenStream(%q) error type = %T, want parseError", tt.in, err)
			}
			ve, failed := validate([]byte(tt.in))
			if !failed {
				t.Fatalf("validate(%q) succeeded, want failure", tt.in)
			}
			if pe.at != ve.at || pe.problem != ve.problem {
				t.Errorf("token error = %d: %q, validate = %d: %q", pe.at, pe.problem, ve.at, ve.problem)
			}
		})
	}
}

// TestTokenNewDecoderMultiValue requires Token over a reader-backed tokenizer to
// read several top-level values from one stream (NDJSON) and to report io.EOF at
// the end. The reader hands out as little as one byte per Read.
func TestTokenNewDecoderMultiValue(t *testing.T) {
	const in = "{\"a\":1}\n[2,3]\ntrue\n"
	want := []string{
		"delim:{", "str:a", "num:1", "delim:}",
		"delim:[", "num:2", "num:3", "delim:]",
		"bool:true",
	}

	for _, chunk := range []int{1, 2, 3, 7, len(in) + 1} {
		tok := NewDecoder(&chunkReader{data: []byte(in), chunk: chunk})
		got, err := tokenStream(tok)
		if err != nil {
			t.Fatalf("chunk=%d: tokenStream(%q) = %v", chunk, in, err)
		}
		if !equalStrings(got, want) {
			t.Fatalf("chunk=%d: tokenStream(%q):\n got %v\nwant %v", chunk, in, got, want)
		}
		if err := tok.Err(); err != nil {
			t.Fatalf("chunk=%d: Err() at end = %v, want nil", chunk, err)
		}
	}
}

// TestTokenNewDecoderMatchesSlice requires the reader-backed Token stream to
// match the slice-backed one for every valid document, at several chunk sizes.
func TestTokenNewDecoderMatchesSlice(t *testing.T) {
	for _, in := range validDocuments {
		want, err := tokenStream(NewTokenizer([]byte(in)))
		if err != nil {
			t.Fatalf("slice tokenStream(%q) = %v", in, err)
		}
		for _, chunk := range []int{1, 2, 3, 5, 7} {
			tok := NewDecoder(&chunkReader{data: []byte(in), chunk: chunk})
			got, err := tokenStream(tok)
			if err != nil {
				t.Fatalf("NewDecoder tokenStream(%q, chunk=%d) = %v", in, chunk, err)
			}
			if !equalStrings(got, want) {
				t.Fatalf("NewDecoder tokenStream(%q, chunk=%d):\n got %v\nwant %v", in, chunk, got, want)
			}
		}
	}
}

// TestTokenNewDecoderReaderError requires a non-EOF read error to surface,
// wrapped, rather than being mistaken for the end of the stream.
func TestTokenNewDecoderReaderError(t *testing.T) {
	boom := errors.New("boom")
	tok := NewDecoder(&errReader{data: []byte(`{"a":1`), err: boom})

	_, err := tokenStream(tok)
	if !errors.Is(err, boom) {
		t.Fatalf("tokenStream() = %v, want it to wrap boom", err)
	}
}

// TestTokenNewDecoderNilReader requires a nil reader to be reported, not to
// panic.
func TestTokenNewDecoderNilReader(t *testing.T) {
	tok := NewDecoder(nil)
	if _, err := tok.Token(); err == nil {
		t.Fatal("Token() with a nil reader = nil, want an error")
	}
}
