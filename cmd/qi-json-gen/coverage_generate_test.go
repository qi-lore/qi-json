package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateReadDirErrors drives the directory-read failure and the missing
// load error propagation.
func TestGenerateReadDirErrors(t *testing.T) {
	if _, err := generate(filepath.Join(t.TempDir(), "does-not-exist"), []string{"A"}); err == nil {
		t.Fatal("generate = nil error for a missing directory")
	}

	empty := t.TempDir()
	if _, err := generate(empty, []string{"A"}); err == nil || !strings.Contains(err.Error(), "no Go files") {
		t.Fatalf("generate(empty dir) = %v, want a no-Go-files error", err)
	}
}

// TestGenerateParseError drives the parse-failure branch in load.
func TestGenerateParseError(t *testing.T) {
	dir := writePkg(t, "package example\n\ntype A struct {\n")
	if _, err := generate(dir, []string{"A"}); err == nil {
		t.Fatal("generate = nil error for unparsable input")
	}
}

// TestGenerateMixedPackages covers the branch that skips a file whose package
// name differs from the first seen.
func TestGenerateMixedPackages(t *testing.T) {
	dir := writePkg(t, "package example\n\ntype A struct{ N int }\n")
	if err := os.WriteFile(filepath.Join(dir, "other.go"), []byte("package other\n\ntype B struct{ N int }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := generate(dir, []string{"A"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(string(got), "BSchema") {
		t.Errorf("a file from a different package leaked in:\n%s", got)
	}
}

// TestGenerateNoMarker covers the empty-target error and the duplicate-argument
// dedup path.
func TestGenerateNoMarker(t *testing.T) {
	dir := writePkg(t, "package example\n\ntype A struct{ N int }\n")
	if _, err := generate(dir, nil); err == nil || !strings.Contains(err.Error(), "no -type given") {
		t.Fatalf("generate(dir, nil) = %v, want a no-type error", err)
	}

	got, err := generate(dir, []string{"A", "A"})
	if err != nil {
		t.Fatalf("generate with a duplicate type arg: %v", err)
	}
	if n := strings.Count(string(got), "var aSchema"); n != 1 {
		t.Errorf("duplicate type arg emitted %d schemas, want 1", n)
	}
}

// TestGenerateBareFunc covers collect skipping a receiver-less function and a
// method receiver it cannot name.
func TestGenerateBareFunc(t *testing.T) {
	dir := writePkg(t, `package example

func helper() int { return 0 }

type A struct{ N int }

func (a A) String() string { return "" }
`)
	if _, err := generate(dir, []string{"A"}); err != nil {
		t.Fatalf("generate: %v", err)
	}
}

// TestGenerateGenericReceiver covers the receiver-naming guard: a method on a
// two-parameter generic type has an index-list receiver whose base name the
// helper does not derive, so it is skipped.
func TestGenerateGenericReceiver(t *testing.T) {
	dir := writePkg(t, `package example

type Box[T, U any] struct {
	V T
	W U
}

func (b Box[T, U]) Foo() {}

type A struct{ N int }
`)
	if _, err := generate(dir, []string{"A"}); err != nil {
		t.Fatalf("generate: %v", err)
	}
}

// TestGenerateAliasedImport covers the import-alias branch: a package whose name
// differs from its directory is imported under an explicit alias.
func TestGenerateAliasedImport(t *testing.T) {
	dir := writePkg(t, `package example

import "github.com/qi-lore/qi-json/cmd/qi-json-gen/internal/covalias"

type Config struct {
	X covname.Aliased
}
`)
	got, err := generate(dir, []string{"Config"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(string(got), `covname "github.com/qi-lore/qi-json/cmd/qi-json-gen/internal/covalias"`) {
		t.Errorf("aliased import missing:\n%s", got)
	}
}

// TestGenerateOmitHookWrongSignature covers the omit-hook detection fallback: a
// type whose IsZero has the wrong signature is not treated as an omit hook.
func TestGenerateOmitHookWrongSignature(t *testing.T) {
	dir := writePkg(t, `package example

type Z int
func (z Z) IsZero(x int) bool { return false }

type Config struct {
	V Z `+"`json:\"v,omitempty\"`"+`
}
`)
	got, err := generate(dir, []string{"Config"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(string(got), "qiEmpty(") {
		t.Errorf("a wrong-signature IsZero must not be used as an omit hook:\n%s", got)
	}
}

// TestGenerateFieldShapes drives the reachable field-codec branches: the
// predeclared scalar constructors, byte/rune resolution, named string/bool
// scalars (value and pointer), a pointer to a numeric, the predeclared pointer
// constructors, container element helpers for the built-in scalars, and helper
// reuse when the same named element type appears twice.
func TestGenerateFieldShapes(t *testing.T) {
	dir := writePkg(t, `package example

type Status string
type Nick bool
type Port int

type Point struct{ X int }

type Config struct {
	Byt      byte
	Rune     rune
	PStr     *string
	PBool    *bool
	PInt64   *int64
	PFloat64 *float64
	PNum8    *int8
	Status   Status
	Nick     Nick
	PStatus  *Status
	PNick    *Nick
	Port     Port
	Bools    []bool
	Ints64   []int64
	Floats   []float64
	Ports    []Port `+"`json:\"ports\"`"+`
	Ports2   []Port `+"`json:\"ports2\"`"+`
	Statuses []Status `+"`json:\"statuses\"`"+`
	Statuses2 []Status `+"`json:\"statuses2\"`"+`
	Points   []Point
	PointMap map[string]Point
	Any      any
	Empty    interface{}
	PF32     *float32
}
`)
	got, err := generate(dir, []string{"Config", "Point"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(got)
	want := []string{
		`qijson.StringPtr[Config]("PStr"`,
		`qijson.BoolPtr[Config]("PBool"`,
		`qijson.IntPtr[Config]("PInt64"`,
		`qijson.FloatPtr[Config]("PFloat64"`,
		`qijson.Field[Config]{Name: "PNum8"`,
		`qijson.Field[Config]{Name: "Status"`,
		`qijson.Field[Config]{Name: "Nick"`,
		`v.PStatus = &e`,
		`v.PNick = &e`,
		`qijson.NewCodec((*qijson.Reader).ReadBool, (*qijson.Writer).WriteBool)`,
		`qijson.NewCodec((*qijson.Reader).ReadInt, (*qijson.Writer).WriteInt)`,
		`qijson.NewCodec((*qijson.Reader).ReadFloat, (*qijson.Writer).WriteFloat)`,
		`qijson.Any[Config]("Any"`,
		`qijson.Any[Config]("Empty"`,
		`v.PF32 = &e`,
	}
	for _, w := range want {
		if !strings.Contains(src, w) {
			t.Errorf("generated source missing %q\n---\n%s", w, src)
		}
	}
	if n := strings.Count(src, "func qiDecodePort"); n != 1 {
		t.Errorf("numeric helper reused across two fields %d times, want 1", n)
	}
	if n := strings.Count(src, "func qiDecodeStatus"); n != 1 {
		t.Errorf("scalar helper reused across two fields %d times, want 1", n)
	}
	if n := strings.Count(src, "func qiEncodeStatus"); n != 1 {
		t.Errorf("scalar encode helper reused across two fields %d times, want 1", n)
	}
}

// TestGenerateCycle covers the recursive-resolution guards on a cyclic named
// type and the byte/rune basic resolution.
func TestGenerateCycle(t *testing.T) {
	dir := writePkg(t, `package example

type A B
type B A

type Config struct {
	Cycle A
	Byt   byte
	Rune  rune
}
`)
	_, err := generate(dir, []string{"Config"})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("generate = %v, want an unsupported-type error for the cyclic field", err)
	}
}

// TestGenerateFieldErrors drives the per-field error branches: an embedded
// field, a multi-name field with a json tag, an interface with methods, a
// pointer to an unsupported type, a nested slice, a map with a bad element and a
// map with a non-string key.
func TestGenerateFieldErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"embedded", "package example\n\ntype In struct{ N int }\n\ntype A struct{ In }\n"},
		{"multi-name tag", "package example\n\ntype A struct{ X, Y int `json:\"n\"` }\n"},
		{"interface methods", "package example\n\ntype A struct{ I interface{ M() } }\n"},
		{"pointer to slice", "package example\n\ntype A struct{ P *[]int }\n"},
		{"nested slice", "package example\n\ntype A struct{ N [][]int }\n"},
		{"nested array", "package example\n\ntype A struct{ N [2][2]int }\n"},
		{"map element", "package example\n\ntype A struct{ M map[string][]int }\n"},
		{"map key", "package example\n\ntype A struct{ M map[int]string }\n"},
		{"unknown elem", "package example\n\ntype A struct{ S []Undefined }\n"},
		{"nested struct not generated", "package example\n\ntype P struct{ N int }\n\ntype A struct{ P []P }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writePkg(t, tc.src)
			if _, err := generate(dir, []string{"A"}); err == nil {
				t.Error("generate = nil error, want an error")
			}
		})
	}
}
