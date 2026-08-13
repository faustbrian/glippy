package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"slices"
	"sort"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

type activePackageAnalyzer struct {
	rule *packageAnalyzerRule
	metadata rules.Metadata
	severity rules.Severity
}

func packageAnalyzersNeedFacts(
	registry *rules.Registry,
	selection []rules.Selection,
) (bool, error) {
	for _, selected := range selection {
		if selected.Requirement != rules.RequireTypes {
			continue
		}
		rule, found := registry.Lookup(selected.ID)
		if !found {
			return false, fmt.Errorf("selected unknown types rule %q", selected.ID)
		}
		if adapted, found := rule.(*packageAnalyzerRule); found && adapted.usesFacts() {
			return true, nil
		}
	}
	return false, nil
}

func partitionPackageAnalyzers(
	registry *rules.Registry,
	selection []rules.Selection,
) ([]rules.Selection, []rules.Selection, error) {
	native := make([]rules.Selection, 0, len(selection))
	adapted := make([]rules.Selection, 0, len(selection))
	for _, selected := range selection {
		rule, found := registry.Lookup(selected.ID)
		if !found {
			return nil, nil, fmt.Errorf("selected unknown types rule %q", selected.ID)
		}
		if _, found := rule.(*packageAnalyzerRule); found {
			adapted = append(adapted, selected)
		} else {
			native = append(native, selected)
		}
	}
	return native, adapted, nil
}

func runPackageAnalyzers(
	ctx context.Context,
	loaded PackageLoadResult,
	loadOptions PackageLoadOptions,
	cachePlan *packageCachePlan,
	registry *rules.Registry,
	selection []rules.Selection,
) ([]rules.Diagnostic, error) {
	if len(selection) == 0 {
		return []rules.Diagnostic{}, nil
	}
	if ctx == nil {
		return nil, fmt.Errorf("package analyzer execution requires a context")
	}
	if registry == nil {
		return nil, fmt.Errorf("package analyzer execution requires a rule registry")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if loaded.Requirement < rules.RequireTypes {
		return nil, fmt.Errorf("package analyzer execution requires a typed package load")
	}
	active, err := preparePackageAnalyzers(registry, selection)
	if err != nil {
		return nil, err
	}
	packages_, err := canonicalPackages(loaded.Packages)
	if err != nil {
		return nil, err
	}
	ownedFiles, err := canonicalTypedFiles(packages_, loaded.Sources)
	if err != nil {
		return nil, err
	}
	owners := make(map[string]string, len(ownedFiles))
	for _, owned := range ownedFiles {
		owners[owned.file.path] = owned.package_.ID
	}

	diagnostics := make([]rules.Diagnostic, 0)
	for _, analyzer := range active {
		if analyzer.rule.usesFacts() {
			produced, err := analyzer.rule.runPackageFactGraph(
				ctx,
				loaded,
				loadOptions,
				cachePlan,
				owners,
				analyzer.metadata,
				analyzer.severity,
			)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", analyzer.metadata.ID, err)
			}
			diagnostics = append(diagnostics, produced...)
			continue
		}
		for _, pkg := range packages_ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			files, err := packageSyntaxFiles(pkg, loaded.Sources)
			if err != nil {
				return nil, err
			}
			if pkg.IllTyped && !analyzer.metadata.RunDespiteTypeErrors {
				continue
			}
			if !packageAnalyzerOwnsEligibleFile(
				pkg.ID,
				files,
				owners,
				analyzer.metadata,
			) {
				continue
			}
			produced, err := analyzer.rule.runPackage(
				ctx,
				pkg,
				files,
				owners,
				analyzer.severity,
				nil,
			)
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", analyzer.metadata.ID, err)
			}
			diagnostics = append(diagnostics, produced...)
		}
	}
	return OrderDiagnostics(diagnostics), nil
}

