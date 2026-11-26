package scanner

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

// drive consumes a whole document with the tokenizer, exercising Peek, the
// Read/Begin/End methods and Skip. It returns the first error, or nil once the
// top-level value and the trailing whitespace are consumed.
func drive(t *Tokenizer) error {
	if err := t.Skip(); err != nil {
		return err
	}
	_, err := t.Peek()
	return err
}

func TestTokenizerPeekKindPerValue(t *testing.T) {
	tests := []struct {
		in   string
		want Kind
	}{
		{`null`, KindNull},
		{`true`, KindBool},
		{`false`, KindBool},
		{`"x"`, KindString},
		{`0`, KindNumber},
		{`-1.5e3`, KindNumber},
		{`{}`, KindObject},
		{`{"a":1}`, KindObject},
		{`[]`, KindArray},
		{`[1,2]`, KindArray},
		{" \t\n 42 ", KindNumber},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			tok := NewTokenizer([]byte(tt.in))
			got, err := tok.Peek()
			if err != nil {
				t.Fatalf("Peek() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("Peek() = %v, want %v", got, tt.want)
			}
			// Peek does not consume: a second Peek reports the same kind.
			if again, err := tok.Peek(); err != nil || again != tt.want {
				t.Fatalf("second Peek() = %v, %v, want %v, nil", again, err, tt.want)
			}
		})
	}
}

func TestTokenizerReadStringKeepsEscapes(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`""`, ``},
		{`"abc"`, `abc`},
		{`"a\"b"`, `a\"b`},
		{`"\n\t"`, `\n\t`},
		{`"\u00e9"`, `\u00e9`},
		{`"héllo"`, `héllo`},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			tok := NewTokenizer([]byte(tt.in))
			got, err := tok.ReadString()
			if err != nil {
				t.Fatalf("ReadString() error = %v, want nil", err)
			}
			if string(got) != tt.want {
				t.Fatalf("ReadString() = %q, want %q", got, tt.want)
			}
			if _, err := tok.Peek(); err != nil {
				t.Fatalf("Peek() after ReadString() error = %v, want KindEnd", err)
			}
		})
	}
}

func TestTokenizerReadNumberIsRawLiteral(t *testing.T) {
	for _, in := range []string{`0`, `-0`, `1.5`, `-1.5e-3`, `1E+10`, `123.456e-789`} {
		t.Run(in, func(t *testing.T) {
			tok := NewTokenizer([]byte(in))
			got, err := tok.ReadNumber()
			if err != nil {
				t.Fatalf("ReadNumber() error = %v, want nil", err)
			}
			if string(got) != in {
				t.Fatalf("ReadNumber() = %q, want %q", got, in)
			}
		})
	}
}

func TestTokenizerReadBoolAndNull(t *testing.T) {
	tok := NewTokenizer([]byte(`true`))
	if v, err := tok.ReadBool(); err != nil || !v {
		t.Fatalf("ReadBool(true) = %v, %v, want true, nil", v, err)
	}

	tok = NewTokenizer([]byte(`false`))
	if v, err := tok.ReadBool(); err != nil || v {
		t.Fatalf("ReadBool(false) = %v, %v, want false, nil", v, err)
	}

	tok = NewTokenizer([]byte(`null`))
	if err := tok.ReadNull(); err != nil {
		t.Fatalf("ReadNull() error = %v, want nil", err)
	}
}

func TestTokenizerBeginEndObject(t *testing.T) {
	tok := NewTokenizer([]byte(`{"a":1,"b":"two"}`))

	if err := tok.BeginObject(); err != nil {
		t.Fatalf("BeginObject() error = %v", err)
	}

	wantKeys := []string{`a`, `b`}
	for _, wantKey := range wantKeys {
		k, err := tok.Peek()
		if err != nil {
			t.Fatalf("Peek() key error = %v", err)
		}
		if k != KindString {
			t.Fatalf("Peek() key = %v, want %v", k, KindString)
		}
		key, err := tok.ReadString()
		if err != nil {
			t.Fatalf("ReadString() key error = %v", err)
		}
		if string(key) != wantKey {
			t.Fatalf("key = %q, want %q", key, wantKey)
		}
		if err := tok.Skip(); err != nil {
			t.Fatalf("Skip() value error = %v", err)
		}
	}

	k, err := tok.Peek()
	if err != nil {
		t.Fatalf("Peek() end error = %v", err)
	}
	if k != KindEnd {
		t.Fatalf("Peek() at object end = %v, want %v", k, KindEnd)
	}
	if err := tok.EndObject(); err != nil {
		t.Fatalf("EndObject() error = %v", err)
	}
}

