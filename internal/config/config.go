// Package config owns typed Glippy configuration defaults and decoding.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"go/build"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/pelletier/go-toml/v2"

	"github.com/faustbrian/glippy/internal/baseline"
	"github.com/faustbrian/glippy/internal/contracts"
	"github.com/faustbrian/glippy/internal/filesystem"
	"github.com/faustbrian/glippy/internal/rules"
)

const (
	// Filename is the canonical project configuration filename.
	Filename = ".glippy.toml"
	// LegacyFilename is accepted as a deprecated fallback for the v0.2 compatibility window.
	LegacyFilename = ".gox.toml"
	// MaxAnalysisTargets bounds explicit CI build-matrix work from configuration.
	MaxAnalysisTargets = 32
)

// Version is the only accepted configuration schema version.
const Version = 1

const (
	// DefaultLineWidth is the formatter's default print width.
	DefaultLineWidth = 100
	// DefaultTabWidth is the formatter's default visual tab width.
	DefaultTabWidth = 8
	// DefaultCacheMaxEntries bounds retained persistent analysis results.
	DefaultCacheMaxEntries = 4096
	// DefaultCacheMaxBytes bounds retained persistent analysis storage.
	DefaultCacheMaxBytes int64 = 512 << 20
)

// Preset identifies one coherent lint-rule selection.
type Preset = rules.Preset

const (
	PresetCorrectness = rules.PresetCorrectness
	PresetSuspicious = rules.PresetSuspicious
	PresetPerformance = rules.PresetPerformance
	PresetComplexity = rules.PresetComplexity
	PresetStyle = rules.PresetStyle
	PresetPedantic = rules.PresetPedantic
	PresetNursery = rules.PresetNursery
	PresetRestriction = rules.PresetRestriction
	PresetMigration = rules.PresetMigration
)

// Profile identifies one curated, versioned lint policy.
type Profile string

const (
	ProfileDefault Profile = "default"
	ProfileRecommended Profile = "recommended"
	ProfileStrict Profile = "strict"
	ProfilePedantic Profile = "pedantic"
)

var recommendedProfileRules = []string{
	"almost-swapped",
	"defer-before-error-check",
	"defer-in-infinite-loop",
	"errors-is-arguments",
	"http-response-body-not-closed",
	"identical-branches",
	"ignored-append-result",
	"ineffective-value-receiver-assignment",
	"nil-error-wrap",
	"nilness",
	"overwritten-error",
	"shadowed-error",
	"subsumed-condition",
	"suspicious-range",
	"suspicious-string-conversion",
	"time-duration-unit",
	"typed-nil-error-return",
	"unchecked-rows-error",
	"unchecked-scanner-error",
}

// Profiles returns the canonical profile names in increasing strictness.
func Profiles() []Profile {
	return []Profile{ProfileDefault, ProfileRecommended, ProfileStrict, ProfilePedantic}
}

// ValidProfile reports whether value names one built-in profile.
func ValidProfile(value Profile) bool {
	switch value {
	case ProfileDefault, ProfileRecommended, ProfileStrict, ProfilePedantic:
		return true
	default:
		return false
	}
}

// Severity controls whether and how a lint rule reports.
type Severity = rules.Severity

const (
	SeverityOff = rules.SeverityOff
	SeverityWarn = rules.SeverityWarn
	SeverityError = rules.SeverityError
)

// Config is one fully defaulted and validated project configuration.
type Config struct {
	Version int
	Format Format
	Analysis Analysis
	Lint Lint
	Cache Cache
}

// Format contains formatter policy that materially affects adoption.
type Format struct {
	LineWidth int
	TabWidth int
}

// Analysis contains the resolved build selection for package-aware rules.
type Analysis struct {
	BuildTags []string
	GOOS string
	GOARCH string
	CGOEnabled bool
	Targets []AnalysisTarget
	ContractFiles []string
	Contracts contracts.Set
}

// AnalysisTarget is one explicit CI-oriented Go build selection.
type AnalysisTarget struct {
	BuildTags []string
	GOOS string
	GOARCH string
	CGOEnabled bool
}

// ID returns the stable machine and human identity of one target.
func (t AnalysisTarget) ID() string {
	result := t.GOOS + "/" + t.GOARCH
	if t.CGOEnabled {
		result += "+cgo"
	}
	if len(t.BuildTags) != 0 {
		result += "+tags=" + strings.Join(t.BuildTags, ",")
	}
	return result
}

