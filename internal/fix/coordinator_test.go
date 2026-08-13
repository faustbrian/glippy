package fix_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	fixengine "github.com/faustbrian/glippy/internal/fix"
	glippyformat "github.com/faustbrian/glippy/internal/format"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestCoordinateAppliesSafeNonOverlappingFixesAndFormatsResult(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){first();second()}\n"
	file := loadSource(t, input)
	selections := []fixengine.Selection{
		selection(
			file,
			"rename-first",
			"rename",
			rules.FixSafe,
			edit(input, "first", "primary"),
		),
		selection(
			file,
			"rename-second",
			"rename",
			rules.FixSafe,
			edit(input, "second", "secondary"),
		),
	}
	result, err := fixengine.Coordinate(file, selections, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := "package sample\n\nfunc run() {\n\tprimary()\n\tsecondary()\n}\n"
	if string(result.Bytes) != want {
		t.Fatalf("Coordinate() bytes = %q, want %q", result.Bytes, want)
	}
	if len(result.Applied) != 2 || len(result.Rejected) != 0 {
		t.Fatalf("Coordinate() result = %#v", result)
	}
}

func TestCoordinateRejectsEveryConflictingFixButAppliesIndependentFixes(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file := loadSource(t, input)
	selections := []fixengine.Selection{
		selection(
			file,
			"replace-target-a",
			"replace",
			rules.FixSafe,
			edit(input, "target", "first"),
		),
		selection(
			file,
			"replace-target-b",
			"replace",
			rules.FixSafe,
			edit(input, "target", "second"),
		),
		selection(
			file,
			"rename-function",
			"rename",
			rules.FixSafe,
			edit(input, "run", "execute"),
		),
	}
	result, err := fixengine.Coordinate(file, selections, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := "package sample\n\nfunc execute() {\n\ttarget()\n}\n"
	if string(result.Bytes) != want {
		t.Fatalf("Coordinate() bytes = %q, want %q", result.Bytes, want)
	}
	if len(result.Applied) != 1 || result.Applied[0].RuleID != "rename-function" {
		t.Fatalf("Coordinate() applied = %#v", result.Applied)
	}
	if len(result.Rejected) != 2 {
		t.Fatalf("Coordinate() rejected = %#v", result.Rejected)
	}
	for _, rejected := range result.Rejected {
		if rejected.Reason != fixengine.RejectionConflict {
			t.Fatalf("rejection = %#v, want conflict", rejected)
		}
	}
}

func TestCoordinateRejectsACompleteMultiEditFixWhenOneEditConflicts(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file := loadSource(t, input)
	result, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{
			selection(
				file,
				"multi-edit",
				"rewrite",
				rules.FixSafe,
				edit(input, "target", "primary"),
				edit(input, "run", "execute"),
			),
			selection(
				file,
				"overlap",
				"rewrite",
				rules.FixSafe,
				edit(input, "target", "secondary"),
			),
		},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 0 ||
		len(result.Rejected) != 2 ||
		!bytes.Equal(result.Bytes, file.Bytes()) {
		t.Fatalf("Coordinate() result = %#v", result)
	}
}

func TestCoordinateRejectsStaleUnsafeAndInvalidSelections(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){café()}\n"
	file := loadSource(t, input)
	stale := selection(
		file,
		"stale-rule",
		"rename",
		rules.FixSafe,
		edit(input, "run", "execute"),
	)
	stale.Diagnostic.Digest = source.Digest{}
	unsafe := selection(
		file,
		"unsafe-rule",
		"rename",
		rules.FixUnsafe,
		edit(input, "run", "execute"),
	)
	cafe := strings.Index(input, "café")
	invalid := selection(
		file,
		"invalid-range",
		"rename",
		rules.FixSafe,
		rules.Edit{
			Range: source.Range{Start: cafe + len("caf") + 1, End: cafe + len("café")},
			NewText: "e",
		},
	)

	result, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{stale, unsafe, invalid},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Bytes, file.Bytes()) ||
		len(result.Applied) != 0 ||
		len(result.Rejected) != 3 {
		t.Fatalf("Coordinate() result = %#v", result)
	}
	wantReasons := map[fixengine.RejectionReason]bool{
		fixengine.RejectionStaleSource: false,
		fixengine.RejectionUnsafe: false,
		fixengine.RejectionInvalidRange: false,
	}
	for _, rejected := range result.Rejected {
		if _, found := wantReasons[rejected.Reason]; !found {
			t.Fatalf("unexpected rejection = %#v", rejected)
		}
		wantReasons[rejected.Reason] = true
	}
	for reason, found := range wantReasons {
		if !found {
			t.Fatalf("missing rejection reason %q", reason)
		}
	}
}

