package scanner

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

// requireInvalid asserts validate rejects in with a problem containing want.
func requireInvalid(t *testing.T, in, want string) {
	t.Helper()

	err, failed := validate([]byte(in))
	if !failed {
		t.Fatalf("validate(%q) = success, want an error containing %q", in, want)
	}
	if !strings.Contains(err.problem, want) {
		t.Errorf("validate(%q) problem = %q, want it to contain %q", in, err.problem, want)
	}
}

// TestCovValidatorMemberKeyFailures drives the two object-member key paths in
// run: the first member after '{' and every member after a ',', including the
// end-of-input and missing-colon branches that follow a key.
func TestCovValidatorMemberKeyFailures(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// First member key after '{' fails inside scanString.
		{`{"\q":1}`, "in string escape code"},
		// A later member key after ',' fails inside scanString.
		{`{"a":1,"\q":2}`, "in string escape code"},
		// A key after ',' is followed by end of input.
		{`{"a":1,"b"`, "unexpected end of JSON input"},
		// A key after ',' is not followed by ':'.
		{`{"a":1,"b"2}`, "after object key"},
	}
	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			requireInvalid(t, tt.in, tt.want)
		})
	}
}

// TestCovKindStringDefault checks the fallback name for an out-of-range kind.
func TestCovKindStringDefault(t *testing.T) {
	if got := Kind(99).String(); got != "unknown" {
		t.Errorf("Kind(99).String() = %q, want %q", got, "unknown")
	}
}

// TestCovTokenizerTypedReadMismatches drives every typed read and both Begin*
// against the wrong kind, so each mismatch branch and mismatch itself run, and
// kindNoun is reached for both container kinds.
func TestCovTokenizerTypedReadMismatches(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		call func(*Tokenizer) error
		want string
	}{
		{"ReadNull", `1`, func(tk *Tokenizer) error { return tk.ReadNull() }, "expected null, found number"},
		{"ReadBool", `1`, func(tk *Tokenizer) error { _, err := tk.ReadBool(); return err }, "expected boolean, found number"},
		{"ReadString", `1`, func(tk *Tokenizer) error { _, err := tk.ReadString(); return err }, "expected string, found number"},
		{"ReadNumber", `"a"`, func(tk *Tokenizer) error { _, err := tk.ReadNumber(); return err }, "expected number, found string"},
		{"BeginObject", `1`, func(tk *Tokenizer) error { return tk.BeginObject() }, "expected object, found number"},
		{"BeginArray", `1`, func(tk *Tokenizer) error { return tk.BeginArray() }, "expected array, found number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(NewTokenizer([]byte(tt.doc)))
			if err == nil {
				t.Fatalf("%s: want an error", tt.name)
			}
			if err.Error() != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, err.Error(), tt.want)
			}
		})
	}
}

// TestCovTokenizerPeekErrors drives the reads, Begin* and End* on empty input,
// where the classification peek itself fails.
func TestCovTokenizerPeekErrors(t *testing.T) {
	tests := []struct {
		name string
		call func(*Tokenizer) error
	}{
		{"ReadNull", func(tk *Tokenizer) error { return tk.ReadNull() }},
		{"ReadBool", func(tk *Tokenizer) error { _, err := tk.ReadBool(); return err }},
		{"ReadString", func(tk *Tokenizer) error { _, err := tk.ReadString(); return err }},
		{"ReadNumber", func(tk *Tokenizer) error { _, err := tk.ReadNumber(); return err }},
		{"ReadRaw", func(tk *Tokenizer) error { _, err := tk.ReadRaw(); return err }},
		{"BeginObject", func(tk *Tokenizer) error { return tk.BeginObject() }},
		{"BeginArray", func(tk *Tokenizer) error { return tk.BeginArray() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(NewTokenizer(nil)); err == nil {
				t.Errorf("%s on empty input: want an error", tt.name)
			}
		})
	}
}

// TestCovTokenizerEndErrors drives end's branches: a missing container, a frame
// of the wrong kind, a matching frame whose peek fails, a matching frame whose
// next token is not the end, and a pre-existing failure.
func TestCovTokenizerEndErrors(t *testing.T) {
	// No container open: misuse naming the expected end (kindNoun both kinds).
	if err := NewTokenizer([]byte(`1`)).EndObject(); err == nil {
		t.Error("EndObject with no open object: want an error")
	}
	if err := NewTokenizer([]byte(`1`)).EndArray(); err == nil {
		t.Error("EndArray with no open array: want an error")
	}

	// A frame of the other kind: end reports the mismatch (kindNoun array).
	tok := NewTokenizer([]byte(`[[`))
	if err := tok.BeginArray(); err != nil {
		t.Fatalf("BeginArray: %v", err)
	}
	if err := tok.EndArray(); err == nil {
		t.Error("EndArray closing an array that is still open: want an error")
	}

	// A matching frame whose peek fails: the tokenizer is truncated.
	tok = NewTokenizer([]byte(`[`))
	if err := tok.BeginArray(); err != nil {
		t.Fatalf("BeginArray: %v", err)
	}
	if err := tok.EndArray(); err == nil {
		t.Error("EndArray on a truncated array: want an error")
	}

	// A pre-existing failure short-circuits end before it peeks.
	tok = NewTokenizer([]byte(`{"a":}`))
	if err := tok.BeginObject(); err != nil {
		t.Fatalf("BeginObject: %v", err)
	}
	if _, err := tok.ReadString(); err != nil {
		t.Fatalf("ReadString(key): %v", err)
	}
	if _, err := tok.Peek(); err == nil {
		t.Fatal("Peek at '}': want an error")
	}
	if err := tok.EndObject(); err == nil {
		t.Error("EndObject after a failure: want the sticky error")
	}
}