// Lint contains the selected preset groups and explicit rule policy.
type Lint struct {
	Profile Profile
	ProfileRules map[string]Severity
	Presets []Preset
	WarningsAsErrors bool
	Rules map[string]Severity
	RuleOptions map[string]rules.OptionSet
	Overrides []LintOverride
	Suppressions Suppressions
	Baseline Baseline
}

// LintOverride applies exact rule severities to matching project-relative paths.
// Overrides retain declaration order because later matches replace earlier ones.
type LintOverride struct {
	Paths []string
	Rules map[string]Severity
}

// Baseline contains deterministic progressive-adoption policy.
type Baseline struct {
	Path string
	ReportStale bool
	ExpiryCutoff string
}

// Suppressions contains project policy for auditable lint waivers.
type Suppressions struct {
	RequireReason bool
	ExpiryCutoff string
}

// Cache contains the opt-in persistent analysis-cache lifecycle policy.
type Cache struct {
	Enabled bool
	MaxEntries int
	MaxBytes int64
}

// ParseOptions supplies registry state needed to validate rule identifiers.
type ParseOptions struct {
	KnownRules []string
	RuleOptions map[string][]rules.OptionMetadata
}

// Error is one path-aware configuration diagnostic.
type Error struct {
	Path string
	Line int
	Column int
	Message string
	cause error
}

// Error renders a source-located diagnostic when line information is known.
func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Unwrap returns the underlying TOML decoder failure when one exists.
func (e *Error) Unwrap() error {
	return e.cause
}

type fileConfig struct {
	Version *int `toml:"version"`
	Format formatConfig `toml:"format"`
	Analysis analysisConfig `toml:"analysis"`
	Lint lintConfig `toml:"lint"`
	Cache cacheConfig `toml:"cache"`
}

type formatConfig struct {
	LineWidth *int `toml:"line-width"`
	TabWidth *int `toml:"tab-width"`
}

type analysisConfig struct {
	BuildTags []string `toml:"build-tags"`
	GOOS *string `toml:"goos"`
	GOARCH *string `toml:"goarch"`
	CGOEnabled *bool `toml:"cgo-enabled"`
	Targets []analysisTargetConfig `toml:"targets"`
	ContractFiles []string `toml:"contract-files"`
}

type analysisTargetConfig struct {
	Tags []string `toml:"tags"`
	GOOS *string `toml:"goos"`
	GOARCH *string `toml:"goarch"`
	CGOEnabled *bool `toml:"cgo-enabled"`
}

type lintConfig struct {
	Profile *string `toml:"profile"`
	Preset *string `toml:"preset"`
	Presets []string `toml:"presets"`
	WarningsAsErrors *bool `toml:"warnings-as-errors"`
	Rules map[string]string `toml:"rules"`
	RuleOptions map[string]map[string]any `toml:"rule-options"`
	Overrides []lintOverrideConfig `toml:"overrides"`
	Suppressions suppressionConfig `toml:"suppressions"`
	Baseline baselineConfig `toml:"baseline"`
}

type lintOverrideConfig struct {
	Paths []string `toml:"paths"`
	Rules map[string]string `toml:"rules"`
}

type baselineConfig struct {
	Path *string `toml:"path"`
	ReportStale *bool `toml:"report-stale"`
	ExpiryCutoff *string `toml:"expiry-cutoff"`
}

type suppressionConfig struct {
	RequireReason *bool `toml:"require-reason"`
	ExpiryCutoff *string `toml:"expiry-cutoff"`
}

type cacheConfig struct {
	Enabled *bool `toml:"enabled"`
	MaxEntries *int `toml:"max-entries"`
	MaxBytes *int64 `toml:"max-bytes"`
}

// Defaults returns an independent configuration containing built-in policy.
func Defaults() Config {
	result := Config{
		Version: Version,
		Format: Format{LineWidth: DefaultLineWidth, TabWidth: DefaultTabWidth},
		Analysis: Analysis{
			GOOS: runtime.GOOS,
			GOARCH: runtime.GOARCH,
			CGOEnabled: build.Default.CgoEnabled,
		},
		Lint: Lint{
			Rules: make(map[string]Severity),
			RuleOptions: make(map[string]rules.OptionSet),
			Baseline: Baseline{ReportStale: true},
		},
		Cache: Cache{MaxEntries: DefaultCacheMaxEntries, MaxBytes: DefaultCacheMaxBytes},
	}
	applyProfile(&result.Lint, ProfileDefault)
	return result
}

