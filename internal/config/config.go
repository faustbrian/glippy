// Package config owns typed Gox configuration defaults and decoding.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
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
)

// Preset identifies one coherent lint-rule selection.
type Preset string

const (
	PresetCorrectness Preset = "correctness"
	PresetSuspicious  Preset = "suspicious"
	PresetPerformance Preset = "performance"
	PresetComplexity  Preset = "complexity"
	PresetStyle       Preset = "style"
)

// Severity controls whether and how a lint rule reports.
type Severity string

const (
	SeverityOff   Severity = "off"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Config is one fully defaulted and validated project configuration.
type Config struct {
	Version int
	Format  Format
	Lint    Lint
}

// Format contains formatter policy that materially affects adoption.
type Format struct {
	LineWidth int
	TabWidth  int
}

// Lint contains the selected preset and explicit rule overrides.
type Lint struct {
	Preset Preset
	Rules  map[string]Severity
}

// ParseOptions supplies registry state needed to validate rule identifiers.
type ParseOptions struct {
	KnownRules []string
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
	Version *int         `toml:"version"`
	Format  formatConfig `toml:"format"`
	Lint    lintConfig   `toml:"lint"`
}

type formatConfig struct {
	LineWidth *int `toml:"line-width"`
	TabWidth  *int `toml:"tab-width"`
}

type lintConfig struct {
	Preset *string           `toml:"preset"`
	Rules  map[string]string `toml:"rules"`
}

// Defaults returns an independent configuration containing built-in policy.
func Defaults() Config {
	return Config{
		Version: Version,
		Format: Format{
			LineWidth: DefaultLineWidth,
			TabWidth:  DefaultTabWidth,
		},
		Lint: Lint{
			Preset: PresetCorrectness,
			Rules:  make(map[string]Severity),
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
	if decoded.Lint.Preset != nil {
		preset := Preset(*decoded.Lint.Preset)
		if !validPreset(preset) {
			return Config{}, semanticError(path, "unknown lint preset %q", preset)
		}
		result.Lint.Preset = preset
	}
	knownRules := make(map[string]struct{}, len(options.KnownRules))
	for _, rule := range options.KnownRules {
		knownRules[rule] = struct{}{}
	}
	rules := make([]string, 0, len(decoded.Lint.Rules))
	for rule := range decoded.Lint.Rules {
		rules = append(rules, rule)
	}
	sort.Strings(rules)
	for _, rule := range rules {
		if _, found := knownRules[rule]; !found {
			return Config{}, semanticError(path, "unknown lint rule %q", rule)
		}
		severity := Severity(decoded.Lint.Rules[rule])
		if !validSeverity(severity) {
			return Config{}, semanticError(path, "invalid severity %q for lint rule %q", severity, rule)
		}
		result.Lint.Rules[rule] = severity
	}
	return result, nil
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
