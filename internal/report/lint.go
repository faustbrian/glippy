package report

import (
	"cmp"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"sort"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/baseline"
	fixengine "github.com/faustbrian/glippy/internal/fix"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"github.com/faustbrian/glippy/internal/suppressions"
)

type LintSummary struct {
	Files int `json:"files"`
	Diagnostics int `json:"diagnostics"`
	PreexistingDiagnostics int `json:"preexisting_diagnostics,omitempty"`
	Suppressed int `json:"suppressed"`
	Baselined int `json:"baselined"`
	BaselineProblems int `json:"baseline_problems"`
	SuppressionProblems int `json:"suppression_problems"`
	UnusedSuppressions int `json:"unused_suppressions"`
	PackageDiagnostics int `json:"package_diagnostics,omitempty"`
	SourceProblems int `json:"source_problems,omitempty"`
	FixedFiles int `json:"fixed_files,omitempty"`
	AppliedFixes int `json:"applied_fixes,omitempty"`
	RejectedFixes int `json:"rejected_fixes,omitempty"`
	Complete bool `json:"complete"`
}

type LintFileStatus string

const (
	LintFileAnalyzed LintFileStatus = "analyzed"
	LintFilePending LintFileStatus = "pending"
	LintFileUnchanged LintFileStatus = "unchanged"
	LintFileFixed LintFileStatus = "fixed"
	LintFileConflict LintFileStatus = "conflict"
	LintFileFailed LintFileStatus = "failed"
	LintFilePossiblyFixed LintFileStatus = "possibly_fixed"
)

type LintFile struct {
	Path string `json:"path"`
	SourceDigest string `json:"source_digest"`
	ResultDigest string `json:"result_digest,omitempty"`
	Status LintFileStatus `json:"status"`
}

// LintFixOutcome binds one original source version to its fix transaction.
type LintFixOutcome struct {
	Path string
	SourceDigest source.Digest
	Status LintFileStatus
	Applied []fixengine.Applied
	Rejected []fixengine.Rejection
}

type LintAppliedFix struct {
	RuleID string `json:"rule_id"`
	FixName string `json:"fix_name"`
	Path string `json:"path"`
	SourceDigest string `json:"source_digest"`
	Range ByteRange `json:"range"`
}

type LintRejectedFix struct {
	RuleID string `json:"rule_id"`
	FixName string `json:"fix_name"`
	Path string `json:"path"`
	SourceDigest string `json:"source_digest"`
	Range ByteRange `json:"range"`
	Reason fixengine.RejectionReason `json:"reason"`
	Message string `json:"message"`
}

type ByteRange struct {
	Start int `json:"start"`
	End int `json:"end"`
}

type LintRelated struct {
	Range ByteRange `json:"range"`
	Message string `json:"message"`
}

type LintFix struct {
	Name string `json:"name"`
	Safety rules.FixSafety `json:"safety"`
}

type LintDiagnostic struct {
	RuleID string `json:"rule_id"`
	Severity rules.Severity `json:"severity"`
	MessageKey string `json:"message_key"`
	Message string `json:"message"`
	Path string `json:"path"`
	SourceDigest string `json:"source_digest"`
	Range ByteRange `json:"range"`
	Related []LintRelated `json:"related"`
	Notes []string `json:"notes"`
	Help string `json:"help"`
	Fixes []LintFix `json:"fixes"`
}

type SuppressionProblem struct {
	Kind suppressions.ProblemKind `json:"kind"`
	Path string `json:"path"`
	SourceDigest string `json:"source_digest"`
	Range ByteRange `json:"range"`
	Message string `json:"message"`
}

type UnusedSuppression struct {
	RuleID string `json:"rule_id"`
	Scope string `json:"scope"`
	Path string `json:"path"`
	SourceDigest string `json:"source_digest"`
	Range ByteRange `json:"range"`
	Target ByteRange `json:"target"`
	Reason string `json:"reason"`
	ExpiresOn string `json:"expires_on,omitempty"`
}

