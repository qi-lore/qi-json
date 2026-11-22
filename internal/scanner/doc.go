// Package scanner is the internal syntax engine of qi-json: Valid, the syntax
// validator, and the Tokenizer the semantic layer in package codec consumes.
// The module root package qijson re-exports this package's whole surface —
// Valid, the Kind taxonomy and the Tokenizer — so a caller reaches it through
// github.com/qi-lore/qi-json. This package is internal and not importable by
// external code. Valid's name and signature match encoding/json.Valid.
//
// Valid reports whether a byte slice is a syntactically valid JSON text. It is
// a single pass over the slice: it walks with an index, scans strings, numbers,
// whitespace and literals in bulk, and keeps an explicit stack of open
// containers. It does not use encoding/json, does not build values, and does not
// validate UTF-8 or surrogate pairs. Unlike encoding/json, Valid accepts any
// JSON value at the top level and intentionally applies no nesting depth limit.
//
//	ok := scanner.Valid([]byte(`{"name":"qi-json","port":8080}`))
//
// A Tokenizer turns a document into a stream of typed tokens without building
// values. It is pull-based: Peek reports the kind of the next value token
// without consuming it, and the Read*/Begin*/End* methods consume it. Whitespace,
// commas and the ':' after an object key are consumed automatically. Inside an
// object the stream is key, value, key, value, ...; inside an array it is value,
// value, ...; Peek reports KindEnd at the closing brace or bracket. At the top
// level it reports KindEnd once the single top-level value and its trailing
// whitespace are consumed.
//
//	tok := scanner.NewTokenizer(data)
//	if err := tok.BeginArray(); err != nil {
//	    return err
//	}
//	for {
//	    k, err := tok.Peek()
//	    if err != nil {
//	        return err
//	    }
//	    if k == scanner.KindEnd {
//	        break
//	    }
//	    if err := tok.Skip(); err != nil {
//	        return err
//	    }
//	}
//	if err := tok.EndArray(); err != nil {
//	    return err
//	}
//
// Byte slices returned by ReadString, ReadNumber and ReadRaw alias the input and
// must not be modified. Valid is safe for concurrent use; a Tokenizer carries
// state and must not be copied or used concurrently.
//
// The exported surface — Valid plus the tokenizer (Kind and Tokenizer) — is
// re-exported in full by package qijson, which is the only import path a caller
// needs. The implementation stays allocation-free for documents nesting at most
// nestingInline levels.
package scanner
