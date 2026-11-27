package main

import "testing"

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
