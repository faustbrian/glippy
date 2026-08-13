package report

import (
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/source"
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
		result.PackageDiagnostics[2].Kind != "type" ||
		len(result.SourceProblems) != 2 ||
		result.SourceProblems[0].Path != "/project/a.go" ||
		result.SourceProblems[0].SourceDigest != digestString(problemDigest) {
		t.Fatalf("NewPackageLintResult() = %#v", result)
	}
	encoded, err := MarshalLintJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"package_diagnostics": 3`) ||
		!strings.Contains(string(encoded), `"kind": "parse"`) ||
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
		"/project/z.go:2:3: package[type] z/package: undefined: z\n" +
		"/project/a.go: source: invalid source a\n" +
		"/project/z.go: source: invalid source z\n"
	if string(output) != want {
		t.Fatalf("RenderPackageLintText() = %q, want %q", output, want)
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
}
