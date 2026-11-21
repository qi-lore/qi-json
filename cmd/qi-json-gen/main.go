// qi-json-gen generates marshal.Schema values and DecodeJSON methods from
// struct tags, so callers need not hand-write the compile-time accessor closures
// that marshal's reflection-free decoder uses.
//
// Usage:
//
//	qi-json-gen -type=Config [-type=Other] [dir]
//	qi-json-gen [dir]                 // generate for every type marked //qi-json:generate
//
// With -type the named types are generated; the flag may be repeated or hold a
// comma-separated list. Without -type the tool scans the package for type
// declarations carrying a //qi-json:generate comment. dir defaults to "." and
// must hold exactly one non-test package. The output file defaults to
// <dir>/zz_qi_json_gen.go.
//
// The generated code targets only the exported marshal surface; it is ordinary
// source in the caller's package. The tool is stdlib-only and uses no reflect or
// unsafe.
//
// A field whose type is a named type from another package is resolved with the
// standard library's type checker when its underlying type is a basic scalar
// (for example time.Duration, or a named string or bool); a cross-package type
// with a non-scalar underlying type is reported as an error.
package main

import (
	"flag"
	"fmt"
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
	var out string
	flag.Var(&types, "type", "type name to generate for; repeatable or comma-separated")
	flag.StringVar(&out, "o", "", "output file (default <dir>/"+defaultOutput+")")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: qi-json-gen [-type=T[,T...]] [-o file] [dir]")
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

	src, err := generate(dir, types)
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
