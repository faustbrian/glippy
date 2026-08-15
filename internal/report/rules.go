package report

import (
	"fmt"
	"strings"

	"github.com/faustbrian/glippy/internal/rules"
)

// RuleListOptions filters rule discovery by exact canonical metadata.
type RuleListOptions struct {
	Preset *rules.Preset
	Requirement *rules.Requirement
	Fixable bool
}

// RuleExplanation is the versioned machine-readable explain contract.
type RuleExplanation struct {
	SchemaVersion int `json:"schema_version"`
	Command string `json:"command"`
	Rule RuleMetadata `json:"rule"`
}

// RuleMetadata is the stable machine-readable projection of canonical rule metadata.
type RuleMetadata struct {
	ID string `json:"id"`
	Summary string `json:"summary"`
	Documentation string `json:"documentation"`
	DefaultSeverity string `json:"default_severity"`
	Presets []string `json:"presets"`
	MinimumGoVersion string `json:"minimum_go_version"`
	AnalysisTier string `json:"analysis_tier"`
	NodeInterests []string `json:"node_interests"`
	RequiresDependencySyntax bool `json:"requires_dependency_syntax"`
	RequiresEffectFacts bool `json:"requires_effect_facts"`
	RunOnGenerated bool `json:"run_on_generated"`
	RunDespiteTypeErrors bool `json:"run_despite_type_errors"`
	Categories []string `json:"categories"`
	Fixes []RuleFix `json:"fixes"`
	Options []RuleOption `json:"options"`
	Deprecation *RuleDeprecation `json:"deprecation,omitempty"`
	KnownLimitations []string `json:"known_limitations"`
	Examples []RuleExample `json:"examples"`
}

type RuleFix struct {
	Name string `json:"name"`
	Description string `json:"description"`
	Safety string `json:"safety"`
}

type RuleOption struct {
	Name string `json:"name"`
	Summary string `json:"summary"`
	Kind string `json:"kind"`
	Required bool `json:"required"`
	Default string `json:"default,omitempty"`
	Minimum *int64 `json:"minimum,omitempty"`
	Maximum *int64 `json:"maximum,omitempty"`
}

type RuleDeprecation struct {
	Since string `json:"since"`
	Replacement string `json:"replacement,omitempty"`
	Message string `json:"message"`
}

type RuleExample struct {
	Title string `json:"title"`
	Incorrect string `json:"incorrect"`
	Correct string `json:"correct"`
}

// RenderRuleListText renders ID-ordered rule discovery output.
func RenderRuleListText(registry *rules.Registry, options RuleListOptions) ([]byte, error) {
	if registry == nil {
		return nil, fmt.Errorf("render rule list requires a registry")
	}
	var output strings.Builder
	for _, id := range registry.IDs() {
		metadata, found := registry.Metadata(id)
		if !found {
			return nil, fmt.Errorf("registered rule %q has no metadata", id)
		}
		if options.Preset != nil && !containsPreset(metadata.Presets, *options.Preset) {
			continue
		}
		if options.Requirement != nil && metadata.Requirement != *options.Requirement {
			continue
		}
		if options.Fixable && len(metadata.Fixes) == 0 {
			continue
		}
		fmt.Fprintf(
			&output,
			"%s\tdefault=%s\tpresets=%s\ttier=%s\tfixes=%s\n",
			metadata.ID,
			metadata.DefaultSeverity,
			joinPresetValues(metadata.Presets),
			RequirementName(metadata.Requirement),
			joinFixSafeties(metadata.Fixes),
		)
	}
	return []byte(output.String()), nil
}

// RenderRuleJSON renders one rule's canonical metadata as versioned JSON.
func RenderRuleJSON(registry *rules.Registry, ruleID string) ([]byte, bool, error) {
	if registry == nil {
		return nil, false, fmt.Errorf("render rule JSON requires a registry")
	}
	metadata, found := registry.Metadata(ruleID)
	if !found {
		return nil, false, nil
	}
	encoded, err := marshalJSON(
		RuleExplanation{
			SchemaVersion: SchemaVersion,
			Command: "explain",
			Rule: mapRuleMetadata(metadata),
		},
	)
	return encoded, true, err
}

