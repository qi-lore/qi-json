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
