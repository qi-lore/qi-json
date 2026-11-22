package qijson

import (
	"os"
	"strings"
	"testing"
)

// TestGoModGoDirectiveFloor pins the module's declared Go language floor to the
// version the documentation promises. Both docs/constitution.md (Additional
// Constraints) and docs/task.md §3 state a Go >= 1.24 floor: the root package is
// a facade of generic type aliases (Schema, Field, Codec), and generic type
// aliases are a Go 1.24 language feature. Since Go 1.21 a dependency's go
// directive is a hard minimum, so a go directive above the documented floor
// silently upgrades or hard-blocks a consumer on that floor (see
// .opencode/plan/2026-11-22-go-version-floor.md and
// .opencode/plan/2026-11-22-internal-codec-extraction.md). Update this test only
// when the documented floor deliberately moves.
func TestGoModGoDirectiveFloor(t *testing.T) {
	const (
		wantGo        = "1.24.0"
		wantToolchain = "go1.27.3"
	)

	directives := readGoModDirectives(t)

	if got := directives["go"]; got != wantGo {
		t.Errorf("go.mod go directive = %q, want %q (the documented floor: docs/constitution.md, docs/task.md); "+
			"raising it makes the enforced floor stricter than the stated one", got, wantGo)
	}
	if got := directives["toolchain"]; got != wantToolchain {
		t.Errorf("go.mod toolchain directive = %q, want %q", got, wantToolchain)
	}
}

// readGoModDirectives returns the single-line directives of the module's go.mod,
// keyed by directive name (for example "go", "toolchain", "module"). It is a
// stdlib-only, minimal reader: directives in this scaffold are single-line
// `name value` pairs, comments start at `//`, and any block form (`require (`)
// is not inspected.
func readGoModDirectives(t *testing.T) map[string]string {
	t.Helper()

	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	directives := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		directives[fields[0]] = fields[1]
	}
	return directives
}
