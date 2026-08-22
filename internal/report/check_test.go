package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestNewCheckResultSortsFilesAndCountsFormattingDifferences(t *testing.T) {
	t.Parallel()

	first, err := source.Load("/project/a.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := source.Load("/project/b.go", []byte("package sample\nfunc run() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := rules.Diagnostic{
		RuleID: "sample-rule",
		Severity: rules.SeverityWarn,
		MessageKey: "sample",
		Message: "sample finding",
		Path: second.Path(),
		Digest: second.Digest(),
		Range: source.Range{Start: 15, End: 18},
	}

	result, err := NewCheckResult(
		"findings",
		1,
		true,
		[]analysis.Result{
			{
				Path: second.Path(),
				Digest: second.Digest(),
				Diagnostics: []rules.Diagnostic{diagnostic},
			},
			{Path: first.Path(), Digest: first.Digest()},
		},
		[]CheckFormatOutcome{
			{Path: first.Path(), Digest: first.Digest(), Different: true},
			{Path: second.Path(), Digest: second.Digest()},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Files != 2 ||
		result.Summary.FormattingDifferences != 1 ||
		result.Summary.Diagnostics != 1 ||
		!result.Summary.Complete {
		t.Fatalf("NewCheckResult() summary = %#v", result.Summary)
	}
	if result.SchemaVersion != 2 {
		t.Fatalf("NewCheckResult() schema version = %d, want 2", result.SchemaVersion)
	}
	if len(result.Files) != 2 ||
		result.Files[0].Path != first.Path() ||
		result.Files[0].FormatStatus != CheckFormatDifferent ||
		result.Files[1].Path != second.Path() ||
		result.Files[1].FormatStatus != CheckFormatUnchanged {
		t.Fatalf("NewCheckResult() files = %#v", result.Files)
	}
	if len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].Path != second.Path() ||
		result.Diagnostics[0].SourceDigest != result.Files[1].SourceDigest {
		t.Fatalf("NewCheckResult() diagnostics = %#v", result.Diagnostics)
	}

	firstJSON, err := MarshalCheckJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := MarshalCheckJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) ||
		bytes.Index(firstJSON, []byte(first.Path())) >
			bytes.Index(firstJSON, []byte(second.Path())) {
		t.Fatalf("MarshalCheckJSON() is not stable or path ordered:\n%s", firstJSON)
	}
}

func TestNewCheckResultRejectsMissingOrMismatchedFormatOutcomes(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	analysisResult := analysis.Result{Path: file.Path(), Digest: file.Digest()}

	tests := []struct {
		name string
		formats []CheckFormatOutcome
		message string
	}{
		{name: "missing", message: "want 1 analysis results"},
		{
			name: "path mismatch",
			formats: []CheckFormatOutcome{
				{Path: "/project/other.go", Digest: file.Digest()},
			},
			message: "source identity",
		},
		{
			name: "digest mismatch",
			formats: []CheckFormatOutcome{
				{Path: file.Path(), Digest: source.Digest{1}},
			},
			message: "source identity",
		},
		{
			name: "relative path",
			formats: []CheckFormatOutcome{{Path: "source.go", Digest: file.Digest()}},
			message: "normalized absolute",
		},
		{
			name: "pending in complete result",
			formats: []CheckFormatOutcome{
				{Path: file.Path(), Digest: file.Digest(), Pending: true},
			},
			message: "complete check result has pending formatting",
		},
		{
			name: "pending and different",
			formats: []CheckFormatOutcome{
				{
					Path: file.Path(),
					Digest: file.Digest(),
					Pending: true,
					Different: true,
				},
			},
			message: "pending with a completed status",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				_, err := NewCheckResult(
					"success",
					0,
					true,
					[]analysis.Result{analysisResult},
					test.formats,
					nil,
				)
				if err == nil || !strings.Contains(err.Error(), test.message) {
					t.Fatalf(
						"NewCheckResult() error = %v, want %q",
						err,
						test.message,
					)
				}
			},
		)
	}
}

func TestNewCheckResultRepresentsPendingFormatting(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewCheckResult(
		"source_error",
		2,
		false,
		[]analysis.Result{{Path: file.Path(), Digest: file.Digest()}},
		[]CheckFormatOutcome{{Path: file.Path(), Digest: file.Digest(), Pending: true}},
		[]Error{{Message: "analysis stopped before formatting"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Complete ||
		len(result.Files) != 1 ||
		result.Files[0].FormatStatus != CheckFormatPending {
		t.Fatalf("pending check result = %#v", result)
	}
}
