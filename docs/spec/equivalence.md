# Normalized Source Equivalence

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

No single comparison proves complete semantic equivalence. Gox accepts
formatted output only when the following independent checks agree.

## Parse And Language Version

Input and output MUST parse without diagnostics under the formatter's declared
toolchain and source-language policy. A successful parse is necessary but not
sufficient.

## Lexical Accounting

After excluding whitespace, comments, inserted semicolons, removable explicit
ordinary-statement semicolons, and formatter-generated required trailing
commas, the ordered token kinds and source values MUST match. Identifiers,
keywords, operators, delimiters, and literal decoded values MUST remain
identical.

Initial Gox formatting preserves literal spelling exactly. Any later literal
normalization requires an explicit safe-normalization decision and separate
token-accounting allowance.

## Normalized AST

AST comparison MUST ignore physical positions and parser bookkeeping only. It
MUST preserve declaration, statement, expression, and type structure; operand
order; identifiers; literal values; and parentheses until a separate decision
proves a safe normalization. Comment fields are excluded from structural AST
comparison because they are validated independently.

The normalizer MAY account for AST-shape effects caused solely by documented
required trailing commas or removed ordinary separators. It MUST NOT erase a
difference merely because both trees type-check.

## Comments And Directives

Every input comment and directive identity MUST map to exactly one output item
with identical raw text. Relative ordering and the documented ownership
boundary—package, declaration, field, statement, operand, list element, case,
build constraint, or file—MUST remain unchanged. No output item may appear
without input provenance unless a separately documented fixer created it.

## Additional Evidence

Owned multi-file fixtures SHOULD type-check or compile after formatting.
Supported corpus runs SHOULD compare pinned gofmt, go/format, and gofumpt output
where their contracts overlap. These checks can discover defects but MUST NOT
replace lexical, AST, comment, or directive accounting.

## Known Blind Spots

The initial model does not prove runtime equivalence for unsafe refactors,
reflection on source positions, external tools that consume exact comments,
compiler bugs, or behavior outside the declared toolchain and build selection.
Those blind spots are why `fmt` is restricted to layout and why lint fixes have
rule-specific safety contracts.
