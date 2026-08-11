package report

import (
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/analysis"
	fixengine "github.com/faustbrian/gox/internal/fix"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
	"github.com/faustbrian/gox/internal/suppressions"
)

func TestRenderLintTextUsesPhysicalLocationsAndNoSourceExcerpt(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run() {\n\ttarget()\n}\n//gox:ignore call-rule -- legacy call\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	targetStart := strings.Index(input, "target()")
	directiveStart := strings.Index(input, "//gox:")
	result := analysis.Result{
		Path:   file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{{
			RuleID:     "call-rule",
			Severity:   rules.SeverityError,
			MessageKey: "call",
			Message:    "call requires review",
			Path:       file.Path(),
			Digest:     file.Digest(),
			Range:      source.Range{Start: targetStart, End: targetStart + len("target()")},
			Related: []rules.Related{{
				Range:   source.Range{Start: strings.Index(input, "func"), End: strings.Index(input, "func") + len("func")},
				Message: "owning function",
			}},
			Notes: []string{"review the result"},
			Help:  "replace the target",
			Fixes: []rules.Fix{{Name: "rewrite", Safety: rules.FixSafe}},
		}},
		SuppressionProblems: []suppressions.Problem{{
			Kind:    suppressions.ProblemMalformed,
			Range:   source.Range{Start: 0, End: len("package")},
			Message: "malformed suppression",
		}},
		UnusedSuppressions: []suppressions.Directive{{
			Scope:  suppressions.ScopeNextLine,
			RuleID: "call-rule",
			Range:  source.Range{Start: directiveStart, End: len(input) - 1},
			Reason: "legacy call",
		}},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	want := "/project/source.go:3:2: error[call-rule]: call requires review\n" +
		"  related /project/source.go:2:1: owning function\n" +
		"  note: review the result\n" +
		"  help: replace the target\n" +
		"  fix[safe]: rewrite\n" +
		"/project/source.go:1:1: suppression[malformed]: malformed suppression\n" +
		"/project/source.go:5:1: unused suppression[call-rule]: legacy call\n"
	if string(got) != want {
		t.Fatalf("RenderLintText() =\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(string(got), "target()") {
		t.Fatal("RenderLintText() disclosed a source excerpt")
	}
}

func TestRenderLintTextRejectsMismatchedSourceIdentity(t *testing.T) {
	t.Parallel()

	input := "package sample\nvar β = 1\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	result := analysis.Result{Path: file.Path(), Digest: source.Digest{1}}
	if _, err := RenderLintText([]LintTextInput{{File: file, Result: result}}); err == nil ||
		!strings.Contains(err.Error(), "source identity") {
		t.Fatalf("RenderLintText() error = %v", err)
	}
	invalidRange := analysis.Result{
		Path:   file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{{
			RuleID:   "invalid-range",
			Severity: rules.SeverityError,
			Message:  "invalid range",
			Path:     file.Path(),
			Digest:   file.Digest(),
			Range:    source.Range{Start: 0, End: len(file.Bytes()) + 1},
		}},
	}
	if _, err := RenderLintText([]LintTextInput{{File: file, Result: invalidRange}}); err == nil ||
		!strings.Contains(err.Error(), "invalid physical range") {
		t.Fatalf("RenderLintText() range error = %v", err)
	}
	midRune := strings.Index(input, "β") + 1
	for _, test := range []struct {
		name         string
		invalidRange source.Range
	}{
		{name: "start", invalidRange: source.Range{Start: midRune, End: midRune + 1}},
		{name: "end", invalidRange: source.Range{Start: midRune - 1, End: midRune}},
	} {
		t.Run("mid-UTF-8 "+test.name, func(t *testing.T) {
			invalidBoundary := analysis.Result{
				Path:   file.Path(),
				Digest: file.Digest(),
				Diagnostics: []rules.Diagnostic{{
					RuleID:   "invalid-boundary",
					Severity: rules.SeverityError,
					Message:  "invalid boundary",
					Path:     file.Path(),
					Digest:   file.Digest(),
					Range:    test.invalidRange,
				}},
			}
			if _, err := RenderLintText([]LintTextInput{{File: file, Result: invalidBoundary}}); err == nil ||
				!strings.Contains(err.Error(), "invalid physical range") {
				t.Fatalf("RenderLintText() UTF-8 boundary error = %v", err)
			}
		})
	}
}

func TestRenderLintTextRejectsInvalidNestedRanges(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	base := analysis.Result{Path: file.Path(), Digest: file.Digest()}
	t.Run("fix edit", func(t *testing.T) {
		result := base
		result.Diagnostics = []rules.Diagnostic{{
			RuleID:   "invalid-fix",
			Severity: rules.SeverityError,
			Message:  "invalid fix",
			Path:     file.Path(),
			Digest:   file.Digest(),
			Range:    source.Range{Start: 0, End: len("package")},
			Fixes: []rules.Fix{{
				Name:   "rewrite",
				Safety: rules.FixSafe,
				Edits: []rules.Edit{{
					Range:   source.Range{Start: 0, End: len(file.Bytes()) + 1},
					NewText: "replacement",
				}},
			}},
		}}
		if _, err := RenderLintText([]LintTextInput{{File: file, Result: result}}); err == nil ||
			!strings.Contains(err.Error(), "fix edit has invalid physical range") {
			t.Fatalf("RenderLintText() fix range error = %v", err)
		}
	})
	t.Run("suppression target", func(t *testing.T) {
		result := base
		result.UnusedSuppressions = []suppressions.Directive{{
			Scope:  suppressions.ScopeNextLine,
			RuleID: "call-rule",
			Range:  source.Range{Start: 0, End: len("package")},
			Target: source.Range{Start: 0, End: len(file.Bytes()) + 1},
		}}
		if _, err := RenderLintText([]LintTextInput{{File: file, Result: result}}); err == nil ||
			!strings.Contains(err.Error(), "suppression target has invalid physical range") {
			t.Fatalf("RenderLintText() suppression target error = %v", err)
		}
	})
}

func TestRenderLintFixTextReportsRejectedFixReasons(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	target := source.Range{Start: strings.Index(input, "target"), End: strings.Index(input, "target") + len("target")}
	output, err := RenderLintFixText([]LintFixTextInput{{
		File:   file,
		Result: analysis.Result{Path: file.Path(), Digest: file.Digest()},
		Outcome: LintFixOutcome{
			Path:         file.Path(),
			SourceDigest: file.Digest(),
			Status:       LintFileConflict,
			Rejected: []fixengine.Rejection{{
				RuleID:  "call-rule",
				FixName: "rewrite",
				Range:   target,
				Reason:  fixengine.RejectionConflict,
				Message: "selected fix conflicts with another edit",
			}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "/project/source.go:2:12: rejected fix[call-rule/rewrite/conflict]: selected fix conflicts with another edit\n"
	if string(output) != want {
		t.Fatalf("RenderLintFixText() = %q, want %q", output, want)
	}
}

func TestRenderLintFixTextSortsRejectedFixesByCompleteIdentity(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	target := source.Range{Start: strings.Index(input, "target"), End: strings.Index(input, "target") + len("target")}
	output, err := RenderLintFixText([]LintFixTextInput{{
		File:   file,
		Result: analysis.Result{Path: file.Path(), Digest: file.Digest()},
		Outcome: LintFixOutcome{
			Path:         file.Path(),
			SourceDigest: file.Digest(),
			Status:       LintFileConflict,
			Rejected: []fixengine.Rejection{
				{
					RuleID:  "call-rule",
					FixName: "rewrite",
					Range:   target,
					Reason:  fixengine.RejectionValidation,
					Message: "validation failed",
				},
				{
					RuleID:  "call-rule",
					FixName: "rewrite",
					Range:   target,
					Reason:  fixengine.RejectionConflict,
					Message: "conflict found",
				},
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	conflict := strings.Index(string(output), "/conflict]")
	validation := strings.Index(string(output), "/validation]")
	if conflict < 0 || validation < 0 || conflict > validation {
		t.Fatalf("RenderLintFixText() rejection order = %q", output)
	}
}
