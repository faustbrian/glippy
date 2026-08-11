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
	writeTypesFixture(t, path, `//gox:ignore-file cfg-function -- reviewed file
//gox:ignore-file ssa-function -- reviewed file
package project

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
	controlFlow := controlFlowRule{
		metadata: controlFlowMetadata("cfg-function"),
		run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
			range_, err := ctx.Range(ctx.Function())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{MessageKey: "cfg-function", Message: "cfg function", Range: range_}}, nil
		},
	}
	ssaRule := ssaRule{
		metadata: ssaMetadata("ssa-function"),
		run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
			range_, err := ctx.Range(ctx.Syntax())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{MessageKey: "ssa-function", Message: "SSA function", Range: range_}}, nil
		},
	}
	registry, err := rules.NewRegistry(typed, syntax, controlFlow, ssaRule)
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
		{ID: "cfg-function", Severity: rules.SeverityWarn, Requirement: rules.RequireControlFlow},
		{ID: "ssa-function", Severity: rules.SeverityWarn, Requirement: rules.RequireSSA},
		{ID: "syntax-call", Severity: rules.SeverityWarn, Requirement: rules.RequireSyntax},
		{ID: "typed-call", Severity: rules.SeverityWarn, Requirement: rules.RequireTypes},
	}
	if result.Requirement != rules.RequireSSA || !reflect.DeepEqual(result.Selection, wantSelection) {
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
	if fileResult.Path != path || fileResult.Requirement != rules.RequireSSA ||
		!reflect.DeepEqual(fileResult.Selection, wantSelection) {
		t.Fatalf("RunPackages() file = %#v", fileResult)
	}
	if len(fileResult.Diagnostics) != 3 ||
		fileResult.Diagnostics[0].RuleID != "syntax-call" ||
		fileResult.Diagnostics[1].RuleID != "syntax-call" ||
		fileResult.Diagnostics[2].RuleID != "typed-call" {
		t.Fatalf("RunPackages() diagnostics = %#v", fileResult.Diagnostics)
	}
	if len(fileResult.Suppressed) != 7 ||
		fileResult.Suppressed[0].Diagnostic.RuleID != "cfg-function" ||
		fileResult.Suppressed[1].Diagnostic.RuleID != "ssa-function" ||
		fileResult.Suppressed[2].Diagnostic.RuleID != "typed-call" ||
		fileResult.Suppressed[3].Diagnostic.RuleID != "cfg-function" ||
		fileResult.Suppressed[4].Diagnostic.RuleID != "ssa-function" ||
		fileResult.Suppressed[5].Diagnostic.RuleID != "cfg-function" ||
		fileResult.Suppressed[6].Diagnostic.RuleID != "ssa-function" ||
		len(fileResult.UnusedSuppressions) != 0 || len(fileResult.SuppressionProblems) != 0 {
		t.Fatalf("RunPackages() suppressions = %#v", fileResult)
	}
}

func TestRunPackagesRoutesTypedOptionsAcrossEveryTier(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/options\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "options.go"), "package options\nfunc run(){target()}\nfunc target(){}\n")
	optionMetadata := []rules.OptionMetadata{{
		Name: "enabled", Summary: "enable the rule", Kind: rules.OptionBoolean, Required: true,
	}}
	assertEnabled := func(name string, enabled bool, found bool) {
		t.Helper()
		if !found || !enabled {
			t.Fatalf("%s option enabled = %t, %t", name, enabled, found)
		}
	}
	visits := make(map[string]int)
	syntaxMetadata := analysisMetadata("syntax-options", rules.NodeCallExpr, false)
	syntaxMetadata.Options = optionMetadata
	typedMetadata := typesMetadata("types-options", rules.NodeCallExpr)
	typedMetadata.Options = optionMetadata
	cfgMetadata := controlFlowMetadata("cfg-options")
	cfgMetadata.Options = optionMetadata
	ssaMetadata := ssaMetadata("ssa-options")
	ssaMetadata.Options = optionMetadata
	registry, err := rules.NewRegistry(
		syntaxRule{metadata: syntaxMetadata, run: func(ctx *rules.Context, _ ast.Node) ([]rules.Finding, error) {
			enabled, found := ctx.BooleanOption("enabled")
			assertEnabled("syntax", enabled, found)
			visits["syntax"]++
			return nil, nil
		}},
		typesRule{metadata: typedMetadata, run: func(ctx *rules.TypesContext, _ ast.Node) ([]rules.Finding, error) {
			enabled, found := ctx.BooleanOption("enabled")
			assertEnabled("types", enabled, found)
			visits["types"]++
			return nil, nil
		}},
		controlFlowRule{metadata: cfgMetadata, run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
			enabled, found := ctx.BooleanOption("enabled")
			assertEnabled("CFG", enabled, found)
			visits["cfg"]++
			return nil, nil
		}},
		ssaRule{metadata: ssaMetadata, run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
			enabled, found := ctx.BooleanOption("enabled")
			assertEnabled("SSA", enabled, found)
			visits["ssa"]++
			return nil, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	options := make(map[string]rules.OptionSet)
	for _, id := range []string{"syntax-options", "types-options", "cfg-options", "ssa-options"} {
		options[id] = rules.NewOptionSet(map[string]rules.OptionValue{
			"enabled": rules.BooleanOption(true),
		})
	}
	if _, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness, RuleOptions: options},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	); err != nil {
		t.Fatal(err)
	}
	for _, tier := range []string{"syntax", "types", "cfg", "ssa"} {
		if visits[tier] == 0 {
			t.Fatalf("%s rule was not visited", tier)
		}
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
	if err == nil || !strings.Contains(err.Error(), "requires at least one types-tier, CFG-tier, or SSA-tier rule") {
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

func TestRunPackagesRunsControlFlowRulesBeforeSuppressing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	path := filepath.Join(root, "project.go")
	writeTypesFixture(t, path, "//gox:ignore-file cfg-rule -- reviewed file\npackage project\nfunc run() {}\n")
	rule := controlFlowRule{
		metadata: controlFlowMetadata("cfg-rule"),
		run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
			range_, err := ctx.Range(ctx.Function())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{MessageKey: "cfg", Message: "cfg", Range: range_}}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
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
	if result.Requirement != rules.RequireControlFlow || len(result.Files) != 1 ||
		result.Files[0].Path != path || len(result.Files[0].Diagnostics) != 0 ||
		len(result.Files[0].Suppressed) != 1 ||
		result.Files[0].Suppressed[0].Diagnostic.RuleID != "cfg-rule" {
		t.Fatalf("RunPackages() CFG result = %#v", result)
	}
}

func TestRunPackagesRunsPackageWideRulesBeforeSuppressing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	path := filepath.Join(root, "project.go")
	writeTypesFixture(
		t,
		path,
		"//gox:ignore-file package-rule -- reviewed package\npackage project\nfunc run() {}\n",
	)
	rule := packageRule{
		metadata: packageMetadata("package-rule"),
		run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
			for _, file := range ctx.Files() {
				if !file.Target() {
					continue
				}
				range_, err := file.Range(file.Syntax().Name)
				if err != nil {
					return nil, err
				}
				return []rules.PackageFinding{{
					File: file,
					Finding: rules.Finding{
						MessageKey: "package", Message: "package", Range: range_,
					},
				}}, nil
			}
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
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
	if result.Requirement != rules.RequireTypes || len(result.Files) != 1 ||
		result.Files[0].Path != path || len(result.Files[0].Diagnostics) != 0 ||
		len(result.Files[0].Suppressed) != 1 ||
		result.Files[0].Suppressed[0].Diagnostic.RuleID != "package-rule" {
		t.Fatalf("RunPackages() package-wide result = %#v", result)
	}
}

func TestRunPackagesRejectsExpiredSuppressions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	path := filepath.Join(root, "project.go")
	writeTypesFixture(
		t,
		path,
		"//gox:ignore-file cfg-rule -- expires=2026-08-11 temporary waiver\npackage project\nfunc run() {}\n",
	)
	rule := controlFlowRule{
		metadata: controlFlowMetadata("cfg-rule"),
		run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
			range_, err := ctx.Range(ctx.Function())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{MessageKey: "cfg", Message: "cfg", Range: range_}}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Preset:                  rules.PresetCorrectness,
			SuppressionExpiryCutoff: "2026-08-11",
		},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 1 || len(result.Files[0].Suppressed) != 0 ||
		len(result.Files[0].SuppressionProblems) != 1 ||
		result.Files[0].SuppressionProblems[0].Kind != "expired" {
		t.Fatalf("RunPackages() expired suppression result = %#v", result)
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
