package qijson_test

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"testing"

	"github.com/qi-lore/qi-json"
	"github.com/qi-lore/qi-json/internal/codec"
	"github.com/qi-lore/qi-json/internal/scanner"
)

// The facade must re-export the engine types identically, so external code sees
// the same types the engine defines. These compile-time assertions fail if an
// alias drifts to a distinct type.
var (
	_ *codec.Reader    = (*qijson.Reader)(nil)
	_ *codec.Writer    = (*qijson.Writer)(nil)
	_ *codec.Error     = (*qijson.Error)(nil)
	_ *codec.TypeError = (*qijson.TypeError)(nil)
	_ codec.ErrorKind  = qijson.KindSyntax
	_ qijson.Decoder   = (*smokeConfig)(nil)
	_ qijson.Encoder   = (*smokeConfig)(nil)
)

// The syntax engine is re-exported through the same facade: Kind and Tokenizer
// are aliases of the internal scanner's, and the seven Kind constants are
// values of that type, so Reader.Peek's result compares equal to them.
var (
	_ scanner.Kind                      = qijson.KindNull
	_ qijson.Kind                       = scanner.KindEnd
	_ *scanner.Tokenizer                = (*qijson.Tokenizer)(nil)
	_ func([]byte) bool                 = qijson.Valid
	_ func([]byte) *qijson.Tokenizer    = qijson.NewTokenizer
	_ func(io.Reader) *qijson.Tokenizer = qijson.NewDecoder

	_ qijson.Kind = qijson.KindNull
	_ qijson.Kind = qijson.KindBool
	_ qijson.Kind = qijson.KindString
	_ qijson.Kind = qijson.KindNumber
	_ qijson.Kind = qijson.KindObject
	_ qijson.Kind = qijson.KindArray
	_ qijson.Kind = qijson.KindEnd
)

// The token-level streaming surface is re-exported through the facade: Token,
// Number and Delim are aliases of the engine's, so a token type-asserted here is
// identical to the engine's, and Tokenizer.Token carries the engine's signature.
var (
	_ qijson.Token  = qijson.Number("1")
	_ qijson.Token  = qijson.Delim('{')
	_ scanner.Token = qijson.Number("1")
	_ qijson.Number = scanner.Number("1")
	_ qijson.Delim  = scanner.Delim('{')

	_ func(*qijson.Tokenizer) (qijson.Token, error) = (*qijson.Tokenizer).Token
)

// These compile-time assertions pin the signature of every constructor and entry
// point. They are the facade's surface contract: a changed parameter, result or
// type parameter fails the build rather than silently changing the public API.
var (
	_ func(string, func(*smokeConfig) *string) qijson.Field[smokeConfig]   = qijson.String[smokeConfig]
	_ func(string, func(*smokeConfig) **string) qijson.Field[smokeConfig]  = qijson.StringPtr[smokeConfig]
	_ func(string, func(*smokeConfig) *int64) qijson.Field[smokeConfig]    = qijson.Int[smokeConfig]
	_ func(string, func(*smokeConfig) **int64) qijson.Field[smokeConfig]   = qijson.IntPtr[smokeConfig]
	_ func(string, func(*smokeConfig) *float64) qijson.Field[smokeConfig]  = qijson.Float[smokeConfig]
	_ func(string, func(*smokeConfig) **float64) qijson.Field[smokeConfig] = qijson.FloatPtr[smokeConfig]
	_ func(string, func(*smokeConfig) *bool) qijson.Field[smokeConfig]     = qijson.Bool[smokeConfig]
	_ func(string, func(*smokeConfig) **bool) qijson.Field[smokeConfig]    = qijson.BoolPtr[smokeConfig]
	_ func(string, func(*smokeConfig) *any) qijson.Field[smokeConfig]      = qijson.Any[smokeConfig]
	_ func(string, func(*smokeConfig) *[]byte) qijson.Field[smokeConfig]   = qijson.Raw[smokeConfig]

	_ func(string, func(*smokeConfig) *[]int64, qijson.Codec[int64]) qijson.Field[smokeConfig]          = qijson.Slice[smokeConfig, int64]
	_ func(string, func(*smokeConfig) *map[string]int64, qijson.Codec[int64]) qijson.Field[smokeConfig] = qijson.Map[smokeConfig, int64]
	_ func(string, func(*smokeConfig) []int64, qijson.Codec[int64]) qijson.Field[smokeConfig]           = qijson.Array[smokeConfig, int64]

	_ func(...qijson.Field[smokeConfig]) *qijson.Schema[smokeConfig]                                   = qijson.Struct[smokeConfig]
	_ func(func(*qijson.Reader) (int64, error), func(*qijson.Writer, int64) error) qijson.Codec[int64] = qijson.NewCodec[int64]
	_ func() *qijson.Writer                                                                            = qijson.NewWriter
	_ func([]byte, qijson.Decoder) error                                                               = qijson.Decode
	_ func(*qijson.Writer, qijson.Encoder) error                                                       = qijson.Encode
	_ func(*qijson.Writer, ...[]byte) []byte                                                           = (*qijson.Writer).BuildBytes
	_ func(*qijson.Writer, io.Writer) (int, error)                                                     = (*qijson.Writer).DumpTo
	_ func(*qijson.Writer) (io.ReadCloser, error)                                                      = (*qijson.Writer).ReadCloser
	_ func(smokeConfig) ([]byte, error)                                                                = qijson.Marshal[smokeConfig, *smokeConfig]
	_ func([]byte) (smokeConfig, error)                                                                = qijson.Unmarshal[smokeConfig, *smokeConfig]
	_ func([]byte, *smokeConfig) error                                                                 = qijson.UnmarshalInto[smokeConfig, *smokeConfig]
)

