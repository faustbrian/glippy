package report

import (
	"fmt"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/baseline"
	fixengine "github.com/faustbrian/glippy/internal/fix"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"github.com/faustbrian/glippy/internal/suppressions"
)

func TestRenderLintTextUsesPhysicalLocationsAndSourceFrames(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run() {\n\ttarget()\n}\n//glippy:ignore call-rule -- legacy call\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	targetStart := strings.Index(input, "target()")
	directiveStart := strings.Index(input, "//glippy:")
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "call-rule",
				Severity: rules.SeverityError,
				MessageKey: "call",
				Message: "call requires review",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{
					Start: targetStart,
					End: targetStart + len("target()"),
				},
				Related: []rules.Related{
					{
						Range: source.Range{
							Start: strings.Index(input, "func"),
							End: strings.Index(input, "func") +
								len("func"),
						},
						Message: "owning function",
					},
				},
				Notes: []string{"review the result"},
				Help: "replace the target",
				Fixes: []rules.Fix{{Name: "rewrite", Safety: rules.FixSafe}},
				WithheldFixes: []rules.WithheldFix{
					{
						Name: "rewrite-with-comments",
						Reason: rules.FixWithheldComments,
						Message: "rewriting this call would remove comments",
					},
				},
			},
		},
		SuppressionProblems: []suppressions.Problem{
			{
				Kind: suppressions.ProblemMalformed,
				Range: source.Range{Start: 0, End: len("package")},
				Message: "malformed suppression",
			},
		},
		UnusedSuppressions: []suppressions.Directive{
			{
				Scope: suppressions.ScopeNextLine,
				RuleID: "call-rule",
				Range: source.Range{Start: directiveStart, End: len(input) - 1},
				Reason: "legacy call",
			},
		},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	want := "error[call-rule]: call requires review\n" +
		"  --> /project/source.go:3:2\n" +
		"   |\n" +
		" 3 |         target()\n" +
		"   |         ^^^^^^^^\n" +
		"   |\n" +
		"   = related: /project/source.go:2:1: owning function\n" +
		"   = note: review the result\n" +
		"   = help: replace the target\n" +
		"   = fix[safe]: rewrite\n" +
		"   = fix[rewrite-with-comments] withheld[comments]: rewriting this call would remove comments\n" +
		"\n" +
		"suppression[malformed]: malformed suppression\n" +
		"  --> /project/source.go:1:1\n" +
		"   |\n" +
		" 1 | package sample\n" +
		"   | ^^^^^^^\n" +
		"\n" +
		"unused suppression[call-rule]: legacy call\n" +
		"  --> /project/source.go:5:1\n" +
		"   |\n" +
		" 5 | //glippy:ignore call-rule -- legacy call\n" +
		"   | ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^\n"
	if string(got) != want {
		t.Fatalf("RenderLintText() =\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(string(got), "target()") {
		t.Fatal("RenderLintText() omitted the source excerpt")
	}
}

