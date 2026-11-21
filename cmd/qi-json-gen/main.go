// qi-json-gen generates qijson.Schema values and DecodeJSON methods from
// struct tags, so callers need not hand-write the compile-time accessor closures
// that qijson's reflection-free decoder uses.
//
// Usage:
//
//	qi-json-gen -type=Config [-type=Other] [-foreign=pkg.T=ident] [dir]
//	qi-json-gen [dir]                 // generate for every type marked //qi-json:generate
//
// With -type the named types are generated; the flag may be repeated or hold a
// comma-separated list. Without -type the tool scans the package for type
// declarations carrying a //qi-json:generate comment. dir defaults to "." and
// must hold exactly one non-test package. The output file defaults to
// <dir>/zz_qi_json_gen.go. With -strict every generated schema rejects unknown
// and duplicate object members. With -foreign=pkg.T=ident a field or element of
// the foreign type pkg.T binds to the Schema named ident, which the consumer
// owns; the flag is repeatable.
//
// The generated code targets only the exported qijson surface; it is ordinary
// source in the caller's package. The tool is stdlib-only and uses no reflect or
// unsafe.
//
// A field whose type is a same-package named type with a basic scalar
// underlying (string, bool, or a numeric) is resolved from the AST. A field
// whose type is a named type from another package is resolved with the standard
// library's type checker: a basic scalar underlying (for example time.Duration,
// or a named string or bool) converts through a shared closure; a struct that
// implements qijson.Decoder and qijson.Encoder is bound by delegation; a plain
// struct has a schema synthesized from its exported fields. A -foreign entry
// overrides all of these for the named type.
package main

import (
	"flag"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// stringList collects a repeatable, comma-separated flag.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

func main() {
	var types stringList
	var foreign stringList
	var out string
	var strict bool
	flag.Var(&types, "type", "type name to generate for; repeatable or comma-separated")
	flag.Var(&foreign, "foreign", "pkg.T=ident: bind a foreign struct to a caller-supplied Schema; repeatable")
	flag.StringVar(&out, "o", "", "output file (default <dir>/"+defaultOutput+")")
	flag.BoolVar(&strict, "strict", false, "generated schemas reject unknown and duplicate members")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: qi-json-gen [-type=T[,T...]] [-foreign=pkg.T=ident] [-strict] [-o file] [dir]")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "qi-json-gen: at most one directory argument")
		os.Exit(2)
	}
	dir := "."
	if flag.NArg() == 1 {
		dir = flag.Arg(0)
	}

	overrides, err := parseForeign(foreign)
	if err != nil {
		fmt.Fprintf(os.Stderr, "qi-json-gen: %v\n", err)
		os.Exit(2)
	}

	src, err := generateOptions(dir, types, strict, overrides)
	if err != nil {
		fmt.Fprintf(os.Stderr, "qi-json-gen: %v\n", err)
		os.Exit(1)
	}
	if out == "" {
		out = filepath.Join(dir, defaultOutput)
	}
	if err := os.WriteFile(out, src, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "qi-json-gen: %v\n", err)
		os.Exit(1)
	}
}

// parseForeign builds the override map from -foreign entries of the form
// pkg.T=ident. A malformed entry is a precise error.
func parseForeign(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		left, ident, ok := strings.Cut(e, "=")
		if !ok {
			return nil, fmt.Errorf("-foreign %q: want pkg.T=ident", e)
		}
		pkg, typ, ok := strings.Cut(left, ".")
		if !ok || !token.IsIdentifier(pkg) || !token.IsIdentifier(typ) {
			return nil, fmt.Errorf("-foreign %q: want pkg.T=ident with a type pkg.T", e)
		}
		if !token.IsIdentifier(ident) {
			return nil, fmt.Errorf("-foreign %q: %q is not a valid identifier", e, ident)
		}
		key := pkg + "." + typ
		if _, dup := m[key]; dup {
			return nil, fmt.Errorf("-foreign %q: %s is given more than once", e, key)
		}
		m[key] = ident
	}
	return m, nil
}
