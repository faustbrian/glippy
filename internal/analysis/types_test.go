package analysis_test

import (
	"context"
	"errors"
	"go/ast"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/packages"
)

type typesRule struct {
	metadata rules.Metadata
	run func(*rules.TypesContext, ast.Node) ([]rules.Finding, error)
}

type packageRule struct {
	metadata rules.Metadata
	run func(*rules.PackageContext) ([]rules.PackageFinding, error)
}

func (r typesRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r typesRule) RunTypes(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
	return r.run(ctx, node)
}

func (r packageRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r packageRule) RunPackage(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
	return r.run(ctx)
}

func TestRunTypesRunsPackageRulesOncePerOwnedPackage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	projectPath := filepath.Join(root, "project.go")
	testPath := filepath.Join(root, "project_test.go")
	writeTypesFixture(t, projectPath, "package project\nfunc target() {}\n")
	writeTypesFixture(
		t,
		testPath,
		"package project\nimport \"testing\"\nfunc TestTarget(t *testing.T) { target() }\n",
	)

	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
			Tests: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	packageIDs := make([]string, 0, 2)
	configured := rules.BooleanOption(true)
	metadata := packageMetadata("package-target")
	defaultValue := rules.BooleanOption(false)
	metadata.Options = []rules.OptionMetadata{
		{
			Name: "enabled",
			Summary: "controls package reporting",
			Kind: rules.OptionBoolean,
			Default: &defaultValue,
		},
	}
	rule := packageRule{
		metadata: metadata,
		run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
			packageIDs = append(packageIDs, ctx.PackageID())
			if ctx.Package() == nil ||
				ctx.Info() == nil ||
				ctx.Sizes() == nil ||
				ctx.FileSet() == nil ||
				ctx.IllTyped() {
				t.Fatalf("package context is incomplete: %#v", ctx)
			}
			if enabled, found := ctx.BooleanOption("enabled"); !found || !enabled {
				t.Fatalf("package option = %v, %v", enabled, found)
			}
			findings := make([]rules.PackageFinding, 0)
			for _, file := range ctx.Files() {
				if !file.Target() {
					continue
				}
				range_, err := file.Range(file.Syntax().Name)
				if err != nil {
					return nil, err
				}
				findings = append(
					findings,
					rules.PackageFinding{
						File: file,
						Finding: rules.Finding{
							MessageKey: "package-target",
							Message: "package target",
							Range: range_,
						},
					},
				)
			}
			return findings, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.ResolveConfigured(
		rules.PresetCorrectness,
		nil,
		map[string]rules.OptionSet{
			"package-target": rules.NewOptionSet(
				map[string]rules.OptionValue{"enabled": configured},
			),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := analysis.RunTypes(context.Background(), loaded, registry, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 ||
		diagnostics[0].Path != projectPath ||
		diagnostics[1].Path != testPath {
		t.Fatalf("RunTypes() package diagnostics = %#v", diagnostics)
	}
	if len(packageIDs) != 2 ||
		packageIDs[0] != "example.com/project" ||
		!strings.Contains(packageIDs[1], "[example.com/project.test]") {
		t.Fatalf("RunTypes() package callbacks = %#v", packageIDs)
	}
}

func TestRunTypesProvidesDeclaredDependenciesInDependencyFirstOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "leaf", "leaf.go"),
		"package leaf\nconst Value = 1\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "middle", "middle.go"),
		"package middle\nimport \"example.com/project/leaf\"\nconst Value = leaf.Value\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nimport \"example.com/project/middle\"\nconst Value = middle.Value\n",
	)

	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
			LoadDependencySyntax: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	awareMetadata := packageMetadata("dependency-aware")
	awareMetadata.RequiresDependencySyntax = true
	var dependencyIDs []string
	registry, err := rules.NewRegistry(
		packageRule{
			metadata: awareMetadata,
			run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
				for _, dependency := range ctx.Dependencies() {
					dependencyIDs = append(
						dependencyIDs,
						dependency.PackageID(),
					)
					if dependency.Package() == nil ||
						dependency.Info() == nil ||
						dependency.Sizes() == nil ||
						dependency.FileSet() == nil {
						t.Fatalf(
							"dependency context is incomplete: %#v",
							dependency,
						)
					}
					for _, file := range dependency.Files() {
						if file.Target() {
							t.Fatalf(
								"dependency file %q is a target",
								file.Source().Path(),
							)
						}
					}
				}
				return nil, nil
			},
		},
		packageRule{
			metadata: packageMetadata("dependency-blind"),
			run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
				if dependencies := ctx.Dependencies(); len(dependencies) != 0 {
					t.Fatalf("dependency-blind rule received %#v", dependencies)
				}
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunTypes(context.Background(), loaded, registry, selection);
		err != nil {
		t.Fatal(err)
	}
	want := []string{"example.com/project/leaf", "example.com/project/middle"}
	if !reflect.DeepEqual(dependencyIDs, want) {
		t.Fatalf("dependency order = %#v, want %#v", dependencyIDs, want)
	}
}

func TestRunTypesRejectsFindingAgainstDependencyFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "dependency", "dependency.go"),
		"package dependency\nconst Value = 1\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nimport \"example.com/project/dependency\"\nconst Value = dependency.Value\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
			LoadDependencySyntax: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	metadata := packageMetadata("dependency-finding")
	metadata.RequiresDependencySyntax = true
	registry, err := rules.NewRegistry(
		packageRule{
			metadata: metadata,
			run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
				file := ctx.Dependencies()[0].Files()[0]
				range_, err := file.Range(file.Syntax().Name)
				if err != nil {
					return nil, err
				}
				return []rules.PackageFinding{
					{
						File: file,
						Finding: rules.Finding{
							MessageKey: "dependency",
							Message: "dependency",
							Range: range_,
						},
					},
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunTypes(context.Background(), loaded, registry, selection)
	if err == nil || !strings.Contains(err.Error(), "requires an owned target file") {
		t.Fatalf("RunTypes() dependency finding error = %v", err)
	}
}

func TestRunTypesRejectsPackageFindingFromUnownedTestVariant(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nfunc target() {}\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project_test.go"),
		"package project\nimport \"testing\"\nfunc TestTarget(t *testing.T) { target() }\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
			Tests: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	rule := packageRule{
		metadata: packageMetadata("foreign-package-target"),
		run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
			for _, file := range ctx.Files() {
				if file.Target() {
					continue
				}
				range_, err := file.Range(file.Syntax().Name)
				if err != nil {
					return nil, err
				}
				return []rules.PackageFinding{
					{
						File: file,
						Finding: rules.Finding{
							MessageKey: "foreign",
							Message: "foreign",
							Range: range_,
						},
					},
				}, nil
			}
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunTypes(context.Background(), loaded, registry, selection)
	if err == nil || !strings.Contains(err.Error(), "requires an owned target file") {
		t.Fatalf("RunTypes() unowned finding error = %v", err)
	}
}

func TestRunTypesPackageRulesHonorGeneratedPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "generated.go")
	writeTypesFixture(
		t,
		path,
		"// Code generated by fixture. DO NOT EDIT.\npackage project\nfunc generated() {}\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	skippedCalls := 0
	allowedCalls := 0
	skipped := packageRule{
		metadata: packageMetadata("generated-skipped"),
		run: func(*rules.PackageContext) ([]rules.PackageFinding, error) {
			skippedCalls++
			return nil, nil
		},
	}
	allowedMetadata := packageMetadata("generated-allowed")
	allowedMetadata.RunOnGenerated = true
	allowed := packageRule{
		metadata: allowedMetadata,
		run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
			allowedCalls++
			files := ctx.Files()
			if len(files) != 1 ||
				!files[0].Target() ||
				files[0].Source().Path() != path {
				t.Fatalf("generated package files = %#v", files)
			}
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(skipped, allowed)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunTypes(context.Background(), loaded, registry, selection);
		err != nil {
		t.Fatal(err)
	}
	if skippedCalls != 0 || allowedCalls != 1 {
		t.Fatalf(
			"generated package callbacks = skipped %d, allowed %d",
			skippedCalls,
			allowedCalls,
		)
	}
}

func TestRunTypesPackageRulesHonorTypeErrorPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nvar value = missing\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	skippedCalls := 0
	allowedCalls := 0
	skipped := packageRule{
		metadata: packageMetadata("type-error-skipped"),
		run: func(*rules.PackageContext) ([]rules.PackageFinding, error) {
			skippedCalls++
			return nil, nil
		},
	}
	allowedMetadata := packageMetadata("type-error-allowed")
	allowedMetadata.RunDespiteTypeErrors = true
	allowed := packageRule{
		metadata: allowedMetadata,
		run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
			allowedCalls++
			if !ctx.IllTyped() {
				t.Fatal("package rule did not receive type-error state")
			}
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(skipped, allowed)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunTypes(context.Background(), loaded, registry, selection);
		err != nil {
		t.Fatal(err)
	}
	if skippedCalls != 0 || allowedCalls != 1 {
		t.Fatalf(
			"type-error package callbacks = skipped %d, allowed %d",
			skippedCalls,
			allowedCalls,
		)
	}
}

func TestRunTypesPackageRulesRequireTypeSizes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nfunc target() {}\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	packageWithoutSizes := *loaded.Packages[0]
	packageWithoutSizes.TypesSizes = nil
	loaded.Packages = append([]*packages.Package(nil), loaded.Packages...)
	loaded.Packages[0] = &packageWithoutSizes
	rule := packageRule{
		metadata: packageMetadata("missing-sizes"),
		run: func(*rules.PackageContext) ([]rules.PackageFinding, error) {
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunTypes(context.Background(), loaded, registry, selection)
	if err == nil || !strings.Contains(err.Error(), "missing type sizes") {
		t.Fatalf("RunTypes() missing sizes error = %v", err)
	}
}

func TestRunTypesRejectsPackageFindingFromPriorCallback(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nfunc target() {}\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var prior rules.PackageFile
	usePrior := false
	rule := packageRule{
		metadata: packageMetadata("stale-package-target"),
		run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
			file := ctx.Files()[0]
			if !usePrior {
				prior = file
				return nil, nil
			}
			range_, err := file.Range(file.Syntax().Name)
			if err != nil {
				return nil, err
			}
			return []rules.PackageFinding{
				{
					File: prior,
					Finding: rules.Finding{
						MessageKey: "stale",
						Message: "stale",
						Range: range_,
					},
				},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunTypes(context.Background(), loaded, registry, selection);
		err != nil {
		t.Fatal(err)
	}
	usePrior = true
	_, err = analysis.RunTypes(context.Background(), loaded, registry, selection)
	if err == nil || !strings.Contains(err.Error(), "same package callback") {
		t.Fatalf("RunTypes() prior callback finding error = %v", err)
	}
}

func TestRunTypesSharesTypedPackageAndExactSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "sample.go")
	writeTypesFixture(
		t,
		path,
		"package sample\nimport \"strings\"\nfunc run(value string) bool { return strings.Contains(value, \"x\") }\n",
	)

	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	rule := typesRule{
		metadata: typesMetadata("typed-call", rules.NodeCallExpr),
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			call := node.(*ast.CallExpr)
			if ctx.PackageID() != "example.com/project" ||
				ctx.Package().Path() != "example.com/project" ||
				ctx.Info().TypeOf(call) == nil ||
				ctx.File().Path() != path {
				t.Fatalf("typed context = %#v", ctx)
			}
			range_, err := ctx.Range(call)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "typed-call", Message: "typed call", Range: range_},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := analysis.RunTypes(context.Background(), loaded, registry, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Path != path {
		t.Fatalf("RunTypes() = %#v", diagnostics)
	}
	file, _ := loaded.Sources.Lookup(path)
	if diagnostics[0].Digest != file.Digest() {
		t.Fatalf("RunTypes() digest = %x, want %x", diagnostics[0].Digest, file.Digest())
	}
	got, valid := file.Slice(diagnostics[0].Range)
	if !valid || got != "strings.Contains(value, \"x\")" {
		t.Fatalf("RunTypes() range = %q, %t", got, valid)
	}
}

func TestRunTypesRunsOnlyOptedInRulesOnTypeErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "sample.go")
	writeTypesFixture(
		t,
		path,
		"package sample\nfunc run() { _ = missing; target() }\nfunc target() {}\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Packages) != 1 || !loaded.Packages[0].IllTyped {
		t.Fatalf("LoadPackages() = %#v", loaded.Packages)
	}

	newRule := func(id string, runDespiteErrors bool) typesRule {
		metadata := typesMetadata(id, rules.NodeCallExpr)
		metadata.RunDespiteTypeErrors = runDespiteErrors
		return typesRule{
			metadata: metadata,
			run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
				if !ctx.IllTyped() {
					t.Fatal(
						"types context did not retain package type-error state",
					)
				}
				range_, err := ctx.Range(node)
				if err != nil {
					return nil, err
				}
				return []rules.Finding{
					{MessageKey: id, Message: id, Range: range_},
				}, nil
			},
		}
	}
	registry, err := rules.NewRegistry(
		newRule("skip-errors", false),
		newRule("run-errors", true),
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := analysis.RunTypes(context.Background(), loaded, registry, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 ||
		diagnostics[0].RuleID != "run-errors" ||
		diagnostics[0].Path != path {
		t.Fatalf("RunTypes() = %#v", diagnostics)
	}
}

func TestRunTypesHonorsGeneratedPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "generated.go")
	writeTypesFixture(
		t,
		path,
		"// Code generated by test. DO NOT EDIT.\npackage project\nfunc run() { target() }\nfunc target() {}\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	newRule := func(id string, generated bool) typesRule {
		metadata := typesMetadata(id, rules.NodeCallExpr)
		metadata.RunOnGenerated = generated
		return typesRule{
			metadata: metadata,
			run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
				range_, err := ctx.Range(node)
				if err != nil {
					return nil, err
				}
				return []rules.Finding{
					{MessageKey: id, Message: id, Range: range_},
				}, nil
			},
		}
	}
	registry, err := rules.NewRegistry(
		newRule("generated-skip", false),
		newRule("generated-run", true),
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := analysis.RunTypes(context.Background(), loaded, registry, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 ||
		diagnostics[0].RuleID != "generated-run" ||
		diagnostics[0].Path != path {
		t.Fatalf("RunTypes() generated diagnostics = %#v", diagnostics)
	}
}

func TestRunTypesPreservesCancellationAfterRuleExecution(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nfunc run() { target() }\nfunc target() {}\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	rule := typesRule{
		metadata: typesMetadata("cancel-types", rules.NodeCallExpr),
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			cancel()
			range_, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "canceled", Message: "canceled", Range: range_},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics, err := analysis.RunTypes(ctx, loaded, registry, selection);
		!errors.Is(err, context.Canceled) || diagnostics != nil {
		t.Fatalf("RunTypes() = %#v, %v", diagnostics, err)
	}
}

func TestRunTypesOrdersPackagesFilesAndRulesCanonically(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	aPath := filepath.Join(root, "a", "a.go")
	zPath := filepath.Join(root, "z", "z.go")
	writeTypesFixture(t, zPath, "package z\nfunc run() { target() }\nfunc target() {}\n")
	writeTypesFixture(t, aPath, "package a\nfunc run() { target() }\nfunc target() {}\n")
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./z", "./a"},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	newRule := func(id string) typesRule {
		return typesRule{
			metadata: typesMetadata(id, rules.NodeCallExpr),
			run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
				range_, err := ctx.Range(node)
				if err != nil {
					return nil, err
				}
				return []rules.Finding{
					{MessageKey: id, Message: id, Range: range_},
				}, nil
			},
		}
	}
	registry, err := rules.NewRegistry(newRule("z-rule"), newRule("a-rule"))
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := analysis.RunTypes(context.Background(), loaded, registry, selection)
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{aPath, aPath, zPath, zPath}
	wantRules := []string{"a-rule", "z-rule", "a-rule", "z-rule"}
	if len(diagnostics) != len(wantPaths) {
		t.Fatalf("RunTypes() = %#v", diagnostics)
	}
	for index, diagnostic := range diagnostics {
		if diagnostic.Path != wantPaths[index] || diagnostic.RuleID != wantRules[index] {
			t.Fatalf("RunTypes()[%d] = %#v", index, diagnostic)
		}
	}
}

func TestRunTypesRejectsCrossFilePositions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "a.go"),
		"package project\nfunc a() { target() }\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "b.go"),
		"package project\nfunc b() { target() }\nfunc target() {}\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var first ast.Node
	rule := typesRule{
		metadata: typesMetadata("foreign-position", rules.NodeCallExpr),
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			if first == nil {
				first = node
				return nil, nil
			}
			_, err := ctx.Range(first)
			return nil, err
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunTypes(context.Background(), loaded, registry, selection);
		err == nil || !strings.Contains(err.Error(), "belong to another source file") {
		t.Fatalf("RunTypes() cross-file error = %v", err)
	}
}

