package analysis_test

import (
	"context"
	"go/ast"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/rules"
)

func TestRunPackagesCombinesSyntaxAndTypesBeforeSuppressing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	path := filepath.Join(root, "project.go")
	writeTypesFixture(t, path, `package project

//gox:ignore typed-call -- reviewed here
func suppressed() { target() }
func visible() { target() }
func target() {}
`)

	syntax := syntaxRule{
		metadata: analysisMetadata("syntax-call", rules.NodeCallExpr, false),
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			range_, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{MessageKey: "syntax-call", Message: "syntax call", Range: range_}}, nil
		},
	}
	typed := typesRule{
		metadata: typesMetadata("typed-call", rules.NodeCallExpr),
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			call := node.(*ast.CallExpr)
			if ctx.Info().TypeOf(call) == nil {
				t.Fatal("typed runner did not share type information")
			}
			range_, err := ctx.Range(call)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{MessageKey: "typed-call", Message: "typed call", Range: range_}}, nil
		},
	}
	registry, err := rules.NewRegistry(typed, syntax)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantSelection := []rules.Selection{
		{ID: "syntax-call", Severity: rules.SeverityWarn, Requirement: rules.RequireSyntax},
		{ID: "typed-call", Severity: rules.SeverityWarn, Requirement: rules.RequireTypes},
	}
	if result.Requirement != rules.RequireTypes || !reflect.DeepEqual(result.Selection, wantSelection) {
		t.Fatalf("RunPackages() plan = %s, %#v", result.Requirement, result.Selection)
	}
	if len(result.LoadDiagnostics) != 0 || len(result.Files) != 1 {
		t.Fatalf("RunPackages() = %#v", result)
	}
	captured, found := result.Sources.Lookup(path)
	if !found || captured.Digest() != result.Files[0].Digest {
		t.Fatalf("RunPackages() sources = %#v, %t", captured, found)
	}
	fileResult := result.Files[0]
	if fileResult.Path != path || fileResult.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(fileResult.Selection, wantSelection) {
		t.Fatalf("RunPackages() file = %#v", fileResult)
	}
	if len(fileResult.Diagnostics) != 3 ||
		fileResult.Diagnostics[0].RuleID != "syntax-call" ||
		fileResult.Diagnostics[1].RuleID != "syntax-call" ||
		fileResult.Diagnostics[2].RuleID != "typed-call" {
		t.Fatalf("RunPackages() diagnostics = %#v", fileResult.Diagnostics)
	}
	if len(fileResult.Suppressed) != 1 ||
		fileResult.Suppressed[0].Diagnostic.RuleID != "typed-call" ||
		len(fileResult.UnusedSuppressions) != 0 || len(fileResult.SuppressionProblems) != 0 {
		t.Fatalf("RunPackages() suppressions = %#v", fileResult)
	}
}

func TestRunPackagesRequiresTypesBeforeLoading(t *testing.T) {
	t.Parallel()

	syntax := syntaxRule{
		metadata: analysisMetadata("syntax-only", rules.NodeCallExpr, false),
		run:      func(*rules.Context, ast.Node) ([]rules.Finding, error) { return nil, nil },
	}
	registry, err := rules.NewRegistry(syntax)
	if err != nil {
		t.Fatal(err)
	}

	_, err = analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "requires at least one types-tier rule") {
		t.Fatalf("RunPackages() error = %v", err)
	}
}

func TestRunPackagesRetainsLoadErrorsAndValidPartialResults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	validPath := filepath.Join(root, "a_valid.go")
	writeTypesFixture(t, validPath, "package project\nfunc run() { target() }\nfunc target() {}\n")
	writeTypesFixture(t, filepath.Join(root, "z_invalid.go"), "package project\nfunc broken( {\n")

	metadata := typesMetadata("partial-types", rules.NodeCallExpr)
	metadata.RunDespiteTypeErrors = true
	typed := typesRule{
		metadata: metadata,
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			range_, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{MessageKey: "partial", Message: "partial", Range: range_}}, nil
		},
	}
	registry, err := rules.NewRegistry(typed)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.LoadDiagnostics) == 0 || len(result.SourceProblems) != 1 ||
		result.SourceProblems[0].Path != filepath.Join(root, "z_invalid.go") || len(result.Files) != 1 ||
		result.Files[0].Path != validPath || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("RunPackages() partial result = %#v", result)
	}
}

func TestRunPackagesRejectsUnimplementedExpensiveTiersBeforeLoading(t *testing.T) {
	t.Parallel()

	metadata := analysisMetadata("cfg-rule", rules.NodeFuncDecl, false)
	metadata.Requirement = rules.RequireControlFlow
	registry, err := rules.NewRegistry(metadataRuleAdapter{metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}

	_, err = analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "control flow rules are not implemented") {
		t.Fatalf("RunPackages() error = %v", err)
	}
}

func TestRunPackagesRejectsUnimplementedCheapTiersBeforeLoading(t *testing.T) {
	t.Parallel()

	lexicalMetadata := analysisMetadata("lexical-rule", rules.NodeFile, false)
	lexicalMetadata.Requirement = rules.RequireLexical
	typedMetadata := typesMetadata("typed-rule", rules.NodeCallExpr)
	registry, err := rules.NewRegistry(
		metadataRuleAdapter{metadata: lexicalMetadata},
		typesRule{metadata: typedMetadata, run: func(*rules.TypesContext, ast.Node) ([]rules.Finding, error) {
			return nil, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), `selected rule "lexical-rule"`) ||
		!strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("RunPackages() error = %v", err)
	}
}