func preparePackageAnalyzers(
	registry *rules.Registry,
	selection []rules.Selection,
) ([]activePackageAnalyzer, error) {
	ordered := slices.Clone(selection)
	sort.Slice(
		ordered,
		func(left, right int) bool {
			return ordered[left].ID < ordered[right].ID
		},
	)
	result := make([]activePackageAnalyzer, 0, len(ordered))
	previousID := ""
	for _, selected := range ordered {
		if selected.ID == previousID {
			return nil, fmt.Errorf(
				"selected package analyzer %q more than once",
				selected.ID,
			)
		}
		previousID = selected.ID
		if selected.Severity != rules.SeverityWarn &&
			selected.Severity != rules.SeverityError {
			return nil, fmt.Errorf(
				"selected package analyzer %q has invalid severity %q",
				selected.ID,
				selected.Severity,
			)
		}
		nativeRule, found := registry.Lookup(selected.ID)
		if !found {
			return nil, fmt.Errorf("selected unknown package analyzer %q", selected.ID)
		}
		metadata, _ := registry.Metadata(selected.ID)
		if selected.Requirement != metadata.Requirement {
			return nil, fmt.Errorf(
				"selected package analyzer %q requirement does not match registry",
				selected.ID,
			)
		}
		if metadata.Requirement != rules.RequireTypes {
			return nil, fmt.Errorf(
				"selected package analyzer %q requires %s; adapter requires types",
				selected.ID,
				metadata.Requirement,
			)
		}
		adapted, found := nativeRule.(*packageAnalyzerRule)
		if !found {
			return nil, fmt.Errorf(
				"selected rule %q is not a package analyzer",
				selected.ID,
			)
		}
		if len(metadata.NodeInterests) != 1 || metadata.NodeInterests[0] != rules.NodeFile {
			return nil, fmt.Errorf(
				"selected package analyzer %q must declare only file interest",
				selected.ID,
			)
		}
		adapted, err := adapted.forRun(selected.Options)
		if err != nil {
			return nil, fmt.Errorf("selected package analyzer %q: %w", selected.ID, err)
		}
		result = append(
			result,
			activePackageAnalyzer{
				rule: adapted,
				metadata: metadata,
				severity: selected.Severity,
			},
		)
	}
	return result, nil
}

func (r *packageAnalyzerRule) forRun(options rules.OptionSet) (*packageAnalyzerRule, error) {
	if r.factory == nil {
		return r, nil
	}
	instance, err := callAnalyzerFactory(r.factory)
	if err != nil {
		return nil, err
	}
	if err := validateAnalyzerFactoryInstance(
		instance,
		r.analyzer.Name,
		r.contract,
		r.admission,
	);
		err != nil {
		return nil, err
	}
	if err := bindAnalyzerFlags(
		instance,
		r.metadata,
		r.bindings,
		analyzerOptionSetLookup{options: options},
	);
		err != nil {
		return nil, err
	}
	plan := analyzerExecutionPlan(instance)
	steps := make([]packageAnalyzerStep, len(plan))
	for index, step := range plan {
		steps[index] = packageAnalyzerStep{original: step, analyzer: *step}
	}
	snapshot := *instance
	snapshot.Requires = nil
	snapshot.FactTypes = nil
	runtime := *r
	runtime.analyzer = snapshot
	runtime.steps = steps
	runtime.factory = nil
	return &runtime, nil
}

func packageAnalyzerOwnsEligibleFile(
	packageID string,
	files []typedSyntaxFile,
	owners map[string]string,
	metadata rules.Metadata,
) bool {
	for _, file := range files {
		if owners[file.path] != packageID {
			continue
		}
		if file.source.Metadata().Generated && !metadata.RunOnGenerated {
			continue
		}
		return true
	}
	return false
}