func TestRunTypesRejectsInvalidExecutionContracts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nfunc run() { target() }\nfunc target() {}\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	rule := typesRule{
		metadata: typesMetadata("valid-types", rules.NodeCallExpr),
		run: func(*rules.TypesContext, ast.Node) ([]rules.Finding, error) {
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := analysis.RunTypes(nil, loaded, registry, selection); err == nil {
		t.Fatal("RunTypes() accepted nil context")
	}
	untyped := loaded
	untyped.Requirement = rules.RequireSyntax
	if _, err := analysis.RunTypes(context.Background(), untyped, registry, selection);
		err == nil {
		t.Fatal("RunTypes() accepted syntax-only load")
	}
	missingSource := loaded
	missingSource.Sources = analysis.PackageSourceSet{}
	if _, err := analysis.RunTypes(context.Background(), missingSource, registry, selection);
		err == nil || !strings.Contains(err.Error(), "was not captured") {
		t.Fatalf("RunTypes() missing source error = %v", err)
	}
	invalidSeverity := append([]rules.Selection(nil), selection...)
	invalidSeverity[0].Severity = "fatal"
	if _, err := analysis.RunTypes(context.Background(), loaded, registry, invalidSeverity);
		err == nil || !strings.Contains(err.Error(), "invalid severity") {
		t.Fatalf("RunTypes() invalid severity error = %v", err)
	}

	metadata := typesMetadata("missing-execution", rules.NodeCallExpr)
	wrongRegistry, err := rules.NewRegistry(metadataRuleAdapter{metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	wrongSelection, err := wrongRegistry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunTypes(context.Background(), loaded, wrongRegistry, wrongSelection);
		err == nil || !strings.Contains(err.Error(), "does not implement types execution") {
		t.Fatalf("RunTypes() missing execution error = %v", err)
	}
}

func TestRunTypesAnalyzesEachPhysicalFileOnceAcrossTestVariants(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	projectPath := filepath.Join(root, "project.go")
	testPath := filepath.Join(root, "project_test.go")
	writeTypesFixture(
		t,
		projectPath,
		"package project\nfunc run() { target() }\nfunc target() {}\n",
	)
	writeTypesFixture(
		t,
		testPath,
		"package project\nimport \"testing\"\nfunc TestInternal(t *testing.T) { target() }\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireTypes,
			Tests: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	packageIDs := make(map[string]string)
	rule := typesRule{
		metadata: typesMetadata("typed-target", rules.NodeCallExpr),
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			call := node.(*ast.CallExpr)
			identifier, found := call.Fun.(*ast.Ident)
			if !found || identifier.Name != "target" {
				return nil, nil
			}
			packageIDs[ctx.File().Path()] = ctx.PackageID()
			range_, err := ctx.Range(call)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "target", Message: "target", Range: range_},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := analysis.RunTypes(context.Background(), loaded, registry, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 ||
		diagnostics[0].Path != projectPath ||
		diagnostics[1].Path != testPath {
		t.Fatalf("RunTypes() test variants = %#v", diagnostics)
	}
	if packageIDs[projectPath] != "example.com/project" ||
		!strings.Contains(packageIDs[testPath], "[example.com/project.test]") {
		t.Fatalf("RunTypes() package owners = %#v", packageIDs)
	}
}

func typesMetadata(id string, interest rules.NodeKind) rules.Metadata {
	return rules.Metadata{
		ID: id,
		Summary: "reports typed syntax",
		Documentation: "Full typed rule documentation.",
		DefaultSeverity: rules.SeverityWarn,
		Presets: []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement: rules.RequireTypes,
		NodeInterests: []rules.NodeKind{interest},
		Categories: []rules.Category{rules.CategoryCorrectness},
		Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
	}
}

func packageMetadata(id string) rules.Metadata {
	metadata := typesMetadata(id, rules.NodeFile)
	metadata.Summary = "reports package-wide typed syntax"
	metadata.Documentation = "Full package-wide typed rule documentation."
	metadata.NodeInterests = nil
	return metadata
}

func writeTypesFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
