# Formatter Specification

The user-facing dialect and examples are published in
[`../formatter-rules.md`](../formatter-rules.md). This specification remains the
normative contract.

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Contract

The formatter owns whitespace and canonical layout only. It MUST NOT perform
semantic refactors, missing-import insertion, or unused-import removal. It MUST
lower a valid immutable source unit into the language-neutral document IR and
MUST render without mutating the source unit.

Import declarations retain their groups, spec order, aliases, comments, and
literal spelling in the initial formatter. Sorting, grouping, insertion, and
removal are outside `fmt`. A later import organizer MAY exist only as an
explicit safe lint fix or code action with separate cgo, blank-import,
dot-import, initialization-order, and comment evidence.

For accepted valid input, output MUST reparse, be byte-idempotent, satisfy the
normalized equivalence model, preserve literal and directive identity under
documented normalizations, preserve comment ownership and order, and remain
deterministic for the same binary, configuration, source, language version,
and build selection.

## Canonical Layout

Every eligible construct SHOULD have one flat form and one canonical broken
form. The initial implementation MUST cover package clauses, imports without
organization, declarations, functions and signatures, blocks, statements,
simple statements, initializer clauses, calls, lists, composite and function
literals, unary and binary expressions by precedence, selectors, indexing,
slicing, assertions, and Go types before Phase 1 exits.

Blocks and ordinary statement lists MUST use hard line boundaries. Explicit
semicolons between ordinary statements MUST lower to hard lines. Semicolons in
`if`, classic `for`, and switch grammar MUST remain in their clause.
Between adjacent statement groups, a physical blank-line gap MUST remain one
blank line. Multiple blank lines MUST collapse to one. A gap after an explicit
ordinary-statement semicolon MUST NOT create a blank group, and leading or
trailing blank lines immediately inside braces MUST NOT be retained. A blank
gap before a boundary comment group belongs to that group; comment-owned gaps
after the group retain the comment-placement behavior below.
An explicit empty statement MUST retain a visible `;`, including after a label,
because removing a labeled empty statement can attach the label to a different
following statement. An implicit empty statement before a closing brace MUST
remain semicolon-free.

Binary operators and type-union operators MUST remain on the preceding line
when a chain breaks. Selector dots MUST remain after the preceding selector
operand. Unary expressions, increment/decrement statements, indexing, slicing,
and other constructs without a grammar-safe broken form MUST remain atomic or
use a separately proven layout.

A line comment after a binary operator MUST remain after that operator and
force the following operand onto the next continuation line. The comment MUST
NOT move before the operator or cause the operator to begin a new line. When
one boundary forces a same-precedence chain to break, every other breakable
boundary in that chain uses the canonical broken form.

An uncommented control-flow keyword MUST remain with its first operand or
clause. Width pressure in `if` and condition-only `for` headers belongs to the
condition's own groups. Range headers MAY break after `range`, and initialized
`if` or switch headers MAY break after their grammar semicolon, but the
formatter MUST NOT strand `if`, `for`, `switch`, or `case` on a line solely to
meet width. An otherwise atomic header MAY remain over width.

An uncommented single method receiver MUST remain flat. A later break in the
method name, type parameters, parameters, or results MUST NOT force the
receiver itself to break after `func`. Comments between `func` and the receiver
or inside the receiver MAY require the receiver's comment-preserving broken
form. An otherwise atomic receiver MAY remain over width.

An uncommented single logical type parameter on a function declaration MUST
remain flat when the following ordinary parameter or result list breaks.
Comments before or inside the type-parameter list MAY require its
comment-preserving broken form. A list with multiple logical type parameters
retains the canonical flat and broken comma-list forms.

Ordinary assignments MUST keep the assignment operator and the first
right-hand expression on the same line. Width pressure inside the right-hand
expression belongs to that expression's canonical groups, so a broken call
keeps its callee beside `:=` or `=` and breaks only its argument list. Glippy does
not introduce a generic assignment-operator break; an otherwise atomic
assignment MAY remain over width. Grammar contexts with a separately specified
layout MAY force the right-hand side onto the following line. A line comment
after the operator MUST force the right-hand side onto the following line.
An ordinary selector-chain target MUST decide whether it fits using the target
through the assignment operator, without width pressure from the right-hand
side. A target that does not fit that boundary retains the selector-chain broken
form. Communication-clause assignments retain their grammar-specific coupled
layout.

Multi-selector chains share one layout group. When that group breaks, every
selector dot remains on the preceding line and every following selector uses
one continuation indentation level. A terminal call's argument list makes an
independent layout decision: when the selector callee and opening delimiter
fit, broken arguments MUST NOT force the callee to break. The opening delimiter
still participates in the selector fit decision, and an empty terminal call
counts both delimiters. A single selector remains atomic so a broken call
argument list does not force an unrelated selector break.

Broken comma-delimited calls, composite literals, parameter lists, result
lists, type-argument lists, and corresponding grammar constructs MUST emit a
trailing comma whenever a newline before the closing delimiter requires one.
The formatter MUST NOT generalize that rule to delimiters whose grammar does
not permit it.

## Comments And Directives

Initial releases MUST preserve comment text byte-for-byte and MAY normalize
only placement and indentation proven not to change ownership. Line comments
act as line suffixes and force a line boundary. Required punctuation MUST be
emitted before a trailing comment. An ambiguous comment between list elements
or operands SHOULD force a stable broken form rather than migrate across a
token boundary.

Build constraints, compiler directives, cgo preambles, line directives,
generated markers, and suppression directives MUST retain exact text, order,
and required adjacency. Comment or directive accounting failure MUST reject the
formatted result.

External `//nolint` comments MUST retain their physical-line diagnostic
ownership. If canonical breaking would move the governed token to another
line, the formatter MUST preserve the affected statement line verbatim. A
`//nolint` comment immediately after an `if` opening brace MUST keep the header
through that brace verbatim while the block body remains formatter owned. An
otherwise unsupported ownership change MUST reject formatting. These
ownership-preserving layouts MAY exceed configured width.

## Physical Output

Formatter-owned structural line endings MUST use LF regardless of whether the
input uses LF, CRLF, or mixed endings. Complete files and fragments MUST end in
exactly one formatter-owned LF. An accepted input byte-order mark MUST remain at
byte zero. Embedded line endings inside preserved raw literals and multiline
comments remain part of those verbatim source tokens and MUST NOT be rewritten
as structural whitespace.

## Width And Rendering

Width is measured in Unicode code points. Tabs advance to configured tab stops;
the default is 8. Logical Go indentation emits tabs. Over-width unbreakable
tokens remain unchanged. The bounded renderer and its conservative fit-budget
behavior are defined in
[`../decisions/0002-document-ir-and-renderer.md`](../decisions/0002-document-ir-and-renderer.md).

## Validation

Before returning success, the formatter MUST:

1. parse the complete output with the declared toolchain;
2. format it again and compare exact bytes;
3. compare normalized syntax and token/literal accounting;
4. account for every comment and directive identity and relative boundary;
5. validate generated commas and removed explicit semicolons;
6. verify documented width opportunities and exceptions; and
7. report a deterministic outcome without writing in stdout or check modes.

Gofmt fixed-point compatibility is required only for corpus classes recorded as
compatible. Import order, literal spelling, redundant parentheses, alignment,
and intentional width-aware layout have documented divergences; see
[`../decisions/0004-gofmt-compatibility.md`](../decisions/0004-gofmt-compatibility.md).