func (r *packageAnalyzerRule) runPackage(
	ctx context.Context,
	pkg *packages.Package,
	files []typedSyntaxFile,
	owners map[string]string,
	severity rules.Severity,
	facts *analyzerFactSet,
) ([]rules.Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if pkg == nil ||
		pkg.Fset == nil ||
		pkg.Types == nil ||
		pkg.TypesInfo == nil ||
		pkg.TypesSizes == nil {
		return nil, fmt.Errorf("adapted package is missing required type information")
	}
	module, err := packageAnalyzerModule(pkg.Module, 0)
	if err != nil {
		return nil, err
	}
	syntax := make([]*ast.File, len(files))
	byPath := make(map[string]*source.File, len(files))
	for index, file := range files {
		syntax[index] = file.syntax
		byPath[file.path] = file.source
	}
	if len(r.steps) == 0 {
		return nil, fmt.Errorf("adapted package has no analyzer execution plan")
	}
	results := make(map[*goanalysis.Analyzer]any, len(r.steps))
	upstream := make([]goanalysis.Diagnostic, 0)
	for stepIndex, planned := range r.steps {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		analyzer := planned.analyzer
		resultOf := make(map[*goanalysis.Analyzer]any, len(planned.original.Requires))
		for _, required := range planned.original.Requires {
			result, found := results[required]
			if !found {
				return nil, fmt.Errorf(
					"analyzer %q prerequisite %q did not run",
					analyzer.Name,
					required.Name,
				)
			}
			resultOf[required] = result
		}
		reported := make([]goanalysis.Diagnostic, 0)
		importObjectFact := func(types.Object, goanalysis.Fact) bool {
			return false
		}
		exportObjectFact := func(types.Object, goanalysis.Fact) {
			panic("adapted analyzer attempted to export an undeclared object fact")
		}
		allObjectFacts := func() []goanalysis.ObjectFact {
			return nil
		}
		importPackageFact := func(*types.Package, goanalysis.Fact) bool {
			return false
		}
		exportPackageFact := func(goanalysis.Fact) {
			panic("adapted analyzer attempted to export an undeclared package fact")
		}
		allPackageFacts := func() []goanalysis.PackageFact {
			return nil
		}
		if facts != nil {
			if err := facts.beginObjectFacts(planned.original, pkg); err != nil {
				return nil, err
			}
			importObjectFact = func(object types.Object, fact goanalysis.Fact) bool {
				return facts.importObjectFact(
					planned.original,
					pkg.Types,
					object,
					fact,
				)
			}
			exportObjectFact = func(object types.Object, fact goanalysis.Fact) {
				facts.exportObjectFact(planned.original, pkg.Types, object, fact)
			}
			allObjectFacts = func() []goanalysis.ObjectFact {
				return facts.allObjectFacts(planned.original, pkg.Types)
			}
			importPackageFact = func(
				package_ *types.Package,
				fact goanalysis.Fact,
			) bool {
				return facts.importPackageFact(planned.original, package_, fact)
			}
			exportPackageFact = func(fact goanalysis.Fact) {
				facts.exportPackageFact(planned.original, pkg.Types, fact)
			}
			allPackageFacts = func() []goanalysis.PackageFact {
				return facts.allPackageFacts(planned.original, pkg.Types)
			}
		}
		pass := &goanalysis.Pass{
			Analyzer: &analyzer,
			Fset: pkg.Fset,
			Files: syntax,
			Pkg: pkg.Types,
			TypesInfo: pkg.TypesInfo,
			TypesSizes: pkg.TypesSizes,
			Module: module,
			Report: func(diagnostic goanalysis.Diagnostic) {
				reported = append(reported, cloneAnalyzerDiagnostic(diagnostic))
			},
			ResultOf: resultOf,
			ReadFile: func(filename string) ([]byte, error) {
				path := filepath.Clean(filename)
				if !filepath.IsAbs(path) || path != filename {
					return nil, fmt.Errorf(
						"read file %q: path is not normalized absolute",
						filename,
					)
				}
				file, found := byPath[path]
				if !found {
					return nil, fmt.Errorf(
						"read file %q: outside the adapted package source",
						filename,
					)
				}
				return file.Bytes(), nil
			},
			ImportObjectFact: importObjectFact,
			ImportPackageFact: importPackageFact,
			ExportObjectFact: exportObjectFact,
			ExportPackageFact: exportPackageFact,
			AllPackageFacts: allPackageFacts,
			AllObjectFacts: allObjectFacts,
		}
		if analyzer.RunDespiteErrors {
			pass.TypeErrors = slices.Clone(pkg.TypeErrors)
		}
		result, err := runAnalyzer(&analyzer, pass)
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		if err != nil {
			return nil, fmt.Errorf("analyzer %q: %w", analyzer.Name, err)
		}
		if got, want := reflect.TypeOf(result), analyzer.ResultType; got != want {
			return nil, fmt.Errorf(
				"analyzer %q returned result type %v; declared %v",
				analyzer.Name,
				got,
				want,
			)
		}
		if stepIndex != len(r.steps) - 1 && len(reported) != 0 {
			return nil, fmt.Errorf(
				"prerequisite analyzer %q reported diagnostics",
				analyzer.Name,
			)
		}
		if stepIndex == len(r.steps) - 1 {
			upstream = reported
		}
		results[planned.original] = result
	}

	diagnostics := make([]rules.Diagnostic, 0, len(upstream))
	for _, diagnostic := range upstream {
		file, finding, err := r.packageFinding(pkg.Fset, byPath, diagnostic)
		if err != nil {
			return nil, err
		}
		if owners[file.Path()] != pkg.ID {
			continue
		}
		if file.Metadata().Generated && !r.metadata.RunOnGenerated {
			continue
		}
		mapped, err := diagnosticForFinding(file, r.metadata, severity, finding)
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, mapped)
	}
	return diagnostics, nil
}

