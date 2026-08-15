package rules_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/rules"
)

type metadataRule struct {
	metadata rules.Metadata
}

func (r metadataRule) Metadata() rules.Metadata {
	return r.metadata
}

type pointerMetadataRule struct {
	metadata rules.Metadata
}

type packageMetadataRule struct {
	metadata rules.Metadata
}

func (r *pointerMetadataRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r packageMetadataRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r packageMetadataRule) RunPackage(*rules.PackageContext) ([]rules.PackageFinding, error) {
	return nil, nil
}

func TestRegistryValidatesPackageWideRuleMetadata(t *testing.T) {
	t.Parallel()

	metadata := validMetadata("package-rule")
	metadata.Requirement = rules.RequireTypes
	metadata.NodeInterests = nil
	if _, err := rules.NewRegistry(packageMetadataRule{metadata: metadata}); err != nil {
		t.Fatalf("NewRegistry() package-wide metadata error = %v", err)
	}

	withInterests := metadata
	withInterests.NodeInterests = []rules.NodeKind{rules.NodeFile}
	if _, err := rules.NewRegistry(packageMetadataRule{metadata: withInterests});
		err == nil ||
			!strings.Contains(
				err.Error(),
				"package-wide rule must not declare node interests",
			) {
		t.Fatalf("NewRegistry() package-wide interests error = %v", err)
	}

	cheap := metadata
	cheap.Requirement = rules.RequireSyntax
	if _, err := rules.NewRegistry(packageMetadataRule{metadata: cheap});
		err == nil ||
			!strings.Contains(err.Error(), "package-wide rule must require types") {
		t.Fatalf("NewRegistry() package-wide requirement error = %v", err)
	}

	dependencies := metadata
	dependencies.RequiresDependencySyntax = true
	if _, err := rules.NewRegistry(packageMetadataRule{metadata: dependencies}); err != nil {
		t.Fatalf("NewRegistry() dependency-aware package metadata error = %v", err)
	}

	nodeScoped := validMetadata("node-scoped-dependencies")
	nodeScoped.Requirement = rules.RequireTypes
	nodeScoped.RequiresDependencySyntax = true
	if _, err := rules.NewRegistry(metadataRule{metadata: nodeScoped});
		err == nil ||
			!strings.Contains(
				err.Error(),
				"dependency syntax requires a package-wide types rule",
			) {
		t.Fatalf("NewRegistry() node-scoped dependency syntax error = %v", err)
	}

	effects := validMetadata("effect-facts")
	effects.Requirement = rules.RequireControlFlow
	effects.NodeInterests = nil
	effects.RequiresEffectFacts = true
	if _, err := rules.NewRegistry(metadataRule{metadata: effects}); err != nil {
		t.Fatalf("NewRegistry() effect-fact metadata error = %v", err)
	}

	cheapEffects := validMetadata("cheap-effects")
	cheapEffects.RequiresEffectFacts = true
	if _, err := rules.NewRegistry(metadataRule{metadata: cheapEffects});
		err == nil ||
			!strings.Contains(
				err.Error(),
				"effect facts require control-flow or SSA analysis",
			) {
		t.Fatalf("NewRegistry() cheap effect-fact error = %v", err)
	}
}

func TestRegistryValidatesAndOrdersNativeRuleMetadata(t *testing.T) {
	t.Parallel()

	later := metadataRule{metadata: validMetadata("z-last")}
	earlier := metadataRule{metadata: validMetadata("a-first")}
	registry, err := rules.NewRegistry(later, earlier)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := registry.IDs(), []string{"a-first", "z-last"};
		!reflect.DeepEqual(got, want) {
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
		t.Fatalf(
			"Metadata() returned mutable known limitations: %#v",
			metadata.KnownLimitations,
		)
	}

	duplicate := metadataRule{metadata: validMetadata("a-first")}
	if _, err := rules.NewRegistry(earlier, duplicate);
		err == nil || !strings.Contains(err.Error(), "duplicate rule ID \"a-first\"") {
		t.Fatalf("NewRegistry() duplicate error = %v", err)
	}
}

func TestRegistryRejectsTypedNilRules(t *testing.T) {
	t.Parallel()

	var nativeRule *pointerMetadataRule
	if _, err := rules.NewRegistry(nativeRule);
		err == nil || !strings.Contains(err.Error(), "rule 0 is nil") {
		t.Fatalf("NewRegistry() typed nil error = %v", err)
	}
}

