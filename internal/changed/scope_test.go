package changed

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestResolveTracksWorkingTreeLinesRenamesAndUntrackedFiles(t *testing.T) {
	t.Parallel()

	root := initializeRepository(t)
	writeChangedFixture(
		t,
		filepath.Join(root, "source.go"),
		"package sample\n\nfunc run() {\n\tprintln(\"old\")\n\tprintln(\"stable\")\n}\n",
	)
	writeChangedFixture(
		t,
		filepath.Join(root, "old_name.go"),
		"package sample\n\nconst renamed = true\n",
	)
	writeChangedFixture(
		t,
		filepath.Join(root, "old_modified.go"),
		"package sample\n\nconst (\n\tfirst = 1\n\tsecond = 2\n\tthird = 3\n\tmodified = true\n)\n",
	)
	runFixtureGit(t, root, "add", "source.go", "old_name.go", "old_modified.go")
	runFixtureGit(t, root, "commit", "-m", "baseline")
	baseline := strings.TrimSpace(runFixtureGit(t, root, "rev-parse", "HEAD"))

	writeChangedFixture(
		t,
		filepath.Join(root, "source.go"),
		"package sample\n\nfunc run() {\n\tprintln(\"new\")\n\tprintln(\"stable\")\n}\n",
	)
	runFixtureGit(t, root, "mv", "old_name.go", "new_name.go")
	runFixtureGit(t, root, "mv", "old_modified.go", "new_modified.go")
	writeChangedFixture(
		t,
		filepath.Join(root, "new_modified.go"),
		"package sample\n\nconst (\n\tfirst = 1\n\tsecond = 2\n\tthird = 3\n\tmodified = false\n)\n",
	)
	writeChangedFixture(
		t,
		filepath.Join(root, "untracked.go"),
		"package sample\n\nconst untracked = true\n",
	)

	first, err := Resolve(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Resolve(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Root() != canonicalRoot ||
		first.MergeBase() != baseline ||
		second.MergeBase() != baseline {
		t.Fatalf(
			"resolved roots and merge bases = (%q, %q), (%q, %q)",
			first.Root(),
			first.MergeBase(),
			second.Root(),
			second.MergeBase(),
		)
	}
	if got := first.Lines(filepath.Join(root, "source.go"));
		!slices.Equal(got, []LineRange{{Start: 4, End: 4}}) {
		t.Fatalf("modified lines = %#v", got)
	}
	if got := first.Lines(filepath.Join(root, "new_name.go")); len(got) != 0 {
		t.Fatalf("pure rename lines = %#v", got)
	}
	if got := first.Lines(filepath.Join(root, "new_modified.go"));
		!slices.Equal(got, []LineRange{{Start: 7, End: 7}}) {
		t.Fatalf("modified rename lines = %#v", got)
	}
	if !first.WholeFile(filepath.Join(root, "untracked.go")) {
		t.Fatal("untracked file was not owned as a whole file")
	}
	if first.Changed(filepath.Join(root, "old_name.go")) {
		t.Fatal("deleted rename source remained a current changed path")
	}
}

func TestFilterDiagnosticsSeparatesPreexistingFindingsAndOwnsCompleteFixes(t *testing.T) {
	t.Parallel()

	root := initializeRepository(t)
	path := filepath.Join(root, "source.go")
	baseline := "package sample\n\nfunc run() {\n\tprintln(\"old\")\n\tprintln(\"stable\")\n}\n"
	current := "package sample\n\nfunc run() {\n\tprintln(\"new\")\n\tprintln(\"stable\")\n}\n"
	writeChangedFixture(t, path, baseline)
	runFixtureGit(t, root, "add", "source.go")
	runFixtureGit(t, root, "commit", "-m", "baseline")
	writeChangedFixture(t, path, current)

	scope, err := Resolve(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load(path, []byte(current))
	if err != nil {
		t.Fatal(err)
	}
	changedLine := byteRangeForText(t, file, current, "println(\"new\")")
	stableLine := byteRangeForText(t, file, current, "println(\"stable\")")
	diagnostics := []rules.Diagnostic{
		{
			RuleID: "changed",
			Path: path,
			Digest: file.Digest(),
			Range: changedLine,
			Fixes: []rules.Fix{
				{
					Name: "owned",
					Safety: rules.FixSafe,
					Edits: []rules.Edit{
						{Range: changedLine, NewText: "println(\"fixed\")"},
					},
				},
				{
					Name: "partial",
					Safety: rules.FixSafe,
					Edits: []rules.Edit{
						{Range: changedLine},
						{Range: stableLine},
					},
				},
			},
		},
		{RuleID: "preexisting", Path: path, Digest: file.Digest(), Range: stableLine},
	}

	visible, preexisting, err := scope.FilterDiagnostics(file, diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 ||
		visible[0].RuleID != "changed" ||
		len(visible[0].Fixes) != 1 ||
		visible[0].Fixes[0].Name != "owned" {
		t.Fatalf("visible diagnostics = %#v", visible)
	}
	if len(preexisting) != 1 || preexisting[0].RuleID != "preexisting" {
		t.Fatalf("preexisting diagnostics = %#v", preexisting)
	}
}

func TestFilterDiagnosticsRejectsSourceChangedAfterScopeResolution(t *testing.T) {
	t.Parallel()

	root := initializeRepository(t)
	path := filepath.Join(root, "source.go")
	baseline := "package sample\n\nconst value = 1\n"
	writeChangedFixture(t, path, baseline)
	runFixtureGit(t, root, "add", "source.go")
	runFixtureGit(t, root, "commit", "-m", "baseline")
	writeChangedFixture(t, path, "package sample\n\nconst value = 2\n")
	scope, err := Resolve(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	current := "package sample\n\nconst value = 3\n"
	writeChangedFixture(t, path, current)
	file, err := source.Load(path, []byte(current))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = scope.FilterDiagnostics(
		file,
		[]rules.Diagnostic{
			{
				RuleID: "changed",
				Path: path,
				Digest: file.Digest(),
				Range: byteRangeForText(t, file, current, "value = 3"),
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "changed after scope resolution") {
		t.Fatalf("FilterDiagnostics() error = %v", err)
	}
}

func TestResolveRejectsUnknownBase(t *testing.T) {
	t.Parallel()

	root := initializeRepository(t)
	writeChangedFixture(t, filepath.Join(root, "source.go"), "package sample\n")
	runFixtureGit(t, root, "add", "source.go")
	runFixtureGit(t, root, "commit", "-m", "baseline")

	_, err := Resolve(context.Background(), root, "missing-reference")
	if err == nil || !strings.Contains(err.Error(), "missing-reference") {
		t.Fatalf("Resolve() error = %v", err)
	}
	_, err = Resolve(context.Background(), root, "--all")
	if err == nil || !strings.Contains(err.Error(), "must not begin with '-'") {
		t.Fatalf("Resolve(option-like base) error = %v", err)
	}
}

func TestResolveIgnoresInheritedGitRepositoryOverrides(t *testing.T) {
	root := initializeRepository(t)
	writeChangedFixture(t, filepath.Join(root, "source.go"), "package sample\n")
	runFixtureGit(t, root, "add", "source.go")
	runFixtureGit(t, root, "commit", "-m", "baseline")
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing.git"))
	t.Setenv("GIT_WORK_TREE", filepath.Join(t.TempDir(), "wrong-worktree"))

	scope, err := Resolve(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Root() != canonicalRoot {
		t.Fatalf("resolved root = %q, want %q", scope.Root(), canonicalRoot)
	}
}

func TestResolveDoesNotExecuteRepositoryCleanFilters(t *testing.T) {
	t.Parallel()

	root := initializeRepository(t)
	path := filepath.Join(root, "source.go")
	writeChangedFixture(t, path, "package sample\n\nconst value = 1\n")
	runFixtureGit(t, root, "add", "source.go")
	runFixtureGit(t, root, "commit", "-m", "baseline")
	writeChangedFixture(t, filepath.Join(root, ".gitattributes"), "*.go filter=hostile\n")
	runFixtureGit(t, root, "add", ".gitattributes")
	runFixtureGit(t, root, "commit", "-m", "attributes")
	runFixtureGit(t, root, "config", "filter.hostile.clean", "false")
	runFixtureGit(t, root, "config", "filter.hostile.process", "false")
	runFixtureGit(t, root, "config", "filter.hostile.required", "true")
	writeChangedFixture(t, path, "package sample\n\nconst value = 2\n")

	scope, err := Resolve(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got := scope.Lines(path); !slices.Equal(got, []LineRange{{Start: 3, End: 3}}) {
		t.Fatalf("modified lines = %#v", got)
	}
}

func TestOwnsTransformationRequiresEveryChangedSourceLine(t *testing.T) {
	t.Parallel()

	root := initializeRepository(t)
	path := filepath.Join(root, "source.go")
	baseline := "package sample\n\nfunc run() {\n\tprintln(\"old\")\n\tprintln(\"stable\")\n}\n"
	current := "package sample\n\nfunc run() {\n\tprintln(\"new\")\n\tprintln(\"stable\")\n}\n"
	writeChangedFixture(t, path, baseline)
	runFixtureGit(t, root, "add", "source.go")
	runFixtureGit(t, root, "commit", "-m", "baseline")
	writeChangedFixture(t, path, current)
	scope, err := Resolve(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load(path, []byte(current))
	if err != nil {
		t.Fatal(err)
	}

	owned := strings.Replace(current, "println(\"new\")", "println(\"fixed\")", 1)
	valid, err := scope.OwnsTransformation(file, []byte(owned))
	if err != nil || !valid {
		t.Fatalf("owned transformation = %t, %v", valid, err)
	}
	unowned := strings.Replace(owned, "println(\"stable\")", "println(\"rewritten\")", 1)
	valid, err = scope.OwnsTransformation(file, []byte(unowned))
	if err != nil || valid {
		t.Fatalf("partially owned transformation = %t, %v", valid, err)
	}
}

func initializeRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runFixtureGit(t, root, "init", "-b", "main")
	runFixtureGit(t, root, "config", "user.name", "Glippy Test")
	runFixtureGit(t, root, "config", "user.email", "glippy@example.invalid")
	return root
}

func runFixtureGit(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.CommandContext(
		context.Background(),
		"git",
		append([]string{"-C", root}, arguments...)...,
	)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func writeChangedFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func byteRangeForText(t *testing.T, file *source.File, input, text string) source.Range {
	t.Helper()
	start := strings.Index(input, text)
	if start < 0 {
		t.Fatalf("fixture does not contain %q", text)
	}
	result := source.Range{Start: start, End: start + len(text)}
	if _, valid := file.Slice(result); !valid {
		t.Fatalf("invalid fixture range %#v", result)
	}
	return result
}
