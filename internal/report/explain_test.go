package report_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rules"
)

type documentedRule struct {
	metadata rules.Metadata
}

func TestRenderRuleTextKeepsEmptyContractsAndDeprecationVisible(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(
		documentedRule{
			metadata: rules.Metadata{
				ID: "deprecated-rule",
				Summary: "reports a deprecated pattern",
				Documentation: "Use the replacement rule for new code.",
				DefaultSeverity: rules.SeverityOff,
				Presets: []rules.Preset{rules.PresetMigration},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireLexical,
				Categories: []rules.Category{rules.CategoryMigration},
				Deprecation: &rules.Deprecation{
					Since: "1.3",
					Replacement: "replacement-rule",
					Message: "use the replacement rule",
				},
				Examples: []rules.Example{{Incorrect: "old()", Correct: "new()"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	output, found := report.RenderRuleText(registry, "deprecated-rule")
	if !found {
		t.Fatal("RenderRuleText() did not find the deprecated rule")
	}
	for _, contract := range
		[]string{
			"node interests: none\n",
			"deprecated since 1.3: use the replacement rule\n",
			"replacement: replacement-rule\n",
			"fixes:\n  none\n",
			"configuration:\n  none\n",
			"known limitations:\n  none documented\n",
			"examples:\n  example 1\n",
		} {
		if !strings.Contains(string(output), contract) {
			t.Fatalf(
				"RenderRuleText() output does not contain %q:\n%s",
				contract,
				output,
			)
		}
	}
}

func (r documentedRule) Metadata() rules.Metadata {
	return r.metadata
}

func TestRenderRuleTextIncludesTypeErrorPolicy(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(
		documentedRule{
			metadata: rules.Metadata{
				ID: "typed-rule",
				Summary: "reports a typed defect",
				Documentation: "Runs when partial type information is sufficient.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireTypes,
				NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
				RunDespiteTypeErrors: true,
				Categories: []rules.Category{rules.CategoryCorrectness},
				Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	output, found := report.RenderRuleText(registry, "typed-rule")
	if !found || !strings.Contains(string(output), "type-error packages: included\n") {
		t.Fatalf("RenderRuleText() = %q, %t", output, found)
	}
}

func TestRenderRuleTextIncludesDependencySyntaxPolicy(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(
		packageDocumentedRule{
			metadata: rules.Metadata{
				ID: "dependency-rule",
				Summary: "reports a dependency-aware defect",
				Documentation: "Inspects dependency syntax through the shared package graph.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireTypes,
				RequiresDependencySyntax: true,
				Categories: []rules.Category{rules.CategoryCorrectness},
				Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	output, found := report.RenderRuleText(registry, "dependency-rule")
	if !found || !strings.Contains(string(output), "dependency syntax: required\n") {
		t.Fatalf("RenderRuleText() = %q, %t", output, found)
	}
}

type packageDocumentedRule struct {
	metadata rules.Metadata
}

func (r packageDocumentedRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r packageDocumentedRule) RunPackage(*rules.PackageContext) ([]rules.PackageFinding, error) {
	return nil, nil
}

func TestRenderRuleTextUsesCanonicalMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(
		documentedRule{
			metadata: rules.Metadata{
				ID: "example-rule",
				Summary: "reports one observable defect",
				Documentation: "Reports calls whose result is ignored.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireSyntax,
				NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
				RunOnGenerated: false,
				Categories: []rules.Category{rules.CategoryCorrectness},
				Fixes: []rules.FixMetadata{
					{
						Name: "rewrite",
						Description: "replace the ignored call",
						Safety: rules.FixSafe,
					},
				},
				Options: []rules.OptionMetadata{
					{
						Name: "allow-comment",
						Summary: "allow an explanatory comment",
						Kind: rules.OptionBoolean,
						Required: false,
						Default: reportOptionValue(
							rules.BooleanOption(false),
						),
					},
				},
				KnownLimitations: []string{"does not inspect generated files"},
				Examples: []rules.Example{
					{
						Title: "ignored result",
						Incorrect: "target()\n",
						Correct: "_ = target()\n",
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	got, found := report.RenderRuleText(registry, "example-rule")
	if !found {
		t.Fatal("RenderRuleText() did not find the registered rule")
	}
	want := "example-rule\n" +
		"reports one observable defect\n\n" +
		"Reports calls whose result is ignored.\n\n" +
		"default severity: warn\n" +
		"presets: correctness\n" +
		"minimum Go: 1.22\n" +
		"analysis tier: syntax\n" +
		"node interests: call-expr\n" +
		"dependency syntax: not required\n" +
		"generated files: excluded\n" +
		"type-error packages: not applicable\n" +
		"categories: correctness\n\n" +
		"fixes:\n" +
		"  rewrite [safe]: replace the ignored call\n\n" +
		"configuration:\n" +
		"  allow-comment (boolean, optional, default false): allow an explanatory comment\n\n" +
		"known limitations:\n" +
		"  - does not inspect generated files\n\n" +
		"examples:\n" +
		"  ignored result\n" +
		"    incorrect:\n" +
		"      target()\n" +
		"    correct:\n" +
		"      _ = target()\n"
	if string(got) != want {
		t.Fatalf("RenderRuleText() =\n%s\nwant:\n%s", got, want)
	}
	if _, found := report.RenderRuleText(registry, "missing-rule"); found {
		t.Fatal("RenderRuleText() found an unknown rule")
	}
}

func reportOptionValue(value rules.OptionValue) *rules.OptionValue {
	return &value
}

func TestRenderRuleCatalogMarkdownUsesCanonicalMetadataAndIDOrder(t *testing.T) {
	t.Parallel()
	if _, err := report.RenderRuleCatalogMarkdown(nil); err == nil {
		t.Fatal("RenderRuleCatalogMarkdown(nil) succeeded")
	}

	registry, err := rules.NewRegistry(
		documentedRule{
			metadata: rules.Metadata{
				ID: "zeta-rule",
				Summary: "reports the later rule",
				Documentation: "The later rule has no fixes or options.",
				DefaultSeverity: rules.SeverityOff,
				Presets: []rules.Preset{rules.PresetMigration},
				MinimumGoVersion: "1.25",
				Requirement: rules.RequireLexical,
				Categories: []rules.Category{rules.CategoryMigration},
				Examples: []rules.Example{{Incorrect: "old()", Correct: "new()"}},
			},
		},
		packageDocumentedRule{
			metadata: rules.Metadata{
				ID: "alpha-rule",
				Summary: "reports the first rule",
				Documentation: "The first rule exercises every documented metadata section.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness, rules.PresetStyle},
				MinimumGoVersion: "1.25",
				Requirement: rules.RequireTypes,
				RequiresDependencySyntax: true,
				RunOnGenerated: true,
				RunDespiteTypeErrors: true,
				Categories: []rules.Category{
					rules.CategoryCorrectness,
					rules.CategorySafety,
				},
				Fixes: []rules.FixMetadata{
					{
						Name: "rewrite",
						Description: "replace the defective expression",
						Safety: rules.FixSafe,
					},
				},
				Options: []rules.OptionMetadata{
					{
						Name: "allow-comment",
						Summary: "allow an explanatory comment",
						Kind: rules.OptionBoolean,
						Default: reportOptionValue(
							rules.BooleanOption(false),
						),
					},
				},
				Deprecation: &rules.Deprecation{
					Since: "1.3",
					Replacement: "zeta-rule",
					Message: "use the later rule",
				},
				KnownLimitations: []string{"does not inspect dynamic calls"},
				Examples: []rules.Example{
					{
						Title: "Replace a call",
						Incorrect: "bad()",
						Correct: "good()",
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := report.RenderRuleCatalogMarkdown(registry)
	if err != nil {
		t.Fatal(err)
	}
	want := `# Lint Rules

<!-- Code generated by go generate ./internal/report; DO NOT EDIT. -->

This catalog is rendered from the same immutable rule metadata used by the
registry, scheduler, configuration decoder, and ` +
		"`glippy explain`" +
		`.
Preset membership and defaults describe the current development binary and are
not stable release promises.

## Rules

- [alpha-rule](#alpha-rule)
- [zeta-rule](#zeta-rule)

## alpha-rule

reports the first rule

The first rule exercises every documented metadata section.

- Default severity: ` +
		"`warn`" +
		`
- Presets: ` +
		"`correctness`, `style`" +
		`
- Minimum Go: ` +
		"`1.25`" +
		`
- Analysis tier: types
- Node interests: none
- Dependency syntax: required
- Generated files: included
- Type-error packages: included
- Categories: ` +
		"`correctness`, `safety`" +
		`

### Deprecation

- Since: ` +
		"`1.3`" +
		`
- Replacement: ` +
		"`zeta-rule`" +
		`
- Message: use the later rule

### Fixes

- ` +
		"`rewrite` (`safe`)" +
		`: replace the defective expression

### Configuration

- ` +
		"`allow-comment` (`boolean`; optional, default `false`)" +
		`: allow an explanatory comment

### Known limitations

- does not inspect dynamic calls

### Example: Replace a call

**Incorrect**

` +
		"```go" +
		`
bad()
` +
		"```" +
		`

**Correct**

` +
		"```go" +
		`
good()
` +
		"```" +
		`

## zeta-rule

reports the later rule

The later rule has no fixes or options.

- Default severity: ` +
		"`off`" +
		`
- Presets: ` +
		"`migration`" +
		`
- Minimum Go: ` +
		"`1.25`" +
		`
- Analysis tier: lexical source
- Node interests: none
- Dependency syntax: not required
- Generated files: excluded
- Type-error packages: not applicable
- Categories: ` +
		"`migration`" +
		`

### Fixes

None.

### Configuration

None.

### Known limitations

None documented.

### Example 1

**Incorrect**

` +
		"```go" +
		`
old()
` +
		"```" +
		`

**Correct**

` +
		"```go" +
		`
new()
` +
		"```" +
		`
`
	if string(got) != want {
		t.Fatalf("RenderRuleCatalogMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderRuleCatalogMarkdownProtectsExampleCodeFences(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(
		documentedRule{
			metadata: rules.Metadata{
				ID: "fenced-example",
				Summary: "documents source containing Markdown fences",
				Documentation: "The source remains one intact Go code block.",
				DefaultSeverity: rules.SeverityOff,
				Presets: []rules.Preset{rules.PresetMigration},
				MinimumGoVersion: "1.25",
				Requirement: rules.RequireLexical,
				Categories: []rules.Category{rules.CategoryMigration},
				Examples: []rules.Example{
					{
						Incorrect: "var value = \"```\"",
						Correct: "var value = \"safe\"",
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	output, err := report.RenderRuleCatalogMarkdown(registry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "````go\nvar value = \"```\"\n````\n") {
		t.Fatalf("RenderRuleCatalogMarkdown() did not protect its Go fence:\n%s", output)
	}
}

func TestPublishedRuleCatalogIsCurrent(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want, err := report.RenderRuleCatalogMarkdown(registry)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../docs/lint-rules.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("docs/lint-rules.md is stale; run go generate ./internal/report")
	}
}
