package analysis

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	goanalysis "golang.org/x/tools/go/analysis"

	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

// AnalyzerFixMapping binds one exact go/analysis suggestion to native metadata.
type AnalyzerFixMapping struct {
	Message     string
	Name        string
	Description string
	Safety      rules.FixSafety
	Audited     bool
}

// AnalyzerAdapterOptions supplies the native product contract for an analyzer.
type AnalyzerAdapterOptions struct {
	Metadata        rules.Metadata
	SuggestedFixes  []AnalyzerFixMapping
	ReadOnlyAudited bool
}

type analyzerFix struct {
	name   string
	safety rules.FixSafety
}

type analyzerRule struct {
	analyzer goanalysis.Analyzer
	metadata rules.Metadata
	fixes    map[string]analyzerFix
}

type packageAnalyzerRule struct {
	analyzer goanalysis.Analyzer
	metadata rules.Metadata
	fixes    map[string]analyzerFix
	steps    []packageAnalyzerStep
}

type packageAnalyzerStep struct {
	original *goanalysis.Analyzer
	analyzer goanalysis.Analyzer
}

// AdaptAnalyzer wraps one suitable go/analysis analyzer as a native rule.
func AdaptAnalyzer(
	analyzer *goanalysis.Analyzer,
	options AnalyzerAdapterOptions,
) (rules.Rule, error) {
	if analyzer == nil {
		return nil, fmt.Errorf("adapt go/analysis: nil analyzer")
	}
	if err := goanalysis.Validate([]*goanalysis.Analyzer{analyzer}); err != nil {
		return nil, fmt.Errorf("adapt go/analysis: %w", err)
	}
	typed := options.Metadata.Requirement == rules.RequireTypes
	if len(analyzer.Requires) != 0 && !typed {
		return nil, fmt.Errorf("adapt go/analysis %q: prerequisite analyzers are not supported", analyzer.Name)
	}
	if analyzer.ResultType != nil && !typed {
		return nil, fmt.Errorf("adapt go/analysis %q: analyzer results require prerequisite scheduling", analyzer.Name)
	}
	plan := analyzerExecutionPlan(analyzer)
	for _, step := range plan {
		if len(step.FactTypes) != 0 && !typed {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: analyzer %q facts require typed package execution",
				analyzer.Name,
				step.Name,
			)
		}
		hasFlags := false
		step.Flags.VisitAll(func(*flag.Flag) { hasFlags = true })
		if hasFlags {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: analyzer %q flags require native typed configuration",
				analyzer.Name,
				step.Name,
			)
		}
	}
	if len(options.Metadata.Fixes) != 0 {
		return nil, fmt.Errorf("adapt go/analysis %q: native fix metadata must come from suggested-fix mappings", analyzer.Name)
	}

	metadata := cloneAnalyzerMetadata(options.Metadata)
	fixes := make(map[string]analyzerFix, len(options.SuggestedFixes))
	fixNames := make(map[string]struct{}, len(options.SuggestedFixes))
	for index, mapping := range options.SuggestedFixes {
		if strings.TrimSpace(mapping.Message) == "" || strings.TrimSpace(mapping.Name) == "" ||
			strings.TrimSpace(mapping.Description) == "" {
			return nil, fmt.Errorf("adapt go/analysis %q: suggested-fix mapping %d is incomplete", analyzer.Name, index)
		}
		if _, duplicate := fixes[mapping.Message]; duplicate {
			return nil, fmt.Errorf("adapt go/analysis %q: duplicate suggested-fix message %q", analyzer.Name, mapping.Message)
		}
		if _, duplicate := fixNames[mapping.Name]; duplicate {
			return nil, fmt.Errorf("adapt go/analysis %q: duplicate native fix name %q", analyzer.Name, mapping.Name)
		}
		safety := mapping.Safety
		if safety == "" {
			safety = rules.FixSuggestion
		}
		if safety == rules.FixSafe && !mapping.Audited {
			return nil, fmt.Errorf("adapt go/analysis %q: safe fix %q requires an explicit safety audit", analyzer.Name, mapping.Name)
		}
		if mapping.Audited && safety != rules.FixSafe {
			return nil, fmt.Errorf("adapt go/analysis %q: fix audit applies only to safe fixes", analyzer.Name)
		}
		fixes[mapping.Message] = analyzerFix{name: mapping.Name, safety: safety}
		fixNames[mapping.Name] = struct{}{}
		metadata.Fixes = append(metadata.Fixes, rules.FixMetadata{
			Name: mapping.Name, Description: mapping.Description, Safety: safety,
		})
	}

	snapshot := *analyzer
	snapshot.Requires = nil
	snapshot.FactTypes = nil
	if len(metadata.NodeInterests) != 1 || metadata.NodeInterests[0] != rules.NodeFile {
		return nil, fmt.Errorf(
			"adapt go/analysis %q: adapter metadata must declare only file interest",
			analyzer.Name,
		)
	}
	var adapted rules.Rule
	switch metadata.Requirement {
	case rules.RequireSyntax:
		adapted = &analyzerRule{analyzer: snapshot, metadata: metadata, fixes: fixes}
	case rules.RequireTypes:
		if !options.ReadOnlyAudited {
			return nil, fmt.Errorf(
				"adapt go/analysis %q: typed package execution requires a read-only analyzer audit",
				analyzer.Name,
			)
		}
		if metadata.RunDespiteTypeErrors {
			for _, step := range plan {
				if !step.RunDespiteErrors {
					return nil, fmt.Errorf(
						"adapt go/analysis %q: native type-error policy exceeds analyzer %q contract",
						analyzer.Name,
						step.Name,
					)
				}
			}
		}
		steps := make([]packageAnalyzerStep, len(plan))
		for index, step := range plan {
			steps[index] = packageAnalyzerStep{original: step, analyzer: *step}
		}
		adapted = &packageAnalyzerRule{
			analyzer: snapshot,
			metadata: metadata,
			fixes:    fixes,
			steps:    steps,
		}
	default:
		return nil, fmt.Errorf(
			"adapt go/analysis %q: adapter metadata must declare syntax or types requirement",
			analyzer.Name,
		)
	}
	if _, err := rules.NewRegistry(adapted); err != nil {
		return nil, fmt.Errorf("adapt go/analysis %q metadata: %w", analyzer.Name, err)
	}
	return adapted, nil
}