func TestRenderLintTextDistinguishesStaleAndExpiredBaselineEntries(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	entry := baseline.Entry{
		RuleID: "call-rule",
		Path: "source.go",
		MessageKey: "call",
		SourceFingerprint: strings.Repeat("a", 64),
		Count: 2,
	}
	expired := entry
	expired.ExpiresOn = "2026-08-13"
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		BaselineProblems: []baseline.Problem{
			{Kind: baseline.ProblemStale, Entry: entry, Remaining: 1},
			{Kind: baseline.ProblemExpired, Entry: expired, Remaining: 2},
		},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	want := "/project/source.go: baseline[stale]: call-rule/call has 1 unmatched occurrence(s)\n" +
		"/project/source.go: baseline[expired]: call-rule/call expired on 2026-08-13 (2 configured occurrence(s))\n"
	if string(got) != want {
		t.Fatalf("RenderLintText() =\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderLintTextSeparatesSourceFramesFromBaselineProblems(t *testing.T) {
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
		BaselineProblems: []baseline.Problem{
			{
				Kind: baseline.ProblemStale,
				Entry: baseline.Entry{
					RuleID: "old-rule",
					MessageKey: "old-message",
				},
				Remaining: 1,
			},
		},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	wantBoundary := "   | ^^^^^^^\n\n/project/source.go: baseline[stale]"
	if !strings.Contains(string(got), wantBoundary) {
		t.Fatalf(
			"RenderLintText() did not separate framed and source-free findings:\n%s",
			got,
		)
	}
}

func TestRenderLintTextFramesEveryLineInAMultilineRange(t *testing.T) {
	t.Parallel()

	input := "package sample\nvar value = `first\nsecond`\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(input, "`first")
	end := strings.Index(input, "second`") + len("second`")
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "multiline",
				Severity: rules.SeverityWarn,
				Message: "range spans source lines",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{Start: start, End: end},
			},
		},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	want := "warn[multiline]: range spans source lines\n" +
		"  --> /project/source.go:2:13\n" +
		"   |\n" +
		" 2 | var value = `first\n" +
		"   |             ^^^^^^\n" +
		" 3 | second`\n" +
		"   | ^^^^^^^\n"
	if string(got) != want {
		t.Fatalf("RenderLintText() =\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderLintTextBoundsLongSourceLinesAroundTheDiagnostic(t *testing.T) {
	t.Parallel()

	prefix := strings.Repeat("a", 4096)
	input := "package sample\n// " + prefix + " target " + prefix + "\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(input, "target")
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "bounded-frame",
				Severity: rules.SeverityWarn,
				Message: "long source line",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{Start: start, End: start + len("target")},
			},
		},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 1024 {
		t.Fatalf("RenderLintText() emitted %d bytes for one long source line", len(got))
	}
	if strings.Count(string(got), "...") != 2 || !strings.Contains(string(got), "target") {
		t.Fatalf("RenderLintText() did not retain a bounded diagnostic window:\n%s", got)
	}
}

func TestRenderLintShortTextKeepsSourceFreeLocationOutput(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run() { target() }\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(input, "target")
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "call-rule",
				Severity: rules.SeverityWarn,
				Message: "call requires review",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{Start: start, End: start + len("target")},
				WithheldFixes: []rules.WithheldFix{
					{
						Name: "rewrite",
						Reason: rules.FixWithheldComments,
						Message: "rewriting this call would remove comments",
					},
				},
			},
		},
	}

	got, err := RenderLintShortText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	want := "/project/source.go:2:14: warn[call-rule]: call requires review\n" +
		"  fix[rewrite] withheld[comments]: rewriting this call would remove comments\n"
	if string(got) != want {
		t.Fatalf("RenderLintShortText() = %q, want %q", got, want)
	}
	if strings.Contains(string(got), "target()") {
		t.Fatal("RenderLintShortText() disclosed a source excerpt")
	}
}

func TestRenderLintTextEscapesTerminalControlCharacters(t *testing.T) {
	t.Parallel()

	input := "package sample\n// dangerous \x1b[31m target\n"
	file, err := source.Load("/project/\x1bsource.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(input, "target")
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "terminal-safe",
				Severity: rules.SeverityWarn,
				Message: "source contains terminal control data \x1b[31m",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{Start: start, End: start + len("target")},
				Related: []rules.Related{
					{
						Range: source.Range{Start: 0, End: len("package")},
						Message: "related terminal data \x1b[32m",
					},
				},
			},
		},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(string(got), '\x1b') {
		t.Fatalf("RenderLintText() emitted a terminal escape byte: %q", got)
	}
	if !strings.Contains(string(got), `\x1b[31m target`) {
		t.Fatalf("RenderLintText() did not render escaped source safely: %q", got)
	}
}

func TestRenderLintTextPlacesZeroWidthMarkersAtTheirPhysicalOffset(t *testing.T) {
	t.Parallel()

	input := "package sample\nvar value = 1\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(input, "value")
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "point",
				Severity: rules.SeverityWarn,
				Message: "point diagnostic",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{Start: start, End: start},
			},
		},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "   |     ^\n") {
		t.Fatalf("RenderLintText() misplaced the point marker:\n%s", got)
	}
}

func TestRenderLintTextFramesPointDiagnosticsAtTrailingEOF(t *testing.T) {
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
				RuleID: "eof-point",
				Severity: rules.SeverityWarn,
				Message: "point at end of file",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{Start: len(input), End: len(input)},
			},
		},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	want := "warn[eof-point]: point at end of file\n" +
		"  --> /project/source.go:2:1\n" +
		"   |\n" +
		" 2 | \n" +
		"   | ^\n"
	if string(got) != want {
		t.Fatalf("RenderLintText() =\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderLintTextAlignsASelectedCombiningRune(t *testing.T) {
	t.Parallel()

	input := "package sample\n// e\u0301x\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(input, "\u0301")
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "combining-marker",
				Severity: rules.SeverityWarn,
				Message: "combining source range",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{Start: start, End: start + len("\u0301")},
			},
		},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "   |    ^\n") {
		t.Fatalf("RenderLintText() misaligned a selected combining rune:\n%s", got)
	}
}

