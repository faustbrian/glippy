package report

import (
	"fmt"
	"strings"

	"github.com/faustbrian/glippy/internal/rules"
)

// RenderRuleText renders one rule's canonical metadata for human readers.
func RenderRuleText(registry *rules.Registry, ruleID string) ([]byte, bool) {
	metadata, found := registry.Metadata(ruleID)
	if !found {
		return nil, false
	}

	var output strings.Builder
	fmt.Fprintf(
		&output,
		"%s\n%s\n\n%s\n\n",
		metadata.ID,
		metadata.Summary,
		metadata.Documentation,
	)
	fmt.Fprintf(&output, "default severity: %s\n", metadata.DefaultSeverity)
	fmt.Fprintf(&output, "presets: %s\n", joinPresets(metadata.Presets))
	fmt.Fprintf(&output, "minimum Go: %s\n", metadata.MinimumGoVersion)
	fmt.Fprintf(&output, "analysis tier: %s\n", metadata.Requirement)
	fmt.Fprintf(&output, "node interests: %s\n", joinNodeKinds(metadata.NodeInterests))
	dependencyPolicy := "not required"
	if metadata.RequiresDependencySyntax {
		dependencyPolicy = "required"
	}
	fmt.Fprintf(&output, "dependency syntax: %s\n", dependencyPolicy)
	effectPolicy := "not required"
	if metadata.RequiresEffectFacts {
		effectPolicy = "required"
	}
	fmt.Fprintf(&output, "effect facts: %s\n", effectPolicy)
	generatedPolicy := "excluded"
	if metadata.RunOnGenerated {
		generatedPolicy = "included"
	}
	fmt.Fprintf(&output, "generated files: %s\n", generatedPolicy)
	typeErrorPolicy := "not applicable"
	if metadata.Requirement >= rules.RequireTypes {
		typeErrorPolicy = "excluded"
		if metadata.RunDespiteTypeErrors {
			typeErrorPolicy = "included"
		}
	}
	fmt.Fprintf(&output, "type-error packages: %s\n", typeErrorPolicy)
	fmt.Fprintf(&output, "categories: %s\n", joinCategories(metadata.Categories))
	if metadata.Deprecation != nil {
		fmt.Fprintf(
			&output,
			"deprecated since %s: %s\n",
			metadata.Deprecation.Since,
			metadata.Deprecation.Message,
		)
		if metadata.Deprecation.Replacement != "" {
			fmt.Fprintf(&output, "replacement: %s\n", metadata.Deprecation.Replacement)
		}
	}

	output.WriteString("\nfixes:\n")
	if len(metadata.Fixes) == 0 {
		output.WriteString("  none\n")
	}
	for _, fix := range metadata.Fixes {
		fmt.Fprintf(&output, "  %s [%s]: %s\n", fix.Name, fix.Safety, fix.Description)
	}

	output.WriteString("\nconfiguration:\n")
	if len(metadata.Options) == 0 {
		output.WriteString("  none\n")
	}
	for _, option := range metadata.Options {
		requirement := "required"
		if !option.Required && option.Default != nil {
			requirement = "optional, default " + option.Default.String()
		}
		if option.Minimum != nil || option.Maximum != nil {
			requirement += ", range " + optionRange(option)
		}
		fmt.Fprintf(
			&output,
			"  %s (%s, %s): %s\n",
			option.Name,
			option.Kind,
			requirement,
			option.Summary,
		)
	}

	output.WriteString("\nknown limitations:\n")
	if len(metadata.KnownLimitations) == 0 {
		output.WriteString("  none documented\n")
	}
	for _, limitation := range metadata.KnownLimitations {
		fmt.Fprintf(&output, "  - %s\n", limitation)
	}

	output.WriteString("\nexamples:\n")
	for index, example := range metadata.Examples {
		title := example.Title
		if strings.TrimSpace(title) == "" {
			title = fmt.Sprintf("example %d", index + 1)
		}
		fmt.Fprintf(&output, "  %s\n", title)
		output.WriteString("    incorrect:\n")
		writeIndentedLines(&output, "      ", example.Incorrect)
		output.WriteString("    correct:\n")
		writeIndentedLines(&output, "      ", example.Correct)
	}
	return []byte(output.String()), true
}

func optionRange(option rules.OptionMetadata) string {
	minimum := "unbounded"
	maximum := "unbounded"
	if option.Minimum != nil {
		minimum = fmt.Sprintf("%d", *option.Minimum)
	}
	if option.Maximum != nil {
		maximum = fmt.Sprintf("%d", *option.Maximum)
	}
	return minimum + ".." + maximum
}

func joinPresets(values []rules.Preset) string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return strings.Join(result, ", ")
}

func joinNodeKinds(values []rules.NodeKind) string {
	if len(values) == 0 {
		return "none"
	}
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return strings.Join(result, ", ")
}

func joinCategories(values []rules.Category) string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return strings.Join(result, ", ")
}

func writeIndentedLines(output *strings.Builder, prefix, value string) {
	for _, line := range strings.Split(strings.TrimRight(value, "\r\n"), "\n") {
		output.WriteString(prefix)
		output.WriteString(line)
		output.WriteByte('\n')
	}
}
