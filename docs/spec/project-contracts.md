# Project Semantic Contract Specification

This document defines Glippy project semantic contract schema version 1. These
contracts describe application and dependency APIs whose behavior cannot be
derived from Go export data alone. They are data inputs to built-in analysis,
not executable plugins or replacement type declarations.

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Configuration And Files

Projects MAY select up to 32 explicit contract files:

```toml
version = 1

[analysis]
contract-files = [
  ".glippy-contracts.toml",
  "internal/analysis/library-contracts.toml",
]
```

Every configured path MUST be a normalized, portable, project-relative path.
Absolute paths, `..`, backslashes, duplicate paths, missing files, non-regular
files, final symlinks, and paths that escape through a symlinked directory MUST
fail configuration loading. Glippy resolves paths against the discovered
project root, or against the explicit configuration directory when no project
root exists.

File order MUST NOT affect semantics. Glippy sorts the configured paths and
parses their exact immutable bytes before command execution. One file is
limited to 1 MiB, the complete snapshot is limited to 4 MiB, and the snapshot
is limited to 4,096 function declarations. Configuration and contract contents
MUST contribute to canonical configuration and persistent-cache identity.

Every contract file MUST declare its own schema version:

```toml
version = 1
```

Unknown fields, duplicate TOML keys, missing versions, unsupported versions,
invalid types, and malformed values MUST fail. Decoder failures carry their
physical TOML location. Function-level semantic failures carry the location of
their `[[functions]]` declaration where that marker can be identified safely.

## Function Identity

Each declaration targets one exact package-qualified function or declared
method:

```toml
[[functions]]
symbol = "example.com/project/log.Fatal"
noreturn = true

[[functions]]
symbol = "example.com/project/resource.Handle.CloseAndStop"
noreturn = true
```

Function spelling is `package/path.Function`. Method spelling is
`package/path.Type.Method`; pointer and value receivers use the same declared
type name. Glippy MUST resolve identities through loaded `go/types` packages
and objects. It MUST NOT match import aliases, suffixes, bare names, promoted
method names, or naming conventions such as `Fatal`, `Must`, or `Close`.

A declaration for a package present in the loaded type graph MUST resolve to an
exact function or declared method and MUST validate against every loaded test
variant of that package. A missing object or incompatible signature MUST fail
analysis. A declaration for a package absent from a partial package load
remains deferred; this permits one repository contract snapshot to serve file,
package, LSP, and whole-repository selections without forcing unrelated
package loads.

Duplicate declarations for one symbol are prohibited, including identical
declarations in different files. Every declaration MUST assert at least one
effect.

## Schema Version 1 Effects

```toml
version = 1

[[functions]]
symbol = "example.com/project/openResource"
must-use = [0, 1]
blocking = true
nil-error = [
  { value = 0, error = 1, when-error-nil = "non-nil", when-error-non-nil = "nil" },
]

[[functions]]
symbol = "example.com/project/consume"
closes = [0]
takes-ownership = [0]

[[functions]]
symbol = "example.com/project/finishTransaction"
completes-transaction = [0]

[[functions]]
symbol = "example.com/project/invokeCancel"
invokes-cancellation = [0]

[[functions]]
symbol = "example.com/project/view"
returns-alias = [{ result = 0, argument = 0 }]
```

| Field | Contract |
| --- | --- |
| `noreturn` | The call has no normal continuation when `true` |
| `must-use` | Each listed zero-based result must be consumed |
| `closes` | Every normally returning path closes the listed zero-based parameter |
| `takes-ownership` | Every normally returning path transfers ownership of the listed parameter away from the caller |
| `completes-transaction` | Every normally returning path commits or rolls back the listed parameter |
| `invokes-cancellation` | Every normally returning path invokes the listed cancellation parameter |
| `blocking` | The call may block when `true` |
| `nil-error` | Relates one nil-capable result to one exact built-in-error result |
| `returns-alias` | One result aliases the listed parameter rather than creating independent ownership |

All indexes are zero-based signature indexes. A parameter index excludes the
method receiver. Index lists MUST contain at most 64 unique non-negative
values. `nil-error` and `returns-alias` lists MUST contain at most 64 unique
relationships. Glippy sorts all lists and relationships before constructing
their semantic identity.

`must-use` indexes MUST exist in the function result tuple. Parameter-effect
indexes MUST exist in the parameter tuple. A `nil-error.error` result MUST be
assignable to the built-in `error` interface, its `value` result MUST be
nil-capable, and the two result indexes MUST differ. Each conditional state MAY
be omitted for unknown or MUST be exactly `"nil"` or `"non-nil"`; at least one
state is REQUIRED. Alias indexes MUST exist and their result and parameter
types MUST be assignment-compatible.

`noreturn = false` and `blocking = false` do not assert an effect. Glippy MUST
NOT infer unstated effects from a configured symbol or combine separate
declarations for the same symbol.

## Analysis Semantics

Contract parsing is part of strict configuration loading. Symbol and signature
resolution is demand-driven: lexical and syntax-only lint MUST NOT start typed
package loading merely because contract files are configured. When an enabled
CFG or SSA rule declares effect-fact requirements, Glippy resolves the same
immutable contract snapshot against the selected type graph.

Configured effects seed the shared native effect set before source-derived
same-module inference. An exact configured parameter or returned-state record
is authoritative for that record; source-derived analysis MAY fill only
unstated records. No-return, must-use, blocking, and alias records are additive
exact facts. External dependency contracts resolve through export type
information and MUST NOT require dependency source loading solely for contract
resolution.

The base `[analysis]` build selection and the LSP use the same contract
snapshot. Every `[[analysis.targets]]` package run uses that snapshot while
validating the signature selected by its target. Identical target findings
retain the ordinary deterministic target-union behavior. Fix modes retain the
existing target-matrix prohibition.

Contract changes MUST invalidate affected persistent native results. The
`native-effects-v4` component binds no-return, parameter, returned-state,
must-use, blocking, and alias facts through stable package-qualified function
identities. Contract source paths and process-local `types.Object` pointers
MUST NOT become semantic identities.

## Safety And Extension Boundary

Contract files are untrusted data. Glippy MUST NOT execute commands, load Go
plugins, evaluate Go expressions, contact a network, or mutate source while
parsing or resolving them. Ordinary package loading retains its documented
offline, module-mode, cancellation, resource, and target policies.

Incorrect contracts can hide a defect or create a false diagnostic. Projects
SHOULD review contract changes as analysis-policy changes and SHOULD pair
non-obvious declarations with API documentation or behavioral tests. Glippy
validates schema and signature compatibility but cannot prove a user assertion
about runtime behavior.

Runtime Go plugin APIs and arbitrary executable configuration remain outside
this contract. A future statically compiled analyzer extension MAY consume the
same immutable facts only after a separate compatibility decision.
