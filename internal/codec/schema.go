package codec

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/qi-lore/qi-json/internal/scanner"
)

// errNoFieldEncoder reports a Field whose Encode is nil, reached by
// Schema.Encode. It is a programming error rather than a property of the input,
// so it is a plain error rather than a *qijson.Error.
var errNoFieldEncoder = errors.New("no encoder")

// errDuplicateFieldName reports a Struct built with two fields of the same Name.
// It is a programming error rather than a property of the input, so it is a
// plain error rather than a *qijson.Error. A Schema carrying it fails every
// Decode and Encode instead of silently binding and writing an ambiguous member.
var errDuplicateFieldName = errors.New("qijson: duplicate field name")

// Schema decodes and encodes JSON objects into and from values of T. Build one
// with Struct; the zero value is not usable.
//
// A Schema is immutable once built and safe for concurrent use by Decode, Encode,
// Marshal, Unmarshal and UnmarshalInto. RejectUnknown and RejectDuplicate mutate
// it, so finish configuring it before sharing it.
type Schema[T any] struct {
	// fields holds the fields sorted by Name for binary-search lookup.
	fields []Field[T]

	// decl holds the fields in declaration order, used by Encode so member
	// order is stable and natural.
	decl []Field[T]

	rejectUnknown   bool
	rejectDuplicate bool

	// buildErr records a misuse caught by Struct, such as a duplicate field
	// name. Decode and Encode return it before doing any work, so a
	// misconfigured Schema fails every use instead of silently misbehaving.
	buildErr error
}

// Struct returns a Schema that decodes and encodes a JSON object using fields.
// Field names must be unique; the order of the arguments does not matter, and
// Encode writes members in that argument (declaration) order. A repeated name is
// a programming error: every Decode and Encode of the returned Schema fails with
// an error instead of silently binding and writing an ambiguous member.
func Struct[T any](fields ...Field[T]) *Schema[T] {
	s := &Schema[T]{
		fields: slices.Clone(fields),
		decl:   slices.Clone(fields),
	}
	slices.SortFunc(s.fields, func(a, b Field[T]) int {
		return strings.Compare(a.Name, b.Name)
	})
	for i := 1; i < len(s.fields); i++ {
		if s.fields[i].Name == s.fields[i-1].Name {
			s.buildErr = fmt.Errorf("%w %q", errDuplicateFieldName, s.fields[i].Name)
			break
		}
	}
	return s
}

// RejectUnknown returns s configured to fail on an object member that no field
// matches. By default such members are ignored. It mutates s, so it must not run
// concurrently with any use of s or with RejectDuplicate.
func (s *Schema[T]) RejectUnknown() *Schema[T] {
	s.rejectUnknown = true
	return s
}

// RejectDuplicate returns s configured to fail when an object repeats a member
// that a field already consumed. By default the last occurrence wins. It mutates
// s, so it must not run concurrently with any use of s or with RejectUnknown.
func (s *Schema[T]) RejectDuplicate() *Schema[T] {
	s.rejectDuplicate = true
	return s
}

// Decode reads one JSON object from r and stores its members into v. A member
// that no field matches is ignored unless RejectUnknown was set; a member that
// repeats is overwritten unless RejectDuplicate was set.
//
// A nil v is a programming error reported as errNilTarget rather than a panic,
// matching the other decode entry points.
func (s *Schema[T]) Decode(r *Reader, v *T) error {
	if s.buildErr != nil {
		return s.buildErr
	}
	if v == nil {
		return errNilTarget
	}

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
	if s.buildErr != nil {
		return s.buildErr
	}
	if err := w.BeginObject(); err != nil {
		return err
	}
	for i := range s.decl {
		f := &s.decl[i]
		if f.omitEmpty && f.Empty != nil && f.Empty(v) {
			continue
		}
		if f.Encode == nil {
			return fmt.Errorf("qijson: field %q: %w", f.Name, errNoFieldEncoder)
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

// Marshal encodes v as a JSON object. It is the counterpart to Unmarshal. The
// returned []byte is freshly allocated and owned by the caller, who may keep and
// modify it. Internally the encoding reuses a pooled buffer, and the result is a
// copy of it, so the returned slice never aliases that shared buffer.
func (s *Schema[T]) Marshal(v T) ([]byte, error) {
	w := newPooledWriter()
	defer w.release()
	if err := s.Encode(w, &v); err != nil {
		return nil, err
	}
	if w.err != nil {
		return nil, w.err
	}
	if !w.complete() {
		return nil, errIncomplete
	}
	return w.BuildBytes(), nil
}

// Unmarshal decodes data into a new T. data is not retained after the call.
func (s *Schema[T]) Unmarshal(data []byte) (T, error) {
	var v T
	err := s.UnmarshalInto(data, &v)
	return v, err
}

// UnmarshalInto decodes data into v. Members present in data overwrite the
// corresponding fields of v; members absent from data leave v unchanged. v is
// written in place and data is not retained after the call.
//
// A nil v is a programming error reported as errNilTarget rather than a panic,
// matching UnmarshalInto and Decode in the semantic layer.
func (s *Schema[T]) UnmarshalInto(data []byte, v *T) error {
	if v == nil {
		return errNilTarget
	}

	r := newReader(scanner.NewTokenizer(data), string(data))
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
