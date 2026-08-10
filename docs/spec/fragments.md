# Standard-Input Fragment Contract

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

Complete files remain the default stdin mode. Fragments require an explicit
`--fragment=declaration`, `--fragment=statement`, or `--fragment=expression`.
The flag is valid only for `fmt` reading stdin and writing stdout. It is invalid
with filesystem inputs, `--write`, `--check`, linting, or fixing. Gox MUST NOT
guess among fragment kinds after a parse error.

## Parsing

Fragments are parsed through synthetic valid files using fixed wrappers:

- declaration: after `package goxfragment`;
- statement: inside `func goxfragment() { ... }`; and
- expression: as the parenthesized initializer of a package variable.

The declaration and statement wrappers place user bytes after a synthetic
newline. The expression wrapper places the opening parenthesis before the
user's syntax-bearing bytes and a synthetic non-line-breaking separator plus
the closing parenthesis after the last non-whitespace user byte. Physical
trailing spaces and line endings remain in the fragment ledger but stay outside
that parenthesized parse boundary so Go semicolon insertion cannot turn an
ordinary final newline into a synthetic syntax error. The separator gives every
wrapper token a position outside the mapped user range.

Wrapper bytes and token positions MUST remain distinguishable from user bytes.
Diagnostics MUST map back to physical fragment byte offsets and MUST NOT expose
synthetic filenames, identifiers, or lines as if they were user source.

The parser MUST select the intended wrapped AST boundary exactly: declaration
list, function body statement list, or initializer expression. Content that
escapes that boundary or relies on wrapper declarations MUST be rejected.
Statement fragments that use the wrapper identifier as a keyed composite
literal element are ambiguous without type information: the key may be a
user-owned struct field or a map key that references the wrapper function. Gox
MUST reject that keyed use unless the key resolves to a user-owned local
declaration or syntax proves the composite is a user-owned struct type. A local
declaration that shadows the wrapper identifier remains valid.

## Formatting

Lowering operates on the selected user boundary and emits only that boundary;
Gox MUST NOT format a complete synthetic file and slice output by guessed line
counts. Leading and trailing fragment whitespace follow the fragment-kind
policy: surrounding whitespace is not preserved, while whitespace inside the
selected syntax and comment boundary is canonicalized by the ordinary
formatter. Successful output ends with exactly one newline, including empty
declaration and statement fragments.

Declaration and statement fragments MAY contain multiple items. Explicit
ordinary-statement semicolons in a statement fragment become hard lines.
Expression fragments use the same precedence, parentheses, call, literal, and
width rules as expressions in complete files.

Build constraints, package documentation, file-wide generated markers, cgo
preambles, `//line`, and directives whose validity depends on file placement
are invalid in fragment mode. Ordinary comments are allowed only when their
ownership remains inside the selected boundary. Parse, comment-accounting, or
directive errors MUST return diagnostics without partial formatted output.

`--stdin-filepath` MAY supply project configuration and source language
context. It MUST NOT change fragment byte identity or authorize filesystem
access beyond read-only configuration discovery.
