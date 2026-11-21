package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// marker selects a type for generation in scan mode.
const marker = "qi-json:generate"

// defaultOutput is the file written when -o is not given.
const defaultOutput = "zz_qi_json_gen.go"

// rangeHelper is emitted once, when a generated field range-checks a number,
// and reports the failure as a marshal type error: the same shape the runtime
// builds for a number that does not fit its target.
const rangeHelper = `func qiRangeError(r *marshal.Reader, want string) error {
	return &marshal.Error{
		Path:   r.Path(),
		Offset: r.Offset(),
		Kind:   marshal.KindType,
		Err:    &marshal.TypeError{Want: want, Got: "number out of range"},
	}
}
`

// gen is the state of one generation run over a directory.
type gen struct {
	fset *token.FileSet

	// pkg is the package name of the target directory.
	pkg string

	// decls maps each type name in the package to its declaration.
	decls map[string]*ast.TypeSpec

	// structs maps each struct type name to its struct type.
	structs map[string]*ast.StructType

	// marked is the set of type names carrying the marker comment.
	marked map[string]bool

	// hasDecodeJSON is the set of type names that already declare DecodeJSON.
	hasDecodeJSON map[string]bool

	// hasEncodeJSON is the set of type names that already declare EncodeJSON.
	hasEncodeJSON map[string]bool

	// targets is the ordered list of struct names to emit, and known is the set
	// of names that will have a schema var (targets, for nested lookups).
	targets []string
	known   map[string]bool

	// Import and helper bookkeeping.
	needScanner bool
	needMath    bool
	needRange   bool
	helpers     map[string]string
	helperOrder []string

	// info is the package's type information, used to resolve cross-package
	// named types. It is nil when type checking failed, in which case such a
	// field reports a precise unsupported-type error.
	info *types.Info

	// extraImports maps an import path to the name it is imported under, for
	// packages referenced by cross-package field types.
	extraImports map[string]string

	// strict makes every generated schema reject unknown and duplicate members.
	strict bool
}

// generate parses dir and returns non-strict generated source for types.
func generate(dir string, types []string) ([]byte, error) {
	return generateWith(dir, types, false)
}

// generateWith parses dir and returns the formatted generated source for types.
// When types is empty, every marked type is generated. When strict is set, every
// schema rejects unknown and duplicate members.
func generateWith(dir string, types []string, strict bool) ([]byte, error) {
	g := &gen{
		fset:          token.NewFileSet(),
		decls:         map[string]*ast.TypeSpec{},
		structs:       map[string]*ast.StructType{},
		marked:        map[string]bool{},
		hasDecodeJSON: map[string]bool{},
		hasEncodeJSON: map[string]bool{},
		known:         map[string]bool{},
		helpers:       map[string]string{},
		extraImports:  map[string]string{},
		strict:        strict,
	}
	if err := g.load(dir); err != nil {
		return nil, err
	}
	if err := g.selectTargets(types); err != nil {
		return nil, err
	}
	return g.render()
}

// load parses the non-test Go files in dir and records their type declarations.
// It skips generated files, so re-running over its own output is a no-op.
func (g *gen) load(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var typeFiles []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(g.fset, filepath.Join(dir, name), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		if g.pkg == "" {
			g.pkg = f.Name.Name
		} else if f.Name.Name != g.pkg {
			continue
		}
		// Generated files are type-checked (so the package resolves fully) but
		// not collected, so re-running over the tool's own output is a no-op.
		typeFiles = append(typeFiles, f)
		if ast.IsGenerated(f) {
			continue
		}
		g.collect(f)
	}
	if g.pkg == "" {
		return fmt.Errorf("no Go files in %s", dir)
	}
	g.typecheck(typeFiles)
	return nil
}

// collect records the type and func declarations of one file.
func (g *gen) collect(f *ast.File) {
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				name := ts.Name.Name
				if _, dup := g.decls[name]; !dup {
					g.decls[name] = ts
				}
				if st, ok := ts.Type.(*ast.StructType); ok {
					g.structs[name] = st
				}
				if hasMarker(d.Doc) || hasMarker(ts.Doc) {
					g.marked[name] = true
				}
			}
		case *ast.FuncDecl:
			if d.Recv == nil || len(d.Recv.List) != 1 {
				continue
			}
			recv := recvTypeName(d.Recv.List[0].Type)
			if recv == "" {
				continue
			}
			switch d.Name.Name {
			case "DecodeJSON":
				g.hasDecodeJSON[recv] = true
			case "EncodeJSON":
				g.hasEncodeJSON[recv] = true
			}
		}
	}
}

