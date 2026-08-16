package report

import (
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"golang.org/x/tools/go/packages"
)

func TestNewPackageLintResultMapsStableProblemChannels(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package project\n"))
	if err != nil {
		t.Fatal(err)
	}
	problemDigest := source.Digest{1}
	result, err := NewPackageLintResult(
		"check",
		"source-error",
		2,
		true,
		analysis.PackageResult{
			Files: []analysis.Result{{Path: file.Path(), Digest: file.Digest()}},
			LoadDiagnostics: []analysis.PackageDiagnostic{
				{
					PackageID: "z/package",
					Position: "/project/z.go:2:3",
					Message: "undefined: z",
					Kind: packages.TypeError,
				},
				{
					PackageID: "a/package",
					Targets: []string{"darwin/arm64", "linux/amd64"},
					Position: "/project/a.go:1:1",
					Message: "expected declaration",
					Kind: packages.ParseError,
				},
				{
					PackageID: "a/package",
					Position: "-",
					Message: "unclassified problem",
					Kind: packages.UnknownError,
				},
			},
			SourceProblems: []analysis.PackageSourceProblem{
				{
					Path: "/project/z.go",
					Digest: problemDigest,
					Message: "invalid source z",
				},
				{
					Path: "/project/a.go",
					Digest: problemDigest,
					Targets: []string{"darwin/arm64"},
					Message: "invalid source a",
				},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.PackageDiagnostics != 3 ||
		result.Summary.SourceProblems != 2 ||
		len(result.PackageDiagnostics) != 3 ||
		result.PackageDiagnostics[0].PackageID != "a/package" ||
		result.PackageDiagnostics[0].Kind != "unknown" ||
		result.PackageDiagnostics[0].Position != "" ||
		result.PackageDiagnostics[1].Kind != "parse" ||
		!slices.Equal(
			result.PackageDiagnostics[1].Targets,
			[]string{"darwin/arm64", "linux/amd64"},
		) ||
		result.PackageDiagnostics[2].Kind != "type" ||
		len(result.SourceProblems) != 2 ||
		result.SourceProblems[0].Path != "/project/a.go" ||
		!slices.Equal(result.SourceProblems[0].Targets, []string{"darwin/arm64"}) ||
		result.SourceProblems[0].SourceDigest != digestString(problemDigest) {
		t.Fatalf("NewPackageLintResult() = %#v", result)
	}
	encoded, err := MarshalLintJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"package_diagnostics": 3`) ||
		!strings.Contains(string(encoded), `"kind": "parse"`) ||
		!strings.Contains(string(encoded), `"targets": [`) ||
		!strings.Contains(string(encoded), `"source_problems": 2`) {
		t.Fatalf("MarshalLintJSON() = %s", encoded)
	}
}

func TestNewPackageCheckResultCombinesFormattingAndPackageProblems(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package project\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewPackageCheckResult(
		"source_error",
		2,
		true,
		analysis.PackageResult{
			Files: []analysis.Result{{Path: file.Path(), Digest: file.Digest()}},
			LoadDiagnostics: []analysis.PackageDiagnostic{
				{
					PackageID: "example.com/project",
					Position: "/project/source.go:1:1",
					Message: "type prerequisite failed",
					Kind: packages.TypeError,
				},
			},
		},
		[]CheckFormatOutcome{{Path: file.Path(), Digest: file.Digest(), Different: true}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Files != 1 ||
		result.Summary.FormattingDifferences != 1 ||
		result.Summary.PackageDiagnostics != 1 ||
		len(result.PackageDiagnostics) != 1 ||
		len(result.Files) != 1 ||
		result.Files[0].FormatStatus != CheckFormatDifferent {
		t.Fatalf("NewPackageCheckResult() = %#v", result)
	}
}

func TestRenderPackageLintTextOrdersDistinctProblemChannels(t *testing.T) {
	t.Parallel()

	output, err := RenderPackageLintText(
		nil,
		[]analysis.PackageDiagnostic{
			{
				PackageID: "z/package",
				Targets: []string{"linux/amd64"},
				Position: "/project/z.go:2:3",
				Message: "undefined: z",
				Kind: packages.TypeError,
			},
			{
				PackageID: "a/package",
				Position: "/project/a.go:1:1",
				Message: "expected declaration",
				Kind: packages.ParseError,
			},
		},
		[]analysis.PackageSourceProblem{
			{
				Path: "/project/z.go",
				Digest: source.Digest{1},
				Targets: []string{"linux/amd64"},
				Message: "invalid source z",
			},
			{
				Path: "/project/a.go",
				Digest: source.Digest{1},
				Message: "invalid source a",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "/project/a.go:1:1: package[parse] a/package: expected declaration\n" +
		"/project/z.go:2:3: package[type][linux/amd64] z/package: undefined: z\n" +
		"/project/a.go: source: invalid source a\n" +
		"/project/z.go: source[linux/amd64]: invalid source z\n"
	if string(output) != want {
		t.Fatalf("RenderPackageLintText() = %q, want %q", output, want)
	}
}

func TestRenderPackageLintTextSeparatesPrerequisitesFromSourceFrames(t *testing.T) {
	t.Parallel()

	input := "package sample\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "package-rule",
				Severity: rules.SeverityWarn,
				Message: "package requires review",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{Start: 0, End: len("package")},
			},
		},
	}

	output, err := RenderPackageLintText(
		[]LintTextInput{{File: file, Result: result}},
		[]analysis.PackageDiagnostic{
			{
				PackageID: "sample",
				Position: "/project/source.go:1:1",
				Message: "package prerequisite",
				Kind: packages.TypeError,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantBoundary := "package[type] sample: package prerequisite\n\nwarn[package-rule]"
	if !strings.Contains(string(output), wantBoundary) {
		t.Fatalf(
			"RenderPackageLintText() did not separate prerequisites and source frames:\n%s",
			output,
		)
	}
}

func TestPackageLintReportingRejectsInvalidProblemMetadata(t *testing.T) {
	t.Parallel()

	invalid := analysis.PackageResult{
		LoadDiagnostics: []analysis.PackageDiagnostic{
			{PackageID: "package", Message: "problem", Kind: packages.ErrorKind(99)},
		},
	}
	if _, err := NewPackageLintResult("check", "source-error", 2, false, invalid, nil);
		err == nil ||
			!strings.Contains(err.Error(), "unsupported package diagnostic kind") {
		t.Fatalf("NewPackageLintResult() error = %v", err)
	}
	if _, err := RenderPackageLintText(
		nil,
		nil,
		[]analysis.PackageSourceProblem{{Path: "relative.go", Message: "problem"}},
	);
		err == nil || !strings.Contains(err.Error(), "normalized absolute") {
		t.Fatalf("RenderPackageLintText() error = %v", err)
	}
	if _, err := NewPackageLintResult(
		"check",
		"source-error",
		2,
		false,
		analysis.PackageResult{
			SourceProblems: []analysis.PackageSourceProblem{
				{Path: "/project/source.go", Message: "problem"},
			},
		},
		nil,
	);
		err == nil || !strings.Contains(err.Error(), "source digest") {
		t.Fatalf("NewPackageLintResult() digest error = %v", err)
	}
	if _, err := NewPackageLintResult(
		"check",
		"source-error",
		2,
		false,
		analysis.PackageResult{
			LoadDiagnostics: []analysis.PackageDiagnostic{
				{
					PackageID: "package",
					Targets: []string{"linux/amd64", "darwin/arm64"},
					Message: "problem",
					Kind: packages.TypeError,
				},
			},
		},
		nil,
	);
		err == nil || !strings.Contains(err.Error(), "targets are not strictly sorted") {
		t.Fatalf("NewPackageLintResult() target order error = %v", err)
	}
	if _, err := RenderPackageLintText(
		nil,
		nil,
		[]analysis.PackageSourceProblem{
			{
				Path: "/project/source.go",
				Digest: source.Digest{1},
				Targets: []string{" linux/amd64"},
				Message: "problem",
			},
		},
	);
		err == nil || !strings.Contains(err.Error(), "empty or not canonical") {
		t.Fatalf("RenderPackageLintText() target identity error = %v", err)
	}
}