func TestRegistryRejectsIncompleteOrInconsistentMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mutate func(*rules.Metadata)
		wantError string
	}{
		{
			name: "unstable identifier",
			mutate: func(metadata *rules.Metadata) {
				metadata.ID = "Bad_ID"
			},
			wantError: "invalid rule ID",
		},
		{
			name: "missing documentation",
			mutate: func(metadata *rules.Metadata) {
				metadata.Documentation = ""
			},
			wantError: "documentation is required",
		},
		{
			name: "invalid severity",
			mutate: func(metadata *rules.Metadata) {
				metadata.DefaultSeverity = "fatal"
			},
			wantError: "invalid default severity",
		},
		{
			name: "syntax rule without node interests",
			mutate: func(metadata *rules.Metadata) {
				metadata.NodeInterests = nil
			},
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
			name: "SSA rule with node interests",
			mutate: func(metadata *rules.Metadata) {
				metadata.Requirement = rules.RequireSSA
			},
			wantError: "SSA rule must not declare node interests",
		},
		{
			name: "SSA rule with type-error policy",
			mutate: func(metadata *rules.Metadata) {
				metadata.Requirement = rules.RequireSSA
				metadata.NodeInterests = nil
				metadata.RunDespiteTypeErrors = true
			},
			wantError: "SSA rule cannot run on type-error packages",
		},
		{
			name: "syntax rule with type-error policy",
			mutate: func(metadata *rules.Metadata) {
				metadata.RunDespiteTypeErrors = true
			},
			wantError: "cheap-tier rule cannot opt into type-error packages",
		},
		{
			name: "missing behavioral examples",
			mutate: func(metadata *rules.Metadata) {
				metadata.Examples = nil
			},
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
			name: "optional option without default",
			mutate: func(metadata *rules.Metadata) {
				metadata.Options[0].Default = nil
			},
			wantError: "optional option \"allow-comment\" requires a default",
		},
		{
			name: "required option with default",
			mutate: func(metadata *rules.Metadata) {
				metadata.Options[0].Required = true
			},
			wantError: "required option \"allow-comment\" must not declare a default",
		},
		{
			name: "option default kind",
			mutate: func(metadata *rules.Metadata) {
				value := rules.StringOption("no")
				metadata.Options[0].Default = &value
			},
			wantError: "option \"allow-comment\" default has kind \"string\"; want \"boolean\"",
		},
		{
			name: "integer bounds on boolean option",
			mutate: func(metadata *rules.Metadata) {
				metadata.Options[0].Minimum = int64Pointer(1)
			},
			wantError: "option \"allow-comment\" bounds require integer kind",
		},
		{
			name: "inverted integer bounds",
			mutate: func(metadata *rules.Metadata) {
				metadata.Options[0].Kind = rules.OptionInteger
				metadata.Options[0].Default = optionValue(rules.IntegerOption(3))
				metadata.Options[0].Minimum = int64Pointer(5)
				metadata.Options[0].Maximum = int64Pointer(4)
			},
			wantError: "option \"allow-comment\" minimum 5 exceeds maximum 4",
		},
		{
			name: "integer default below minimum",
			mutate: func(metadata *rules.Metadata) {
				metadata.Options[0].Kind = rules.OptionInteger
				metadata.Options[0].Default = optionValue(rules.IntegerOption(2))
				metadata.Options[0].Minimum = int64Pointer(3)
			},
			wantError: "option \"allow-comment\" default 2 must be at least 3",
		},
		{
			name: "integer default above maximum",
			mutate: func(metadata *rules.Metadata) {
				metadata.Options[0].Kind = rules.OptionInteger
				metadata.Options[0].Default = optionValue(rules.IntegerOption(6))
				metadata.Options[0].Maximum = int64Pointer(5)
			},
			wantError: "option \"allow-comment\" default 6 must be at most 5",
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
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				metadata := validMetadata("example-rule")
				test.mutate(&metadata)
				_, err := rules.NewRegistry(metadataRule{metadata: metadata})
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf(
						"NewRegistry() error = %v, want %q",
						err,
						test.wantError,
					)
				}
			},
		)
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

	selection, err := registry.Resolve(
		rules.PresetCorrectness,
		map[string]rules.Severity{
			"correctness-rule": rules.SeverityOff,
			"suspicious-rule": rules.SeverityError,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 1 ||
		selection[0].ID != "suspicious-rule" ||
		selection[0].Severity != rules.SeverityError {
		t.Fatalf("Resolve() = %#v", selection)
	}
	if got := rules.MaximumRequirement(selection); got != rules.RequireTypes {
		t.Fatalf("MaximumRequirement() = %v, want %v", got, rules.RequireTypes)
	}

	if _, err := registry.Resolve(
		rules.PresetCorrectness,
		map[string]rules.Severity{"missing-rule": rules.SeverityWarn},
	);
		err == nil || !strings.Contains(err.Error(), "unknown rule \"missing-rule\"") {
		t.Fatalf("Resolve() unknown rule error = %v", err)
	}
}

func TestRegistryAppliesExactRuleFiltersAfterConfiguredPolicy(t *testing.T) {
	t.Parallel()

	correctness := validMetadata("correctness-rule")
	suspicious := validMetadata("suspicious-rule")
	suspicious.Presets = []rules.Preset{rules.PresetSuspicious}
	suspicious.DefaultSeverity = rules.SeverityOff
	registry, err := rules.NewRegistry(
		metadataRule{metadata: correctness},
		metadataRule{metadata: suspicious},
	)
	if err != nil {
		t.Fatal(err)
	}

	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetCorrectness},
			Overrides: map[string]rules.Severity{"correctness-rule": rules.SeverityOff},
			Only: []string{"correctness-rule", "suspicious-rule"},
			Except: []string{"correctness-rule"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 1 ||
		selection[0].ID != "suspicious-rule" ||
		selection[0].Severity != rules.SeverityWarn {
		t.Fatalf("ResolveOptions(filtered) = %#v", selection)
	}

	selection, err = registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetCorrectness},
			Overrides: map[string]rules.Severity{"correctness-rule": rules.SeverityOff},
			Only: []string{"correctness-rule"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 1 ||
		selection[0].ID != "correctness-rule" ||
		selection[0].Severity != rules.SeverityWarn {
		t.Fatalf("ResolveOptions(only overrides off) = %#v", selection)
	}

	for _, options := range
		[]rules.ResolveOptions{
			{Only: []string{"missing-rule"}},
			{Except: []string{"missing-rule"}},
			{Only: []string{"correctness-rule", "correctness-rule"}},
			{Except: []string{"correctness-rule", "correctness-rule"}},
		} {
		if _, err := registry.ResolveOptions(options); err == nil {
			t.Fatalf("ResolveOptions(%#v) succeeded", options)
		}
	}
}

func TestRegistryComposesPresetGroupsAndEscalatesWarnings(t *testing.T) {
	t.Parallel()

	correctness := validMetadata("correctness-rule")
	style := validMetadata("style-rule")
	style.Presets = []rules.Preset{rules.PresetStyle}
	pedantic := validMetadata("pedantic-rule")
	pedantic.Presets = []rules.Preset{rules.PresetPedantic}
	restriction := validMetadata("restriction-rule")
	restriction.Presets = []rules.Preset{rules.PresetRestriction}
	registry, err := rules.NewRegistry(
		metadataRule{metadata: restriction},
		metadataRule{metadata: style},
		metadataRule{metadata: pedantic},
		metadataRule{metadata: correctness},
	)
	if err != nil {
		t.Fatal(err)
	}

	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetCorrectness, rules.PresetPedantic},
			Overrides: map[string]rules.Severity{
				"restriction-rule": rules.SeverityWarn,
				"style-rule": rules.SeverityOff,
			},
			WarningsAsErrors: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 3 ||
		selection[0].ID != "correctness-rule" ||
		selection[0].Severity != rules.SeverityError ||
		selection[1].ID != "pedantic-rule" ||
		selection[1].Severity != rules.SeverityError ||
		selection[2].ID != "restriction-rule" ||
		selection[2].Severity != rules.SeverityError {
		t.Fatalf("ResolveOptions() = %#v", selection)
	}

	for _, presets := range
		[][]rules.Preset{
			{rules.PresetRestriction},
			{rules.PresetMigration},
			{rules.PresetStyle, rules.PresetStyle},
		} {
		if _, err := registry.ResolveOptions(rules.ResolveOptions{Presets: presets});
			err == nil {
			t.Fatalf("ResolveOptions() accepted preset selection %v", presets)
		}
	}
}