// selectTargets resolves the type names to generate for and records them.
func (g *gen) selectTargets(types []string) error {
	if len(types) == 0 {
		for name := range g.marked {
			types = append(types, name)
		}
		sort.Strings(types)
		if len(types) == 0 {
			return fmt.Errorf("no -type given and no //%s marker found", marker)
		}
	}
	seen := map[string]bool{}
	for _, name := range types {
		if seen[name] {
			continue
		}
		seen[name] = true
		if _, ok := g.decls[name]; !ok {
			return fmt.Errorf("type %s not found", name)
		}
		if _, ok := g.structs[name]; !ok {
			return fmt.Errorf("type %s is not a struct", name)
		}
		if g.hasDecodeJSON[name] {
			return fmt.Errorf("type %s already declares DecodeJSON", name)
		}
		if g.hasEncodeJSON[name] {
			return fmt.Errorf("type %s already declares EncodeJSON", name)
		}
		g.targets = append(g.targets, name)
		g.known[name] = true
	}
	return nil
}

// render builds the generated file: header, package clause, imports, one schema
// var and DecodeJSON method per target, then the helper functions.
func (g *gen) render() ([]byte, error) {
	var body bytes.Buffer
	for _, name := range g.targets {
		if err := g.renderType(&body, name); err != nil {
			return nil, err
		}
	}
	for _, hn := range g.helperOrder {
		body.WriteString("\n")
		body.WriteString(g.helpers[hn])
	}
	if g.needRange {
		body.WriteString("\n")
		body.WriteString(rangeHelper)
	}

	var buf bytes.Buffer
	buf.WriteString("// Code generated by qi-json-gen; DO NOT EDIT.\n\n")
	fmt.Fprintf(&buf, "package %s\n\n", g.pkg)
	g.writeImports(&buf)
	buf.Write(body.Bytes())

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting generated source: %w\n--- raw ---\n%s", err, buf.Bytes())
	}
	return src, nil
}

// writeImports writes the import block, sorted by path.
func (g *gen) writeImports(buf *bytes.Buffer) {
	lines := map[string]string{ // import path -> import line
		"qi-json/marshal": `"qi-json/marshal"`,
	}
	if g.needScanner {
		lines["qi-json/scanner"] = `"qi-json/scanner"`
	}
	if g.needMath {
		lines["math"] = `"math"`
	}
	for path, name := range g.extraImports {
		if name == filepath.Base(path) {
			lines[path] = fmt.Sprintf("%q", path)
		} else {
			lines[path] = fmt.Sprintf("%s %q", name, path)
		}
	}
	paths := make([]string, 0, len(lines))
	for p := range lines {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	buf.WriteString("import (\n")
	for _, p := range paths {
		fmt.Fprintf(buf, "\t%s\n", lines[p])
	}
	buf.WriteString(")\n\n")
}

// renderType writes the schema var and DecodeJSON method for one struct.
func (g *gen) renderType(buf *bytes.Buffer, name string) error {
	st := g.structs[name]
	schemaVar := schemaVarName(name)

	fmt.Fprintf(buf, "var %s = marshal.Struct[%s](\n", schemaVar, name)
	used := map[string]bool{}
	for _, field := range st.Fields.List {
		codecs, err := g.fieldCodecs(name, field)
		if err != nil {
			return err
		}
		for _, c := range codecs {
			if used[c.json] {
				return fmt.Errorf("%s: duplicate member name %q", name, c.json)
			}
			used[c.json] = true
			fmt.Fprintf(buf, "\t%s,\n", c.code)
		}
	}
	if g.strict {
		buf.WriteString(").RejectUnknown().RejectDuplicate()\n\n")
	} else {
		buf.WriteString(")\n\n")
	}
	fmt.Fprintf(buf, "func (v *%s) DecodeJSON(r *marshal.Reader) error {\n\treturn %s.Decode(r, v)\n}\n", name, schemaVar)
	fmt.Fprintf(buf, "\nfunc (v *%s) EncodeJSON(w *marshal.Writer) error {\n\treturn %s.Encode(w, v)\n}\n", name, schemaVar)
	return nil
}

// codec is one generated field: its JSON member name and the Go expression that
// builds the marshal.Field.
type codec struct {
	json string
	code string
}

// fieldCodecs returns the codecs for one struct field, one per declared name.
func (g *gen) fieldCodecs(owner string, field *ast.Field) ([]codec, error) {
	if len(field.Names) == 0 {
		return nil, fmt.Errorf("%s: embedded field %s is not supported", owner, typeString(g.fset, field.Type))
	}

	jname, skip, omitEmpty, err := jsonTag(field.Tag)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", owner, err)
	}
	if len(field.Names) > 1 && jname != "" {
		return nil, fmt.Errorf("%s: a json tag on a multi-name field is not supported", owner)
	}

	var out []codec
	for _, ident := range field.Names {
		if !ast.IsExported(ident.Name) || skip {
			continue
		}
		name := jname
		if name == "" {
			name = ident.Name
		}
		code, err := g.fieldCodec(owner, ident.Name, name, field.Type)
		if err != nil {
			return nil, err
		}
		if omitEmpty {
			code += ".OmitEmpty()"
		}
		out = append(out, codec{json: name, code: code})
	}
	return out, nil
}

