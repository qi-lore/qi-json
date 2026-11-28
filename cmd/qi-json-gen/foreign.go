package main

import (
	"bytes"
	"fmt"
	"go/types"
	"sort"
)

// foreignType is one foreign struct whose schema the generator synthesizes into
// the consumer package (approach A).
type foreignType struct {
	goType string // qualified name, for example "xtypes.Thing"
	strct  *types.Struct
}

// foreignSchemaVar is the unexported schema var name for a foreign struct, for
// example "xtypesThingSchema" for xtypes.Thing.
func foreignSchemaVar(goType string) string {
	return lowerFirst(crossHelperSuffix(goType)) + "Schema"
}

// addForeign records a plain foreign struct so its schema is emitted, and
// recurses into its exported fields to discover transitively nested foreign
// structs. It is idempotent, so a type reachable by two paths is emitted once.
func (g *gen) addForeign(named *types.Named) {
	obj := named.Obj()
	pkg := obj.Pkg()
	if pkg == nil || !obj.Exported() {
		return
	}
	strct, ok := named.Underlying().(*types.Struct)
	if !ok {
		return
	}
	goType := pkg.Name() + "." + obj.Name()
	if _, ok := g.foreign[goType]; ok {
		return
	}
	g.foreign[goType] = &foreignType{goType: goType, strct: strct}
	g.addImport(pkg.Path(), pkg.Name())
	for i := 0; i < strct.NumFields(); i++ {
		v := strct.Field(i)
		if !v.Exported() {
			continue
		}
		g.discoverForeign(v.Type())
	}
}

// discoverForeign recurses into a field type to register every plain foreign
// struct it reaches, so the full set is known before any schema is rendered and
// the emission order can be sorted.
func (g *gen) discoverForeign(t types.Type) {
	switch tt := t.(type) {
	case *types.Named:
		if tt.Obj().Pkg() == nil || !tt.Obj().Exported() {
			return
		}
		if _, ok := tt.Underlying().(*types.Struct); ok {
			if dec, enc := g.decoderEncoder(tt); !dec && !enc {
				g.addForeign(tt)
			}
		}
	case *types.Pointer:
		g.discoverForeign(tt.Elem())
	case *types.Slice:
		g.discoverForeign(tt.Elem())
	case *types.Array:
		g.discoverForeign(tt.Elem())
	case *types.Map:
		g.discoverForeign(tt.Elem())
	}
}

// renderForeign emits one schema var per discovered foreign struct, in sorted
// qualified-name order so generation is deterministic.
func (g *gen) renderForeign(buf *bytes.Buffer) error {
	names := make([]string, 0, len(g.foreign))
	for n := range g.foreign {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		buf.WriteString("\n")
		if err := g.renderForeignSchema(buf, g.foreign[n]); err != nil {
			return err
		}
	}
	return nil
}

// renderForeignSchema emits the schema var for one foreign struct from its
// exported fields, skipping unexported ones (which sets the approximate note).
func (g *gen) renderForeignSchema(buf *bytes.Buffer, ft *foreignType) error {
	fmt.Fprintf(buf, "var %s = qijson.Struct[%s](\n", foreignSchemaVar(ft.goType), ft.goType)
	used := make(map[string]bool, ft.strct.NumFields())
	for i := 0; i < ft.strct.NumFields(); i++ {
		v := ft.strct.Field(i)
		if !v.Exported() {
			g.foreignApprox = true
			continue
		}
		if v.Embedded() {
			return fmt.Errorf("%s: embedded field %s is not supported", ft.goType, v.Name())
		}
		jname, skip, omitEmpty, err := jsonTagValue(ft.strct.Tag(i))
		if err != nil {
			return fmt.Errorf("%s: %w", ft.goType, err)
		}
		if skip {
			continue
		}
		name := jname
		if name == "" {
			name = v.Name()
		}
		g.omitEmpty = omitEmpty
		code, err := g.foreignFieldCodec(ft.goType, v.Name(), name, v.Type())
		if err != nil {
			return err
		}
		if omitEmpty {
			code += ".OmitEmpty()"
		}
		if used[name] {
			return fmt.Errorf("%s: duplicate member name %q", ft.goType, name)
		}
		used[name] = true
		fmt.Fprintf(buf, "\t%s,\n", code)
	}
	buf.WriteString(")\n\n")
	return nil
}

