# Task registry

The single source of truth for task state in this repository. Every task — done
or planned — appears here exactly once. Plan files under `.opencode/plan/` hold
design rationale and history only; they do not carry task status or a backlog.

## Format

```text
- [X] <PREFIX><NNN> - <SHORT_DESCRIPTION>   completed
- [ ] <PREFIX><NNN> - <SHORT_DESCRIPTION>   planned
```

The prefix names the area (see the legend), the number is zero-padded to three
digits and allocated in completion order within its area. Identifiers are stable:
never renumber, never reuse a retired identifier.

## Legend

| Prefix | Area |
|---|---|
| `S` | Scanner and syntax layer (`scanner/`) |
| `D` | Decoder (`internal/codec/`, decode path) |
| `E` | Encoder (`internal/codec/`, encode path) |
| `G` | Code generator (`cmd/qi-json-gen/`) |
| `F` | Foreign struct fields in the generator |
| `R` | Internal refactor (`internal/codec` extraction) |
| `Q` | Code quality |
| `M` | Module, packaging and governance |
| `P` | Process and repository settings |

## Completed

### Scanner and syntax layer

- [X] S001 - Initial scanner package with `Valid`
- [X] S002 - Scanner rewrite: drop the state machine, keep the contract
- [X] S003 - Remove `goto` labels from the scanner
- [X] S004 - Tokenizer in `scanner` for the decoder
- [X] S005 - Scanner consolidation: move to `internal/scanner` and publish `qijson.Kind`

### Decoder

- [X] D001 - Decoder governance: constitution Article VI amendment
- [X] D002 - Reader primitives and typed reads
- [X] D003 - `Schema`/`Struct` and the field constructors
- [X] D004 - Generic entry points `Unmarshal`/`UnmarshalInto`
- [X] D005 - Container fields `Slice`/`Map`/`Array`
- [X] D006 - Fuzz and differential testing against `Valid`
- [X] D007 - Bulk performance tuning
- [X] D008 - README documentation for the decoder
- [X] D009 - Non-generic `Decode` entry point
- [X] D010 - Decoder UTF-8 policy and pinned semantic defaults

### Encoder

- [X] E001 - Encoder: `Writer`, `Marshal`/`Encode`, `Codec[E]`, `Schema`/`Field` encode

### Code generator

- [X] G001 - Generator skeleton and scalar fields
- [X] G002 - Extended numerics
- [X] G003 - Container fields
- [X] G004 - Errors and edge cases
- [X] G005 - Generator documentation and verification
- [X] G006 - Cross-package named scalar types
- [X] G007 - `-strict` flag
- [X] G008 - Same-package named `string`/`bool` types
- [X] G009 - Document the `qi-json-gen` install paths (go run / tool / go install)
- [X] G010 - Publish `qi-json-gen` release binaries and a Homebrew tap (withdrawn in G011)
- [X] G011 - Remove the release-binary and Homebrew distribution; `go install` is the install path

### Foreign struct fields

- [X] F001 - Delegation constructors `ObjectDecoder`/`ObjectDecoderPtr`/`DecoderCodec`
- [X] F002 - Generator: delegate to a qi-json-aware foreign type
- [X] F003 - Generator: synthesize a schema for a plain foreign struct
- [X] F004 - Generator: `-foreign=pkg.T=ident` escape hatch
- [X] F005 - Foreign-field interactions: pointer, slice, map, `omitempty`
- [X] F006 - Foreign-field documentation and fixtures

### Internal refactor

- [X] R001 - Constitution amendment for the `internal/codec` extraction
- [X] R002 - Snapshot the public API and benchmarks
- [X] R003 - Move the engine into `internal/codec`
- [X] R004 - Root facade `qijson.go`
- [X] R005 - Move the white-box tests next to the engine
- [X] R006 - Facade contract test from outside the package
- [X] R007 - Prove generator drift is zero
- [X] R008 - Documentation update
- [X] R009 - Full verification and benchmark comparison

### Code quality

- [X] Q001 - Pin the error-wrapping policy
- [X] Q002 - Make every library sentinel inspectable
- [X] Q003 - Document ownership, concurrency and lifecycle
- [X] Q004 - Uber Go Style Guide review pass
- [X] Q005 - Keep error messages and doc comments free of project-document references

### Module, packaging and governance

- [X] M001 - Relocate and rename the semantic layer to the module root
- [X] M002 - Go version floor guard and fix
- [X] M003 - Adopt the canonical module path `github.com/qi-lore/qi-json`
- [X] M004 - Git and Code Quality governance (constitution v3.5.0)
- [X] M005 - Release flow governance (constitution v3.6.0)
- [X] M006 - Retire `docs/task.md` and introduce the task registry (constitution v3.9.0)
- [X] M007 - Normalize the plan files' headers to design records

## Planned

Next up: **P001**, protect `main` and `dev` with branch protection and the `verify` check.

- [ ] P001 - Protect `main` and `dev` with branch protection and the `verify` check
- [ ] Q006 - Return `errNilTarget` instead of panicking on a nil `Schema` target
- [ ] Q007 - Make the `Marshal` completeness guard hold against an empty `WriteRaw`
- [ ] Q008 - Return a sentinel instead of panicking on a nil nested `Schema` in `Object`
- [ ] Q009 - Enforce unique field names in `Struct`