// fieldCodec builds the marshal.Field expression for one field.
func (g *gen) fieldCodec(owner, fname, jname string, expr ast.Expr) (string, error) {
	switch t := expr.(type) {
	case *ast.Ident:
		return g.identCodec(owner, fname, jname, t.Name)
	case *ast.StarExpr:
		return g.starCodec(owner, fname, jname, t.X)
	case *ast.SelectorExpr:
		return g.crossFieldCodec(owner, fname, jname, t)
	case *ast.ArrayType:
		return g.arrayCodec(owner, fname, jname, t)
	case *ast.MapType:
		return g.mapCodec(owner, fname, jname, t)
	case *ast.InterfaceType:
		if len(t.Methods.List) == 0 {
			return g.anyCodec(owner, fname, jname), nil
		}
	}
	return "", fmt.Errorf("%s.%s: unsupported type %s", owner, fname, typeString(g.fset, expr))
}

// identCodec builds a codec for a non-pointer identifier type.
func (g *gen) identCodec(owner, fname, jname, tname string) (string, error) {
	switch tname {
	case "string":
		return ctor("String", owner, jname, fname, "*string"), nil
	case "bool":
		return ctor("Bool", owner, jname, fname, "*bool"), nil
	case "int64":
		return ctor("Int", owner, jname, fname, "*int64"), nil
	case "float64":
		return ctor("Float", owner, jname, fname, "*float64"), nil
	case "any":
		return g.anyCodec(owner, fname, jname), nil
	}
	if basic, ok := g.basicUnderlying(tname); ok {
		return g.numericCodec(owner, fname, jname, tname, basic, false), nil
	}
	if _, ok := g.structs[tname]; ok {
		return g.objectCodec(owner, fname, jname, tname)
	}
	return "", fmt.Errorf("%s.%s: unsupported type %s", owner, fname, tname)
}

// starCodec builds a codec for a pointer field.
func (g *gen) starCodec(owner, fname, jname string, x ast.Expr) (string, error) {
	if sel, ok := x.(*ast.SelectorExpr); ok {
		return g.crossFieldCodecPtr(owner, fname, jname, sel)
	}
	id, ok := x.(*ast.Ident)
	if !ok {
		return "", fmt.Errorf("%s.%s: unsupported pointer type %s", owner, fname, typeString(g.fset, x))
	}
	switch id.Name {
	case "string":
		return ctor("StringPtr", owner, jname, fname, "**string"), nil
	case "bool":
		return ctor("BoolPtr", owner, jname, fname, "**bool"), nil
	case "int64":
		return ctor("IntPtr", owner, jname, fname, "**int64"), nil
	case "float64":
		return ctor("FloatPtr", owner, jname, fname, "**float64"), nil
	}
	if basic, ok := g.basicUnderlying(id.Name); ok {
		return g.numericCodec(owner, fname, jname, id.Name, basic, true), nil
	}
	return "", fmt.Errorf("%s.%s: unsupported pointer type *%s", owner, fname, id.Name)
}