func TestRenderLintTextMeasuresWideUnicodeForMarkerAlignment(t *testing.T) {
	t.Parallel()

	input := "package sample\n// 界 target\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(input, "target")
	result := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "wide-marker",
				Severity: rules.SeverityWarn,
				Message: "wide source prefix",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{Start: start, End: start + len("target")},
			},
		},
	}

	got, err := RenderLintText([]LintTextInput{{File: file, Result: result}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "   |       ^^^^^^\n") {
		t.Fatalf("RenderLintText() misaligned a wide source prefix:\n%s", got)
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
	if _, err := RenderLintText([]LintTextInput{{File: file, Result: result}});
		err == nil || !strings.Contains(err.Error(), "source identity") {
		t.Fatalf("RenderLintText() error = %v", err)
	}
	invalidRange := analysis.Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Diagnostics: []rules.Diagnostic{
			{
				RuleID: "invalid-range",
				Severity: rules.SeverityError,
				Message: "invalid range",
				Path: file.Path(),
				Digest: file.Digest(),
				Range: source.Range{Start: 0, End: len(file.Bytes()) + 1},
			},
		},
	}
	if _, err := RenderLintText([]LintTextInput{{File: file, Result: invalidRange}});
		err == nil || !strings.Contains(err.Error(), "invalid physical range") {
		t.Fatalf("RenderLintText() range error = %v", err)
	}
	midRune := strings.Index(input, "β") + 1
	for _, test := range
		[]struct {
			name string
			invalidRange source.Range
		}{
			{
				name: "start",
				invalidRange: source.Range{Start: midRune, End: midRune + 1},
			},
			{name: "end", invalidRange: source.Range{Start: midRune - 1, End: midRune}},
		} {
		t.Run(
			"mid-UTF-8 " + test.name,
			func(t *testing.T) {
				invalidBoundary := analysis.Result{
					Path: file.Path(),
					Digest: file.Digest(),
					Diagnostics: []rules.Diagnostic{
						{
							RuleID: "invalid-boundary",
							Severity: rules.SeverityError,
							Message: "invalid boundary",
							Path: file.Path(),
							Digest: file.Digest(),
							Range: test.invalidRange,
						},
					},
				}
				if _, err := RenderLintText(
					[]LintTextInput{{File: file, Result: invalidBoundary}},
				);
					err == nil ||
						!strings.Contains(
							err.Error(),
							"invalid physical range",
						) {
					t.Fatalf("RenderLintText() UTF-8 boundary error = %v", err)
				}
			},
		)
	}
}

func TestRenderLintTextRejectsInvalidNestedRanges(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	base := analysis.Result{Path: file.Path(), Digest: file.Digest()}
	t.Run(
		"fix edit",
		func(t *testing.T) {
			result := base
			result.Diagnostics = []rules.Diagnostic{
				{
					RuleID: "invalid-fix",
					Severity: rules.SeverityError,
					Message: "invalid fix",
					Path: file.Path(),
					Digest: file.Digest(),
					Range: source.Range{Start: 0, End: len("package")},
					Fixes: []rules.Fix{
						{
							Name: "rewrite",
							Safety: rules.FixSafe,
							Edits: []rules.Edit{
								{
									Range: source.Range{
										Start: 0,
										End: len(
											file.Bytes(),
										) +
											1,
									},
									NewText: "replacement",
								},
							},
						},
					},
				},
			}
			if _, err := RenderLintText([]LintTextInput{{File: file, Result: result}});
				err == nil ||
					!strings.Contains(
						err.Error(),
						"fix edit has invalid physical range",
					) {
				t.Fatalf("RenderLintText() fix range error = %v", err)
			}
		},
	)
	t.Run(
		"suppression target",
		func(t *testing.T) {
			result := base
			result.UnusedSuppressions = []suppressions.Directive{
				{
					Scope: suppressions.ScopeNextLine,
					RuleID: "call-rule",
					Range: source.Range{Start: 0, End: len("package")},
					Target: source.Range{Start: 0, End: len(file.Bytes()) + 1},
				},
			}
			if _, err := RenderLintText([]LintTextInput{{File: file, Result: result}});
				err == nil ||
					!strings.Contains(
						err.Error(),
						"suppression target has invalid physical range",
					) {
				t.Fatalf("RenderLintText() suppression target error = %v", err)
			}
		},
	)
}

