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
// cannot tell that time.Duration is an int64.
//
// It is best-effort. Type errors are collected rather than fatal, so an unrelated
// error elsewhere in the package (for example a stale generated file) still
// leaves the resolvable types in g.info; an expression the checker could not
// resolve is simply missing from g.info and its field reports a precise
// unsupported-type error.
//
// The importer is the stdlib source importer, so no third-party code is involved
// and module mode is handled by the toolchain. Type checking runs once per
// generation.
func (g *gen) typecheck(files []*ast.File) {
	if len(files) == 0 {
		return
	}
	info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue)}
	conf := types.Config{
		Importer: importer.ForCompiler(g.fset, "source", nil),
		Error:    func(error) {}, // keep going; g.info holds what resolved
	}
	_, _ = conf.Check("qi-json-gen/target", g.fset, files, info)
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
	if code, handled := g.overrideField(owner, fname, jname, named, false); handled {
		return code, nil
	}
	if code, handled, err := g.crossDelegation(owner, fname, jname, named, false); handled {
		return code, err
	}
	return g.crossNamedCodec(owner, fname, jname, named, false)
}

// crossFieldCodecPtr builds a codec for a pointer to a cross-package named type.
func (g *gen) crossFieldCodecPtr(owner, fname, jname string, sel *ast.SelectorExpr) (string, error) {
	named, ok := g.namedOf(sel)
	if !ok {
		return "", fmt.Errorf("%s.%s: unsupported pointer type *%s", owner, fname, typeString(g.fset, sel))
	}
	if code, handled := g.overrideField(owner, fname, jname, named, true); handled {
		return code, nil
	}
	if code, handled, err := g.crossDelegation(owner, fname, jname, named, true); handled {
		return code, err
	}
	return g.crossNamedCodec(owner, fname, jname, named, true)
}

// crossDelegation builds a codec for a cross-package named type that implements
// both qijson.Decoder and qijson.Encoder: a field bound by ObjectDecoder (or
// ObjectDecoderPtr for a pointer field), calling the type's own methods. It
// returns handled=false when the type implements neither interface. A type that
// implements only one of the two is a precise error, so decode and encode stay
// symmetric.
func (g *gen) crossDelegation(owner, fname, jname string, named *types.Named, ptr bool) (string, bool, error) {
	dec, enc := g.decoderEncoder(named)
	if !dec && !enc {
		return "", false, nil
	}
	obj := named.Obj()
	pkg := obj.Pkg()
	if pkg == nil {
		return "", false, nil
	}
	goType := pkg.Name() + "." + obj.Name()
	if dec != enc {
		return "", true, fmt.Errorf("%s.%s: cross-package type %s implements only one of DecodeJSON/EncodeJSON; both are required for a symmetric binding", owner, fname, goType)
	}
	g.addImport(pkg.Path(), pkg.Name())
	if ptr {
		return fmt.Sprintf("qijson.ObjectDecoderPtr[%s, %s](%q, func(v *%s) **%s { return &v.%s })", owner, goType, jname, owner, goType, fname), true, nil
	}
	return fmt.Sprintf("qijson.ObjectDecoder[%s, %s](%q, func(v *%s) *%s { return &v.%s })", owner, goType, jname, owner, goType, fname), true, nil
}

// decoderEncoder reports whether the pointer type *named implements qijson's
// Decoder and Encoder, by inspecting its method set with go/types. Only methods
// with the exact qijson signature count.
func (g *gen) decoderEncoder(named *types.Named) (dec, enc bool) {
	ms := types.NewMethodSet(types.NewPointer(named))
	for i := 0; i < ms.Len(); i++ {
		fn, ok := ms.At(i).Obj().(*types.Func)
		if !ok {
			continue
		}
		switch fn.Name() {
		case "DecodeJSON":
			dec = dec || qijsonMethod(fn, "Reader")
		case "EncodeJSON":
			enc = enc || qijsonMethod(fn, "Writer")
		}
	}
	return dec, enc
}

// qijsonMethod reports whether fn has the shape of a qijson interface method:
// one parameter that is a pointer to a named type called want in package
// qijson, and a single error result.
func qijsonMethod(fn *types.Func, want string) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return false
	}
	ptr, ok := sig.Params().At(0).Type().(*types.Pointer)
	if !ok {
		return false
	}
	if !isQijsonType(ptr.Elem(), want) {
		return false
	}
	return types.Identical(sig.Results().At(0).Type(), types.Universe.Lookup("error").Type())
}

// The public facade package and the engine it re-exports. A method written as
// DecodeJSON(*qijson.Reader) type-checks either as an alias to the engine type
// or as the engine type itself, depending on how the toolchain materializes
// aliases (GODEBUG=gotypesalias), so both identities are accepted.
const (
	qijsonPath       = "qi-json"
	qijsonEnginePath = "qi-json/internal/codec"
)

// isQijsonType reports whether t is the public qijson type named want (Reader
// or Writer), either as the facade alias or as the engine type it resolves to.
func isQijsonType(t types.Type, want string) bool {
	if a, ok := t.(*types.Alias); ok {
		obj := a.Obj()
		if obj.Pkg() != nil && obj.Pkg().Path() == qijsonPath && obj.Name() == want {
			return true
		}
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj.Pkg() == nil || obj.Name() != want {
		return false
	}
	switch obj.Pkg().Path() {
	case qijsonPath, qijsonEnginePath:
		return true
	}
	return false
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
		if _, isStruct := named.Underlying().(*types.Struct); isStruct {
			return g.crossStructCodec(owner, fname, jname, named, ptr)
		}
		return "", fmt.Errorf("%s.%s: unsupported cross-package type %s (only basic scalar or struct underlying types are supported)", owner, fname, goType)
	}
	g.addImport(pkg.Path(), pkg.Name())
	return g.crossScalarCodec(owner, fname, jname, goType, basic, ptr)
}

