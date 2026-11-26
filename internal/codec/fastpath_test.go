package codec

import "testing"

// fastList decodes a JSON array of strings, preallocating the output so the
// slice never grows: that isolates the per-string allocation the string fast
// path is meant to remove.
type fastList []string

func (s *fastList) DecodeJSON(r *Reader) error {
	out := make([]string, 0, 512)
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

// TestDecodeCleanStringsAllocsBounded pins the decode string fast path: a clean
// string (no escape) must not allocate per value, so the number of allocations
// for an array of n plain strings stays a small constant instead of growing with
// n.
func TestDecodeCleanStringsAllocsBounded(t *testing.T) {
	data := buildStringsDocument(300, 32)

	allocs := testing.AllocsPerRun(10, func() {
		var v fastList
		if err := UnmarshalInto[fastList](data, &v); err != nil {
			t.Fatalf("UnmarshalInto() error = %v", err)
		}
		if len(v) != 300 {
			t.Fatalf("len(v) = %d, want 300", len(v))
		}
	})

	if allocs > 20 {
		t.Fatalf("decode of 300 clean strings allocated %.0f times, want <= 20 (the per-string copy must be gone)", allocs)
	}
}

// TestDecodedStringDoesNotAliasInput guards the ownership contract: a decoded
// string must not alias the caller's input buffer, so mutating (or reusing) the
// caller's bytes after the call cannot change a decoded value.
func TestDecodedStringDoesNotAliasInput(t *testing.T) {
	data := []byte(`{"name":"qi-json","port":1}`)
	var v uhConfig
	if err := uhConfigSchema.UnmarshalInto(data, &v); err != nil {
		t.Fatalf("UnmarshalInto() error = %v", err)
	}
	if v.Name != "qi-json" {
		t.Fatalf("Name = %q, want %q", v.Name, "qi-json")
	}

	// Overwrite the caller's buffer. A decoded string that aliased it would
	// change with it.
	for i := range data {
		data[i] = 'x'
	}
	if v.Name != "qi-json" {
		t.Fatalf("decoded string aliases the caller's buffer: Name = %q after mutating the input", v.Name)
	}
}
