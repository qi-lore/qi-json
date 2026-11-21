package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/types"
	"strings"
)

// typecheck fills g.info by type-checking the package files with the standard
// library's source importer. Cross-package named types need this: the AST alone
// cannot tell that time.Duration is an int64. It is best-effort — when the
// package does not type-check, g.info stays nil and a cross-package field
// reports a precise unsupported-type error instead.
//
// The importer is the stdlib source importer, so no third-party code is involved
// and module mode is handled by the toolchain. Type checking runs once per
// generation.
func (g *gen) typecheck(files []*ast.File) {
	if len(files) == 0 {
		return
	}
	info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue)}
	conf := types.Config{Importer: importer.ForCompiler(g.fset, "source", nil)}
	if _, err := conf.Check("qi-json-gen/target", g.fset, files, info); err != nil {
		return
	}
	g.info = info
}

// namedOf returns the *types.Named denoted by expr, if type information is
// available and expr denotes a named type.
func (g *gen) namedOf(expr ast.Expr) (*types.Named, bool) {
	if g.info == nil {
		return nil, false
	}
	named, ok := g.info.TypeOf(expr).(*types.Named)
	return named, ok
}

// addImport records that the generated file must import path under name.
func (g *gen) addImport(path, name string) {
	g.extraImports[path] = name
}

// crossFieldCodec builds a codec for a non-pointer cross-package named type.
func (g *gen) crossFieldCodec(owner, fname, jname string, sel *ast.SelectorExpr) (string, error) {
	named, ok := g.namedOf(sel)
	if !ok {
		return "", fmt.Errorf("%s.%s: unsupported type %s", owner, fname, typeString(g.fset, sel))
	}
	return g.crossNamedCodec(owner, fname, jname, named, false)
}

// crossFieldCodecPtr builds a codec for a pointer to a cross-package named type.
func (g *gen) crossFieldCodecPtr(owner, fname, jname string, sel *ast.SelectorExpr) (string, error) {
	named, ok := g.namedOf(sel)
	if !ok {
		return "", fmt.Errorf("%s.%s: unsupported pointer type *%s", owner, fname, typeString(g.fset, sel))
	}
	return g.crossNamedCodec(owner, fname, jname, named, true)
}

// crossNamedCodec builds a codec for a cross-package named type whose underlying
// type is a supported basic scalar. A non-scalar underlying type (such as a
// cross-package struct) is reported precisely: it would need its own generated
// DecodeJSON in the other package, which is out of scope.
func (g *gen) crossNamedCodec(owner, fname, jname string, named *types.Named, ptr bool) (string, error) {
	obj := named.Obj()
	pkg := obj.Pkg()
	if pkg == nil {
		return "", fmt.Errorf("%s.%s: unsupported type %s", owner, fname, obj.Name())
	}
	goType := pkg.Name() + "." + obj.Name()
	basic, ok := named.Underlying().(*types.Basic)
	if !ok {
		return "", fmt.Errorf("%s.%s: unsupported cross-package type %s (only basic scalar underlying types are supported)", owner, fname, goType)
	}
	g.addImport(pkg.Path(), pkg.Name())
	return g.crossScalarCodec(owner, fname, jname, goType, basic, ptr)
}

// crossScalarCodec builds a codec for a cross-package named scalar. String and
// bool are decoded directly; a numeric basic type reuses numericCodec, which
// range-checks by round-trip.
func (g *gen) crossScalarCodec(owner, fname, jname, goType string, basic *types.Basic, ptr bool) (string, error) {
	switch basic.Kind() {
	case types.String:
		g.needScanner = true
		return g.crossScalarClosure(owner, fname, jname, goType, ptr,
			"\ts, err := r.ReadString()\n\tif err != nil {\n\t\treturn err\n\t}\n",
			"s"), nil
	case types.Bool:
		g.needScanner = true
		return g.crossScalarClosure(owner, fname, jname, goType, ptr,
			"\tb, err := r.ReadBool()\n\tif err != nil {\n\t\treturn err\n\t}\n",
			"b"), nil
	}
	name, ok := basicKindName(basic.Kind())
	if !ok {
		return "", fmt.Errorf("%s.%s: unsupported cross-package type %s (underlying %s)", owner, fname, goType, basic.Name())
	}
	return g.numericCodec(owner, fname, jname, goType, name, ptr), nil
}

// crossScalarClosure builds a Field closure for a cross-package named string or
// bool type. read is the statement that reads the value into `name`, and name is
// the local holding it.
func (g *gen) crossScalarClosure(owner, fname, jname, goType string, ptr bool, read, name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "marshal.Field[%s]{Name: %q, Decode: func(r *marshal.Reader, v *%s) error {\n", owner, jname, owner)
	b.WriteString("\tk, err := r.Peek()\n\tif err != nil {\n\t\treturn err\n\t}\n")
	if ptr {
		b.WriteString("\tif k == scanner.KindNull {\n\t\tif err := r.ReadNull(); err != nil {\n\t\t\treturn err\n\t\t}\n")
		fmt.Fprintf(&b, "\t\tv.%s = nil\n\t\treturn nil\n\t}\n", fname)
	} else {
		b.WriteString("\tif k == scanner.KindNull {\n\t\treturn r.ReadNull()\n\t}\n")
	}
	b.WriteString(read)
	if ptr {
		fmt.Fprintf(&b, "\te := %s(%s)\n\tv.%s = &e\n", goType, name, fname)
	} else {
		fmt.Fprintf(&b, "\tv.%s = %s(%s)\n", fname, goType, name)
	}
	b.WriteString("\treturn nil\n}}")
	return b.String()
}

