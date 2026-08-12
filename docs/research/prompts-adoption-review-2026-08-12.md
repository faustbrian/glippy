# `pkg/prompts` Adoption Review

## Decision Requested

The maintainer must decide whether the output below is an acceptable canonical
layout for `go-libraries/pkg/prompts`. Approval means:

1. accept the Gox dialect for this module;
2. apply the formatter-only migration as one reviewable change; and
3. replace the module's gofmt-based formatting authority with Gox in CI,
   editor, and contributor workflows.

Approval does not authorize semantic source changes, a Gox release, a tag, or
publication. Rejection should identify an unacceptable layout class or exact
hunk so the formatter contract can be corrected before another migration.

## Review Artifact

The baseline is immutable `go-libraries` revision
`c60393a86b17b070b699805d1b8df99b87a7bfa6`. The formatter selects 77 Go files
and changes 65: 29 production files, 34 test files, and two Go maintenance
scripts. The patch contains 7,625 insertions and 3,599 deletions and has
SHA-256:

```text
14b23895b77a43531833bd11f5ec2428e878e26d98a81ab649d8822e392a0c1e
```

The patch is intentionally not committed into Gox: it contains a large copy of
external source and remains a disposable adoption artifact. The live
`go-libraries` worktree has not been modified.

## Intentional Layout Classes

### Tabular alignment is removed

Gox uses structural indentation and does not preserve gofmt's spacing columns.
This is a deliberate dialect decision, not a lost field or type.

Before:

```go
type KeyMap struct {
	mappings   [keyCount]Key
	bound      [keyCount]bool
	configured bool
}
```

After:

```go
type KeyMap struct {
	mappings [keyCount]Key
	bound [keyCount]bool
	configured bool
}
```

Review `form_navigation.go`, `keymap.go`, `policy.go`, and `validation.go` for
small examples dominated by this decision.

### Width-pressure breaks signatures at grammar-owned lists

The type parameter remains attached to the function name. Ordinary parameters
break one logical item per line, while named result syntax remains intact.

Before:

```go
func runInteractive[T any](ctx context.Context, prompt Prompt[T], execution Execution) (result T, resultErr error) {
```

After:

```go
func runInteractive[T any](
	ctx context.Context,
	prompt Prompt[T],
	execution Execution,
) (result T, resultErr error) {
```

Review `interactive.go` and `typed_prompts.go` for the largest signature and
parameter-list cases.

### Broken calls and literals use one logical item per line

Calls, keyed literals, and long positional literals choose one canonical
broken form and add the trailing comma required by Go syntax.

Before:

```go
prompt := newTextPrompt(t, prompts.TextConfig{
	ID:       "name",
	Label:    "Name",
	Default:  prompts.Some(""),
	Fallback: prompts.Some("batch-name"),
})
```

After:

```go
prompt := newTextPrompt(
	t,
	prompts.TextConfig{
		ID: "name",
		Label: "Name",
		Default: prompts.Some(""),
		Fallback: prompts.Some("batch-name"),
	},
)
```

Review `prompt_test.go` for a compact example and `typed_prompts.go`,
`select.go`, and `interactive_test.go` for the highest-volume cases.

### Dense function literals and conditions expand structurally

Multiline call arguments give function literals a normal block. Long boolean
conditions break only at grammar-safe operator boundaries. Nested error
construction and rendering calls become deeper but use the same deterministic
list rules rather than special cases.

Review `interactive.go`, `select.go`, and `validation_test.go`. The earlier
review found no stranded control keyword, receiver, or selector boundary in
the complete migration.

### Source-authored statement grouping survives

One physical blank line between statement groups is retained, repeated gaps
collapse to one, and no gap is invented after an explicit statement semicolon.
All 168 exact blank-line boundaries following `t.Parallel()` in the baseline
remain present in the formatted result.

## Current Verification

Gox revision `88c01b7` reproduced every one of the 77 formatted files
byte-for-byte from the immutable baseline. The formatted snapshot currently
passes:

- `go test ./... -count=1`;
- `go test -race ./... -count=1`;
- `go vet ./...`; and
- `go mod tidy -diff`.

The output is a clean Gox fixed point. Sixty-three of the 77 files are not
gofmt fixed points, so adoption must replace the existing gofmt-based
`format-check`; running both formatters would create perpetual diffs.

These checks prove deterministic reproduction, the module's current executable
contracts, race instrumentation, vet acceptance, and unchanged module
metadata. They do not replace the required human judgment that this amount and
shape of vertical expansion is readable enough for daily use.

## Suggested Review Order

1. `form_navigation.go` and `keymap.go`: alignment-only and small literal
   changes.
2. `prompt_test.go`: typical nested-call behavior.
3. `interactive.go`: control flow, conditions, nested errors, and rendering.
4. `typed_prompts.go`: the highest-volume production-file expansion.
5. `select.go` and `interactive_test.go`: dense generics, callbacks, and tests.
6. `theme.go`: the original line-comment-after-operator regression boundary.

Approve only if the representative classes and the complete patch are
acceptable as the module's canonical formatter output.
