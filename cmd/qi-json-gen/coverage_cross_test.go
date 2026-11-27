package main

import (
	"strings"
	"testing"
)

const xtypesImport = `"github.com/qi-lore/qi-json/cmd/qi-json-gen/internal/gentest/xtypes"`

// xtypesPkg builds a temp package named Config with the given body fields and
// the xtypes import.
func xtypesPkg(fields string) string {
	return "package example\n\nimport " + xtypesImport + "\n\ntype Config struct {\n" + fields + "\n}\n"
}

// TestGenerateCrossRich covers the successful cross-package paths: a rich plain
// foreign struct (every synthesized field shape), cross-package scalar
// elements, delegation elements, and a foreign type whose DecodeJSON does not
// match the interface.
func TestGenerateCrossRich(t *testing.T) {
	dir := writePkg(t, xtypesPkg(`
		X       xtypes.Rich
		Weird   xtypes.Weird
		Weird2  xtypes.Weird2
		Weird3  xtypes.Weird3
		Weird4  xtypes.Weird4
		Weird5  xtypes.Weird5
		Skip    xtypes.HasSkip
		Nums    []xtypes.Num
		Labels  []xtypes.Label `+"`json:\"labels\"`"+`
		Labels2 []xtypes.Label `+"`json:\"labels2\"`"+`
		Flags   []xtypes.Flag
		F32s    []xtypes.F32
		Us      []xtypes.U
		Awares  []xtypes.Aware
	`))
	got, err := generate(dir, []string{"Config"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(got)
	want := []string{
		"var xtypesRichSchema = qijson.Struct[xtypes.Rich](",
		`qijson.Object[Config, xtypes.Rich]("X"`,
		// named string/bool/numeric foreign fields.
		`qijson.Field[xtypes.Rich]{Name: "Named"`,
		`qijson.Field[xtypes.Rich]{Name: "NamedB"`,
		// the qi-json-aware struct field delegates.
		`qijson.ObjectDecoder[xtypes.Rich, xtypes.Aware]("Aware"`,
		// container elements.
		`qijson.NewCodec(qiDecodeXtypesNum, qiEncodeXtypesNum)`,
		`qijson.Array[xtypes.Rich, xtypes.Inner]("InnerArr"`,
		`qijson.Map[xtypes.Rich, xtypes.Inner]("InnerMap"`,
		"func qiDecodeXtypesLabel",
		"func qiEncodeXtypesLabel",
		"func qiDecodeXtypesF32",
		"func qiDecodeXtypesU",
		`qijson.DecoderCodec[xtypes.Aware, *xtypes.Aware]()`,
	}
	for _, w := range want {
		if !strings.Contains(src, w) {
			t.Errorf("generated source missing %q\n---\n%s", w, src)
		}
	}
	if n := strings.Count(src, "func qiDecodeXtypesLabel"); n != 1 {
		t.Errorf("cross element decode helper reused %d times, want 1", n)
	}
	if n := strings.Count(src, "func qiEncodeXtypesLabel"); n != 1 {
		t.Errorf("cross element encode helper reused %d times, want 1", n)
	}
}

// TestGenerateCrossErrors drives the cross-package and foreign error branches.
func TestGenerateCrossErrors(t *testing.T) {
	cases := []struct {
		name   string
		fields string
	}{
		{"unresolved selector", "A foo.T\n"},
		{"unresolved pointer", "A *foo.T\n"},
		{"unresolved slice elem", "A []foo.T\n"},
		{"named list", "A xtypes.LabelList\n"},
		{"named complex", "A xtypes.ComplexN\n"},
		{"slice of named list", "A []xtypes.LabelList\n"},
		{"slice of named complex", "A []xtypes.ComplexN\n"},
		{"slice of one-sided", "A []xtypes.Half\n"},
		{"foreign chan field", "X xtypes.BadHasChan\n"},
		{"foreign iface method", "X xtypes.BadHasIfaceMethod\n"},
		{"foreign half field", "X xtypes.BadHasHalf\n"},
		{"foreign named list field", "X xtypes.BadHasNamedList\n"},
		{"foreign unexported field", "X xtypes.BadHasBadI\n"},
		{"foreign complex field", "X xtypes.BadHasComplex\n"},
		{"foreign duplicate member", "X xtypes.Dupe\n"},
		{"foreign bad slice elem", "X xtypes.BadSlice\n"},
		{"foreign bad array elem", "X xtypes.BadArray\n"},
		{"foreign bad map key", "X xtypes.BadMapKey\n"},
		{"foreign bad map elem", "X xtypes.BadMapElem\n"},
		{"foreign bad star", "X xtypes.BadStar\n"},
		{"foreign bad star named", "X xtypes.BadStarNamed\n"},
		{"foreign bad star complex", "X xtypes.BadStarComplex\n"},
		{"foreign bad star unexported", "X xtypes.BadStarUnexported\n"},
		{"foreign bad star half", "X xtypes.BadStarHalf\n"},
		{"foreign bad elem unsupported", "X xtypes.BadElemUnsupported\n"},
		{"foreign bad elem half", "X xtypes.BadElemHalf\n"},
		{"foreign bad elem named list", "X xtypes.BadElemNamedList\n"},
		{"foreign bad elem complex", "X xtypes.BadElemComplex\n"},
		{"foreign bad elem unexported", "X xtypes.BadElemUnexported\n"},
		{"foreign named scalar elems", "X xtypes.BadNamedElems\n"},
		{"foreign bad basic", "X xtypes.BadBasic\n"},
		{"foreign bad elem basic", "X xtypes.BadElemBase\n"},
		{"error field", "A error\n"},
		{"error elem", "A []error\n"},
		{"func field", "A func()\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writePkg(t, xtypesPkg(tc.fields))
			if _, err := generate(dir, []string{"Config"}); err == nil {
				t.Error("generate = nil error, want an error")
			}
		})
	}
}

// TestGenerateCrossOverrides covers the -foreign override branches: an override
// applied to a container element, to a field inside a synthesized schema, and to
// a pointer field inside a synthesized schema.
func TestGenerateCrossOverrides(t *testing.T) {
	overrides := map[string]string{"xtypes.Inner": "innerSchema"}

	t.Run("element override", func(t *testing.T) {
		dir := writePkg(t, xtypesPkg("A []xtypes.Inner\n"))
		got, err := generateOptions(dir, []string{"Config"}, false, overrides)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if !strings.Contains(string(got), "qijson.NewCodec(qiDecodeXtypesInner, qiEncodeXtypesInner)") {
			t.Errorf("override element helper missing:\n%s", got)
		}
	})

	t.Run("nested field override", func(t *testing.T) {
		dir := writePkg(t, xtypesPkg("X xtypes.HasInner\n"))
		got, err := generateOptions(dir, []string{"Config"}, false, overrides)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if !strings.Contains(string(got), `qijson.Object[xtypes.HasInner, xtypes.Inner]("i", func(v *xtypes.HasInner) *xtypes.Inner { return &v.I }, innerSchema)`) {
			t.Errorf("nested field override missing:\n%s", got)
		}
	})

	t.Run("pointer field override", func(t *testing.T) {
		dir := writePkg(t, xtypesPkg("X xtypes.PtrInner\n"))
		got, err := generateOptions(dir, []string{"Config"}, false, overrides)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		src := string(got)
		if !strings.Contains(src, `v.P = nil`) || !strings.Contains(src, "innerSchema.Decode(r, e)") {
			t.Errorf("pointer field override missing:\n%s", src)
		}
	})

	t.Run("nested element override", func(t *testing.T) {
		dir := writePkg(t, xtypesPkg("X xtypes.InnerList\n"))
		got, err := generateOptions(dir, []string{"Config"}, false, overrides)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if !strings.Contains(string(got), "qijson.NewCodec(qiDecodeXtypesInner, qiEncodeXtypesInner)") {
			t.Errorf("nested element override missing:\n%s", got)
		}
	})
}