// foreignFieldCodec builds the Field expression for an exported foreign field
// whose Go type is t. owner is the qualified struct name, fname the Go field,
// and jname the JSON member.
func (g *gen) foreignFieldCodec(owner, fname, jname string, t types.Type) (string, error) {
	switch tt := t.(type) {
	case *types.Basic:
		return g.basicFieldCodec(owner, fname, jname, tt)
	case *types.Named:
		return g.namedFieldCodec(owner, fname, jname, tt)
	case *types.Pointer:
		return g.foreignStarCodec(owner, fname, jname, tt)
	case *types.Slice:
		return g.foreignSliceCodec(owner, fname, jname, tt)
	case *types.Array:
		return g.foreignArrayCodec(owner, fname, jname, tt)
	case *types.Map:
		return g.foreignMapCodec(owner, fname, jname, tt)
	case *types.Interface:
		if tt.NumMethods() == 0 {
			return g.anyCodec(owner, fname, jname), nil
		}
	}
	return "", fmt.Errorf("%s.%s: unsupported type %s", owner, fname, t.String())
}

// basicFieldCodec builds a codec for a predeclared basic field type.
func (g *gen) basicFieldCodec(owner, fname, jname string, t *types.Basic) (string, error) {
	switch t.Kind() {
	case types.String:
		return ctor("String", owner, jname, fname, "*string"), nil
	case types.Bool:
		return ctor("Bool", owner, jname, fname, "*bool"), nil
	case types.Int64:
		return ctor("Int", owner, jname, fname, "*int64"), nil
	case types.Float64:
		return ctor("Float", owner, jname, fname, "*float64"), nil
	}
	if name, ok := basicKindName(t.Kind()); ok {
		return g.numericCodec(owner, fname, jname, name, name, false), nil
	}
	return "", fmt.Errorf("%s.%s: unsupported type %s", owner, fname, t.String())
}

// namedFieldCodec builds a codec for a named field type. A named basic scalar
// converts through the shared closure; a struct is delegated (B) or synthesized
// (A); anything else is a precise error.
func (g *gen) namedFieldCodec(owner, fname, jname string, t *types.Named) (string, error) {
	if goType, ident, ok := g.override(t); ok {
		return fmt.Sprintf("qijson.Object[%s, %s](%q, func(v *%s) *%s { return &v.%s }, %s)",
			owner, goType, jname, owner, goType, fname, ident), nil
	}
	obj := t.Obj()
	pkg := obj.Pkg()
	if pkg == nil {
		return "", fmt.Errorf("%s.%s: unsupported type %s", owner, fname, obj.Name())
	}
	if !obj.Exported() {
		return "", fmt.Errorf("%s.%s: unexported type %s cannot be bound from another package", owner, fname, obj.Name())
	}
	goType := pkg.Name() + "." + obj.Name()
	switch u := t.Underlying().(type) {
	case *types.Basic:
		g.addImport(pkg.Path(), pkg.Name())
		switch u.Kind() {
		case types.String:
			return g.scalarClosure(owner, fname, jname, goType, "string", false), nil
		case types.Bool:
			return g.scalarClosure(owner, fname, jname, goType, "bool", false), nil
		}
		name, ok := basicKindName(u.Kind())
		if !ok {
			return "", fmt.Errorf("%s.%s: unsupported type %s", owner, fname, goType)
		}
		return g.numericCodec(owner, fname, jname, goType, name, false), nil
	case *types.Struct:
		g.addImport(pkg.Path(), pkg.Name())
		dec, enc := g.decoderEncoder(t)
		if dec && enc {
			return fmt.Sprintf("qijson.ObjectDecoder[%s, %s](%q, func(v *%s) *%s { return &v.%s })",
				owner, goType, jname, owner, goType, fname), nil
		}
		if dec != enc {
			return "", fmt.Errorf("%s.%s: cross-package type %s implements only one of DecodeJSON/EncodeJSON; both are required for a symmetric binding", owner, fname, goType)
		}
		g.addForeign(t)
		return fmt.Sprintf("qijson.Object[%s, %s](%q, func(v *%s) *%s { return &v.%s }, %s)",
			owner, goType, jname, owner, goType, fname, foreignSchemaVar(goType)), nil
	case *types.Interface:
		if u.NumMethods() == 0 {
			return g.anyCodec(owner, fname, jname), nil
		}
	}
	return "", fmt.Errorf("%s.%s: unsupported type %s", owner, fname, goType)
}

