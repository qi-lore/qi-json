package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"
)

// TestParseTag exercises parseTag's scanning branches directly: leading spaces,
// a name with no value, a quoted value containing an escape, an unterminated
// value, a non-matching key, and a well-formed match.
func TestParseTag(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		key  string
		want string
	}{
		{"plain", `json:"name"`, "json", "name"},
		{"leading spaces", `   json:"name"`, "json", "name"},
		{"with options", `json:"name,omitempty"`, "json", "name,omitempty"},
		{"other key first", `xml:"x" json:"name"`, "json", "name"},
		{"escape in value", `json:"a\"b"`, "json", `a\"b`},
		{"unterminated value", `json:"abc`, "json", ""},
		{"no colon quote", `jsonname`, "json", ""},
		{"key not present", `xml:"x"`, "json", ""},
		{"empty tag", ``, "json", ""},
		{"dangling colon", `json:`, "json", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseTag(tt.tag, tt.key); got != tt.want {
				t.Errorf("parseTag(%q, %q) = %q, want %q", tt.tag, tt.key, got, tt.want)
			}
		})
	}
}

// TestHasTagOption exercises both the comma walk and the not-found return.
func TestHasTagOption(t *testing.T) {
	tests := []struct {
		opts string
		want string
		ok   bool
	}{
		{"omitempty", "omitempty", true},
		{"a,omitempty", "omitempty", true},
		{"omitempty,b", "omitempty", true},
		{"a,b", "omitempty", false},
		{"", "omitempty", false},
	}
	for _, tt := range tests {
		if got := hasTagOption(tt.opts, tt.want); got != tt.ok {
			t.Errorf("hasTagOption(%q, %q) = %v, want %v", tt.opts, tt.want, got, tt.ok)
		}
	}
}

// TestJSONTag exercises the string-literal wrapper: a nil tag, a bad literal
// that fails to unquote, the skip forms and the omitempty forms.
func TestJSONTag(t *testing.T) {
	lit := func(v string) *ast.BasicLit { return &ast.BasicLit{Kind: token.STRING, Value: v} }

	if n, skip, om, err := jsonTag(nil); n != "" || skip || om || err != nil {
		t.Errorf("jsonTag(nil) = %q %v %v %v, want zero values", n, skip, om, err)
	}

	if _, _, _, err := jsonTag(lit(`"json:\q"`)); err == nil {
		t.Error("jsonTag with a bad string literal = nil error, want an error")
	}

	tests := []struct {
		name    string
		value   string
		wantN   string
		wantSk  bool
		wantOmi bool
	}{
		{"skip", "`json:\"-\"`", "", true, false},
		{"dash omitempty", "`json:\"-,\"`", "-", false, false},
		{"dash omitempty2", "`json:\"-,omitempty\"`", "-", false, true},
		{"named omitempty", "`json:\"x,omitempty\"`", "x", false, true},
		{"named only", "`json:\"x\"`", "x", false, false},
		{"bare", "`json:\"\"`", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, skip, omi, err := jsonTag(lit(tt.value))
			if err != nil {
				t.Fatalf("jsonTag(%s): %v", tt.value, err)
			}
			if n != tt.wantN || skip != tt.wantSk || omi != tt.wantOmi {
				t.Errorf("jsonTag(%s) = %q %v %v, want %q %v %v", tt.value, n, skip, omi, tt.wantN, tt.wantSk, tt.wantOmi)
			}
		})
	}
}

// TestLowerUpperFirst covers the empty-string short circuit and the normal path.
func TestLowerUpperFirst(t *testing.T) {
	if got := lowerFirst(""); got != "" {
		t.Errorf("lowerFirst(\"\") = %q, want empty", got)
	}
	if got := upperFirst(""); got != "" {
		t.Errorf("upperFirst(\"\") = %q, want empty", got)
	}
	if got := lowerFirst("Config"); got != "config" {
		t.Errorf("lowerFirst(Config) = %q, want config", got)
	}
	if got := upperFirst("port"); got != "Port" {
		t.Errorf("upperFirst(port) = %q, want Port", got)
	}
}

