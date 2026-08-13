# `pkg/prompts` Adoption Review

## Decision

The maintainer approved Phase 2 and this complete adoption layout on
2026-08-13. The decision:

1. accept the Gox dialect for this module;
2. approve the dedicated migration commit as one reviewable change; and
3. replace the module's gofmt-based formatting authority with Gox in CI,
   editor, and contributor workflows.

Approval does not authorize semantic source changes, pushing or integrating the
adoption branch, a Gox release, a tag, or publication.

## Review Artifact

The baseline is immutable local `go-libraries` `main` revision
`8c9c1e7abb3d3d99bf7c950f1acc771fcd0dcabf`. The formatter selects 77 Go files
and changes 65: 29 production files, 34 test files, and two Go maintenance
scripts. The Go-only patch contains 7,596 insertions and 3,591 deletions and
has SHA-256:

```text
1510f2002a6ac0599741d5a35f875ea40aeb1f34de3c237d550c3d59d6078638
```

The coordinated migration also changes the module Makefile, lint configuration,
README, and changelog. The complete 69-file patch contains 7,608 insertions and
3,598 deletions and has SHA-256
`f89e966a6ab29aabb244afc8dcc6124e57ff7c29fd004e612aa5c762beb072d6`.
It is committed as `d6b0fba81ec31b7ea9134d8aa6acaf481c933e38` on the isolated
`feature/gox-prompts-adoption` branch. The original dirty `go-libraries`
checkout was not modified.

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

Gox revision `d84842b08ff9009a778edf7d7f5924abda6cb52d` produced a clean
second-format fixed point from the immutable baseline. The migration currently
passes:

- the pinned module `format-check`;
- module tests, race tests, vet, and `go mod tidy -diff`;
- the module documentation gate;
- the configured golangci-lint gate with zero findings; and
- the nested comparison module tests through the repository workspace.

The first lint run found that width-driven breaks changed physical-line
ownership for eight external `//nolint` suppressions. Gox revision `d84842b`
fixes that source-fidelity defect and rejects unsupported ownership movement;
the refreshed migration preserves all eight suppressions and the lint gate now
passes. Sixty-three of the 77 files are not gofmt fixed points, so the commit
replaces the previous gofmt and goimports authorities rather than running
competing formatters.

These checks prove deterministic reproduction, the module's current executable
contracts, race instrumentation, analyzer acceptance, and unchanged module
metadata. The nested comparison module's standalone tidy command cannot resolve
its unreleased `pkg/prompts v0.0.0` dependency without the repository workspace;
no module metadata changed. The maintainer's 2026-08-13 approval supplies the
separate human judgment that the complete layout is acceptable for daily use.

## Suggested Review Order

1. `form_navigation.go` and `keymap.go`: alignment-only and small literal
   changes.
2. `prompt_test.go`: typical nested-call behavior.
3. `interactive.go`: control flow, conditions, nested errors, and rendering.
4. `typed_prompts.go`: the highest-volume production-file expansion.
5. `select.go` and `interactive_test.go`: dense generics, callbacks, and tests.
6. `theme.go`: the original line-comment-after-operator regression boundary.

This order remains the review map for the approved canonical formatter output.
