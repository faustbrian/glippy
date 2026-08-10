# Source And Trivia Model

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Source Unit

Each physical input MUST produce one immutable run-owned source unit containing
at least:

```go
type SourceFile struct {
	Identity   FileIdentity
	Version    SourceDigest
	Bytes      []byte
	FileSet    *token.FileSet
	TokenFile  *token.File
	AST        *ast.File
	Tokens     []Token
	Trivia     TriviaIndex
	Comments   []Comment
	Directives []Directive
	Metadata   FileMetadata
	Parse      ParseState
}
```

Concrete exported fields are illustrative. Production APIs MUST NOT permit a
consumer to mutate shared bytes, token slices, indexes, or ASTs. A source
version MUST be the digest of the exact bytes that produced positions,
diagnostics, and edits.

## Physical Mapping

All stored ranges MUST be half-open physical byte ranges within one source
identity and version. Edit ordering and conflict checks MUST use normalized
physical path plus byte offset. Logical filenames and locations produced by
`//line` MAY be reported to users but MUST NOT control reads or writes.

The lexical pass MUST record every scanner token with raw start/end offsets,
kind, raw bytes, and whether a semicolon was explicit or inserted. The source
layer MUST derive token gaps from the original bytes and classify whitespace,
comments, BOM, newline sequences, and end-of-file state without discarding raw
bytes.

## Trivia And Directives

Comments MUST have stable identity independent of AST comment-group
normalization. Attachment MUST consider physical adjacency, lines, intervening
tokens and gaps, AST context, delimiters, directive class, and grammar
boundaries. `ast.CommentMap` MAY be an input but MUST NOT be the ownership
authority.

Directive classification MUST cover build constraints, `//go:generate`,
`//go:embed`, compiler directives including linkname, `//line`, cgo preambles,
generated-file markers, and Gox suppression directives. Exact directive bytes
and relative order MUST survive formatting. Their same-line relationship to
both neighboring physical tokens MUST remain stable, as MUST their
adjacent-line or blank-line relationship to the following token. Suppressions
MUST preserve the adjacent-versus-blank distinction on both sides.
Class-specific rules MAY permit canonical adjacent-versus-blank spacing before
other directives. Build constraints SHOULD be parsed with
`go/build/constraint` while retaining the original text independently.

## Parse Policy

The stable formatter MUST parse with comments and without deprecated object
resolution. Any parse diagnostic makes the file diagnostic-only: stdout and
in-place formatted output MUST be refused. Partial ASTs and malformed regions
MAY support precise diagnostics but MUST NOT authorize a write.

Standard-input fragments are represented by the same physical byte identity
plus a separate synthetic-wrapper mapping. Wrapper tokens MUST NOT enter the
user token/trivia ledger. Fragment behavior is defined in
[`fragments.md`](fragments.md).

Typed loading MAY substitute its own concurrent-safe parse hook only if that
hook constructs the same source identity, digest, lexical ledger, and physical
mapping. Synthesized `CompiledGoFiles`, including cgo output, MUST NOT become
editable source units.

## Validation Invariants

For every accepted source unit:

1. token and trivia ranges MUST be ordered, non-overlapping where their kinds
   require it, and within the exact byte length;
2. concatenating raw token and gap slices MUST reconstruct the original bytes;
3. scanner and parser positions MUST map to the same physical token starts;
4. comment and directive identities MUST be unique and accounted for;
5. explicit and inserted semicolons MUST remain distinguishable; and
6. the source digest MUST be checked immediately before applying any edit or
   filesystem replacement.

The evidence and standard-library fidelity limits motivating this model are in
[`../research/go-frontend-audit.md`](../research/go-frontend-audit.md).
