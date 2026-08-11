package suppressions_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
	"github.com/faustbrian/gox/internal/suppressions"
)

func TestParseAssignsPhysicalSuppressionScopes(t *testing.T) {
	t.Parallel()

	input := `//gox:ignore-file file-rule -- generated adapter
package sample

func run() {
	lineTarget() //gox:ignore-line line-rule -- platform branch
	//gox:ignore next-rule -- legacy call
	nextTarget()
	outsideNext()
	//gox:ignore-start range-rule -- expires=2026-09-01 paired setup
	rangeTarget()
	//gox:ignore-end range-rule
	outsideRange()
}
`
	file := loadFile(t, input)
	index, problems := suppressions.Parse(file, suppressions.ParseOptions{
		KnownRules: []string{"file-rule", "line-rule", "next-rule", "range-rule"},
	})
	if len(problems) != 0 {
		t.Fatalf("Parse() problems = %#v", problems)
	}
	directives := index.Directives()
	if len(directives) != 4 {
		t.Fatalf("Directives() length = %d, want 4", len(directives))
	}
	directives[0].RuleID = "mutated"
	if index.Directives()[0].RuleID == "mutated" {
		t.Fatal("Directives() exposed mutable index state")
	}
	if directives := index.Directives(); directives[3].ExpiresOn != "2026-09-01" ||
		directives[3].Reason != "paired setup" {
		t.Fatalf("range expiry metadata = %#v", directives[3])
	}

	tests := []struct {
		rule       string
		target     string
		suppressed bool
		scope      suppressions.Scope
	}{
		{rule: "file-rule", target: "outsideRange()", suppressed: true, scope: suppressions.ScopeFile},
		{rule: "line-rule", target: "lineTarget()", suppressed: true, scope: suppressions.ScopeLine},
		{rule: "line-rule", target: "nextTarget()", suppressed: false},
		{rule: "next-rule", target: "nextTarget()", suppressed: true, scope: suppressions.ScopeNextLine},
		{rule: "next-rule", target: "outsideNext()", suppressed: false},
		{rule: "range-rule", target: "rangeTarget()", suppressed: true, scope: suppressions.ScopeRange},
		{rule: "range-rule", target: "outsideRange()", suppressed: false},
	}
	for _, test := range tests {
		t.Run(test.rule+"/"+test.target, func(t *testing.T) {
			t.Parallel()
			match, found := index.Match(diagnostic(file, test.rule, rangeOf(input, test.target)))
			if found != test.suppressed {
				t.Fatalf("Match() found = %v, want %v", found, test.suppressed)
			}
			if found && match.Scope != test.scope {
				t.Fatalf("Match() scope = %v, want %v", match.Scope, test.scope)
			}
		})
	}
}

