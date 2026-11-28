package bench

import (
	"encoding/json"
	"testing"

	easyjson "github.com/mailru/easyjson"
	qijson "github.com/qi-lore/qi-json"
)

// --- sample values -------------------------------------------------------

// sampleRecord returns a populated Record so every codec encodes the same data.
func sampleRecord() Record {
	return Record{
		ID:        123456789,
		Name:      "Alexandra the Great",
		Email:     "alexandra.great@example.com",
		Active:    true,
		Score:     98.7654,
		Tags:      []string{"alpha", "beta", "gamma", "delta"},
		CreatedAt: "2026-11-23T12:34:56Z",
	}
}

// sampleRecords returns n records that differ in ID, so the 100-record document
// is not one value repeated.
func sampleRecords(n int) Records {
	rs := make(Records, n)
	base := sampleRecord()
	for i := range rs {
		rs[i] = base
		rs[i].ID = int64(i)
	}
	return rs
}

// sampleNested returns a document with three levels of nesting below the root:
// Nested -> Company -> Address -> Geo.
func sampleNested() Nested {
	return Nested{
		ID: 42,
		Company: Company{
			Name: "Acme 株式会社",
			Address: Address{
				Street: "1-2-3 桜通り",
				City:   "Tōkyō",
				Geo:    Geo{Lat: 35.6762, Lon: 139.6503},
			},
		},
	}
}

// sampleMapDoc returns a struct with two map fields, exercising dynamic keys and
// the encoder's key-ordering path.
func sampleMapDoc() MapDoc {
	return MapDoc{
		ID:       7,
		Labels:   map[string]string{"env": "prod", "region": "eu-west-1", "tier": "gold"},
		Counters: map[string]int64{"requests": 1000, "errors": 3, "retries": 12},
	}
}

// sampleUnicodeDoc returns string fields holding non-ASCII runes, JSON escape
// sequences (\n, \t, \r, \"), and HTML-sensitive bytes that encoding/json writes
// as \u escapes. Marshalled with encoding/json it thus contains both raw UTF-8
// and \u/\n escape sequences.
func sampleUnicodeDoc() UnicodeDoc {
	return UnicodeDoc{
		ID:      99,
		Text:    "こんにちは世界 — Ω≈ç√∫˜µ",
		Escaped: "line1\nline2\ttab\r\"quoted\"\\backslash",
		Mixed:   "<b>a&b</b>\u2028separator",
	}
}

var (
	oneRecord  = sampleRecord()
	manyRecord = sampleRecords(100)
	nestedDoc  = sampleNested()
	mapDoc     = sampleMapDoc()
	unicodeDoc = sampleUnicodeDoc()

	// The JSON input for every decoder is built once, outside every timed loop,
	// so all three decoders read the identical bytes.
	jsonRecord  []byte
	jsonRecords []byte
	jsonNested  []byte
	jsonMapDoc  []byte
	jsonUnicode []byte
)

func init() {
	jsonRecord = mustMarshal(oneRecord)
	jsonRecords = mustMarshal(manyRecord)
	jsonNested = mustMarshal(nestedDoc)
	jsonMapDoc = mustMarshal(mapDoc)
	jsonUnicode = mustMarshal(unicodeDoc)
}