// foreignStarCodec builds a codec for a pointer field of a foreign struct.
func (g *gen) foreignStarCodec(owner, fname, jname string, t *types.Pointer) (string, error) {
	switch et := t.Elem().(type) {
	case *types.Basic:
		switch et.Kind() {
		case types.String:
			return ctor("StringPtr", owner, jname, fname, "**string"), nil
		case types.Bool:
			return ctor("BoolPtr", owner, jname, fname, "**bool"), nil
		case types.Int64:
			return ctor("IntPtr", owner, jname, fname, "**int64"), nil
		case types.Float64:
			return ctor("FloatPtr", owner, jname, fname, "**float64"), nil
		}
		if name, ok := basicKindName(et.Kind()); ok {
			return g.numericCodec(owner, fname, jname, name, name, true), nil
		}
	case *types.Named:
		if goType, ident, ok := g.override(et); ok {
			return g.schemaPtrCodec(owner, fname, jname, goType, ident), nil
		}
		obj := et.Obj()
		pkg := obj.Pkg()
		if pkg == nil || !obj.Exported() {
			return "", fmt.Errorf("%s.%s: unsupported pointer type *%s", owner, fname, obj.Name())
		}
		goType := pkg.Name() + "." + obj.Name()
		switch u := et.Underlying().(type) {
		case *types.Basic:
			g.addImport(pkg.Path(), pkg.Name())
			switch u.Kind() {
			case types.String:
				return g.scalarClosure(owner, fname, jname, goType, "string", true), nil
			case types.Bool:
				return g.scalarClosure(owner, fname, jname, goType, "bool", true), nil
			}
			if name, ok := basicKindName(u.Kind()); ok {
				return g.numericCodec(owner, fname, jname, goType, name, true), nil
			}
		case *types.Struct:
			g.addImport(pkg.Path(), pkg.Name())
			dec, enc := g.decoderEncoder(et)
			if dec && enc {
				return fmt.Sprintf("qijson.ObjectDecoderPtr[%s, %s](%q, func(v *%s) **%s { return &v.%s })",
					owner, goType, jname, owner, goType, fname), nil
			}
			if dec != enc {
				return "", fmt.Errorf("%s.%s: cross-package type %s implements only one of DecodeJSON/EncodeJSON; both are required for a symmetric binding", owner, fname, goType)
			}
			g.addForeign(et)
			return g.schemaPtrCodec(owner, fname, jname, goType, foreignSchemaVar(goType)), nil
		}
	}
	return "", fmt.Errorf("%s.%s: unsupported pointer type *%s", owner, fname, t.Elem().String())
}

// schemaPtrCodec binds a pointer to a foreign struct through a schema: a JSON
// null sets it nil, otherwise a fresh value is decoded through schema, and a nil
// pointer encodes as null. It is the A and D counterpart to ObjectDecoderPtr.
func (g *gen) schemaPtrCodec(owner, fname, jname, goType, schema string) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "qijson.Field[%s]{Name: %q, Decode: func(r *qijson.Reader, v *%s) error {\n", owner, jname, owner)
	b.WriteString("\tk, err := r.Peek()\n\tif err != nil {\n\t\treturn err\n\t}\n")
	b.WriteString("\tif k == qijson.KindNull {\n\t\tif err := r.ReadNull(); err != nil {\n\t\t\treturn err\n\t\t}\n")
	fmt.Fprintf(&b, "\t\tv.%s = nil\n\t\treturn nil\n\t}\n", fname)
	fmt.Fprintf(&b, "\te := &%s{}\n\tif err := %s.Decode(r, e); err != nil {\n\t\treturn err\n\t}\n", goType, schema)
	fmt.Fprintf(&b, "\tv.%s = e\n\treturn nil\n\t},\n", fname)
	fmt.Fprintf(&b, "\tEncode: func(w *qijson.Writer, v *%s) error {\n", owner)
	fmt.Fprintf(&b, "\t\tif v.%s == nil {\n\t\t\treturn w.WriteNull()\n\t\t}\n", fname)
	fmt.Fprintf(&b, "\t\treturn %s.Encode(w, v.%s)\n\t},\n", schema, fname)
	if g.omitEmpty {
		fmt.Fprintf(&b, "\tEmpty: func(v *%s) bool { return v.%s == nil },\n", owner, fname)
	}
	b.WriteString("}")
	return b.String()
}

// foreignSliceCodec builds a codec for a slice field of a foreign struct.
func (g *gen) foreignSliceCodec(owner, fname, jname string, t *types.Slice) (string, error) {
	if b, ok := t.Elem().(*types.Basic); ok && b.Kind() == types.Uint8 {
		return ctor("Raw", owner, jname, fname, "*[]byte"), nil
	}
	elem, el, err := g.foreignElem(t.Elem())
	if err != nil {
		return "", fmt.Errorf("%s.%s: %w", owner, fname, err)
	}
	return fmt.Sprintf("qijson.Slice[%s, %s](%q, func(v *%s) *[]%s { return &v.%s }, %s)",
		owner, elem, jname, owner, elem, fname, el), nil
}