func TestRegistryAppliesOrderedClippyStyleLintLevels(t *testing.T) {
	t.Parallel()

	correctness := validMetadata("correctness-rule")
	performance := validMetadata("performance-rule")
	performance.Presets = []rules.Preset{rules.PresetPerformance}
	pedantic := validMetadata("pedantic-rule")
	pedantic.Presets = []rules.Preset{rules.PresetPedantic}
	registry, err := rules.NewRegistry(
		metadataRule{metadata: correctness},
		metadataRule{metadata: performance},
		metadataRule{metadata: pedantic},
	)
	if err != nil {
		t.Fatal(err)
	}

	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetCorrectness},
			LintLevels: []rules.LintLevelDirective{
				{Level: rules.LintWarn, Targets: []string{"performance"}},
				{Level: rules.LintDeny, Targets: []string{"correctness"}},
				{Level: rules.LintAllow, Targets: []string{"correctness"}},
				{Level: rules.LintAllow, Targets: []string{"pedantic"}},
				{Level: rules.LintForbid, Targets: []string{"pedantic-rule"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 2 ||
		selection[0].ID != "pedantic-rule" ||
		selection[0].Severity != rules.SeverityError ||
		selection[1].ID != "performance-rule" ||
		selection[1].Severity != rules.SeverityWarn {
		t.Fatalf("ResolveOptions(lint levels) = %#v", selection)
	}

	selection, err = registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetCorrectness},
			WarningsAsErrors: true,
			LintLevels: []rules.LintLevelDirective{
				{Level: rules.LintWarn, Targets: []string{"performance"}},
				{Level: rules.LintAllow, Targets: []string{"correctness"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 1 ||
		selection[0].ID != "performance-rule" ||
		selection[0].Severity != rules.SeverityError {
		t.Fatalf("ResolveOptions(escalated lint levels) = %#v", selection)
	}
}

func TestRegistryRejectsLoweringAForbiddenLintLevel(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(
		metadataRule{metadata: validMetadata("correctness-rule")},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.ResolveOptions(
		rules.ResolveOptions{
			LintLevels: []rules.LintLevelDirective{
				{Level: rules.LintForbid, Targets: []string{"correctness-rule"}},
				{Level: rules.LintAllow, Targets: []string{"correctness"}},
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "cannot lower forbidden rule") {
		t.Fatalf("ResolveOptions(forbid lowering) error = %v", err)
	}
}

func TestRegistryKeepsWarningsTargetScopedToEnabledWarnings(t *testing.T) {
	t.Parallel()

	correctness := validMetadata("correctness-rule")
	disabled := validMetadata("disabled-rule")
	disabled.Presets = []rules.Preset{rules.PresetSuspicious}
	registry, err := rules.NewRegistry(
		metadataRule{metadata: disabled},
		metadataRule{metadata: correctness},
	)
	if err != nil {
		t.Fatal(err)
	}

	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetCorrectness},
			LintLevels: []rules.LintLevelDirective{
				{Level: rules.LintDeny, Targets: []string{"warnings"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 1 ||
		selection[0].ID != "correctness-rule" ||
		selection[0].Severity != rules.SeverityError {
		t.Fatalf("ResolveOptions(warnings) = %#v", selection)
	}
}

func TestRegistryRejectsInvalidLintLevelTargets(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(
		metadataRule{metadata: validMetadata("correctness-rule")},
	)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		directive rules.LintLevelDirective
		want string
	}{
		{
			name: "unknown",
			directive: rules.LintLevelDirective{
				Level: rules.LintWarn,
				Targets: []string{"missing-rule"},
			},
			want: "unknown lint level target",
		},
		{
			name: "restriction group",
			directive: rules.LintLevelDirective{
				Level: rules.LintWarn,
				Targets: []string{"restriction"},
			},
			want: "restriction lint level must target exact rule IDs",
		},
		{
			name: "migration group",
			directive: rules.LintLevelDirective{
				Level: rules.LintWarn,
				Targets: []string{"migration"},
			},
			want: "migration lint level requires an explicit target",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				_, err := registry.ResolveOptions(
					rules.ResolveOptions{
						LintLevels: []rules.LintLevelDirective{
							test.directive,
						},
					},
				)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"ResolveOptions(%s) error = %v, want %q",
						test.name,
						err,
						test.want,
					)
				}
			},
		)
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
		_, err := registry.Resolve(
			rules.PresetCorrectness,
			map[string]rules.Severity{"z-last": "invalid-z", "a-first": "invalid-a"},
		)
		if err == nil || !strings.Contains(err.Error(), "invalid-a") {
			t.Fatalf("Resolve() first invalid override error = %v", err)
		}
	}
}

func TestRegistryResolvesTypedRuleOptionsForEnabledRules(t *testing.T) {
	t.Parallel()

	metadata := validMetadata("configured-rule")
	metadata.Options[0].Required = true
	metadata.Options[0].Default = nil
	registry, err := rules.NewRegistry(metadataRule{metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	configured := rules.NewOptionSet(
		map[string]rules.OptionValue{"allow-comment": rules.BooleanOption(true)},
	)
	selection, err := registry.ResolveConfigured(
		rules.PresetCorrectness,
		nil,
		map[string]rules.OptionSet{"configured-rule": configured},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 1 {
		t.Fatalf("ResolveConfigured() = %#v", selection)
	}
	allow, found := selection[0].Options.Boolean("allow-comment")
	if !found || !allow {
		t.Fatalf("resolved allow-comment = %t, %t", allow, found)
	}

	if _, err := registry.ResolveConfigured(rules.PresetCorrectness, nil, nil);
		err == nil || !strings.Contains(err.Error(), "required option \"allow-comment\"") {
		t.Fatalf("ResolveConfigured() missing required option error = %v", err)
	}
	disabled, err := registry.ResolveConfigured(rules.PresetSuspicious, nil, nil)
	if err != nil || len(disabled) != 0 {
		t.Fatalf("ResolveConfigured() disabled required option = %#v, %v", disabled, err)
	}
	unknown := rules.NewOptionSet(
		map[string]rules.OptionValue{"unknown": rules.BooleanOption(true)},
	)
	if _, err := registry.ResolveConfigured(
		rules.PresetCorrectness,
		nil,
		map[string]rules.OptionSet{"configured-rule": unknown},
	);
		err == nil || !strings.Contains(err.Error(), "unknown option \"unknown\"") {
		t.Fatalf("ResolveConfigured() unknown option error = %v", err)
	}
	wrongKind := rules.NewOptionSet(
		map[string]rules.OptionValue{"allow-comment": rules.StringOption("yes")},
	)
	if _, err := registry.ResolveConfigured(
		rules.PresetCorrectness,
		nil,
		map[string]rules.OptionSet{"configured-rule": wrongKind},
	);
		err == nil ||
			!strings.Contains(err.Error(), "has kind \"string\"; want \"boolean\"") {
		t.Fatalf("ResolveConfigured() wrong option kind error = %v", err)
	}
}

func TestRegistryAppliesDocumentedOptionDefaults(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(metadataRule{metadata: validMetadata("defaulted-rule")})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.ResolveConfigured(rules.PresetCorrectness, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	value, found := selection[0].Options.Boolean("allow-comment")
	if !found || value {
		t.Fatalf("default allow-comment = %t, %t", value, found)
	}
}

func TestRegistryFiltersRulesBySelectedSourceVersion(t *testing.T) {
	t.Parallel()

	older := validMetadata("older-rule")
	older.MinimumGoVersion = "1.25"
	newer := validMetadata("newer-rule")
	newer.MinimumGoVersion = "1.26"
	registry, err := rules.NewRegistry(
		metadataRule{metadata: newer},
		metadataRule{metadata: older},
	)
	if err != nil {
		t.Fatal(err)
	}

	selection, err := registry.ResolveConfiguredForGoVersion(
		rules.PresetCorrectness,
		nil,
		nil,
		"go1.25",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 1 || selection[0].ID != "older-rule" {
		t.Fatalf("ResolveConfiguredForGoVersion() = %#v, want older-rule only", selection)
	}

	if _, err := registry.ResolveConfiguredForGoVersion(
		rules.PresetCorrectness,
		nil,
		nil,
		"1.25",
	);
		err == nil || !strings.Contains(err.Error(), "invalid source Go version") {
		t.Fatalf("ResolveConfiguredForGoVersion() invalid version error = %v", err)
	}
}

func validMetadata(id string) rules.Metadata {
	return rules.Metadata{
		ID: id,
		Summary: "reports one observable defect",
		Documentation: "Full rule documentation.",
		DefaultSeverity: rules.SeverityWarn,
		Presets: []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement: rules.RequireSyntax,
		NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
		RunOnGenerated: false,
		Categories: []rules.Category{rules.CategoryCorrectness},
		Fixes: []rules.FixMetadata{
			{
				Name: "remove-empty",
				Description: "remove the ineffective expression",
				Safety: rules.FixSafe,
			},
		},
		Options: []rules.OptionMetadata{
			{
				Name: "allow-comment",
				Summary: "allow an explanatory comment",
				Kind: rules.OptionBoolean,
				Default: optionValue(rules.BooleanOption(false)),
			},
		},
		KnownLimitations: []string{"does not inspect generated files"},
		Examples: []rules.Example{{Incorrect: "empty()", Correct: "work()"}},
	}
}

func optionValue(value rules.OptionValue) *rules.OptionValue {
	return &value
}

func int64Pointer(value int64) *int64 {
	return &value
}
