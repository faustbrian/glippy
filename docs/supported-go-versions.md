# Supported Go Versions

Gox supports Go 1.25 and Go 1.26 source. The source language
is normalized to its language family, so directives such as `go 1.26.5` select
Go 1.26. Source versions older than Go 1.25 or newer than Go 1.26 fail before
formatting, analysis, or writes begin.

For a source path, Gox selects the nearest containing `go.mod` up to the
discovered project root. If no module owns the path, it uses the project-root
`go.work`. If the selected file has no `go` directive, or neither file exists,
Gox defaults to Go 1.26. A malformed selected module or workspace file is an
error. `--stdin-filepath` uses the same resolution without reading or writing
the named source file.

Formatting and syntax-only linting validate grammar with the Go 1.26 frontend;
they do not type-check source merely to diagnose version-gated semantics. For
example, a Go 1.25 module containing the Go 1.26 `new(expression)` form still
parses as an ordinary call-shaped expression and is formatted without claiming
that the program type-checks. Typed linting delegates the module language
version to `go/packages` and reports the resulting prerequisite error.

Rule metadata declares the earliest supported language family for each rule.
The scheduler omits a rule when the selected source version is older than that
minimum. The current built-in rules support both Go 1.25 and Go 1.26.

Gox v0.1.0 release archives were built with Go 1.26.5 for macOS and Linux on
amd64 and arm64. They do not require a separately installed Go runtime.
Installing from source requires the toolchain selected by the module's
`go 1.26.0` directive; the Go command may choose a newer compatible toolchain
according to the user's toolchain policy. Windows and other operating systems
are unsupported.

Newer Go source is rejected until Gox is rebuilt with a frontend that
understands it and the formatter, lint, corpus, and compatibility gates for
that language family pass. Adding a newer maximum does not automatically drop
the current minimum; supported-version changes are release compatibility
changes.

This source-language range is distinct from the lifetime of a published Gox
binary. Which Gox release receives fixes is defined by the
[product support policy](support-policy.md).