// arrayCodec builds a codec for a slice or fixed-size array field.
func (g *gen) arrayCodec(owner, fname, jname string, t *ast.ArrayType) (string, error) {
	elem := typeString(g.fset, t.Elt)
	if t.Len == nil {
		if elem == "byte" || elem == "uint8" {
			return ctor("Raw", owner, jname, fname, "*[]byte"), nil
		}
		el, err := g.elemCodec(t.Elt)
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", owner, fname, err)
		}
		return fmt.Sprintf("marshal.Slice[%s, %s](%q, func(v *%s) *[]%s { return &v.%s }, %s)",
			owner, elem, jname, owner, elem, fname, el), nil
	}
	el, err := g.elemCodec(t.Elt)
	if err != nil {
		return "", fmt.Errorf("%s.%s: %w", owner, fname, err)
	}
	return fmt.Sprintf("marshal.Array[%s, %s](%q, func(v *%s) []%s { return v.%s[:] }, %s)",
		owner, elem, jname, owner, elem, fname, el), nil
}

// mapCodec builds a codec for a map field with string keys.
func (g *gen) mapCodec(owner, fname, jname string, t *ast.MapType) (string, error) {
	if key := typeString(g.fset, t.Key); key != "string" {
		return "", fmt.Errorf("%s.%s: map key %s is not supported; only string keys are", owner, fname, key)
	}
	val := typeString(g.fset, t.Value)
	el, err := g.elemCodec(t.Value)
	if err != nil {
		return "", fmt.Errorf("%s.%s: %w", owner, fname, err)
	}
	return fmt.Sprintf("marshal.Map[%s, %s](%q, func(v *%s) *map[string]%s { return &v.%s }, %s)",
		owner, val, jname, owner, val, fname, el), nil
}

// anyCodec builds a codec for an any field.
func (g *gen) anyCodec(owner, fname, jname string) string {
	return fmt.Sprintf("marshal.Any[%s](%q, func(v *%s) *any { return &v.%s })", owner, jname, owner, fname)
}

// objectCodec builds a codec for a nested struct field.
func (g *gen) objectCodec(owner, fname, jname, sn string) (string, error) {
	if !g.known[sn] {
		return "", fmt.Errorf("%s.%s: nested struct %s is not generated; add -type=%s", owner, fname, sn, sn)
	}
	return fmt.Sprintf("marshal.Object[%s, %s](%q, func(v *%s) *%s { return &v.%s }, %s)",
		owner, sn, jname, owner, sn, fname, schemaVarName(sn)), nil
}

// ctor builds a call to one of marshal's scalar constructors. retType is the
// accessor's return type, such as "*string" or "**string".
func ctor(fn, owner, jname, fname, retType string) string {
	return fmt.Sprintf("marshal.%s[%s](%q, func(v *%s) %s { return &v.%s })", fn, owner, jname, owner, retType, fname)
}