func analyzerExecutionPlan(root *goanalysis.Analyzer) []*goanalysis.Analyzer {
	visited := make(map[*goanalysis.Analyzer]struct{})
	plan := make([]*goanalysis.Analyzer, 0)
	var visit func(*goanalysis.Analyzer)
	visit = func(analyzer *goanalysis.Analyzer) {
		if _, found := visited[analyzer]; found {
			return
		}
		visited[analyzer] = struct{}{}
		requires := slices.Clone(analyzer.Requires)
		sort.Slice(requires, func(left, right int) bool {
			return requires[left].Name < requires[right].Name
		})
		for _, required := range requires {
			visit(required)
		}
		plan = append(plan, analyzer)
	}
	visit(root)
	return plan
}

func (r *analyzerRule) Metadata() rules.Metadata { return cloneAnalyzerMetadata(r.metadata) }

func (r *packageAnalyzerRule) Metadata() rules.Metadata {
	return cloneAnalyzerMetadata(r.metadata)
}

func (r *analyzerRule) RunSyntaxFile(file *source.File) ([]rules.Finding, error) {
	if file == nil {
		return nil, fmt.Errorf("go/analysis adapter requires a source file")
	}
	analyzer := r.analyzer
	findings := make([]rules.Finding, 0)
	err := file.ReadSyntaxView(func(fileSet *token.FileSet, syntax *ast.File) error {
		tokenFile := fileSet.File(syntax.Pos())
		if tokenFile == nil {
			return fmt.Errorf("isolated syntax view has no token file")
		}
		diagnostics := make([]goanalysis.Diagnostic, 0)
		pass := &goanalysis.Pass{
			Analyzer: &analyzer,
			Fset:     fileSet,
			Files:    []*ast.File{syntax},
			Pkg:      types.NewPackage("command-line-arguments", syntax.Name.Name),
			Report: func(diagnostic goanalysis.Diagnostic) {
				diagnostics = append(diagnostics, cloneAnalyzerDiagnostic(diagnostic))
			},
			ResultOf: make(map[*goanalysis.Analyzer]any),
			ReadFile: func(filename string) ([]byte, error) {
				if filepath.Clean(filename) != file.Path() {
					return nil, fmt.Errorf("read file %q: outside the adapted source", filename)
				}
				return file.Bytes(), nil
			},
		}
		result, err := runAnalyzer(&analyzer, pass)
		if err != nil {
			return err
		}
		if result != nil {
			return fmt.Errorf("analyzer returned an unexpected result")
		}
		for _, diagnostic := range diagnostics {
			finding, err := r.finding(file, fileSet, tokenFile, diagnostic)
			if err != nil {
				return err
			}
			findings = append(findings, finding)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}

func (r *analyzerRule) finding(
	file *source.File,
	fileSet *token.FileSet,
	tokenFile *token.File,
	diagnostic goanalysis.Diagnostic,
) (rules.Finding, error) {
	primary, err := analyzerRange(file, fileSet, tokenFile, diagnostic.Pos, diagnostic.End)
	if err != nil {
		return rules.Finding{}, fmt.Errorf("diagnostic range: %w", err)
	}
	messageKey := diagnostic.Category
	if strings.TrimSpace(messageKey) == "" {
		messageKey = r.analyzer.Name
	}
	related := make([]rules.Related, len(diagnostic.Related))
	for index, item := range diagnostic.Related {
		sourceRange, err := analyzerRange(file, fileSet, tokenFile, item.Pos, item.End)
		if err != nil {
			return rules.Finding{}, fmt.Errorf("related range %d: %w", index, err)
		}
		related[index] = rules.Related{Range: sourceRange, Message: item.Message}
	}
	fixes := make([]rules.Fix, len(diagnostic.SuggestedFixes))
	for fixIndex, suggested := range diagnostic.SuggestedFixes {
		mapped, found := r.fixes[suggested.Message]
		if !found {
			return rules.Finding{}, fmt.Errorf("undeclared suggested fix %q", suggested.Message)
		}
		edits := make([]rules.Edit, len(suggested.TextEdits))
		for editIndex, edit := range suggested.TextEdits {
			sourceRange, err := analyzerRange(file, fileSet, tokenFile, edit.Pos, edit.End)
			if err != nil {
				return rules.Finding{}, fmt.Errorf("suggested fix %q edit %d: %w", suggested.Message, editIndex, err)
			}
			edits[editIndex] = rules.Edit{Range: sourceRange, NewText: string(edit.NewText)}
		}
		fixes[fixIndex] = rules.Fix{Name: mapped.name, Safety: mapped.safety, Edits: edits}
	}
	help, err := analyzerDiagnosticURL(&r.analyzer, diagnostic)
	if err != nil {
		return rules.Finding{}, err
	}
	return rules.Finding{
		MessageKey: messageKey,
		Message:    diagnostic.Message,
		Range:      primary,
		Related:    related,
		Help:       help,
		Fixes:      fixes,
	}, nil
}

func analyzerDiagnosticURL(
	analyzer *goanalysis.Analyzer,
	diagnostic goanalysis.Diagnostic,
) (string, error) {
	if analyzer.URL == "" && diagnostic.URL == "" && diagnostic.Category == "" {
		return "", nil
	}
	raw := diagnostic.URL
	if raw == "" && diagnostic.Category != "" {
		raw = "#" + diagnostic.Category
	}
	diagnosticURL, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid diagnostic URL %q: %w", raw, err)
	}
	baseURL, err := url.Parse(analyzer.URL)
	if err != nil {
		return "", fmt.Errorf("invalid analyzer URL %q: %w", analyzer.URL, err)
	}
	return baseURL.ResolveReference(diagnosticURL).String(), nil
}

func analyzerRange(
	file *source.File,
	fileSet *token.FileSet,
	tokenFile *token.File,
	start, end token.Pos,
) (source.Range, error) {
	if !start.IsValid() {
		return source.Range{}, fmt.Errorf("position is invalid")
	}
	if !end.IsValid() {
		end = start
	}
	if fileSet.File(start) != tokenFile || fileSet.File(end) != tokenFile {
		return source.Range{}, fmt.Errorf("position is outside the adapted source")
	}
	result := source.Range{Start: tokenFile.Offset(start), End: tokenFile.Offset(end)}
	if _, valid := file.Slice(result); !valid {
		return source.Range{}, fmt.Errorf("position maps to an invalid physical range")
	}
	return result, nil
}

func runAnalyzer(analyzer *goanalysis.Analyzer, pass *goanalysis.Pass) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("analyzer panicked: %v", recovered)
		}
	}()
	return analyzer.Run(pass)
}