// Load returns defaults or reads and parses one selected configuration.
func Load(selection Selection, options ParseOptions) (Config, error) {
	if selection.Path == "" {
		return Defaults(), nil
	}
	input, err := os.ReadFile(selection.Path)
	if err != nil {
		return Config{}, &Error{
			Path: selection.Path,
			Message: fmt.Sprintf("read configuration: %v", err),
			cause: err,
		}
	}
	loaded, err := Parse(selection.Path, input, options)
	if err != nil {
		return Config{}, err
	}
	if len(loaded.Analysis.ContractFiles) == 0 {
		return loaded, nil
	}
	root := selection.Root
	if root == "" {
		root = filepath.Dir(selection.Path)
	}
	files := make([]contracts.File, len(loaded.Analysis.ContractFiles))
	remainingBytes := int64(contracts.MaxTotalBytes)
	for index, relative := range loaded.Analysis.ContractFiles {
		absolute := filepath.Join(root, filepath.FromSlash(relative))
		limit := min(int64(contracts.MaxFileBytes), remainingBytes)
		snapshot, readErr := filesystem.ReadWithinLimit(root, absolute, limit)
		if readErr != nil {
			return Config{}, &Error{
				Path: absolute,
				Message: fmt.Sprintf("read contract file: %v", readErr),
				cause: readErr,
			}
		}
		files[index] = contracts.File{Path: absolute, Bytes: snapshot.Bytes()}
		remainingBytes -= int64(len(files[index].Bytes))
	}
	loaded.Analysis.Contracts, err = contracts.ParseFiles(files)
	if err != nil {
		return Config{}, err
	}
	return loaded, nil
}