// numericCodec builds an inline closure for a numeric field whose Go type is
// goType and whose underlying basic type is basic. marshal has no width-aware
// constructor, so the closure converts and range-checks by round-trip.
func (g *gen) numericCodec(owner, fname, jname, goType, basic string, ptr bool) string {
	g.needScanner = true
	g.needRange = true

	var b strings.Builder
	fmt.Fprintf(&b, "marshal.Field[%s]{Name: %q, Decode: func(r *marshal.Reader, v *%s) error {\n", owner, jname, owner)
	b.WriteString("\tk, err := r.Peek()\n\tif err != nil {\n\t\treturn err\n\t}\n")
	if ptr {
		b.WriteString("\tif k == scanner.KindNull {\n\t\tif err := r.ReadNull(); err != nil {\n\t\t\treturn err\n\t\t}\n")
		fmt.Fprintf(&b, "\t\tv.%s = nil\n\t\treturn nil\n\t}\n", fname)
	} else {
		b.WriteString("\tif k == scanner.KindNull {\n\t\treturn r.ReadNull()\n\t}\n")
	}

	switch basic {
	case "float32", "float64":
		if basic == "float32" {
			g.needMath = true
		}
		b.WriteString("\tf, err := r.ReadFloat()\n\tif err != nil {\n\t\treturn err\n\t}\n")
		if basic == "float32" {
			fmt.Fprintf(&b, "\tif f > math.MaxFloat32 || f < -math.MaxFloat32 {\n\t\treturn qiRangeError(r, %q)\n\t}\n", goType)
		}
		if ptr {
			fmt.Fprintf(&b, "\te := %s(f)\n\tv.%s = &e\n", goType, fname)
		} else {
			fmt.Fprintf(&b, "\tv.%s = %s(f)\n", fname, goType)
		}
	default:
		b.WriteString("\tn, err := r.ReadInt()\n\tif err != nil {\n\t\treturn err\n\t}\n")
		if isUnsigned(basic) {
			fmt.Fprintf(&b, "\tif n < 0 || int64(%s(n)) != n {\n\t\treturn qiRangeError(r, %q)\n\t}\n", goType, goType)
		} else {
			fmt.Fprintf(&b, "\tif int64(%s(n)) != n {\n\t\treturn qiRangeError(r, %q)\n\t}\n", goType, goType)
		}
		if ptr {
			fmt.Fprintf(&b, "\te := %s(n)\n\tv.%s = &e\n", goType, fname)
		} else {
			fmt.Fprintf(&b, "\tv.%s = %s(n)\n", fname, goType)
		}
	}
	b.WriteString("\treturn nil\n\t},\n")
	g.writeNumericEncode(&b, owner, fname, basic, ptr)
	g.writeEmpty(&b, owner, fname, ptr)
	b.WriteString("}")
	return b.String()
}

// writeNumericEncode writes the Encode field of a numeric Field literal. access
// is the field selector; a pointer field writes null when nil.
func (g *gen) writeNumericEncode(b *strings.Builder, owner, fname, basic string, ptr bool) {
	access := "v." + fname
	fmt.Fprintf(b, "\tEncode: func(w *marshal.Writer, v *%s) error {\n", owner)
	if ptr {
		fmt.Fprintf(b, "\t\tif %s == nil {\n\t\t\treturn w.WriteNull()\n\t\t}\n", access)
		access = "*" + access
	}
	switch {
	case basic == "float32" || basic == "float64":
		fmt.Fprintf(b, "\t\treturn w.WriteFloat(float64(%s))\n", access)
	case isUnsigned(basic):
		fmt.Fprintf(b, "\t\treturn w.WriteUint(uint64(%s))\n", access)
	default:
		fmt.Fprintf(b, "\t\treturn w.WriteInt(int64(%s))\n", access)
	}
	b.WriteString("\t},\n")
}

// writeEmpty writes the Empty field of a Field literal: a nil pointer or a zero
// value is empty for omitempty.
func (g *gen) writeEmpty(b *strings.Builder, owner, fname string, ptr bool) {
	if ptr {
		fmt.Fprintf(b, "\tEmpty: func(v *%s) bool { return v.%s == nil },\n", owner, fname)
	} else {
		fmt.Fprintf(b, "\tEmpty: func(v *%s) bool { return v.%s == 0 },\n", owner, fname)
	}
}

// elemCodec returns a marshal.Codec expression for a container element type,
// pairing the element's decoder and encoder.
func (g *gen) elemCodec(expr ast.Expr) (string, error) {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		return g.crossElemCodec(sel)
	}
	id, ok := expr.(*ast.Ident)
	if !ok {
		return "", fmt.Errorf("unsupported element type %s", typeString(g.fset, expr))
	}
	switch id.Name {
	case "string":
		return "marshal.NewCodec((*marshal.Reader).ReadString, (*marshal.Writer).WriteString)", nil
	case "bool":
		return "marshal.NewCodec((*marshal.Reader).ReadBool, (*marshal.Writer).WriteBool)", nil
	case "int64":
		return "marshal.NewCodec((*marshal.Reader).ReadInt, (*marshal.Writer).WriteInt)", nil
	case "float64":
		return "marshal.NewCodec((*marshal.Reader).ReadFloat, (*marshal.Writer).WriteFloat)", nil
	}
	if basic, ok := g.basicUnderlying(id.Name); ok {
		return fmt.Sprintf("marshal.NewCodec(%s, %s)", g.numericHelper(id.Name, basic), g.numericEncodeHelper(id.Name, basic)), nil
	}
	if _, ok := g.structs[id.Name]; ok {
		dec, err := g.structHelper(id.Name)
		if err != nil {
			return "", err
		}
		enc, err := g.structEncodeHelper(id.Name)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("marshal.NewCodec(%s, %s)", dec, enc), nil
	}
	return "", fmt.Errorf("unsupported element type %s", id.Name)
}