type BaselineProblem struct {
	Kind baseline.ProblemKind `json:"kind"`
	Path string `json:"path"`
	RuleID string `json:"rule_id"`
	MessageKey string `json:"message_key"`
	SourceFingerprint string `json:"source_fingerprint"`
	Count int `json:"count"`
	Remaining int `json:"remaining"`
	Reason string `json:"reason,omitempty"`
	ExpiresOn string `json:"expires_on,omitempty"`
}

type LintResult struct {
	SchemaVersion int `json:"schema_version"`
	Command string `json:"command"`
	Mode string `json:"mode"`
	Outcome Outcome `json:"outcome"`
	Summary LintSummary `json:"summary"`
	Files []LintFile `json:"files"`
	Diagnostics []LintDiagnostic `json:"diagnostics"`
	SuppressionProblems []SuppressionProblem `json:"suppression_problems"`
	UnusedSuppressions []UnusedSuppression `json:"unused_suppressions"`
	BaselineProblems []BaselineProblem `json:"baseline_problems"`
	PackageDiagnostics []LintPackageDiagnostic `json:"package_diagnostics,omitempty"`
	SourceProblems []LintSourceProblem `json:"source_problems,omitempty"`
	AppliedFixes []LintAppliedFix `json:"applied_fixes,omitempty"`
	RejectedFixes []LintRejectedFix `json:"rejected_fixes,omitempty"`
	Errors []Error `json:"errors"`
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
	sort.Slice(
		ordered,
		func(left, right int) bool {
			return ordered[left].Path < ordered[right].Path
		},
	)
	result := LintResult{
		SchemaVersion: SchemaVersion,
		Command: "lint",
		Mode: mode,
		Outcome: Outcome{Category: category, ExitCode: exitCode},
		Summary: LintSummary{Files: len(ordered), Complete: complete},
		Files: make([]LintFile, 0, len(ordered)),
		Diagnostics: make([]LintDiagnostic, 0),
		SuppressionProblems: make([]SuppressionProblem, 0),
		UnusedSuppressions: make([]UnusedSuppression, 0),
		BaselineProblems: make([]BaselineProblem, 0),
		Errors: slices.Clone(errs),
	}
	if result.Errors == nil {
		result.Errors = []Error{}
	}
	for index, analyzed := range ordered {
		if analyzed.Path == "" {
			return LintResult{}, fmt.Errorf("lint result %d has no source path", index)
		}
		if !filepath.IsAbs(analyzed.Path) ||
			filepath.Clean(analyzed.Path) != analyzed.Path {
			return LintResult{}, fmt.Errorf(
				"lint result path %q is not normalized absolute",
				analyzed.Path,
			)
		}
		if analyzed.Digest == (source.Digest{}) {
			return LintResult{}, fmt.Errorf(
				"lint result %q has no source digest",
				analyzed.Path,
			)
		}
		if index > 0 && ordered[index - 1].Path == analyzed.Path {
			return LintResult{}, fmt.Errorf("duplicate source path %q", analyzed.Path)
		}
		digest := encodeDigest(analyzed.Digest)
		result.Files = append(
			result.Files,
			LintFile{
				Path: analyzed.Path,
				SourceDigest: digest,
				Status: LintFileAnalyzed,
			},
		)
		for _, diagnostic := range analysis.OrderDiagnostics(analyzed.Diagnostics) {
			if diagnostic.Path != analyzed.Path ||
				diagnostic.Digest != analyzed.Digest {
				return LintResult{}, fmt.Errorf(
					"diagnostic source identity does not match analysis result %q",
					analyzed.Path,
				)
			}
			result.Diagnostics = append(result.Diagnostics, lintDiagnostic(diagnostic))
		}
		for _, problem := range analyzed.SuppressionProblems {
			result.SuppressionProblems = append(
				result.SuppressionProblems,
				SuppressionProblem{
					Kind: problem.Kind,
					Path: analyzed.Path,
					SourceDigest: digest,
					Range: byteRange(problem.Range),
					Message: problem.Message,
				},
			)
		}
		for _, directive := range analyzed.UnusedSuppressions {
			scope, err := suppressionScope(directive.Scope)
			if err != nil {
				return LintResult{}, fmt.Errorf("%s: %w", analyzed.Path, err)
			}
			result.UnusedSuppressions = append(
				result.UnusedSuppressions,
				UnusedSuppression{
					RuleID: directive.RuleID,
					Scope: scope,
					Path: analyzed.Path,
					SourceDigest: digest,
					Range: byteRange(directive.Range),
					Target: byteRange(directive.Target),
					Reason: directive.Reason,
					ExpiresOn: directive.ExpiresOn,
				},
			)
		}
		for _, problem := range analyzed.BaselineProblems {
			result.BaselineProblems = append(
				result.BaselineProblems,
				BaselineProblem{
					Kind: problem.Kind,
					Path: problem.Entry.Path,
					RuleID: problem.Entry.RuleID,
					MessageKey: problem.Entry.MessageKey,
					SourceFingerprint: problem.Entry.SourceFingerprint,
					Count: problem.Entry.Count,
					Remaining: problem.Remaining,
					Reason: problem.Entry.Reason,
					ExpiresOn: problem.Entry.ExpiresOn,
				},
			)
		}
		result.Summary.Diagnostics += len(analyzed.Diagnostics)
		result.Summary.PreexistingDiagnostics += len(analyzed.PreexistingDiagnostics)
		result.Summary.Suppressed += len(analyzed.Suppressed)
		result.Summary.Baselined += len(analyzed.Baselined)
		result.Summary.BaselineProblems += len(analyzed.BaselineProblems)
		result.Summary.SuppressionProblems += len(analyzed.SuppressionProblems)
		result.Summary.UnusedSuppressions += len(analyzed.UnusedSuppressions)
	}
	return result, nil
}

