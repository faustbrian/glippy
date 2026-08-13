package config_test

import (
	"bytes"
	"errors"
	"go/build"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/config"
	"github.com/faustbrian/gox/internal/rules"
)

func TestParseReportsStrictDecodeErrorsAtSourceLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		wantLine int
	}{
		{name: "unknown key", input: "version = 1\nwidht = 80\n", wantLine: 2},
		{name: "duplicate key", input: "version = 1\nversion = 1\n", wantLine: 2},
		{name: "invalid type", input: "version = \"one\"\n", wantLine: 1},
		{
			name: "build tags type",
			input: "version = 1\n[analysis]\nbuild-tags = \"selected\"\n",
			wantLine: 3,
		},
		{name: "goos type", input: "version = 1\n[analysis]\ngoos = true\n", wantLine: 3},
		{name: "goarch type", input: "version = 1\n[analysis]\ngoarch = 64\n", wantLine: 3},
		{
			name: "cgo type",
			input: "version = 1\n[analysis]\ncgo-enabled = \"yes\"\n",
			wantLine: 3,
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := config.Parse(
					"project/.gox.toml",
					[]byte(test.input),
					config.ParseOptions{},
				)
				var diagnostic *config.Error
				if !errors.As(err, &diagnostic) {
					t.Fatalf(
						"Parse() error = %T %v, want *config.Error",
						err,
						err,
					)
				}
				if diagnostic.Path != "project/.gox.toml" ||
					diagnostic.Line != test.wantLine ||
					diagnostic.Column <= 0 {
					t.Fatalf(
						"Parse() diagnostic = %#v, want path project/.gox.toml at line %d",
						diagnostic,
						test.wantLine,
					)
				}
				location := "project/.gox.toml:" + strconv.Itoa(test.wantLine) + ":"
				if !strings.HasPrefix(err.Error(), location) {
					t.Fatalf("Parse() error = %q, want source location", err)
				}
			},
		)
	}
}

func TestParseRequiresVersionAndUsesOptionalDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		wantError string
	}{
		{name: "defaults", input: "version = 1\n"},
		{
			name: "missing version",
			input: "[format]\nline-width = 80\n",
			wantError: "version is required",
		},
		{
			name: "unsupported version",
			input: "version = 2\n",
			wantError: "unsupported configuration version 2",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				got, err := config.Parse(
					"project/.gox.toml",
					[]byte(test.input),
					config.ParseOptions{},
				)
				if test.wantError != "" {
					if err == nil ||
						!strings.Contains(err.Error(), test.wantError) {
						t.Fatalf(
							"Parse() error = %v, want containing %q",
							err,
							test.wantError,
						)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if got.Format.LineWidth != config.DefaultLineWidth ||
					got.Format.TabWidth != config.DefaultTabWidth {
					t.Fatalf(
						"Parse() defaults = %#v, want width %d and tab width %d",
						got.Format,
						config.DefaultLineWidth,
						config.DefaultTabWidth,
					)
				}
				if got.Lint.Preset != config.PresetCorrectness ||
					len(got.Lint.Rules) != 0 {
					t.Fatalf(
						"Parse() lint defaults = %#v, want correctness with no overrides",
						got.Lint,
					)
				}
				if len(got.Analysis.BuildTags) != 0 ||
					got.Analysis.GOOS != runtime.GOOS ||
					got.Analysis.GOARCH != runtime.GOARCH ||
					got.Analysis.CGOEnabled != build.Default.CgoEnabled {
					t.Fatalf("Parse() analysis defaults = %#v", got.Analysis)
				}
				if got.Cache.Enabled ||
					got.Cache.MaxEntries != config.DefaultCacheMaxEntries ||
					got.Cache.MaxBytes != config.DefaultCacheMaxBytes {
					t.Fatalf("Parse() cache defaults = %#v", got.Cache)
				}
			},
		)
	}
}

func TestParseRejectsInvalidSemanticValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		knownRules []string
		wantError string
	}{
		{
			name: "line width",
			input: "version = 1\n[format]\nline-width = 0\n",
			wantError: "line-width must be positive",
		},
		{
			name: "tab width",
			input: "version = 1\n[format]\ntab-width = -1\n",
			wantError: "tab-width must be positive",
		},
		{
			name: "preset",
			input: "version = 1\n[lint]\npreset = \"everything\"\n",
			wantError: "unknown lint preset \"everything\"",
		},
		{
			name: "migration preset without target",
			input: "version = 1\n[lint]\npreset = \"migration\"\n",
			wantError: "unknown lint preset \"migration\"",
		},
		{
			name: "empty build tag",
			input: "version = 1\n[analysis]\nbuild-tags = [\"\"]\n",
			wantError: "analysis.build-tags contains invalid tag",
		},
		{
			name: "invalid build tag",
			input: "version = 1\n[analysis]\nbuild-tags = [\"one,two\"]\n",
			wantError: "analysis.build-tags contains invalid tag",
		},
		{
			name: "invalid goos",
			input: "version = 1\n[analysis]\ngoos = \"Linux\"\n",
			wantError: "analysis.goos must contain only lowercase ASCII letters and digits",
		},
		{
			name: "invalid goarch",
			input: "version = 1\n[analysis]\ngoarch = \"amd/64\"\n",
			wantError: "analysis.goarch must contain only lowercase ASCII letters and digits",
		},
		{
			name: "severity",
			input: "version = 1\n[lint.rules]\nknown-rule = \"fatal\"\n",
			knownRules: []string{"known-rule"},
			wantError: "invalid severity \"fatal\" for lint rule \"known-rule\"",
		},
		{
			name: "unknown rule",
			input: "version = 1\n[lint.rules]\nmissing-rule = \"warn\"\n",
			wantError: "unknown lint rule \"missing-rule\"",
		},
		{
			name: "negative cache entries",
			input: "version = 1\n[cache]\nmax-entries = -1\n",
			wantError: "cache.max-entries must not be negative",
		},
		{
			name: "negative cache bytes",
			input: "version = 1\n[cache]\nmax-bytes = -1\n",
			wantError: "cache.max-bytes must not be negative",
		},
		{
			name: "enabled unbounded cache",
			input: "version = 1\n[cache]\nenabled = true\nmax-entries = 0\nmax-bytes = 0\n",
			wantError: "enabled cache requires a positive max-entries or max-bytes limit",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := config.Parse(
					"project/.gox.toml",
					[]byte(test.input),
					config.ParseOptions{KnownRules: test.knownRules},
				)
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf(
						"Parse() error = %v, want containing %q",
						err,
						test.wantError,
					)
				}
				var diagnostic *config.Error
				if !errors.As(err, &diagnostic) ||
					diagnostic.Path != "project/.gox.toml" {
					t.Fatalf(
						"Parse() error = %T %v, want path-aware *config.Error",
						err,
						err,
					)
				}
			},
		)
	}
}

