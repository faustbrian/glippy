package report

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/source"
	"golang.org/x/tools/go/packages"
)

// LintPackageDiagnostic is one stable package-loading, parse, or type error.
type LintPackageDiagnostic struct {
	PackageID string `json:"package_id"`
	Kind string `json:"kind"`
	Position string `json:"position,omitempty"`
	Message string `json:"message"`
}

// LintSourceProblem is one source-model failure retained outside rule results.
type LintSourceProblem struct {
	Path string `json:"path"`
	SourceDigest string `json:"source_digest"`
	Message string `json:"message"`
}

// NewPackageLintResult adds typed prerequisite and source-model problems to a
// versioned lint result without treating them as rule diagnostics.
func NewPackageLintResult(
	mode, category string,
	exitCode int,
	complete bool,
	packageResult analysis.PackageResult,
	errs []Error,
) (LintResult, error) {
	result, err := NewLintResult(mode, category, exitCode, complete, packageResult.Files, errs)
	if err != nil {
		return LintResult{}, err
	}
	packageDiagnostics, err := mapPackageDiagnostics(packageResult.LoadDiagnostics)
	if err != nil {
		return LintResult{}, err
	}
	sourceProblems, err := mapSourceProblems(packageResult.SourceProblems)
	if err != nil {
		return LintResult{}, err
	}
	result.PackageDiagnostics = packageDiagnostics
	result.SourceProblems = sourceProblems
	result.Summary.PackageDiagnostics = len(packageDiagnostics)
	result.Summary.SourceProblems = len(sourceProblems)
	return result, nil
}

// RenderPackageLintText renders prerequisite problems before exact-source rule
// diagnostics while preserving their distinct ownership channels.
func RenderPackageLintText(
	inputs []LintTextInput,
	packageDiagnostics []analysis.PackageDiagnostic,
	sourceProblems []analysis.PackageSourceProblem,
) ([]byte, error) {
	return renderPackageLintText(inputs, packageDiagnostics, sourceProblems, true)
}

// RenderPackageLintShortText renders package prerequisites and source-free
// location-oriented rule diagnostics.
func RenderPackageLintShortText(
	inputs []LintTextInput,
	packageDiagnostics []analysis.PackageDiagnostic,
	sourceProblems []analysis.PackageSourceProblem,
) ([]byte, error) {
	return renderPackageLintText(inputs, packageDiagnostics, sourceProblems, false)
}

func renderPackageLintText(
	inputs []LintTextInput,
	packageDiagnostics []analysis.PackageDiagnostic,
	sourceProblems []analysis.PackageSourceProblem,
	sourceFrames bool,
) ([]byte, error) {
	mappedPackages, err := mapPackageDiagnostics(packageDiagnostics)
	if err != nil {
		return nil, err
	}
	mappedSources, err := mapSourceProblems(sourceProblems)
	if err != nil {
		return nil, err
	}
	var lint []byte
	if sourceFrames {
		lint, err = RenderLintText(inputs)
	} else {
		lint, err = RenderLintShortText(inputs)
	}
	if err != nil {
		return nil, err
	}
	var output strings.Builder
	for _, diagnostic := range mappedPackages {
		if diagnostic.Position != "" && diagnostic.Position != "-" {
			fmt.Fprintf(&output, "%s: ", safeHumanText(diagnostic.Position))
		}
		fmt.Fprintf(
			&output,
			"package[%s] %s: %s\n",
			safeHumanText(diagnostic.Kind),
			safeHumanText(diagnostic.PackageID),
			safeHumanText(diagnostic.Message),
		)
	}
	for _, problem := range mappedSources {
		fmt.Fprintf(
			&output,
			"%s: source: %s\n",
			safeHumanText(problem.Path),
			safeHumanText(problem.Message),
		)
	}
	if sourceFrames && output.Len() > 0 && len(lint) > 0 {
		output.WriteByte('\n')
	}
	output.Write(lint)
	return []byte(output.String()), nil
}

func mapPackageDiagnostics(input []analysis.PackageDiagnostic) ([]LintPackageDiagnostic, error) {
	result := make([]LintPackageDiagnostic, len(input))
	for index, diagnostic := range input {
		if strings.TrimSpace(diagnostic.PackageID) == "" ||
			strings.TrimSpace(diagnostic.Message) == "" {
			return nil, fmt.Errorf(
				"package diagnostic %d requires package ID and message",
				index,
			)
		}
		kind, valid := packageDiagnosticKind(diagnostic.Kind)
		if !valid {
			return nil, fmt.Errorf(
				"package diagnostic %d has unsupported package diagnostic kind %d",
				index,
				diagnostic.Kind,
			)
		}
		position := diagnostic.Position
		if position == "-" {
			position = ""
		}
		result[index] = LintPackageDiagnostic{
			PackageID: diagnostic.PackageID,
			Kind: kind,
			Position: position,
			Message: diagnostic.Message,
		}
	}
	sort.Slice(
		result,
		func(left, right int) bool {
			first, second := result[left], result[right]
			if order := cmp.Compare(first.PackageID, second.PackageID); order != 0 {
				return order < 0
			}
			if order := cmp.Compare(first.Position, second.Position); order != 0 {
				return order < 0
			}
			if order := cmp.Compare(first.Message, second.Message); order != 0 {
				return order < 0
			}
			return first.Kind < second.Kind
		},
	)
	return result, nil
}

func mapSourceProblems(input []analysis.PackageSourceProblem) ([]LintSourceProblem, error) {
	ordered := slices.Clone(input)
	sort.Slice(
		ordered,
		func(left, right int) bool {
			if ordered[left].Path != ordered[right].Path {
				return ordered[left].Path < ordered[right].Path
			}
			return ordered[left].Message < ordered[right].Message
		},
	)
	result := make([]LintSourceProblem, len(ordered))
	for index, problem := range ordered {
		if !filepath.IsAbs(problem.Path) || filepath.Clean(problem.Path) != problem.Path {
			return nil, fmt.Errorf(
				"source problem path %q is not normalized absolute",
				problem.Path,
			)
		}
		if strings.TrimSpace(problem.Message) == "" {
			return nil, fmt.Errorf("source problem %q has no message", problem.Path)
		}
		if problem.Digest == (source.Digest{}) {
			return nil, fmt.Errorf(
				"source problem %q has no source digest",
				problem.Path,
			)
		}
		result[index] = LintSourceProblem{
			Path: problem.Path,
			SourceDigest: encodeDigest(problem.Digest),
			Message: problem.Message,
		}
	}
	return result, nil
}

func packageDiagnosticKind(kind packages.ErrorKind) (string, bool) {
	switch kind {
	case packages.UnknownError:
		return "unknown", true
	case packages.ListError:
		return "list", true
	case packages.ParseError:
		return "parse", true
	case packages.TypeError:
		return "type", true
	default:
		return "", false
	}
}