func (r *packageAnalyzerRule) packageFinding(
	fileSet *token.FileSet,
	files map[string]*source.File,
	diagnostic goanalysis.Diagnostic,
) (*source.File, rules.Finding, error) {
	file, primary, err := packageAnalyzerRange(fileSet, files, diagnostic.Pos, diagnostic.End)
	if err != nil {
		return nil, rules.Finding{}, fmt.Errorf("diagnostic range: %w", err)
	}
	messageKey := diagnostic.Category
	if messageKey == "" {
		messageKey = r.analyzer.Name
	}
	related := make([]rules.Related, len(diagnostic.Related))
	for index, item := range diagnostic.Related {
		relatedFile, sourceRange, err := packageAnalyzerRange(
			fileSet,
			files,
			item.Pos,
			item.End,
		)
		if err != nil {
			return nil, rules.Finding{}, fmt.Errorf("related range %d: %w", index, err)
		}
		if relatedFile != file {
			return nil, rules.Finding{}, fmt.Errorf(
				"related range %d belongs to another source file",
				index,
			)
		}
		related[index] = rules.Related{Range: sourceRange, Message: item.Message}
	}
	fixes := make([]rules.Fix, len(diagnostic.SuggestedFixes))
	for fixIndex, suggested := range diagnostic.SuggestedFixes {
		mapped, found := r.fixes[suggested.Message]
		if !found {
			return nil, rules.Finding{}, fmt.Errorf(
				"undeclared suggested fix %q",
				suggested.Message,
			)
		}
		edits := make([]rules.Edit, len(suggested.TextEdits))
		for editIndex, edit := range suggested.TextEdits {
			editFile, sourceRange, err := packageAnalyzerRange(
				fileSet,
				files,
				edit.Pos,
				edit.End,
			)
			if err != nil {
				return nil, rules.Finding{}, fmt.Errorf(
					"suggested fix %q edit %d: %w",
					suggested.Message,
					editIndex,
					err,
				)
			}
			if editFile != file {
				return nil, rules.Finding{}, fmt.Errorf(
					"suggested fix %q edit %d belongs to another source file",
					suggested.Message,
					editIndex,
				)
			}
			edits[editIndex] = rules.Edit{
				Range: sourceRange,
				NewText: string(edit.NewText),
			}
		}
		fixes[fixIndex] = rules.Fix{Name: mapped.name, Safety: mapped.safety, Edits: edits}
	}
	help, err := analyzerDiagnosticURL(&r.analyzer, diagnostic)
	if err != nil {
		return nil, rules.Finding{}, err
	}
	return file, rules.Finding{
		MessageKey: messageKey,
		Message: diagnostic.Message,
		Range: primary,
		Related: related,
		Help: help,
		Fixes: fixes,
	}, nil
}

func packageAnalyzerRange(
	fileSet *token.FileSet,
	files map[string]*source.File,
	start token.Pos,
	end token.Pos,
) (*source.File, source.Range, error) {
	if fileSet == nil || !start.IsValid() {
		return nil, source.Range{}, fmt.Errorf("position is invalid")
	}
	if !end.IsValid() {
		end = start
	}
	physicalStart := fileSet.PositionFor(start, false)
	physicalEnd := fileSet.PositionFor(end, false)
	path := filepath.Clean(physicalStart.Filename)
	if !filepath.IsAbs(path) || path != physicalStart.Filename || physicalEnd.Filename != path {
		return nil, source.Range{}, fmt.Errorf(
			"positions do not belong to one adapted package source",
		)
	}
	file, found := files[path]
	if !found {
		return nil, source.Range{}, fmt.Errorf(
			"position is outside the adapted package source",
		)
	}
	range_ := source.Range{Start: physicalStart.Offset, End: physicalEnd.Offset}
	if _, valid := file.Slice(range_); !valid {
		return nil, source.Range{}, fmt.Errorf("positions map to an invalid physical range")
	}
	return file, range_, nil
}

func packageAnalyzerModule(module *packages.Module, depth int) (*goanalysis.Module, error) {
	if module == nil {
		return nil, nil
	}
	if depth >= 16 {
		return nil, fmt.Errorf(
			"adapted package module replacement chain exceeds 16 entries",
		)
	}
	replacement, err := packageAnalyzerModule(module.Replace, depth + 1)
	if err != nil {
		return nil, err
	}
	result := &goanalysis.Module{
		Path: module.Path,
		Version: module.Version,
		Replace: replacement,
		Main: module.Main,
		Indirect: module.Indirect,
		Dir: module.Dir,
		GoMod: module.GoMod,
		GoVersion: module.GoVersion,
	}
	if module.Time != nil {
		created := *module.Time
		result.Time = &created
	}
	if module.Error != nil {
		result.Error = &goanalysis.ModuleError{Err: module.Error.Err}
	}
	return result, nil
}
