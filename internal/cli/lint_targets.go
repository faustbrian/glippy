package cli

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/config"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/suppressions"
)

func runLintTargetMatrix(
	ctx context.Context,
	registry *rules.Registry,
	task lintPackageTask,
) (analysis.PackageResult, error) {
	targets := slices.Clone(task.options.buildSelection.Targets)
	sort.Slice(
		targets,
		func(left, right int) bool {
			return targets[left].ID() < targets[right].ID()
		},
	)
	var combined analysis.PackageResult
	for _, target := range targets {
		selected := task
		selected.options.buildSelection = config.Analysis{
			BuildTags: slices.Clone(target.BuildTags),
			GOOS: target.GOOS,
			GOARCH: target.GOARCH,
			CGOEnabled: target.CGOEnabled,
			ContractFiles: slices.Clone(task.options.buildSelection.ContractFiles),
			Contracts: task.options.buildSelection.Contracts,
		}
		result, err := runPackageAnalysis(ctx, registry, selected)
		if err != nil {
			return combined, fmt.Errorf("analyze target %s: %w", target.ID(), err)
		}
		if err := tagPackageResult(&result, target.ID()); err != nil {
			return combined, fmt.Errorf("tag target %s: %w", target.ID(), err)
		}
		combined, err = mergeTargetPackageResults(combined, result)
		if err != nil {
			return combined, err
		}
	}
	return combined, nil
}

func tagPackageResult(result *analysis.PackageResult, target string) error {
	targetedSources, err := result.Sources.WithProblemTargets([]string{target})
	if err != nil {
		return err
	}
	result.Sources = targetedSources
	for index := range result.LoadDiagnostics {
		result.LoadDiagnostics[index].Targets = []string{target}
	}
	for index := range result.SourceProblems {
		result.SourceProblems[index].Targets = []string{target}
	}
	for index := range result.Files {
		file := &result.Files[index]
		file.Targets = []string{target}
		tagDiagnostics(file.Diagnostics, target)
		tagDiagnostics(file.PreexistingDiagnostics, target)
		tagDiagnostics(file.Baselined, target)
		for suppressedIndex := range file.Suppressed {
			file.Suppressed[suppressedIndex].Diagnostic.Targets = []string{target}
		}
	}
	return nil
}

func tagDiagnostics(diagnostics []rules.Diagnostic, target string) {
	for index := range diagnostics {
		diagnostics[index].Targets = []string{target}
	}
}

func mergeTargetPackageResults(
	left analysis.PackageResult,
	right analysis.PackageResult,
) (analysis.PackageResult, error) {
	if len(left.Files) == 0 &&
		len(left.LoadDiagnostics) == 0 &&
		len(left.SourceProblems) == 0 &&
		len(left.Sources.Paths()) == 0 {
		return right, nil
	}
	partial := left
	mergedSources, err := analysis.MergePackageSourceSets(left.Sources, right.Sources)
	if err != nil {
		return partial, err
	}
	left.Sources = mergedSources
	if right.Requirement > left.Requirement {
		left.Requirement = right.Requirement
	}
	if len(left.Selection) == 0 {
		left.Selection = slices.Clone(right.Selection)
	} else if !reflect.DeepEqual(left.Selection, right.Selection) {
		return partial, fmt.Errorf("target matrix resolved incompatible lint selections")
	}
	left.LoadDiagnostics = mergePackageDiagnostics(left.LoadDiagnostics, right.LoadDiagnostics)
	left.SourceProblems = mergeSourceProblems(left.SourceProblems, right.SourceProblems)
	byPath := make(map[string]int, len(left.Files))
	for index := range left.Files {
		byPath[left.Files[index].Path] = index
	}
	for _, candidate := range right.Files {
		index, found := byPath[candidate.Path]
		if !found {
			left.Files = append(left.Files, candidate)
			byPath[candidate.Path] = len(left.Files) - 1
			continue
		}
		merged, err := mergeTargetFileResults(left.Files[index], candidate)
		if err != nil {
			return partial, err
		}
		left.Files[index] = merged
	}
	sort.Slice(
		left.Files,
		func(first, second int) bool {
			return left.Files[first].Path < left.Files[second].Path
		},
	)
	return left, nil
}

