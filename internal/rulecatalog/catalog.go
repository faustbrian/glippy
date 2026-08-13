// Package rulecatalog composes native Glippy rules with admitted analyzers.
package rulecatalog

import (
	"fmt"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/assign"
	atomicanalyzer "golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/ifaceassert"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
)

// NewRegistry constructs the canonical product rule registry.
func NewRegistry() (*rules.Registry, error) {
	standard, err := standardAnalyzerRules()
	if err != nil {
		return nil, err
	}
	all := append(rules.DefaultRules(), standard...)
	all = append(all, rules.NewContextCancelLeakRule(), rules.NewLoopCaptureRule())
	registry, err := rules.NewRegistry(all...)
	if err != nil {
		return nil, fmt.Errorf("construct product rule registry: %w", err)
	}
	return registry, nil
}

func standardAnalyzerRules() ([]rules.Rule, error) {
	constructors := []func() (
		rules.Rule,
		error,
	){
		selfAssignmentRule,
		copiedLockRule,
		impossibleTypeAssertionRule,
		httpResponseBeforeErrorRule,
		atomicUpdateAssignmentRule,
	}
	result := make([]rules.Rule, 0, len(constructors))
	for _, construct := range constructors {
		rule, err := construct()
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, nil
}

func selfAssignmentRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		assign.Analyzer,
		rules.Metadata{
			ID: "self-assignment",
			Summary: "detects assignments that leave a value unchanged",
			Documentation: "A value assigned directly to itself has no effect and usually indicates a copied expression, a mistaken assignment target, or code left behind after a refactor. Glippy exposes the standard Go assign analyzer through its typed scheduler, deterministic diagnostics, suppressions, baselines, and fix-safety model.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The rule follows the standard Go assign analyzer and intentionally excludes assignments whose evaluation can have observable effects, including map index expressions.",
				"The removal fix is suggestion-only because deleting an assignment requires confirmation that the statement was not an intentional marker.",
				"Removing a statement that shares a physical line through an explicit semicolon can retain an empty statement after formatting; review suggestion output before accepting it.",
			},
			Examples: []rules.Example{
				{
					Title: "Assign the intended value",
					Incorrect: "value = value",
					Correct: "value = replacement",
				},
			},
		},
		[]analysis.AnalyzerFixMapping{
			{
				Message: "Remove self-assignment",
				Name: "remove-self-assignment",
				Description: "remove the ineffective self-assignment",
				Safety: rules.FixSuggestion,
			},
		},
	)
}

func copiedLockRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		copylock.Analyzer,
		rules.Metadata{
			ID: "copied-lock",
			Summary: "detects locks copied after first use",
			Documentation: "Copying a value that contains sync.Mutex, sync.RWMutex, or another lock-like noCopy value can split synchronization state and make callers believe they coordinate through the same lock. Glippy adapts the standard Go copylocks analyzer across assignments, declarations, returns, calls, ranges, and value receivers.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			RunDespiteTypeErrors: true,
			Categories: []rules.Category{
				rules.CategoryCorrectness,
				rules.CategorySafety,
			},
			KnownLimitations: []string{
				"The rule follows the standard Go copylocks analyzer's lock-path and noCopy conventions.",
				"Diagnostics do not prove that a particular copied lock has already been used at runtime; the structural copy is the reported hazard.",
			},
			Examples: []rules.Example{
				{
					Title: "Keep lock-bearing values behind pointers",
					Incorrect: "func clone(value *state) state { return *value }",
					Correct: "func share(value *state) *state { return value }",
				},
			},
		},
		nil,
	)
}

func impossibleTypeAssertionRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		ifaceassert.Analyzer,
		rules.Metadata{
			ID: "impossible-type-assertion",
			Summary: "detects interface assertions that can never succeed",
			Documentation: "An interface-to-interface assertion is impossible when no concrete type can implement both interfaces because they declare the same method with conflicting signatures. Such an assertion always panics in its single-result form and always fails in its comma-ok form.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{rules.CategoryCorrectness},
			KnownLimitations: []string{
				"The standard ifaceassert analyzer reports only conflicts it can prove from interface method sets.",
				"Assertions to concrete types are already checked by the Go type checker and are not duplicated here.",
			},
			Examples: []rules.Example{
				{
					Title: "Assert compatible interfaces",
					Incorrect: "value.(interface{ Read(int) })",
					Correct: "value.(interface{ Read() })",
				},
			},
		},
		nil,
	)
}

func httpResponseBeforeErrorRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		httpresponse.Analyzer,
		rules.Metadata{
			ID: "http-response-before-error",
			Summary: "detects HTTP responses used before their errors are checked",
			Documentation: "An HTTP request may return a nil response with a non-nil error. Deferring response.Body.Close before checking that error can therefore dereference nil on the failure path and hide the original request failure.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			Categories: []rules.Category{
				rules.CategoryCorrectness,
				rules.CategorySafety,
			},
			KnownLimitations: []string{
				"The standard httpresponse analyzer recognizes net/http functions and Client methods returning (*http.Response, error).",
				"The rule targets the immediate defer-before-error-check pattern and does not attempt general response ownership analysis.",
			},
			Examples: []rules.Example{
				{
					Title: "Check the request error before using the response",
					Incorrect: "response, err := http.Get(url)\ndefer response.Body.Close()\nif err != nil { return err }",
					Correct: "response, err := http.Get(url)\nif err != nil { return err }\ndefer response.Body.Close()",
				},
			},
		},
		nil,
	)
}

func atomicUpdateAssignmentRule() (rules.Rule, error) {
	return adaptStandardAnalyzer(
		atomicanalyzer.Analyzer,
		rules.Metadata{
			ID: "atomic-update-assignment",
			Summary: "detects atomic updates assigned back non-atomically",
			Documentation: "Functions such as atomic.AddUint64 already update the pointed-to value atomically and return the new value. Assigning that return value back through the same pointer performs a second non-atomic write, defeating the synchronization contract and introducing a race.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.25",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile},
			RunDespiteTypeErrors: true,
			Categories: []rules.Category{
				rules.CategoryCorrectness,
				rules.CategorySafety,
			},
			KnownLimitations: []string{
				"The standard atomic analyzer covers the legacy sync/atomic AddInt32, AddInt64, AddUint32, AddUint64, and AddUintptr functions.",
				"Typed atomic wrapper methods and higher-level synchronization patterns are outside this analyzer's scope.",
			},
			Examples: []rules.Example{
				{
					Title: "Use the atomic update without a second write",
					Incorrect: "*value = atomic.AddUint64(value, 1)",
					Correct: "atomic.AddUint64(value, 1)",
				},
			},
		},
		nil,
	)
}

func adaptStandardAnalyzer(
	analyzer *goanalysis.Analyzer,
	metadata rules.Metadata,
	fixes []analysis.AnalyzerFixMapping,
) (rules.Rule, error) {
	adapted, err := analysis.AdaptAnalyzer(
		analyzer,
		analysis.AnalyzerAdapterOptions{
			Metadata: metadata,
			SuggestedFixes: fixes,
			ReadOnlyAudited: true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("adapt %s analyzer: %w", metadata.ID, err)
	}
	return adapted, nil
}
