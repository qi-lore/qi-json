// Package xtypes provides cross-package named scalar types for the code
// generator's fixture: the generator must resolve a selector type in another
// package to decide how to decode it.
package xtypes

// Label is a named string decoded through a cross-package field.
type Label string

// Flag is a named bool decoded through a cross-package field.
type Flag bool
