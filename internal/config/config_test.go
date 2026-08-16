package config_test

import (
	"bytes"
	"errors"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/config"
	"github.com/faustbrian/glippy/internal/rules"
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
					"project/.glippy.toml",
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
				if diagnostic.Path != "project/.glippy.toml" ||
					diagnostic.Line != test.wantLine ||
					diagnostic.Column <= 0 {
					t.Fatalf(
						"Parse() diagnostic = %#v, want path project/.glippy.toml at line %d",
						diagnostic,
						test.wantLine,
					)
				}
				location := "project/.glippy.toml:" +
					strconv.Itoa(test.wantLine) +
					":"
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
					"project/.glippy.toml",
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
				if !slices.Equal(
					got.Lint.Presets,
					[]config.Preset{config.PresetCorrectness},
				) ||
					got.Lint.WarningsAsErrors ||
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

func TestParseSupportsComposablePresetsAndWarningEscalation(t *testing.T) {
	t.Parallel()

	configured, err := config.Parse(
		"project/.glippy.toml",
		[]byte(
			`version = 1

[lint]
presets = ["pedantic", "correctness", "suspicious"]
warnings-as-errors = true
`,
		),
		config.ParseOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []config.Preset{
		config.PresetCorrectness,
		config.PresetSuspicious,
		config.PresetPedantic,
	}
	if !slices.Equal(configured.Lint.Presets, want) || !configured.Lint.WarningsAsErrors {
		t.Fatalf(
			"Parse() lint = %#v, want presets %v with warning escalation",
			configured.Lint,
			want,
		)
	}

	legacy, err := config.Parse(
		"legacy/.glippy.toml",
		[]byte("version = 1\n[lint]\npreset = \"style\"\n"),
		config.ParseOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(legacy.Lint.Presets, []config.Preset{config.PresetStyle}) {
		t.Fatalf("legacy preset resolved to %v, want style", legacy.Lint.Presets)
	}
}

func TestParseExplicitEmptyPresetSetDisablesAllGroups(t *testing.T) {
	t.Parallel()

	configured, err := config.Parse(
		"project/.glippy.toml",
		[]byte("version = 1\n[lint]\npresets = []\n"),
		config.ParseOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Lint.Presets == nil || len(configured.Lint.Presets) != 0 {
		t.Fatalf("Parse() presets = %#v, want explicit empty set", configured.Lint.Presets)
	}
}

func TestParseRejectsAmbiguousOrInvalidPresetGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		want string
	}{
		{
			name: "singular and plural",
			input: "version = 1\n[lint]\npreset = \"correctness\"\npresets = [\"style\"]\n",
			want: "lint.preset and lint.presets cannot both be configured",
		},
		{
			name: "duplicate",
			input: "version = 1\n[lint]\npresets = [\"style\", \"style\"]\n",
			want: "duplicate lint preset \"style\"",
		},
		{
			name: "restriction wholesale",
			input: "version = 1\n[lint]\npresets = [\"restriction\"]\n",
			want: "lint preset \"restriction\" must be enabled rule by rule",
		},
		{
			name: "migration without target",
			input: "version = 1\n[lint]\npresets = [\"migration\"]\n",
			want: "lint preset \"migration\" requires an explicit migration target",
		},
		{
			name: "unknown",
			input: "version = 1\n[lint]\npresets = [\"everything\"]\n",
			want: "unknown lint preset \"everything\"",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				_, err := config.Parse(
					"project/.glippy.toml",
					[]byte(test.input),
					config.ParseOptions{},
				)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"Parse() error = %v, want containing %q",
						err,
						test.want,
					)
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
			name: "target missing goos",
			input: "version = 1\n[[analysis.targets]]\ngoarch = \"amd64\"\n",
			wantError: "analysis.targets[0].goos must contain only lowercase ASCII letters and digits",
		},
		{
			name: "target missing goarch",
			input: "version = 1\n[[analysis.targets]]\ngoos = \"linux\"\n",
			wantError: "analysis.targets[0].goarch must contain only lowercase ASCII letters and digits",
		},
		{
			name: "invalid target tag",
			input: "version = 1\n[[analysis.targets]]\ngoos = \"linux\"\ngoarch = \"amd64\"\ntags = [\"one,two\"]\n",
			wantError: "analysis.targets[0].tags contains invalid tag",
		},
		{
			name: "duplicate target",
			input: "version = 1\n[[analysis.targets]]\ngoos = \"linux\"\ngoarch = \"amd64\"\ntags = [\"integration\", \"linux\"]\n[[analysis.targets]]\ngoos = \"linux\"\ngoarch = \"amd64\"\ntags = [\"linux\", \"integration\"]\n",
			wantError: "analysis.targets repeats target \"linux/amd64+tags=integration,linux\"",
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
					"project/.glippy.toml",
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
					diagnostic.Path != "project/.glippy.toml" {
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

func TestCanonicalBytesNormalizePresetOrderAndRetainWarningEscalation(t *testing.T) {
	t.Parallel()

	first, err := config.Parse(
		"first.toml",
		[]byte("version = 1\n[lint]\npresets = [\"style\", \"correctness\"]\n"),
		config.ParseOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := config.Parse(
		"second.toml",
		[]byte("version = 1\n[lint]\npresets = [\"correctness\", \"style\"]\n"),
		config.ParseOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.CanonicalBytes(), second.CanonicalBytes()) {
		t.Fatal("preset source order changed canonical configuration identity")
	}
	second.Lint.WarningsAsErrors = true
	if bytes.Equal(first.CanonicalBytes(), second.CanonicalBytes()) {
		t.Fatal("warning escalation was omitted from canonical configuration identity")
	}
}

func TestParseReportsRuleErrorsDeterministically(t *testing.T) {
	t.Parallel()

	input := []byte("version = 1\n[lint.rules]\nz-rule = \"warn\"\na-rule = \"warn\"\n")
	for range 20 {
		_, err := config.Parse("project/.glippy.toml", input, config.ParseOptions{})
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
presets = ["suspicious", "pedantic"]
warnings-as-errors = true

[lint.baseline]
path = ".glippy-baseline.json"
report-stale = true
expiry-cutoff = "2026-08-13"

[lint.rules]
known-rule = "warn"
disabled-rule = "off"
`,
	)
	got, err := config.Parse(
		"project/.glippy.toml",
		input,
		config.ParseOptions{KnownRules: []string{"disabled-rule", "known-rule"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Format.LineWidth != 88 || got.Format.TabWidth != 4 {
		t.Fatalf("Parse() format = %#v, want version 1, width 88, tab width 4", got)
	}
	if !slices.Equal(
		got.Lint.Presets,
		[]config.Preset{config.PresetSuspicious, config.PresetPedantic},
	) ||
		!got.Lint.WarningsAsErrors {
		t.Fatalf("Parse() lint = %#v", got.Lint)
	}
	if got.Lint.Baseline.Path != ".glippy-baseline.json" ||
		!got.Lint.Baseline.ReportStale ||
		got.Lint.Baseline.ExpiryCutoff != "2026-08-13" {
		t.Fatalf("Parse() baseline = %#v", got.Lint.Baseline)
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

func TestParseCanonicalizesAnalysisTargets(t *testing.T) {
	t.Parallel()

	configuration, err := config.Parse(
		"project/.glippy.toml",
		[]byte(
			`version = 1

[[analysis.targets]]
goos = "linux"
goarch = "amd64"
tags = ["integration", "linux", "integration"]

[[analysis.targets]]
goos = "darwin"
goarch = "arm64"
cgo-enabled = true
`,
		),
		config.ParseOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []config.AnalysisTarget{
		{GOOS: "darwin", GOARCH: "arm64", CGOEnabled: true},
		{GOOS: "linux", GOARCH: "amd64", BuildTags: []string{"integration", "linux"}},
	}
	if !reflect.DeepEqual(configuration.Analysis.Targets, want) {
		t.Fatalf("analysis targets = %#v, want %#v", configuration.Analysis.Targets, want)
	}
	if configuration.Analysis.Targets[0].ID() != "darwin/arm64+cgo" ||
		configuration.Analysis.Targets[1].ID() != "linux/amd64+tags=integration,linux" {
		t.Fatalf("analysis target IDs = %#v", configuration.Analysis.Targets)
	}
}

func TestParseRejectsTooManyAnalysisTargets(t *testing.T) {
	t.Parallel()

	var input strings.Builder
	input.WriteString("version = 1\n")
	for index := 0; index < config.MaxAnalysisTargets + 1; index++ {
		fmt.Fprintf(
			&input,
			"[[analysis.targets]]\ngoos = \"linux\"\ngoarch = \"arch%d\"\n",
			index,
		)
	}
	_, err := config.Parse(
		"project/.glippy.toml",
		[]byte(input.String()),
		config.ParseOptions{},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"analysis.targets must not contain more than 32 targets",
		) {
		t.Fatalf("Parse() target limit error = %v", err)
	}
}

func TestCanonicalBytesNormalizeAndRetainAnalysisTargets(t *testing.T) {
	t.Parallel()

	first, err := config.Parse(
		"first.toml",
		[]byte(
			`version = 1

[[analysis.targets]]
goos = "linux"
goarch = "amd64"
tags = ["linux", "integration"]

[[analysis.targets]]
goos = "darwin"
goarch = "arm64"
cgo-enabled = true
`,
		),
		config.ParseOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := config.Parse(
		"second.toml",
		[]byte(
			`version = 1

[[analysis.targets]]
goos = "darwin"
goarch = "arm64"
cgo-enabled = true

[[analysis.targets]]
goos = "linux"
goarch = "amd64"
tags = ["integration", "linux", "integration"]
`,
		),
		config.ParseOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.CanonicalBytes(), second.CanonicalBytes()) {
		t.Fatal("analysis target source order changed canonical configuration identity")
	}
	second.Analysis.Targets[0].CGOEnabled = false
	if bytes.Equal(first.CanonicalBytes(), second.CanonicalBytes()) {
		t.Fatal("analysis targets were omitted from canonical configuration identity")
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
		"project/.glippy.toml",
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
			{
				Name: "limit",
				Summary: "bound complexity",
				Kind: rules.OptionInteger,
				Minimum: configInt64Pointer(1),
				Maximum: configInt64Pointer(100),
			},
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
		{
			name: "integer below minimum",
			input: "version = 1\n[lint.rule-options.configured-rule]\nlimit = 0\n",
			wantError: "option \"limit\" for lint rule \"configured-rule\" must be at least 1",
		},
		{
			name: "integer above maximum",
			input: "version = 1\n[lint.rule-options.configured-rule]\nlimit = 101\n",
			wantError: "option \"limit\" for lint rule \"configured-rule\" must be at most 100",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := config.Parse(
					"project/.glippy.toml",
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

func configInt64Pointer(value int64) *int64 {
	return &value
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
		"project/.glippy.toml",
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
		"project/.glippy.toml",
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

func TestLoadReadsCanonicalProjectContracts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configurationPath := filepath.Join(root, config.Filename)
	firstPath := filepath.Join(root, "contracts", "first.toml")
	secondPath := filepath.Join(root, "contracts", "second.toml")
	if err := os.MkdirAll(filepath.Dir(firstPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		firstPath,
		[]byte(
			"version = 1\n[[functions]]\nsymbol = \"example.com/project.Stop\"\nnoreturn = true\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		secondPath,
		[]byte(
			"version = 1\n[[functions]]\nsymbol = \"example.com/project.Open\"\nmust-use = [0]\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	writeConfiguration := func(paths string) {
		t.Helper()
		input := "version = 1\n[analysis]\ncontract-files = [" + paths + "]\n"
		if err := os.WriteFile(configurationPath, []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	load := func() config.Config {
		t.Helper()
		loaded, err := config.Load(
			config.Selection{Root: root, Path: configurationPath},
			config.ParseOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		return loaded
	}

	writeConfiguration("\"contracts/second.toml\", \"contracts/first.toml\"")
	first := load()
	if !slices.Equal(
		first.Analysis.ContractFiles,
		[]string{"contracts/first.toml", "contracts/second.toml"},
	) {
		t.Fatalf("contract files = %v", first.Analysis.ContractFiles)
	}
	functions := first.Analysis.Contracts.Functions()
	if len(functions) != 2 ||
		functions[0].Symbol != "example.com/project.Open" ||
		functions[1].Symbol != "example.com/project.Stop" {
		t.Fatalf("contracts = %#v", functions)
	}

	writeConfiguration("\"contracts/first.toml\", \"contracts/second.toml\"")
	reordered := load()
	if !bytes.Equal(first.CanonicalBytes(), reordered.CanonicalBytes()) {
		t.Fatal("configuration identity depends on contract-file order")
	}
	if err := os.WriteFile(
		secondPath,
		[]byte(
			"version = 1\n[[functions]]\nsymbol = \"example.com/project.Open\"\nmust-use = [0, 1]\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	changed := load()
	if bytes.Equal(first.CanonicalBytes(), changed.CanonicalBytes()) {
		t.Fatal("configuration identity ignored changed contract contents")
	}
}

func TestLoadRejectsInvalidContractFileSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		paths string
		want string
	}{
		{
			name: "outside root",
			paths: "[\"../contracts.toml\"]",
			want: "portable project-relative paths",
		},
		{
			name: "absolute",
			paths: "[\"/tmp/contracts.toml\"]",
			want: "portable project-relative paths",
		},
		{
			name: "duplicate",
			paths: "[\"contracts.toml\", \"contracts.toml\"]",
			want: "duplicate contract file",
		},
		{name: "missing", paths: "[\"missing.toml\"]", want: "read contract file"},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				root := t.TempDir()
				configurationPath := filepath.Join(root, config.Filename)
				input := "version = 1\n[analysis]\ncontract-files = " +
					test.paths +
					"\n"
				if err := os.WriteFile(configurationPath, []byte(input), 0o600);
					err != nil {
					t.Fatal(err)
				}
				_, err := config.Load(
					config.Selection{Root: root, Path: configurationPath},
					config.ParseOptions{},
				)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"Load() error = %v, want containing %q",
						err,
						test.want,
					)
				}
			},
		)
	}
}
