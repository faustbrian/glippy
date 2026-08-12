// Package config owns typed Gox configuration defaults and decoding.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"go/build"
	"os"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/pelletier/go-toml/v2"

	"github.com/faustbrian/gox/internal/rules"
)

// Filename is the project configuration filename.
const Filename = ".gox.toml"

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
	PresetSuspicious  = rules.PresetSuspicious
	PresetPerformance = rules.PresetPerformance
	PresetComplexity  = rules.PresetComplexity
	PresetStyle       = rules.PresetStyle
)

// Severity controls whether and how a lint rule reports.
type Severity = rules.Severity

const (
	SeverityOff   = rules.SeverityOff
	SeverityWarn  = rules.SeverityWarn
	SeverityError = rules.SeverityError
)

// Config is one fully defaulted and validated project configuration.
type Config struct {
	Version  int
	Format   Format
	Analysis Analysis
	Lint     Lint
	Cache    Cache
}

// Format contains formatter policy that materially affects adoption.
type Format struct {
	LineWidth int
	TabWidth  int
}

// Analysis contains the resolved build selection for package-aware rules.
type Analysis struct {
	BuildTags  []string
	GOOS       string
	GOARCH     string
	CGOEnabled bool
}

// Lint contains the selected preset and explicit rule overrides.
type Lint struct {
	Preset       Preset
	Rules        map[string]Severity
	RuleOptions  map[string]rules.OptionSet
	Suppressions Suppressions
}

// Suppressions contains project policy for auditable lint waivers.
type Suppressions struct {
	RequireReason bool
	ExpiryCutoff  string
}

// Cache contains the opt-in persistent analysis-cache lifecycle policy.
type Cache struct {
	Enabled    bool
	MaxEntries int
	MaxBytes   int64
}

// ParseOptions supplies registry state needed to validate rule identifiers.
type ParseOptions struct {
	KnownRules  []string
	RuleOptions map[string][]rules.OptionMetadata
}

// Error is one path-aware configuration diagnostic.
type Error struct {
	Path    string
	Line    int
	Column  int
	Message string
	cause   error
}

// Error renders a source-located diagnostic when line information is known.
func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Unwrap returns the underlying TOML decoder failure when one exists.
func (e *Error) Unwrap() error { return e.cause }

type fileConfig struct {
	Version  *int           `toml:"version"`
	Format   formatConfig   `toml:"format"`
	Analysis analysisConfig `toml:"analysis"`
	Lint     lintConfig     `toml:"lint"`
	Cache    cacheConfig    `toml:"cache"`
}

type formatConfig struct {
	LineWidth *int `toml:"line-width"`
	TabWidth  *int `toml:"tab-width"`
}

type analysisConfig struct {
	BuildTags  []string `toml:"build-tags"`
	GOOS       *string  `toml:"goos"`
	GOARCH     *string  `toml:"goarch"`
	CGOEnabled *bool    `toml:"cgo-enabled"`
}

type lintConfig struct {
	Preset       *string                   `toml:"preset"`
	Rules        map[string]string         `toml:"rules"`
	RuleOptions  map[string]map[string]any `toml:"rule-options"`
	Suppressions suppressionConfig         `toml:"suppressions"`
}

type suppressionConfig struct {
	RequireReason *bool   `toml:"require-reason"`
	ExpiryCutoff  *string `toml:"expiry-cutoff"`
}

type cacheConfig struct {
	Enabled    *bool  `toml:"enabled"`
	MaxEntries *int   `toml:"max-entries"`
	MaxBytes   *int64 `toml:"max-bytes"`
}

// Defaults returns an independent configuration containing built-in policy.
func Defaults() Config {
	return Config{
		Version: Version,
		Format: Format{
			LineWidth: DefaultLineWidth,
			TabWidth:  DefaultTabWidth,
		},
		Analysis: Analysis{
			GOOS:       runtime.GOOS,
			GOARCH:     runtime.GOARCH,
			CGOEnabled: build.Default.CgoEnabled,
		},
		Lint: Lint{
			Preset:      PresetCorrectness,
			Rules:       make(map[string]Severity),
			RuleOptions: make(map[string]rules.OptionSet),
		},
		Cache: Cache{
			MaxEntries: DefaultCacheMaxEntries,
			MaxBytes:   DefaultCacheMaxBytes,
		},
	}
}

// Load returns defaults or reads and parses one selected configuration.
func Load(selection Selection, options ParseOptions) (Config, error) {
	if selection.Path == "" {
		return Defaults(), nil
	}
	input, err := os.ReadFile(selection.Path)
	if err != nil {
		return Config{}, &Error{
			Path:    selection.Path,
			Message: fmt.Sprintf("read configuration: %v", err),
			cause:   err,
		}
	}
	return Parse(selection.Path, input, options)
}

// Parse strictly decodes, defaults, and validates one configuration source.
func Parse(path string, input []byte, options ParseOptions) (Config, error) {
	var decoded fileConfig
	if err := toml.NewDecoder(bytes.NewReader(input)).DisallowUnknownFields().Decode(&decoded); err != nil {
		return Config{}, locatedDecodeError(path, err)
	}
	if decoded.Version == nil {
		return Config{}, semanticError(path, "version is required")
	}
	if *decoded.Version != Version {
		return Config{}, semanticError(path, "unsupported configuration version %d", *decoded.Version)
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
				return Config{}, semanticError(path, "analysis.build-tags contains invalid tag %q", tag)
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
	if decoded.Lint.Preset != nil {
		preset := Preset(*decoded.Lint.Preset)
		if !validPreset(preset) {
			return Config{}, semanticError(path, "unknown lint preset %q", preset)
		}
		result.Lint.Preset = preset
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
			return Config{}, semanticError(path, "cache.max-entries must not be negative")
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
			return Config{}, semanticError(path, "invalid severity %q for lint rule %q", severity, rule)
		}
		result.Lint.Rules[rule] = severity
	}
	optionRuleIDs := make([]string, 0, len(decoded.Lint.RuleOptions))
	for rule := range decoded.Lint.RuleOptions {
		optionRuleIDs = append(optionRuleIDs, rule)
	}
	sort.Strings(optionRuleIDs)
	for _, rule := range optionRuleIDs {
		if _, found := knownRules[rule]; !found {
			return Config{}, semanticError(path, "unknown lint rule %q in lint.rule-options", rule)
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
				return Config{}, semanticError(path, "unknown option %q for lint rule %q", name, rule)
			}
			value, err := decodeRuleOption(decoded.Lint.RuleOptions[rule][name], metadata.Kind)
			if err != nil {
				return Config{}, semanticError(
					path,
					"option %q for lint rule %q must be %s",
					name,
					rule,
					metadata.Kind,
				)
			}
			values[name] = value
		}
		result.Lint.RuleOptions[rule] = rules.NewOptionSet(values)
	}
	return result, nil
}

func validBuildTag(tag string) bool {
	if tag == "" {
		return false
	}
	for _, character := range tag {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) &&
			character != '_' && character != '.' {
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

func validPreset(value Preset) bool {
	switch value {
	case PresetCorrectness, PresetSuspicious, PresetPerformance,
		PresetComplexity, PresetStyle:
		return true
	default:
		return false
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
