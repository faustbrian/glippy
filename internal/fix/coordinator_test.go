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

func TestCoordinateReportsDerivedImportRemovalsInStableOrder(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"strings"
	"fmt"
)

func run(text string) {
	_ = fmt.Sprintf("%s", text)
	_ = strings.TrimSpace(text)
}
`
	file := loadSource(t, input)
	result, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{
			selection(
				file,
				"remove-strings-use",
				"rewrite",
				rules.FixSafe,
				edit(input, `strings.TrimSpace(text)`, "text"),
			),
			selection(
				file,
				"remove-fmt-use",
				"rewrite",
				rules.FixSafe,
				edit(input, `fmt.Sprintf("%s", text)`, "text"),
			),
		},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []fixengine.ImportChange{
		{Action: fixengine.ImportRemove, Path: "fmt", Name: "fmt"},
		{Action: fixengine.ImportRemove, Path: "strings", Name: "strings"},
	}
	if !reflect.DeepEqual(result.ImportChanges, want) ||
		bytes.Contains(result.Bytes, []byte("import")) ||
		len(result.Applied) != 2 ||
		len(result.Rejected) != 0 {
		t.Fatalf("Coordinate() import result = %#v, bytes %q", result, result.Bytes)
	}
}

func TestCoordinateAddsAnExactImportRequiredByAFix(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nfunc run() { _ = 0 }\n"
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-net-constant",
		"replace-zero",
		rules.FixSafe,
		edit(input, "0", "net.IPv4len"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "net", Name: "net"},
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := "package sample\n\nimport \"net\"\n\nfunc run() {\n\t_ = net.IPv4len\n}\n"
	wantChanges := []fixengine.ImportChange{
		{Action: fixengine.ImportAdd, Path: "net", Name: "net"},
	}
	if string(result.Bytes) != want ||
		!reflect.DeepEqual(result.ImportChanges, wantChanges) ||
		len(result.Applied) != 1 ||
		len(result.Rejected) != 0 {
		t.Fatalf("Coordinate() import addition = %#v, bytes %q", result, result.Bytes)
	}
}

func TestCoordinateReusesAnExistingExactRequiredImport(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nimport \"net\"\n\nfunc run() { _ = 0 }\n"
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-net-constant",
		"replace-zero",
		rules.FixSafe,
		edit(input, "0", "net.IPv4len"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "net", Name: "net"},
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ImportChanges) != 0 ||
		len(result.Applied) != 1 ||
		!bytes.Contains(result.Bytes, []byte("net.IPv4len")) {
		t.Fatalf(
			"Coordinate() existing import result = %#v, bytes %q",
			result,
			result.Bytes,
		)
	}
}

func TestCoordinateAppendsRequiredImportsWithoutChangingAGroupedImport(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	// formatting owns strings
	"strings"

	"time" // retained group
)

func run() { _ = 0 }
`
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-net-constant",
		"replace-zero",
		rules.FixSafe,
		edit(input, "0", "network.IPv4len"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "net", Name: "network"},
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	wantImports := "import (\n\t// formatting owns strings\n\t\"strings\"\n\n\t\"time\" // retained group\n\tnetwork \"net\"\n)"
	if !bytes.Contains(result.Bytes, []byte(wantImports)) ||
		len(result.ImportChanges) != 1 ||
		result.ImportChanges[0] !=
			(fixengine.ImportChange{
				Action: fixengine.ImportAdd,
				Path: "net",
				Name: "network",
			}) {
		t.Fatalf("Coordinate() grouped import result = %#v, bytes %q", result, result.Bytes)
	}
}

func TestCoordinateUsesASeparateDeclarationAfterAGroupedImportFooterComment(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"strings"

	// keep this footer with the existing group
)