// NewLintFixResult validates and maps one ordered set of fix transactions.
func NewLintFixResult(
	category string,
	exitCode int,
	complete bool,
	results []analysis.Result,
	outcomes []LintFixOutcome,
	errs []Error,
) (LintResult, error) {
	result, err := NewLintResult("fix", category, exitCode, complete, results, errs)
	if err != nil {
		return LintResult{}, err
	}
	ordered := slices.Clone(outcomes)
	sort.Slice(
		ordered,
		func(left, right int) bool {
			return ordered[left].Path < ordered[right].Path
		},
	)
	if len(ordered) != len(result.Files) {
		return LintResult{}, fmt.Errorf(
			"lint fix outcomes contain %d files, want %d analysis results",
			len(ordered),
			len(result.Files),
		)
	}
	result.AppliedFixes = make([]LintAppliedFix, 0)
	result.RejectedFixes = make([]LintRejectedFix, 0)
	for index, outcome := range ordered {
		file := &result.Files[index]
		if outcome.Path != file.Path {
			return LintResult{}, fmt.Errorf(
				"lint fix outcome %q does not match analysis result %q",
				outcome.Path,
				file.Path,
			)
		}
		if outcome.SourceDigest == (source.Digest{}) {
			return LintResult{}, fmt.Errorf(
				"lint fix outcome %q has no source digest",
				outcome.Path,
			)
		}
		if !validLintFixStatus(outcome.Status) {
			return LintResult{}, fmt.Errorf(
				"lint fix outcome %q has invalid status %q",
				outcome.Path,
				outcome.Status,
			)
		}
		resultMatchesSource := file.SourceDigest == encodeDigest(outcome.SourceDigest)
		switch outcome.Status {
		case LintFileFixed, LintFilePossiblyFixed:
			if resultMatchesSource {
				return LintResult{}, fmt.Errorf(
					"lint fix outcome %q status %q has an unchanged result digest",
					outcome.Path,
					outcome.Status,
				)
			}
		default:
			if !resultMatchesSource {
				return LintResult{}, fmt.Errorf(
					"lint fix outcome %q status %q has a changed result digest",
					outcome.Path,
					outcome.Status,
				)
			}
		}
		file.ResultDigest = file.SourceDigest
		file.SourceDigest = encodeDigest(outcome.SourceDigest)
		file.Status = outcome.Status
		if outcome.Status == LintFileFixed {
			result.Summary.FixedFiles++
		}
		for _, applied := range outcome.Applied {
			if err := validateFixRecord(
				outcome.Path,
				applied.RuleID,
				applied.FixName,
				applied.Range,
			);
				err != nil {
				return LintResult{}, err
			}
			result.AppliedFixes = append(
				result.AppliedFixes,
				LintAppliedFix{
					RuleID: applied.RuleID,
					FixName: applied.FixName,
					Path: outcome.Path,
					SourceDigest: encodeDigest(outcome.SourceDigest),
					Range: byteRange(applied.Range),
				},
			)
		}
		for _, rejected := range outcome.Rejected {
			if err := validateFixRecord(
				outcome.Path,
				rejected.RuleID,
				rejected.FixName,
				rejected.Range,
			);
				err != nil {
				return LintResult{}, err
			}
			if rejected.Reason == "" || rejected.Message == "" {
				return LintResult{}, fmt.Errorf(
					"lint fix rejection %q/%q has no reason or message",
					rejected.RuleID,
					rejected.FixName,
				)
			}
			result.RejectedFixes = append(
				result.RejectedFixes,
				LintRejectedFix{
					RuleID: rejected.RuleID,
					FixName: rejected.FixName,
					Path: outcome.Path,
					SourceDigest: encodeDigest(outcome.SourceDigest),
					Range: byteRange(rejected.Range),
					Reason: rejected.Reason,
					Message: rejected.Message,
				},
			)
		}
	}
	sort.Slice(
		result.AppliedFixes,
		func(left, right int) bool {
			return compareAppliedFix(
				result.AppliedFixes[left],
				result.AppliedFixes[right],
			) <
				0
		},
	)
	sort.Slice(
		result.RejectedFixes,
		func(left, right int) bool {
			return compareRejectedFix(
				result.RejectedFixes[left],
				result.RejectedFixes[right],
			) <
				0
		},
	)
	result.Summary.AppliedFixes = len(result.AppliedFixes)
	result.Summary.RejectedFixes = len(result.RejectedFixes)
	return result, nil
}

