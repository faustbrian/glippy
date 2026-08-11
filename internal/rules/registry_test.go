package rules_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/rules"
)

type metadataRule struct {
	metadata rules.Metadata
}

func (r metadataRule) Metadata() rules.Metadata { return r.metadata }

type pointerMetadataRule struct {
	metadata rules.Metadata
}

func (r *pointerMetadataRule) Metadata() rules.Metadata { return r.metadata }

func TestRegistryValidatesAndOrdersNativeRuleMetadata(t *testing.T) {
	t.Parallel()

	later := metadataRule{metadata: validMetadata("z-last")}
	earlier := metadataRule{metadata: validMetadata("a-first")}
	registry, err := rules.NewRegistry(later, earlier)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := registry.IDs(), []string{"a-first", "z-last"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs() = %#v, want %#v", got, want)
	}
	if _, found := registry.Lookup("a-first"); !found {
		t.Fatal("Lookup() did not return the registered rule")
	}
	earlier.metadata.KnownLimitations[0] = "mutated caller metadata"
	metadata, found := registry.Metadata("a-first")
	if !found || metadata.KnownLimitations[0] != "does not inspect generated files" {
		t.Fatalf("Metadata() known limitations = %#v", metadata.KnownLimitations)
	}
	metadata.KnownLimitations[0] = "mutated result metadata"
	metadata, _ = registry.Metadata("a-first")
	if metadata.KnownLimitations[0] != "does not inspect generated files" {
		t.Fatalf("Metadata() returned mutable known limitations: %#v", metadata.KnownLimitations)
	}

	duplicate := metadataRule{metadata: validMetadata("a-first")}
	if _, err := rules.NewRegistry(earlier, duplicate); err == nil ||
		!strings.Contains(err.Error(), "duplicate rule ID \"a-first\"") {
		t.Fatalf("NewRegistry() duplicate error = %v", err)
	}
}

func TestRegistryRejectsTypedNilRules(t *testing.T) {
	t.Parallel()

	var nativeRule *pointerMetadataRule
	if _, err := rules.NewRegistry(nativeRule); err == nil ||
		!strings.Contains(err.Error(), "rule 0 is nil") {
		t.Fatalf("NewRegistry() typed nil error = %v", err)
	}
}

