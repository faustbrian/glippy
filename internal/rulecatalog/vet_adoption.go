package rulecatalog

import (
	"golang.org/x/tools/go/analysis/passes/buildtag"
	"golang.org/x/tools/go/analysis/passes/directive"
	"golang.org/x/tools/go/analysis/passes/sigchanyzer"
	"golang.org/x/tools/go/analysis/passes/stdversion"
	"golang.org/x/tools/go/analysis/passes/tests"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
)

func invalidBuildConstraintRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		buildtag.Analyzer,
		rules.Metadata{
			ID:               "invalid-build-constraint",
			Summary:          "detects malformed, misplaced, or inconsistent build constraints",
			Documentation:    "Build constraints select which source participates in a build. A malformed or misplaced //go:build or legacy // +build line can be ignored or select a different build than intended. Glippy adapts the standard buildtag analyzer for each discovered Go source file.",
			DefaultSeverity:  rules.SeverityWarn,
			Presets:          []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement:      rules.RequireSyntax,
			NodeInterests:    []rules.NodeKind{rules.NodeFile},
			Categories:       []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The syntax-tier adapter checks discovered Go files independently; build constraints in non-Go package files are not yet inspected.",
				"Cross-file package loading is not required, so build-excluded Go files must be included by Glippy discovery to be checked.",
			},
			Examples: []rules.Example{{
				Title:     "Place the constraint in the file header",
				Incorrect: "package sample\n//go:build linux",
				Correct:   "//go:build linux\n\npackage sample",
			}},
		},
		nil,
	)
}

func invalidDirectiveRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		directive.Analyzer,
		rules.Metadata{
			ID:               "invalid-directive",
			Summary:          "detects invalid placement or use of Go toolchain directives",
			Documentation:    "Known Go toolchain directives have placement and package contracts that determine whether the toolchain honors them. Glippy adapts the standard directive analyzer so invalid //go:debug directives are reported through the shared syntax pipeline.",
			DefaultSeverity:  rules.SeverityWarn,
			Presets:          []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement:      rules.RequireSyntax,
			NodeInterests:    []rules.NodeKind{rules.NodeFile},
			Categories:       []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The upstream analyzer currently validates known //go:debug contracts and intentionally leaves unknown future directives alone.",
				"The syntax-tier adapter checks discovered Go files independently; directives in non-Go package files are not yet inspected.",
			},
			Examples: []rules.Example{{
				Title:     "Use go:debug only where the toolchain accepts it",
				Incorrect: "//go:debug panicnil=1\npackage library",
				Correct:   "//go:debug panicnil=1\npackage main",
			}},
		},
		nil,
	)
}

func invalidTestSignatureRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		tests.Analyzer,
		rules.Metadata{
			ID:               "invalid-test-signature",
			Summary:          "detects malformed tests, benchmarks, fuzz targets, and examples",
			Documentation:    "The go test driver discovers functions by exact naming and signature contracts. A near miss can leave intended coverage silently unexecuted or make a fuzz target invalid. Glippy adapts the standard tests analyzer through its typed package scheduler.",
			DefaultSeverity:  rules.SeverityWarn,
			Presets:          []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement:      rules.RequireTypes,
			NodeInterests:    []rules.NodeKind{rules.NodeFile},
			Categories:       []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The rule follows the standard analyzer's test, benchmark, fuzz, and example discovery contracts.",
				"Only files loaded as part of the selected package and test variants are analyzed.",
			},
			Examples: []rules.Example{{
				Title:     "Keep test entry points non-generic",
				Incorrect: "func TestValue[T any](t *testing.T) {}",
				Correct:   "func TestValue(t *testing.T) {}",
			}},
		},
		nil,
	)
}

func unbufferedSignalChannelRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		sigchanyzer.Analyzer,
		rules.Metadata{
			ID:               "unbuffered-signal-channel",
			Summary:          "detects unbuffered channels passed to signal.Notify",
			Documentation:    "signal.Notify sends without blocking and may drop a signal when an unbuffered channel has no receiver ready. A buffer of at least one lets the notification survive normal scheduling delay. Glippy adapts the standard sigchanyzer analyzer.",
			DefaultSeverity:  rules.SeverityWarn,
			Presets:          []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement:      rules.RequireTypes,
			NodeInterests:    []rules.NodeKind{rules.NodeFile},
			Categories:       []rules.Category{rules.CategoryCorrectness, rules.CategorySafety},
			KnownLimitations: []string{
				"The analyzer recognizes direct signal.Notify calls and locally traceable channel declarations.",
				"The buffer change is suggestion-only because applications may have an intentional delivery protocol.",
			},
			Examples: []rules.Example{{
				Title:     "Buffer the signal channel",
				Incorrect: "signals := make(chan os.Signal)\nsignal.Notify(signals, os.Interrupt)",
				Correct:   "signals := make(chan os.Signal, 1)\nsignal.Notify(signals, os.Interrupt)",
			}},
		},
		[]analysis.AnalyzerFixMapping{{
			Message:     "Change to buffer channel",
			Name:        "buffer-signal-channel",
			Description: "give the signal channel a buffer of one",
			Safety:      rules.FixSuggestion,
		}},
	)
}

func standardLibraryVersionRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		stdversion.Analyzer,
		rules.Metadata{
			ID:                   "standard-library-version",
			Summary:              "detects standard-library APIs newer than the source Go version",
			Documentation:        "Using a standard-library symbol introduced after the module or file language version makes the declared compatibility contract inaccurate and may fail on the supported toolchain. Glippy adapts the standard stdversion analyzer using package and per-file Go versions.",
			DefaultSeverity:      rules.SeverityWarn,
			Presets:              []rules.Preset{rules.PresetCorrectness, rules.PresetMigration},
			MinimumGoVersion:     "1.25",
			Requirement:          rules.RequireTypes,
			NodeInterests:        []rules.NodeKind{rules.NodeFile},
			RunDespiteTypeErrors: true,
			Categories:           []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The rule follows the standard analyzer's version database and deliberate exceptions for symbols guarded by experiments or version constraints.",
				"Modules declaring a language version before Go 1.21 are outside the upstream analyzer's reporting contract.",
			},
			Examples: []rules.Example{{
				Title:     "Use APIs available at the declared Go version",
				Incorrect: "// module declares Go 1.25\nbuffer.Peek(1)",
				Correct:   "// module declares Go 1.26\nbuffer.Peek(1)",
			}},
		},
		nil,
	)
}
