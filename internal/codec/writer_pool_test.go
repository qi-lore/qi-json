package codec

import (
	"strings"
	"testing"
)

// The writer buffer pool is internal: it backs a Writer's byte slice so a
// short-lived encode can reuse a warmed allocation without adding any exported
// name. These tests drive the unexported helpers directly.
//
// sync.Pool is free to drop a stored item and to return a different one, and the
// race detector drops one in four on purpose, so a reuse assertion offers a
// buffer and looks for its capacity back rather than assuming a single Put is
// followed by that same Get.

// wantedCap is a capacity the pool's New never produces, so observing it proves
// reuse rather than a fresh allocation.
const wantedCap = 3 * pooledBufCap

// sawWanted reports whether the pool eventually hands back a buffer of at least
// wantedCap, retrying because a stored item may be dropped.
func sawWanted() bool {
	for i := 0; i < 64; i++ {
		releaseBuf(make([]byte, wantedCap)[:0])
		if cap(acquireBuf()) >= wantedCap {
			return true
		}
	}
	return false
}

func TestWriterPoolReusesBuffer(t *testing.T) {
	if cap(acquireBuf()) == 0 {
		t.Fatal("acquireBuf returned a zero-capacity buffer")
	}
	if !sawWanted() {
		t.Errorf("pool never handed back a pooled buffer of capacity %d", wantedCap)
	}
}

func TestWriterPoolIgnoresEmptyBuffer(t *testing.T) {
	releaseBuf(nil)
	if cap(acquireBuf()) == 0 {
		t.Error("pool returned a zero-capacity buffer")
	}
}

func TestWriterPoolDropsOversizedBuffer(t *testing.T) {
	releaseBuf(make([]byte, 0, pooledBufMax+1))

	for i := 0; i < 4; i++ {
		if got := acquireBuf(); cap(got) > pooledBufMax {
			t.Fatalf("acquireBuf cap = %d, want no pooled buffer over %d", cap(got), pooledBufMax)
		}
	}
}

func TestPooledWriterRelease(t *testing.T) {
	w := newPooledWriter()
	if err := w.WriteInt(7); err != nil {
		t.Fatal(err)
	}
	if got := string(w.Bytes()); got != "7" {
		t.Fatalf("Bytes = %q, want 7", got)
	}

	w.release()
	if w.buf != nil {
		t.Errorf("buf = %v, want nil after release", w.buf)
	}
	if w.err != nil || w.wroteRoot || w.pendingKey || len(w.stack) != 0 {
		t.Errorf("release left state behind: err=%v wroteRoot=%v pendingKey=%v stack=%d",
			w.err, w.wroteRoot, w.pendingKey, len(w.stack))
	}
	w.release() // a second release of an empty Writer must be a no-op
	if err := w.WriteInt(9); err != nil {
		t.Fatalf("reused Writer: %v", err)
	}
	if got := string(w.Bytes()); got != "9" {
		t.Errorf("reused Writer Bytes = %q, want 9", got)
	}

	// release hands the buffer back to the pool: offer a large buffer through a
	// Writer and look for its capacity again.
	reused := false
	for i := 0; i < 64 && !reused; i++ {
		p := newPooledWriter()
		p.buf = append(p.buf, make([]byte, wantedCap)...)
		p.buf = p.buf[:0]
		p.release()
		reused = cap(acquireBuf()) >= wantedCap
	}
	if !reused {
		t.Errorf("release never returned a buffer of capacity %d to the pool", wantedCap)
	}
}

// pooledMin is a document size the pool's New never reaches (New allocates
// pooledBufCap), so observing a retained buffer this large proves a Marshaller
// used the pool. It stays under pooledBufMax so releaseBuf keeps the buffer.
const pooledMin = 8 * pooledBufCap

// wideString is an Encoder whose single member carries a string long enough to
// grow the writer past pooledBufCap, so the buffer it hands back is recognizable
// by capacity.
type wideString struct{ s string }

func (v *wideString) EncodeJSON(w *Writer) error {
	if err := w.BeginObject(); err != nil {
		return err
	}
	if err := w.Key("s"); err != nil {
		return err
	}
	if err := w.WriteString(v.s); err != nil {
		return err
	}
	return w.EndObject()
}

// TestMarshalReusesPooledBuffer proves Marshal writes through a pooled Writer and
// returns its buffer for reuse: it encodes a document large enough that the
// buffer must have come from the pool's warm allocation, then looks for that
// capacity back in the pool. sync.Pool may drop a stored item, so it retries.
func TestMarshalReusesPooledBuffer(t *testing.T) {
	v := &wideString{s: strings.Repeat("x", pooledMin)}

	reused := false
	for i := 0; i < 64 && !reused; i++ {
		got, err := Marshal[wideString](*v)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if len(got) < pooledMin {
			t.Fatalf("Marshal produced %d bytes, want at least %d", len(got), pooledMin)
		}
		reused = cap(acquireBuf()) >= pooledMin
	}
	if !reused {
		t.Errorf("Marshal never returned a buffer of capacity %d to the pool", pooledMin)
	}
}

// TestSchemaMarshalReusesPooledBuffer is the Schema.Marshal counterpart: the
// schema path must reuse the same pooled buffer.
func TestSchemaMarshalReusesPooledBuffer(t *testing.T) {
	schema := Struct(String[wideString]("s", func(v *wideString) *string { return &v.s }))
	v := wideString{s: strings.Repeat("x", pooledMin)}

	reused := false
	for i := 0; i < 64 && !reused; i++ {
		got, err := schema.Marshal(v)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if len(got) < pooledMin {
			t.Fatalf("Marshal produced %d bytes, want at least %d", len(got), pooledMin)
		}
		reused = cap(acquireBuf()) >= pooledMin
	}
	if !reused {
		t.Errorf("Marshal never returned a buffer of capacity %d to the pool", pooledMin)
	}
}

// TestMarshalResultIndependentOfPool proves the returned slice is a copy: a later
// Marshal reusing the pooled buffer must not mutate an earlier result. Returning
// the pooled buffer itself would let the next encode overwrite the caller's data.
func TestMarshalResultIndependentOfPool(t *testing.T) {
	v := &wideString{s: strings.Repeat("x", pooledMin)}

	first, err := Marshal[wideString](*v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := string(first)

	for i := 0; i < 64; i++ {
		if _, err := Marshal[wideString](*v); err != nil {
			t.Fatalf("Marshal: %v", err)
		}
	}
	if string(first) != want {
		t.Error("a later Marshal mutated an earlier result: the returned slice aliases the pool")
	}
}
