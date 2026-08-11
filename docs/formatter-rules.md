# Formatter Rules

This document describes the current formatter dialect. Gox remains a
pre-release working name, and formatter output is not yet covered by a stable
release compatibility promise.

The formatter changes whitespace and canonical layout only. It does not add or
remove imports, rename identifiers, simplify expressions, or perform semantic
refactors. Every successful result must parse, preserve the normalized source
contract, and be byte-idempotent.

## Width And Indentation

The default line width is 100 Unicode code points. Tabs advance to tab stops;
the default tab width is 8. Go indentation is emitted as tabs. Configuration
can change `format.line-width` and `format.tab-width`, but the formatter has no
per-rule whitespace switches.

Width is a layout target where Go has a proven break opportunity, not a hard
error limit. Long identifiers, strings, raw literals, unary expressions,
indexing, slicing, type assertions, and increment or decrement statements may
remain over width rather than receive a grammar-risking break.

Groups use an exact-fit boundary: a flat form is retained when it fits, and the
same construct uses its canonical broken form one column below that boundary.

## Files, Declarations, And Blocks

- Package clauses are followed by the canonical declaration separation.
- Import groups, spec order, aliases, comments, and literal spelling are
  preserved. Gox does not organize imports.
- Constant, variable, type, function, method, field, and signature structure is
  retained while spacing and eligible lists are canonicalized.
- Blocks always place ordinary statements on hard line boundaries.
- Ordinary explicit semicolons become line breaks. Grammar semicolons in `if`,
  classic `for`, and switch headers remain in those headers.
- Explicit empty statements remain visible when their removal could change a
  label's target. Implicit empty statements before `}` remain invisible.

At width 100, the compressed fixture
[`compressed-if.input`](../testdata/format/motivating/compressed-if.input)
formats to:

```go
func check() {
	if _, err := client.Discover(nil); !errors.Is(err, ErrContextRequired) {
		t.Fatal(err)
	}
}
```

Ordinary statement semicolons become separate statements:

```go
func run() {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result := work(ctx)
	_ = result
}
```

The complete expected files are exercised by
[`TestFormatExpandsMotivatingHostileGo`](../internal/format/format_test.go).

## Control Flow

`if`, classic and range `for`, `switch`, type switch, `select`, case clauses,
communication clauses, labels, branches, `go`, `defer`, and `return` have
structural layouts. An uncommented control keyword stays with its first operand
or clause even when that atomic header exceeds width.

When a header has a grammar-owned break, Gox uses it. An initialized `if` or
switch may break after its semicolon, classic `for` clauses may break after
their semicolons, and range loops may break after `range`. Case expression lists
break one expression per continuation line.

For example, an initialized `if` is flat at width 60 and breaks at width 59:

```go
if current := initialValue;
	current != expectedVal {
	work()
}
```

## Delimited Lists

Calls, composite literals, receiver and parameter lists, result lists, type
parameters, type arguments, fields, and corresponding Go type lists remain flat
when they fit. Their canonical broken form places one logical element per line.
When Go requires a trailing comma before a newline and closing delimiter, Gox
emits it.

At width 30, a long call becomes:

```go
result, err := client.executeContent(
	ctx,
	OperationInfo,
	http.MethodGet,
	"/",
	nil,
	"application/json",
	200,
)
```

An uncommented single method receiver stays flat even when a later signature
list breaks. Comments inside a receiver or a noncanonical receiver list may
require the receiver's comment-preserving broken form.

## Binary Expressions And Type Unions

Binary chains are grouped by precedence. When a chain breaks, its operators
remain on the preceding lines so Go's semicolon insertion cannot terminate the
expression early:

```go
return foo &&
	bar &&
	baz &&
	somethingReallyLong
```

The example is the width-24 output in
[`boolean-chain.golden`](../testdata/format/motivating/boolean-chain.golden).
Type unions use the same trailing-operator rule for `|`.

A line comment after an operator remains with that operator and forces the next
operand onto a continuation line:

```go
return first || //nolint:example
	second
```

## Assignments, Selectors, And Calls

An ordinary assignment keeps its operator and first right-hand expression
together. If that expression is a call, the argument list breaks independently
rather than moving the callee away from `:=` or `=`. A line comment after the
assignment operator is the exception and forces the right-hand side to the next
line.

A single selector remains atomic. A selector chain that does not fit breaks
after each dot with one continuation indentation level. The tested chain is
flat at width 49 and broken at width 48:

```go
client.
	WithFirst().
	WithSecond().
	Execute()
```

A terminal call's argument list makes an independent fit decision. Broken
arguments do not force a fitting selector callee to break; when the callee and
opening delimiter also exceed width, the selector chain and arguments each use
their own continuation level.

## Comments And Directives

Comment text is preserved byte-for-byte. Gox may normalize placement and
indentation only within its proven ownership model. Trailing line comments act
as line suffixes, punctuation remains before them, and ambiguous comments force
a stable broken layout rather than migration across an operand, element,
declaration, statement, case, or field boundary.

These source anchors retain their text, order, ownership, and required
adjacency:

- build constraints and legacy `+build` lines;
- generated-file markers;
- cgo preambles;
- `//go:embed`, `//go:generate`, `//go:linkname`, and compiler directives;
- `//line` directives; and
- Gox suppression directives.

Formatting is rejected if comment or directive accounting cannot prove one
valid owner for every input comment.

## Preserved Source Choices

Gox deliberately preserves several choices that gofmt may normalize:

- import spec order;
- numeric literal spelling;
- redundant parentheses;
- explicit versus implicit empty-statement spelling; and
- structural indentation without gofmt-style tabular alignment.

These choices, plus width-aware layouts that gofmt flattens, mean Gox is not a
product-wide gofmt fixed point. Repositories must follow the
[formatter migration guide](migration-from-go-formatters.md) and use one
formatter authority.

## Physical Output And Refusal

Formatter-owned line endings use LF, and a complete output ends in exactly one
LF. A supported byte-order mark remains at byte zero. Line endings inside raw
literals and preserved multiline comments remain part of those tokens.

Gox refuses successful formatting of syntactically invalid complete files or
fragments. Check, diff, and stdout modes do not write. Write mode additionally
refuses generated files and unsafe symlink selections and validates the entire
selected result before replacement.

The normative contracts and implementation evidence remain in the
[formatter specification](spec/formatter.md),
[width decision](decisions/0003-width-and-indentation.md), and
[comment/directive decision](decisions/0009-comment-attachment-and-directives.md).