func TestRegistryRejectsIncompleteOrInconsistentMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*rules.Metadata)
		wantError string
	}{
		{
			name:      "unstable identifier",
			mutate:    func(metadata *rules.Metadata) { metadata.ID = "Bad_ID" },
			wantError: "invalid rule ID",
		},
		{
			name:      "missing documentation",
			mutate:    func(metadata *rules.Metadata) { metadata.Documentation = "" },
			wantError: "documentation is required",
		},
		{
			name:      "invalid severity",
			mutate:    func(metadata *rules.Metadata) { metadata.DefaultSeverity = "fatal" },
			wantError: "invalid default severity",
		},
		{
			name:      "syntax rule without node interests",
			mutate:    func(metadata *rules.Metadata) { metadata.NodeInterests = nil },
			wantError: "syntax rule must declare node interests",
		},
		{
			name: "types rule without node interests",
			mutate: func(metadata *rules.Metadata) {
				metadata.Requirement = rules.RequireTypes
				metadata.NodeInterests = nil
			},
			wantError: "types rule must declare node interests",
		},
		{
			name: "control-flow rule with node interests",
			mutate: func(metadata *rules.Metadata) {
				metadata.Requirement = rules.RequireControlFlow
			},
			wantError: "control flow rule must not declare node interests",
		},
		{
			name: "syntax rule with type-error policy",
			mutate: func(metadata *rules.Metadata) {
				metadata.RunDespiteTypeErrors = true
			},
			wantError: "cheap-tier rule cannot opt into type-error packages",
		},
		{
			name:      "missing behavioral examples",
			mutate:    func(metadata *rules.Metadata) { metadata.Examples = nil },
			wantError: "at least one example is required",
		},
		{
			name: "duplicate fix name",
			mutate: func(metadata *rules.Metadata) {
				metadata.Fixes = append(metadata.Fixes, metadata.Fixes[0])
			},
			wantError: "duplicate fix name \"remove-empty\"",
		},
		{
			name: "duplicate option name",
			mutate: func(metadata *rules.Metadata) {
				metadata.Options = append(metadata.Options, metadata.Options[0])
			},
			wantError: "duplicate option name \"allow-comment\"",
		},
		{
			name: "empty known limitation",
			mutate: func(metadata *rules.Metadata) {
				metadata.KnownLimitations = []string{" "}
			},
			wantError: "known limitations must not be empty",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			metadata := validMetadata("example-rule")
			test.mutate(&metadata)
			_, err := rules.NewRegistry(metadataRule{metadata: metadata})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("NewRegistry() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestRegistryResolvesPresetOverridesAndMaximumRequirement(t *testing.T) {
	t.Parallel()

	correctness := validMetadata("correctness-rule")
	correctness.Requirement = rules.RequireSyntax
	suspicious := validMetadata("suspicious-rule")
	suspicious.Presets = []rules.Preset{rules.PresetSuspicious}
	suspicious.Requirement = rules.RequireTypes
	registry, err := rules.NewRegistry(
		metadataRule{metadata: suspicious},
		metadataRule{metadata: correctness},
	)
	if err != nil {
		t.Fatal(err)
	}

	selection, err := registry.Resolve(rules.PresetCorrectness, map[string]rules.Severity{
		"correctness-rule": rules.SeverityOff,
		"suspicious-rule":  rules.SeverityError,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 1 || selection[0].ID != "suspicious-rule" ||
		selection[0].Severity != rules.SeverityError {
		t.Fatalf("Resolve() = %#v", selection)
	}
	if got := rules.MaximumRequirement(selection); got != rules.RequireTypes {
		t.Fatalf("MaximumRequirement() = %v, want %v", got, rules.RequireTypes)
	}

	if _, err := registry.Resolve(rules.PresetCorrectness, map[string]rules.Severity{
		"missing-rule": rules.SeverityWarn,
	}); err == nil || !strings.Contains(err.Error(), "unknown rule \"missing-rule\"") {
		t.Fatalf("Resolve() unknown rule error = %v", err)
	}
}

func TestRegistryReportsInvalidOverridesInRuleIDOrder(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(
		metadataRule{metadata: validMetadata("z-last")},
		metadataRule{metadata: validMetadata("a-first")},
	)
	if err != nil {
		t.Fatal(err)
	}
	for range 128 {
		_, err := registry.Resolve(rules.PresetCorrectness, map[string]rules.Severity{
			"z-last":  "invalid-z",
			"a-first": "invalid-a",
		})
		if err == nil || !strings.Contains(err.Error(), "invalid-a") {
			t.Fatalf("Resolve() first invalid override error = %v", err)
		}
	}
}

func validMetadata(id string) rules.Metadata {
	return rules.Metadata{
		ID:               id,
		Summary:          "reports one observable defect",
		Documentation:    "Full rule documentation.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement:      rules.RequireSyntax,
		NodeInterests:    []rules.NodeKind{rules.NodeCallExpr},
		RunOnGenerated:   false,
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Fixes: []rules.FixMetadata{{
			Name:        "remove-empty",
			Description: "remove the ineffective expression",
			Safety:      rules.FixSafe,
		}},
		Options: []rules.OptionMetadata{{
			Name:    "allow-comment",
			Summary: "allow an explanatory comment",
			Kind:    rules.OptionBoolean,
		}},
		KnownLimitations: []string{"does not inspect generated files"},
		Examples: []rules.Example{{
			Incorrect: "empty()",
			Correct:   "work()",
		}},
	}
}
