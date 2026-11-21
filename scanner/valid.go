// Package scanner validates JSON syntax.
//
// The package exposes a single function, Valid, whose name and signature match
// encoding/json.Valid. The implementation is a from-scratch byte-at-a-time
// state machine: it does not use encoding/json, does not build values, and
// does not validate UTF-8 or surrogate pairs.
//
// Unlike encoding/json, Valid accepts any JSON value at the top level and
// intentionally applies no nesting depth limit.
package scanner

import "sync"

// machinePool recycles machines so that steady-state validation does not
// allocate. Like encoding/json, the pool bounds retained memory by dropping
// oversized context stacks before returning a machine.
var machinePool = sync.Pool{
	New: func() any { return new(machine) },
}

// Valid reports whether data is a syntactically valid JSON text.
//
// Valid ignores surrounding whitespace, performs a single pass over data, and
// is safe for concurrent use. Steady-state calls do not allocate for typical
// documents. Input that is not valid UTF-8 may still be reported as valid
// because only JSON syntax is checked.
func Valid(data []byte) bool {
	m := machinePool.Get().(*machine)
	m.reset()

	ok := true
	for _, c := range data {
		if m.step(c) == opFailure {
			ok = false
			break
		}
	}
	if ok {
		ok = m.finish() != opFailure
	}

	m.release()
	return ok
}

// release returns m to the pool, dropping an oversized context stack.
func (m *machine) release() {
	if cap(m.stack) > 1024 {
		m.stack = nil
	}
	machinePool.Put(m)
}