func TestCanonicalBytesIgnoreSourceOrderAndCacheLifecyclePolicy(t *testing.T) {
	t.Parallel()

	first, err := config.Parse(
		"first.toml",
		[]byte(
			`version = 1

[analysis]
build-tags = ["z", "a", "a"]
goos = "linux"
goarch = "amd64"
cgo-enabled = true

[cache]
enabled = true
max-entries = 32
max-bytes = 1048576

[lint.rules]
z-rule = "warn"
a-rule = "error"
`,
		),
		config.ParseOptions{KnownRules: []string{"a-rule", "z-rule"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := config.Parse(
		"second.toml",
		[]byte(
			`version = 1

[analysis]
cgo-enabled = true
goarch = "amd64"
goos = "linux"
build-tags = ["a", "z"]

[lint.rules]
a-rule = "error"
z-rule = "warn"

[cache]
max-bytes = 1048576
max-entries = 32
enabled = true
`,
		),
		config.ParseOptions{KnownRules: []string{"z-rule", "a-rule"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.CanonicalBytes(), second.CanonicalBytes()) {
		t.Fatal("canonical configuration changed with source order")
	}
	second.Cache.Enabled = false
	second.Cache.MaxEntries++
	second.Cache.MaxBytes++
	if !bytes.Equal(first.CanonicalBytes(), second.CanonicalBytes()) {
		t.Fatal("cache lifecycle policy changed result configuration identity")
	}
	second.Format.LineWidth++
	if bytes.Equal(first.CanonicalBytes(), second.CanonicalBytes()) {
		t.Fatal("result-affecting configuration was omitted from identity")
	}
	for _, mutate := range
		[]func(
			*config.Config,
		){
			func(configuration *config.Config) {
				configuration.Analysis.BuildTags = []string{"different"}
			},
			func(configuration *config.Config) {
				configuration.Analysis.GOOS = "darwin"
			},
			func(configuration *config.Config) {
				configuration.Analysis.GOARCH = "arm64"
			},
			func(configuration *config.Config) {
				configuration.Analysis.CGOEnabled = false
			},
		} {
		candidate := first
		candidate.Analysis.BuildTags = append([]string(nil), first.Analysis.BuildTags...)
		mutate(&candidate)
		if bytes.Equal(first.CanonicalBytes(), candidate.CanonicalBytes()) {
			t.Fatal("analysis selection was omitted from result configuration identity")
		}
	}
}

func TestParseReportsRuleErrorsDeterministically(t *testing.T) {
	t.Parallel()

	input := []byte("version = 1\n[lint.rules]\nz-rule = \"warn\"\na-rule = \"warn\"\n")
	for range 20 {
		_, err := config.Parse("project/.gox.toml", input, config.ParseOptions{})
		if err == nil || !strings.Contains(err.Error(), "unknown lint rule \"a-rule\"") {
			t.Fatalf("Parse() error = %v, want deterministic first rule a-rule", err)
		}
	}
}

func TestParseAppliesTypedConfiguration(t *testing.T) {
	t.Parallel()

	input := []byte(
		`version = 1

[format]
line-width = 88
tab-width = 4

[analysis]
build-tags = ["selected", "integration", "selected"]
goos = "linux"
goarch = "amd64"
cgo-enabled = true

[lint]
preset = "suspicious"

[lint.rules]
known-rule = "warn"
disabled-rule = "off"
`,
	)
	got, err := config.Parse(
		"project/.gox.toml",
		input,
		config.ParseOptions{KnownRules: []string{"disabled-rule", "known-rule"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Format.LineWidth != 88 || got.Format.TabWidth != 4 {
		t.Fatalf("Parse() format = %#v, want version 1, width 88, tab width 4", got)
	}
	if got.Lint.Preset != config.PresetSuspicious {
		t.Fatalf("Parse() preset = %q, want %q", got.Lint.Preset, config.PresetSuspicious)
	}
	if strings.Join(got.Analysis.BuildTags, ",") != "integration,selected" ||
		got.Analysis.GOOS != "linux" ||
		got.Analysis.GOARCH != "amd64" ||
		!got.Analysis.CGOEnabled {
		t.Fatalf("Parse() analysis = %#v", got.Analysis)
	}
	if got.Lint.Rules["known-rule"] != config.SeverityWarn {
		t.Fatalf(
			"Parse() known-rule = %q, want %q",
			got.Lint.Rules["known-rule"],
			config.SeverityWarn,
		)
	}
	if got.Lint.Rules["disabled-rule"] != config.SeverityOff {
		t.Fatalf(
			"Parse() disabled-rule = %q, want %q",
			got.Lint.Rules["disabled-rule"],
			config.SeverityOff,
		)
	}
}

func TestParseAppliesTypedRuleOptions(t *testing.T) {
	t.Parallel()

	input := []byte(
		`version = 1

[lint.rule-options."configured-rule"]
allow-comment = true
limit = 12
label = "stable"
names = ["first", "second"]
`,
	)
	configured, err := config.Parse(
		"project/.gox.toml",
		input,
		config.ParseOptions{
			KnownRules: []string{"configured-rule"},
			RuleOptions: map[string][]rules.OptionMetadata{
				"configured-rule": {
					{
						Name: "allow-comment",
						Summary: "allow comments",
						Kind: rules.OptionBoolean,
					},
					{
						Name: "limit",
						Summary: "set a limit",
						Kind: rules.OptionInteger,
					},
					{
						Name: "label",
						Summary: "set a label",
						Kind: rules.OptionString,
					},
					{
						Name: "names",
						Summary: "set names",
						Kind: rules.OptionStrings,
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	options, found := configured.Lint.RuleOptions["configured-rule"]
	if !found {
		t.Fatal("Parse() omitted configured rule options")
	}
	allow, allowFound := options.Boolean("allow-comment")
	limit, limitFound := options.Integer("limit")
	label, labelFound := options.String("label")
	names, namesFound := options.Strings("names")
	if !allowFound ||
		!allow ||
		!limitFound ||
		limit != 12 ||
		!labelFound ||
		label != "stable" ||
		!namesFound ||
		strings.Join(names, ",") != "first,second" {
		t.Fatalf("Parse() rule options = %#v", options)
	}
	names[0] = "mutated"
	again, _ := options.Strings("names")
	if again[0] != "first" {
		t.Fatalf("rule option strings were mutable: %#v", again)
	}
}

func TestParseRejectsInvalidRuleOptionsDeterministically(t *testing.T) {
	t.Parallel()

	schema := map[string][]rules.OptionMetadata{
		"configured-rule": {
			{Name: "enabled", Summary: "enable behavior", Kind: rules.OptionBoolean},
		},
	}
	tests := []struct {
		name string
		input string
		wantError string
	}{
		{
			name: "unknown rule",
			input: "version = 1\n[lint.rule-options.missing]\nenabled = true\n",
			wantError: "unknown lint rule \"missing\" in lint.rule-options",
		},
		{
			name: "unknown option",
			input: "version = 1\n[lint.rule-options.configured-rule]\nunknown = true\n",
			wantError: "unknown option \"unknown\" for lint rule \"configured-rule\"",
		},
		{
			name: "wrong type",
			input: "version = 1\n[lint.rule-options.configured-rule]\nenabled = \"yes\"\n",
			wantError: "option \"enabled\" for lint rule \"configured-rule\" must be boolean",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := config.Parse(
					"project/.gox.toml",
					[]byte(test.input),
					config.ParseOptions{
						KnownRules: []string{"configured-rule"},
						RuleOptions: schema,
					},
				)
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Parse() error = %v, want %q", err, test.wantError)
				}
			},
		)
	}
}

func TestParseAppliesSuppressionReasonPolicy(t *testing.T) {
	t.Parallel()

	defaults := config.Defaults()
	if defaults.Lint.Suppressions.RequireReason {
		t.Fatal("Defaults() requires suppression reasons")
	}
	if defaults.Lint.Suppressions.ExpiryCutoff != "" {
		t.Fatalf(
			"Defaults() expiry cutoff = %q, want empty",
			defaults.Lint.Suppressions.ExpiryCutoff,
		)
	}
	configured, err := config.Parse(
		"project/.gox.toml",
		[]byte(
			`version = 1

[lint.suppressions]
require-reason = true
expiry-cutoff = "2026-08-11"
`,
		),
		config.ParseOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !configured.Lint.Suppressions.RequireReason {
		t.Fatal("Parse() did not require suppression reasons")
	}
	if configured.Lint.Suppressions.ExpiryCutoff != "2026-08-11" {
		t.Fatalf(
			"Parse() expiry cutoff = %q, want 2026-08-11",
			configured.Lint.Suppressions.ExpiryCutoff,
		)
	}

	_, err = config.Parse(
		"project/.gox.toml",
		[]byte(
			`version = 1

[lint.suppressions]
expiry-cutoff = "2026-02-30"
`,
		),
		config.ParseOptions{},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"lint.suppressions.expiry-cutoff must be a valid YYYY-MM-DD date",
		) {
		t.Fatalf("Parse() invalid expiry cutoff error = %v", err)
	}
}

func TestLoadUsesDefaultsOrSelectedConfiguration(t *testing.T) {
	t.Parallel()

	defaults, err := config.Load(config.Selection{}, config.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Format.LineWidth != config.DefaultLineWidth {
		t.Fatalf(
			"Load() default width = %d, want %d",
			defaults.Format.LineWidth,
			config.DefaultLineWidth,
		)
	}

	path := filepath.Join(t.TempDir(), config.Filename)
	if err := os.WriteFile(path, []byte("version = 1\n[format]\nline-width = 88\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(config.Selection{Path: path}, config.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Format.LineWidth != 88 {
		t.Fatalf("Load() width = %d, want 88", loaded.Format.LineWidth)
	}
}
