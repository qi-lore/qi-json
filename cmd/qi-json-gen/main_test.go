package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunGeneratesFile checks a successful run writes the generated source to the
// default output path and reports nothing.
func TestRunGeneratesFile(t *testing.T) {
	dir := writePkg(t, `package example

type Config struct {
	Name string
}
`)
	var out, errOut bytes.Buffer
	if err := run([]string{"-type=Config", dir}, &out, &errOut); err != nil {
		t.Fatalf("run: %v (stderr %q)", err, errOut.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, defaultOutput))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "var configSchema = qijson.Struct[Config](") {
		t.Errorf("generated file missing the schema var:\n%s", data)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
}

// TestRunOutputFile checks -o writes to the named file.
func TestRunOutputFile(t *testing.T) {
	dir := writePkg(t, `package example

type Config struct {
	Name string
}
`)
	target := filepath.Join(dir, "custom_gen.go")
	var out, errOut bytes.Buffer
	if err := run([]string{"-type=Config", "-o", target, dir}, &out, &errOut); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("output file not written: %v", err)
	}
}

// TestRunUsageErrors drives the usage-error branches, which must return errUsage
// and report to stderr.
func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"too many dirs", []string{".", "."}, "at most one directory argument"},
		{"bad foreign", []string{"-foreign=broken", "."}, "-foreign \"broken\""},
		{"unknown flag", []string{"-nope"}, "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			err := run(tt.args, &out, &errOut)
			if !errors.Is(err, errUsage) {
				t.Fatalf("run error = %v, want errUsage", err)
			}
			if !strings.Contains(errOut.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", errOut.String(), tt.wantErr)
			}
		})
	}
}

// TestRunGenerationErrors drives the generation-failure branch, which returns a
// plain error (exit status 1) and reports to stderr.
func TestRunGenerationErrors(t *testing.T) {
	dir := writePkg(t, `package example

type Config struct {
	Name string
}
`)
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown type", []string{"-type=Missing", dir}, "type Missing not found"},
		{"unused override", []string{"-type=Config", "-foreign=xtypes.Thing=x", dir}, "not used"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			err := run(tt.args, &out, &errOut)
			if err == nil || errors.Is(err, errUsage) {
				t.Fatalf("run error = %v, want a plain generation error", err)
			}
			if !strings.Contains(errOut.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", errOut.String(), tt.wantErr)
			}
		})
	}
}

// TestRunWriteError checks a write failure (an output path whose parent does not
// exist) is reported and returned as a plain error.
func TestRunWriteError(t *testing.T) {
	dir := writePkg(t, `package example

type Config struct {
	Name string
}
`)
	bad := filepath.Join(dir, "no-such-dir", "out.go")
	var out, errOut bytes.Buffer
	err := run([]string{"-type=Config", "-o", bad, dir}, &out, &errOut)
	if err == nil || errors.Is(err, errUsage) {
		t.Fatalf("run error = %v, want a plain write error", err)
	}
	if !strings.Contains(errOut.String(), "qi-json-gen:") {
		t.Errorf("stderr = %q, want the qi-json-gen prefix", errOut.String())
	}
}
