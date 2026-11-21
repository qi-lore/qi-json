package scanner

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// drive feeds in to a fresh machine one byte at a time and returns every
// opcode in order, followed by the opcode returned by finish when scanning
// succeeds. Scanning stops at the first failure.
func drive(t *testing.T, in string) ([]opcode, error) {
	t.Helper()

	var m machine
	ops := make([]opcode, 0, len(in)+1)

	for i := 0; i < len(in); i++ {
		op := m.step(in[i])
		ops = append(ops, op)
		if op == opFailure {
			return ops, m.failure
		}
	}

	op := m.finish()
	ops = append(ops, op)
	if op == opFailure {
		return ops, m.failure
	}
	return ops, nil
}

func requireParseError(t *testing.T, err error, wantAt int64, wantProblem string) {
	t.Helper()

	var pe *parseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v (%T), want *parseError", err, err)
	}
	if pe.at != wantAt {
		t.Errorf("error position = %d, want %d", pe.at, wantAt)
	}
	if !strings.Contains(pe.problem, wantProblem) {
		t.Errorf("problem = %q, want it to contain %q", pe.problem, wantProblem)
	}
}

func TestZeroValueMachine(t *testing.T) {
	var m machine

	if m.failure != nil {
		t.Errorf("failure = %v, want nil", m.failure)
	}
	if got := m.consumed; got != 0 {
		t.Errorf("consumed = %d, want 0", got)
	}

	if op := m.step('n'); op != opLiteralStart {
		t.Fatalf("first opcode = %v, want %v", op, opLiteralStart)
	}
	for i := 1; i < len("null"); i++ {
		if op := m.step("null"[i]); op != opContinue {
			t.Fatalf("opcode after byte %d = %v, want %v", i+1, op, opContinue)
		}
	}
	if op := m.finish(); op != opDocumentEnd {
		t.Fatalf("finish opcode = %v, want %v", op, opDocumentEnd)
	}
	if m.failure != nil {
		t.Fatalf("failure = %v, want nil", m.failure)
	}

	var empty machine
	if op := empty.finish(); op != opFailure {
		t.Fatalf("finish on zero value = %v, want %v", op, opFailure)
	}
}

func TestOpcodeSequence(t *testing.T) {
	tests := []struct {
		in   string
		want []opcode
	}{
		{`{}`, []opcode{opObjectStart, opObjectEnd, opDocumentEnd}},
		{`[]`, []opcode{opArrayStart, opArrayEnd, opDocumentEnd}},
		{`null`, []opcode{opLiteralStart, opContinue, opContinue, opContinue, opDocumentEnd}},
		{`true`, []opcode{opLiteralStart, opContinue, opContinue, opContinue, opDocumentEnd}},
		{`false`, []opcode{opLiteralStart, opContinue, opContinue, opContinue, opContinue, opDocumentEnd}},
		{`123`, []opcode{opLiteralStart, opContinue, opContinue, opDocumentEnd}},
		{`"hi"`, []opcode{opLiteralStart, opContinue, opContinue, opContinue, opDocumentEnd}},
		{`""`, []opcode{opLiteralStart, opContinue, opDocumentEnd}},
		{`[1,2]`, []opcode{opArrayStart, opLiteralStart, opContinue, opLiteralStart, opArrayEnd, opDocumentEnd}},
		{
			// A delimiter byte both completes the number and closes the object.
			`{"a":1}`,
			[]opcode{
				opObjectStart, opLiteralStart, opContinue,
				opContinue, opContinue, opLiteralStart,
				opObjectEnd, opDocumentEnd,
			},
		},
		{
			// The trailing space proves the number complete before finish.
			`123 `,
			[]opcode{opLiteralStart, opContinue, opContinue, opDocumentEnd, opDocumentEnd},
		},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			ops, err := drive(t, tt.in)
			if err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if !reflect.DeepEqual(ops, tt.want) {
				t.Fatalf("opcodes = %v, want %v", ops, tt.want)
			}
		})
	}
}

