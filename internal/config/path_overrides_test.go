package config_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/config"
)

func TestParseResolvesOrderedPathScopedLintPolicy(t *testing.T) {
	t.Parallel()

	configured, err := config.Parse(
		"project/.glippy.toml",
		[]byte(
			`version = 1

[lint]
presets = []

[lint.rules]
duplicate-condition = "warn"

[[lint.overrides]]
paths = ["**/*_test.go", "testdata/**"]

[lint.overrides.rules]
duplicate-condition = "off"
nilness = "warn"

[[lint.overrides]]
paths = ["integration/**"]

[lint.overrides.rules]
nilness = "error"
`,
		),
		config.ParseOptions{KnownRules: []string{"duplicate-condition", "nilness"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path string
		wantDuplicate config.Severity
		wantNilness config.Severity
		wantNilnessSet bool
		wantMatches []int
	}{
		{path: "main.go", wantDuplicate: config.SeverityWarn},
		{
			path: "main_test.go",
			wantDuplicate: config.SeverityOff,
			wantNilness: config.SeverityWarn,
			wantNilnessSet: true,
			wantMatches: []int{1},
		},
		{
			path: "testdata/case/input.go",
			wantDuplicate: config.SeverityOff,
			wantNilness: config.SeverityWarn,
			wantNilnessSet: true,
			wantMatches: []int{1},
		},
		{
			path: "integration/client_test.go",
			wantDuplicate: config.SeverityOff,
			wantNilness: config.SeverityError,
			wantNilnessSet: true,
			wantMatches: []int{1, 2},
		},
	}
	for _, test := range tests {
		t.Run(
			test.path,
			func(t *testing.T) {
				lint, matches, resolveErr := configured.LintForPath(test.path)
				if resolveErr != nil {
					t.Fatal(resolveErr)
				}
				if got := lint.Rules["duplicate-condition"];
					got != test.wantDuplicate {
					t.Fatalf(
						"duplicate-condition = %q, want %q",
						got,
						test.wantDuplicate,
					)
				}
				gotNilness, found := lint.Rules["nilness"]
				if found != test.wantNilnessSet || gotNilness != test.wantNilness {
					t.Fatalf(
						"nilness = %q, %t; want %q, %t",
						gotNilness,
						found,
						test.wantNilness,
						test.wantNilnessSet,
					)
				}
				if !slices.Equal(matches, test.wantMatches) {
					t.Fatalf("matches = %v, want %v", matches, test.wantMatches)
				}
			},
		)
	}
}

func TestParseRejectsInvalidPathScopedLintPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		override string
		want string
	}{
		{
			name: "missing paths",
			override: "[lint.overrides.rules]\nknown-rule = \"warn\"\n",
			want: "lint override 1 requires at least one path pattern",
		},
		{
			name: "missing rules",
			override: "paths = [\"**/*_test.go\"]\n",
			want: "lint override 1 requires at least one rule",
		},
		{
			name: "absolute path",
			override: "paths = [\"/tmp/**\"]\n[lint.overrides.rules]\nknown-rule = \"warn\"\n",
			want: "lint override 1 path pattern \"/tmp/**\" must be project-relative",
		},
		{
			name: "parent traversal",
			override: "paths = [\"../shared/**\"]\n[lint.overrides.rules]\nknown-rule = \"warn\"\n",
			want: "lint override 1 path pattern \"../shared/**\" must be project-relative",
		},
		{
			name: "invalid glob",
			override: "paths = [\"internal/[abc\"]\n[lint.overrides.rules]\nknown-rule = \"warn\"\n",
			want: "lint override 1 path pattern \"internal/[abc\" is invalid",
		},
		{
			name: "duplicate path",
			override: "paths = [\"testdata/**\", \"testdata/**\"]\n[lint.overrides.rules]\nknown-rule = \"warn\"\n",
			want: "lint override 1 contains duplicate path pattern \"testdata/**\"",
		},
		{
			name: "unknown rule",
			override: "paths = [\"**/*_test.go\"]\n[lint.overrides.rules]\nmissing-rule = \"warn\"\n",
			want: "unknown lint rule \"missing-rule\" in lint override 1",
		},
		{
			name: "invalid severity",
			override: "paths = [\"**/*_test.go\"]\n[lint.overrides.rules]\nknown-rule = \"maybe\"\n",
			want: "invalid severity \"maybe\" for lint rule \"known-rule\" in lint override 1",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				input := "version = 1\n[[lint.overrides]]\n" + test.override
				_, err := config.Parse(
					"project/.glippy.toml",
					[]byte(input),
					config.ParseOptions{KnownRules: []string{"known-rule"}},
				)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("Parse() error = %v, want %q", err, test.want)
				}
			},
		)
	}
}

func TestPathScopedLintPolicyHasDeterministicIdentity(t *testing.T) {
	t.Parallel()

	parse := func(input string) config.Config {
		t.Helper()
		configured, err := config.Parse(
			"project/.glippy.toml",
			[]byte(input),
			config.ParseOptions{KnownRules: []string{"known-rule"}},
		)
		if err != nil {
			t.Fatal(err)
		}
		return configured
	}
	first := parse(
		`version = 1
[[lint.overrides]]
paths = ["testdata/**", "**/*_test.go"]
[lint.overrides.rules]
known-rule = "off"
[[lint.overrides]]
paths = ["integration/**"]
[lint.overrides.rules]
known-rule = "error"
`,
	)
	pathOrder := parse(
		`version = 1
[[lint.overrides]]
paths = ["**/*_test.go", "testdata/**"]
[lint.overrides.rules]
known-rule = "off"
[[lint.overrides]]
paths = ["integration/**"]
[lint.overrides.rules]
known-rule = "error"
`,
	)
	if !bytes.Equal(first.CanonicalBytes(), pathOrder.CanonicalBytes()) {
		t.Fatal("order-independent path sets changed canonical identity")
	}
	overrideOrder := parse(
		`version = 1
[[lint.overrides]]
paths = ["integration/**"]
[lint.overrides.rules]
known-rule = "error"
[[lint.overrides]]
paths = ["**/*_test.go", "testdata/**"]
[lint.overrides.rules]
known-rule = "off"
`,
	)
	if bytes.Equal(first.CanonicalBytes(), overrideOrder.CanonicalBytes()) {
		t.Fatal("order-sensitive path overrides shared canonical identity")
	}
}
