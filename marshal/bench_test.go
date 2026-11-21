package marshal

import (
	"strings"
	"testing"
)

// strList decodes a JSON array of strings at the document root.
type strList []string

func (s *strList) DecodeJSON(r *Reader) error {
	out := make([]string, 0, 1024)
	err := r.Array(func(int) error {
		v, err := r.ReadString()
		if err != nil {
			return err
		}
		out = append(out, v)
		return nil
	})
	if err != nil {
		return err
	}
	*s = out
	return nil
}

// buildStringsDocument builds a JSON array of n plain strings, each about width
// bytes long.
func buildStringsDocument(n, width int) []byte {
	var b strings.Builder
	b.Grow(n * (width + 3))
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		for j := 0; j < width; j++ {
			b.WriteByte('a')
		}
		b.WriteByte('"')
	}
	b.WriteByte(']')
	return []byte(b.String())
}

func benchmarkStrings(b *testing.B, n, width int) {
	data := buildStringsDocument(n, width)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v strList
		if err := UnmarshalInto[strList](data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeStringsShort(b *testing.B)  { benchmarkStrings(b, 20000, 16) }
func BenchmarkDecodeStringsMedium(b *testing.B) { benchmarkStrings(b, 10000, 64) }
func BenchmarkDecodeStringsLong(b *testing.B)   { benchmarkStrings(b, 2000, 512) }

// oneString decodes a single JSON string at the document root.
type oneString string

func (s *oneString) DecodeJSON(r *Reader) error {
	v, err := r.ReadString()
	if err != nil {
		return err
	}
	*s = oneString(v)
	return nil
}

func BenchmarkDecodeBigString(b *testing.B) {
	var sb strings.Builder
	sb.Grow(1<<16 + 2)
	sb.WriteByte('"')
	for i := 0; i < 1<<16; i++ {
		sb.WriteByte('a')
	}
	sb.WriteByte('"')
	data := []byte(sb.String())
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v oneString
		if err := UnmarshalInto[oneString](data, &v); err != nil {
			b.Fatal(err)
		}
	}
}
