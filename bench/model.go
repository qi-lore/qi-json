package bench

// Record is the representative document type every benchmark encodes and
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