func TestSignificantOpsForNestedDocument(t *testing.T) {
	const in = `{"a":1,"b":[true,null]}`

	ops, err := drive(t, in)
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}

	got := make([]opcode, 0, len(ops))
	for _, op := range ops {
		if op != opContinue {
			got = append(got, op)
		}
	}

	want := []opcode{
		opObjectStart,
		opLiteralStart, // key "a"
		opLiteralStart, // 1
		opLiteralStart, // key "b"
		opArrayStart,
		opLiteralStart, // true
		opLiteralStart, // null
		opArrayEnd,
		opObjectEnd,
		opDocumentEnd,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("significant opcodes = %v, want %v", got, want)
	}
}

func TestDelayedDocumentEnd(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"integer", `123`},
		{"negative fraction", `-0.5`},
		{"exponent", `1e10`},
		{"true", `true`},
		{"false", `false`},
		{"null", `null`},
		{"string", `"x"`},
		{"unicode escape", `"\u00e9"`},
		{"object", `{"a":1}`},
		{"array", `[1]`},
		{"nested arrays", `[[[]]]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m machine
			for i := 0; i < len(tt.in); i++ {
				op := m.step(tt.in[i])
				if op == opDocumentEnd {
					t.Fatalf("document end reported after byte %d of %q, want it delayed", i+1, tt.in)
				}
				if op == opFailure {
					t.Fatalf("unexpected failure after byte %d of %q: %v", i+1, tt.in, m.failure)
				}
			}
			if op := m.finish(); op != opDocumentEnd {
				t.Fatalf("finish opcode = %v, want %v", op, opDocumentEnd)
			}
		})
	}
}

func TestDocumentEndOnDelimiterByte(t *testing.T) {
	for _, in := range []string{`123 `, " true\t", `[1] `, `{} `, `"x"` + "\r"} {
		t.Run(strconv.Quote(in), func(t *testing.T) {
			var m machine

			endPos := 0
			for i := 0; i < len(in); i++ {
				if op := m.step(in[i]); op == opDocumentEnd && endPos == 0 {
					endPos = i + 1
				}
			}

			if endPos != len(in) {
				t.Fatalf("first document end at position %d, want %d", endPos, len(in))
			}
			if op := m.finish(); op != opDocumentEnd {
				t.Fatalf("finish opcode = %v, want %v", op, opDocumentEnd)
			}
		})
	}
}

func TestMachineReset(t *testing.T) {
	var m machine

	feed := func(in string) opcode {
		var op opcode
		for i := 0; i < len(in); i++ {
			op = m.step(in[i])
		}
		return op
	}

	feed(`{`)
	if op := m.finish(); op != opFailure {
		t.Fatalf("finish opcode = %v, want %v", op, opFailure)
	}
	if m.failure == nil {
		t.Fatal("failure = nil, want a parse error")
	}

	m.reset()
	if m.failure != nil {
		t.Fatalf("failure after reset = %v, want nil", m.failure)
	}
	if got := m.consumed; got != 1 {
		t.Fatalf("consumed after reset = %d, want 1 (reset must not clear it)", got)
	}

	if op := feed(`[]`); op != opArrayEnd {
		t.Fatalf("opcode = %v, want %v", op, opArrayEnd)
	}
	if op := m.finish(); op != opDocumentEnd {
		t.Fatalf("finish opcode = %v, want %v", op, opDocumentEnd)
	}
	if got := m.consumed; got != 3 {
		t.Fatalf("consumed = %d, want 3", got)
	}
}

func TestMachineReuse(t *testing.T) {
	var m machine

	documents := []string{`{"a":1}`, `[]`, `true`, `-1.5e3`}
	for _, in := range documents {
		m.reset()
		for i := 0; i < len(in); i++ {
			if op := m.step(in[i]); op == opFailure {
				t.Fatalf("%q: unexpected failure: %v", in, m.failure)
			}
		}
		if op := m.finish(); op != opDocumentEnd {
			t.Fatalf("%q: finish opcode = %v, want %v", in, op, opDocumentEnd)
		}
	}
}

func TestFailureIsSticky(t *testing.T) {
	var m machine

	if op := m.step('x'); op != opFailure {
		t.Fatalf("opcode = %v, want %v", op, opFailure)
	}
	first := m.failure
	if first == nil {
		t.Fatal("failure = nil, want a parse error")
	}

	for _, c := range []byte(`{["]}1`) {
		if op := m.step(c); op != opFailure {
			t.Fatalf("after failure, step(%q) = %v, want %v", c, op, opFailure)
		}
		if m.failure != first {
			t.Fatalf("failure changed after an error; want the first error to be preserved")
		}
	}

	if op := m.finish(); op != opFailure {
		t.Fatalf("finish opcode = %v, want %v", op, opFailure)
	}
}

func TestFinishIsRepeatable(t *testing.T) {
	var m machine
	for i := 0; i < len(`null`); i++ {
		if op := m.step("null"[i]); op == opFailure {
			t.Fatalf("unexpected failure: %v", m.failure)
		}
	}

	if op := m.finish(); op != opDocumentEnd {
		t.Fatalf("first finish opcode = %v, want %v", op, opDocumentEnd)
	}
	if op := m.finish(); op != opDocumentEnd {
		t.Fatalf("second finish opcode = %v, want %v", op, opDocumentEnd)
	}

	m.reset()
	if op := m.finish(); op != opFailure {
		t.Fatalf("finish opcode on empty input = %v, want %v", op, opFailure)
	}
	if op := m.finish(); op != opFailure {
		t.Fatalf("repeated finish opcode = %v, want %v", op, opFailure)
	}
}

func TestFinishDoesNotConsumeBytes(t *testing.T) {
	var m machine
	for i := 0; i < len(`123`); i++ {
		m.step("123"[i])
	}

	if op := m.finish(); op != opDocumentEnd {
		t.Fatalf("finish opcode = %v, want %v", op, opDocumentEnd)
	}
	if got := m.consumed; got != 3 {
		t.Fatalf("consumed = %d, want 3", got)
	}
}

func TestFailureOffsetAccumulatesAcrossReset(t *testing.T) {
	var m machine

	for i := 0; i < len(`null`); i++ {
		m.step("null"[i])
	}
	if op := m.finish(); op != opDocumentEnd {
		t.Fatalf("finish opcode = %v, want %v", op, opDocumentEnd)
	}

	m.reset()
	if op := m.finish(); op != opFailure {
		t.Fatalf("finish on empty input = %v, want %v", op, opFailure)
	}
	if m.failure == nil {
		t.Fatal("failure = nil, want a parse error")
	}

	// Positions count bytes consumed by the machine, not bytes in the current
	// document; reset deliberately preserves that count.
	if got := m.failure.at; got != 4 {
		t.Errorf("failure position = %d, want 4", got)
	}
}

func TestInvalidDocuments(t *testing.T) {
	tests := []struct {
		in      string
		at      int64
		problem string
	}{
		// Empty input and truncated documents.
		{``, 0, "unexpected end of JSON input"},
		{` `, 1, "unexpected end of JSON input"},
		{`{`, 1, "unexpected end of JSON input"},
		{`[`, 1, "unexpected end of JSON input"},
		{`{"a"`, 4, "unexpected end of JSON input"},
		{`{"a":`, 5, "unexpected end of JSON input"},
		{`[1`, 2, "unexpected end of JSON input"},
		{`"`, 1, "unexpected end of JSON input"},
		{`"abc`, 4, "unexpected end of JSON input"},

		// Truncated or mistyped literals.
		{`tru`, 3, "in literal true (expecting 'e')"},
		{`trux`, 4, "in literal true (expecting 'e')"},
		{`tx`, 2, "in literal true (expecting 'r')"},
		{`trx`, 3, "in literal true (expecting 'u')"},
		{`fals`, 4, "in literal false (expecting 'e')"},
		{`falsy`, 5, "in literal false (expecting 'e')"},
		{`fx`, 2, "in literal false (expecting 'a')"},
		{`falx`, 4, "in literal false (expecting 's')"},
		{`nul`, 3, "in literal null (expecting 'l')"},
		{`nulx`, 4, "in literal null (expecting 'l')"},
		{`nx`, 2, "in literal null (expecting 'u')"},
		{`nuX`, 3, "in literal null (expecting 'l')"},

		// Invalid numbers.
		{`-`, 1, "in numeric literal"},
		{`--1`, 2, "in numeric literal"},
		{`01`, 2, "in numeric literal"},
		{`-01`, 3, "in numeric literal"},
		{`1.`, 2, "after decimal point in numeric literal"},
		{`1.e1`, 3, "after decimal point in numeric literal"},
		{`1e`, 2, "in exponent of numeric literal"},
		{`1e+`, 3, "in exponent of numeric literal"},
		{`1e++1`, 4, "in exponent of numeric literal"},
		{`1e+-1`, 4, "in exponent of numeric literal"},

		// Numbers may contain at most one fraction and one exponent.
		{`1.2.3`, 4, "after top-level value"},
		{`1e1e2`, 4, "after top-level value"},
		{`1e1.5`, 4, "after top-level value"},
		{`0e1e2`, 4, "after top-level value"},
		{`1e+1e2`, 5, "after top-level value"},
		{`-1.5E2.3`, 7, "after top-level value"},
		{`[1.2.3]`, 5, "after array value"},
		{`{"a":1.5.6}`, 9, "after object value"},

		// Bytes that cannot begin a value.
		{`+1`, 1, "looking for beginning of value"},
		{`.5`, 1, "looking for beginning of value"},
		{`NaN`, 1, "looking for beginning of value"},
		{`'a'`, 1, "looking for beginning of value"},

		// Object structure.
		{`{a:1}`, 2, "looking for beginning of object key string"},
		{`{,}`, 2, "looking for beginning of object key string"},
		{`{]`, 2, "looking for beginning of object key string"},
		{`{"a" 1}`, 6, "after object key"},
		{`{"a":1 "b":2}`, 8, "after object value"},
		{`{"a":1,}`, 8, "looking for beginning of object key string"},
		{`{"a":1,,"b":2}`, 8, "looking for beginning of object key string"},
		{`{"a":1]`, 7, "after object value"},
		{`{"a":1;}`, 7, "after object value"},
		{`{"a":}`, 6, "looking for beginning of value"},

		// Array structure.
		{`[1 2]`, 4, "after array value"},
		{`[1,]`, 4, "looking for beginning of value"},
		{`[1,,2]`, 4, "looking for beginning of value"},
		{`[,]`, 2, "looking for beginning of value"},
		{`[1}`, 3, "after array value"},
		{`[}`, 2, "looking for beginning of value"},

		// Bytes after a complete top-level value.
		{`{}x`, 3, "after top-level value"},
		{`{} {}`, 4, "after top-level value"},
		{`1 2`, 3, "after top-level value"},
		{`"a""b"`, 4, "after top-level value"},
		{`[1]]`, 4, "after top-level value"},

		// Strings and escapes.
		{`"\q"`, 3, "in string escape code"},
		{`"\u"`, 4, `in \u hexadecimal character escape`},
		{`"\u00"`, 6, `in \u hexadecimal character escape`},
		{`"\u12g"`, 6, `in \u hexadecimal character escape`},
		{`"\u12`, 5, `in \u hexadecimal character escape`},
		{`"\u0g00"`, 5, `in \u hexadecimal character escape`},
		{`"\u000g"`, 7, `in \u hexadecimal character escape`},
		{"\"a\nb\"", 3, "in string literal"},
		{"\"a\tb\"", 3, "in string literal"},
		{"\"a\x00b\"", 3, `invalid character '\x00' in string literal`},

		// Incomplete key position after a comma.
		{"{\"a\":1, ", 8, "unexpected end of JSON input"},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.in), func(t *testing.T) {
			ops, err := drive(t, tt.in)
			if err == nil {
				t.Fatalf("got no error, want %q at position %d", tt.problem, tt.at)
			}
			if last := ops[len(ops)-1]; last != opFailure {
				t.Errorf("last opcode = %v, want %v", last, opFailure)
			}
			requireParseError(t, err, tt.at, tt.problem)
		})
	}
}

func TestErrorMessage(t *testing.T) {
	_, err := drive(t, `{"a":1]`)
	if err == nil {
		t.Fatal("got no error")
	}

	const want = `invalid character ']' after object value`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestOpcodeString(t *testing.T) {
	tests := []struct {
		op   opcode
		want string
	}{
		{opContinue, "continue"},
		{opLiteralStart, "literalStart"},
		{opObjectStart, "objectStart"},
		{opObjectEnd, "objectEnd"},
		{opArrayStart, "arrayStart"},
		{opArrayEnd, "arrayEnd"},
		{opDocumentEnd, "documentEnd"},
		{opFailure, "failure"},
		{opcode(99), "opcode(99)"},
	}

	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("opcode(%d).String() = %q, want %q", int(tt.op), got, tt.want)
		}
	}
}