// TestRecvTypeName covers the receiver shapes: a value receiver, a pointer
// receiver, an index (generic) receiver, and an unsupported expression.
func TestRecvTypeName(t *testing.T) {
	if got := recvTypeName(&ast.Ident{Name: "T"}); got != "T" {
		t.Errorf("recvTypeName(Ident) = %q, want T", got)
	}
	if got := recvTypeName(&ast.StarExpr{X: &ast.Ident{Name: "T"}}); got != "T" {
		t.Errorf("recvTypeName(StarExpr) = %q, want T", got)
	}
	if got := recvTypeName(&ast.IndexExpr{X: &ast.Ident{Name: "T"}}); got != "T" {
		t.Errorf("recvTypeName(IndexExpr) = %q, want T", got)
	}
	if got := recvTypeName(&ast.ArrayType{}); got != "" {
		t.Errorf("recvTypeName(ArrayType) = %q, want empty", got)
	}
}

// TestSchemaNaming covers the small naming helpers.
func TestSchemaNaming(t *testing.T) {
	if got := schemaVarName("Config"); got != "configSchema" {
		t.Errorf("schemaVarName(Config) = %q, want configSchema", got)
	}
	if got := foreignSchemaVar("xtypes.Thing"); got != "xtypesThingSchema" {
		t.Errorf("foreignSchemaVar(xtypes.Thing) = %q, want xtypesThingSchema", got)
	}
	if got := crossHelperSuffix("time.Duration"); got != "TimeDuration" {
		t.Errorf("crossHelperSuffix(time.Duration) = %q, want TimeDuration", got)
	}
	if !isUnsigned("uint16") || isUnsigned("int16") {
		t.Error("isUnsigned misclassified an integer width")
	}
}

// TestBasicKindName covers every numeric kind mapping and the default.
func TestBasicKindName(t *testing.T) {
	cases := map[types.BasicKind]string{
		types.Int:     "int",
		types.Int8:    "int8",
		types.Int16:   "int16",
		types.Int32:   "int32",
		types.Int64:   "int64",
		types.Uint:    "uint",
		types.Uint8:   "uint8",
		types.Uint16:  "uint16",
		types.Uint32:  "uint32",
		types.Uint64:  "uint64",
		types.Float32: "float32",
		types.Float64: "float64",
	}
	for kind, want := range cases {
		got, ok := basicKindName(kind)
		if !ok || got != want {
			t.Errorf("basicKindName(%v) = %q, %v, want %q, true", kind, got, ok, want)
		}
	}
	if got, ok := basicKindName(types.String); ok || got != "" {
		t.Errorf("basicKindName(string) = %q, %v, want \"\", false", got, ok)
	}
}

// TestTypecheckNoFiles covers the empty-file short circuit, and namedOf with no
// type info.
func TestTypecheckNoFiles(t *testing.T) {
	g := &gen{}
	g.typecheck(nil)
	if g.info != nil {
		t.Error("typecheck(nil) set info, want nil")
	}
	if _, ok := g.namedOf(&ast.Ident{Name: "T"}); ok {
		t.Error("namedOf with nil info = ok, want false")
	}
}

// TestOmitHookNoInfo covers the omit-hook guards: no type info, and an
// expression the checker did not resolve.
func TestOmitHookNoInfo(t *testing.T) {
	g := &gen{}
	if _, ok := g.omitHook("Config", "V", &ast.Ident{Name: "V"}); ok {
		t.Error("omitHook with nil info = ok, want false")
	}

	g.info = &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	if _, ok := g.omitHook("Config", "V", &ast.Ident{Name: "V"}); ok {
		t.Error("omitHook with an unresolved expression = ok, want false")
	}
}
