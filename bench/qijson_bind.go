package bench

import qijson "github.com/qi-lore/qi-json"

// stringCodec decodes and encodes a JSON string element; Record.Tags and
// MapDoc.Labels use it.
var stringCodec = qijson.NewCodec[string](
	func(r *qijson.Reader) (string, error) { return r.ReadString() },
	func(w *qijson.Writer, v string) error { return w.WriteString(v) },
)

// int64Codec decodes and encodes a JSON integer element; MapDoc.Counters uses it.
var int64Codec = qijson.NewCodec[int64](
	func(r *qijson.Reader) (int64, error) { return r.ReadInt() },
	func(w *qijson.Writer, v int64) error { return w.WriteInt(v) },
)

// recordSchema maps Record's members to its fields with the qi-json field
// constructors. It is hand-written (the generator alternative emits the same
// accessor closures). The member names match the json tags, so the qi-json
// binding reads and writes the same document as encoding/json and easyjson.
var recordSchema = qijson.Struct(
	qijson.Int[Record]("id", func(v *Record) *int64 { return &v.ID }),
	qijson.String[Record]("name", func(v *Record) *string { return &v.Name }),
	qijson.String[Record]("email", func(v *Record) *string { return &v.Email }),
	qijson.Bool[Record]("active", func(v *Record) *bool { return &v.Active }),
	qijson.Float[Record]("score", func(v *Record) *float64 { return &v.Score }),
	qijson.Slice[Record, string]("tags", func(v *Record) *[]string { return &v.Tags }, stringCodec),
	qijson.String[Record]("created_at", func(v *Record) *string { return &v.CreatedAt }),
)

// DecodeJSON decodes a JSON object into v. It implements qijson.Decoder.
func (v *Record) DecodeJSON(r *qijson.Reader) error { return recordSchema.Decode(r, v) }

// EncodeJSON encodes v as a JSON object. It implements qijson.Encoder.
func (v *Record) EncodeJSON(w *qijson.Writer) error { return recordSchema.Encode(w, v) }

// DecodeJSON decodes a JSON array of records into v. It implements
// qijson.Decoder.
//
// NOTE: this grows the result with append and cannot pre-size it: a hand-written
// top-level array decoder has no public length hint, so it pays the slice-growth
// allocations the library's own Slice field codec avoids. This is a measured
// property of the harness, not an accident; see bench/README.md and the decode
// performance roadmap. Do not "optimize" it away without changing the library.
func (v *Records) DecodeJSON(r *qijson.Reader) error {
	var out []Record
	if err := r.Array(func(int) error {
		var rec Record
		if err := recordSchema.Decode(r, &rec); err != nil {
			return err
		}
		out = append(out, rec)
		return nil
	}); err != nil {
		return err
	}
	*v = out
	return nil
}

// EncodeJSON encodes v as a JSON array of records. It implements qijson.Encoder.
func (v *Records) EncodeJSON(w *qijson.Writer) error {
	if err := w.BeginArray(); err != nil {
		return err
	}
	for i := range *v {
		if err := recordSchema.Encode(w, &(*v)[i]); err != nil {
			return err
		}
	}
	return w.EndArray()
}

// geoSchema maps Geo's members to its fields.
var geoSchema = qijson.Struct(
	qijson.Float[Geo]("lat", func(v *Geo) *float64 { return &v.Lat }),
	qijson.Float[Geo]("lon", func(v *Geo) *float64 { return &v.Lon }),
)

// addressSchema maps Address's members to its fields, nesting Geo as an object.
var addressSchema = qijson.Struct(
	qijson.String[Address]("street", func(v *Address) *string { return &v.Street }),
	qijson.String[Address]("city", func(v *Address) *string { return &v.City }),
	qijson.Object[Address, Geo]("geo", func(v *Address) *Geo { return &v.Geo }, geoSchema),
)

// companySchema maps Company's members to its fields, nesting Address as an
// object.
var companySchema = qijson.Struct(
	qijson.String[Company]("name", func(v *Company) *string { return &v.Name }),
	qijson.Object[Company, Address]("address", func(v *Company) *Address { return &v.Address }, addressSchema),
)

// nestedSchema maps Nested's members to its fields, nesting Company as an object.
var nestedSchema = qijson.Struct(
	qijson.Int[Nested]("id", func(v *Nested) *int64 { return &v.ID }),
	qijson.Object[Nested, Company]("company", func(v *Nested) *Company { return &v.Company }, companySchema),
)

// DecodeJSON decodes a nested JSON object into v. It implements qijson.Decoder.
func (v *Nested) DecodeJSON(r *qijson.Reader) error { return nestedSchema.Decode(r, v) }

// EncodeJSON encodes v as a nested JSON object. It implements qijson.Encoder.
func (v *Nested) EncodeJSON(w *qijson.Writer) error { return nestedSchema.Encode(w, v) }

// mapDocSchema maps MapDoc's members to its fields, binding the two map fields
// through the Map field constructor.
var mapDocSchema = qijson.Struct(
	qijson.Int[MapDoc]("id", func(v *MapDoc) *int64 { return &v.ID }),
	qijson.Map[MapDoc, string]("labels", func(v *MapDoc) *map[string]string { return &v.Labels }, stringCodec),
	qijson.Map[MapDoc, int64]("counters", func(v *MapDoc) *map[string]int64 { return &v.Counters }, int64Codec),
)

// DecodeJSON decodes a JSON object with map members into v. It implements
// qijson.Decoder.
func (v *MapDoc) DecodeJSON(r *qijson.Reader) error { return mapDocSchema.Decode(r, v) }

// EncodeJSON encodes v as a JSON object with map members. It implements
// qijson.Encoder.
func (v *MapDoc) EncodeJSON(w *qijson.Writer) error { return mapDocSchema.Encode(w, v) }

// unicodeSchema maps UnicodeDoc's members to its fields.
var unicodeSchema = qijson.Struct(
	qijson.Int[UnicodeDoc]("id", func(v *UnicodeDoc) *int64 { return &v.ID }),
	qijson.String[UnicodeDoc]("text", func(v *UnicodeDoc) *string { return &v.Text }),
	qijson.String[UnicodeDoc]("escaped", func(v *UnicodeDoc) *string { return &v.Escaped }),
	qijson.String[UnicodeDoc]("mixed", func(v *UnicodeDoc) *string { return &v.Mixed }),
)

// DecodeJSON decodes a JSON object with unicode/escape strings into v. It
// implements qijson.Decoder.
func (v *UnicodeDoc) DecodeJSON(r *qijson.Reader) error { return unicodeSchema.Decode(r, v) }

// EncodeJSON encodes v as a JSON object with unicode/escape strings. It
// implements qijson.Encoder.
func (v *UnicodeDoc) EncodeJSON(w *qijson.Writer) error { return unicodeSchema.Encode(w, v) }