// mustMarshal marshals v with encoding/json and returns the bytes, panicking on
// error. The shared inputs are built with the standard library so the document
// is the canonical, compact form an encoder would emit.
func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// runCodecs registers the three sub-benchmarks for one operation and reports
// allocations for each. std, easy and qi run the same operation with
// encoding/json, mailru/easyjson and qi-json respectively; the closures are
// built once outside the timed loop, so only the codec's own cost is measured.
func runCodecs(b *testing.B, std, easy, qi func() error) {
	b.Run("stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := std(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("easyjson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := easy(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("qijson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := qi(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkMarshal1 encodes a single flat Record with each codec.
func BenchmarkMarshal1(b *testing.B) {
	v := oneRecord
	runCodecs(b,
		func() error { _, err := json.Marshal(v); return err },
		func() error { _, err := easyjson.Marshal(v); return err },
		func() error { _, err := qijson.Marshal[Record](v); return err },
	)
}

// BenchmarkUnmarshal1 decodes a single flat Record with each codec. The qi-json
// decode uses UnmarshalInto to match the merge semantics of the other two.
func BenchmarkUnmarshal1(b *testing.B) {
	data := jsonRecord
	runCodecs(b,
		func() error { var v Record; return json.Unmarshal(data, &v) },
		func() error { var v Record; return easyjson.Unmarshal(data, &v) },
		func() error { var v Record; return qijson.UnmarshalInto[Record](data, &v) },
	)
}

// BenchmarkMarshal100 encodes a 100-record document with each codec.
func BenchmarkMarshal100(b *testing.B) {
	v := manyRecord
	runCodecs(b,
		func() error { _, err := json.Marshal(v); return err },
		func() error { _, err := easyjson.Marshal(v); return err },
		func() error { _, err := qijson.Marshal[Records](v); return err },
	)
}

// BenchmarkUnmarshal100 decodes a 100-record document with each codec. The
// qi-json decode uses UnmarshalInto to match the merge semantics of the others.
func BenchmarkUnmarshal100(b *testing.B) {
	data := jsonRecords
	runCodecs(b,
		func() error { var v Records; return json.Unmarshal(data, &v) },
		func() error { var v Records; return easyjson.Unmarshal(data, &v) },
		func() error { var v Records; return qijson.UnmarshalInto[Records](data, &v) },
	)
}

// BenchmarkMarshalNested encodes a three-level nested document with each codec.
func BenchmarkMarshalNested(b *testing.B) {
	v := nestedDoc
	runCodecs(b,
		func() error { _, err := json.Marshal(v); return err },
		func() error { _, err := easyjson.Marshal(v); return err },
		func() error { _, err := qijson.Marshal[Nested](v); return err },
	)
}

// BenchmarkUnmarshalNested decodes a three-level nested document with each codec.
func BenchmarkUnmarshalNested(b *testing.B) {
	data := jsonNested
	runCodecs(b,
		func() error { var v Nested; return json.Unmarshal(data, &v) },
		func() error { var v Nested; return easyjson.Unmarshal(data, &v) },
		func() error { var v Nested; return qijson.UnmarshalInto[Nested](data, &v) },
	)
}

// BenchmarkMarshalMap encodes a struct with map fields with each codec.
func BenchmarkMarshalMap(b *testing.B) {
	v := mapDoc
	runCodecs(b,
		func() error { _, err := json.Marshal(v); return err },
		func() error { _, err := easyjson.Marshal(v); return err },
		func() error { _, err := qijson.Marshal[MapDoc](v); return err },
	)
}

// BenchmarkUnmarshalMap decodes a struct with map fields with each codec.
func BenchmarkUnmarshalMap(b *testing.B) {
	data := jsonMapDoc
	runCodecs(b,
		func() error { var v MapDoc; return json.Unmarshal(data, &v) },
		func() error { var v MapDoc; return easyjson.Unmarshal(data, &v) },
		func() error { var v MapDoc; return qijson.UnmarshalInto[MapDoc](data, &v) },
	)
}

// BenchmarkMarshalUnicode encodes a document with non-ASCII runes and escape
// sequences with each codec.
func BenchmarkMarshalUnicode(b *testing.B) {
	v := unicodeDoc
	runCodecs(b,
		func() error { _, err := json.Marshal(v); return err },
		func() error { _, err := easyjson.Marshal(v); return err },
		func() error { _, err := qijson.Marshal[UnicodeDoc](v); return err },
	)
}

// BenchmarkUnmarshalUnicode decodes a document with non-ASCII runes and escape
// sequences with each codec.
func BenchmarkUnmarshalUnicode(b *testing.B) {
	data := jsonUnicode
	runCodecs(b,
		func() error { var v UnicodeDoc; return json.Unmarshal(data, &v) },
		func() error { var v UnicodeDoc; return easyjson.Unmarshal(data, &v) },
		func() error { var v UnicodeDoc; return qijson.UnmarshalInto[UnicodeDoc](data, &v) },
	)
}
