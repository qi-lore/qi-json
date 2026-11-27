package codec

// This file implements the unknown-member round-trip proxy: the UnmarshalUnknown
// and MarshalUnknowns hooks a decode or encode target implements to capture and
// re-emit object members that match no field. It is the complement to
// RejectUnknown, which fails on such members instead of preserving them.

// UnmarshalUnknown is implemented by a decode target that wants to receive the
// object members no field matches, instead of seeing them ignored. During
// Schema.Decode the decoder calls UnmarshalUnknown once per unmatched member,
// after the member's name is known and while its value is still unconsumed.
//
// name is the member name, decoded from any JSON escape sequences. raw is the
// member value as its exact JSON bytes; a copy is passed, so the callee owns the
// bytes and may retain them past the call. Returning a non-nil error aborts the
// decode and is returned to the caller unchanged.
//
// Precedence: when a Schema also has RejectUnknown set, RejectUnknown wins and an
// unmatched member is reported as an unknown-field error; UnmarshalUnknown is not
// called. Known members are unaffected — they bind to their fields as usual, and
// a duplicate known member follows the RejectDuplicate policy independently.
//
// A target may implement UnmarshalUnknown through a value receiver or a pointer
// receiver; it is discovered by a type assertion on the decode target, so no
// reflection or registration is involved.
type UnmarshalUnknown interface {
	// UnmarshalUnknown receives one unmatched object member, by name and raw
	// JSON value, while Schema.Decode runs.
	UnmarshalUnknown(name string, raw []byte) error
}

// MarshalUnknowns is implemented by an encode target that holds members captured
// by UnmarshalUnknown and wants them written back. During Schema.Encode the
// encoder calls MarshalUnknowns once, after the declared fields are written, and
// while the object is still open, so the implementation writes its extra members
// with Writer.Key followed by a value, the same way a Field's Encode does:
//
//	func (v *T) MarshalUnknowns(w *qijson.Writer) error {
//	    for _, m := range v.extra {
//	        if err := w.Key(m.name); err != nil {
//	            return err
//	        }
//	        if err := w.WriteRaw(m.raw); err != nil {
//	            return err
//	        }
//	    }
//	    return nil
//	}
//
// The extra members follow the declared fields, in the order the implementation
// writes them; the Schema imposes no order of its own. Returning a non-nil error
// aborts the encode and is returned to the caller unchanged.
//
// A target may implement MarshalUnknowns through a value receiver or a pointer
// receiver; it is discovered by a type assertion on the encode target, so no
// reflection or registration is involved.
type MarshalUnknowns interface {
	// MarshalUnknowns writes the target's extra object members into the open
	// object of w.
	MarshalUnknowns(*Writer) error
}

// captureUnknown reads the current object member's value as raw JSON and hands
// it, with the member's decoded name, to uu. rawKey is the member key with escape
// sequences intact, as objectRaw passes it.
func (r *Reader) captureUnknown(uu UnmarshalUnknown, rawKey []byte) error {
	// Decode the name before reading the value: a later read may refill and move
	// the buffer that rawKey aliases.
	name, err := unescape(rawKey)
	if err != nil {
		// A malformed escape cannot reach here: the scanner rejects it while
		// reading the key, so this guards the invariant.
		name = string(rawKey)
	}

	before := len(r.comps)
	r.pushKey(name)
	val, err := r.raw()
	r.comps = r.comps[:before]
	if err != nil {
		return err
	}

	// r.raw aliases the input; copy so the callee owns the bytes.
	raw := make([]byte, len(val))
	copy(raw, val)
	return uu.UnmarshalUnknown(name, raw)
}