// foreignArrayCodec builds a codec for a fixed-size array field.
func (g *gen) foreignArrayCodec(owner, fname, jname string, t *types.Array) (string, error) {
	elem, el, err := g.foreignElem(t.Elem())
	if err != nil {
		return "", fmt.Errorf("%s.%s: %w", owner, fname, err)
	}
	return fmt.Sprintf("qijson.Array[%s, %s](%q, func(v *%s) []%s { return v.%s[:] }, %s)",
		owner, elem, jname, owner, elem, fname, el), nil
}

// foreignMapCodec builds a codec for a map field with string keys.
func (g *gen) foreignMapCodec(owner, fname, jname string, t *types.Map) (string, error) {
	if k, ok := t.Key().(*types.Basic); !ok || k.Kind() != types.String {
		return "", fmt.Errorf("%s.%s: map key %s is not supported; only string keys are", owner, fname, t.Key().String())
	}
	val, el, err := g.foreignElem(t.Elem())
	if err != nil {
		return "", fmt.Errorf("%s.%s: %w", owner, fname, err)
	}
	return fmt.Sprintf("qijson.Map[%s, %s](%q, func(v *%s) *map[string]%s { return &v.%s }, %s)",
		owner, val, jname, owner, val, fname, el), nil
}

// foreignElem returns the Go type name and the qijson.Codec expression for a
// foreign container element type.
func (g *gen) foreignElem(t types.Type) (name, codec string, err error) {
	switch tt := t.(type) {
	case *types.Basic:
		switch tt.Kind() {
		case types.String:
			return "string", "qijson.NewCodec((*qijson.Reader).ReadString, (*qijson.Writer).WriteString)", nil
		case types.Bool:
			return "bool", "qijson.NewCodec((*qijson.Reader).ReadBool, (*qijson.Writer).WriteBool)", nil
		case types.Int64:
			return "int64", "qijson.NewCodec((*qijson.Reader).ReadInt, (*qijson.Writer).WriteInt)", nil
		case types.Float64:
			return "float64", "qijson.NewCodec((*qijson.Reader).ReadFloat, (*qijson.Writer).WriteFloat)", nil
		}
		if bn, ok := basicKindName(tt.Kind()); ok {
			return bn, fmt.Sprintf("qijson.NewCodec(%s, %s)", g.numericHelper(bn, bn), g.numericEncodeHelper(bn, bn)), nil
		}
		return "", "", fmt.Errorf("unsupported element type %s", tt.String())
	case *types.Named:
		if goType, ident, ok := g.override(tt); ok {
			return goType, fmt.Sprintf("qijson.NewCodec(%s, %s)", g.schemaStructHelper(goType, ident), g.schemaStructEncodeHelper(goType, ident)), nil
		}
		obj := tt.Obj()
		pkg := obj.Pkg()
		if pkg == nil || !obj.Exported() {
			return "", "", fmt.Errorf("unsupported element type %s", obj.Name())
		}
		goType := pkg.Name() + "." + obj.Name()
		switch u := tt.Underlying().(type) {
		case *types.Basic:
			g.addImport(pkg.Path(), pkg.Name())
			switch u.Kind() {
			case types.String:
				return goType, fmt.Sprintf("qijson.NewCodec(%s, %s)", g.scalarHelper(goType, "string"), g.scalarEncodeHelper(goType, "string")), nil
			case types.Bool:
				return goType, fmt.Sprintf("qijson.NewCodec(%s, %s)", g.scalarHelper(goType, "bool"), g.scalarEncodeHelper(goType, "bool")), nil
			}
			bn, ok := basicKindName(u.Kind())
			if !ok {
				return "", "", fmt.Errorf("unsupported element type %s", goType)
			}
			return goType, fmt.Sprintf("qijson.NewCodec(%s, %s)", g.numericHelper(goType, bn), g.numericEncodeHelper(goType, bn)), nil
		case *types.Struct:
			g.addImport(pkg.Path(), pkg.Name())
			dec, enc := g.decoderEncoder(tt)
			if dec && enc {
				return goType, fmt.Sprintf("qijson.DecoderCodec[%s, *%s]()", goType, goType), nil
			}
			if dec != enc {
				return "", "", fmt.Errorf("cross-package element type %s implements only one of DecodeJSON/EncodeJSON; both are required for a symmetric binding", goType)
			}
			g.addForeign(tt)
			return goType, fmt.Sprintf("qijson.NewCodec(%s, %s)", g.foreignStructHelper(goType), g.foreignStructEncodeHelper(goType)), nil
		}
	}
	return "", "", fmt.Errorf("unsupported element type %s", t.String())
}

