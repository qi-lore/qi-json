// Command example is a minimal, runnable demonstration of the qi-json
// decode-then-encode round-trip. It defines a small struct with scalar fields,
// binds a qijson schema to it through the qi-json-gen code generator, and
// decodes and re-encodes an inline JSON document. Run it with:
//
//	go run ./example
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/qi-lore/qi-json"
)

// User is a small JSON object with scalar fields. Its qijson binding — the
// schema plus the DecodeJSON and EncodeJSON methods — is emitted by qi-json-gen
// into zz_qi_json_gen.go, so this file declares no accessor closures by hand.
//
//go:generate go run github.com/qi-lore/qi-json/cmd/qi-json-gen -type=User
type User struct {
	Name     string  `json:"name"`
	Age      int64   `json:"age"`
	Verified bool    `json:"verified"`
	Rating   float64 `json:"rating"`
}

// roundTrip checks the syntax of doc, decodes it into a User, and re-encodes the
// result. It returns the decoded value and the canonical JSON. It is the
// testable core of the example.
func roundTrip(doc []byte) (User, []byte, error) {
	if !qijson.Valid(doc) {
		return User{}, nil, fmt.Errorf("example: input is not valid JSON")
	}

	user, err := qijson.Unmarshal[User](doc)
	if err != nil {
		return User{}, nil, err
	}

	out, err := qijson.Marshal[User](user)
	if err != nil {
		return User{}, nil, err
	}

	return user, out, nil
}

// exampleDoc is the document the example decodes and re-encodes. It is a
// variable so a test can drive the failure path through main.
var exampleDoc = `{"name":"Ada Lovelace","age":36,"verified":true,"rating":4.5}`

// stdout and stderr are the process streams the example writes to. osExit maps a
// failure to the process exit status. They are variables so a test can capture
// the output and observe the exit status without terminating the test process.
var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
	osExit           = os.Exit
)

// main runs the example against the process streams and maps a failure to a
// non-zero exit status.
func main() {
	if err := run(exampleDoc, stdout, stderr); err != nil {
		osExit(1)
	}
}

// run performs the example's round-trip on doc and prints the decoded value and
// the canonical JSON to stdout. It is the testable shell around roundTrip.
func run(doc string, stdout, stderr io.Writer) error {
	user, out, err := roundTrip([]byte(doc))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "example:", err)
		return err
	}

	_, _ = fmt.Fprintf(stdout, "decoded:  name=%s age=%d verified=%t rating=%g\n", user.Name, user.Age, user.Verified, user.Rating)
	_, _ = fmt.Fprintf(stdout, "encoded:  %s\n", out)
	return nil
}