// Parse strictly decodes, defaults, and validates one configuration source.
func Parse(path string, input []byte, options ParseOptions) (Config, error) {
	var decoded fileConfig
	if err := toml.NewDecoder(bytes.NewReader(input)).DisallowUnknownFields().Decode(&decoded);
		err != nil {
		return Config{}, locatedDecodeError(path, err)
	}
	if decoded.Version == nil {
		return Config{}, semanticError(path, "version is required")
	}
	if *decoded.Version != Version {
		return Config{}, semanticError(
			path,
			"unsupported configuration version %d",
			*decoded.Version,
		)
	}
	result := Defaults()
	if decoded.Format.LineWidth != nil {
		if *decoded.Format.LineWidth <= 0 {
			return Config{}, semanticError(path, "format.line-width must be positive")
		}
		result.Format.LineWidth = *decoded.Format.LineWidth
	}
	if decoded.Format.TabWidth != nil {
		if *decoded.Format.TabWidth <= 0 {
			return Config{}, semanticError(path, "format.tab-width must be positive")
		}
		result.Format.TabWidth = *decoded.Format.TabWidth
	}
	if decoded.Analysis.BuildTags != nil {
		result.Analysis.BuildTags = append([]string(nil), decoded.Analysis.BuildTags...)
		for _, tag := range result.Analysis.BuildTags {
			if !validBuildTag(tag) {
				return Config{}, semanticError(
					path,
					"analysis.build-tags contains invalid tag %q",
					tag,
				)
			}
		}
		sort.Strings(result.Analysis.BuildTags)
		result.Analysis.BuildTags = slices.Compact(result.Analysis.BuildTags)
	}
	if decoded.Analysis.GOOS != nil {
		if !validTarget(*decoded.Analysis.GOOS) {
			return Config{}, semanticError(
				path,
				"analysis.goos must contain only lowercase ASCII letters and digits",
			)
		}
		result.Analysis.GOOS = *decoded.Analysis.GOOS
	}
	if decoded.Analysis.GOARCH != nil {
		if !validTarget(*decoded.Analysis.GOARCH) {
			return Config{}, semanticError(
				path,
				"analysis.goarch must contain only lowercase ASCII letters and digits",
			)
		}
		result.Analysis.GOARCH = *decoded.Analysis.GOARCH
	}
	if decoded.Analysis.CGOEnabled != nil {
		result.Analysis.CGOEnabled = *decoded.Analysis.CGOEnabled
	}
	if decoded.Analysis.Targets != nil {
		if len(decoded.Analysis.Targets) > MaxAnalysisTargets {
			return Config{}, semanticError(
				path,
				"analysis.targets must not contain more than %d targets",
				MaxAnalysisTargets,
			)
		}
		result.Analysis.Targets = make([]AnalysisTarget, 0, len(decoded.Analysis.Targets))
		seen := make(map[string]struct{}, len(decoded.Analysis.Targets))
		for index, decodedTarget := range decoded.Analysis.Targets {
			if decodedTarget.GOOS == nil || !validTarget(*decodedTarget.GOOS) {
				return Config{}, semanticError(
					path,
					"analysis.targets[%d].goos must contain only lowercase ASCII letters and digits",
					index,
				)
			}
			if decodedTarget.GOARCH == nil || !validTarget(*decodedTarget.GOARCH) {
				return Config{}, semanticError(
					path,
					"analysis.targets[%d].goarch must contain only lowercase ASCII letters and digits",
					index,
				)
			}
			tags := slices.Clone(decodedTarget.Tags)
			for _, tag := range tags {
				if !validBuildTag(tag) {
					return Config{}, semanticError(
						path,
						"analysis.targets[%d].tags contains invalid tag %q",
						index,
						tag,
					)
				}
			}
			sort.Strings(tags)
			tags = slices.Compact(tags)
			target := AnalysisTarget{
				BuildTags: tags,
				GOOS: *decodedTarget.GOOS,
				GOARCH: *decodedTarget.GOARCH,
			}
			if decodedTarget.CGOEnabled != nil {
				target.CGOEnabled = *decodedTarget.CGOEnabled
			}
			identity := target.ID()
			if _, duplicate := seen[identity]; duplicate {
				return Config{}, semanticError(
					path,
					"analysis.targets repeats target %q",
					identity,
				)
			}
			seen[identity] = struct{}{}
			result.Analysis.Targets = append(result.Analysis.Targets, target)
		}
		sort.Slice(
			result.Analysis.Targets,
			func(left, right int) bool {
				return result.Analysis.Targets[left].ID() <
					result.Analysis.Targets[right].ID()
			},
		)
	}
	if decoded.Analysis.ContractFiles != nil {
		if len(decoded.Analysis.ContractFiles) > contracts.MaxFiles {
			return Config{}, semanticError(
				path,
				"analysis.contract-files must not contain more than %d paths",
				contracts.MaxFiles,
			)
		}
		result.Analysis.ContractFiles = slices.Clone(decoded.Analysis.ContractFiles)
		for _, contractPath := range result.Analysis.ContractFiles {
			if !baseline.ValidPath(contractPath) {
				return Config{}, semanticError(
					path,
					"analysis.contract-files must contain portable project-relative paths",
				)
			}
		}
		sort.Strings(result.Analysis.ContractFiles)
		for index := 1; index < len(result.Analysis.ContractFiles); index++ {
			if result.Analysis.ContractFiles[index - 1] ==
				result.Analysis.ContractFiles[index] {
				return Config{}, semanticError(
					path,
					"analysis.contract-files contains duplicate contract file %q",
					result.Analysis.ContractFiles[index],
				)
			}
		}
	}
	if decoded.Lint.Profile != nil &&
		(decoded.Lint.Preset != nil || decoded.Lint.Presets != nil) {
		return Config{}, semanticError(
			path,
			"lint.profile cannot be configured with lint.preset or lint.presets",
		)
	}
	if decoded.Lint.Preset != nil && decoded.Lint.Presets != nil {
		return Config{}, semanticError(
			path,
			"lint.preset and lint.presets cannot both be configured",
		)
	}
	if decoded.Lint.Profile != nil {
		profile := Profile(*decoded.Lint.Profile)
		if !ValidProfile(profile) {
			return Config{}, semanticError(path, "unknown lint profile %q", profile)
		}
		applyProfile(&result.Lint, profile)
	}
	if decoded.Lint.Preset != nil {
		clearProfile(&result.Lint)
		preset := Preset(*decoded.Lint.Preset)
		if !validSelectablePreset(preset) {
			return Config{}, semanticError(path, "unknown lint preset %q", preset)
		}
		result.Lint.Presets = []Preset{preset}
	}
	if decoded.Lint.Presets != nil {
		clearProfile(&result.Lint)
		result.Lint.Presets = make([]Preset, 0, len(decoded.Lint.Presets))
		seen := make(map[Preset]struct{}, len(decoded.Lint.Presets))
		for _, configured := range decoded.Lint.Presets {
			preset := Preset(configured)
			switch preset {
			case PresetRestriction:
				return Config{}, semanticError(
					path,
					"lint preset %q must be enabled rule by rule",
					preset,
				)
			case PresetMigration:
				return Config{}, semanticError(
					path,
					"lint preset %q requires an explicit migration target",
					preset,
				)
			}
			if !validSelectablePreset(preset) {
				return Config{}, semanticError(
					path,
					"unknown lint preset %q",
					preset,
				)
			}
			if _, duplicate := seen[preset]; duplicate {
				return Config{}, semanticError(
					path,
					"duplicate lint preset %q",
					preset,
				)
			}
			seen[preset] = struct{}{}
			result.Lint.Presets = append(result.Lint.Presets, preset)
		}
		sort.Slice(
			result.Lint.Presets,
			func(first, second int) bool {
				return presetOrder(result.Lint.Presets[first]) <
					presetOrder(result.Lint.Presets[second])
			},
		)
	}
	if decoded.Lint.WarningsAsErrors != nil {
		result.Lint.WarningsAsErrors = *decoded.Lint.WarningsAsErrors
	}
	if decoded.Lint.Baseline.Path != nil {
		if !baseline.ValidPath(*decoded.Lint.Baseline.Path) {
			return Config{}, semanticError(
				path,
				"lint.baseline.path must be a portable relative path",
			)
		}
		result.Lint.Baseline.Path = *decoded.Lint.Baseline.Path
	}
	if decoded.Lint.Baseline.ReportStale != nil {
		if decoded.Lint.Baseline.Path == nil {
			return Config{}, semanticError(
				path,
				"lint.baseline.report-stale requires lint.baseline.path",
			)
		}
		result.Lint.Baseline.ReportStale = *decoded.Lint.Baseline.ReportStale
	}
	if decoded.Lint.Baseline.ExpiryCutoff != nil {
		if decoded.Lint.Baseline.Path == nil {
			return Config{}, semanticError(
				path,
				"lint.baseline.expiry-cutoff requires lint.baseline.path",
			)
		}
		cutoff := *decoded.Lint.Baseline.ExpiryCutoff
		parsed, parseErr := time.Parse(time.DateOnly, cutoff)
		if parseErr != nil || parsed.Format(time.DateOnly) != cutoff {
			return Config{}, semanticError(
				path,
				"lint.baseline.expiry-cutoff must use a valid YYYY-MM-DD date",
			)
		}
		result.Lint.Baseline.ExpiryCutoff = cutoff
	}
	if decoded.Lint.Suppressions.RequireReason != nil {
		result.Lint.Suppressions.RequireReason = *decoded.Lint.Suppressions.RequireReason
	}
	if decoded.Lint.Suppressions.ExpiryCutoff != nil {
		cutoff := *decoded.Lint.Suppressions.ExpiryCutoff
		parsed, parseErr := time.Parse(time.DateOnly, cutoff)
		if parseErr != nil || parsed.Format(time.DateOnly) != cutoff {
			return Config{}, semanticError(
				path,
				"lint.suppressions.expiry-cutoff must be a valid YYYY-MM-DD date",
			)
		}
		result.Lint.Suppressions.ExpiryCutoff = cutoff
	}
	if decoded.Cache.Enabled != nil {
		result.Cache.Enabled = *decoded.Cache.Enabled
	}
	if decoded.Cache.MaxEntries != nil {
		if *decoded.Cache.MaxEntries < 0 {
			return Config{}, semanticError(
				path,
				"cache.max-entries must not be negative",
			)
		}
		result.Cache.MaxEntries = *decoded.Cache.MaxEntries
	}
	if decoded.Cache.MaxBytes != nil {
		if *decoded.Cache.MaxBytes < 0 {
			return Config{}, semanticError(path, "cache.max-bytes must not be negative")
		}
		result.Cache.MaxBytes = *decoded.Cache.MaxBytes
	}
	if result.Cache.Enabled && result.Cache.MaxEntries == 0 && result.Cache.MaxBytes == 0 {
		return Config{}, semanticError(
			path,
			"enabled cache requires a positive max-entries or max-bytes limit",
		)
	}
	knownRules := make(map[string]struct{}, len(options.KnownRules))
	for _, rule := range options.KnownRules {
		knownRules[rule] = struct{}{}
	}
	ruleIDs := make([]string, 0, len(decoded.Lint.Rules))
	for rule := range decoded.Lint.Rules {
		ruleIDs = append(ruleIDs, rule)
	}
	sort.Strings(ruleIDs)
	for _, rule := range ruleIDs {
		if _, found := knownRules[rule]; !found {
			return Config{}, semanticError(path, "unknown lint rule %q", rule)
		}
		severity := Severity(decoded.Lint.Rules[rule])
		if !validSeverity(severity) {
			return Config{}, semanticError(
				path,
				"invalid severity %q for lint rule %q",
				severity,
				rule,
			)
		}
		result.Lint.Rules[rule] = severity
	}
	for index, override := range decoded.Lint.Overrides {
		number := index + 1
		if len(override.Paths) == 0 {
			return Config{}, semanticError(
				path,
				"lint override %d requires at least one path pattern",
				number,
			)
		}
		if len(override.Rules) == 0 {
			return Config{}, semanticError(
				path,
				"lint override %d requires at least one rule",
				number,
			)
		}
		patterns := append([]string(nil), override.Paths...)
		for _, pattern := range patterns {
			if err := validateLintPathPattern(pattern); err != nil {
				return Config{}, semanticError(
					path,
					"lint override %d path pattern %q %s",
					number,
					pattern,
					err,
				)
			}
		}
		sort.Strings(patterns)
		for patternIndex := 1; patternIndex < len(patterns); patternIndex++ {
			if patterns[patternIndex] == patterns[patternIndex - 1] {
				return Config{}, semanticError(
					path,
					"lint override %d contains duplicate path pattern %q",
					number,
					patterns[patternIndex],
				)
			}
		}
		overrideRuleIDs := make([]string, 0, len(override.Rules))
		for rule := range override.Rules {
			overrideRuleIDs = append(overrideRuleIDs, rule)
		}
		sort.Strings(overrideRuleIDs)
		resolvedRules := make(map[string]Severity, len(overrideRuleIDs))
		for _, rule := range overrideRuleIDs {
			if _, found := knownRules[rule]; !found {
				return Config{}, semanticError(
					path,
					"unknown lint rule %q in lint override %d",
					rule,
					number,
				)
			}
			severity := Severity(override.Rules[rule])
			if !validSeverity(severity) {
				return Config{}, semanticError(
					path,
					"invalid severity %q for lint rule %q in lint override %d",
					severity,
					rule,
					number,
				)
			}
			resolvedRules[rule] = severity
		}
		result.Lint.Overrides = append(
			result.Lint.Overrides,
			LintOverride{Paths: patterns, Rules: resolvedRules},
		)
	}
	optionRuleIDs := make([]string, 0, len(decoded.Lint.RuleOptions))
	for rule := range decoded.Lint.RuleOptions {
		optionRuleIDs = append(optionRuleIDs, rule)
	}
	sort.Strings(optionRuleIDs)
	for _, rule := range optionRuleIDs {
		if _, found := knownRules[rule]; !found {
			return Config{}, semanticError(
				path,
				"unknown lint rule %q in lint.rule-options",
				rule,
			)
		}
		schema := make(map[string]rules.OptionMetadata, len(options.RuleOptions[rule]))
		for _, option := range options.RuleOptions[rule] {
			schema[option.Name] = option
		}
		names := make([]string, 0, len(decoded.Lint.RuleOptions[rule]))
		for name := range decoded.Lint.RuleOptions[rule] {
			names = append(names, name)
		}
		sort.Strings(names)
		values := make(map[string]rules.OptionValue, len(names))
		for _, name := range names {
			metadata, found := schema[name]
			if !found {
				return Config{}, semanticError(
					path,
					"unknown option %q for lint rule %q",
					name,
					rule,
				)
			}
			value, err := decodeRuleOption(
				decoded.Lint.RuleOptions[rule][name],
				metadata.Kind,
			)
			if err != nil {
				return Config{}, semanticError(
					path,
					"option %q for lint rule %q must be %s",
					name,
					rule,
					metadata.Kind,
				)
			}
			if err := rules.ValidateOptionValue(metadata, value); err != nil {
				return Config{}, semanticError(
					path,
					"option %q for lint rule %q %s",
					name,
					rule,
					err,
				)
			}
			values[name] = value
		}
		result.Lint.RuleOptions[rule] = rules.NewOptionSet(values)
	}
	return result, nil
}