func TestTokenizerBeginEndArray(t *testing.T) {
	tok := NewTokenizer([]byte(`[1,true,"x",null,[]]`))

	if err := tok.BeginArray(); err != nil {
		t.Fatalf("BeginArray() error = %v", err)
	}

	wantKinds := []Kind{KindNumber, KindBool, KindString, KindNull, KindArray}
	for i, want := range wantKinds {
		k, err := tok.Peek()
		if err != nil {
			t.Fatalf("element %d: Peek() error = %v", i, err)
		}
		if k != want {
			t.Fatalf("element %d: Peek() = %v, want %v", i, k, want)
		}
		if err := tok.Skip(); err != nil {
			t.Fatalf("element %d: Skip() error = %v", i, err)
		}
	}

	k, err := tok.Peek()
	if err != nil {
		t.Fatalf("Peek() end error = %v", err)
	}
	if k != KindEnd {
		t.Fatalf("Peek() at array end = %v, want %v", k, KindEnd)
	}
	if err := tok.EndArray(); err != nil {
		t.Fatalf("EndArray() error = %v", err)
	}
}

func TestTokenizerSkipNested(t *testing.T) {
	const in = `{"a":[1,{"b":[true,null,"x"]}],"c":{}}`

	tok := NewTokenizer([]byte(in))
	if err := tok.Skip(); err != nil {
		t.Fatalf("Skip() error = %v", err)
	}

	k, err := tok.Peek()
	if err != nil {
		t.Fatalf("Peek() after Skip() error = %v", err)
	}
	if k != KindEnd {
		t.Fatalf("Peek() after Skip() = %v, want %v", k, KindEnd)
	}
	if got := tok.Offset(); got != int64(len(in)) {
		t.Fatalf("Offset() after Skip() = %d, want %d", got, len(in))
	}
}

func TestTokenizerReadRawSpansWholeValue(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`true`, `true`},
		{`"a\"b"`, `"a\"b"`},
		{`-1.5e3`, `-1.5e3`},
		{`{}`, `{}`},
		{`[1,2]`, `[1,2]`},
		{`{"a":[1,{"b":2}],"c":null}`, `{"a":[1,{"b":2}],"c":null}`},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			tok := NewTokenizer([]byte(tt.in))
			got, err := tok.ReadRaw()
			if err != nil {
				t.Fatalf("ReadRaw() error = %v, want nil", err)
			}
			if string(got) != tt.want {
				t.Fatalf("ReadRaw() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTokenizerOffsetTracksBytesConsumed(t *testing.T) {
	tok := NewTokenizer([]byte(`{"a":1}`))
	if got := tok.Offset(); got != 0 {
		t.Fatalf("Offset() at start = %d, want 0", got)
	}
	if err := tok.BeginObject(); err != nil {
		t.Fatalf("BeginObject() error = %v", err)
	}
	if got := tok.Offset(); got != 1 {
		t.Fatalf("Offset() after BeginObject() = %d, want 1", got)
	}
	if _, err := tok.ReadString(); err != nil { // the key also consumes ':'
		t.Fatalf("ReadString() key error = %v", err)
	}
	if got := tok.Offset(); got != 5 {
		t.Fatalf("Offset() after key = %d, want 5", got)
	}
	if _, err := tok.ReadNumber(); err != nil {
		t.Fatalf("ReadNumber() error = %v", err)
	}
	if got := tok.Offset(); got != 6 {
		t.Fatalf("Offset() after value = %d, want 6", got)
	}
	if err := tok.EndObject(); err != nil {
		t.Fatalf("EndObject() error = %v", err)
	}
	if got := tok.Offset(); got != 7 {
		t.Fatalf("Offset() after EndObject() = %d, want 7", got)
	}
}

func TestTokenizerErrorsMatchValid(t *testing.T) {
	for _, tt := range invalidDocuments {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			wantAt := tt.at
			var wantProblem = tt.problem

			tok := NewTokenizer([]byte(tt.in))
			err := drive(tok)
			if err == nil {
				t.Fatalf("drive(%q) = nil, want error %q at %d", tt.in, wantProblem, wantAt)
			}

			pe, ok := err.(parseError)
			if !ok {
				t.Fatalf("drive(%q) error type = %T, want parseError", tt.in, err)
			}
			if pe.at != wantAt {
				t.Errorf("error position = %d, want %d", pe.at, wantAt)
			}
			if !strings.Contains(pe.problem, wantProblem) {
				t.Errorf("problem = %q, want it to contain %q", pe.problem, wantProblem)
			}

			// The same position and problem as validate reports.
			ve, failed := validate([]byte(tt.in))
			if !failed {
				t.Fatalf("validate(%q) succeeded, want failure", tt.in)
			}
			if pe.at != ve.at || pe.problem != ve.problem {
				t.Errorf("tokenizer error = %d: %q, validate = %d: %q", pe.at, pe.problem, ve.at, ve.problem)
			}
		})
	}
}