func mergeTargetFileResults(left, right analysis.Result) (analysis.Result, error) {
	if left.Path != right.Path || left.Digest != right.Digest {
		return analysis.Result{}, fmt.Errorf(
			"target matrix source identity changed for %q",
			right.Path,
		)
	}
	left.Targets = mergeTargetNames(left.Targets, right.Targets)
	if right.Requirement > left.Requirement {
		left.Requirement = right.Requirement
	}
	if !reflect.DeepEqual(left.Selection, right.Selection) {
		return analysis.Result{}, fmt.Errorf(
			"target matrix selection changed for %q",
			right.Path,
		)
	}
	left.Diagnostics = mergeRuleDiagnostics(left.Diagnostics, right.Diagnostics)
	left.PreexistingDiagnostics = mergeRuleDiagnostics(
		left.PreexistingDiagnostics,
		right.PreexistingDiagnostics,
	)
	left.Baselined = mergeRuleDiagnostics(left.Baselined, right.Baselined)
	left.BaselineProblems = mergeComparableValues(left.BaselineProblems, right.BaselineProblems)
	left.Suppressed = mergeSuppressedDiagnostics(left.Suppressed, right.Suppressed)
	left.UnusedSuppressions = mergeComparableValues(
		left.UnusedSuppressions,
		right.UnusedSuppressions,
	)
	left.SuppressionProblems = mergeComparableValues(
		left.SuppressionProblems,
		right.SuppressionProblems,
	)
	return left, nil
}

func mergeRuleDiagnostics(left, right []rules.Diagnostic) []rules.Diagnostic {
	result := slices.Clone(left)
	for _, candidate := range right {
		found := false
		for index := range result {
			currentTargets := result[index].Targets
			candidateTargets := candidate.Targets
			result[index].Targets = nil
			candidate.Targets = nil
			equal := reflect.DeepEqual(result[index], candidate)
			result[index].Targets = currentTargets
			candidate.Targets = candidateTargets
			if equal {
				result[index].Targets = mergeTargetNames(
					currentTargets,
					candidateTargets,
				)
				found = true
				break
			}
		}
		if !found {
			result = append(result, candidate)
		}
	}
	return analysis.OrderDiagnostics(result)
}

func mergeSuppressedDiagnostics(
	left, right []suppressions.SuppressedDiagnostic,
) []suppressions.SuppressedDiagnostic {
	result := slices.Clone(left)
	for _, candidate := range right {
		found := false
		for index := range result {
			currentTargets := result[index].Diagnostic.Targets
			candidateTargets := candidate.Diagnostic.Targets
			result[index].Diagnostic.Targets = nil
			candidate.Diagnostic.Targets = nil
			equal := reflect.DeepEqual(result[index], candidate)
			result[index].Diagnostic.Targets = currentTargets
			candidate.Diagnostic.Targets = candidateTargets
			if equal {
				result[index].Diagnostic.Targets = mergeTargetNames(
					currentTargets,
					candidateTargets,
				)
				found = true
				break
			}
		}
		if !found {
			result = append(result, candidate)
		}
	}
	return result
}

func mergePackageDiagnostics(
	left, right []analysis.PackageDiagnostic,
) []analysis.PackageDiagnostic {
	result := slices.Clone(left)
	for _, candidate := range right {
		found := false
		for index := range result {
			if result[index].PackageID == candidate.PackageID &&
				result[index].Position == candidate.Position &&
				result[index].Message == candidate.Message &&
				result[index].Kind == candidate.Kind {
				result[index].Targets = mergeTargetNames(
					result[index].Targets,
					candidate.Targets,
				)
				found = true
				break
			}
		}
		if !found {
			result = append(result, candidate)
		}
	}
	return result
}

func mergeSourceProblems(
	left, right []analysis.PackageSourceProblem,
) []analysis.PackageSourceProblem {
	result := slices.Clone(left)
	for _, candidate := range right {
		found := false
		for index := range result {
			if result[index].Path == candidate.Path &&
				result[index].Digest == candidate.Digest &&
				result[index].Message == candidate.Message {
				result[index].Targets = mergeTargetNames(
					result[index].Targets,
					candidate.Targets,
				)
				found = true
				break
			}
		}
		if !found {
			result = append(result, candidate)
		}
	}
	return result
}

func mergeTargetNames(left, right []string) []string {
	result := append(slices.Clone(left), right...)
	sort.Strings(result)
	return slices.Compact(result)
}

func mergeComparableValues[T any](left, right []T) []T {
	result := slices.Clone(left)
	for _, candidate := range right {
		found := false
		for _, existing := range result {
			if reflect.DeepEqual(existing, candidate) {
				found = true
				break
			}
		}
		if !found {
			result = append(result, candidate)
		}
	}
	return result
}
