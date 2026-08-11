package report

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"sort"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
	"github.com/faustbrian/gox/internal/suppressions"
)

type LintSummary struct {
	Files               int  `json:"files"`
	Diagnostics         int  `json:"diagnostics"`
	Suppressed          int  `json:"suppressed"`
	SuppressionProblems int  `json:"suppression_problems"`
	UnusedSuppressions  int  `json:"unused_suppressions"`
	Complete            bool `json:"complete"`
}

type LintFileStatus string

const LintFileAnalyzed LintFileStatus = "analyzed"

type LintFile struct {
	Path         string         `json:"path"`
	SourceDigest string         `json:"source_digest"`
	Status       LintFileStatus `json:"status"`
}

type ByteRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type LintRelated struct {
	Range   ByteRange `json:"range"`
	Message string    `json:"message"`
}

type LintFix struct {
	Name   string          `json:"name"`
	Safety rules.FixSafety `json:"safety"`
}

type LintDiagnostic struct {
	RuleID       string         `json:"rule_id"`
	Severity     rules.Severity `json:"severity"`
	MessageKey   string         `json:"message_key"`
	Message      string         `json:"message"`
	Path         string         `json:"path"`
	SourceDigest string         `json:"source_digest"`
	Range        ByteRange      `json:"range"`
	Related      []LintRelated  `json:"related"`
	Notes        []string       `json:"notes"`
	Help         string         `json:"help"`
	Fixes        []LintFix      `json:"fixes"`
}

type SuppressionProblem struct {
	Kind         suppressions.ProblemKind `json:"kind"`
	Path         string                   `json:"path"`
	SourceDigest string                   `json:"source_digest"`
	Range        ByteRange                `json:"range"`
	Message      string                   `json:"message"`
}

type UnusedSuppression struct {
	RuleID       string    `json:"rule_id"`
	Scope        string    `json:"scope"`
	Path         string    `json:"path"`
	SourceDigest string    `json:"source_digest"`
	Range        ByteRange `json:"range"`
	Target       ByteRange `json:"target"`
	Reason       string    `json:"reason"`
}

type LintResult struct {
	SchemaVersion       int                  `json:"schema_version"`
	Command             string               `json:"command"`
	Mode                string               `json:"mode"`
	Outcome             Outcome              `json:"outcome"`
	Summary             LintSummary          `json:"summary"`
	Files               []LintFile           `json:"files"`
	Diagnostics         []LintDiagnostic     `json:"diagnostics"`
	SuppressionProblems []SuppressionProblem `json:"suppression_problems"`
	UnusedSuppressions  []UnusedSuppression  `json:"unused_suppressions"`
	Errors              []Error              `json:"errors"`
}

