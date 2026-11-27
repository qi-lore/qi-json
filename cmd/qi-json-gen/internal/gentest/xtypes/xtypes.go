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

// Num is a named numeric scalar in another package.
type Num int32

// F32 is a named float32 scalar in another package.
type F32 float32

// U is a named unsigned scalar in another package.
type U uint

// LabelList is a named slice, so a field of this type has a non-scalar,
// non-struct underlying type the generator must reject.
type LabelList []int

// ComplexN is a named complex scalar, so its basic kind is not representable.
type ComplexN complex64

// Weird has a DecodeJSON with the wrong parameter shape, so it is not treated as
// a qi-json-aware type.
type Weird struct {
	V int `json:"v"`
}

// DecodeJSON has a non-pointer parameter, so it does not match the interface.
func (w Weird) DecodeJSON(x int) error { return nil }

// Weird2 has a DecodeJSON whose parameter is not a qijson type.
type Weird2 struct {
	V int `json:"v"`
}

// DecodeJSON takes a pointer to int, not *qijson.Reader.
func (w Weird2) DecodeJSON(r *int) error { return nil }

// badI is unexported, so a field of this type cannot be bound from another
// package.
type badI int

// Rich is a plain foreign struct exercising every supported field shape the
// synthesizer handles: predeclared scalars, named scalars, pointers, containers
// and nested structs.
type Rich struct {
	S        string
	B        bool
	I        int64
	F        float64
	I8       int8
	U16      uint16
	PStr     *string
	PBool    *bool
	PInt     *int64
	PFloat   *float64
	PI16     *int16
	PNum     *Num
	PStatus  *Label
	PFlag    *Flag
	PAware   *Aware
	PInner   *Inner
	Named    Label
	NamedB   Flag
	NamedN   Num
	Aware    Aware
	Any      interface{}
	Empty    Iface
	Byt      []byte
	Strs     []string
	Bools    []bool
	I64s     []int64
	F64s     []float64
	I8s      []int8
	U16s     []uint16
	AwareOn  []Aware
	Inners   []Inner
	InnerArr [2]Inner
	InnerMap map[string]Inner
}

// BadNamedElems has named-scalar element slices inside a foreign struct. The
// generator currently builds their helper names from the dotted qualified name,
// which is not a valid identifier, so this shape fails to format.
type BadNamedElems struct {
	Nums   []Num
	Labels []Label
	Flags  []Flag
}

// Dupe has two fields bound to the same JSON member, which the synthesizer must
// reject. The member name is spelled once by the field name and once by a tag so
// there is no literal duplicate json tag.
type Dupe struct {
	X int
	Y int `json:"X"`
}

// BadHasChan has a channel field, an unsupported foreign field type.
type BadHasChan struct {
	C chan int
}

// BadHasIfaceMethod has an interface with a method, which the foreign field
// codec does not support.
type BadHasIfaceMethod struct {
	I interface{ M() }
}

// BadHasHalf has a field of a one-sided qi-json-aware type.
type BadHasHalf struct {
	H Half
}

// BadHasNamedList has a field whose named type has a slice underlying type.
type BadHasNamedList struct {
	L LabelList
}

// BadHasBadI has an exported field whose type is unexported in this package.
type BadHasBadI struct {
	H badI
}

// BadHasComplex has a field whose named type has a complex underlying type.
type BadHasComplex struct {
	C ComplexN
}

// BadSlice has a slice of a slice, an unsupported element type.
type BadSlice struct {
	S [][]int
}

// BadArray has an array of a slice, an unsupported element type.
type BadArray struct {
	A [2][]int
}

// BadMapKey has a map with a non-string key.
type BadMapKey struct {
	M map[int]Inner
}

// BadMapElem has a map whose element type is unsupported.
type BadMapElem struct {
	M map[string][]int
}

// BadStar has a pointer to a slice, an unsupported pointer type.
type BadStar struct {
	P *[]int
}

// BadStarNamed has a pointer to a named slice type.
type BadStarNamed struct {
	P *LabelList
}

// BadStarComplex has a pointer to a named complex type.
type BadStarComplex struct {
	P *ComplexN
}

// BadStarUnexported has a pointer to an unexported named type.
type BadStarUnexported struct {
	P *badI
}

// BadElemUnsupported has a slice of a func, an unsupported element type.
type BadElemUnsupported struct {
	E []func()
}

// BadElemHalf has a slice of a one-sided qi-json-aware type.
type BadElemHalf struct {
	H []Half
}

// BadElemNamedList has a slice of a named slice type.
type BadElemNamedList struct {
	N []LabelList
}

// BadElemComplex has a slice of a named complex type.
type BadElemComplex struct {
	E []ComplexN
}

// BadElemUnexported has a slice of an unexported named type.
type BadElemUnexported struct {
	E []badI
}

// BadStarHalf has a pointer to a one-sided qi-json-aware type.
type BadStarHalf struct {
	P *Half
}

// HasSkip has a field skipped by a json:"-" tag and a tagged field.
type HasSkip struct {
	Keep string `json:"keep"`
	Skip int    `json:"-"`
}

// HasInner has a field of a plain foreign struct, so an override for it is
// exercised from inside a synthesized schema.
type HasInner struct {
	I Inner `json:"i"`
}

// PtrInner has a pointer field of a plain foreign struct, so an override for it
// is exercised through the pointer path.
type PtrInner struct {
	P *Inner `json:"p"`
}

// InnerList has a slice of a plain foreign struct, so an override for that
// element type is exercised from inside a synthesized schema.
type InnerList struct {
	L []Inner `json:"l"`
}

// Iface is a named empty interface, so a field of this type has an interface
// underlying type the foreign codec must accept.
type Iface interface{}

// Reader is named like the qijson facade type but lives here, so a method that
// mentions it must not be mistaken for a qi-json interface method.
type Reader struct {
	N int `json:"n"`
}

// Weird3 implements DecodeJSON with a parameter named like the facade Reader but
// from this package.
type Weird3 struct {
	V int `json:"v"`
}

// DecodeJSON mentions xtypes.Reader, not qijson.Reader.
func (w Weird3) DecodeJSON(r *Reader) error { return nil }

// Weird4 implements DecodeJSON with a parameter that is a named type, not the
// facade Reader.
type Weird4 struct {
	V int `json:"v"`
}

// DecodeJSON mentions xtypes.Num, not qijson.Reader.
func (w Weird4) DecodeJSON(r *Num) error { return nil }

// Weird5 implements a DecodeJSON with the wrong arity, so it is not treated as a
// qi-json-aware type.
type Weird5 struct {
	V int `json:"v"`
}

// DecodeJSON takes no parameter, so it does not match the interface.
func (w Weird5) DecodeJSON() {}

// BadBasic has a predeclared non-scalar basic field.
type BadBasic struct {
	C complex64
}

// BadElemBase has a slice of a predeclared non-scalar basic type.
type BadElemBase struct {
	E []complex64
}
