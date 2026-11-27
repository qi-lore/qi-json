package bench

import qijson "github.com/qi-lore/qi-json"

// stringCodec decodes and encodes a JSON string element; Record.Tags uses it.
var stringCodec = qijson.NewCodec[string](
	func(r *qijson.Reader) (string, error) { return r.ReadString() },
	func(w *qijson.Writer, v string) error { return w.WriteString(v) },
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