func run() { _ = strings.TrimSpace(""); _ = 0 }
`
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-net-constant",
		"replace-zero",
		rules.FixSafe,
		edit(input, "_ = 0", "_ = net.IPv4len"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "net", Name: "net"},
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	wantImports := "import (\n\t\"strings\"\n\n\t// keep this footer with the existing group\n)\n\nimport \"net\""
	if len(result.Applied) != 1 ||
		len(result.Rejected) != 0 ||
		len(result.ImportChanges) != 1 ||
		!bytes.Contains(result.Bytes, []byte(wantImports)) {
		t.Fatalf(
			"Coordinate() footer comment import result = %#v, bytes %q",
			result,
			result.Bytes,
		)
	}
}

func TestCoordinateRejectsIncompatibleRequiredImportBindings(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nfunc run() { first(); second() }\n"
	file := loadSource(t, input)
	first := selection(
		file,
		"first-rule",
		"rewrite-first",
		rules.FixSafe,
		edit(input, "first()", "shared.First()"),
	)
	first.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "example.com/first", Name: "shared"},
	}
	second := selection(
		file,
		"second-rule",
		"rewrite-second",
		rules.FixSafe,
		edit(input, "second()", "shared.Second()"),
	)
	second.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "example.com/second", Name: "shared"},
	}
	result, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{first, second},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 0 ||
		len(result.Rejected) != 2 ||
		result.Rejected[0].Reason != fixengine.RejectionConflict ||
		result.Rejected[1].Reason != fixengine.RejectionConflict ||
		!bytes.Equal(result.Bytes, file.Bytes()) {
		t.Fatalf("Coordinate() conflicting import requirements = %#v", result)
	}
}

func TestCoordinateRejectsEveryParticipantInAnImportBindingConflict(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nfunc run() { first(); second(); third() }\n"
	file := loadSource(t, input)
	first := selection(
		file,
		"a-first-rule",
		"rewrite-first",
		rules.FixSafe,
		edit(input, "first()", "shared.First()"),
	)
	first.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "example.com/first", Name: "shared"},
	}
	second := selection(
		file,
		"b-second-rule",
		"rewrite-second",
		rules.FixSafe,
		edit(input, "second()", "shared.Second()"),
	)
	second.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "example.com/second", Name: "shared"},
	}
	third := selection(
		file,
		"c-third-rule",
		"rewrite-third",
		rules.FixSafe,
		edit(input, "third()", "shared.Third()"),
	)
	third.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "example.com/first", Name: "shared"},
	}
	result, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{first, second, third},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 0 ||
		len(result.Rejected) != 3 ||
		!bytes.Equal(result.Bytes, file.Bytes()) {
		t.Fatalf("Coordinate() import conflict participants = %#v", result)
	}
	for _, rejected := range result.Rejected {
		if rejected.Reason != fixengine.RejectionConflict {
			t.Fatalf("Coordinate() rejection = %#v", rejected)
		}
	}
}

func TestCoordinateRejectsARequiredImportThatConflictsWithASourceBinding(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nfunc run() { net := 0; _ = net }\n"
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-net-constant",
		"replace-zero",
		rules.FixSafe,
		edit(input, "_ = net", "_ = net.IPv4len"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "net", Name: "net"},
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 0 ||
		len(result.Rejected) != 1 ||
		result.Rejected[0].Reason != fixengine.RejectionValidation ||
		!strings.Contains(
			result.Rejected[0].Message,
			"resolves to a local source binding",
		) ||
		!bytes.Equal(result.Bytes, file.Bytes()) {
		t.Fatalf("Coordinate() source binding conflict = %#v", result)
	}
}

func TestCoordinateAllowsAnUnrelatedLocalWithARequiredImportName(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nfunc unrelated() { net := 0; _ = net }\n\nfunc run() { _ = 0 }\n"
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-net-constant",
		"replace-zero",
		rules.FixSafe,
		edit(input, "func run() { _ = 0 }", "func run() { _ = net.IPv4len }"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "net", Name: "net"},
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 1 ||
		len(result.Rejected) != 0 ||
		len(result.ImportChanges) != 1 ||
		!bytes.Contains(result.Bytes, []byte("import \"net\"")) ||
		!bytes.Contains(result.Bytes, []byte("net.IPv4len")) {
		t.Fatalf(
			"Coordinate() unrelated local binding = %#v, bytes %q",
			result,
			result.Bytes,
		)
	}
}

func TestCoordinateRejectsARequiredImportWithAnExistingInexactBinding(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nimport network \"net\"\n\nfunc run() { _ = network.IPv4len; _ = 0 }\n"
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-net-constant",
		"replace-zero",
		rules.FixSafe,
		edit(input, "_ = 0", "_ = net.IPv6len"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "net", Name: "net"},
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 0 ||
		len(result.Rejected) != 1 ||
		result.Rejected[0].Reason != fixengine.RejectionValidation ||
		!strings.Contains(result.Rejected[0].Message, "already uses name") ||
		!bytes.Equal(result.Bytes, file.Bytes()) {
		t.Fatalf("Coordinate() existing inexact import = %#v", result)
	}
}

func TestCoordinateRejectsARequiredImportAlreadyPresentAsBlank(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nimport _ \"net/http/pprof\"\n\nfunc run() { _ = 0 }\n"
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-pprof",
		"replace-zero",
		rules.FixSafe,
		edit(input, "0", "pprof.Handler(\"index\")"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "net/http/pprof", Name: "pprof"},
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 0 ||
		len(result.Rejected) != 1 ||
		result.Rejected[0].Reason != fixengine.RejectionValidation ||
		!strings.Contains(result.Rejected[0].Message, "already uses name") ||
		!bytes.Equal(result.Bytes, file.Bytes()) {
		t.Fatalf("Coordinate() blank required import = %#v", result)
	}
}

func TestCoordinateRejectsDuplicateRequiredImports(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nfunc run() { _ = 0 }\n"
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-net-constant",
		"replace-zero",
		rules.FixSafe,
		edit(input, "0", "net.IPv4len"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "net", Name: "net"},
		{Path: "net", Name: "net"},
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 0 ||
		len(result.Rejected) != 1 ||
		result.Rejected[0].Reason != fixengine.RejectionValidation ||
		!strings.Contains(result.Rejected[0].Message, "duplicated") ||
		!bytes.Equal(result.Bytes, file.Bytes()) {
		t.Fatalf("Coordinate() duplicate required imports = %#v", result)
	}
}

func TestCoordinateOrdersMultipleRequiredImportAdditions(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nfunc run() { _ = 0 }\n"
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-imports",
		"replace-zero",
		rules.FixSafe,
		edit(input, "0", "alpha.Value + zeta.Value"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "example.com/zeta", Name: "zeta"},
		{Path: "example.com/alpha", Name: "alpha"},
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, fixOptions())
	if err != nil {
		t.Fatal(err)
	}
	wantChanges := []fixengine.ImportChange{
		{Action: fixengine.ImportAdd, Path: "example.com/alpha", Name: "alpha"},
		{Action: fixengine.ImportAdd, Path: "example.com/zeta", Name: "zeta"},
	}
	if !reflect.DeepEqual(result.ImportChanges, wantChanges) ||
		bytes.Index(result.Bytes, []byte(`"example.com/alpha"`)) >
			bytes.Index(result.Bytes, []byte(`"example.com/zeta"`)) {
		t.Fatalf("Coordinate() ordered imports = %#v, bytes %q", result, result.Bytes)
	}
}

func TestCoordinatePreservesOriginalBytesWhenRequiredImportValidationFails(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nfunc run() { _ = 0 }\n"
	file := loadSource(t, input)
	selected := selection(
		file,
		"use-net-constant",
		"replace-zero",
		rules.FixSafe,
		edit(input, "0", "net.IPv4len"),
	)
	selected.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "net", Name: "net"},
	}
	options := fixOptions()
	options.Validate = func(*source.File) error {
		return errors.New("typed validation failed")
	}
	result, err := fixengine.Coordinate(file, []fixengine.Selection{selected}, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 0 ||
		len(result.ImportChanges) != 0 ||
		len(result.Rejected) != 1 ||
		result.Rejected[0].Reason != fixengine.RejectionValidation ||
		!strings.Contains(result.Rejected[0].Message, "typed validation failed") ||
		!bytes.Equal(result.Bytes, file.Bytes()) {
		t.Fatalf("Coordinate() required import rollback = %#v", result)
	}
}

func TestCoordinateDoesNotPruneAnImportRequiredByAnotherAcceptedFix(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nimport \"fmt\"\n\nfunc run(text string) { _ = fmt.Sprintf(\"%s\", text); _ = 0 }\n"
	file := loadSource(t, input)
	removeUse := selection(
		file,
		"remove-format-use",
		"use-text",
		rules.FixSafe,
		edit(input, "fmt.Sprintf(\"%s\", text)", "text"),
	)
	addUse := selection(
		file,
		"add-format-use",
		"format-number",
		rules.FixSafe,
		edit(input, "_ = 0", "_ = fmt.Sprint(0)"),
	)
	addUse.Diagnostic.Fixes[0].RequiredImports = []rules.ImportRequirement{
		{Path: "fmt", Name: "fmt"},
	}
	result, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{removeUse, addUse},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 2 ||
		len(result.ImportChanges) != 0 ||
		!bytes.Contains(result.Bytes, []byte("import \"fmt\"")) ||
		!bytes.Contains(result.Bytes, []byte("fmt.Sprint(0)")) {
		t.Fatalf(
			"Coordinate() retained required import = %#v, bytes %q",
			result,
			result.Bytes,
		)
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
