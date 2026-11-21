// Package scanner is the syntax layer of qi-json. It exports Valid, the syntax
// validator, and the Tokenizer that the semantic layer in qi-json/marshal
// consumes. Valid's name and signature match encoding/json.Valid.
//
// The implementation is a from-scratch single-pass walk over the byte slice:
// it walks with an index, scans strings, numbers, whitespace and literals in
// bulk, and keeps an explicit stack of open containers. It does not use
// encoding/json, does not build values, and does not validate UTF-8 or surrogate
// pairs.
//
// Unlike encoding/json, Valid accepts any JSON value at the top level and
// intentionally applies no nesting depth limit.
//
// The exported surface — Valid plus the tokenizer (Kind and Tokenizer, in
// token.go) — is sanctioned by docs/constitution.md Article VI; the decoder in
// qi-json/marshal consumes the tokenizer.
package scanner

// Valid reports whether data is a syntactically valid JSON text.
//
// Valid ignores surrounding whitespace, performs a single pass over data, and is
// safe for concurrent use. Steady-state calls do not allocate for documents that
// nest no deeper than nestingInline levels. Input that is not valid UTF-8 may
// still be reported as valid because only JSON syntax is checked.
func Valid(data []byte) bool {
	_, failed := validate(data)
	return !failed
}

// validate scans data and returns the first syntax error together with whether
// the input is invalid. The error is returned by value so that the validator's
// inline stack does not escape to the heap.
func validate(data []byte) (parseError, bool) {
	var inline [nestingInline]container
	v := validator{data: data, stack: inline[:nestingInline]}
	if v.run() {
		return parseError{}, false
	}
	return *v.failure, true
}
