package xtypes

import (
	"errors"
	"testing"

	"github.com/qi-lore/qi-json"
)

// TestAwareRoundTrip drives the qi-json-aware type's DecodeJSON and EncodeJSON
// end to end, so the generator's delegation target is exercised.
func TestAwareRoundTrip(t *testing.T) {
	data, err := qijson.Marshal[Aware](Aware{A: 3, B: "z"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(data), `{"a":3,"b":"z"}`; got != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}

	got, err := qijson.Unmarshal[Aware](data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != (Aware{A: 3, B: "z"}) {
		t.Errorf("Unmarshal = %+v, want {3 z}", got)
	}
}

// TestAwareDecodeError checks a malformed member value surfaces the decoder's
// error through DecodeJSON.
func TestAwareDecodeError(t *testing.T) {
	var a Aware
	err := qijson.UnmarshalInto[Aware]([]byte(`{"a":"nope"}`), &a)
	var te *qijson.Error
	if !errors.As(err, &te) {
		t.Fatalf("error = %v (%T), want *qijson.Error", err, err)
	}
}

// TestHalfDecodeJSON checks the one-sided type's DecodeJSON skips its value.
func TestHalfDecodeJSON(t *testing.T) {
	var h Half
	if err := qijson.UnmarshalInto[Half]([]byte(`{"a":1}`), &h); err != nil {
		t.Fatalf("UnmarshalInto: %v", err)
	}
	if h.A != 0 {
		t.Errorf("Half = %+v, want zero (DecodeJSON skips)", h)
	}
}

// TestWeirdMethods drives the deliberately mis-shaped DecodeJSON methods, which
// exist so the generator's interface-shape detection has non-matching inputs.
func TestWeirdMethods(t *testing.T) {
	if err := (Weird{}).DecodeJSON(0); err != nil {
		t.Errorf("Weird.DecodeJSON: %v", err)
	}
	if err := (Weird2{}).DecodeJSON(nil); err != nil {
		t.Errorf("Weird2.DecodeJSON: %v", err)
	}
	if err := (Weird3{}).DecodeJSON(&Reader{}); err != nil {
		t.Errorf("Weird3.DecodeJSON: %v", err)
	}
	var n Num
	if err := (Weird4{}).DecodeJSON(&n); err != nil {
		t.Errorf("Weird4.DecodeJSON: %v", err)
	}
}
