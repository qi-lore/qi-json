package codec

import "testing"

// TestWriterBuildBytesFresh proves BuildBytes with no reuse buffer returns the
// document in a fresh slice that does not alias the Writer's internal buffer, so
// the caller owns the result and the Writer stays usable.
func TestWriterBuildBytesFresh(t *testing.T) {
	var w Writer
	if err := w.WriteString("hi"); err != nil {
		t.Fatal(err)
	}

	got := w.BuildBytes()
	if want := `"hi"`; string(got) != want {
		t.Fatalf("BuildBytes = %q, want %q", got, want)
	}

	// Mutating the result must not disturb the Writer: BuildBytes is a copy.
	if len(got) > 0 {
		got[0] = 'X'
	}
	if want := `"hi"`; string(w.Bytes()) != want {
		t.Errorf("Bytes = %q, want %q: result aliases the writer buffer", w.Bytes(), want)
	}
}

// TestWriterBuildBytesReuse proves BuildBytes writes into the caller-supplied
// buffer, appending the document into its capacity instead of allocating a new
// slice.
func TestWriterBuildBytesReuse(t *testing.T) {
	var w Writer
	if err := w.WriteInt(12345); err != nil {
		t.Fatal(err)
	}

	// A capacity the append growth of a five-byte document would never reach, so
	// observing it back proves the buffer was reused.
	const reuseCap = 4096
	buf := make([]byte, 0, reuseCap)

	got := w.BuildBytes(buf)
	if want := "12345"; string(got) != want {
		t.Fatalf("BuildBytes = %q, want %q", got, want)
	}
	if cap(got) != reuseCap {
		t.Errorf("cap = %d, want the reused %d: buffer not reused", cap(got), reuseCap)
	}
}

// TestWriterBuildBytesAppends proves BuildBytes appends the document after
// whatever the caller-supplied buffer already holds, the Go append contract that
// "reuse its capacity" describes.
func TestWriterBuildBytesAppends(t *testing.T) {
	var w Writer
	if err := w.WriteInt(7); err != nil {
		t.Fatal(err)
	}

	got := w.BuildBytes([]byte("prefix:"))
	if want := "prefix:7"; string(got) != want {
		t.Fatalf("BuildBytes = %q, want %q", got, want)
	}
}

// TestWriterBuildBytesError proves BuildBytes reports a failed Writer the same
// way Bytes does: it returns nil instead of a partial or empty document.
func TestWriterBuildBytesError(t *testing.T) {
	var w Writer
	if err := w.EndObject(); err == nil {
		t.Fatal("EndObject: want an error")
	}

	if got := w.BuildBytes(); got != nil {
		t.Errorf("BuildBytes = %q, want nil after an error", got)
	}
	if got := w.BuildBytes(make([]byte, 0, 64)); got != nil {
		t.Errorf("BuildBytes(reuse) = %q, want nil after an error", got)
	}
}

// TestWriterBuildBytesKeepsWriterReusable proves taking the document with
// BuildBytes leaves the Writer ready for the next value after a Reset, the
// lifecycle BuildBytes exists to support.
func TestWriterBuildBytesKeepsWriterReusable(t *testing.T) {
	var w Writer
	if err := w.WriteInt(1); err != nil {
		t.Fatal(err)
	}
	if got := string(w.BuildBytes()); got != "1" {
		t.Fatalf("first BuildBytes = %q, want 1", got)
	}

	w.Reset()
	if err := w.WriteInt(2); err != nil {
		t.Fatalf("WriteInt after Reset: %v", err)
	}
	if got := string(w.BuildBytes()); got != "2" {
		t.Errorf("second BuildBytes = %q, want 2", got)
	}
}
