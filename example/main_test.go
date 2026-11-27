package main

import "testing"

// TestRoundTrip checks that the example's decode-then-encode helper produces the
// expected field values and a stable, canonical re-encoded form.
func TestRoundTrip(t *testing.T) {
	const doc = `{"host":"db.internal","port":5432,"secure":true,"weight":0.75}`

	srv, out, err := roundTrip([]byte(doc))
	if err != nil {
		t.Fatalf("roundTrip() error = %v, want nil", err)
	}

	want := Server{Host: "db.internal", Port: 5432, Secure: true, Weight: 0.75}
	if srv != want {
		t.Errorf("decoded = %+v, want %+v", srv, want)
	}

	const wantJSON = `{"host":"db.internal","port":5432,"secure":true,"weight":0.75}`
	if got := string(out); got != wantJSON {
		t.Errorf("encoded = %s, want %s", got, wantJSON)
	}
}

// TestRoundTripInvalid checks that a syntactically invalid document is rejected
// by the syntax check before decoding is attempted.
func TestRoundTripInvalid(t *testing.T) {
	if _, _, err := roundTrip([]byte(`{"host":}`)); err == nil {
		t.Fatal("roundTrip() error = nil, want non-nil for invalid JSON")
	}
}