// LintForPath returns an independent lint policy after ordered path overrides.
// Match indexes are one-based so they correspond to configuration declarations.
func (c Config) LintForPath(projectRelativePath string) (Lint, []int, error) {
	if err := validateProjectRelativePath(projectRelativePath); err != nil {
		return Lint{}, nil, fmt.Errorf("resolve lint path %q: %w", projectRelativePath, err)
	}
	resolved := cloneLint(c.Lint)
	matches := make([]int, 0, len(c.Lint.Overrides))
	for index, override := range c.Lint.Overrides {
		matched := false
		for _, pattern := range override.Paths {
			if matchLintPathPattern(pattern, projectRelativePath) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		matches = append(matches, index + 1)
		for ruleID, severity := range override.Rules {
			resolved.Rules[ruleID] = severity
		}
	}
	return resolved, matches, nil
}

// LintForExecution enables every rule that can be selected by a path override.
// Per-path filtering restores the exact severity after shared analysis runs.
func (c Config) LintForExecution() Lint {
	resolved := cloneLint(c.Lint)
	for _, override := range c.Lint.Overrides {
		for ruleID, severity := range override.Rules {
			if severity == SeverityOff {
				continue
			}
			current := resolved.Rules[ruleID]
			if current == SeverityError || current == severity {
				continue
			}
			resolved.Rules[ruleID] = severity
		}
	}
	return resolved
}

func cloneLint(value Lint) Lint {
	result := value
	result.ProfileRules = cloneSeverityMap(value.ProfileRules)
	result.Presets = slices.Clone(value.Presets)
	result.Rules = cloneSeverityMap(value.Rules)
	result.RuleOptions = make(map[string]rules.OptionSet, len(value.RuleOptions))
	for id, options := range value.RuleOptions {
		result.RuleOptions[id] = options
	}
	result.Overrides = make([]LintOverride, len(value.Overrides))
	for index, override := range value.Overrides {
		result.Overrides[index] = LintOverride{
			Paths: slices.Clone(override.Paths),
			Rules: make(map[string]Severity, len(override.Rules)),
		}
		for id, severity := range override.Rules {
			result.Overrides[index].Rules[id] = severity
		}
	}
	return result
}

// EffectiveRules returns profile policy followed by explicit rule overrides.
func (l Lint) EffectiveRules() map[string]Severity {
	result := cloneSeverityMap(l.ProfileRules)
	for id, severity := range l.Rules {
		result[id] = severity
	}
	return result
}

func cloneSeverityMap(value map[string]Severity) map[string]Severity {
	result := make(map[string]Severity, len(value))
	for id, severity := range value {
		result[id] = severity
	}
	return result
}

func clearProfile(lint *Lint) {
	lint.Profile = ""
	lint.ProfileRules = make(map[string]Severity)
}

func applyProfile(lint *Lint, profile Profile) {
	lint.Profile = profile
	lint.ProfileRules = make(map[string]Severity)
	switch profile {
	case ProfileDefault:
		lint.Presets = []Preset{PresetCorrectness}
	case ProfileRecommended:
		lint.Presets = []Preset{PresetCorrectness}
		for _, id := range recommendedProfileRules {
			lint.ProfileRules[id] = SeverityWarn
		}
	case ProfileStrict:
		lint.Presets = []Preset{
			PresetCorrectness,
			PresetSuspicious,
			PresetPerformance,
			PresetComplexity,
			PresetStyle,
		}
	case ProfilePedantic:
		lint.Presets = []Preset{
			PresetCorrectness,
			PresetSuspicious,
			PresetPerformance,
			PresetComplexity,
			PresetStyle,
			PresetPedantic,
		}
	}
}

func validateLintPathPattern(pattern string) error {
	if pattern == "" || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, "\\") {
		return fmt.Errorf("must be project-relative")
	}
	segments := strings.Split(pattern, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("must be project-relative")
		}
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, ""); err != nil {
			return fmt.Errorf("is invalid: %v", err)
		}
	}
	return nil
}