// numericHelper emits, once, a function that decodes a number into goType and
// returns its name.
func (g *gen) numericHelper(goType, basic string) string {
	hn := "qiDecode" + upperFirst(goType)
	if _, ok := g.helpers[hn]; ok {
		return hn
	}
	g.needRange = true

	var b strings.Builder
	fmt.Fprintf(&b, "func %s(r *marshal.Reader) (%s, error) {\n", hn, goType)
	switch basic {
	case "float32", "float64":
		if basic == "float32" {
			g.needMath = true
		}
		b.WriteString("\tf, err := r.ReadFloat()\n\tif err != nil {\n\t\treturn 0, err\n\t}\n")
		if basic == "float32" {
			fmt.Fprintf(&b, "\tif f > math.MaxFloat32 || f < -math.MaxFloat32 {\n\t\treturn 0, qiRangeError(r, %q)\n\t}\n", goType)
		}
		fmt.Fprintf(&b, "\treturn %s(f), nil\n}\n", goType)
	default:
		b.WriteString("\tn, err := r.ReadInt()\n\tif err != nil {\n\t\treturn 0, err\n\t}\n")
		if isUnsigned(basic) {
			fmt.Fprintf(&b, "\tif n < 0 || int64(%s(n)) != n {\n\t\treturn 0, qiRangeError(r, %q)\n\t}\n", goType, goType)
		} else {
			fmt.Fprintf(&b, "\tif int64(%s(n)) != n {\n\t\treturn 0, qiRangeError(r, %q)\n\t}\n", goType, goType)
		}
		fmt.Fprintf(&b, "\treturn %s(n), nil\n}\n", goType)
	}
	g.helpers[hn] = b.String()
	g.helperOrder = append(g.helperOrder, hn)
	return hn
}

// structHelper emits, once, a function that decodes a nested struct and returns
// its name.
func (g *gen) structHelper(name string) (string, error) {
	if !g.known[name] {
		return "", fmt.Errorf("nested struct %s is not generated; add -type=%s", name, name)
	}
	hn := "qiDecode" + upperFirst(name)
	if _, ok := g.helpers[hn]; ok {
		return hn, nil
	}
	g.helpers[hn] = fmt.Sprintf(
		"func %s(r *marshal.Reader) (%s, error) {\n\tvar v %s\n\tif err := %s.Decode(r, &v); err != nil {\n\t\treturn v, err\n\t}\n\treturn v, nil\n}\n",
		hn, name, name, schemaVarName(name))
	g.helperOrder = append(g.helperOrder, hn)
	return hn, nil
}

// numericEncodeHelper emits, once, a function that encodes goType to a Writer and
// returns its name.
func (g *gen) numericEncodeHelper(goType, basic string) string {
	hn := "qiEncode" + upperFirst(goType)
	if _, ok := g.helpers[hn]; ok {
		return hn
	}
	var b strings.Builder
	fmt.Fprintf(&b, "func %s(w *marshal.Writer, v %s) error {\n", hn, goType)
	switch {
	case basic == "float32" || basic == "float64":
		b.WriteString("\treturn w.WriteFloat(float64(v))\n}\n")
	case isUnsigned(basic):
		b.WriteString("\treturn w.WriteUint(uint64(v))\n}\n")
	default:
		b.WriteString("\treturn w.WriteInt(int64(v))\n}\n")
	}
	g.helpers[hn] = b.String()
	g.helperOrder = append(g.helperOrder, hn)
	return hn
}