func TestRenderLintFixTextReportsRejectedFixReasons(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	target := source.Range{
		Start: strings.Index(input, "target"),
		End: strings.Index(input, "target") + len("target"),
	}
	output, err := RenderLintFixText(
		[]LintFixTextInput{
			{
				File: file,
				Result: analysis.Result{Path: file.Path(), Digest: file.Digest()},
				Outcome: LintFixOutcome{
					Path: file.Path(),
					SourceDigest: file.Digest(),
					Status: LintFileConflict,
					Rejected: []fixengine.Rejection{
						{
							RuleID: "call-rule",
							FixName: "rewrite",
							Range: target,
							Reason: fixengine.RejectionConflict,
							Message: "selected fix conflicts with another edit",
						},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "rejected fix[call-rule/rewrite/conflict]: selected fix conflicts with another edit\n" +
		"  --> /project/source.go:2:12\n" +
		"   |\n" +
		" 2 | func run(){target()}\n" +
		"   |            ^^^^^^\n"
	if string(output) != want {
		t.Fatalf("RenderLintFixText() = %q, want %q", output, want)
	}

	short, err := RenderLintFixShortText(
		[]LintFixTextInput{
			{
				File: file,
				Result: analysis.Result{Path: file.Path(), Digest: file.Digest()},
				Outcome: LintFixOutcome{
					Path: file.Path(),
					SourceDigest: file.Digest(),
					Status: LintFileConflict,
					Rejected: []fixengine.Rejection{
						{
							RuleID: "call-rule",
							FixName: "rewrite",
							Range: target,
							Reason: fixengine.RejectionConflict,
							Message: "selected fix conflicts with another edit",
						},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	shortWant := "/project/source.go:2:12: rejected fix[call-rule/rewrite/conflict]: selected fix conflicts with another edit\n"
	if string(short) != shortWant {
		t.Fatalf("RenderLintFixShortText() = %q, want %q", short, shortWant)
	}
}

func TestRenderLintFixTextSortsRejectedFixesByCompleteIdentity(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	target := source.Range{
		Start: strings.Index(input, "target"),
		End: strings.Index(input, "target") + len("target"),
	}
	output, err := RenderLintFixText(
		[]LintFixTextInput{
			{
				File: file,
				Result: analysis.Result{Path: file.Path(), Digest: file.Digest()},
				Outcome: LintFixOutcome{
					Path: file.Path(),
					SourceDigest: file.Digest(),
					Status: LintFileConflict,
					Rejected: []fixengine.Rejection{
						{
							RuleID: "call-rule",
							FixName: "rewrite",
							Range: target,
							Reason: fixengine.RejectionValidation,
							Message: "validation failed",
						},
						{
							RuleID: "call-rule",
							FixName: "rewrite",
							Range: target,
							Reason: fixengine.RejectionConflict,
							Message: "conflict found",
						},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	conflict := strings.Index(string(output), "/conflict]")
	validation := strings.Index(string(output), "/validation]")
	if conflict < 0 || validation < 0 || conflict > validation {
		t.Fatalf("RenderLintFixText() rejection order = %q", output)
	}
}

func BenchmarkRenderLintTextSourceFrames(b *testing.B) {
	var input strings.Builder
	input.WriteString("package sample\n\nfunc run() {\n")
	ranges := make([]source.Range, 0, 100)
	for index := range 100 {
		name := fmt.Sprintf("value%d", index)
		line := fmt.Sprintf("\t%s := %d\n", name, index)
		start := input.Len() + strings.Index(line, name)
		input.WriteString(line)
		ranges = append(ranges, source.Range{Start: start, End: start + len(name)})
	}
	input.WriteString("}\n")
	file, err := source.Load("/project/source.go", []byte(input.String()))
	if err != nil {
		b.Fatal(err)
	}
	diagnostics := make([]rules.Diagnostic, len(ranges))
	for index, range_ := range ranges {
		diagnostics[index] = rules.Diagnostic{
			RuleID: "benchmark-rule",
			Severity: rules.SeverityWarn,
			Message: "benchmark diagnostic",
			Path: file.Path(),
			Digest: file.Digest(),
			Range: range_,
		}
	}
	inputs := []LintTextInput{
		{
			File: file,
			Result: analysis.Result{
				Path: file.Path(),
				Digest: file.Digest(),
				Diagnostics: diagnostics,
			},
		},
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := RenderLintText(inputs); err != nil {
			b.Fatal(err)
		}
	}
}
