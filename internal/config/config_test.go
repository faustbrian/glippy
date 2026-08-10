package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/config"
)

func TestParseReportsStrictDecodeErrorsAtSourceLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantLine int
	}{
		{name: "unknown key", input: "version = 1\nwidht = 80\n", wantLine: 2},
		{name: "duplicate key", input: "version = 1\nversion = 1\n", wantLine: 2},
		{name: "invalid type", input: "version = \"one\"\n", wantLine: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := config.Parse("project/.gox.toml", []byte(test.input), config.ParseOptions{})
			var diagnostic *config.Error
			if !errors.As(err, &diagnostic) {
				t.Fatalf("Parse() error = %T %v, want *config.Error", err, err)
			}
			if diagnostic.Path != "project/.gox.toml" || diagnostic.Line != test.wantLine || diagnostic.Column <= 0 {
				t.Fatalf("Parse() diagnostic = %#v, want path project/.gox.toml at line %d", diagnostic, test.wantLine)
			}
			location := "project/.gox.toml:" + strconv.Itoa(test.wantLine) + ":"
			if !strings.HasPrefix(err.Error(), location) {
				t.Fatalf("Parse() error = %q, want source location", err)
			}
		})
	}
}

func TestParseRequiresVersionAndUsesOptionalDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantError string
	}{
		{name: "defaults", input: "version = 1\n"},
		{name: "missing version", input: "[format]\nline-width = 80\n", wantError: "version is required"},
		{name: "unsupported version", input: "version = 2\n", wantError: "unsupported configuration version 2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := config.Parse("project/.gox.toml", []byte(test.input), config.ParseOptions{})
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Parse() error = %v, want containing %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Format.LineWidth != config.DefaultLineWidth || got.Format.TabWidth != config.DefaultTabWidth {
				t.Fatalf("Parse() defaults = %#v, want width %d and tab width %d", got.Format, config.DefaultLineWidth, config.DefaultTabWidth)
			}
			if got.Lint.Preset != config.PresetCorrectness || len(got.Lint.Rules) != 0 {
				t.Fatalf("Parse() lint defaults = %#v, want correctness with no overrides", got.Lint)
			}
		})
	}
}

func TestParseRejectsInvalidSemanticValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		knownRules []string
		wantError  string
	}{
		{
			name:      "line width",
			input:     "version = 1\n[format]\nline-width = 0\n",
			wantError: "line-width must be positive",
		},
		{
			name:      "tab width",
			input:     "version = 1\n[format]\ntab-width = -1\n",
			wantError: "tab-width must be positive",
		},
		{
			name:      "preset",
			input:     "version = 1\n[lint]\npreset = \"everything\"\n",
			wantError: "unknown lint preset \"everything\"",
		},
		{
			name:      "migration preset without target",
			input:     "version = 1\n[lint]\npreset = \"migration\"\n",
			wantError: "unknown lint preset \"migration\"",
		},
		{
			name:       "severity",
			input:      "version = 1\n[lint.rules]\nknown-rule = \"fatal\"\n",
			knownRules: []string{"known-rule"},
			wantError:  "invalid severity \"fatal\" for lint rule \"known-rule\"",
		},
		{
			name:      "unknown rule",
			input:     "version = 1\n[lint.rules]\nmissing-rule = \"warn\"\n",
			wantError: "unknown lint rule \"missing-rule\"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := config.Parse("project/.gox.toml", []byte(test.input), config.ParseOptions{
				KnownRules: test.knownRules,
			})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Parse() error = %v, want containing %q", err, test.wantError)
			}
			var diagnostic *config.Error
			if !errors.As(err, &diagnostic) || diagnostic.Path != "project/.gox.toml" {
				t.Fatalf("Parse() error = %T %v, want path-aware *config.Error", err, err)
			}
		})
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

	input := []byte(`version = 1

[format]
line-width = 88
tab-width = 4

[lint]
preset = "suspicious"

[lint.rules]
known-rule = "warn"
disabled-rule = "off"
`)
	got, err := config.Parse("project/.gox.toml", input, config.ParseOptions{
		KnownRules: []string{"disabled-rule", "known-rule"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Format.LineWidth != 88 || got.Format.TabWidth != 4 {
		t.Fatalf("Parse() format = %#v, want version 1, width 88, tab width 4", got)
	}
	if got.Lint.Preset != config.PresetSuspicious {
		t.Fatalf("Parse() preset = %q, want %q", got.Lint.Preset, config.PresetSuspicious)
	}
	if got.Lint.Rules["known-rule"] != config.SeverityWarn {
		t.Fatalf("Parse() known-rule = %q, want %q", got.Lint.Rules["known-rule"], config.SeverityWarn)
	}
	if got.Lint.Rules["disabled-rule"] != config.SeverityOff {
		t.Fatalf("Parse() disabled-rule = %q, want %q", got.Lint.Rules["disabled-rule"], config.SeverityOff)
	}
}

func TestLoadUsesDefaultsOrSelectedConfiguration(t *testing.T) {
	t.Parallel()

	defaults, err := config.Load(config.Selection{}, config.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Format.LineWidth != config.DefaultLineWidth {
		t.Fatalf("Load() default width = %d, want %d", defaults.Format.LineWidth, config.DefaultLineWidth)
	}

	path := filepath.Join(t.TempDir(), config.Filename)
	if err := os.WriteFile(path, []byte("version = 1\n[format]\nline-width = 88\n"), 0o600); err != nil {
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
