package report

import (
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
	"github.com/faustbrian/gox/internal/suppressions"
)

func TestNewLintResultEmitsStableVersionedDiagnostics(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\nfunc run(){target()}\n"))
	if err != nil {
		t.Fatal(err)
	}
	target := source.Range{Start: 26, End: 34}
	diagnostic := rules.Diagnostic{
		RuleID:     "call-rule",
		Severity:   rules.SeverityError,
		MessageKey: "call",
		Message:    "call requires review",
		Path:       file.Path(),
		Digest:     file.Digest(),
		Range:      target,
		Related: []rules.Related{{
			Range:   source.Range{Start: 15, End: 20},
			Message: "owning function",
		}},
		Notes: []string{"review the result"},
		Help:  "replace the target",
		Fixes: []rules.Fix{{
			Name:   "rewrite",
			Safety: rules.FixSafe,
			Edits:  []rules.Edit{{Range: target, NewText: "primary()"}},
		}},
	}
	result, err := NewLintResult(
		"check",
		"findings",
		1,
		true,
		[]analysis.Result{{
			Path:        file.Path(),
			Digest:      file.Digest(),
			Requirement: rules.RequireSyntax,
			Diagnostics: []rules.Diagnostic{diagnostic},
			Suppressed: []suppressions.SuppressedDiagnostic{{
				Diagnostic: diagnostic,
			}},
			SuppressionProblems: []suppressions.Problem{{
				Kind:    suppressions.ProblemMalformed,
				Range:   source.Range{Start: 0, End: 7},
				Message: "malformed suppression",
			}},
			UnusedSuppressions: []suppressions.Directive{{
				Scope:  suppressions.ScopeNextLine,
				RuleID: "call-rule",
				Range:  source.Range{Start: 8, End: 20},
				Target: source.Range{Start: 21, End: 35},
				Reason: "legacy call",
			}},
		}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalLintJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	digest := file.Digest()
	want := "{\n" +
		"  \"schema_version\": 1,\n" +
		"  \"command\": \"lint\",\n" +
		"  \"mode\": \"check\",\n" +
		"  \"outcome\": {\n" +
		"    \"category\": \"findings\",\n" +
		"    \"exit_code\": 1\n" +
		"  },\n" +
		"  \"summary\": {\n" +
		"    \"files\": 1,\n" +
		"    \"diagnostics\": 1,\n" +
		"    \"suppressed\": 1,\n" +
		"    \"suppression_problems\": 1,\n" +
		"    \"unused_suppressions\": 1,\n" +
		"    \"complete\": true\n" +
		"  },\n" +
		"  \"files\": [\n" +
		"    {\n" +
		"      \"path\": \"/project/source.go\",\n" +
		"      \"source_digest\": \"" + digestString(digest) + "\",\n" +
		"      \"status\": \"analyzed\"\n" +
		"    }\n" +
		"  ],\n" +
		"  \"diagnostics\": [\n" +
		"    {\n" +
		"      \"rule_id\": \"call-rule\",\n" +
		"      \"severity\": \"error\",\n" +
		"      \"message_key\": \"call\",\n" +
		"      \"message\": \"call requires review\",\n" +
		"      \"path\": \"/project/source.go\",\n" +
		"      \"source_digest\": \"" + digestString(digest) + "\",\n" +
		"      \"range\": {\n" +
		"        \"start\": 26,\n" +
		"        \"end\": 34\n" +
		"      },\n" +
		"      \"related\": [\n" +
		"        {\n" +
		"          \"range\": {\n" +
		"            \"start\": 15,\n" +
		"            \"end\": 20\n" +
		"          },\n" +
		"          \"message\": \"owning function\"\n" +
		"        }\n" +
		"      ],\n" +
		"      \"notes\": [\n" +
		"        \"review the result\"\n" +
		"      ],\n" +
		"      \"help\": \"replace the target\",\n" +
		"      \"fixes\": [\n" +
		"        {\n" +
		"          \"name\": \"rewrite\",\n" +
		"          \"safety\": \"safe\"\n" +
		"        }\n" +
		"      ]\n" +
		"    }\n" +
		"  ],\n" +
		"  \"suppression_problems\": [\n" +
		"    {\n" +
		"      \"kind\": \"malformed\",\n" +
		"      \"path\": \"/project/source.go\",\n" +
		"      \"source_digest\": \"" + digestString(digest) + "\",\n" +
		"      \"range\": {\n" +
		"        \"start\": 0,\n" +
		"        \"end\": 7\n" +
		"      },\n" +
		"      \"message\": \"malformed suppression\"\n" +
		"    }\n" +
		"  ],\n" +
		"  \"unused_suppressions\": [\n" +
		"    {\n" +
		"      \"rule_id\": \"call-rule\",\n" +
		"      \"scope\": \"next-line\",\n" +
		"      \"path\": \"/project/source.go\",\n" +
		"      \"source_digest\": \"" + digestString(digest) + "\",\n" +
		"      \"range\": {\n" +
		"        \"start\": 8,\n" +
		"        \"end\": 20\n" +
		"      },\n" +
		"      \"target\": {\n" +
		"        \"start\": 21,\n" +
		"        \"end\": 35\n" +
		"      },\n" +
		"      \"reason\": \"legacy call\"\n" +
		"    }\n" +
		"  ],\n" +
		"  \"errors\": []\n" +
		"}\n"
	if string(encoded) != want {
		t.Fatalf("MarshalLintJSON() =\n%s\nwant:\n%s", encoded, want)
	}
}

func TestNewLintResultRejectsMismatchedOrDuplicateSources(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	base := analysis.Result{Path: file.Path(), Digest: file.Digest()}
	mismatch := base
	mismatch.Diagnostics = []rules.Diagnostic{{Path: "/project/other.go", Digest: file.Digest()}}
	if _, err := NewLintResult("check", "findings", 1, true, []analysis.Result{mismatch}, nil); err == nil ||
		!strings.Contains(err.Error(), "diagnostic source identity") {
		t.Fatalf("NewLintResult() mismatch error = %v", err)
	}
	if _, err := NewLintResult("check", "success", 0, true, []analysis.Result{base, base}, nil); err == nil ||
		!strings.Contains(err.Error(), "duplicate source path") {
		t.Fatalf("NewLintResult() duplicate error = %v", err)
	}
	relative := base
	relative.Path = "source.go"
	if _, err := NewLintResult("check", "success", 0, true, []analysis.Result{relative}, nil); err == nil ||
		!strings.Contains(err.Error(), "absolute") {
		t.Fatalf("NewLintResult() relative-path error = %v", err)
	}
	missingDigest := base
	missingDigest.Digest = source.Digest{}
	if _, err := NewLintResult("check", "success", 0, true, []analysis.Result{missingDigest}, nil); err == nil ||
		!strings.Contains(err.Error(), "digest") {
		t.Fatalf("NewLintResult() missing-digest error = %v", err)
	}
}

func TestNewLintResultSortsSourcesAndDiagnostics(t *testing.T) {
	t.Parallel()

	first, err := source.Load("/project/a.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := source.Load("/project/b.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := func(file *source.File, start int, ruleID string) rules.Diagnostic {
		return rules.Diagnostic{
			RuleID:     ruleID,
			Severity:   rules.SeverityWarn,
			MessageKey: "finding",
			Message:    "finding",
			Path:       file.Path(),
			Digest:     file.Digest(),
			Range:      source.Range{Start: start, End: start},
		}
	}
	result, err := NewLintResult("check", "findings", 1, true, []analysis.Result{
		{
			Path:   second.Path(),
			Digest: second.Digest(),
			Diagnostics: []rules.Diagnostic{
				diagnostic(second, 10, "later"),
				diagnostic(second, 1, "earlier"),
			},
		},
		{Path: first.Path(), Digest: first.Digest()},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files[0].Path != first.Path() || result.Files[1].Path != second.Path() {
		t.Fatalf("NewLintResult() file order = %#v", result.Files)
	}
	if result.Diagnostics[0].RuleID != "earlier" || result.Diagnostics[1].RuleID != "later" {
		t.Fatalf("NewLintResult() diagnostic order = %#v", result.Diagnostics)
	}
}

func digestString(digest source.Digest) string {
	const hexadecimal = "0123456789abcdef"
	encoded := make([]byte, len(digest)*2)
	for index, value := range digest {
		encoded[index*2] = hexadecimal[value>>4]
		encoded[index*2+1] = hexadecimal[value&0x0f]
	}
	return string(encoded)
}
