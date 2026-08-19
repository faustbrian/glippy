package analysis

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	goanalysis "golang.org/x/tools/go/analysis"

	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

// AnalyzerExternalFactExecution declares one audited exact external execution
// mode for a fact-bearing analyzer.
type AnalyzerExternalFactExecution struct {
	Identity string
	Analyzer string
	Audited bool
}

// PackageFactAnalyzerRequest describes one exact external analyzer execution.
type PackageFactAnalyzerRequest struct {
	Identity string
	Analyzer string
	PackageErrors bool
	LoadOptions PackageLoadOptions
	Sources PackageSourceSet
}

// PackageFactAnalyzerSuggestedFix is one external analyzer suggestion.
type PackageFactAnalyzerSuggestedFix struct {
	Message string
	Edits []PackageFactAnalyzerEdit
}

// PackageFactAnalyzerEdit is one exact source edit from an external analyzer.
type PackageFactAnalyzerEdit struct {
	Path string
	Range source.Range
	NewText string
}

// PackageFactAnalyzerDiagnostic is one external analyzer diagnostic over an
// exact retained source version.
type PackageFactAnalyzerDiagnostic struct {
	Analyzer string
	Path string
	Range source.Range
	Message string
	SuggestedFixes []PackageFactAnalyzerSuggestedFix
}

// PackageFactAnalyzerRunner executes one audited fact-bearing analyzer without
// retaining its complete dependency type graph in the Glippy process.
type PackageFactAnalyzerRunner interface {
	RunPackageFactAnalyzer(
		context.Context,
		PackageFactAnalyzerRequest,
	) ([]PackageFactAnalyzerDiagnostic, error)
}

type packageFactAnalyzerRunnerContextKey struct{}

// WithPackageFactAnalyzerRunner installs one run-owned external analyzer
// boundary. A nil runner leaves the existing in-process adapter active.
func WithPackageFactAnalyzerRunner(
	ctx context.Context,
	runner PackageFactAnalyzerRunner,
) context.Context {
	if ctx == nil {
		panic("package fact analyzer runner requires a context")
	}
	if runner == nil {
		return ctx
	}
	return context.WithValue(ctx, packageFactAnalyzerRunnerContextKey{}, runner)
}

func packageFactAnalyzerRunnerFromContext(ctx context.Context) PackageFactAnalyzerRunner {
	if ctx == nil {
		return nil
	}
	runner, _ := ctx.Value(packageFactAnalyzerRunnerContextKey{}).(PackageFactAnalyzerRunner)
	return runner
}

func (r *packageAnalyzerRule) runExternalFactAnalyzer(
	ctx context.Context,
	runner PackageFactAnalyzerRunner,
	retainedSources PackageSourceSet,
	loadOptions PackageLoadOptions,
	metadata rules.Metadata,
	severity rules.Severity,
	packageErrors bool,
) ([]rules.Diagnostic, error) {
	if r == nil || r.externalFactExecution == nil {
		return nil, fmt.Errorf("external package fact analyzer is not configured")
	}
	upstream, err := runner.RunPackageFactAnalyzer(
		ctx,
		PackageFactAnalyzerRequest{
			Identity: r.externalFactExecution.Identity,
			Analyzer: r.externalFactExecution.Analyzer,
			PackageErrors: packageErrors,
			LoadOptions: clonePackageLoadOptions(loadOptions),
			Sources: retainedSources,
		},
	)
	if err != nil {
		return nil, err
	}
	diagnostics := make([]rules.Diagnostic, 0, len(upstream))
	for index, diagnostic := range upstream {
		if diagnostic.Analyzer != r.externalFactExecution.Analyzer {
			return nil, fmt.Errorf(
				"external diagnostic %d belongs to analyzer %q; want %q",
				index,
				diagnostic.Analyzer,
				r.externalFactExecution.Analyzer,
			)
		}
		path := filepath.Clean(diagnostic.Path)
		if path == "" || !filepath.IsAbs(path) || path != diagnostic.Path {
			return nil, fmt.Errorf(
				"external diagnostic %d has invalid source path %q",
				index,
				diagnostic.Path,
			)
		}
		file, found := retainedSources.Lookup(path)
		if !found {
			continue
		}
		if strings.TrimSpace(diagnostic.Message) == "" {
			return nil, fmt.Errorf("external diagnostic %d has an empty message", index)
		}
		if _, valid := file.Slice(diagnostic.Range); !valid {
			return nil, fmt.Errorf(
				"external diagnostic %d maps to an invalid physical range",
				index,
			)
		}
		if file.Metadata().Generated && !metadata.RunOnGenerated {
			continue
		}
		fixes := make([]rules.Fix, len(diagnostic.SuggestedFixes))
		for fixIndex, suggested := range diagnostic.SuggestedFixes {
			mapped, found := r.fixes[suggested.Message]
			if !found {
				return nil, fmt.Errorf(
					"undeclared suggested fix %q",
					suggested.Message,
				)
			}
			edits := make([]rules.Edit, len(suggested.Edits))
			for editIndex, edit := range suggested.Edits {
				if filepath.Clean(edit.Path) != path {
					return nil, fmt.Errorf(
						"suggested fix %q edit %d belongs to another source file",
						suggested.Message,
						editIndex,
					)
				}
				if _, valid := file.Slice(edit.Range); !valid {
					return nil, fmt.Errorf(
						"suggested fix %q edit %d maps to an invalid physical range",
						suggested.Message,
						editIndex,
					)
				}
				edits[editIndex] = rules.Edit{
					Range: edit.Range,
					NewText: edit.NewText,
				}
			}
			fixes[fixIndex] = rules.Fix{
				Name: mapped.name,
				Safety: mapped.safety,
				Edits: edits,
				RequiredImports: slices.Clone(mapped.requiredImports),
			}
		}
		help, err := analyzerDiagnosticURL(&r.analyzer, goanalysis.Diagnostic{})
		if err != nil {
			return nil, err
		}
		mapped, err := diagnosticForFinding(
			file,
			metadata,
			severity,
			rules.Finding{
				MessageKey: diagnostic.Analyzer,
				Message: diagnostic.Message,
				Range: diagnostic.Range,
				Help: help,
				Fixes: fixes,
			},
		)
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, mapped)
	}
	return OrderDiagnostics(diagnostics), nil
}