func validateProjectRelativePath(value string) error {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return fmt.Errorf("path must be project-relative and use forward slashes")
	}
	segments := strings.Split(value, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("path must be normalized and remain inside the project")
		}
	}
	return nil
}

func matchLintPathPattern(pattern, projectRelativePath string) bool {
	patternSegments := strings.Split(pattern, "/")
	pathSegments := strings.Split(projectRelativePath, "/")
	type state struct {
		pattern, path int
	}
	memo := make(map[state]bool)
	known := make(map[state]bool)
	var match func(int, int) bool
	match = func(patternIndex, pathIndex int) bool {
		current := state{pattern: patternIndex, path: pathIndex}
		if known[current] {
			return memo[current]
		}
		known[current] = true
		if patternIndex == len(patternSegments) {
			memo[current] = pathIndex == len(pathSegments)
			return memo[current]
		}
		segment := patternSegments[patternIndex]
		if segment == "**" {
			memo[current] = match(patternIndex + 1, pathIndex) ||
				(pathIndex < len(pathSegments) &&
					match(patternIndex, pathIndex + 1))
			return memo[current]
		}
		if pathIndex >= len(pathSegments) {
			return false
		}
		matched, _ := path.Match(segment, pathSegments[pathIndex])
		memo[current] = matched && match(patternIndex + 1, pathIndex + 1)
		return memo[current]
	}
	return match(0, 0)
}