func validLintFixStatus(status LintFileStatus) bool {
	switch status {
	case LintFilePending,
		LintFileUnchanged,
		LintFileFixed,
		LintFileConflict,
		LintFileFailed,
		LintFilePossiblyFixed:
		return true
	default:
		return false
	}
}

func validateFixRecord(path, ruleID, fixName string, sourceRange source.Range) error {
	if ruleID == "" || fixName == "" {
		return fmt.Errorf("lint fix record %q has no rule ID or fix name", path)
	}
	if sourceRange.Start < 0 || sourceRange.End < sourceRange.Start {
		return fmt.Errorf("lint fix record %q/%q has invalid range", ruleID, fixName)
	}
	return nil
}

func compareAppliedFix(left, right LintAppliedFix) int {
	if order := cmp.Compare(left.Path, right.Path); order != 0 {
		return order
	}
	if order := cmp.Compare(left.Range.Start, right.Range.Start); order != 0 {
		return order
	}
	if order := cmp.Compare(left.Range.End, right.Range.End); order != 0 {
		return order
	}
	if order := cmp.Compare(left.RuleID, right.RuleID); order != 0 {
		return order
	}
	return cmp.Compare(left.FixName, right.FixName)
}

func compareRejectedFix(left, right LintRejectedFix) int {
	if order := compareAppliedFix(
		LintAppliedFix{
			Path: left.Path,
			Range: left.Range,
			RuleID: left.RuleID,
			FixName: left.FixName,
		},
		LintAppliedFix{
			Path: right.Path,
			Range: right.Range,
			RuleID: right.RuleID,
			FixName: right.FixName,
		},
	);
		order != 0 {
		return order
	}
	if order := cmp.Compare(left.Reason, right.Reason); order != 0 {
		return order
	}
	return cmp.Compare(left.Message, right.Message)
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
		RuleID: diagnostic.RuleID,
		Severity: diagnostic.Severity,
		MessageKey: diagnostic.MessageKey,
		Message: diagnostic.Message,
		Path: diagnostic.Path,
		SourceDigest: encodeDigest(diagnostic.Digest),
		Range: byteRange(diagnostic.Range),
		Related: related,
		Notes: cloneStrings(diagnostic.Notes),
		Help: diagnostic.Help,
		Fixes: fixes,
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
