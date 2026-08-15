package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/config"
	fixengine "github.com/faustbrian/glippy/internal/fix"
	glippyformat "github.com/faustbrian/glippy/internal/format"
	"github.com/faustbrian/glippy/internal/goversion"
	"github.com/faustbrian/glippy/internal/lsp"
	glippyreport "github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

const lspUsage = "glippy: expected 'lsp [--fix-suggestions] [--fix-unsafe] [--config=<path>]'\n"

const lintRuleDocumentationURL = "https://github.com/faustbrian/gox/blob/main/docs/lint-rules.md#"

type lspInvocation struct {
	configPath string
	fixSuggestions bool
	fixUnsafe bool
}

type lspAnalysisState struct {
	task lintTask
	packageTask *lintPackageTask
	result analysis.Result
}

type lspBackend struct {
	registry *rules.Registry
	invocation lspInvocation
}

func parseLSPInvocation(arguments []string) (lspInvocation, bool) {
	if len(arguments) == 0 || arguments[0] != "lsp" {
		return lspInvocation{}, false
	}
	result := lspInvocation{}
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--fix-suggestions" && !result.fixSuggestions:
			result.fixSuggestions = true
		case argument == "--fix-unsafe" && !result.fixUnsafe:
			result.fixUnsafe = true
		case strings.HasPrefix(argument, "--config=") && result.configPath == "":
			result.configPath = strings.TrimPrefix(argument, "--config=")
			if result.configPath == "" {
				return lspInvocation{}, false
			}
		case argument == "--config" &&
			result.configPath == "" &&
			index + 1 < len(arguments) &&
			!strings.HasPrefix(arguments[index + 1], "--"):
			index++
			result.configPath = arguments[index]
		default:
			return lspInvocation{}, false
		}
	}
	return result, true
}

func runLSP(
	ctx context.Context,
	invocation lspInvocation,
	stdin io.Reader,
	stdout, stderr io.Writer,
	registry *rules.Registry,
) int {
	err := lsp.Serve(
		ctx,
		stdin,
		stdout,
		&lspBackend{registry: registry, invocation: invocation},
	)
	if err == nil {
		return ExitSuccess
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return report(stderr, ExitCanceled, "glippy lsp: %v\n", err)
	}
	return report(stderr, ExitInternalError, "glippy lsp: %v\n", err)
}

func (b *lspBackend) Analyze(ctx context.Context, document lsp.Document) (lsp.Analysis, error) {
	file, err := source.Load(document.Path, document.Text)
	if err != nil {
		return lsp.Analysis{}, err
	}
	task, requirement, err := b.task(document.Path)
	if err != nil {
		return lsp.Analysis{}, err
	}
	if requirement < rules.RequireTypes {
		result, err := analysis.Run(ctx, file, b.registry, task.options.analysis)
		if err != nil {
			return lsp.Analysis{}, err
		}
		inputs := []glippyreport.LintTextInput{{File: file, Result: result}}
		results := []analysis.Result{result}
		if err := applyConfiguredBaselines([]lintTask{task}, inputs, results, b.registry);
			err != nil {
			return lsp.Analysis{}, err
		}
		return editorAnalysis(
			file,
			results[0],
			&lspAnalysisState{task: task, result: results[0]},
		), nil
	}
	if task.root == "" {
		return lsp.Analysis{}, errors.New(
			"typed editor diagnostics require a module, workspace, or repository root",
		)
	}
	packageTask := lintPackageTask{
		root: task.root,
		patterns: []string{"file=" + document.Path},
		options: task.options,
	}
	packageResult, err := runPackageAnalysisWithOverlay(
		ctx,
		b.registry,
		packageTask,
		map[string][]byte{document.Path: slices.Clone(document.Text)},
	)
	if err != nil {
		return lsp.Analysis{}, err
	}
	if err := applyConfiguredPackageBaseline(packageTask, &packageResult, b.registry);
		err != nil {
		return lsp.Analysis{}, err
	}
	if err := validateLintPackagePrerequisites(packageResult); err != nil {
		return lsp.Analysis{}, err
	}
	for _, result := range packageResult.Files {
		if result.Path != document.Path {
			continue
		}
		bound, found := packageResult.Sources.Lookup(result.Path)
		if !found || bound.Digest() != file.Digest() {
			return lsp.Analysis{}, errors.New(
				"typed editor analysis does not match the document source",
			)
		}
		return editorAnalysis(
			bound,
			result,
			&lspAnalysisState{task: task, packageTask: &packageTask, result: result},
		), nil
	}
	return lsp.Analysis{}, fmt.Errorf("typed editor analysis did not return %q", document.Path)
}

