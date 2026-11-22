package scanner

import (
	"strconv"
	"strings"
	"testing"
)

func BenchmarkValidObject(b *testing.B) {
	data := []byte(`{"id":12345,"name":"demo","active":true,"tags":["a","b","c"],"score":-1.5e3,"nil":null}`)
	benchmarkValid(b, data)
}

func BenchmarkValidArrayOfObjects(b *testing.B) {
	benchmarkValid(b, buildArrayDocument(1000))
}

func BenchmarkValidNumbers(b *testing.B) {
	benchmarkValid(b, buildNumberDocument(10000))
}

func BenchmarkValidString(b *testing.B) {
	benchmarkValid(b, buildStringDocument(1<<16))
}

func benchmarkValid(b *testing.B, data []byte) {
	b.Helper()

	if !Valid(data) {
		b.Fatal("benchmark seed is not valid JSON")
	}

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if !Valid(data) {
			b.Fatal("Valid() = false, want true")
		}
	}
}

func buildArrayDocument(n int) []byte {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":`)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`,"name":"item","ok":true}`)
	}
	b.WriteByte(']')
	return []byte(b.String())
}

func buildNumberDocument(n int) []byte {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(i))
		b.WriteString(".5e-3")
	}
	b.WriteByte(']')
	return []byte(b.String())
}

func buildStringDocument(n int) []byte {
	var b strings.Builder
	b.Grow(n + 2)
	b.WriteByte('"')
	for i := 0; i < n; i++ {
		switch i % 64 {
		case 0:
			b.WriteString(`\n`)
		case 32:
			b.WriteString(`\u00e9`)
		default:
			b.WriteByte('a')
		}
	}
	b.WriteByte('"')
	return []byte(b.String())
}
