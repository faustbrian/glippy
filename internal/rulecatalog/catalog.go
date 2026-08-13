// Package rulecatalog composes native Glippy rules with admitted analyzers.
package rulecatalog

import (
	"fmt"

	"golang.org/x/tools/go/analysis/passes/assign"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
)

// NewRegistry constructs the canonical product rule registry.
func NewRegistry() (*rules.Registry, error) {
	standard, err := selfAssignmentRule()
	if err != nil {
		return nil, err
	}
	all := append(rules.DefaultRules(), standard)
	registry, err := rules.NewRegistry(all...)
	if err != nil {
		return nil, fmt.Errorf("construct product rule registry: %w", err)
	}
	return registry, nil
}

func selfAssignmentRule() (rules.Rule, error) {
	adapted, err := analysis.AdaptAnalyzer(
		assign.Analyzer,
		analysis.AnalyzerAdapterOptions{
			Metadata: rules.Metadata{
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
			SuggestedFixes: []analysis.AnalyzerFixMapping{
				{
					Message: "Remove self-assignment",
					Name: "remove-self-assignment",
					Description: "remove the ineffective self-assignment",
					Safety: rules.FixSuggestion,
				},
			},
			ReadOnlyAudited: true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("adapt self-assignment analyzer: %w", err)
	}
	return adapted, nil
}
