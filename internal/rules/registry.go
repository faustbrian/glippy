package rules

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var (
	ruleIDPattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	goVersionPattern = regexp.MustCompile(`^1\.(?:0|[1-9][0-9]*)$`)
)

type registryEntry struct {
	rule     Rule
	metadata Metadata
}

// Registry is an immutable, ID-ordered native rule set.
type Registry struct {
	ids     []string
	entries map[string]registryEntry
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

// NewDefaultRegistry constructs the canonical built-in rule registry.
func NewDefaultRegistry() (*Registry, error) {
	return NewRegistry(
		contextKeyRule{},
		deferInInfiniteLoopRule{},
		duplicateConditionRule{},
		errorsIsArgumentsRule{},
		ineffectiveBreakRule{},
		nilnessRule{},
		redundantBoolComparisonRule{},
	)
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
	if r == nil {
		return nil, fmt.Errorf("resolve requires a registry")
	}
	if !validPreset(preset) {
		return nil, fmt.Errorf("unknown preset %q", preset)
	}
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
	optionRuleIDs := make([]string, 0, len(options))
	for id := range options {
		optionRuleIDs = append(optionRuleIDs, id)
	}
	sort.Strings(optionRuleIDs)
	for _, id := range optionRuleIDs {
		entry, found := r.entries[id]
		if !found {
			return nil, fmt.Errorf("unknown rule %q in rule options", id)
		}
		if err := validateOptionSet(entry.metadata, options[id]); err != nil {
			return nil, err
		}
	}
	selection := make([]Selection, 0, len(r.ids))
	for _, id := range r.ids {
		metadata := r.entries[id].metadata
		severity := SeverityOff
		if slices.Contains(metadata.Presets, preset) {
			severity = metadata.DefaultSeverity
		}
		if override, found := overrides[id]; found {
			severity = override
		}
		if severity == SeverityOff {
			continue
		}
		configured := options[id]
		resolvedValues := make(map[string]OptionValue, len(metadata.Options))
		for _, option := range metadata.Options {
			value, found := configured.values[option.Name]
			if option.Required && !found {
				return nil, fmt.Errorf("rule %q is missing required option %q", id, option.Name)
			}
			if !found && option.Default != nil {
				value, found = *option.Default, true
			}
			if found {
				resolvedValues[option.Name] = value
			}
		}
		selection = append(selection, Selection{
			ID:          id,
			Severity:    severity,
			Requirement: metadata.Requirement,
			Options:     NewOptionSet(resolvedValues),
		})
	}
	return selection, nil
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
		if value.kind != option.Kind {
			return fmt.Errorf(
				"rule %q option %q has kind %q; want %q",
				metadata.ID,
				name,
				value.kind,
				option.Kind,
			)
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
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
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
		return fmt.Errorf("%s: invalid default severity %q", metadata.ID, metadata.DefaultSeverity)
	}
	if len(metadata.Presets) == 0 {
		return fmt.Errorf("%s: at least one preset is required", metadata.ID)
	}
	if err := validateUnique(metadata.Presets, validPreset, "preset"); err != nil {
		return fmt.Errorf("%s: %w", metadata.ID, err)
	}
	if !goVersionPattern.MatchString(metadata.MinimumGoVersion) {
		return fmt.Errorf("%s: invalid minimum Go version %q", metadata.ID, metadata.MinimumGoVersion)
	}
	if metadata.Requirement < RequireLexical || metadata.Requirement > RequireSSA {
		return fmt.Errorf("%s: invalid analysis requirement %d", metadata.ID, metadata.Requirement)
	}
	if metadata.RunDespiteTypeErrors && metadata.Requirement < RequireTypes {
		return fmt.Errorf("%s: cheap-tier rule cannot opt into type-error packages", metadata.ID)
	}
	if packageWide && metadata.Requirement != RequireTypes {
		return fmt.Errorf("%s: package-wide rule must require types", metadata.ID)
	}
	if metadata.RequiresDependencySyntax && (!packageWide || metadata.Requirement != RequireTypes) {
		return fmt.Errorf("%s: dependency syntax requires a package-wide types rule", metadata.ID)
	}
	if metadata.Requirement == RequireSyntax && len(metadata.NodeInterests) == 0 {
		return fmt.Errorf("%s: %s rule must declare node interests", metadata.ID, metadata.Requirement)
	}
	if metadata.Requirement == RequireTypes && packageWide && len(metadata.NodeInterests) != 0 {
		return fmt.Errorf("%s: package-wide rule must not declare node interests", metadata.ID)
	}
	if metadata.Requirement == RequireTypes && !packageWide && len(metadata.NodeInterests) == 0 {
		return fmt.Errorf("%s: types rule must declare node interests", metadata.ID)
	}
	if metadata.Requirement == RequireControlFlow && len(metadata.NodeInterests) != 0 {
		return fmt.Errorf("%s: control flow rule must not declare node interests", metadata.ID)
	}
	if metadata.Requirement == RequireSSA && len(metadata.NodeInterests) != 0 {
		return fmt.Errorf("%s: SSA rule must not declare node interests", metadata.ID)
	}
	if metadata.Requirement == RequireSSA && metadata.RunDespiteTypeErrors {
		return fmt.Errorf("%s: SSA rule cannot run on type-error packages", metadata.ID)
	}
	if err := validateUnique(metadata.NodeInterests, validNodeKind, "node interest"); err != nil {
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
			return fmt.Errorf("%s: required option %q must not declare a default", metadata.ID, option.Name)
		}
		if !option.Required && option.Default == nil {
			return fmt.Errorf("%s: optional option %q requires a default", metadata.ID, option.Name)
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
		if strings.TrimSpace(example.Incorrect) == "" || strings.TrimSpace(example.Correct) == "" {
			return fmt.Errorf("%s: examples require incorrect and correct source", metadata.ID)
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
	case PresetCorrectness, PresetSuspicious, PresetPerformance,
		PresetComplexity, PresetStyle, PresetMigration:
		return true
	default:
		return false
	}
}

func validCategory(value Category) bool {
	switch value {
	case CategoryCorrectness, CategorySafety, CategorySuspicious,
		CategoryPerformance, CategoryComplexity, CategoryStyle,
		CategoryMigration, CategoryMaintainability:
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
