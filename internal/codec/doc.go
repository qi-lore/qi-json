// Package codec is the implementation of the qijson semantic layer: a
// reflection-free JSON decoder and encoder built on the syntax engine in
// qi-json/scanner.
//
// It is internal. The public contract — types, functions, sentinels and their
// documentation — lives in package qijson at the module root, which re-exports
// this package through type aliases and wrapper functions. Nothing here is a
// compatibility promise; only the facade is.
//
// The package MUST NOT import encoding/json and MUST NOT use reflect or unsafe.
package codec