func (b *lspBackend) CodeActions(
	ctx context.Context,
	document lsp.Document,
	analyzed lsp.Analysis,
	requested source.Range,
) ([]lsp.CodeAction, error) {
	state, valid := analyzed.State.(*lspAnalysisState)
	if !valid || state == nil || analyzed.File == nil {
		return nil, errors.New("editor analysis state is unavailable")
	}
	if analyzed.File.Path() != document.Path ||
		!bytes.Equal(analyzed.File.Bytes(), document.Text) {
		return nil, errors.New("editor analysis source is stale")
	}
	if analyzed.File.Metadata().Generated {
		return []lsp.CodeAction{}, nil
	}
	actions := make([]lsp.CodeAction, 0)
	for _, diagnostic := range state.result.Diagnostics {
		if !rangesIntersect(diagnostic.Range, requested) {
			continue
		}
		for _, offered := range diagnostic.Fixes {
			if !b.authorized(offered.Safety) {
				continue
			}
			coordinated, err := b.coordinateEditorFixes(
				ctx,
				analyzed.File,
				state,
				[]fixengine.Selection{
					{Diagnostic: diagnostic, FixName: offered.Name},
				},
				b.invocation.fixSuggestions,
				b.invocation.fixUnsafe,
			)
			if err != nil {
				return nil, err
			}
			if len(coordinated.Applied) != 1 ||
				len(coordinated.Rejected) != 0 ||
				bytes.Equal(coordinated.Bytes, analyzed.File.Bytes()) {
				continue
			}
			actions = append(
				actions,
				lsp.CodeAction{
					Title: fmt.Sprintf(
						"%s: %s [%s]",
						diagnostic.RuleID,
						offered.Name,
						offered.Safety,
					),
					Kind: "quickfix",
					Preferred: offered.Safety == rules.FixSafe,
					DiagnosticCode: diagnostic.RuleID,
					DiagnosticRange: diagnostic.Range,
					DiagnosticSeverity: editorSeverity(diagnostic.Severity),
					DiagnosticMessage: diagnostic.Message,
					DiagnosticDocumentationURI: lintRuleDocumentationURL +
						diagnostic.RuleID,
					DiagnosticWithheldFixes: editorWithheldFixes(
						diagnostic.WithheldFixes,
					),
					NewText: coordinated.Bytes,
				},
			)
		}
	}
	safeSelections, err := fixengine.SelectSafe(state.result.Diagnostics)
	if err != nil {
		safeSelections = nil
	}
	if len(safeSelections) > 0 {
		coordinated, err := b.coordinateEditorFixes(
			ctx,
			analyzed.File,
			state,
			safeSelections,
			false,
			false,
		)
		if err != nil {
			return nil, err
		}
		if len(coordinated.Applied) == len(safeSelections) &&
			len(coordinated.Rejected) == 0 &&
			!bytes.Equal(coordinated.Bytes, analyzed.File.Bytes()) {
			actions = append(
				actions,
				lsp.CodeAction{
					Title: "Fix all safe Glippy findings",
					Kind: "source.fixAll.glippy",
					Preferred: true,
					NewText: coordinated.Bytes,
				},
			)
		}
	}
	return actions, nil
}

func (b *lspBackend) coordinateEditorFixes(
	ctx context.Context,
	file *source.File,
	state *lspAnalysisState,
	selections []fixengine.Selection,
	allowSuggestion bool,
	allowUnsafe bool,
) (fixengine.Result, error) {
	options := fixengine.Options{
		AllowSuggestion: allowSuggestion,
		AllowUnsafe: allowUnsafe,
		Format: state.task.options.format,
	}
	options.Validate = func(formatted *source.File) error {
		if state.packageTask != nil {
			_, _, err := validateLintPackageFix(
				ctx,
				b.registry,
				*state.packageTask,
				formatted,
				nil,
			)
			return err
		}
		result, err := analysis.Run(ctx, formatted, b.registry, state.task.options.analysis)
		if err != nil {
			return err
		}
		inputs := []glippyreport.LintTextInput{{File: formatted, Result: result}}
		results := []analysis.Result{result}
		return applyConfiguredBaselines([]lintTask{state.task}, inputs, results, b.registry)
	}
	return fixengine.Coordinate(file, selections, options)
}