// RequirementName returns the canonical machine-facing tier name.
func RequirementName(requirement rules.Requirement) string {
	switch requirement {
	case rules.RequireLexical:
		return "lexical"
	case rules.RequireSyntax:
		return "syntax"
	case rules.RequireTypes:
		return "types"
	case rules.RequireControlFlow:
		return "control-flow"
	case rules.RequireSSA:
		return "ssa"
	default:
		return fmt.Sprintf("unknown-%d", requirement)
	}
}

func mapRuleMetadata(metadata rules.Metadata) RuleMetadata {
	result := RuleMetadata{
		ID: metadata.ID,
		Summary: metadata.Summary,
		Documentation: metadata.Documentation,
		DefaultSeverity: string(metadata.DefaultSeverity),
		Presets: make([]string, len(metadata.Presets)),
		MinimumGoVersion: metadata.MinimumGoVersion,
		AnalysisTier: RequirementName(metadata.Requirement),
		NodeInterests: make([]string, len(metadata.NodeInterests)),
		RequiresDependencySyntax: metadata.RequiresDependencySyntax,
		RequiresEffectFacts: metadata.RequiresEffectFacts,
		RunOnGenerated: metadata.RunOnGenerated,
		RunDespiteTypeErrors: metadata.RunDespiteTypeErrors,
		Categories: make([]string, len(metadata.Categories)),
		Fixes: make([]RuleFix, len(metadata.Fixes)),
		Options: make([]RuleOption, len(metadata.Options)),
		KnownLimitations: append([]string{}, metadata.KnownLimitations...),
		Examples: make([]RuleExample, len(metadata.Examples)),
	}
	for index, preset := range metadata.Presets {
		result.Presets[index] = string(preset)
	}
	for index, node := range metadata.NodeInterests {
		result.NodeInterests[index] = string(node)
	}
	for index, category := range metadata.Categories {
		result.Categories[index] = string(category)
	}
	for index, fix := range metadata.Fixes {
		result.Fixes[index] = RuleFix{
			Name: fix.Name,
			Description: fix.Description,
			Safety: string(fix.Safety),
		}
	}
	for index, option := range metadata.Options {
		mapped := RuleOption{
			Name: option.Name,
			Summary: option.Summary,
			Kind: string(option.Kind),
			Required: option.Required,
		}
		if option.Default != nil {
			mapped.Default = option.Default.String()
		}
		if option.Minimum != nil {
			minimum := *option.Minimum
			mapped.Minimum = &minimum
		}
		if option.Maximum != nil {
			maximum := *option.Maximum
			mapped.Maximum = &maximum
		}
		result.Options[index] = mapped
	}
	if metadata.Deprecation != nil {
		result.Deprecation = &RuleDeprecation{
			Since: metadata.Deprecation.Since,
			Replacement: metadata.Deprecation.Replacement,
			Message: metadata.Deprecation.Message,
		}
	}
	for index, example := range metadata.Examples {
		result.Examples[index] = RuleExample{
			Title: example.Title,
			Incorrect: example.Incorrect,
			Correct: example.Correct,
		}
	}
	return result
}

func containsPreset(values []rules.Preset, target rules.Preset) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func joinPresetValues(values []rules.Preset) string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return strings.Join(result, ",")
}

func joinFixSafeties(fixes []rules.FixMetadata) string {
	if len(fixes) == 0 {
		return "none"
	}
	seen := make(map[rules.FixSafety]bool, 3)
	for _, fix := range fixes {
		seen[fix.Safety] = true
	}
	order := []rules.FixSafety{rules.FixSafe, rules.FixSuggestion, rules.FixUnsafe}
	result := make([]string, 0, len(order))
	for _, safety := range order {
		if seen[safety] {
			result = append(result, string(safety))
		}
	}
	return strings.Join(result, ",")
}