// smokeConfig is a minimal type bound through the facade only: its schema, its
// methods and the field closures all name qijson, never codec.
type smokeConfig struct {
	Name string
	Port int64
}

var smokeSchema = qijson.Struct(
	qijson.String[smokeConfig]("name", func(c *smokeConfig) *string { return &c.Name }),
	qijson.Int[smokeConfig]("port", func(c *smokeConfig) *int64 { return &c.Port }),
)

func (c *smokeConfig) DecodeJSON(r *qijson.Reader) error { return smokeSchema.Decode(r, c) }

func (c *smokeConfig) EncodeJSON(w *qijson.Writer) error { return smokeSchema.Encode(w, c) }

// TestFacadeRoundTrip proves the aliases and wrappers carry real behavior, not
// just type identity: marshal, unmarshal, merge and encode all work from outside
// the module's engine package.
func TestFacadeRoundTrip(t *testing.T) {
	want := smokeConfig{Name: "qi-json", Port: 8080}

	data, err := qijson.Marshal[smokeConfig](want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(data); got != `{"name":"qi-json","port":8080}` {
		t.Fatalf("Marshal = %s", got)
	}

	got, err := qijson.Unmarshal[smokeConfig](data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != want {
		t.Fatalf("Unmarshal = %+v, want %+v", got, want)
	}

	var merged smokeConfig
	if err := qijson.Decode([]byte(`{"port":9000}`), &merged); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if merged.Port != 9000 || merged.Name != "" {
		t.Fatalf("Decode = %+v", merged)
	}

	w := qijson.NewWriter()
	if err := qijson.Encode(w, &want); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got := string(w.Bytes()); got != string(data) {
		t.Fatalf("Encode = %s, want %s", got, data)
	}
}

// TestFacadeBuildBytes proves the writer-buffer reuse surface is reachable from
// outside the module: BuildBytes returns a caller-owned copy of the document and
// reuses the capacity of a supplied buffer.
func TestFacadeBuildBytes(t *testing.T) {
	want := smokeConfig{Name: "qi-json", Port: 8080}
	w := qijson.NewWriter()
	if err := qijson.Encode(w, &want); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	doc := w.BuildBytes()
	if wantDoc := `{"name":"qi-json","port":8080}`; string(doc) != wantDoc {
		t.Fatalf("BuildBytes = %s, want %s", doc, wantDoc)
	}

	const reuseCap = 4096
	buf := make([]byte, 0, reuseCap)
	got := w.BuildBytes(buf)
	if string(got) != string(doc) {
		t.Fatalf("BuildBytes(reuse) = %s, want %s", got, doc)
	}
	if cap(got) != reuseCap {
		t.Errorf("cap(got) = %d, want the reused %d", cap(got), reuseCap)
	}
}

// TestFacadeStreamOutput proves the encode-streaming surface is reachable from
// outside the module: DumpTo writes the document to an io.Writer and ReadCloser
// hands it out as an io.ReadCloser.
func TestFacadeStreamOutput(t *testing.T) {
	want := smokeConfig{Name: "qi-json", Port: 8080}
	const wantDoc = `{"name":"qi-json","port":8080}`

	w := qijson.NewWriter()
	if err := qijson.Encode(w, &want); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var out bytes.Buffer
	n, err := w.DumpTo(&out)
	if err != nil {
		t.Fatalf("DumpTo: %v", err)
	}
	if out.String() != wantDoc || n != len(wantDoc) {
		t.Fatalf("DumpTo = (%q, %d), want (%q, %d)", out.String(), n, wantDoc, len(wantDoc))
	}

	if err := qijson.Encode(w, &want); err != nil {
		t.Fatalf("Encode after DumpTo: %v", err)
	}
	rc, err := w.ReadCloser()
	if err != nil {
		t.Fatalf("ReadCloser: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if string(got) != wantDoc {
		t.Fatalf("ReadCloser = %s, want %s", got, wantDoc)
	}
}

// TestFacadeErrorIdentity proves a *qijson.Error crosses the alias boundary, so
// errors.As and errors.Is behave as they did before the engine moved.
func TestFacadeErrorIdentity(t *testing.T) {
	var cfg smokeConfig
	err := qijson.Decode([]byte(`{"port":"nope"}`), &cfg)
	if err == nil {
		t.Fatal("Decode: want a type error, got nil")
	}

	var e *qijson.Error
	if !errors.As(err, &e) {
		t.Fatalf("errors.As(*qijson.Error) = false for %T", err)
	}
	if e.Kind != qijson.KindType {
		t.Fatalf("Kind = %v, want %v", e.Kind, qijson.KindType)
	}
	if e.Path != "$.port" {
		t.Fatalf("Path = %q, want %q", e.Path, "$.port")
	}

	var te *qijson.TypeError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As(*qijson.TypeError) = false for %T", err)
	}
	if te.Want != "integer" || te.Got != "string" {
		t.Fatalf("TypeError = %+v", te)
	}
}

// TestFacadeFieldLiteral proves a hand-written Field literal compiles through the
// alias, which is what generated code does.
func TestFacadeFieldLiteral(t *testing.T) {
	type holder struct{ N int64 }
	schema := qijson.Struct(qijson.Field[holder]{
		Name: "n",
		Decode: func(r *qijson.Reader, h *holder) error {
			n, err := r.ReadInt()
			h.N = n
			return err
		},
		Encode: func(w *qijson.Writer, h *holder) error { return w.WriteInt(h.N) },
	})

	data, err := schema.Marshal(holder{N: -7})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(data); got != `{"n":-7}` {
		t.Fatalf("Marshal = %s", got)
	}
}

// TestFacadeScannerSurface proves the re-exported syntax engine works through
// the facade alone: Valid validates, and the tokenizer walks a document and
// reports the Kind constants the facade publishes.
func TestFacadeScannerSurface(t *testing.T) {
	if !qijson.Valid([]byte(`{"a":[1,true,null]}`)) {
		t.Fatal("Valid(valid) = false")
	}
	if qijson.Valid([]byte(`{"a":}`)) {
		t.Fatal("Valid(invalid) = true")
	}

	tok := qijson.NewTokenizer([]byte(`[1,"two"]`))
	if err := tok.BeginArray(); err != nil {
		t.Fatalf("BeginArray: %v", err)
	}
	for i, want := range []qijson.Kind{qijson.KindNumber, qijson.KindString, qijson.KindEnd} {
		k, err := tok.Peek()
		if err != nil {
			t.Fatalf("Peek #%d: %v", i, err)
		}
		if k != want {
			t.Fatalf("Peek #%d = %v, want %v", i, k, want)
		}
		if want == qijson.KindEnd {
			break
		}
		if err := tok.Skip(); err != nil {
			t.Fatalf("Skip #%d: %v", i, err)
		}
	}
	if err := tok.EndArray(); err != nil {
		t.Fatalf("EndArray: %v", err)
	}
}

// TestFacadeNewDecoder proves the reader-backed tokenizer is reachable through
// the facade: NewDecoder reads a document from an io.Reader and reports the
// same Kind constants as the slice-backed tokenizer.
func TestFacadeNewDecoder(t *testing.T) {
	tok := qijson.NewDecoder(bytes.NewReader([]byte(`[1,"two"]`)))
	if err := tok.BeginArray(); err != nil {
		t.Fatalf("BeginArray: %v", err)
	}
	for i, want := range []qijson.Kind{qijson.KindNumber, qijson.KindString, qijson.KindEnd} {
		k, err := tok.Peek()
		if err != nil {
			t.Fatalf("Peek #%d: %v", i, err)
		}
		if k != want {
			t.Fatalf("Peek #%d = %v, want %v", i, k, want)
		}
		if want == qijson.KindEnd {
			break
		}
		if err := tok.Skip(); err != nil {
			t.Fatalf("Skip #%d: %v", i, err)
		}
	}
	if err := tok.EndArray(); err != nil {
		t.Fatalf("EndArray: %v", err)
	}
}

// TestFacadeNewDecoderMultiValue proves the NDJSON loop through the facade
// alone: NewDecoder reads several top-level values from one io.Reader, each
// value's raw bytes are decoded with Decode, and Peek reports KindEnd at the end
// of the stream.
func TestFacadeNewDecoderMultiValue(t *testing.T) {
	const stream = "{\"name\":\"a\",\"port\":1}\n{\"name\":\"b\",\"port\":2}\n"
	dec := qijson.NewDecoder(bytes.NewReader([]byte(stream)))

	var got []smokeConfig
	for {
		k, err := dec.Peek()
		if err != nil {
			t.Fatalf("Peek: %v", err)
		}
		if k == qijson.KindEnd {
			break
		}
		if k != qijson.KindObject {
			t.Fatalf("Peek = %v, want %v", k, qijson.KindObject)
		}
		raw, err := dec.ReadRaw()
		if err != nil {
			t.Fatalf("ReadRaw: %v", err)
		}
		var cfg smokeConfig
		if err := qijson.Decode(raw, &cfg); err != nil {
			t.Fatalf("Decode(%q): %v", raw, err)
		}
		got = append(got, cfg)
	}

	want := []smokeConfig{{Name: "a", Port: 1}, {Name: "b", Port: 2}}
	if len(got) != len(want) {
		t.Fatalf("decoded %d values, want %d: %v", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("value %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestFacadeTokenStream proves token-level streaming is reachable through the
// facade alone: Token returns the qijson.Token/Number/Delim union over a nested
// document and reports io.EOF at the end of the stream.
func TestFacadeTokenStream(t *testing.T) {
	tok := qijson.NewTokenizer([]byte(`{"a":[1,true,null],"b":"two"}`))

	var got []string
	for {
		tokv, err := tok.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		switch v := tokv.(type) {
		case nil:
			got = append(got, "null")
		case bool:
			got = append(got, "bool:"+strconv.FormatBool(v))
		case qijson.Delim:
			got = append(got, "delim:"+string(rune(v)))
		case qijson.Number:
			got = append(got, "num:"+string(v))
		case string:
			got = append(got, "str:"+v)
		default:
			t.Fatalf("Token = %T, want a qijson token type", tokv)
		}
	}

	want := []string{
		"delim:{", "str:a",
		"delim:[", "num:1", "bool:true", "null", "delim:]",
		"str:b", "str:two",
		"delim:}",
	}
	if len(got) != len(want) {
		t.Fatalf("Token stream = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestFacadeTokenNewDecoder proves the token stream reads several top-level
// values (NDJSON) through the facade and reports io.EOF at the end.
func TestFacadeTokenNewDecoder(t *testing.T) {
	dec := qijson.NewDecoder(bytes.NewReader([]byte("1\ntrue\n")))

	var got []qijson.Token
	for {
		tokv, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		got = append(got, tokv)
	}

	if len(got) != 2 {
		t.Fatalf("Token stream = %v, want 2 tokens", got)
	}
	if n, ok := got[0].(qijson.Number); !ok || string(n) != "1" {
		t.Fatalf("token 0 = %#v, want Number(\"1\")", got[0])
	}
	if b, ok := got[1].(bool); !ok || !b {
		t.Fatalf("token 1 = %#v, want true", got[1])
	}
}

// TestFacadePeekKindIdentity proves a custom Field can branch on the token kind
// through the facade: Reader.Peek returns a qijson.Kind, and a JSON null is
// recognised by comparing against qijson.KindNull.
func TestFacadePeekKindIdentity(t *testing.T) {
	type holder struct{ N *int64 }

	schema := qijson.Struct(qijson.Field[holder]{
		Name: "n",
		Decode: func(r *qijson.Reader, h *holder) error {
			k, err := r.Peek()
			if err != nil {
				return err
			}
			if k == qijson.KindNull {
				return r.ReadNull()
			}
			n, err := r.ReadInt()
			if err != nil {
				return err
			}
			h.N = &n
			return nil
		},
		Encode: func(w *qijson.Writer, h *holder) error {
			if h.N == nil {
				return w.WriteNull()
			}
			return w.WriteInt(*h.N)
		},
	})

	var got holder
	if err := schema.UnmarshalInto([]byte(`{"n":7}`), &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if got.N == nil || *got.N != 7 {
		t.Fatalf("n = %v, want 7", got.N)
	}

	got = holder{}
	if err := schema.UnmarshalInto([]byte(`{"n":null}`), &got); err != nil {
		t.Fatalf("UnmarshalInto(null): %v", err)
	}
	if got.N != nil {
		t.Fatalf("n = %v, want nil", *got.N)
	}
}