// structEncodeHelper emits, once, a function that encodes a nested struct and
// returns its name.
func (g *gen) structEncodeHelper(name string) (string, error) {
	if !g.known[name] {
		return "", fmt.Errorf("nested struct %s is not generated; add -type=%s", name, name)
	}
	hn := "qiEncode" + upperFirst(name)
	if _, ok := g.helpers[hn]; ok {
		return hn, nil
	}
	g.helpers[hn] = fmt.Sprintf(
		"func %s(w *marshal.Writer, v %s) error {\n\treturn %s.Encode(w, &v)\n}\n",
		hn, name, schemaVarName(name))
	g.helperOrder = append(g.helperOrder, hn)
	return hn, nil
}

// basicUnderlying returns the predeclared basic type a name resolves to in this
// package, following named types, or false when it resolves to something else.
func (g *gen) basicUnderlying(name string) (string, bool) {
	return g.resolveBasic(name, map[string]bool{})
}

func (g *gen) resolveBasic(name string, seen map[string]bool) (string, bool) {
	switch name {
	case "byte":
		return "uint8", true
	case "rune":
		return "int32", true
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64":
		return name, true
	}
	ts, ok := g.decls[name]
	if !ok || seen[name] {
		return "", false
	}
	seen[name] = true
	id, ok := ts.Type.(*ast.Ident)
	if !ok {
		return "", false
	}
	return g.resolveBasic(id.Name, seen)
}

// jsonTag returns the json member name (empty when absent), whether the field is
// skipped, and whether it is tagged omitempty. It mirrors encoding/json's tag
// rules: `json:"-"` skips, `json:"-,"` names the member "-", and a bare
// `json:"name,omitempty"` names it "name" and marks it omitempty.
func jsonTag(tag *ast.BasicLit) (name string, skip, omitEmpty bool, err error) {
	if tag == nil {
		return "", false, false, nil
	}
	raw, err := strconv.Unquote(tag.Value)
	if err != nil {
		return "", false, false, fmt.Errorf("bad struct tag %s", tag.Value)
	}
	val := parseTag(raw, "json")
	switch {
	case val == "-":
		return "", true, false, nil
	case strings.HasPrefix(val, "-,"):
		return "-", false, hasTagOption(val[2:], "omitempty"), nil
	default:
		parts := strings.SplitN(val, ",", 2)
		if len(parts) == 2 && hasTagOption(parts[1], "omitempty") {
			omitEmpty = true
		}
		return parts[0], false, omitEmpty, nil
	}
}

// hasTagOption reports whether a json tag's comma-separated option list holds
// want.
func hasTagOption(opts, want string) bool {
	for opts != "" {
		var opt string
		if i := strings.IndexByte(opts, ','); i >= 0 {
			opt, opts = opts[:i], opts[i+1:]
		} else {
			opt, opts = opts, ""
		}
		if opt == want {
			return true
		}
	}
	return false
}

// parseTag returns the value of the key in a struct tag, or "". It is a
// dependency-free stand-in for reflect.StructTag.Get, which the tool may not use.
func parseTag(tag, key string) string {
	for tag != "" {
		i := 0
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		tag = tag[i:]
		i = 0
		for i < len(tag) && tag[i] > ' ' && tag[i] != ':' && tag[i] != '"' {
			i++
		}
		if i == 0 || i+1 >= len(tag) || tag[i] != ':' || tag[i+1] != '"' {
			break
		}
		name := tag[:i]
		tag = tag[i+1:]
		i = 1
		for i < len(tag) && tag[i] != '"' {
			if tag[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(tag) {
			break
		}
		value := tag[1:i]
		tag = tag[i+1:]
		if name == key {
			return value
		}
	}
	return ""
}

// hasMarker reports whether a comment group contains the marker.
func hasMarker(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	return strings.Contains(doc.Text(), marker)
}

// recvTypeName returns the name of a method receiver's base type, or "".
func recvTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.IndexExpr:
		return recvTypeName(t.X)
	}
	return ""
}

// typeString renders a type expression back to source.
func typeString(fset *token.FileSet, expr ast.Expr) string {
	var b bytes.Buffer
	_ = printer.Fprint(&b, fset, expr)
	return b.String()
}

// isUnsigned reports whether basic is an unsigned integer type.
func isUnsigned(basic string) bool { return strings.HasPrefix(basic, "uint") }

// schemaVarName is the unexported name of the schema var for a type.
func schemaVarName(name string) string { return lowerFirst(name) + "Schema" }

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