// TestCovTokenizerReadRawAtEnd checks ReadRaw reports a misuse when the next
// token closes the current container.
func TestCovTokenizerReadRawAtEnd(t *testing.T) {
	tok := NewTokenizer([]byte(`[]`))
	if err := tok.BeginArray(); err != nil {
		t.Fatalf("BeginArray: %v", err)
	}
	if _, err := tok.ReadRaw(); err == nil {
		t.Error("ReadRaw at ']': want a misuse error")
	}
}

// TestCovTokenizerSkipAtEnd checks Skip reports a misuse when the next token
// closes the current container.
func TestCovTokenizerSkipAtEnd(t *testing.T) {
	tok := NewTokenizer([]byte(`[]`))
	if err := tok.BeginArray(); err != nil {
		t.Fatalf("BeginArray: %v", err)
	}
	if err := tok.Skip(); err == nil {
		t.Error("Skip at ']': want a misuse error")
	}
}

// errDataReader returns its data together with err from the first Read, then
// err on every Read after. A non-EOF error alongside data is legal for an
// io.Reader and forces the tokenizer's read loops to surface the reader error.
type errDataReader struct {
	data []byte
	err  error
}

func (r *errDataReader) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = nil
	if n > 0 {
		return n, r.err
	}
	return 0, r.err
}

// TestCovTokenizerReaderErrorsInReads drives the read loops' reader-error branch:
// a value that the reader delivers together with a non-EOF error.
func TestCovTokenizerReaderErrorsInReads(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		doc  string
		// setup performs any Begin* needed before call.
		setup func(*Tokenizer) error
		call  func(*Tokenizer) error
	}{
		{
			name: "readString value",
			doc:  `"abc`,
			call: func(tk *Tokenizer) error { _, err := tk.ReadString(); return err },
		},
		{
			name:  "readString key colon",
			doc:   `{"a"`,
			setup: func(tk *Tokenizer) error { return tk.BeginObject() },
			call:  func(tk *Tokenizer) error { _, err := tk.ReadString(); return err },
		},
		{
			name: "readNumber",
			doc:  `1e`,
			call: func(tk *Tokenizer) error { _, err := tk.ReadNumber(); return err },
		},
		{
			name: "readBool literal",
			doc:  `tru`,
			call: func(tk *Tokenizer) error { _, err := tk.ReadBool(); return err },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := NewDecoder(&errDataReader{data: []byte(tt.doc), err: boom})
			if tt.setup != nil {
				if err := tt.setup(tok); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			err := tt.call(tok)
			if !errors.Is(err, boom) {
				t.Errorf("%s error = %v, want it to wrap boom", tt.name, err)
			}
		})
	}
}

// TestCovTokenizerDeepFrames drives the overflow frame stack: a document nested
// deeper than nestingInline uses frame's overflow branch and grows the overflow
// slice, while still tokenizing correctly.
func TestCovTokenizerDeepFrames(t *testing.T) {
	const depth = nestingInline + 33 // deep enough to make push grow overflow twice
	doc := strings.Repeat("[", depth) + "1" + strings.Repeat("]", depth)

	tok := NewTokenizer([]byte(doc))
	for i := 0; i < depth; i++ {
		if err := tok.BeginArray(); err != nil {
			t.Fatalf("BeginArray %d: %v", i, err)
		}
	}
	if n, err := tok.ReadNumber(); err != nil {
		t.Fatalf("ReadNumber: %v", err)
	} else if string(n) != "1" {
		t.Fatalf("ReadNumber = %q, want 1", n)
	}
	for i := 0; i < depth; i++ {
		if err := tok.EndArray(); err != nil {
			t.Fatalf("EndArray %d: %v", i, err)
		}
	}
	if k, err := tok.Peek(); err != nil || k != KindEnd {
		t.Errorf("final Peek = %v, %v, want KindEnd, nil", k, err)
	}
}

// TestCovTokenizerNilReaderErrors checks NewDecoder(nil) reports the nil-reader
// error rather than panicking, through every entry point.
func TestCovTokenizerNilReaderErrors(t *testing.T) {
	if _, err := NewDecoder(nil).Peek(); !errors.Is(err, errNilReader) {
		t.Errorf("Peek = %v, want errNilReader", err)
	}
	if err := NewDecoder(nil).Err(); !errors.Is(err, errNilReader) {
		t.Errorf("Err = %v, want errNilReader", err)
	}
}

// covCountingReader returns its data in one-byte reads, so every value straddles
// a refill boundary.
type covCountingReader struct {
	data []byte
	pos  int
}

func (r *covCountingReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}

// TestCovTokenizerRefillPaths drives refill at every value, plus the multi-value
// stream at the top level, so peek's refill branches and a reader-backed
// tokenizer's top-level continuation run.
func TestCovTokenizerRefillPaths(t *testing.T) {
	for _, in := range []string{`{"a":[1,2,{"b":true}]}`, `1 2 3`, `true false`, `"x" "y"`} {
		tok := NewDecoder(&covCountingReader{data: []byte(in)})
		for {
			k, err := tok.Peek()
			if err != nil {
				t.Fatalf("Peek(%q) = %v", in, err)
			}
			if k == KindEnd {
				break
			}
			if err := tok.Skip(); err != nil {
				t.Fatalf("Skip(%q) = %v", in, err)
			}
		}
	}
}