func TestCoordinateRollsBackEveryAcceptedFixWhenValidationFails(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file := loadSource(t, input)
	result, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{
			selection(
				file,
				"break-syntax",
				"rewrite",
				rules.FixSafe,
				edit(input, "target()", "("),
			),
		},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Bytes, file.Bytes()) ||
		len(result.Applied) != 0 ||
		len(result.Rejected) != 1 {
		t.Fatalf("Coordinate() result = %#v", result)
	}
	if result.Rejected[0].Reason != fixengine.RejectionValidation {
		t.Fatalf("rejection = %#v, want validation", result.Rejected[0])
	}
}

func TestCoordinateRollsBackFixWhenFormatterWouldChangeSuppressionOwnership(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(ready bool) {\n" +
		"//glippy:ignore duplicate-condition -- legacy branch\n" +
		"if ready { use() } else if ready { retry() }\n}\nfunc use(){}\nfunc retry(){}\n"
	file := loadSource(t, input)
	result, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{
			selection(
				file,
				"rename",
				"rewrite",
				rules.FixSafe,
				edit(input, "use", "primary"),
			),
		},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Bytes, file.Bytes()) ||
		len(result.Applied) != 0 ||
		len(result.Rejected) != 1 ||
		result.Rejected[0].Reason != fixengine.RejectionValidation ||
		!strings.Contains(result.Rejected[0].Message, "suppression ownership changed") {
		t.Fatalf("Coordinate() result = %#v", result)
	}
}

func TestCoordinateRunsPostFormatValidationBeforeAcceptingFixes(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file := loadSource(t, input)
	options := fixOptions()
	var validated []byte
	options.Validate = func(formatted *source.File) error {
		validated = formatted.Bytes()
		return nil
	}

	result, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{
			selection(
				file,
				"rename",
				"rewrite",
				rules.FixSafe,
				edit(input, "target", "primary"),
			),
		},
		options,
	)

	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 1 || !bytes.Equal(validated, result.Bytes) {
		t.Fatalf("Coordinate() result = %#v, validated = %q", result, validated)
	}
}

func TestCoordinateRollsBackFixesRejectedByPostFormatValidation(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file := loadSource(t, input)
	options := fixOptions()
	options.Validate = func(*source.File) error {
		return errors.New("analysis failed")
	}

	result, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{
			selection(
				file,
				"rename",
				"rewrite",
				rules.FixSafe,
				edit(input, "target", "primary"),
			),
		},
		options,
	)

	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Bytes, file.Bytes()) ||
		len(result.Applied) != 0 ||
		len(result.Rejected) != 1 ||
		result.Rejected[0].Reason != fixengine.RejectionValidation ||
		!strings.Contains(result.Rejected[0].Message, "analysis failed") {
		t.Fatalf("Coordinate() result = %#v", result)
	}
}

func TestCoordinateAllowsBoundaryInsertionsButRejectsSameOffsetInsertions(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file := loadSource(t, input)
	target := strings.Index(input, "target")
	boundary, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{
			selection(
				file,
				"replace",
				"rewrite",
				rules.FixSafe,
				rules.Edit{
					Range: source.Range{
						Start: target,
						End: target + len("target"),
					},
					NewText: "primary",
				},
			),
			selection(
				file,
				"suffix",
				"insert",
				rules.FixSafe,
				rules.Edit{
					Range: source.Range{
						Start: target + len("target"),
						End: target + len("target"),
					},
					NewText: "Suffix",
				},
			),
		},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(boundary.Rejected) != 0 ||
		!strings.Contains(string(boundary.Bytes), "primarySuffix()") {
		t.Fatalf("boundary insert result = %#v", boundary)
	}

	conflicting, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{
			selection(
				file,
				"insert-a",
				"insert",
				rules.FixSafe,
				rules.Edit{
					Range: source.Range{Start: target, End: target},
					NewText: "first",
				},
			),
			selection(
				file,
				"insert-b",
				"insert",
				rules.FixSafe,
				rules.Edit{
					Range: source.Range{Start: target, End: target},
					NewText: "second",
				},
			),
		},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicting.Applied) != 0 ||
		len(conflicting.Rejected) != 2 ||
		!bytes.Equal(conflicting.Bytes, file.Bytes()) {
		t.Fatalf("same-offset insertion result = %#v", conflicting)
	}
}

