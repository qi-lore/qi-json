package codec

import (
	"errors"
	"testing"

	"github.com/qi-lore/qi-json/internal/scanner"
)

// newHintReader returns a Reader at the document root whose tokenizer and
// library-owned source string cover the same bytes, matching production
// construction.
func newHintReader(doc string) *Reader {
	return newReader(scanner.NewTokenizer([]byte(doc)), doc)
}

func TestArrayLenHint(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
		want int
	}{
		{"empty", `[]`, 0},
		{"whitespaceOnly", `[   ]`, 0},
		{"one", `[1]`, 1},
		{"three", `[1,2,3]`, 3},
		{"strings", `["a","b"]`, 2},
		{"nestedArrays", `[[1,2],[3],[4,5,6]]`, 3},
		{"nestedObjects", `[{"a":1},{"a":2,"b":3}]`, 2},
		{"commaInsideString", `["a,b","c"]`, 2},
		{"bracketsInsideString", `["]","[",","]`, 3},
		{"escapedQuoteInsideString", `["a\"]","b"]`, 2},
		{"escapedBackslashInsideString", `["a\\","b"]`, 2},
		{"leadingAndInnerSpace", "  [ 1 , 2 ]  ", 2},
		{"wrongKind", `{"a":1}`, 0},
		{"notAContainer", `42`, 0},
		{"truncatedNoClose", `[1,2`, 0},
		{"emptyInput", ``, 0},
		{"unterminatedString", `["a`, 0},
		{"strayClose", `[1,2}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := newHintReader(tc.doc).arrayLenHint(); got != tc.want {
				t.Errorf("arrayLenHint(%q) = %d, want %d", tc.doc, got, tc.want)
			}
		})
	}
}

func TestObjectLenHint(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
		want int
	}{
		{"empty", `{}`, 0},
		{"whitespaceOnly", `{   }`, 0},
		{"one", `{"a":1}`, 1},
		{"three", `{"a":1,"b":2,"c":3}`, 3},
		{"nestedArray", `{"a":[1,2,3],"b":[]}`, 2},
		{"nestedObject", `{"a":{"x":1,"y":2},"b":{}}`, 2},
		{"commaInsideString", `{"a":"x,y","b":"z"}`, 2},
		{"bracesInsideString", `{"a":"}{","b":"{}"}`, 2},
		{"escapedQuoteInsideString", `{"a":"x\""}`, 1},
		{"leadingAndInnerSpace", "  { \"a\" : 1 }  ", 1},
		{"wrongKind", `[1,2]`, 0},
		{"notAContainer", `true`, 0},
		{"truncatedNoClose", `{"a":1`, 0},
		{"emptyInput", ``, 0},
		{"strayClose", `{"a":1]`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := newHintReader(tc.doc).objectLenHint(); got != tc.want {
				t.Errorf("objectLenHint(%q) = %d, want %d", tc.doc, got, tc.want)
			}
		})
	}
}

// TestSlicePreallocatesCapacity proves Slice.Decode sizes the destination slice
// from the element count so the backing array is allocated exactly once.
func TestSlicePreallocatesCapacity(t *testing.T) {
	type cfg struct {
		Tags []string
	}
	schema := Struct(
		Slice[cfg, string]("tags", func(c *cfg) *[]string { return &c.Tags }, dec((*Reader).ReadString)),
	)

	got, err := schema.Unmarshal([]byte(`{"tags":["a","b","c","d","e"]}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	if len(got.Tags) != 5 {
		t.Fatalf("Tags = %v, want 5 elements", got.Tags)
	}
	if cap(got.Tags) != len(got.Tags) {
		t.Errorf("cap(Tags) = %d, len(Tags) = %d; want cap == len (destination preallocated from the length hint)",
			cap(got.Tags), len(got.Tags))
	}
}

// TestAnyArrayPreallocatesCapacity proves decodeAny sizes an array destination
// from the element count.
func TestAnyArrayPreallocatesCapacity(t *testing.T) {
	type cfg struct {
		V any
	}
	schema := Struct(Any[cfg]("v", func(c *cfg) *any { return &c.V }))

	got, err := schema.Unmarshal([]byte(`{"v":[1,2,3,4,5]}`))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}
	arr, ok := got.V.([]any)
	if !ok {
		t.Fatalf("V = %T, want []any", got.V)
	}
	if len(arr) != 5 {
		t.Fatalf("V = %v, want 5 elements", arr)
	}
	if cap(arr) != len(arr) {
		t.Errorf("cap(V) = %d, len(V) = %d; want cap == len (destination preallocated from the length hint)",
			cap(arr), len(arr))
	}
}

// TestMapPreallocatesCapacity proves Map.Decode sizes the destination map from
// the member count. A map is opaque, so this compares the allocation count of a
// large object to a small one: a map sized up front allocates its buckets once,
// independent of the member count, while a map grown by insertion reallocates
// as it fills.
func TestMapPreallocatesCapacity(t *testing.T) {
	if testing.Short() {
		t.Skip("allocation test is skipped in short mode")
	}
	type cfg struct {
		Attrs map[string]int64
	}
	schema := Struct(
		Map[cfg, int64]("attrs", func(c *cfg) *map[string]int64 { return &c.Attrs }, dec((*Reader).ReadInt)),
	)

	small := buildHintAttrsDocument(16)
	large := buildHintAttrsDocument(512)

	smallAllocs := testing.AllocsPerRun(50, func() {
		var c cfg
		if err := schema.UnmarshalInto(small, &c); err != nil {
			t.Fatal(err)
		}
	})
	largeAllocs := testing.AllocsPerRun(50, func() {
		var c cfg
		if err := schema.UnmarshalInto(large, &c); err != nil {
			t.Fatal(err)
		}
	})

	if largeAllocs > smallAllocs+1 {
		t.Errorf("decoding a 512-member object allocated %v times per run vs %v for a 16-member object; want the same (map preallocated from the length hint)",
			largeAllocs, smallAllocs)
	}
}

// TestHintKeepsSyntaxError proves the length hint never changes which error a
// malformed container produces: the syntax error keeps the same path, kind and
// byte offset it had before the hint existed.
func TestHintKeepsSyntaxError(t *testing.T) {
	type cfg struct {
		Tags  []string
		Attrs map[string]int64
	}
	schema := Struct(
		Slice[cfg, string]("tags", func(c *cfg) *[]string { return &c.Tags }, dec((*Reader).ReadString)),
		Map[cfg, int64]("attrs", func(c *cfg) *map[string]int64 { return &c.Attrs }, dec((*Reader).ReadInt)),
	)

	for _, tc := range []struct {
		name   string
		doc    string
		path   string
		offset int64
	}{
		{"truncatedArray", `{"tags":["a","b"`, "$.tags", 16},
		{"arrayMissingValue", `{"tags":["a",]}`, "$.tags", 13},
		{"truncatedObject", `{"attrs":{"a":1,`, "$.attrs", 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := schema.Unmarshal([]byte(tc.doc))
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("Unmarshal(%q) error = %v, want *Error", tc.doc, err)
			}
			if e.Kind != KindSyntax {
				t.Errorf("Kind = %v, want %v", e.Kind, KindSyntax)
			}
			if e.Path != tc.path {
				t.Errorf("Path = %q, want %q", e.Path, tc.path)
			}
			if e.Offset != tc.offset {
				t.Errorf("Offset = %d, want %d", e.Offset, tc.offset)
			}
		})
	}
}
