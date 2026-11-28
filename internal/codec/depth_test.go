package codec

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// chain is a recursive JSON object type: each value may hold the next one, so a
// document of n nested objects decodes into a chain of n links. It is the shape
// that drives the decoder's nesting-depth counter, and it reaches the depth
// check through objectRaw (Schema.Decode) and beginObject/EndObject.
type chain struct {
	Next *chain
}

var chainSchema = Struct(
	ObjectDecoderPtr("next", func(c *chain) **chain { return &c.Next }),
)

func (c *chain) DecodeJSON(r *Reader) error { return chainSchema.Decode(r, c) }
func (c *chain) EncodeJSON(w *Writer) error { return chainSchema.Encode(w, c) }

// nestedChainDocument builds a document of n nested objects:
// {"next":{"next": ... null ...}}. Its outermost object opens at offset 0 and
// each successive object opens 8 bytes later (`{"next":` is 8 bytes), so the
// k-th object (k from 0) opens at byte offset k*8.
func nestedChainDocument(n int) []byte {
	var b strings.Builder
	b.Grow(n*8 + 4)
	for i := 0; i < n; i++ {
		b.WriteString(`{"next":`)
	}
	b.WriteString(`null`)
	b.WriteString(strings.Repeat(`}`, n))
	return []byte(b.String())
}

// TestDecodeDepthExceeded proves a document nested deeper than the fixed limit
// fails with an *Error of Kind KindDepth, carrying the sentinel cause and the
// offset of the object that crossed the limit.
func TestDecodeDepthExceeded(t *testing.T) {
	data := nestedChainDocument(maxDepth + 1)

	var got chain
	err := UnmarshalInto[chain](data, &got)
	if err == nil {
		t.Fatalf("UnmarshalInto() error = nil, want a depth error")
	}

	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != KindDepth {
		t.Errorf("Kind = %v, want %v", e.Kind, KindDepth)
	}
	if !errors.Is(err, errDepthExceeded) {
		t.Errorf("errors.Is(err, errDepthExceeded) = false; err = %v", err)
	}
	if want := int64(maxDepth * 8); e.Offset != want {
		t.Errorf("Offset = %d, want %d (the (maxDepth+1)-th object)", e.Offset, want)
	}
	wantPath := "$" + strings.Repeat(".next", maxDepth)
	if e.Path != wantPath {
		t.Errorf("Path = %q, want the path to the offending object", e.Path)
	}
	// The sentinel carries no package prefix: Error renders exactly one.
	want := "qijson: " + wantPath + ": nesting depth exceeds 10000 (at offset " +
		strconv.Itoa(maxDepth*8) + ")"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestDecodeDepthExceededArrays proves the array path enforces the same limit,
// so a deeply nested array is rejected too.
func TestDecodeDepthExceededArrays(t *testing.T) {
	data := []byte(strings.Repeat("[", maxDepth+1) + strings.Repeat("]", maxDepth+1))

	var got fuzzAny
	err := UnmarshalInto[fuzzAny](data, &got)
	if err == nil {
		t.Fatalf("UnmarshalInto() error = nil, want a depth error")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if e.Kind != KindDepth {
		t.Errorf("Kind = %v, want %v", e.Kind, KindDepth)
	}
	if want := int64(maxDepth); e.Offset != want {
		t.Errorf("Offset = %d, want %d (the (maxDepth+1)-th array)", e.Offset, want)
	}
}

// TestDecodeDepthAtLimit proves a document nested exactly to the limit decodes
// successfully, so the limit is a bound on the crossing, not on the limit itself.
func TestDecodeDepthAtLimit(t *testing.T) {
	data := nestedChainDocument(maxDepth)

	var got chain
	if err := UnmarshalInto[chain](data, &got); err != nil {
		t.Fatalf("UnmarshalInto() error = %v, want nil", err)
	}

	n := 0
	for c := &got; c != nil; c = c.Next {
		n++
	}
	if n != maxDepth {
		t.Errorf("chain length = %d, want %d", n, maxDepth)
	}
}

// TestDecodeWideShallowDepth proves the counter is decremented when a container
// closes: a wide but shallow document (an array of many empty objects) must not
// trip the limit even though it opens far more containers than the limit.
func TestDecodeWideShallowDepth(t *testing.T) {
	const objects = 20000

	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < objects; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{}`)
	}
	b.WriteByte(']')
	data := []byte(b.String())

	var got fuzzAny
	if err := UnmarshalInto[fuzzAny](data, &got); err != nil {
		t.Fatalf("UnmarshalInto() error = %v, want nil", err)
	}
	arr, ok := got.V.([]any)
	if !ok {
		t.Fatalf("decoded value type = %T, want []any", got.V)
	}
	if len(arr) != objects {
		t.Errorf("decoded %d elements, want %d", len(arr), objects)
	}
}
