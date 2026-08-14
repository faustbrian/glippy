package rules

import (
	"fmt"
	"go/version"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var (
	ruleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	goVersionPattern = regexp.MustCompile(`^1\.(?:0|[1-9][0-9]*)$`)
)

type registryEntry struct {
	rule Rule
	metadata Metadata
}

// Registry is an immutable, ID-ordered native rule set.
type Registry struct {
	ids []string
	entries map[string]registryEntry
}

// ResolveOptions is one complete, immutable rule-selection policy.
type ResolveOptions struct {
	Presets []Preset
	Overrides map[string]Severity
	RuleOptions map[string]OptionSet
	SourceGoVersion string
	WarningsAsErrors bool
	LintLevels []LintLevelDirective
	Only []string
	Except []string
}

// CloneMetadata returns an independent copy of rule metadata.
func CloneMetadata(metadata Metadata) Metadata {
	return cloneMetadata(metadata)
}

// NewRegistry validates and snapshots native rules.
func NewRegistry(nativeRules ...Rule) (*Registry, error) {
	entries := make(map[string]registryEntry, len(nativeRules))
	for index, nativeRule := range nativeRules {
		if nilRule(nativeRule) {
			return nil, fmt.Errorf("rule %d is nil", index)
		}
		metadata := cloneMetadata(nativeRule.Metadata())
		_, packageWide := nativeRule.(PackageRule)
		if err := validateMetadata(metadata, packageWide); err != nil {
			return nil, fmt.Errorf("rule %d: %w", index, err)
		}
		if _, exists := entries[metadata.ID]; exists {
			return nil, fmt.Errorf("duplicate rule ID %q", metadata.ID)
		}
		entries[metadata.ID] = registryEntry{rule: nativeRule, metadata: metadata}
	}
	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return &Registry{ids: ids, entries: entries}, nil
}

// NewDefaultRegistry constructs the native built-in rule registry.
func NewDefaultRegistry() (*Registry, error) {
	return NewRegistry(DefaultRules()...)
}

// DefaultRules returns the native built-in rules for product-level registry
// composition with adapted analyzers.
func DefaultRules() []Rule {
	return []Rule{
		contextKeyRule{},
		deferInInfiniteLoopRule{},
		duplicateConditionRule{},
		errorsIsArgumentsRule{},
		identicalBranchesRule{},
		ineffectiveBreakRule{},
		nilnessRule{},
		redundantBoolComparisonRule{},
	}
}

// IDs returns registered IDs in canonical order.
func (r *Registry) IDs() []string {
	if r == nil {
		return nil
	}
	return slices.Clone(r.ids)
}

// Lookup returns one registered native rule.
func (r *Registry) Lookup(id string) (Rule, bool) {
	if r == nil {
		return nil, false
	}
	entry, found := r.entries[id]
	return entry.rule, found
}

// Metadata returns an independent metadata copy for one rule.
func (r *Registry) Metadata(id string) (Metadata, bool) {
	if r == nil {
		return Metadata{}, false
	}
	entry, found := r.entries[id]
	if !found {
		return Metadata{}, false
	}
	return cloneMetadata(entry.metadata), true
}

// OptionSchemas returns independent typed option metadata by rule ID.
func (r *Registry) OptionSchemas() map[string][]OptionMetadata {
	if r == nil {
		return nil
	}
	result := make(map[string][]OptionMetadata, len(r.entries))
	for _, id := range r.ids {
		result[id] = cloneOptionMetadata(r.entries[id].metadata.Options)
	}
	return result
}

// Resolve applies one preset and explicit overrides into an ordered selection.
func (r *Registry) Resolve(preset Preset, overrides map[string]Severity) ([]Selection, error) {
	return r.ResolveConfigured(preset, overrides, nil)
}

// ResolveConfigured applies severity and typed option configuration into an
// ordered immutable selection.
func (r *Registry) ResolveConfigured(
	preset Preset,
	overrides map[string]Severity,
	options map[string]OptionSet,
) ([]Selection, error) {
	return r.ResolveConfiguredForGoVersion(preset, overrides, options, "")
}

// ResolveConfiguredForGoVersion applies configuration and omits rules whose
// declared minimum exceeds the selected normalized source language version.
// An empty source version retains the version-independent registry contract.
func (r *Registry) ResolveConfiguredForGoVersion(
	preset Preset,
	overrides map[string]Severity,
	options map[string]OptionSet,
	sourceGoVersion string,
) ([]Selection, error) {
	return r.ResolveOptions(
		ResolveOptions{
			Presets: []Preset{preset},
			Overrides: overrides,
			RuleOptions: options,
			SourceGoVersion: sourceGoVersion,
		},
	)
}

// ResolveOptions composes preset groups, applies explicit rule policy, and
// returns one ID-ordered selection. Restriction rules remain individually
// selectable through overrides, while migration requires a separate target
// contract before its group can be selected.
func (r *Registry) ResolveOptions(options ResolveOptions) ([]Selection, error) {
	if r == nil {
		return nil, fmt.Errorf("resolve requires a registry")
	}
	sourceGoVersion := options.SourceGoVersion
	if sourceGoVersion != "" &&
		(!version.IsValid(sourceGoVersion) ||
			version.Lang(sourceGoVersion) != sourceGoVersion) {
		return nil, fmt.Errorf("invalid source Go version %q", sourceGoVersion)
	}
	selectedPresets := make(map[Preset]struct{}, len(options.Presets))
	for _, preset := range options.Presets {
		if !validPreset(preset) {
			return nil, fmt.Errorf("unknown preset %q", preset)
		}
		if preset == PresetRestriction {
			return nil, fmt.Errorf("restriction preset must be enabled rule by rule")
		}
		if preset == PresetMigration {
			return nil, fmt.Errorf("migration preset requires an explicit target")
		}
		if _, duplicate := selectedPresets[preset]; duplicate {
			return nil, fmt.Errorf("duplicate preset %q", preset)
		}
		selectedPresets[preset] = struct{}{}
	}
	overrides := options.Overrides
	overrideIDs := make([]string, 0, len(overrides))
	for id := range overrides {
		overrideIDs = append(overrideIDs, id)
	}
	sort.Strings(overrideIDs)
	for _, id := range overrideIDs {
		severity := overrides[id]
		if _, found := r.entries[id]; !found {
			return nil, fmt.Errorf("unknown rule %q", id)
		}
		if !validSeverity(severity) {
			return nil, fmt.Errorf("invalid severity %q for rule %q", severity, id)
		}
	}
	ruleOptions := options.RuleOptions
	optionRuleIDs := make([]string, 0, len(ruleOptions))
	for id := range ruleOptions {
		optionRuleIDs = append(optionRuleIDs, id)
	}
	sort.Strings(optionRuleIDs)
	for _, id := range optionRuleIDs {
		entry, found := r.entries[id]
		if !found {
			return nil, fmt.Errorf("unknown rule %q in rule options", id)
		}
		if err := validateOptionSet(entry.metadata, ruleOptions[id]); err != nil {
			return nil, err
		}
	}
	only, err := r.validateRuleFilter("only", options.Only)
	if err != nil {
		return nil, err
	}
	except, err := r.validateRuleFilter("except", options.Except)
	if err != nil {
		return nil, err
	}
	levelTargets, err := r.validateLintLevels(options.LintLevels)
	if err != nil {
		return nil, err
	}
	severities := make(map[string]Severity, len(r.ids))
	eligible := make(map[string]struct{}, len(r.ids))
	forbidden := make(map[string]struct{})
	for _, id := range r.ids {
		metadata := r.entries[id].metadata
		severity := SeverityOff
		for _, preset := range metadata.Presets {
			if _, enabled := selectedPresets[preset]; enabled {
				severity = metadata.DefaultSeverity
				break
			}
		}
		if override, found := overrides[id]; found {
			severity = override
		}
		if len(only) > 0 {
			if _, selected := only[id]; !selected {
				continue
			}
			if severity == SeverityOff {
				severity = metadata.DefaultSeverity
				if severity == SeverityOff {
					severity = SeverityWarn
				}
			}
		}
		if _, excluded := except[id]; excluded {
			continue
		}
		eligible[id] = struct{}{}
		severities[id] = severity
	}
	for index, directive := range options.LintLevels {
		matched := slices.Clone(levelTargets[index].ids)
		if levelTargets[index].warnings {
			for _, id := range r.ids {
				if severities[id] == SeverityWarn {
					matched = append(matched, id)
				}
			}
		}
		slices.Sort(matched)
		matched = slices.Compact(matched)
		for _, id := range matched {
			if _, applies := eligible[id]; !applies {
				continue
			}
			if _, locked := forbidden[id]; locked {
				if directive.Level == LintAllow || directive.Level == LintWarn {
					return nil, fmt.Errorf(
						"cannot lower forbidden rule %q to %s",
						id,
						directive.Level,
					)
				}
				continue
			}
			switch directive.Level {
			case LintAllow:
				severities[id] = SeverityOff
			case LintWarn:
				severities[id] = SeverityWarn
			case LintDeny:
				severities[id] = SeverityError
			case LintForbid:
				severities[id] = SeverityError
				forbidden[id] = struct{}{}
			}
		}
	}
	selection := make([]Selection, 0, len(r.ids))
	for _, id := range r.ids {
		if _, selected := eligible[id]; !selected {
			continue
		}
		metadata := r.entries[id].metadata
		severity := severities[id]
		if severity == SeverityOff {
			continue
		}
		if options.WarningsAsErrors && severity == SeverityWarn {
			severity = SeverityError
		}
		if sourceGoVersion != "" &&
			version.Compare(sourceGoVersion, "go" + metadata.MinimumGoVersion) < 0 {
			continue
		}
		configured := ruleOptions[id]
		resolvedValues := make(map[string]OptionValue, len(metadata.Options))
		for _, option := range metadata.Options {
			value, found := configured.values[option.Name]
			if option.Required && !found {
				return nil, fmt.Errorf(
					"rule %q is missing required option %q",
					id,
					option.Name,
				)
			}
			if !found && option.Default != nil {
				value, found = *option.Default, true
			}
			if found {
				resolvedValues[option.Name] = value
			}
		}
		selection = append(
			selection,
			Selection{
				ID: id,
				Severity: severity,
				Requirement: metadata.Requirement,
				Options: NewOptionSet(resolvedValues),
			},
		)
	}
	return selection, nil
}

type validatedLintLevelTargets struct {
	ids []string
	warnings bool
}

func (r *Registry) validateLintLevels(
	directives []LintLevelDirective,
) ([]validatedLintLevelTargets, error) {
	result := make([]validatedLintLevelTargets, len(directives))
	for index, directive := range directives {
		if !validLintLevel(directive.Level) {
			return nil, fmt.Errorf("invalid lint level %q", directive.Level)
		}
		if len(directive.Targets) == 0 {
			return nil, fmt.Errorf(
				"lint level %q requires at least one target",
				directive.Level,
			)
		}
		seen := make(map[string]struct{}, len(directive.Targets))
		matched := make(map[string]struct{})
		for _, target := range directive.Targets {
			if _, duplicate := seen[target]; duplicate {
				return nil, fmt.Errorf("duplicate lint level target %q", target)
			}
			seen[target] = struct{}{}
			if target == "warnings" {
				result[index].warnings = true
				continue
			}
			preset := Preset(target)
			if validPreset(preset) {
				if preset == PresetRestriction {
					return nil, fmt.Errorf(
						"restriction lint level must target exact rule IDs",
					)
				}
				if preset == PresetMigration {
					return nil, fmt.Errorf(
						"migration lint level requires an explicit target",
					)
				}
				for _, id := range r.ids {
					if slices.Contains(r.entries[id].metadata.Presets, preset) {
						matched[id] = struct{}{}
					}
				}
				continue
			}
			if _, found := r.entries[target]; !found {
				return nil, fmt.Errorf("unknown lint level target %q", target)
			}
			matched[target] = struct{}{}
		}
		result[index].ids = make([]string, 0, len(matched))
		for id := range matched {
			result[index].ids = append(result[index].ids, id)
		}
		sort.Strings(result[index].ids)
	}
	return result, nil
}

func validLintLevel(level LintLevel) bool {
	switch level {
	case LintAllow, LintWarn, LintDeny, LintForbid:
		return true
	default:
		return false
	}
}

func (r *Registry) validateRuleFilter(name string, ids []string) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, found := r.entries[id]; !found {
			return nil, fmt.Errorf("unknown rule %q in %s filter", id, name)
		}
		if _, duplicate := result[id]; duplicate {
			return nil, fmt.Errorf("duplicate rule %q in %s filter", id, name)
		}
		result[id] = struct{}{}
	}
	return result, nil
}