func (b *lspBackend) Format(ctx context.Context, document lsp.Document) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	selection, err := config.DiscoverFileContext(document.Path, b.invocation.configPath)
	if err != nil {
		return nil, err
	}
	if _, err := goversion.Resolve(document.Path, selection.Root); err != nil {
		return nil, err
	}
	options, _, err := formatOptionsForSelection(selection)
	if err != nil {
		return nil, err
	}
	file, err := source.Load(document.Path, document.Text)
	if err != nil {
		return nil, err
	}
	formatted, err := glippyformat.File(file, options)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return formatted, nil
}

func (b *lspBackend) task(path string) (lintTask, rules.Requirement, error) {
	selection, err := config.DiscoverFileContext(path, b.invocation.configPath)
	if err != nil {
		return lintTask{}, 0, err
	}
	language, err := goversion.Resolve(path, selection.Root)
	if err != nil {
		return lintTask{}, 0, err
	}
	options, _, err := lintOptionsForSelection(
		selection,
		language.Language,
		b.registry,
		nil,
		nil,
		nil,
	)
	if err != nil {
		return lintTask{}, 0, err
	}
	resolution, err := options.analysis.RuleResolution()
	if err != nil {
		return lintTask{}, 0, err
	}
	selected, err := b.registry.ResolveOptions(resolution)
	if err != nil {
		return lintTask{}, 0, err
	}
	return lintTask{
		root: selection.Root,
		options: options,
	}, rules.MaximumRequirement(selected), nil
}

func (b *lspBackend) authorized(safety rules.FixSafety) bool {
	switch safety {
	case rules.FixSafe:
		return true
	case rules.FixSuggestion:
		return b.invocation.fixSuggestions
	case rules.FixUnsafe:
		return b.invocation.fixUnsafe
	default:
		return false
	}
}

func editorAnalysis(
	file *source.File,
	result analysis.Result,
	state *lspAnalysisState,
) lsp.Analysis {
	diagnostics := make([]lsp.Diagnostic, 0, len(result.Diagnostics))
	for _, diagnostic := range analysis.OrderDiagnostics(result.Diagnostics) {
		related := make([]lsp.Related, len(diagnostic.Related))
		for index, item := range diagnostic.Related {
			related[index] = lsp.Related{Range: item.Range, Message: item.Message}
		}
		severity := editorSeverity(diagnostic.Severity)
		diagnostics = append(
			diagnostics,
			lsp.Diagnostic{
				Range: diagnostic.Range,
				Severity: severity,
				Code: diagnostic.RuleID,
				DocumentationURI: lintRuleDocumentationURL + diagnostic.RuleID,
				Message: diagnostic.Message,
				Related: related,
				WithheldFixes: editorWithheldFixes(diagnostic.WithheldFixes),
			},
		)
	}
	for _, problem := range result.SuppressionProblems {
		diagnostics = append(
			diagnostics,
			lsp.Diagnostic{
				Range: problem.Range,
				Severity: lsp.SeverityWarning,
				Code: "suppression:" + string(problem.Kind),
				Message: problem.Message,
			},
		)
	}
	for _, directive := range result.UnusedSuppressions {
		diagnostics = append(
			diagnostics,
			lsp.Diagnostic{
				Range: directive.Range,
				Severity: lsp.SeverityWarning,
				Code: "suppression:unused",
				Message: "unused suppression for " + directive.RuleID,
			},
		)
	}
	for _, problem := range result.BaselineProblems {
		diagnostics = append(
			diagnostics,
			lsp.Diagnostic{
				Range: source.Range{},
				Severity: lsp.SeverityWarning,
				Code: "baseline:" + string(problem.Kind),
				Message: fmt.Sprintf(
					"%s/%s has %d unmatched occurrence(s)",
					problem.Entry.RuleID,
					problem.Entry.MessageKey,
					problem.Remaining,
				),
			},
		)
	}
	return lsp.Analysis{File: file, Diagnostics: diagnostics, State: state}
}

func editorWithheldFixes(fixes []rules.WithheldFix) []lsp.WithheldFix {
	result := make([]lsp.WithheldFix, len(fixes))
	for index, fix := range fixes {
		result[index] = lsp.WithheldFix{
			Name: fix.Name,
			Reason: string(fix.Reason),
			Message: fix.Message,
		}
	}
	return result
}

func editorSeverity(severity rules.Severity) lsp.Severity {
	if severity == rules.SeverityError {
		return lsp.SeverityError
	}
	return lsp.SeverityWarning
}

func rangesIntersect(left, right source.Range) bool {
	if left.Start == left.End {
		return left.Start >= right.Start && left.Start <= right.End
	}
	if right.Start == right.End {
		return right.Start >= left.Start && right.Start <= left.End
	}
	return left.Start < right.End && right.Start < left.End
}