// crossElemDecoderExpr returns a decoder for a cross-package element type.
func (g *gen) crossElemDecoderExpr(sel *ast.SelectorExpr) (string, error) {
	named, ok := g.namedOf(sel)
	if !ok {
		return "", fmt.Errorf("unsupported element type %s", typeString(g.fset, sel))
	}
	return g.crossElemDecoder(named)
}

// crossElemDecoder emits, once, a function that decodes a cross-package named
// scalar and returns its name.
func (g *gen) crossElemDecoder(named *types.Named) (string, error) {
	obj := named.Obj()
	pkg := obj.Pkg()
	if pkg == nil {
		return "", fmt.Errorf("unsupported element type %s", obj.Name())
	}
	goType := pkg.Name() + "." + obj.Name()
	basic, ok := named.Underlying().(*types.Basic)
	if !ok {
		return "", fmt.Errorf("unsupported cross-package element type %s (only basic scalar underlying types are supported)", goType)
	}
	g.addImport(pkg.Path(), pkg.Name())

	hn := "qiDecode" + crossHelperSuffix(goType)
	if _, ok := g.helpers[hn]; ok {
		return hn, nil
	}
	body, err := g.crossElemBody(hn, goType, basic)
	if err != nil {
		return "", err
	}
	g.helpers[hn] = body
	g.helperOrder = append(g.helperOrder, hn)
	return hn, nil
}

// crossElemBody builds the body of a cross-package element helper.
func (g *gen) crossElemBody(hn, goType string, basic *types.Basic) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "func %s(r *marshal.Reader) (%s, error) {\n", hn, goType)
	switch basic.Kind() {
	case types.String:
		b.WriteString("\ts, err := r.ReadString()\n\tif err != nil {\n\t\treturn \"\", err\n\t}\n")
		fmt.Fprintf(&b, "\treturn %s(s), nil\n}\n", goType)
		return b.String(), nil
	case types.Bool:
		b.WriteString("\tb, err := r.ReadBool()\n\tif err != nil {\n\t\treturn false, err\n\t}\n")
		fmt.Fprintf(&b, "\treturn %s(b), nil\n}\n", goType)
		return b.String(), nil
	}

	basicName, ok := basicKindName(basic.Kind())
	if !ok {
		return "", fmt.Errorf("unsupported cross-package type %s (underlying %s)", goType, basic.Name())
	}
	g.needRange = true
	switch basicName {
	case "float32", "float64":
		if basicName == "float32" {
			g.needMath = true
		}
		b.WriteString("\tf, err := r.ReadFloat()\n\tif err != nil {\n\t\treturn 0, err\n\t}\n")
		if basicName == "float32" {
			fmt.Fprintf(&b, "\tif f > math.MaxFloat32 || f < -math.MaxFloat32 {\n\t\treturn 0, qiRangeError(r, %q)\n\t}\n", goType)
		}
		fmt.Fprintf(&b, "\treturn %s(f), nil\n}\n", goType)
		return b.String(), nil
	}

	b.WriteString("\tn, err := r.ReadInt()\n\tif err != nil {\n\t\treturn 0, err\n\t}\n")
	if isUnsigned(basicName) {
		fmt.Fprintf(&b, "\tif n < 0 || int64(%s(n)) != n {\n\t\treturn 0, qiRangeError(r, %q)\n\t}\n", goType, goType)
	} else {
		fmt.Fprintf(&b, "\tif int64(%s(n)) != n {\n\t\treturn 0, qiRangeError(r, %q)\n\t}\n", goType, goType)
	}
	fmt.Fprintf(&b, "\treturn %s(n), nil\n}\n", goType)
	return b.String(), nil
}

// basicKindName maps a numeric basic kind to its Go spelling.
func basicKindName(k types.BasicKind) (string, bool) {
	switch k {
	case types.Int:
		return "int", true
	case types.Int8:
		return "int8", true
	case types.Int16:
		return "int16", true
	case types.Int32:
		return "int32", true
	case types.Int64:
		return "int64", true
	case types.Uint:
		return "uint", true
	case types.Uint8:
		return "uint8", true
	case types.Uint16:
		return "uint16", true
	case types.Uint32:
		return "uint32", true
	case types.Uint64:
		return "uint64", true
	case types.Float32:
		return "float32", true
	case types.Float64:
		return "float64", true
	}
	return "", false
}

// crossHelperSuffix turns a qualified type name into an identifier suffix:
// "time.Duration" becomes "TimeDuration".
func crossHelperSuffix(goType string) string {
	parts := strings.Split(goType, ".")
	for i, p := range parts {
		parts[i] = upperFirst(p)
	}
	return strings.Join(parts, "")
}
