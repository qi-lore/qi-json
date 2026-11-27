package codec

import (
	"errors"
	"testing"
)

// uuMember is one captured unknown member: the decoded member name and the raw
// JSON value bytes, which the capture owns.
type uuMember struct {
	name string
	raw  []byte
}

// uuCapture is a decode and encode target that implements both unknown-member
// hooks: it records every unmatched member on decode and writes the recorded
// members back on encode.
type uuCapture struct {
	A     int64
	extra []uuMember
}

func (v *uuCapture) UnmarshalUnknown(name string, raw []byte) error {
	v.extra = append(v.extra, uuMember{name: name, raw: raw})
	return nil
}

func (v *uuCapture) MarshalUnknowns(w *Writer) error {
	for _, m := range v.extra {
		if err := w.Key(m.name); err != nil {
			return err
		}
		if err := w.WriteRaw(m.raw); err != nil {
			return err
		}
	}
	return nil
}

// uuSchema binds only "a"; every other member is unknown to it.
func uuSchema() *Schema[uuCapture] {
	return Struct(Int[uuCapture]("a", func(v *uuCapture) *int64 { return &v.A }))
}

// uuMemberNames returns the captured member names in order.
func uuMemberNames(members []uuMember) []string {
	names := make([]string, len(members))
	for i, m := range members {
		names[i] = m.name
	}
	return names
}

// TestUnmarshalUnknownCapturesMembers checks that a target implementing
// UnmarshalUnknown receives every unmatched member, with its name and raw value,
// instead of the member being skipped.
func TestUnmarshalUnknownCapturesMembers(t *testing.T) {
	var got uuCapture
	err := uuSchema().UnmarshalInto([]byte(`{"a":1,"b":"x","c":{"d":[1,2,3]}}`), &got)
	if err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if got.A != 1 {
		t.Errorf("A = %d, want 1", got.A)
	}

	want := []uuMember{
		{name: "b", raw: []byte(`"x"`)},
		{name: "c", raw: []byte(`{"d":[1,2,3]}`)},
	}
	if len(got.extra) != len(want) {
		t.Fatalf("captured %d members %v, want %d", len(got.extra), uuMemberNames(got.extra), len(want))
	}
	for i, w := range want {
		if got.extra[i].name != w.name || string(got.extra[i].raw) != string(w.raw) {
			t.Errorf("member %d = {%q %s}, want {%q %s}", i, got.extra[i].name, got.extra[i].raw, w.name, w.raw)
		}
	}
}

// TestUnmarshalUnknownEscapedKey checks the captured name is the decoded member
// name, not the raw key bytes.
func TestUnmarshalUnknownEscapedKey(t *testing.T) {
	var got uuCapture
	err := uuSchema().UnmarshalInto([]byte(`{"a":1,"\u0062":2}`), &got)
	if err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if len(got.extra) != 1 || got.extra[0].name != "b" {
		t.Fatalf("captured %v, want one member named b", uuMemberNames(got.extra))
	}
}

// TestUnmarshalUnknownRawDoesNotAliasInput checks the captured raw value is a
// copy: clobbering the input buffer after the call leaves the captured bytes
// intact, so a caller may retain them.
func TestUnmarshalUnknownRawDoesNotAliasInput(t *testing.T) {
	data := []byte(`{"a":1,"b":"original"}`)
	var got uuCapture
	if err := uuSchema().UnmarshalInto(data, &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	for i := range data {
		data[i] = 'X'
	}
	if len(got.extra) != 1 || string(got.extra[0].raw) != `"original"` {
		t.Fatalf("captured raw = %s, want %s", rawOrNil(got.extra), `"original"`)
	}
}

// rawOrNil renders the first captured member's raw bytes for a test message.
func rawOrNil(members []uuMember) string {
	if len(members) == 0 {
		return "<none>"
	}
	return string(members[0].raw)
}

// TestUnmarshalUnknownRoundTrip decodes a document with extra members into a
// capturing type and encodes it back: the unknown members reappear with the same
// names and values, in decode order, after the declared fields.
func TestUnmarshalUnknownRoundTrip(t *testing.T) {
	schema := uuSchema()
	in := `{"a":1,"b":"x","c":{"d":[1,2,3]},"e":[true,null]}`
	var got uuCapture
	if err := schema.UnmarshalInto([]byte(in), &got); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	out, err := schema.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(out) != in {
		t.Errorf("round trip:\n got %s\nwant %s", out, in)
	}
}

// TestUnmarshalUnknownRejectUnknownWins checks the precedence between the two
// strictness settings: when RejectUnknown is set, an unmatched member is an error
// and UnmarshalUnknown is not called.
func TestUnmarshalUnknownRejectUnknownWins(t *testing.T) {
	schema := uuSchema().RejectUnknown()
	var got uuCapture
	err := schema.UnmarshalInto([]byte(`{"a":1,"b":2}`), &got)
	var e *Error
	if !errors.As(err, &e) || e.Kind != KindUnknownField {
		t.Fatalf("error = %v, want a *Error with Kind KindUnknownField", err)
	}
	if len(got.extra) != 0 {
		t.Errorf("captured %v, want none under RejectUnknown", uuMemberNames(got.extra))
	}
}

// TestUnmarshalUnknownRejectDuplicateUnaffected checks that the duplicate-member
// policy still applies to known fields: with RejectDuplicate a repeated known
// member is an error even though UnmarshalUnknown is implemented.
func TestUnmarshalUnknownRejectDuplicateUnaffected(t *testing.T) {
	schema := uuSchema().RejectDuplicate()
	var got uuCapture
	err := schema.UnmarshalInto([]byte(`{"a":1,"a":2}`), &got)
	var e *Error
	if !errors.As(err, &e) || e.Kind != KindDuplicateField {
		t.Fatalf("error = %v, want a *Error with Kind KindDuplicateField", err)
	}
}

// uuWriteOnly implements only MarshalUnknowns: it writes extra members but does
// not capture them on decode. It proves the encode hook is independent of the
// decode hook.
type uuWriteOnly struct {
	A    int64
	When string
}

func (v *uuWriteOnly) MarshalUnknowns(w *Writer) error {
	if v.When == "" {
		return nil
	}
	if err := w.Key("when"); err != nil {
		return err
	}
	return w.WriteString(v.When)
}

// TestMarshalUnknownsWritesExtraMembers checks that a type implementing only
// MarshalUnknowns has its extra members written after the declared fields.
func TestMarshalUnknownsWritesExtraMembers(t *testing.T) {
	schema := Struct(Int[uuWriteOnly]("a", func(v *uuWriteOnly) *int64 { return &v.A }))
	out, err := schema.Marshal(uuWriteOnly{A: 1, When: "now"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(out) != `{"a":1,"when":"now"}` {
		t.Errorf("Marshal = %s, want {\"a\":1,\"when\":\"now\"}", out)
	}
}

// uuErrEncoder implements MarshalUnknowns by always failing, so the
// error-propagation test can assert the hook's error surfaces.
type uuErrEncoder struct {
	err error
}

func (v *uuErrEncoder) MarshalUnknowns(*Writer) error { return v.err }

// TestMarshalUnknownsErrorPropagates checks an error from the encode hook is
// returned unchanged.
func TestMarshalUnknownsErrorPropagates(t *testing.T) {
	sentinel := errors.New("boom")
	schema := Struct(Int[uuErrEncoder]("a", func(v *uuErrEncoder) *int64 { return new(int64) }))
	if _, err := schema.Marshal(uuErrEncoder{err: sentinel}); !errors.Is(err, sentinel) {
		t.Fatalf("Marshal error = %v, want %v", err, sentinel)
	}
}
