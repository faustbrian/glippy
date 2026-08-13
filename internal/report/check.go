package report

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/source"
)

type CheckSummary struct {
	Files int `json:"files"`
	FormattingDifferences int `json:"formatting_differences"`
	Diagnostics int `json:"diagnostics"`
	Suppressed int `json:"suppressed"`
	Baselined int `json:"baselined"`
	BaselineProblems int `json:"baseline_problems"`
	SuppressionProblems int `json:"suppression_problems"`
	UnusedSuppressions int `json:"unused_suppressions"`
	PackageDiagnostics int `json:"package_diagnostics,omitempty"`
	SourceProblems int `json:"source_problems,omitempty"`
	Complete bool `json:"complete"`
}

type CheckFormatStatus string

const (
	CheckFormatUnchanged CheckFormatStatus = "unchanged"
	CheckFormatDifferent CheckFormatStatus = "different"
)

type CheckFile struct {
	Path string `json:"path"`
	SourceDigest string `json:"source_digest"`
	FormatStatus CheckFormatStatus `json:"format_status"`
}

// CheckFormatOutcome binds a format comparison to one analyzed source version.
type CheckFormatOutcome struct {
	Path string
	Digest source.Digest
	Different bool
}

type CheckResult struct {
	SchemaVersion int `json:"schema_version"`
	Command string `json:"command"`
	Mode string `json:"mode"`
	Outcome Outcome `json:"outcome"`
	Summary CheckSummary `json:"summary"`
	Files []CheckFile `json:"files"`
	Diagnostics []LintDiagnostic `json:"diagnostics"`
	SuppressionProblems []SuppressionProblem `json:"suppression_problems"`
	UnusedSuppressions []UnusedSuppression `json:"unused_suppressions"`
	BaselineProblems []BaselineProblem `json:"baseline_problems"`
	PackageDiagnostics []LintPackageDiagnostic `json:"package_diagnostics,omitempty"`
	SourceProblems []LintSourceProblem `json:"source_problems,omitempty"`
	Errors []Error `json:"errors"`
}

// NewCheckResult validates and combines formatting and lint outcomes produced
// from the same source versions.
func NewCheckResult(
	category string,
	exitCode int,
	complete bool,
	analyses []analysis.Result,
	formats []CheckFormatOutcome,
	errs []Error,
) (CheckResult, error) {
	lintResult, err := NewLintResult("check", category, exitCode, complete, analyses, errs)
	if err != nil {
		return CheckResult{}, err
	}
	return newCheckResult(lintResult, formats)
}

// NewPackageCheckResult combines formatting with package-aware lint outcomes
// produced from the package loader's exact source versions.
func NewPackageCheckResult(
	category string,
	exitCode int,
	complete bool,
	packageResult analysis.PackageResult,
	formats []CheckFormatOutcome,
	errs []Error,
) (CheckResult, error) {
	lintResult, err := NewPackageLintResult(
		"check",
		category,
		exitCode,
		complete,
		packageResult,
		errs,
	)
	if err != nil {
		return CheckResult{}, err
	}
	return newCheckResult(lintResult, formats)
}

func newCheckResult(lintResult LintResult, formats []CheckFormatOutcome) (CheckResult, error) {
	orderedFormats := slices.Clone(formats)
	sort.Slice(
		orderedFormats,
		func(left, right int) bool {
			return orderedFormats[left].Path < orderedFormats[right].Path
		},
	)
	if len(orderedFormats) != len(lintResult.Files) {
		return CheckResult{}, fmt.Errorf(
			"check format outcomes contain %d files, want %d analysis results",
			len(orderedFormats),
			len(lintResult.Files),
		)
	}
	result := CheckResult{
		SchemaVersion: SchemaVersion,
		Command: "check",
		Mode: "check",
		Outcome: lintResult.Outcome,
		Summary: CheckSummary{
			Files: lintResult.Summary.Files,
			Diagnostics: lintResult.Summary.Diagnostics,
			Suppressed: lintResult.Summary.Suppressed,
			Baselined: lintResult.Summary.Baselined,
			BaselineProblems: lintResult.Summary.BaselineProblems,
			SuppressionProblems: lintResult.Summary.SuppressionProblems,
			UnusedSuppressions: lintResult.Summary.UnusedSuppressions,
			PackageDiagnostics: lintResult.Summary.PackageDiagnostics,
			SourceProblems: lintResult.Summary.SourceProblems,
			Complete: lintResult.Summary.Complete,
		},
		Files: make([]CheckFile, len(orderedFormats)),
		Diagnostics: lintResult.Diagnostics,
		SuppressionProblems: lintResult.SuppressionProblems,
		UnusedSuppressions: lintResult.UnusedSuppressions,
		BaselineProblems: lintResult.BaselineProblems,
		PackageDiagnostics: lintResult.PackageDiagnostics,
		SourceProblems: lintResult.SourceProblems,
		Errors: lintResult.Errors,
	}
	for index, format := range orderedFormats {
		lintFile := lintResult.Files[index]
		if format.Path == "" ||
			!filepath.IsAbs(format.Path) ||
			filepath.Clean(format.Path) != format.Path {
			return CheckResult{}, fmt.Errorf(
				"check format outcome path %q is not normalized absolute",
				format.Path,
			)
		}
		if format.Path != lintFile.Path ||
			encodeDigest(format.Digest) != lintFile.SourceDigest {
			return CheckResult{}, fmt.Errorf(
				"check format source identity does not match analysis result %q",
				lintFile.Path,
			)
		}
		status := CheckFormatUnchanged
		if format.Different {
			status = CheckFormatDifferent
			result.Summary.FormattingDifferences++
		}
		result.Files[index] = CheckFile{
			Path: format.Path,
			SourceDigest: lintFile.SourceDigest,
			FormatStatus: status,
		}
	}
	return result, nil
}

func MarshalCheckJSON(result CheckResult) ([]byte, error) {
	return marshalJSON(result)
}
