package config_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/config"
)

func TestParseResolvesCuratedLintProfiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		wantProfile config.Profile
		wantPresets []config.Preset
		wantProfileRules []string
	}{
		{
			name: "implicit default",
			input: "version = 1\n",
			wantProfile: config.ProfileDefault,
			wantPresets: []config.Preset{config.PresetCorrectness},
		},
		{
			name: "explicit default",
			input: "version = 1\n[lint]\nprofile = \"default\"\n",
			wantProfile: config.ProfileDefault,
			wantPresets: []config.Preset{config.PresetCorrectness},
		},
		{
			name: "recommended",
			input: "version = 1\n[lint]\nprofile = \"recommended\"\n",
			wantProfile: config.ProfileRecommended,
			wantPresets: []config.Preset{config.PresetCorrectness},
			wantProfileRules: []string{
				"almost-swapped",
				"defer-before-error-check",
				"defer-in-infinite-loop",
				"errors-is-arguments",
				"http-response-body-not-closed",
				"identical-branches",
				"ignored-append-result",
				"ineffective-value-receiver-assignment",
				"nilness",
				"overwritten-error",
				"resource-used-after-close",
				"shadowed-error",
				"subsumed-condition",
				"suspicious-range",
				"suspicious-string-conversion",
				"time-duration-unit",
				"typed-nil-error-return",
				"unchecked-rows-error",
				"unchecked-scanner-error",
			},
		},
		{
			name: "strict",
			input: "version = 1\n[lint]\nprofile = \"strict\"\n",
			wantProfile: config.ProfileStrict,
			wantPresets: []config.Preset{
				config.PresetCorrectness,
				config.PresetSuspicious,
				config.PresetPerformance,
				config.PresetComplexity,
				config.PresetStyle,
			},
		},
		{
			name: "pedantic",
			input: "version = 1\n[lint]\nprofile = \"pedantic\"\n",
			wantProfile: config.ProfilePedantic,
			wantPresets: []config.Preset{
				config.PresetCorrectness,
				config.PresetSuspicious,
				config.PresetPerformance,
				config.PresetComplexity,
				config.PresetStyle,
				config.PresetPedantic,
			},
		},
		{
			name: "explicit presets disable profiles",
			input: "version = 1\n[lint]\npresets = []\n",
			wantPresets: []config.Preset{},
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				configured, err := config.Parse(
					"project/.glippy.toml",
					[]byte(test.input),
					config.ParseOptions{},
				)
				if err != nil {
					t.Fatal(err)
				}
				if configured.Lint.Profile != test.wantProfile {
					t.Fatalf(
						"profile = %q, want %q",
						configured.Lint.Profile,
						test.wantProfile,
					)
				}
				if !slices.Equal(configured.Lint.Presets, test.wantPresets) {
					t.Fatalf(
						"presets = %v, want %v",
						configured.Lint.Presets,
						test.wantPresets,
					)
				}
				gotRules := make([]string, 0, len(configured.Lint.ProfileRules))
				for id, severity := range configured.Lint.ProfileRules {
					if severity != config.SeverityWarn {
						t.Fatalf(
							"profile rule %q severity = %q, want warn",
							id,
							severity,
						)
					}
					gotRules = append(gotRules, id)
				}
				slices.Sort(gotRules)
				if !slices.Equal(gotRules, test.wantProfileRules) {
					t.Fatalf(
						"profile rules = %v, want %v",
						gotRules,
						test.wantProfileRules,
					)
				}
			},
		)
	}
}

func TestParseRejectsInvalidOrAmbiguousLintProfiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		lint string
		want string
	}{
		{
			name: "unknown",
			lint: "profile = \"maximum\"\n",
			want: "unknown lint profile \"maximum\"",
		},
		{
			name: "singular preset",
			lint: "profile = \"strict\"\npreset = \"correctness\"\n",
			want: "lint.profile cannot be configured with lint.preset or lint.presets",
		},
		{
			name: "plural presets",
			lint: "profile = \"strict\"\npresets = [\"correctness\"]\n",
			want: "lint.profile cannot be configured with lint.preset or lint.presets",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				_, err := config.Parse(
					"project/.glippy.toml",
					[]byte("version = 1\n[lint]\n" + test.lint),
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

func TestLintProfileContributesToCanonicalIdentityAndPathPolicy(t *testing.T) {
	t.Parallel()

	parse := func(input string) config.Config {
		t.Helper()
		configured, err := config.Parse(
			"project/.glippy.toml",
			[]byte(input),
			config.ParseOptions{KnownRules: []string{"identical-branches"}},
		)
		if err != nil {
			t.Fatal(err)
		}
		return configured
	}
	recommended := parse("version = 1\n[lint]\nprofile = \"recommended\"\n")
	manual := parse(
		"version = 1\n[lint]\npresets = [\"correctness\"]\n" +
			"[lint.rules]\nidentical-branches = \"warn\"\n",
	)
	if bytes.Equal(recommended.CanonicalBytes(), manual.CanonicalBytes()) {
		t.Fatal("profile identity collapsed into equivalent manual policy")
	}

	resolved, _, err := recommended.LintForPath("internal/client.go")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Profile != config.ProfileRecommended ||
		resolved.ProfileRules["identical-branches"] != config.SeverityWarn {
		t.Fatalf("path policy lost profile state: %+v", resolved)
	}

	changed := recommended
	changed.Lint.ProfileRules = map[string]config.Severity{
		"identical-branches": config.SeverityError,
	}
	if bytes.Equal(recommended.CanonicalBytes(), changed.CanonicalBytes()) {
		t.Fatal("resolved profile policy was omitted from canonical identity")
	}
}