// NewLintResult validates and maps ordered analysis results into schema version 1.
func NewLintResult(
	mode, category string,
	exitCode int,
	complete bool,
	results []analysis.Result,
	errs []Error,
) (LintResult, error) {
	ordered := slices.Clone(results)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].Path < ordered[right].Path
	})
	result := LintResult{
		SchemaVersion:       SchemaVersion,
		Command:             "lint",
		Mode:                mode,
		Outcome:             Outcome{Category: category, ExitCode: exitCode},
		Summary:             LintSummary{Files: len(ordered), Complete: complete},
		Files:               make([]LintFile, 0, len(ordered)),
		Diagnostics:         make([]LintDiagnostic, 0),
		SuppressionProblems: make([]SuppressionProblem, 0),
		UnusedSuppressions:  make([]UnusedSuppression, 0),
		Errors:              slices.Clone(errs),
	}
	if result.Errors == nil {
		result.Errors = []Error{}
	}
	for index, analyzed := range ordered {
		if analyzed.Path == "" {
			return LintResult{}, fmt.Errorf("lint result %d has no source path", index)
		}
		if !filepath.IsAbs(analyzed.Path) || filepath.Clean(analyzed.Path) != analyzed.Path {
			return LintResult{}, fmt.Errorf("lint result path %q is not normalized absolute", analyzed.Path)
		}
		if analyzed.Digest == (source.Digest{}) {
			return LintResult{}, fmt.Errorf("lint result %q has no source digest", analyzed.Path)
		}
		if index > 0 && ordered[index-1].Path == analyzed.Path {
			return LintResult{}, fmt.Errorf("duplicate source path %q", analyzed.Path)
		}
		digest := encodeDigest(analyzed.Digest)
		result.Files = append(result.Files, LintFile{
			Path:         analyzed.Path,
			SourceDigest: digest,
			Status:       LintFileAnalyzed,
		})
		for _, diagnostic := range analysis.OrderDiagnostics(analyzed.Diagnostics) {
			if diagnostic.Path != analyzed.Path || diagnostic.Digest != analyzed.Digest {
				return LintResult{}, fmt.Errorf(
					"diagnostic source identity does not match analysis result %q",
					analyzed.Path,
				)
			}
			result.Diagnostics = append(result.Diagnostics, lintDiagnostic(diagnostic))
		}
		for _, problem := range analyzed.SuppressionProblems {
			result.SuppressionProblems = append(result.SuppressionProblems, SuppressionProblem{
				Kind:         problem.Kind,
				Path:         analyzed.Path,
				SourceDigest: digest,
				Range:        byteRange(problem.Range),
				Message:      problem.Message,
			})
		}
		for _, directive := range analyzed.UnusedSuppressions {
			scope, err := suppressionScope(directive.Scope)
			if err != nil {
				return LintResult{}, fmt.Errorf("%s: %w", analyzed.Path, err)
			}
			result.UnusedSuppressions = append(result.UnusedSuppressions, UnusedSuppression{
				RuleID:       directive.RuleID,
				Scope:        scope,
				Path:         analyzed.Path,
				SourceDigest: digest,
				Range:        byteRange(directive.Range),
				Target:       byteRange(directive.Target),
				Reason:       directive.Reason,
			})
		}
		result.Summary.Diagnostics += len(analyzed.Diagnostics)
		result.Summary.Suppressed += len(analyzed.Suppressed)
		result.Summary.SuppressionProblems += len(analyzed.SuppressionProblems)
		result.Summary.UnusedSuppressions += len(analyzed.UnusedSuppressions)
	}
	return result, nil
}

func lintDiagnostic(diagnostic rules.Diagnostic) LintDiagnostic {
	related := make([]LintRelated, len(diagnostic.Related))
	for index, item := range diagnostic.Related {
		related[index] = LintRelated{Range: byteRange(item.Range), Message: item.Message}
	}
	fixes := make([]LintFix, len(diagnostic.Fixes))
	for fixIndex, fix := range diagnostic.Fixes {
		fixes[fixIndex] = LintFix{Name: fix.Name, Safety: fix.Safety}
	}
	return LintDiagnostic{
		RuleID:       diagnostic.RuleID,
		Severity:     diagnostic.Severity,
		MessageKey:   diagnostic.MessageKey,
		Message:      diagnostic.Message,
		Path:         diagnostic.Path,
		SourceDigest: encodeDigest(diagnostic.Digest),
		Range:        byteRange(diagnostic.Range),
		Related:      related,
		Notes:        cloneStrings(diagnostic.Notes),
		Help:         diagnostic.Help,
		Fixes:        fixes,
	}
}

func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return slices.Clone(values)
}

func byteRange(sourceRange source.Range) ByteRange {
	return ByteRange{Start: sourceRange.Start, End: sourceRange.End}
}

func encodeDigest(digest source.Digest) string {
	return hex.EncodeToString(digest[:])
}

func suppressionScope(scope suppressions.Scope) (string, error) {
	switch scope {
	case suppressions.ScopeLine:
		return "line", nil
	case suppressions.ScopeNextLine:
		return "next-line", nil
	case suppressions.ScopeRange:
		return "range", nil
	case suppressions.ScopeFile:
		return "file", nil
	default:
		return "", fmt.Errorf("invalid suppression scope %d", scope)
	}
}

// MarshalLintJSON buffers one complete lint result before emitting JSON.
func MarshalLintJSON(result LintResult) ([]byte, error) {
	return marshalJSON(result)
}