func TestParseReportsMalformedAndUnknownDirectivesInSourceOrder(t *testing.T) {
	t.Parallel()

	input := `//gox:ignore known-rule
//gox:ignore
//gox:ignore unknown-rule -- reason
//gox:ignore known-rule --
package sample

//gox:ignore-file known-rule -- too late
func run() {}
`
	file := loadFile(t, input)
	_, problems := suppressions.Parse(file, suppressions.ParseOptions{
		KnownRules:    []string{"known-rule"},
		RequireReason: true,
	})
	want := []suppressions.ProblemKind{
		suppressions.ProblemMissingReason,
		suppressions.ProblemMalformed,
		suppressions.ProblemUnknownRule,
		suppressions.ProblemMissingReason,
		suppressions.ProblemMisplacedFileScope,
	}
	got := make([]suppressions.ProblemKind, len(problems))
	for index, problem := range problems {
		got[index] = problem.Kind
		if problem.Range.Start < 0 || problem.Range.End <= problem.Range.Start {
			t.Fatalf("problem %d range = %#v", index, problem.Range)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() problem kinds = %#v, want %#v", got, want)
	}
}

func TestParseRequiresDeterministicRangePairs(t *testing.T) {
	t.Parallel()

	input := `package sample

//gox:ignore-end unmatched-rule
//gox:ignore-start nested-rule -- first
//gox:ignore-start nested-rule -- duplicate
func run() {}
`
	file := loadFile(t, input)
	index, problems := suppressions.Parse(file, suppressions.ParseOptions{
		KnownRules: []string{"unmatched-rule", "nested-rule"},
	})
	want := []suppressions.ProblemKind{
		suppressions.ProblemUnmatchedRangeEnd,
		suppressions.ProblemUnclosedRange,
		suppressions.ProblemNestedRange,
	}
	got := make([]suppressions.ProblemKind, len(problems))
	for position, problem := range problems {
		got[position] = problem.Kind
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() problem kinds = %#v, want %#v", got, want)
	}
	if _, found := index.Match(diagnostic(file, "nested-rule", rangeOf(input, "func run"))); found {
		t.Fatal("an unclosed suppression range must not suppress diagnostics")
	}
}

func TestParseRejectsDuplicateKnownRuleConfiguration(t *testing.T) {
	t.Parallel()

	file := loadFile(t, "package sample\n")
	_, problems := suppressions.Parse(file, suppressions.ParseOptions{
		KnownRules: []string{"known-rule", "known-rule"},
	})
	if len(problems) != 1 || problems[0].Kind != suppressions.ProblemInvalidConfiguration {
		t.Fatalf("Parse() problems = %#v", problems)
	}
	_, problems = suppressions.Parse(file, suppressions.ParseOptions{
		KnownRules:   []string{"known-rule"},
		ExpiryCutoff: "2026-02-30",
	})
	if len(problems) != 1 || problems[0].Kind != suppressions.ProblemInvalidConfiguration {
		t.Fatalf("Parse() invalid cutoff problems = %#v", problems)
	}
}

func TestParseAcceptsTabsAndCRLFAtDirectiveBoundaries(t *testing.T) {
	t.Parallel()

	input := "package sample\r\n\r\nfunc run() {\r\n\t//gox:ignore\tnext-rule\t--\tlegacy call\r\n\tnextTarget()\r\n}\r\n"
	file := loadFile(t, input)
	index, problems := suppressions.Parse(file, suppressions.ParseOptions{
		KnownRules:    []string{"next-rule"},
		RequireReason: true,
	})
	if len(problems) != 0 {
		t.Fatalf("Parse() problems = %#v", problems)
	}
	match, found := index.Match(diagnostic(file, "next-rule", rangeOf(input, "nextTarget()")))
	if !found || match.Reason != "legacy call" || match.Scope != suppressions.ScopeNextLine {
		t.Fatalf("Match() = %#v, %v", match, found)
	}
}

func TestParseDiagnosesSuppressionsExpiredAtExplicitCutoff(t *testing.T) {
	t.Parallel()

	input := `package sample

func run() {
	//gox:ignore active-rule -- expires=2026-08-12 temporary compatibility
	activeTarget()
	//gox:ignore expired-rule -- expires=2026-08-11 obsolete compatibility
	expiredTarget()
	//gox:ignore invalid-rule -- expires=2026-02-30 invalid date
	invalidTarget()
}
`
	file := loadFile(t, input)
	index, problems := suppressions.Parse(file, suppressions.ParseOptions{
		KnownRules:   []string{"active-rule", "expired-rule", "invalid-rule"},
		ExpiryCutoff: "2026-08-11",
	})
	want := []suppressions.ProblemKind{
		suppressions.ProblemExpired,
		suppressions.ProblemInvalidExpiry,
	}
	got := make([]suppressions.ProblemKind, len(problems))
	for position, problem := range problems {
		got[position] = problem.Kind
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() problem kinds = %#v, want %#v", got, want)
	}
	active, found := index.Match(diagnostic(file, "active-rule", rangeOf(input, "activeTarget()")))
	if !found || active.ExpiresOn != "2026-08-12" || active.Reason != "temporary compatibility" {
		t.Fatalf("Match(active) = %#v, %v", active, found)
	}
	if _, found := index.Match(diagnostic(file, "expired-rule", rangeOf(input, "expiredTarget()"))); found {
		t.Fatal("expired suppression hid its diagnostic")
	}
	if _, found := index.Match(diagnostic(file, "invalid-rule", rangeOf(input, "invalidTarget()"))); found {
		t.Fatal("invalid suppression expiry hid its diagnostic")
	}
}

func TestParseRetainsExpiryWithoutCutoffAndRequiresHumanReason(t *testing.T) {
	t.Parallel()

	input := `package sample

func run() {
	//gox:ignore active-rule -- expires=2026-08-12 temporary compatibility
	activeTarget()
	//gox:ignore missing-rule -- expires=2026-08-12
	missingTarget()
}
`
	file := loadFile(t, input)
	index, problems := suppressions.Parse(file, suppressions.ParseOptions{
		KnownRules: []string{"active-rule", "missing-rule"},
	})
	if len(problems) != 1 || problems[0].Kind != suppressions.ProblemMissingReason {
		t.Fatalf("Parse() problems = %#v", problems)
	}
	active, found := index.Match(diagnostic(file, "active-rule", rangeOf(input, "activeTarget()")))
	if !found || active.ExpiresOn != "2026-08-12" || active.Reason != "temporary compatibility" {
		t.Fatalf("Match(active) = %#v, %v", active, found)
	}
	if _, found := index.Match(diagnostic(file, "missing-rule", rangeOf(input, "missingTarget()"))); found {
		t.Fatal("expiry metadata without a human reason hid its diagnostic")
	}
}

func TestIndexDoesNotMatchAnotherSourceVersion(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run() {\n\ttarget() //gox:ignore-line known-rule\n}\n"
	file := loadFile(t, input)
	index, problems := suppressions.Parse(file, suppressions.ParseOptions{
		KnownRules: []string{"known-rule"},
	})
	if len(problems) != 0 {
		t.Fatalf("Parse() problems = %#v", problems)
	}
	other, err := source.Load("other.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, found := index.Match(diagnostic(other, "known-rule", rangeOf(string(other.Bytes()), "target()"))); found {
		t.Fatal("Match() accepted a range from another source identity")
	}
	stale, err := source.Load("sample.go", []byte(strings.Replace(input, "target", "targot", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, found := index.Match(diagnostic(stale, "known-rule", rangeOf(string(stale.Bytes()), "targot()"))); found {
		t.Fatal("Match() accepted a range from another source version")
	}
	invalid := diagnostic(file, "known-rule", source.Range{Start: 0, End: len(input) + 1})
	if _, found := index.Match(invalid); found {
		t.Fatal("Match() accepted an out-of-bounds diagnostic range")
	}
	if _, found := index.Match(diagnostic(file, "known-rule", rangeOf(input, "target()"))); !found {
		t.Fatal("Match() rejected its own source identity")
	}
}

func TestApplyFiltersDiagnosticsAndReportsUnusedDirectives(t *testing.T) {
	t.Parallel()

	input := `//gox:ignore-file used-rule -- broad waiver
//gox:ignore-file used-rule -- redundant waiver
//gox:ignore-file unused-rule -- no finding
package sample

func run() { target() }
`
	file := loadFile(t, input)
	index, problems := suppressions.Parse(file, suppressions.ParseOptions{
		KnownRules: []string{"used-rule", "unused-rule", "visible-rule"},
	})
	if len(problems) != 0 {
		t.Fatalf("Parse() problems = %#v", problems)
	}
	target := rangeOf(input, "target()")
	other, err := source.Load("other.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	application := index.Apply([]rules.Diagnostic{
		diagnostic(file, "used-rule", target),
		diagnostic(file, "visible-rule", target),
		diagnostic(other, "used-rule", target),
	})
	if len(application.Suppressed) != 1 ||
		application.Suppressed[0].Directive.Reason != "broad waiver" {
		t.Fatalf("Apply() suppressed = %#v", application.Suppressed)
	}
	if len(application.Diagnostics) != 2 ||
		application.Diagnostics[0].RuleID != "visible-rule" ||
		application.Diagnostics[1].Path != "other.go" {
		t.Fatalf("Apply() diagnostics = %#v", application.Diagnostics)
	}
	if len(application.Unused) != 2 {
		t.Fatalf("Apply() unused = %#v", application.Unused)
	}
	if got := []string{application.Unused[0].Reason, application.Unused[1].Reason}; !reflect.DeepEqual(got, []string{"redundant waiver", "no finding"}) {
		t.Fatalf("Apply() unused reasons = %#v", got)
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte("package sample\n//gox:ignore known-rule -- reason\nfunc run() {}\n"))
	f.Add([]byte("package sample\n//gox:ignore known-rule -- expires=2026-08-11 temporary\nfunc run() {}\n"))
	f.Add([]byte("package sample\n//gox:ignore known-rule -- expires=2026-02-30 invalid\nfunc run() {}\n"))
	f.Add([]byte("//gox:ignore-file known-rule\r\npackage sample\r\n"))
	f.Add([]byte("package sample\n//gox:ignore-start known-rule\n//gox:ignore-end known-rule\n"))
	f.Fuzz(func(t *testing.T, input []byte) {
		file, _ := source.Load("fuzz.go", input)
		if file == nil {
			return
		}
		options := suppressions.ParseOptions{
			KnownRules:   []string{"known-rule"},
			ExpiryCutoff: "2026-08-11",
		}
		first, firstProblems := suppressions.Parse(file, options)
		second, secondProblems := suppressions.Parse(file, options)
		if !reflect.DeepEqual(first.Directives(), second.Directives()) ||
			!reflect.DeepEqual(firstProblems, secondProblems) {
			t.Fatal("Parse() is nondeterministic")
		}
		for _, directive := range first.Directives() {
			if !validRange(directive.Range, len(input)) || !validRange(directive.Target, len(input)) {
				t.Fatalf("directive has invalid physical ranges: %#v", directive)
			}
		}
		for _, problem := range firstProblems {
			if !validRange(problem.Range, len(input)) {
				t.Fatalf("problem has invalid physical range: %#v", problem)
			}
		}
	})
}

func loadFile(t *testing.T, input string) *source.File {
	t.Helper()
	file, err := source.Load("sample.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func rangeOf(input, target string) source.Range {
	start := strings.Index(input, target)
	return source.Range{Start: start, End: start + len(target)}
}

func diagnostic(file *source.File, ruleID string, sourceRange source.Range) rules.Diagnostic {
	return rules.Diagnostic{
		RuleID: ruleID,
		Path:   file.Path(),
		Digest: file.Digest(),
		Range:  sourceRange,
	}
}

func validRange(candidate source.Range, size int) bool {
	return candidate.Start >= 0 && candidate.End >= candidate.Start && candidate.End <= size
}
