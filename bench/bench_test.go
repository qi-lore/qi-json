package bench

import (
	"encoding/json"
	"testing"

	easyjson "github.com/mailru/easyjson"
	qijson "github.com/qi-lore/qi-json"
)

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

var (
	oneRecord  = sampleRecord()
	manyRecord = sampleRecords(100)

	// jsonRecord and jsonRecords are built once, outside every timed loop, so
	// all three decoders read the identical bytes.
	jsonRecord  []byte
	jsonRecords []byte
)

func init() {
	var err error
	if jsonRecord, err = json.Marshal(oneRecord); err != nil {
		panic(err)
	}
	if jsonRecords, err = json.Marshal(manyRecord); err != nil {
		panic(err)
	}
}

// BenchmarkMarshal1 encodes a single Record with each codec.
func BenchmarkMarshal1(b *testing.B) {
	b.Run("stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := json.Marshal(oneRecord); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("easyjson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := easyjson.Marshal(oneRecord); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("qijson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := qijson.Marshal[Record](oneRecord); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkUnmarshal1 decodes a single Record with each codec. The qi-json
// decode uses UnmarshalInto to match the merge semantics of the other two.
func BenchmarkUnmarshal1(b *testing.B) {
	b.Run("stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var r Record
			if err := json.Unmarshal(jsonRecord, &r); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("easyjson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var r Record
			if err := easyjson.Unmarshal(jsonRecord, &r); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("qijson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var r Record
			if err := qijson.UnmarshalInto[Record](jsonRecord, &r); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkMarshal100 encodes a 100-record document with each codec.
func BenchmarkMarshal100(b *testing.B) {
	b.Run("stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := json.Marshal(manyRecord); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("easyjson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := easyjson.Marshal(manyRecord); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("qijson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := qijson.Marshal[Records](manyRecord); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkUnmarshal100 decodes a 100-record document with each codec. The
// qi-json decode uses UnmarshalInto to match the merge semantics of the others.
func BenchmarkUnmarshal100(b *testing.B) {
	b.Run("stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var r Records
			if err := json.Unmarshal(jsonRecords, &r); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("easyjson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var r Records
			if err := easyjson.Unmarshal(jsonRecords, &r); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("qijson", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var r Records
			if err := qijson.UnmarshalInto[Records](jsonRecords, &r); err != nil {
				b.Fatal(err)
			}
		}
	})
}