func validateOptionSet(metadata Metadata, configured OptionSet) error {
	schema := make(map[string]OptionMetadata, len(metadata.Options))
	for _, option := range metadata.Options {
		schema[option.Name] = option
	}
	names := make([]string, 0, len(configured.values))
	for name := range configured.values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := configured.values[name]
		option, found := schema[name]
		if !found {
			return fmt.Errorf("rule %q has unknown option %q", metadata.ID, name)
		}
		if err := ValidateOptionValue(option, value); err != nil {
			return fmt.Errorf("rule %q option %q %w", metadata.ID, name, err)
		}
	}
	return nil
}

func nilRule(nativeRule Rule) bool {
	if nativeRule == nil {
		return true
	}
	value := reflect.ValueOf(nativeRule)
	switch value.Kind() {
	case reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Map,
		reflect.Pointer,
		reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// MaximumRequirement returns the most expensive selected representation.
func MaximumRequirement(selection []Selection) Requirement {
	maximum := RequireLexical
	for _, selected := range selection {
		if selected.Requirement > maximum {
			maximum = selected.Requirement
		}
	}
	return maximum
}

func validateMetadata(metadata Metadata, packageWide bool) error {
	if !ruleIDPattern.MatchString(metadata.ID) {
		return fmt.Errorf("invalid rule ID %q", metadata.ID)
	}
	if strings.TrimSpace(metadata.Summary) == "" {
		return fmt.Errorf("%s: summary is required", metadata.ID)
	}
	if strings.TrimSpace(metadata.Documentation) == "" {
		return fmt.Errorf("%s: documentation is required", metadata.ID)
	}
	if !validSeverity(metadata.DefaultSeverity) {
		return fmt.Errorf(
			"%s: invalid default severity %q",
			metadata.ID,
			metadata.DefaultSeverity,
		)
	}
	if len(metadata.Presets) == 0 {
		return fmt.Errorf("%s: at least one preset is required", metadata.ID)
	}
	if err := validateUnique(metadata.Presets, validPreset, "preset"); err != nil {
		return fmt.Errorf("%s: %w", metadata.ID, err)
	}
	if !goVersionPattern.MatchString(metadata.MinimumGoVersion) {
		return fmt.Errorf(
			"%s: invalid minimum Go version %q",
			metadata.ID,
			metadata.MinimumGoVersion,
		)
	}
	if metadata.Requirement > RequireSSA {
		return fmt.Errorf(
			"%s: invalid analysis requirement %d",
			metadata.ID,
			metadata.Requirement,
		)
	}
	if metadata.RunDespiteTypeErrors && metadata.Requirement < RequireTypes {
		return fmt.Errorf(
			"%s: cheap-tier rule cannot opt into type-error packages",
			metadata.ID,
		)
	}
	if packageWide && metadata.Requirement != RequireTypes {
		return fmt.Errorf("%s: package-wide rule must require types", metadata.ID)
	}
	if metadata.RequiresDependencySyntax &&
		(!packageWide || metadata.Requirement != RequireTypes) {
		return fmt.Errorf(
			"%s: dependency syntax requires a package-wide types rule",
			metadata.ID,
		)
	}
	if metadata.Requirement == RequireSyntax && len(metadata.NodeInterests) == 0 {
		return fmt.Errorf(
			"%s: %s rule must declare node interests",
			metadata.ID,
			metadata.Requirement,
		)
	}
	if metadata.Requirement == RequireTypes && packageWide && len(metadata.NodeInterests) != 0 {
		return fmt.Errorf(
			"%s: package-wide rule must not declare node interests",
			metadata.ID,
		)
	}
	if metadata.Requirement == RequireTypes &&
		!packageWide &&
		len(metadata.NodeInterests) == 0 {
		return fmt.Errorf("%s: types rule must declare node interests", metadata.ID)
	}
	if metadata.Requirement == RequireControlFlow && len(metadata.NodeInterests) != 0 {
		return fmt.Errorf(
			"%s: control flow rule must not declare node interests",
			metadata.ID,
		)
	}
	if metadata.Requirement == RequireSSA && len(metadata.NodeInterests) != 0 {
		return fmt.Errorf("%s: SSA rule must not declare node interests", metadata.ID)
	}
	if metadata.Requirement == RequireSSA && metadata.RunDespiteTypeErrors {
		return fmt.Errorf("%s: SSA rule cannot run on type-error packages", metadata.ID)
	}
	if err := validateUnique(metadata.NodeInterests, validNodeKind, "node interest");
		err != nil {
		return fmt.Errorf("%s: %w", metadata.ID, err)
	}
	if len(metadata.Categories) == 0 {
		return fmt.Errorf("%s: at least one category is required", metadata.ID)
	}
	if err := validateUnique(metadata.Categories, validCategory, "category"); err != nil {
		return fmt.Errorf("%s: %w", metadata.ID, err)
	}
	fixNames := make(map[string]struct{}, len(metadata.Fixes))
	for _, fix := range metadata.Fixes {
		if strings.TrimSpace(fix.Name) == "" || strings.TrimSpace(fix.Description) == "" {
			return fmt.Errorf("%s: fix name and description are required", metadata.ID)
		}
		if !validFixSafety(fix.Safety) {
			return fmt.Errorf("%s: invalid fix safety %q", metadata.ID, fix.Safety)
		}
		if _, duplicate := fixNames[fix.Name]; duplicate {
			return fmt.Errorf("%s: duplicate fix name %q", metadata.ID, fix.Name)
		}
		fixNames[fix.Name] = struct{}{}
	}
	optionNames := make(map[string]struct{}, len(metadata.Options))
	for _, option := range metadata.Options {
		if strings.TrimSpace(option.Name) == "" || strings.TrimSpace(option.Summary) == "" {
			return fmt.Errorf("%s: option name and summary are required", metadata.ID)
		}
		if !validOptionKind(option.Kind) {
			return fmt.Errorf("%s: invalid option kind %q", metadata.ID, option.Kind)
		}
		if option.Required && option.Default != nil {
			return fmt.Errorf(
				"%s: required option %q must not declare a default",
				metadata.ID,
				option.Name,
			)
		}
		if !option.Required && option.Default == nil {
			return fmt.Errorf(
				"%s: optional option %q requires a default",
				metadata.ID,
				option.Name,
			)
		}
		if option.Default != nil && option.Default.kind != option.Kind {
			return fmt.Errorf(
				"%s: option %q default has kind %q; want %q",
				metadata.ID,
				option.Name,
				option.Default.kind,
				option.Kind,
			)
		}
		if (option.Minimum != nil || option.Maximum != nil) &&
			option.Kind != OptionInteger {
			return fmt.Errorf(
				"%s: option %q bounds require integer kind",
				metadata.ID,
				option.Name,
			)
		}
		if option.Minimum != nil &&
			option.Maximum != nil &&
			*option.Minimum > *option.Maximum {
			return fmt.Errorf(
				"%s: option %q minimum %d exceeds maximum %d",
				metadata.ID,
				option.Name,
				*option.Minimum,
				*option.Maximum,
			)
		}
		if option.Default != nil {
			if err := ValidateOptionValue(option, *option.Default); err != nil {
				return fmt.Errorf(
					"%s: option %q default %s %w",
					metadata.ID,
					option.Name,
					option.Default.String(),
					err,
				)
			}
		}
		if _, duplicate := optionNames[option.Name]; duplicate {
			return fmt.Errorf("%s: duplicate option name %q", metadata.ID, option.Name)
		}
		optionNames[option.Name] = struct{}{}
	}
	if metadata.Deprecation != nil && strings.TrimSpace(metadata.Deprecation.Message) == "" {
		return fmt.Errorf("%s: deprecation message is required", metadata.ID)
	}
	for _, limitation := range metadata.KnownLimitations {
		if strings.TrimSpace(limitation) == "" {
			return fmt.Errorf("%s: known limitations must not be empty", metadata.ID)
		}
	}
	if len(metadata.Examples) == 0 {
		return fmt.Errorf("%s: at least one example is required", metadata.ID)
	}
	for _, example := range metadata.Examples {
		if strings.TrimSpace(example.Incorrect) == "" ||
			strings.TrimSpace(example.Correct) == "" {
			return fmt.Errorf(
				"%s: examples require incorrect and correct source",
				metadata.ID,
			)
		}
	}
	return nil
}

func validateUnique[T comparable](values []T, valid func(T) bool, label string) error {
	seen := make(map[T]struct{}, len(values))
	for _, value := range values {
		if !valid(value) {
			return fmt.Errorf("invalid %s %v", label, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("duplicate %s %v", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validSeverity(value Severity) bool {
	switch value {
	case SeverityOff, SeverityWarn, SeverityError:
		return true
	default:
		return false
	}
}

func validPreset(value Preset) bool {
	switch value {
	case PresetCorrectness,
		PresetSuspicious,
		PresetPerformance,
		PresetComplexity,
		PresetStyle,
		PresetPedantic,
		PresetRestriction,
		PresetMigration:
		return true
	default:
		return false
	}
}

func validCategory(value Category) bool {
	switch value {
	case CategoryCorrectness,
		CategorySafety,
		CategorySuspicious,
		CategoryPerformance,
		CategoryComplexity,
		CategoryStyle,
		CategoryMigration,
		CategoryMaintainability:
		return true
	default:
		return false
	}
}

func validFixSafety(value FixSafety) bool {
	switch value {
	case FixSafe, FixSuggestion, FixUnsafe:
		return true
	default:
		return false
	}
}

func validOptionKind(value OptionKind) bool {
	switch value {
	case OptionBoolean, OptionInteger, OptionString, OptionStrings:
		return true
	default:
		return false
	}
}

func validNodeKind(value NodeKind) bool {
	_, valid := NodePrototype(value)
	return valid
}

func cloneMetadata(metadata Metadata) Metadata {
	result := metadata
	result.Presets = slices.Clone(metadata.Presets)
	result.NodeInterests = slices.Clone(metadata.NodeInterests)
	result.Categories = slices.Clone(metadata.Categories)
	result.Fixes = slices.Clone(metadata.Fixes)
	result.Options = cloneOptionMetadata(metadata.Options)
	result.KnownLimitations = slices.Clone(metadata.KnownLimitations)
	result.Examples = slices.Clone(metadata.Examples)
	if metadata.Deprecation != nil {
		copy := *metadata.Deprecation
		result.Deprecation = &copy
	}
	return result
}