func cloneAnalyzerDiagnostic(diagnostic goanalysis.Diagnostic) goanalysis.Diagnostic {
	result := diagnostic
	result.Related = slices.Clone(diagnostic.Related)
	result.SuggestedFixes = make([]goanalysis.SuggestedFix, len(diagnostic.SuggestedFixes))
	for index, fix := range diagnostic.SuggestedFixes {
		result.SuggestedFixes[index] = fix
		result.SuggestedFixes[index].TextEdits = make([]goanalysis.TextEdit, len(fix.TextEdits))
		for editIndex, edit := range fix.TextEdits {
			result.SuggestedFixes[index].TextEdits[editIndex] = edit
			result.SuggestedFixes[index].TextEdits[editIndex].NewText = slices.Clone(edit.NewText)
		}
	}
	return result
}

func cloneAnalyzerMetadata(metadata rules.Metadata) rules.Metadata {
	result := metadata
	result.Presets = slices.Clone(metadata.Presets)
	result.NodeInterests = slices.Clone(metadata.NodeInterests)
	result.Categories = slices.Clone(metadata.Categories)
	result.Fixes = slices.Clone(metadata.Fixes)
	result.Options = slices.Clone(metadata.Options)
	result.KnownLimitations = slices.Clone(metadata.KnownLimitations)
	result.Examples = slices.Clone(metadata.Examples)
	if metadata.Deprecation != nil {
		deprecation := *metadata.Deprecation
		result.Deprecation = &deprecation
	}
	return result
}

var _ rules.SyntaxFileRule = (*analyzerRule)(nil)