func TestCoordinateRequiresExplicitSuggestionAuthorization(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	file := loadSource(t, input)
	selected := selection(
		file,
		"suggestion",
		"rename",
		rules.FixSuggestion,
		edit(input, "target", "primary"),
	)
	rejected, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected.Rejected) != 1 ||
		rejected.Rejected[0].Reason != fixengine.RejectionSuggestion {
		t.Fatalf("default suggestion result = %#v", rejected)
	}
	options := fixOptions()
	options.AllowSuggestion = true
	applied, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Applied) != 1 || !strings.Contains(string(applied.Bytes), "primary()") {
		t.Fatalf("authorized suggestion result = %#v", applied)
	}
}

func TestCoordinateRefusesAnInvalidInputFile(t *testing.T) {
	t.Parallel()

	file, loadErr := source.Load("invalid.go", []byte("package sample\nfunc run(\n"))
	if loadErr == nil || file == nil {
		t.Fatalf("source.Load() = %#v, %v", file, loadErr)
	}
	if _, err := fixengine.Coordinate(file, nil, fixOptions()); err == nil {
		t.Fatal("Coordinate() error = nil, want invalid-source refusal")
	}
}

func TestCoordinateIsIndependentOfSelectionOrder(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){first();second()}\n"
	file := loadSource(t, input)
	first := selection(file, "z-rule", "rename", rules.FixSafe, edit(input, "first", "primary"))
	second := selection(
		file,
		"a-rule",
		"rename",
		rules.FixSafe,
		edit(input, "second", "secondary"),
	)
	forward, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{first, second},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{second, first},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(forward, reverse) {
		t.Fatalf(
			"selection order changed result:\nforward: %#v\nreverse: %#v",
			forward,
			reverse,
		)
	}
}

func FuzzCoordinate(f *testing.F) {
	f.Add(26, 29, "primary", 34, 40, "secondary")
	f.Add(26, 32, "(", 26, 26, "prefix")
	f.Add(28, 28, "first", 28, 28, "second")
	f.Fuzz(
		func(
			t *testing.T,
			firstStart, firstEnd int,
			firstText string,
			secondStart, secondEnd int,
			secondText string,
		) {
			input := "package sample\nfunc run(){first();second()}\n"
			file := loadSource(t, input)
			selections := []fixengine.Selection{
				selection(
					file,
					"first-rule",
					"rewrite",
					rules.FixSafe,
					rules.Edit{
						Range: source.Range{
							Start: firstStart,
							End: firstEnd,
						},
						NewText: firstText,
					},
				),
				selection(
					file,
					"second-rule",
					"rewrite",
					rules.FixSafe,
					rules.Edit{
						Range: source.Range{
							Start: secondStart,
							End: secondEnd,
						},
						NewText: secondText,
					},
				),
			}
			first, err := fixengine.Coordinate(file, selections, fixOptions())
			if err != nil {
				t.Fatal(err)
			}
			second, err := fixengine.Coordinate(file, selections, fixOptions())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("Coordinate() is nondeterministic")
			}
			if _, err := source.Load("result.go", first.Bytes); err != nil {
				t.Fatalf("Coordinate() returned invalid Go: %v", err)
			}
		},
	)
}

func loadSource(t *testing.T, input string) *source.File {
	t.Helper()
	file, err := source.Load("sample.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func selection(
	file *source.File,
	ruleID string,
	fixName string,
	safety rules.FixSafety,
	edits ...rules.Edit,
) fixengine.Selection {
	return fixengine.Selection{
		Diagnostic: rules.Diagnostic{
			RuleID: ruleID,
			Path: file.Path(),
			Digest: file.Digest(),
			Range: edits[0].Range,
			Fixes: []rules.Fix{{Name: fixName, Safety: safety, Edits: edits}},
		},
		FixName: fixName,
	}
}

func edit(input, old, replacement string) rules.Edit {
	start := strings.Index(input, old)
	return rules.Edit{
		Range: source.Range{Start: start, End: start + len(old)},
		NewText: replacement,
	}
}

func fixOptions() fixengine.Options {
	return fixengine.Options{
		Format: glippyformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	}
}