func TestTokenizerAgreesWithValid(t *testing.T) {
	for _, in := range validDocuments {
		t.Run("valid/"+strconv.Quote(in), func(t *testing.T) {
			tok := NewTokenizer([]byte(in))
			if err := drive(tok); err != nil {
				t.Fatalf("drive(%q) = %v, want nil", in, err)
			}
			if k, err := tok.Peek(); err != nil || k != KindEnd {
				t.Fatalf("Peek() at end of %q = %v, %v, want %v, nil", in, k, err, KindEnd)
			}
		})
	}
}

func TestTokenizerKindString(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{KindNull, "null"},
		{KindBool, "boolean"},
		{KindString, "string"},
		{KindNumber, "number"},
		{KindObject, "object"},
		{KindArray, "array"},
		{KindEnd, "end of container"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestTokenizerSkipAndReadRawRejectKeyPosition(t *testing.T) {
	tok := NewTokenizer([]byte(`{"a":1}`))
	if err := tok.BeginObject(); err != nil {
		t.Fatalf("BeginObject() error = %v", err)
	}
	// Positioned at the key, not a value.
	if err := tok.Skip(); err == nil {
		t.Fatal("Skip() at key position = nil, want a misuse error")
	}

	tok = NewTokenizer([]byte(`{"a":1}`))
	if err := tok.BeginObject(); err != nil {
		t.Fatalf("BeginObject() error = %v", err)
	}
	if _, err := tok.ReadRaw(); err == nil {
		t.Fatal("ReadRaw() at key position = nil, want a misuse error")
	}
}

func TestTokenizerErrIsSticky(t *testing.T) {
	tok := NewTokenizer([]byte(`{"a":`))
	if err := drive(tok); err == nil {
		t.Fatal("drive() error = nil, want end-of-input error")
	}
	if tok.Err() == nil {
		t.Fatal("Err() = nil, want the recorded error")
	}
	// A later call must not clear the failure.
	if _, err := tok.Peek(); err == nil {
		t.Fatal("Peek() after failure = nil, want the sticky error")
	}
	if err := tok.Err(); err == nil {
		t.Fatal("second Err() = nil, want the sticky error")
	}
}

// chunkReader hands out at most chunk bytes per Read, so a document can be
// forced to straddle a refill boundary at every value.
type chunkReader struct {
	data  []byte
	chunk int
	pos   int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := r.chunk
	if n > len(p) {
		n = len(p)
	}
	if n > len(r.data)-r.pos {
		n = len(r.data) - r.pos
	}
	copy(p, r.data[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

// errReader yields its data, then fails with err instead of returning io.EOF.
type errReader struct {
	data []byte
	pos  int
	err  error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// traceOp is one recorded tokenizer step: the token kind, a detail string (the
// scalar content or an object key) and the Offset at which the step was taken.
type traceOp struct {
	kind   Kind
	detail string
	offset int64
}

// traceValue walks one value with the typed readers, recording every step. It
// descends into composites so the whole document is exercised.
func traceValue(t *Tokenizer, tr *[]traceOp) error {
	k, err := t.Peek()
	if err != nil {
		return err
	}
	*tr = append(*tr, traceOp{kind: k, offset: t.Offset()})
	last := func() *traceOp { return &(*tr)[len(*tr)-1] }

	switch k {
	case KindNull:
		return t.ReadNull()
	case KindBool:
		v, err := t.ReadBool()
		if err != nil {
			return err
		}
		last().detail = strconv.FormatBool(v)
		return nil
	case KindString:
		s, err := t.ReadString()
		if err != nil {
			return err
		}
		last().detail = string(s)
		return nil
	case KindNumber:
		n, err := t.ReadNumber()
		if err != nil {
			return err
		}
		last().detail = string(n)
		return nil
	case KindObject:
		if err := t.BeginObject(); err != nil {
			return err
		}
		for {
			kk, err := t.Peek()
			if err != nil {
				return err
			}
			if kk == KindEnd {
				return t.EndObject()
			}
			*tr = append(*tr, traceOp{kind: kk, offset: t.Offset()})
			key, err := t.ReadString()
			if err != nil {
				return err
			}
			last().detail = string(key)
			if err := traceValue(t, tr); err != nil {
				return err
			}
		}
	case KindArray:
		if err := t.BeginArray(); err != nil {
			return err
		}
		for {
			kk, err := t.Peek()
			if err != nil {
				return err
			}
			if kk == KindEnd {
				return t.EndArray()
			}
			if err := traceValue(t, tr); err != nil {
				return err
			}
		}
	}
	return nil
}

// traceDoc walks a whole document and records the closing KindEnd, so the
// returned trace covers the trailing offset too.
func traceDoc(t *Tokenizer) ([]traceOp, error) {
	var tr []traceOp
	if err := traceValue(t, &tr); err != nil {
		return tr, err
	}
	k, err := t.Peek()
	if err != nil {
		return tr, err
	}
	return append(tr, traceOp{kind: k, offset: t.Offset()}), nil
}

func traceEqual(a, b []traceOp) bool {
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

// TestNewDecoderMatchesSlice drives every valid document through both the
// slice-backed and the reader-backed tokenizer and requires identical Kind
// sequences, scalar values and offsets. The reader hands out as little as one
// byte per Read, so every value straddles a refill boundary.
func TestNewDecoderMatchesSlice(t *testing.T) {
	for _, in := range validDocuments {
		want, err := traceDoc(NewTokenizer([]byte(in)))
		if err != nil {
			t.Fatalf("slice trace(%q) = %v", in, err)
		}
		for _, chunk := range []int{1, 2, 3, 5, 7, len(in) + 1} {
			tok := NewDecoder(&chunkReader{data: []byte(in), chunk: chunk})
			got, err := traceDoc(tok)
			if err != nil {
				t.Fatalf("NewDecoder trace(%q, chunk=%d) = %v", in, chunk, err)
			}
			if !traceEqual(got, want) {
				t.Fatalf("NewDecoder trace(%q, chunk=%d):\n got %v\nwant %v", in, chunk, got, want)
			}
			if off := tok.Offset(); off != int64(len(in)) {
				t.Fatalf("Offset(%q, chunk=%d) = %d, want %d", in, chunk, off, len(in))
			}
		}
	}
}

// TestNewDecoderInvalidMatchesSlice requires the reader-backed tokenizer to
// report the same error message and 1-based position as the slice-backed one for
// every invalid document.
func TestNewDecoderInvalidMatchesSlice(t *testing.T) {
	for _, tt := range invalidDocuments {
		wantErr := drive(NewTokenizer([]byte(tt.in)))
		if wantErr == nil {
			t.Fatalf("slice drive(%q) = nil, want an error", tt.in)
		}
		want, ok := wantErr.(parseError)
		if !ok {
			t.Fatalf("slice drive(%q) error type = %T, want parseError", tt.in, wantErr)
		}

		for _, chunk := range []int{1, 2, 3, 5} {
			tok := NewDecoder(&chunkReader{data: []byte(tt.in), chunk: chunk})
			gotErr := drive(tok)
			if gotErr == nil {
				t.Fatalf("NewDecoder drive(%q, chunk=%d) = nil, want %v", tt.in, chunk, want)
			}
			got, ok := gotErr.(parseError)
			if !ok {
				t.Fatalf("NewDecoder drive(%q, chunk=%d) error type = %T, want parseError", tt.in, chunk, gotErr)
			}
			if got.at != want.at || got.problem != want.problem {
				t.Fatalf("NewDecoder drive(%q, chunk=%d) = {%d %q}, want {%d %q}",
					tt.in, chunk, got.at, got.problem, want.at, want.problem)
			}
		}
	}
}

// TestNewDecoderReadRawMatchesSlice requires ReadRaw over a reader to return the
// same raw bytes and final offset as over a slice.
func TestNewDecoderReadRawMatchesSlice(t *testing.T) {
	for _, in := range validDocuments {
		wantTok := NewTokenizer([]byte(in))
		want, err := wantTok.ReadRaw()
		if err != nil {
			t.Fatalf("slice ReadRaw(%q) = %v", in, err)
		}

		for _, chunk := range []int{1, 2, 3} {
			tok := NewDecoder(&chunkReader{data: []byte(in), chunk: chunk})
			got, err := tok.ReadRaw()
			if err != nil {
				t.Fatalf("NewDecoder ReadRaw(%q, chunk=%d) = %v", in, chunk, err)
			}
			if string(got) != string(want) {
				t.Fatalf("NewDecoder ReadRaw(%q, chunk=%d) = %q, want %q", in, chunk, got, want)
			}
			if tok.Offset() != wantTok.Offset() {
				t.Fatalf("NewDecoder ReadRaw(%q, chunk=%d) Offset = %d, want %d",
					in, chunk, tok.Offset(), wantTok.Offset())
			}
		}
	}
}

// TestNewDecoderReaderError requires a non-EOF read error to surface, wrapped,
// instead of being mistaken for end of input.
func TestNewDecoderReaderError(t *testing.T) {
	boom := errors.New("boom")
	tok := NewDecoder(&errReader{data: []byte(`{"a":1`), err: boom})

	if err := drive(tok); !errors.Is(err, boom) {
		t.Fatalf("drive() = %v, want it to wrap boom", err)
	}
	if err := tok.Err(); !errors.Is(err, boom) {
		t.Fatalf("Err() = %v, want it to wrap boom", err)
	}
}

// TestNewDecoderNilReader requires a nil reader to be reported, not to panic.
func TestNewDecoderNilReader(t *testing.T) {
	tok := NewDecoder(nil)
	if _, err := tok.Peek(); err == nil {
		t.Fatal("Peek() with a nil reader = nil, want an error")
	}
}

// TestNewDecoderManyRefills drives a document far larger than one refill chunk
// through a one-byte reader, exercising many refills.
func TestNewDecoderManyRefills(t *testing.T) {
	const in = `{"a":[1,2,3],"b":{"c":true},"d":null,"e":"x"}`

	want, err := traceDoc(NewTokenizer([]byte(in)))
	if err != nil {
		t.Fatalf("slice trace = %v", err)
	}

	tok := NewDecoder(&chunkReader{data: []byte(in), chunk: 1})
	got, err := traceDoc(tok)
	if err != nil {
		t.Fatalf("NewDecoder trace = %v", err)
	}
	if !traceEqual(got, want) {
		t.Fatalf("NewDecoder trace:\n got %v\nwant %v", got, want)
	}

	// A large document that needs several 4096-byte refills must still tokenize
	// identically to the slice-backed tokenizer.
	big := buildArrayDocument(2000)
	wantBig, err := traceDoc(NewTokenizer(big))
	if err != nil {
		t.Fatalf("slice big trace = %v", err)
	}
	gotBig, err := traceDoc(NewDecoder(&chunkReader{data: big, chunk: 997}))
	if err != nil {
		t.Fatalf("NewDecoder big trace = %v", err)
	}
	if !traceEqual(gotBig, wantBig) {
		t.Fatal("NewDecoder big trace differs from the slice-backed trace")
	}
}

// TestNewDecoderEmptyReader requires an empty reader to report the same
// end-of-input error as an empty slice.
func TestNewDecoderEmptyReader(t *testing.T) {
	wantErr := drive(NewTokenizer(nil))
	if wantErr == nil {
		t.Fatal("slice drive(nil) = nil, want an error")
	}
	gotErr := drive(NewDecoder(&chunkReader{data: nil, chunk: 1}))
	if gotErr == nil {
		t.Fatalf("NewDecoder drive(empty) = nil, want %v", wantErr)
	}
	if gotErr.Error() != wantErr.Error() {
		t.Fatalf("NewDecoder drive(empty) = %q, want %q", gotErr, wantErr)
	}
}
