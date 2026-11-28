package bench

// Record is the representative flat document type every benchmark encodes and
// decodes. The json tags fix the member names all three codecs read and write,
// so the shared input bytes are interpreted identically.
//
//easyjson:json
type Record struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Active    bool     `json:"active"`
	Score     float64  `json:"score"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
}

// Records is a slice of Record, so the 100-record case is one document.
//
//easyjson:json
type Records []Record

// Geo is the innermost object of the nested shape, so Nested reaches three
// levels of nesting below the root.
//
//easyjson:json
type Geo struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Address is the second level of the nested shape.
//
//easyjson:json
type Address struct {
	Street string `json:"street"`
	City   string `json:"city"`
	Geo    Geo    `json:"geo"`
}

// Company is the first level of the nested shape.
//
//easyjson:json
type Company struct {
	Name    string  `json:"name"`
	Address Address `json:"address"`
}

// Nested is the root of the nested-object shape: Nested -> Company -> Address ->
// Geo, three levels of nesting below the root.
//
//easyjson:json
type Nested struct {
	ID      int64   `json:"id"`
	Company Company `json:"company"`
}

// MapDoc is a struct with two map fields, so the dynamic-key paths of all three
// codecs are exercised.
//
//easyjson:json
type MapDoc struct {
	ID       int64             `json:"id"`
	Labels   map[string]string `json:"labels"`
	Counters map[string]int64  `json:"counters"`
}

// UnicodeDoc holds string fields with non-ASCII runes and JSON escape
// sequences, so the decoder's UTF-8 and escape handling is measured, not just
// plain ASCII.
//
//easyjson:json
type UnicodeDoc struct {
	ID      int64  `json:"id"`
	Text    string `json:"text"`
	Escaped string `json:"escaped"`
	Mixed   string `json:"mixed"`
}
