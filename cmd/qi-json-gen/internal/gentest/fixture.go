// Package gentest is the code generator's end-to-end fixture: the types here
// are decoded through code emitted by cmd/qi-json-gen, checked in as
// zz_qi_json_gen.go. Regenerate with `make generate` (or `go generate ./...`);
// TestGeneratedFixtureUpToDate fails if the checked-in file drifts.
package gentest

import (
	"time"

	"github.com/qi-lore/qi-json"
	"github.com/qi-lore/qi-json/cmd/qi-json-gen/internal/gentest/xtypes"
)

//go:generate go run github.com/qi-lore/qi-json/cmd/qi-json-gen -type=Config,Point,Nums
//go:generate go run github.com/qi-lore/qi-json/cmd/qi-json-gen -type=Overridden -foreign=xtypes.Inner=overriddenInnerSchema -o zz_qi_json_gen_foreign.go
//go:generate go run github.com/qi-lore/qi-json/cmd/qi-json-gen -strict -type=Strict -o zz_qi_json_gen_strict.go

// Config exercises every field family the generator supports.
type Config struct {
	Name    string           `json:"name"`
	Port    int64            `json:"port"`
	Rate    float64          `json:"rate"`
	Debug   bool             `json:"debug"`
	Level   int8             `json:"level"`
	Count   uint16           `json:"count"`
	Ratio   float32          `json:"ratio"`
	Size    int              `json:"size"`
	Limit   *int32           `json:"limit"`
	Tags    []string         `json:"tags"`
	Matrix  [2]int           `json:"matrix"`
	Scores  map[string]int32 `json:"scores"`
	Blob    []byte           `json:"blob"`
	Meta    any              `json:"meta"`
	Origin  Point            `json:"origin"`
	Points  []Point          `json:"points"`
	Ports   map[string]Point `json:"ports"`
	Note    string           `json:"note,omitempty"`
	Skipped string           `json:"-"`
	//lint:ignore U1000 unexported fields must be ignored by the generator
	hidden string

	// Cross-package named scalars: the generator resolves these through go/types.
	Timeout   time.Duration           `json:"timeout"`
	Deadline  *time.Duration          `json:"deadline"`
	Durations []time.Duration         `json:"durations"`
	Label     xtypes.Label            `json:"label"`
	Labels    map[string]xtypes.Label `json:"labels"`
	Flag      xtypes.Flag             `json:"flag"`

	// Same-package named string/bool types: resolved from the AST.
	Status   Status          `json:"status"`
	Prev     *Status         `json:"prev"`
	Statuses []Status        `json:"statuses"`
	Mode     Mode            `json:"mode"`
	Modes    map[string]Mode `json:"modes"`

	// Foreign structs. Thing (and its nested Twin/Inner) is a plain struct, so
	// the generator synthesizes a schema (approach A); Aware implements
	// DecodeJSON/EncodeJSON, so the generator delegates (approach B).
	Thing    xtypes.Thing            `json:"thing"`
	Maybe    *xtypes.Thing           `json:"maybe,omitempty"`
	Things   []xtypes.Thing          `json:"things"`
	Twin     xtypes.Twin             `json:"twin"`
	Aware    xtypes.Aware            `json:"aware"`
	AwareMap map[string]xtypes.Aware `json:"aware_map"`
}

// Status is a same-package named string, decoded through the AST path.
type Status string

// Mode is a same-package named bool, decoded through the AST path.
type Mode bool

// Point is a nested struct used by Config, so it must be generated too.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Nums exercises the element helpers and unsigned types.
type Nums struct {
	Ints  []int8    `json:"ints"`
	Uints []uint32  `json:"uints"`
	Float []float32 `json:"float"`
}

// Strict is generated with -strict, so its schema rejects unknown and duplicate
// object members.
type Strict struct {
	A int64  `json:"a"`
	B string `json:"b"`
}

// Overridden exercises -foreign: its Inner field binds to a schema this package
// owns, emitted into zz_qi_json_gen_foreign.go rather than synthesized.
type Overridden struct {
	Inner xtypes.Inner `json:"inner"`
}

// overriddenInnerSchema is the caller-supplied schema -foreign points the
// generator at for xtypes.Inner.
var overriddenInnerSchema = qijson.Struct(
	qijson.Int[xtypes.Inner]("code", func(v *xtypes.Inner) *int64 { return &v.Code }),
)
