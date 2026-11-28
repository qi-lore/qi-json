package bench

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	easyjson "github.com/mailru/easyjson"
	qijson "github.com/qi-lore/qi-json"
)

// TestCodecsAgree fixes the harness contract: for every shape the three codecs
// read the same bytes and produce the same value. If a codec ever disagreed, the
// comparative numbers would not be comparable, so this fails loudly instead.
func TestCodecsAgree(t *testing.T) {
	t.Run("record", func(t *testing.T) {
		want := oneRecord
		var std, ej, qi Record
		decode(t, jsonRecord, &std, &ej, &qi)
		report(t, equalRecord, want, std, ej, qi)
	})
	t.Run("records", func(t *testing.T) {
		want := manyRecord
		var std, ej, qi Records
		decode(t, jsonRecords, &std, &ej, &qi)
		report(t, equalRecords, want, std, ej, qi)
	})
	t.Run("nested", func(t *testing.T) {
		want := nestedDoc
		var std, ej, qi Nested
		decode(t, jsonNested, &std, &ej, &qi)
		report(t, func(a, b Nested) bool { return a == b }, want, std, ej, qi)
	})
	t.Run("map", func(t *testing.T) {
		want := mapDoc
		var std, ej, qi MapDoc
		decode(t, jsonMapDoc, &std, &ej, &qi)
		report(t, equalMapDoc, want, std, ej, qi)
	})
	t.Run("unicode", func(t *testing.T) {
		want := unicodeDoc
		var std, ej, qi UnicodeDoc
		decode(t, jsonUnicode, &std, &ej, &qi)
		report(t, func(a, b UnicodeDoc) bool { return a == b }, want, std, ej, qi)
	})
}

// decode decodes data into the three typed targets with each codec. The targets
// are typed pointers; the codec interfaces are checked by the type assertions, so
// a type that stops implementing one of them fails this test rather than passing
// silently.
func decode(t *testing.T, data []byte, std, ej, qi any) {
	t.Helper()
	if err := json.Unmarshal(data, std); err != nil {
		t.Fatalf("stdlib: %v", err)
	}
	if err := easyjson.Unmarshal(data, ej.(easyjson.Unmarshaler)); err != nil {
		t.Fatalf("easyjson: %v", err)
	}
	if err := qijson.Decode(data, qi.(qijson.Decoder)); err != nil {
		t.Fatalf("qijson: %v", err)
	}
}

// report asserts each decoded value equals want and that the three agree.
func report[T any](t *testing.T, equal func(T, T) bool, want, std, ej, qi T) {
	t.Helper()
	if !equal(std, want) {
		t.Errorf("stdlib decoded %+v, want %+v", std, want)
	}
	if !equal(ej, want) {
		t.Errorf("easyjson decoded %+v, want %+v", ej, want)
	}
	if !equal(qi, want) {
		t.Errorf("qijson decoded %+v, want %+v", qi, want)
	}
	if !equal(std, ej) || !equal(ej, qi) {
		t.Errorf("codecs disagree: stdlib=%+v easyjson=%+v qijson=%+v", std, ej, qi)
	}
}

// equalRecord reports whether two Record values are equal field by field.
func equalRecord(a, b Record) bool {
	return a.ID == b.ID &&
		a.Name == b.Name &&
		a.Email == b.Email &&
		a.Active == b.Active &&
		a.Score == b.Score &&
		slices.Equal(a.Tags, b.Tags) &&
		a.CreatedAt == b.CreatedAt
}

// equalRecords reports whether two Records slices are equal element by element.
func equalRecords(a, b Records) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalRecord(a[i], b[i]) {
			return false
		}
	}
	return true
}

// equalMapDoc reports whether two MapDoc values are equal field by field.
func equalMapDoc(a, b MapDoc) bool {
	return a.ID == b.ID && maps.Equal(a.Labels, b.Labels) && maps.Equal(a.Counters, b.Counters)
}
