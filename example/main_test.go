package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRoundTrip checks that the example's decode-then-encode helper produces the
// expected field values and a stable, canonical re-encoded form.
func TestRoundTrip(t *testing.T) {
	const doc = `{"name":"Ada Lovelace","age":36,"verified":true,"rating":4.5}`

	user, out, err := roundTrip([]byte(doc))
	if err != nil {
		t.Fatalf("roundTrip() error = %v, want nil", err)
	}

	want := User{Name: "Ada Lovelace", Age: 36, Verified: true, Rating: 4.5}
	if user != want {
		t.Errorf("decoded = %+v, want %+v", user, want)
	}

	const wantJSON = `{"name":"Ada Lovelace","age":36,"verified":true,"rating":4.5}`
	if got := string(out); got != wantJSON {
		t.Errorf("encoded = %s, want %s", got, wantJSON)
	}
}

// TestRoundTripInvalid checks that a syntactically invalid document is rejected
// by the syntax check before decoding is attempted.
func TestRoundTripInvalid(t *testing.T) {
	if _, _, err := roundTrip([]byte(`{"name":}`)); err == nil {
		t.Fatal("roundTrip() error = nil, want non-nil for invalid JSON")
	}
}

// TestRoundTripTypeError checks that a syntactically valid but type-mismatched
// document is rejected by the decoder.
func TestRoundTripTypeError(t *testing.T) {
	if _, _, err := roundTrip([]byte(`{"name":1}`)); err == nil {
		t.Fatal("roundTrip() error = nil, want a decode type error")
	}
}

// TestRun checks the example's shell prints the decoded value and the canonical
// JSON to stdout and reports nothing on stderr.
func TestRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(exampleDoc, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "decoded:  name=Ada Lovelace age=36 verified=true rating=4.5") {
		t.Errorf("stdout missing the decoded line:\n%s", got)
	}
	if !strings.Contains(got, `encoded:  {"name":"Ada Lovelace","age":36,"verified":true,"rating":4.5}`) {
		t.Errorf("stdout missing the encoded line:\n%s", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

// TestRunError checks the shell reports a bad document to stderr and returns the
// error rather than printing a result.
func TestRunError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(`{"name":}`, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want an error")
	}
	if !strings.Contains(stderr.String(), "example:") {
		t.Errorf("stderr = %q, want the example: prefix", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty on failure", stdout.String())
	}
}

// TestMainSuccess checks main prints the round-trip result and does not exit.
func TestMainSuccess(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1
	defer swapStreams(&out, &errBuf, func(code int) { exitCode = code })()

	main()

	if exitCode != -1 {
		t.Errorf("main exit code = %d, want no exit", exitCode)
	}
	if !strings.Contains(out.String(), "decoded:") {
		t.Errorf("stdout missing the decoded line:\n%s", out.String())
	}
}

// TestMainFailure checks main maps a bad document to a non-zero exit status and
// reports the failure to stderr.
func TestMainFailure(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := 0
	defer swapStreams(&out, &errBuf, func(code int) { exitCode = code })()

	bad := exampleDoc
	exampleDoc = `{"name":}`
	defer func() { exampleDoc = bad }()

	main()

	if exitCode != 1 {
		t.Errorf("main exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(errBuf.String(), "example:") {
		t.Errorf("stderr = %q, want the example: prefix", errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty on failure", out.String())
	}
}

// swapStreams points the process streams and exit hook at the test's buffers and
// returns a function that restores the originals.
func swapStreams(out, errBuf *bytes.Buffer, exit func(int)) func() {
	oldOut, oldErr, oldExit := stdout, stderr, osExit
	stdout, stderr, osExit = out, errBuf, exit
	return func() { stdout, stderr, osExit = oldOut, oldErr, oldExit }
}