// crossStructCodec builds a codec for a plain cross-package struct (one that is
// not qi-json-aware, or crossDelegation would have handled it). It synthesizes a
// schema for the foreign struct's exported fields (approach A) and binds the
// field with Object.
func (g *gen) crossStructCodec(owner, fname, jname string, named *types.Named, ptr bool) (string, error) {
	obj := named.Obj()
	pkg := obj.Pkg()
	goType := pkg.Name() + "." + obj.Name()
	g.addForeign(named)
	g.addImport(pkg.Path(), pkg.Name())
	if ptr {
		return g.schemaPtrCodec(owner, fname, jname, goType, foreignSchemaVar(goType)), nil
	}
	return fmt.Sprintf("qijson.Object[%s, %s](%q, func(v *%s) *%s { return &v.%s }, %s)",
		owner, goType, jname, owner, goType, fname, foreignSchemaVar(goType)), nil
}

// crossScalarCodec builds a codec for a cross-package named scalar. String and
// bool are decoded directly through the shared scalarClosure; a numeric basic
// type reuses numericCodec, which range-checks by round-trip.
func (g *gen) crossScalarCodec(owner, fname, jname, goType string, basic *types.Basic, ptr bool) (string, error) {
	switch basic.Kind() {
	case types.String:
		return g.scalarClosure(owner, fname, jname, goType, "string", ptr), nil
	case types.Bool:
		return g.scalarClosure(owner, fname, jname, goType, "bool", ptr), nil
	}
	name, ok := basicKindName(basic.Kind())
	if !ok {
		return "", fmt.Errorf("%s.%s: unsupported cross-package type %s (underlying %s)", owner, fname, goType, basic.Name())
	}
	return g.numericCodec(owner, fname, jname, goType, name, ptr), nil
}

// crossElemCodec returns a qijson.Codec expression for a cross-package element
// type, emitting paired decode and encode helpers.
func (g *gen) crossElemCodec(sel *ast.SelectorExpr) (string, error) {
	named, ok := g.namedOf(sel)
	if !ok {
		return "", fmt.Errorf("unsupported element type %s", typeString(g.fset, sel))
	}
	if code, handled := g.overrideElem(named); handled {
		return code, nil
	}
	if code, handled, err := g.crossElemDelegation(named); handled {
		return code, err
	}
	if _, ok := named.Underlying().(*types.Struct); ok {
		return g.crossElemStruct(named)
	}
	dec, err := g.crossElemDecoder(named)
	if err != nil {
		return "", err
	}
	enc, err := g.crossElemEncoder(named)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("qijson.NewCodec(%s, %s)", dec, enc), nil
}

// crossElemDelegation returns a DecoderCodec expression for a cross-package
// element type that implements both qijson.Decoder and qijson.Encoder, or
// handled=false when it implements neither. A one-sided type is a precise error.
func (g *gen) crossElemDelegation(named *types.Named) (string, bool, error) {
	dec, enc := g.decoderEncoder(named)
	if !dec && !enc {
		return "", false, nil
	}
	obj := named.Obj()
	pkg := obj.Pkg()
	if pkg == nil {
		return "", false, nil
	}
	goType := pkg.Name() + "." + obj.Name()
	if dec != enc {
		return "", true, fmt.Errorf("cross-package element type %s implements only one of DecodeJSON/EncodeJSON; both are required for a symmetric binding", goType)
	}
	g.addImport(pkg.Path(), pkg.Name())
	return fmt.Sprintf("qijson.DecoderCodec[%s, *%s]()", goType, goType), true, nil
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
	switch basic.Kind() {
	case types.String:
		return namedScalarDecodeBody(hn, goType, "string"), nil
	case types.Bool:
		return namedScalarDecodeBody(hn, goType, "bool"), nil
	}

	basicName, ok := basicKindName(basic.Kind())
	if !ok {
		return "", fmt.Errorf("unsupported cross-package type %s (underlying %s)", goType, basic.Name())
	}
	g.needRange = true
	var b strings.Builder
	fmt.Fprintf(&b, "func %s(r *qijson.Reader) (%s, error) {\n", hn, goType)
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

// crossElemEncoder emits, once, a function that encodes a cross-package named
// scalar and returns its name.
func (g *gen) crossElemEncoder(named *types.Named) (string, error) {
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

	hn := "qiEncode" + crossHelperSuffix(goType)
	if _, ok := g.helpers[hn]; ok {
		return hn, nil
	}
	body, err := g.crossElemEncodeBody(hn, goType, basic)
	if err != nil {
		return "", err
	}
	g.helpers[hn] = body
	g.helperOrder = append(g.helperOrder, hn)
	return hn, nil
}

// crossElemEncodeBody builds the body of a cross-package element encode helper.
func (g *gen) crossElemEncodeBody(hn, goType string, basic *types.Basic) (string, error) {
	switch basic.Kind() {
	case types.String:
		return namedScalarEncodeBody(hn, goType, "string"), nil
	case types.Bool:
		return namedScalarEncodeBody(hn, goType, "bool"), nil
	}

	basicName, ok := basicKindName(basic.Kind())
	if !ok {
		return "", fmt.Errorf("unsupported cross-package type %s (underlying %s)", goType, basic.Name())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "func %s(w *qijson.Writer, v %s) error {\n", hn, goType)
	switch {
	case basicName == "float32" || basicName == "float64":
		b.WriteString("\treturn w.WriteFloat(float64(v))\n}\n")
	case isUnsigned(basicName):
		b.WriteString("\treturn w.WriteUint(uint64(v))\n}\n")
	default:
		b.WriteString("\treturn w.WriteInt(int64(v))\n}\n")
	}
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
