// Package xtypes provides cross-package named scalar types for the code
// generator's fixture: the generator must resolve a selector type in another
// package to decide how to decode it.
package xtypes

import "github.com/qi-lore/qi-json"

// Label is a named string decoded through a cross-package field.
type Label string

// Flag is a named bool decoded through a cross-package field.
type Flag bool

// Aware is a qi-json-aware foreign type: it implements both DecodeJSON and
// EncodeJSON so the generator binds a field of this type by delegation.
type Aware struct {
	A int64
	B string
}

var awareSchema = qijson.Struct(
	qijson.Int[Aware]("a", func(v *Aware) *int64 { return &v.A }),
	qijson.String[Aware]("b", func(v *Aware) *string { return &v.B }),
)

// DecodeJSON implements qijson.Decoder.
func (v *Aware) DecodeJSON(r *qijson.Reader) error { return awareSchema.Decode(r, v) }

// EncodeJSON implements qijson.Encoder.
func (v *Aware) EncodeJSON(w *qijson.Writer) error { return awareSchema.Encode(w, v) }

// Half implements only DecodeJSON, so a field of this type is a precise error:
// the generator requires both directions to keep decode and encode symmetric.
type Half struct {
	A int64
}

// DecodeJSON implements qijson.Decoder but not qijson.Encoder.
func (v *Half) DecodeJSON(r *qijson.Reader) error { return r.Skip() }

// Inner is a plain foreign struct nested inside Thing, so the generator must
// recurse and emit its schema once.
type Inner struct {
	Code int64 `json:"code"`
}

// Thing is a plain foreign struct (not qi-json-aware): the generator synthesizes
// its schema from the exported fields. Count is a non-canonical width and hidden
// is unexported, so it is skipped.
type Thing struct {
	Name  string `json:"name"`
	Count int    `json:"count,omitempty"`
	Tiny  int8   `json:"tiny,omitempty"`
	Extra Inner  `json:"extra"`
	//lint:ignore U1000 unexported foreign fields must be skipped by the generator
	hidden string
}

// Embedded has an embedded field, which v1 rejects precisely.
type Embedded struct {
	Inner
	Name string `json:"name"`
}

// Twin reaches Inner by two paths, so the generator must emit its schema once.
type Twin struct {
	A Inner   `json:"a"`
	B []Inner `json:"b"`
}

// Linked has a pointer to a nested foreign struct.
type Linked struct {
	Inner *Inner `json:"inner"`
}