// foreignStructHelper emits, once, a function that decodes a plain foreign
// struct through its synthesized schema.
func (g *gen) foreignStructHelper(goType string) string {
	return g.schemaStructHelper(goType, foreignSchemaVar(goType))
}

// foreignStructEncodeHelper emits, once, a function that encodes a plain foreign
// struct through its synthesized schema.
func (g *gen) foreignStructEncodeHelper(goType string) string {
	return g.schemaStructEncodeHelper(goType, foreignSchemaVar(goType))
}

// schemaStructHelper emits, once, a function that decodes goType through the
// named schema var or identifier and returns the helper's name.
func (g *gen) schemaStructHelper(goType, schema string) string {
	hn := "qiDecode" + crossHelperSuffix(goType)
	if _, ok := g.helpers[hn]; ok {
		return hn
	}
	g.helpers[hn] = fmt.Sprintf(
		"func %s(r *qijson.Reader) (%s, error) {\n\tvar v %s\n\tif err := %s.Decode(r, &v); err != nil {\n\t\treturn v, err\n\t}\n\treturn v, nil\n}\n",
		hn, goType, goType, schema)
	g.helperOrder = append(g.helperOrder, hn)
	return hn
}

// schemaStructEncodeHelper emits, once, a function that encodes goType through
// the named schema var or identifier and returns the helper's name.
func (g *gen) schemaStructEncodeHelper(goType, schema string) string {
	hn := "qiEncode" + crossHelperSuffix(goType)
	if _, ok := g.helpers[hn]; ok {
		return hn
	}
	g.helpers[hn] = fmt.Sprintf(
		"func %s(w *qijson.Writer, v %s) error {\n\treturn %s.Encode(w, &v)\n}\n",
		hn, goType, schema)
	g.helperOrder = append(g.helperOrder, hn)
	return hn
}

// foreignGoType returns the qualified name of a named cross-package type that
// can be denoted from the consumer package.
func foreignGoType(named *types.Named) (string, bool) {
	obj := named.Obj()
	pkg := obj.Pkg()
	if pkg == nil || !obj.Exported() {
		return "", false
	}
	return pkg.Name() + "." + obj.Name(), true
}

// override returns the caller-supplied schema identifier for named, from the
// -foreign flag, marking the entry used, or ok=false when there is none.
func (g *gen) override(named *types.Named) (goType, ident string, ok bool) {
	if len(g.overrides) == 0 {
		return "", "", false
	}
	goType, ok = foreignGoType(named)
	if !ok {
		return "", "", false
	}
	ident, ok = g.overrides[goType]
	if !ok {
		return "", "", false
	}
	g.overrideUsed[goType] = true
	obj := named.Obj()
	g.addImport(obj.Pkg().Path(), obj.Pkg().Name())
	return goType, ident, true
}

// overrideField binds a field of a -foreign type to the caller-supplied schema,
// returning handled=false when the type has no override.
func (g *gen) overrideField(owner, fname, jname string, named *types.Named, ptr bool) (string, bool) {
	goType, ident, ok := g.override(named)
	if !ok {
		return "", false
	}
	if ptr {
		return g.schemaPtrCodec(owner, fname, jname, goType, ident), true
	}
	return fmt.Sprintf("qijson.Object[%s, %s](%q, func(v *%s) *%s { return &v.%s }, %s)",
		owner, goType, jname, owner, goType, fname, ident), true
}

// overrideElem binds a container element of a -foreign type to a codec built
// from the caller-supplied schema, returning handled=false when there is none.
func (g *gen) overrideElem(named *types.Named) (string, bool) {
	goType, ident, ok := g.override(named)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("qijson.NewCodec(%s, %s)", g.schemaStructHelper(goType, ident), g.schemaStructEncodeHelper(goType, ident)), true
}

// crossElemStruct returns a qijson.Codec expression for a plain cross-package
// struct element type, using the synthesized schema (approach A).
func (g *gen) crossElemStruct(named *types.Named) (string, error) {
	obj := named.Obj()
	pkg := obj.Pkg()
	if pkg == nil {
		return "", fmt.Errorf("unsupported element type %s", obj.Name())
	}
	goType := pkg.Name() + "." + obj.Name()
	g.addForeign(named)
	g.addImport(pkg.Path(), pkg.Name())
	return fmt.Sprintf("qijson.NewCodec(%s, %s)", g.foreignStructHelper(goType), g.foreignStructEncodeHelper(goType)), nil
}
