// Command example is a minimal, runnable demonstration of the qi-json
// decode-then-encode round-trip. It defines a small struct with scalar fields,
// binds a qijson.Schema to it, and decodes and re-encodes an inline JSON
// document. Run it with:
//
//	go run ./example
package main

import (
	"fmt"
	"os"

	"github.com/qi-lore/qi-json"
)

// Server is a small JSON object with scalar fields only. It implements
// qijson.Decoder and qijson.Encoder by delegating to serverSchema.
type Server struct {
	Host   string
	Port   int64
	Secure bool
	Weight float64
}

// serverSchema binds Server's JSON object members to its fields. Members are
// written in this declaration order, so the re-encoded form is stable.
var serverSchema = qijson.Struct(
	qijson.String[Server]("host", func(s *Server) *string { return &s.Host }),
	qijson.Int[Server]("port", func(s *Server) *int64 { return &s.Port }),
	qijson.Bool[Server]("secure", func(s *Server) *bool { return &s.Secure }),
	qijson.Float[Server]("weight", func(s *Server) *float64 { return &s.Weight }),
)

// DecodeJSON decodes one JSON object from r into s.
func (s *Server) DecodeJSON(r *qijson.Reader) error {
	return serverSchema.Decode(r, s)
}

// EncodeJSON encodes s as one JSON object to w.
func (s *Server) EncodeJSON(w *qijson.Writer) error {
	return serverSchema.Encode(w, s)
}

// roundTrip checks the syntax of doc, decodes it into a Server, and re-encodes
// the result. It returns the decoded value and the canonical JSON. It is the
// testable core of the example.
func roundTrip(doc []byte) (Server, []byte, error) {
	if !qijson.Valid(doc) {
		return Server{}, nil, fmt.Errorf("example: input is not valid JSON")
	}

	srv, err := qijson.Unmarshal[Server](doc)
	if err != nil {
		return Server{}, nil, err
	}

	out, err := qijson.Marshal[Server](srv)
	if err != nil {
		return Server{}, nil, err
	}

	return srv, out, nil
}

func main() {
	const doc = `{
		"host": "db.internal",
		"port": 5432,
		"secure": true,
		"weight": 0.75
	}`

	srv, out, err := roundTrip([]byte(doc))
	if err != nil {
		fmt.Fprintln(os.Stderr, "example:", err)
		os.Exit(1)
	}

	fmt.Printf("decoded:  host=%s port=%d secure=%t weight=%g\n", srv.Host, srv.Port, srv.Secure, srv.Weight)
	fmt.Printf("encoded:  %s\n", out)
}
