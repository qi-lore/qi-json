package codec

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// failingWriter is an io.Writer that always fails, used to prove DumpTo
// propagates a write error and leaves the Writer's buffer intact.
type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

// TestWriterDumpTo proves DumpTo writes the document to the target, returns the
// byte count, and resets the Writer so it is immediately reusable.
func TestWriterDumpTo(t *testing.T) {
	var w Writer
	if err := w.WriteInt(12345); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	n, err := w.DumpTo(&out)
	if err != nil {
		t.Fatalf("DumpTo: %v", err)
	}
	if want := "12345"; out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
	if want := 5; n != want {
		t.Errorf("n = %d, want %d", n, want)
	}
	if got := w.Bytes(); len(got) != 0 {
		t.Errorf("Bytes after DumpTo = %q, want empty: DumpTo must reset the Writer", got)
	}
}

// TestWriterDumpToStreamsValues proves DumpTo resets the Writer so a caller can
// build and dump value after value into one stream.
func TestWriterDumpToStreamsValues(t *testing.T) {
	var w Writer
	var out bytes.Buffer
	for i := 0; i < 3; i++ {
		if err := w.BeginObject(); err != nil {
			t.Fatal(err)
		}
		if err := w.Key("i"); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteInt(int64(i)); err != nil {
			t.Fatal(err)
		}
		if err := w.EndObject(); err != nil {
			t.Fatal(err)
		}
		if _, err := w.DumpTo(&out); err != nil {
			t.Fatalf("DumpTo(%d): %v", i, err)
		}
	}
	if want := `{"i":0}{"i":1}{"i":2}`; out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}

// TestWriterDumpToFailedWriter proves DumpTo reports a Writer's sticky error and
// writes nothing, matching Bytes/BuildBytes.
func TestWriterDumpToFailedWriter(t *testing.T) {
	var w Writer
	if err := w.EndObject(); err == nil {
		t.Fatal("EndObject: want an error")
	}

	var out bytes.Buffer
	n, err := w.DumpTo(&out)
	if !errors.Is(err, errEndWithoutBegin) {
		t.Errorf("err = %v, want errEndWithoutBegin", err)
	}
	if n != 0 || out.Len() != 0 {
		t.Errorf("DumpTo wrote %d bytes (n=%d), want nothing on a failed Writer", out.Len(), n)
	}
}

// TestWriterDumpToWriteError proves DumpTo propagates the target's write error
// and does not reset the Writer, so the document is not silently dropped.
func TestWriterDumpToWriteError(t *testing.T) {
	var w Writer
	if err := w.WriteInt(7); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("boom")
	n, err := w.DumpTo(failingWriter{err: boom})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	if got := string(w.Bytes()); got != "7" {
		t.Errorf("Bytes = %q, want 7: DumpTo must not reset after a write error", got)
	}
}

// TestWriterDumpToNilTarget proves DumpTo reports a nil target instead of
// panicking on the Write call.
func TestWriterDumpToNilTarget(t *testing.T) {
	var w Writer
	if err := w.WriteInt(1); err != nil {
		t.Fatal(err)
	}
	if _, err := w.DumpTo(nil); !errors.Is(err, errNilDumpTarget) {
		t.Errorf("err = %v, want errNilDumpTarget", err)
	}
}

// TestWriterReadCloser proves ReadCloser reads the document and resets the
// Writer; Close releases it, and a closed reader reports io.EOF.
func TestWriterReadCloser(t *testing.T) {
	var w Writer
	if err := w.WriteString("hi"); err != nil {
		t.Fatal(err)
	}

	rc, err := w.ReadCloser()
	if err != nil {
		t.Fatalf("ReadCloser: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if want := `"hi"`; string(got) != want {
		t.Errorf("read = %q, want %q", got, want)
	}
	if err := rc.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if n, err := rc.Read(make([]byte, 8)); n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("Read after Close = (%d, %v), want (0, io.EOF)", n, err)
	}
	if got := w.Bytes(); len(got) != 0 {
		t.Errorf("Bytes after ReadCloser = %q, want empty: ReadCloser must reset the Writer", got)
	}
}

// TestWriterReadCloserCloseIdempotent proves Close can be called more than once
// and always returns nil.
func TestWriterReadCloserCloseIdempotent(t *testing.T) {
	var w Writer
	if err := w.WriteInt(1); err != nil {
		t.Fatal(err)
	}
	rc, err := w.ReadCloser()
	if err != nil {
		t.Fatalf("ReadCloser: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Errorf("second Close: %v, want nil", err)
	}
}

// TestWriterReadCloserResetsWriter proves ReadCloser leaves the Writer ready to
// build the next value, the lifecycle DumpTo also has.
func TestWriterReadCloserResetsWriter(t *testing.T) {
	var w Writer
	if err := w.WriteInt(1); err != nil {
		t.Fatal(err)
	}
	rc, err := w.ReadCloser()
	if err != nil {
		t.Fatalf("ReadCloser: %v", err)
	}
	if first, err := io.ReadAll(rc); err != nil || string(first) != "1" {
		t.Fatalf("first read = (%q, %v), want (1, nil)", first, err)
	}

	// The Writer is reusable immediately: build and read a second value.
	if err := w.WriteInt(2); err != nil {
		t.Fatalf("second write: %v", err)
	}
	rc2, err := w.ReadCloser()
	if err != nil {
		t.Fatalf("second ReadCloser: %v", err)
	}
	if second, err := io.ReadAll(rc2); err != nil || string(second) != "2" {
		t.Errorf("second read = (%q, %v), want (2, nil)", second, err)
	}
}

// TestWriterReadCloserIndependentOfWriter proves the reader does not alias the
// Writer: a later write to the Writer must not corrupt the reader's data.
func TestWriterReadCloserIndependentOfWriter(t *testing.T) {
	var w Writer
	if err := w.WriteString("first"); err != nil {
		t.Fatal(err)
	}
	rc, err := w.ReadCloser()
	if err != nil {
		t.Fatalf("ReadCloser: %v", err)
	}

	if err := w.WriteString("second"); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if want := `"first"`; string(got) != want {
		t.Errorf("read = %q, want %q: the reader aliases the Writer", got, want)
	}
	if got := string(w.Bytes()); got != `"second"` {
		t.Errorf("Bytes = %q, want %q", got, `"second"`)
	}
}

// TestWriterReadCloserFailedWriter proves ReadCloser reports the Writer's sticky
// error instead of returning a reader.
func TestWriterReadCloserFailedWriter(t *testing.T) {
	var w Writer
	if err := w.EndObject(); err == nil {
		t.Fatal("EndObject: want an error")
	}

	rc, err := w.ReadCloser()
	if !errors.Is(err, errEndWithoutBegin) {
		t.Errorf("err = %v, want errEndWithoutBegin", err)
	}
	if rc != nil {
		t.Errorf("ReadCloser = %v, want nil on a failed Writer", rc)
	}
}