func validBuildTag(tag string) bool {
	if tag == "" {
		return false
	}
	for _, character := range tag {
		if !unicode.IsLetter(character) &&
			!unicode.IsDigit(character) &&
			character != '_' &&
			character != '.' {
			return false
		}
	}
	return true
}

func validTarget(target string) bool {
	if target == "" {
		return false
	}
	for _, character := range target {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func decodeRuleOption(value any, kind rules.OptionKind) (rules.OptionValue, error) {
	switch kind {
	case rules.OptionBoolean:
		value, valid := value.(bool)
		if valid {
			return rules.BooleanOption(value), nil
		}
	case rules.OptionInteger:
		value, valid := value.(int64)
		if valid {
			return rules.IntegerOption(value), nil
		}
	case rules.OptionString:
		value, valid := value.(string)
		if valid {
			return rules.StringOption(value), nil
		}
	case rules.OptionStrings:
		items, valid := value.([]any)
		if !valid {
			break
		}
		values := make([]string, len(items))
		for index, item := range items {
			text, valid := item.(string)
			if !valid {
				return rules.OptionValue{}, fmt.Errorf("invalid string list")
			}
			values[index] = text
		}
		return rules.StringsOption(values), nil
	}
	return rules.OptionValue{}, fmt.Errorf("invalid %s option", kind)
}

func semanticError(path, format string, arguments ...any) error {
	return &Error{Path: path, Message: fmt.Sprintf(format, arguments...)}
}

func locatedDecodeError(path string, cause error) error {
	result := &Error{Path: path, Message: cause.Error(), cause: cause}
	var decoded *toml.DecodeError
	if errors.As(cause, &decoded) {
		result.Line, result.Column = decoded.Position()
		result.Message = strings.TrimPrefix(decoded.Error(), "toml: ")
	}
	return result
}

func validSelectablePreset(value Preset) bool {
	switch value {
	case PresetCorrectness,
		PresetSuspicious,
		PresetPerformance,
		PresetComplexity,
		PresetStyle,
		PresetPedantic,
		PresetNursery:
		return true
	default:
		return false
	}
}

func presetOrder(value Preset) int {
	switch value {
	case PresetCorrectness:
		return 0
	case PresetSuspicious:
		return 1
	case PresetPerformance:
		return 2
	case PresetComplexity:
		return 3
	case PresetStyle:
		return 4
	case PresetPedantic:
		return 5
	case PresetNursery:
		return 6
	default:
		return 7
	}
}

func validSeverity(value Severity) bool {
	switch value {
	case SeverityOff, SeverityWarn, SeverityError:
		return true
	default:
		return false
	}
}
