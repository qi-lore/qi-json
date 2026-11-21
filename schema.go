package qijson

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"qi-json/scanner"
)

// Schema decodes JSON objects into values of T. Build one with Struct; the zero
// value is not usable.
type Schema[T any] struct {
	// fields holds the fields sorted by Name for binary-search lookup.
	fields []Field[T]

	// decl holds the fields in declaration order, used by Encode so member
	// order is stable and natural.
	decl []Field[T]

	rejectUnknown   bool
	rejectDuplicate bool
}

// Struct returns a Schema that decodes and encodes a JSON object using fields.
// Field names must be unique; the order of the arguments does not matter, and
// Encode writes members in that argument (declaration) order.
func Struct[T any](fields ...Field[T]) *Schema[T] {
	s := &Schema[T]{
		fields: slices.Clone(fields),
		decl:   slices.Clone(fields),
	}
	slices.SortFunc(s.fields, func(a, b Field[T]) int {
		return strings.Compare(a.Name, b.Name)
	})
	return s
}

// RejectUnknown returns s configured to fail on an object member that no field
// matches. By default such members are ignored.
func (s *Schema[T]) RejectUnknown() *Schema[T] {
	s.rejectUnknown = true
	return s
}

// RejectDuplicate returns s configured to fail when an object repeats a member
// that a field already consumed. By default the last occurrence wins.
func (s *Schema[T]) RejectDuplicate() *Schema[T] {
	s.rejectDuplicate = true
	return s
}

// Decode reads one JSON object from r and stores its members into v. A member
// that no field matches is ignored unless RejectUnknown was set; a member that
// repeats is overwritten unless RejectDuplicate was set.
func (s *Schema[T]) Decode(r *Reader, v *T) error {
	var seen []bool
	if s.rejectDuplicate {
		seen = make([]bool, len(s.fields))
	}

	return r.objectRaw(func(raw []byte) error {
		idx, f := s.lookup(raw)
		if f == nil {
			if s.rejectUnknown {
				return r.unknownField(raw)
			}
			return r.Skip()
		}
		if s.rejectDuplicate {
			if seen[idx] {
				return r.duplicateField(f.Name)
			}
			seen[idx] = true
		}

		before := len(r.comps)
		r.pushKey(f.Name)
		err := f.Decode(r, v)
		r.comps = r.comps[:before]
		return err
	})
}

// Encode writes v as a JSON object to w, emitting members in declaration order.
// A field whose OmitEmpty is set and whose Empty reports true is skipped.
func (s *Schema[T]) Encode(w *Writer, v *T) error {
	if err := w.BeginObject(); err != nil {
		return err
	}
	for i := range s.decl {
		f := &s.decl[i]
		if f.omitEmpty && f.Empty != nil && f.Empty(v) {
			continue
		}
		if f.Encode == nil {
			return fmt.Errorf("qijson: field %q has no encoder", f.Name)
		}
		if err := w.Key(f.Name); err != nil {
			return err
		}
		if err := f.Encode(w, v); err != nil {
			return err
		}
	}
	return w.EndObject()
}

// Marshal encodes v as a JSON object. It is the counterpart to Unmarshal.
func (s *Schema[T]) Marshal(v T) ([]byte, error) {
	var w Writer
	if err := s.Encode(&w, &v); err != nil {
		return nil, err
	}
	if w.err != nil {
		return nil, w.err
	}
	if !w.complete() {
		return nil, errIncomplete
	}
	return w.buf, nil
}

// Unmarshal decodes data into a new T.
func (s *Schema[T]) Unmarshal(data []byte) (T, error) {
	var v T
	err := s.UnmarshalInto(data, &v)
	return v, err
}

// UnmarshalInto decodes data into v, which must be non-nil. Members present in
// data overwrite the corresponding fields of v; members absent from data leave
// v unchanged.
func (s *Schema[T]) UnmarshalInto(data []byte, v *T) error {
	r := newReader(scanner.NewTokenizer(data))
	if err := s.Decode(r, v); err != nil {
		return err
	}
	// The document must hold exactly one object; Peek reports trailing content.
	if _, err := r.Peek(); err != nil {
		return err
	}
	return nil
}

// lookup returns the index and field whose Name matches raw, or (-1, nil). raw
// is the member key with escape sequences intact; a key that contains an escape
// is decoded before the comparison.
func (s *Schema[T]) lookup(raw []byte) (int, *Field[T]) {
	if idx, f := s.lookupBytes(raw); f != nil {
		return idx, f
	}
	if bytes.IndexByte(raw, '\\') >= 0 {
		if key, err := unescape(raw); err == nil {
			return s.lookupString(key)
		}
	}
	return -1, nil
}

// lookupBytes finds the field whose Name equals raw by binary search.
func (s *Schema[T]) lookupBytes(raw []byte) (int, *Field[T]) {
	lo, hi := 0, len(s.fields)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		switch c := compareKey(raw, s.fields[mid].Name); {
		case c == 0:
			return mid, &s.fields[mid]
		case c < 0:
			hi = mid
		default:
			lo = mid + 1
		}
	}
	return -1, nil
}

// lookupString finds the field whose Name equals key by binary search.
func (s *Schema[T]) lookupString(key string) (int, *Field[T]) {
	lo, hi := 0, len(s.fields)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		switch c := strings.Compare(key, s.fields[mid].Name); {
		case c == 0:
			return mid, &s.fields[mid]
		case c < 0:
			hi = mid
		default:
			lo = mid + 1
		}
	}
	return -1, nil
}

// compareKey compares a raw key to a field name without allocating.
func compareKey(raw []byte, name string) int {
	n := len(raw)
	if len(name) < n {
		n = len(name)
	}
	for i := 0; i < n; i++ {
		if raw[i] != name[i] {
			if raw[i] < name[i] {
				return -1
			}
			return 1
		}
	}
	return len(raw) - len(name)
}
