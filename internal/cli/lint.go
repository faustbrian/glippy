package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/cache"
	"github.com/faustbrian/gox/internal/config"
	"github.com/faustbrian/gox/internal/discovery"
	"github.com/faustbrian/gox/internal/filesystem"
	fixengine "github.com/faustbrian/gox/internal/fix"
	goxformat "github.com/faustbrian/gox/internal/format"
	"github.com/faustbrian/gox/internal/goversion"
	goxreport "github.com/faustbrian/gox/internal/report"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

const lintUsage = "gox: expected 'lint [--fix] [--fix-suggestions] [--fix-unsafe] [--reporter=text|json] [--config=<path>] [path...]'\n"

type lintInvocation struct {
	configPath string
	fix bool
	fixSuggestions bool
	fixUnsafe bool
	paths []string
	reporter goxreport.Format
}

type lintTaskOptions struct {
	analysis analysis.RunOptions
	buildSelection config.Analysis
	format goxformat.Options
	cache config.Cache
	configurationDigest cache.Digest
	sourceGoVersion string
}

type lintTask struct {
	file discovery.File
	root string
	options lintTaskOptions
}

type lintPackageTask struct {
	root string
	patterns []string
	options lintTaskOptions
}

type lintInputPlan struct {
	input string
	anchor string
	pattern bool
	selection config.Selection
	options lintTaskOptions
	requirement rules.Requirement
}

type lintPackageValidationError struct {
	err error
}

func (e *lintPackageValidationError) Error() string {
	return e.err.Error()
}

func (e *lintPackageValidationError) Unwrap() error {
	return e.err
}

type lintFixExecution struct {
	file *source.File
	resultFile *source.File
	result analysis.Result
	outcome goxreport.LintFixOutcome
	selections []fixengine.Selection
	task lintTask
	packageTask *lintPackageTask
	snapshot *filesystem.Snapshot
}

func parseLintInvocation(arguments []string) (lintInvocation, bool) {
	if len(arguments) == 0 || arguments[0] != "lint" {
		return lintInvocation{}, false
	}
	result := lintInvocation{reporter: goxreport.Text}
	reporterSet := false
	fixSet := false
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--fix" && !fixSet:
			result.fix = true
			fixSet = true
		case argument == "--fix-suggestions" && !result.fixSuggestions:
			result.fixSuggestions = true
		case argument == "--fix-unsafe" && !result.fixUnsafe:
			result.fixUnsafe = true
		case strings.HasPrefix(argument, "--reporter=") && !reporterSet:
			reporter, valid := parseReporter(
				strings.TrimPrefix(argument, "--reporter="),
			)
			if !valid {
				return lintInvocation{}, false
			}
			result.reporter = reporter
			reporterSet = true
		case argument == "--reporter" &&
			!reporterSet &&
			index + 1 < len(arguments) &&
			!strings.HasPrefix(arguments[index + 1], "--"):
			index++
			reporter, valid := parseReporter(arguments[index])
			if !valid {
				return lintInvocation{}, false
			}
			result.reporter = reporter
			reporterSet = true
		case strings.HasPrefix(argument, "--config=") && result.configPath == "":
			result.configPath = strings.TrimPrefix(argument, "--config=")
			if result.configPath == "" {
				return lintInvocation{}, false
			}
		case argument == "--config" &&
			result.configPath == "" &&
			index + 1 < len(arguments) &&
			!strings.HasPrefix(arguments[index + 1], "--"):
			index++
			result.configPath = arguments[index]
			if result.configPath == "" {
				return lintInvocation{}, false
			}
		case !strings.HasPrefix(argument, "-"):
			result.paths = append(result.paths, argument)
		default:
			return lintInvocation{}, false
		}
	}
	if len(result.paths) == 0 {
		result.paths = []string{"."}
	}
	return result, true
}

func (invocation lintInvocation) fixEnabled() bool {
	return invocation.fix || invocation.fixSuggestions || invocation.fixUnsafe
}

func (invocation lintInvocation) selectionOptions() fixengine.SelectionOptions {
	return fixengine.SelectionOptions{
		AllowSafe: invocation.fix,
		AllowSuggestion: invocation.fixSuggestions,
		AllowUnsafe: invocation.fixUnsafe,
	}
}

func requestsLintJSONReporter(arguments []string) bool {
	if len(arguments) == 0 || arguments[0] != "lint" {
		return false
	}
	for index, argument := range arguments {
		if argument == "--reporter=json" ||
			(argument == "--reporter" &&
				index + 1 < len(arguments) &&
				arguments[index + 1] == "json") {
			return true
		}
	}
	return false
}

func runLintCheck(
	ctx context.Context,
	invocation lintInvocation,
	stdout, stderr io.Writer,
	registry *rules.Registry,
) int {
	if ctx == nil {
		return reportLintFailure(
			invocation,
			stdout,
			stderr,
			ExitInternalError,
			nil,
			errors.New("context is required"),
		)
	}
	if registry == nil {
		return reportLintFailure(
			invocation,
			stdout,
			stderr,
			ExitInternalError,
			nil,
			errors.New("rule registry is required"),
		)
	}
	if err := ctx.Err(); err != nil {
		return reportLintFailure(invocation, stdout, stderr, ExitCanceled, nil, err)
	}
	plans, exitCode, err := prepareLintInputPlans(ctx, invocation, registry)
	if err != nil {
		return reportLintFailure(invocation, stdout, stderr, exitCode, nil, err)
	}
	packageTask, packageMode, exitCode, err := prepareLintPackageTask(plans)
	if err != nil {
		return reportLintFailure(invocation, stdout, stderr, exitCode, nil, err)
	}
	if packageMode {
		return runLintPackageCheck(ctx, invocation, stdout, stderr, registry, packageTask)
	}
	tasks, exitCode, err := prepareLintTasksFromPlans(
		ctx,
		plans,
		invocation.configPath,
		registry,
	)
	if err != nil {
		return reportLintFailure(invocation, stdout, stderr, exitCode, nil, err)
	}

	inputs := make([]goxreport.LintTextInput, 0, len(tasks))
	results := make([]analysis.Result, 0, len(tasks))
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			return reportLintFailure(
				invocation,
				stdout,
				stderr,
				ExitCanceled,
				results,
				err,
			)
		}
		input, err := source.ReadFile(task.file.Path)
		if err != nil {
			return reportLintFailure(
				invocation,
				stdout,
				stderr,
				exitCodeForError(ExitFilesystemError, err),
				results,
				fmt.Errorf("read %q: %w", task.file.Path, err),
			)
		}
		file, err := source.Load(task.file.Path, input)
		if err != nil {
			return reportLintFailure(
				invocation,
				stdout,
				stderr,
				ExitSourceError,
				results,
				err,
			)
		}
		analyzed, err := analysis.Run(ctx, file, registry, task.options.analysis)
		if err != nil {
			return reportLintFailure(
				invocation,
				stdout,
				stderr,
				exitCodeForError(ExitInternalError, err),
				results,
				err,
			)
		}
		results = append(results, analyzed)
		inputs = append(inputs, goxreport.LintTextInput{File: file, Result: analyzed})
	}
	if err := ctx.Err(); err != nil {
		return reportLintFailure(invocation, stdout, stderr, ExitCanceled, results, err)
	}
	exitCode = lintResultExitCode(results)
	if invocation.reporter == goxreport.JSON {
		return reportLintJSON(stdout, stderr, "check", exitCode, true, results, nil)
	}
	output, err := goxreport.RenderLintText(inputs)
	if err != nil {
		return report(stderr, ExitInternalError, "gox lint: render text report: %v\n", err)
	}
	if len(output) > 0 {
		if err := write(stdout, output); err != nil {
			return report(
				stderr,
				ExitFilesystemError,
				"gox lint: write standard output: %v\n",
				err,
			)
		}
	}
	return exitCode
}

func runLintPackageCheck(
	ctx context.Context,
	invocation lintInvocation,
	stdout, stderr io.Writer,
	registry *rules.Registry,
	task lintPackageTask,
) int {
	result, err := runPackageAnalysis(ctx, registry, task)
	if err != nil {
		return reportLintPackageFailure(
			invocation,
			stdout,
			stderr,
			packageAnalysisErrorExitCode(err),
			result,
			err,
		)
	}
	if err := ctx.Err(); err != nil {
		return reportLintPackageFailure(
			invocation,
			stdout,
			stderr,
			ExitCanceled,
			result,
			err,
		)
	}
	exitCode := lintPackageResultExitCode(result)
	if invocation.reporter == goxreport.JSON {
		return reportLintPackageJSON(stdout, stderr, "check", exitCode, true, result, nil)
	}
	inputs, err := packageLintTextInputs(result)
	if err != nil {
		return report(
			stderr,
			ExitInternalError,
			"gox lint: prepare typed text report: %v\n",
			err,
		)
	}
	output, err := goxreport.RenderPackageLintText(
		inputs,
		result.LoadDiagnostics,
		result.SourceProblems,
	)
	if err != nil {
		return report(
			stderr,
			ExitInternalError,
			"gox lint: render typed text report: %v\n",
			err,
		)
	}
	if len(output) > 0 {
		if err := write(stdout, output); err != nil {
			return report(
				stderr,
				ExitFilesystemError,
				"gox lint: write standard output: %v\n",
				err,
			)
		}
	}
	return exitCode
}

func prepareLintInputPlans(
	ctx context.Context,
	invocation lintInvocation,
	registry *rules.Registry,
) ([]lintInputPlan, int, error) {
	type resolvedOptions struct {
		options lintTaskOptions
		requirement rules.Requirement
	}
	inputs := invocation.paths
	if len(inputs) == 0 {
		inputs = []string{"."}
	}
	plans := make([]lintInputPlan, 0, len(inputs))
	optionsByConfiguration := make(map[string]resolvedOptions)
	for _, input := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, ExitCanceled, err
		}
		anchor, pattern, err := lintInputAnchor(input)
		if err != nil {
			return nil, ExitInvalidInvocation, err
		}
		selection, err := config.Discover(anchor, invocation.configPath)
		if err != nil {
			return nil, ExitFilesystemError, err
		}
		language, err := goversion.Resolve(anchor, selection.Root)
		if err != nil {
			return nil, sourceVersionErrorExitCode(err), err
		}
		optionsKey := selection.Path + "\x00" + language.Language
		resolved, found := optionsByConfiguration[optionsKey]
		if !found {
			options, exitCode, err := lintOptionsForSelection(
				selection,
				language.Language,
				registry,
			)
			if err != nil {
				return nil, exitCode, err
			}
			selected, err := registry.ResolveConfiguredForGoVersion(
				options.analysis.Preset,
				options.analysis.Overrides,
				options.analysis.RuleOptions,
				options.sourceGoVersion,
			)
			if err != nil {
				return nil, ExitInvalidInvocation, err
			}
			resolved = resolvedOptions{
				options: options,
				requirement: rules.MaximumRequirement(selected),
			}
			optionsByConfiguration[optionsKey] = resolved
		}
		plans = append(
			plans,
			lintInputPlan{
				input: input,
				anchor: anchor,
				pattern: pattern,
				selection: selection,
				options: resolved.options,
				requirement: resolved.requirement,
			},
		)
	}
	return plans, ExitSuccess, nil
}

func prepareLintPackageTask(plans []lintInputPlan) (lintPackageTask, bool, int, error) {
	typed := false
	for _, plan := range plans {
		typed = typed || plan.requirement >= rules.RequireTypes
	}
	if !typed {
		return lintPackageTask{}, false, ExitSuccess, nil
	}
	if len(plans) == 0 {
		return lintPackageTask{}, false, ExitInternalError, errors.New(
			"typed lint planning produced no inputs",
		)
	}
	first := plans[0]
	if first.selection.Root == "" {
		return lintPackageTask{}, false, ExitInvalidInvocation, errors.New(
			"typed lint requires a module, workspace, or repository root",
		)
	}
	patterns := make([]string, 0, len(plans))
	for _, plan := range plans {
		if plan.requirement < rules.RequireTypes ||
			plan.selection.Root != first.selection.Root ||
			plan.selection.Path != first.selection.Path {
			return lintPackageTask{}, false, ExitInvalidInvocation, errors.New(
				"typed lint inputs must resolve to one project root and configuration",
			)
		}
		if plan.options.sourceGoVersion != first.options.sourceGoVersion {
			return lintPackageTask{}, false, ExitInvalidInvocation, errors.New(
				"typed lint inputs must resolve to one source Go version",
			)
		}
		pattern, exitCode, err := lintPackageQuery(
			plan.input,
			plan.anchor,
			plan.pattern,
			first.selection.Root,
		)
		if err != nil {
			return lintPackageTask{}, false, exitCode, err
		}
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	patterns = slices.Compact(patterns)
	return lintPackageTask{
		root: first.selection.Root,
		patterns: patterns,
		options: first.options,
	}, true, ExitSuccess, nil
}

func lintInputAnchor(input string) (string, bool, error) {
	cleaned := filepath.Clean(input)
	if filepath.Base(cleaned) == "..." {
		return filepath.Dir(cleaned), true, nil
	}
	if strings.Contains(input, "...") {
		return "", false, fmt.Errorf("invalid package pattern %q", input)
	}
	return input, false, nil
}

func lintPackageQuery(input, anchor string, recursive bool, root string) (string, int, error) {
	absolute, err := filepath.Abs(anchor)
	if err != nil {
		return "", ExitFilesystemError, fmt.Errorf("resolve lint input %q: %w", input, err)
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil ||
		relative == ".." ||
		strings.HasPrefix(relative, ".." + string(filepath.Separator)) {
		return "", ExitInvalidInvocation, fmt.Errorf(
			"typed lint input %q is outside project root %q",
			input,
			root,
		)
	}
	if recursive {
		if relative == "." {
			return "./...", ExitSuccess, nil
		}
		return "./" + filepath.ToSlash(relative) + "/...", ExitSuccess, nil
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", ExitFilesystemError, fmt.Errorf(
			"inspect lint input %q: %w",
			absolute,
			err,
		)
	}
	if !info.IsDir() {
		return "file=" + absolute, ExitSuccess, nil
	}
	if relative == "." {
		return ".", ExitSuccess, nil
	}
	return "./" + filepath.ToSlash(relative), ExitSuccess, nil
}

func packageLintTextInputs(result analysis.PackageResult) ([]goxreport.LintTextInput, error) {
	inputs := make([]goxreport.LintTextInput, 0, len(result.Files))
	for _, fileResult := range result.Files {
		file, found := result.Sources.Lookup(fileResult.Path)
		if !found {
			return nil, fmt.Errorf(
				"typed lint result source %q is missing",
				fileResult.Path,
			)
		}
		inputs = append(inputs, goxreport.LintTextInput{File: file, Result: fileResult})
	}
	return inputs, nil
}

func prepareLintTasks(
	ctx context.Context,
	invocation lintInvocation,
	registry *rules.Registry,
) ([]lintTask, int, error) {
	plans, exitCode, err := prepareLintInputPlans(ctx, invocation, registry)
	if err != nil {
		return nil, exitCode, err
	}
	return prepareLintTasksFromPlans(ctx, plans, invocation.configPath, registry)
}

func prepareLintTasksFromPlans(
	ctx context.Context,
	plans []lintInputPlan,
	configPath string,
	registry *rules.Registry,
) ([]lintTask, int, error) {
	selected := make(map[string]discovery.File)
	optionsByConfiguration := make(map[string]lintTaskOptions)
	for _, plan := range plans {
		if err := ctx.Err(); err != nil {
			return nil, ExitCanceled, err
		}
		optionsByConfiguration[plan.selection.Path +
			"\x00" +
			plan.options.sourceGoVersion] = plan.options
		files, err := discovery.GoFiles(
			ctx,
			[]string{plan.anchor},
			discovery.Options{Root: plan.selection.Root},
		)
		if err != nil {
			return nil, exitCodeForError(ExitFilesystemError, err), err
		}
		for _, file := range files {
			current, exists := selected[file.Path]
			if !exists || file.Explicit || !current.Explicit {
				selected[file.Path] = file
			}
		}
	}
	paths := make([]string, 0, len(selected))
	for path := range selected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	tasks := make([]lintTask, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, ExitCanceled, err
		}
		selection, err := config.Discover(path, configPath)
		if err != nil {
			return nil, ExitFilesystemError, err
		}
		language, err := goversion.Resolve(path, selection.Root)
		if err != nil {
			return nil, sourceVersionErrorExitCode(err), err
		}
		optionsKey := selection.Path + "\x00" + language.Language
		options, exists := optionsByConfiguration[optionsKey]
		if !exists {
			var exitCode int
			options, exitCode, err = lintOptionsForSelection(
				selection,
				language.Language,
				registry,
			)
			if err != nil {
				return nil, exitCode, err
			}
			optionsByConfiguration[optionsKey] = options
		}
		tasks = append(
			tasks,
			lintTask{file: selected[path], root: selection.Root, options: options},
		)
	}
	return tasks, ExitSuccess, nil
}

func lintOptionsForSelection(
	selection config.Selection,
	sourceGoVersion string,
	registry *rules.Registry,
) (lintTaskOptions, int, error) {
	loaded, err := config.Load(
		selection,
		config.ParseOptions{
			KnownRules: registry.IDs(),
			RuleOptions: registry.OptionSchemas(),
		},
	)
	if err != nil {
		return lintTaskOptions{}, configurationErrorExitCode(err), err
	}
	return lintTaskOptions{
		analysis: analysis.RunOptions{
			SourceGoVersion: sourceGoVersion,
			Preset: loaded.Lint.Preset,
			Overrides: loaded.Lint.Rules,
			RuleOptions: loaded.Lint.RuleOptions,
			RequireSuppressionReason: loaded.Lint.Suppressions.RequireReason,
			SuppressionExpiryCutoff: loaded.Lint.Suppressions.ExpiryCutoff,
		},
		buildSelection: loaded.Analysis,
		format: goxformat.Options{
			Width: loaded.Format.LineWidth,
			TabWidth: loaded.Format.TabWidth,
			FitBudget: defaultFormatOptions.FitBudget,
		},
		cache: loaded.Cache,
		configurationDigest: cache.DigestOf(loaded.CanonicalBytes()),
		sourceGoVersion: sourceGoVersion,
	}, ExitSuccess, nil
}

func runLintFix(
	ctx context.Context,
	invocation lintInvocation,
	stdout, stderr io.Writer,
	registry *rules.Registry,
) int {
	if ctx == nil {
		return reportLintFixFailure(
			invocation,
			stdout,
			stderr,
			ExitInternalError,
			nil,
			errors.New("context is required"),
		)
	}
	if registry == nil {
		return reportLintFixFailure(
			invocation,
			stdout,
			stderr,
			ExitInternalError,
			nil,
			errors.New("rule registry is required"),
		)
	}
	if err := ctx.Err(); err != nil {
		return reportLintFixFailure(invocation, stdout, stderr, ExitCanceled, nil, err)
	}
	plans, exitCode, err := prepareLintInputPlans(ctx, invocation, registry)
	if err != nil {
		return reportLintFixFailure(invocation, stdout, stderr, exitCode, nil, err)
	}
	packageTask, packageMode, exitCode, err := prepareLintPackageTask(plans)
	if err != nil {
		return reportLintFixFailure(invocation, stdout, stderr, exitCode, nil, err)
	}
	var executions []lintFixExecution
	if packageMode {
		executions, exitCode, err = prepareLintPackageFixExecutions(
			ctx,
			packageTask,
			registry,
			invocation.selectionOptions(),
		)
	} else {
		var tasks []lintTask
		tasks, exitCode, err = prepareLintTasksFromPlans(
			ctx,
			plans,
			invocation.configPath,
			registry,
		)
		if err == nil {
			executions, exitCode, err = prepareLintFixExecutions(
				ctx,
				tasks,
				registry,
				invocation.selectionOptions(),
			)
		}
	}
	if err != nil {
		return reportLintFixFailure(invocation, stdout, stderr, exitCode, executions, err)
	}

	for index := range executions {
		if err := ctx.Err(); err != nil {
			return reportLintFixFailure(
				invocation,
				stdout,
				stderr,
				ExitCanceled,
				executions,
				err,
			)
		}
		execution := &executions[index]
		if execution.packageTask != nil {
			exitCode, err := refreshLintPackageFixExecution(
				ctx,
				registry,
				execution,
				invocation.selectionOptions(),
			)
			if err != nil {
				return reportLintFixFailure(
					invocation,
					stdout,
					stderr,
					exitCode,
					executions,
					err,
				)
			}
		}
		postResult := execution.result
		postFile := execution.file
		var postAnalysisErr error
		options := fixengine.Options{
			AllowSuggestion: invocation.fixSuggestions,
			AllowUnsafe: invocation.fixUnsafe,
			Format: execution.task.options.format,
		}
		options.Validate = func(formatted *source.File) error {
			var analyzed analysis.Result
			var analyzedFile *source.File
			var err error
			if execution.packageTask == nil {
				analyzed, err = analysis.Run(
					ctx,
					formatted,
					registry,
					execution.task.options.analysis,
				)
				analyzedFile = formatted
			} else {
				analyzed, analyzedFile, err = validateLintPackageFix(
					ctx,
					registry,
					*execution.packageTask,
					formatted,
				)
			}
			if err != nil {
				var validationErr *lintPackageValidationError
				if !errors.As(err, &validationErr) {
					postAnalysisErr = err
				}
				return err
			}
			postResult = analyzed
			postFile = analyzedFile
			return nil
		}
		transaction, transactionErr := fixengine.CoordinateAndReplace(
			execution.snapshot,
			execution.selections,
			options,
		)
		recordLintFixTransaction(
			execution,
			postResult,
			postFile,
			transaction,
			transactionErr,
		)
		if postAnalysisErr != nil {
			return reportLintFixFailure(
				invocation,
				stdout,
				stderr,
				exitCodeForError(ExitInternalError, postAnalysisErr),
				executions,
				postAnalysisErr,
			)
		}
		if transactionErr != nil {
			if errors.Is(transactionErr, filesystem.ErrStale) {
				continue
			}
			return reportLintFixFailure(
				invocation,
				stdout,
				stderr,
				ExitFilesystemError,
				executions,
				transactionErr,
			)
		}
	}
	if packageMode {
		exitCode, err := refreshFinalLintPackageResults(
			ctx,
			registry,
			packageTask,
			executions,
		)
		if err != nil {
			return reportLintFixFailure(
				invocation,
				stdout,
				stderr,
				exitCode,
				executions,
				err,
			)
		}
	}
	if err := ctx.Err(); err != nil {
		return reportLintFixFailure(
			invocation,
			stdout,
			stderr,
			ExitCanceled,
			executions,
			err,
		)
	}
	exitCode = lintFixExitCode(executions)
	if invocation.reporter == goxreport.JSON {
		return reportLintFixJSON(stdout, stderr, exitCode, true, executions, nil)
	}
	inputs := make([]goxreport.LintFixTextInput, len(executions))
	for index, execution := range executions {
		inputs[index] = goxreport.LintFixTextInput{
			File: execution.file,
			ResultFile: execution.resultFile,
			Result: execution.result,
			Outcome: execution.outcome,
		}
	}
	output, err := goxreport.RenderLintFixText(inputs)
	if err != nil {
		return reportLintFixFailure(
			invocation,
			stdout,
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			executions,
			fmt.Errorf("render fix report: %w", err),
		)
	}
	if len(output) > 0 {
		if err := write(stdout, output); err != nil {
			return reportLintFixFailure(
				invocation,
				stdout,
				stderr,
				moreSevereExitCode(exitCode, ExitFilesystemError),
				executions,
				fmt.Errorf("write standard output: %w", err),
			)
		}
	}
	return exitCode
}

func prepareLintPackageFixExecutions(
	ctx context.Context,
	task lintPackageTask,
	registry *rules.Registry,
	selectionOptions fixengine.SelectionOptions,
) ([]lintFixExecution, int, error) {
	packageResult, err := runUncachedPackageAnalysis(ctx, registry, task, nil)
	if err != nil {
		return nil, packageAnalysisErrorExitCode(err), err
	}
	if err := validateLintPackagePrerequisites(packageResult); err != nil {
		return nil, ExitSourceError, err
	}
	executions := make([]lintFixExecution, 0, len(packageResult.Files))
	for _, result := range packageResult.Files {
		if err := ctx.Err(); err != nil {
			return executions, ExitCanceled, err
		}
		file, found := packageResult.Sources.Lookup(result.Path)
		if !found {
			return executions, ExitInternalError, fmt.Errorf(
				"typed lint result source %q is missing",
				result.Path,
			)
		}
		if file.Metadata().Generated {
			return executions, ExitFilesystemError, fmt.Errorf(
				"refusing to fix generated file %q",
				file.Path(),
			)
		}
		snapshot, exitCode, err := prepareLintPackageSnapshot(ctx, task.root, file)
		if err != nil {
			return executions, exitCode, err
		}
		selections, err := fixengine.Select(result.Diagnostics, selectionOptions)
		if err != nil {
			return executions, ExitInternalError, err
		}
		packageTask := task
		executions = append(
			executions,
			lintFixExecution{
				file: file,
				resultFile: file,
				result: result,
				outcome: goxreport.LintFixOutcome{
					Path: file.Path(),
					SourceDigest: file.Digest(),
					Status: goxreport.LintFilePending,
				},
				selections: selections,
				task: lintTask{
					file: discovery.File{Path: file.Path()},
					root: task.root,
					options: task.options,
				},
				packageTask: &packageTask,
				snapshot: snapshot,
			},
		)
	}
	return executions, ExitSuccess, nil
}

func refreshLintPackageFixExecution(
	ctx context.Context,
	registry *rules.Registry,
	execution *lintFixExecution,
	selectionOptions fixengine.SelectionOptions,
) (int, error) {
	packageResult, err := runUncachedPackageAnalysis(ctx, registry, *execution.packageTask, nil)
	if err != nil {
		return packageAnalysisErrorExitCode(err), err
	}
	if err := validateLintPackagePrerequisites(packageResult); err != nil {
		return ExitSourceError, err
	}
	for _, result := range packageResult.Files {
		if result.Path != execution.file.Path() {
			continue
		}
		file, found := packageResult.Sources.Lookup(result.Path)
		if !found {
			return ExitInternalError, fmt.Errorf(
				"typed lint result source %q is missing",
				result.Path,
			)
		}
		if file.Metadata().Generated {
			return ExitFilesystemError, fmt.Errorf(
				"refusing to fix generated file %q",
				file.Path(),
			)
		}
		snapshot, exitCode, err := prepareLintPackageSnapshot(
			ctx,
			execution.task.root,
			file,
		)
		if err != nil {
			return exitCode, err
		}
		selections, err := fixengine.Select(result.Diagnostics, selectionOptions)
		if err != nil {
			return ExitInternalError, err
		}
		execution.file = file
		execution.resultFile = file
		execution.result = result
		execution.selections = selections
		execution.snapshot = snapshot
		execution.outcome = goxreport.LintFixOutcome{
			Path: file.Path(),
			SourceDigest: file.Digest(),
			Status: goxreport.LintFilePending,
		}
		return ExitSuccess, nil
	}
	return ExitSourceError, newLintPackageValidationError(
		"typed lint result %q is missing during fix planning",
		execution.file.Path(),
	)
}

func refreshFinalLintPackageResults(
	ctx context.Context,
	registry *rules.Registry,
	task lintPackageTask,
	executions []lintFixExecution,
) (int, error) {
	packageResult, err := runUncachedPackageAnalysis(ctx, registry, task, nil)
	if err != nil {
		return packageAnalysisErrorExitCode(err), err
	}
	if err := validateLintPackagePrerequisites(packageResult); err != nil {
		return ExitSourceError, err
	}
	results := make(map[string]analysis.Result, len(packageResult.Files))
	for _, result := range packageResult.Files {
		results[result.Path] = result
	}
	if len(results) != len(executions) {
		return ExitSourceError, newLintPackageValidationError(
			"typed package source selection changed during fix execution",
		)
	}
	for index := range executions {
		execution := &executions[index]
		result, found := results[execution.file.Path()]
		if !found {
			return ExitSourceError, newLintPackageValidationError(
				"final typed lint result %q is missing",
				execution.file.Path(),
			)
		}
		file, found := packageResult.Sources.Lookup(result.Path)
		if !found {
			return ExitInternalError, fmt.Errorf(
				"final typed lint source %q is missing",
				result.Path,
			)
		}
		execution.result = result
		execution.resultFile = file
	}
	return ExitSuccess, nil
}

func prepareLintPackageSnapshot(
	ctx context.Context,
	root string,
	file *source.File,
) (*filesystem.Snapshot, int, error) {
	files, err := discovery.GoFiles(ctx, []string{file.Path()}, discovery.Options{Root: root})
	if err != nil {
		return nil, exitCodeForError(ExitFilesystemError, err), err
	}
	if len(files) != 1 || files[0].Path != file.Path() {
		return nil, ExitInternalError, fmt.Errorf(
			"typed fix source %q was not rediscovered exactly",
			file.Path(),
		)
	}
	if files[0].TraversesSymlink {
		return nil, ExitFilesystemError, fmt.Errorf(
			"refusing to fix symlink %q",
			file.Path(),
		)
	}
	snapshot, err := filesystem.ReadWithin(root, file.Path())
	if err != nil {
		return nil, exitCodeForError(ExitFilesystemError, err), err
	}
	snapshotFile, err := source.Load(snapshot.Path(), snapshot.Bytes())
	if err != nil {
		return nil, ExitSourceError, err
	}
	if snapshotFile.Digest() != file.Digest() {
		return nil, ExitConflict, fmt.Errorf(
			"typed fix source %q changed during package analysis: %w",
			file.Path(),
			filesystem.ErrStale,
		)
	}
	return snapshot, ExitSuccess, nil
}

func validateLintPackageFix(
	ctx context.Context,
	registry *rules.Registry,
	task lintPackageTask,
	formatted *source.File,
) (analysis.Result, *source.File, error) {
	packageResult, err := runUncachedPackageAnalysis(
		ctx,
		registry,
		task,
		map[string][]byte{formatted.Path(): formatted.Bytes()},
	)
	if err != nil {
		return analysis.Result{}, nil, err
	}
	if err := validateLintPackagePrerequisites(packageResult); err != nil {
		return analysis.Result{}, nil, err
	}
	for _, result := range packageResult.Files {
		if result.Path != formatted.Path() {
			continue
		}
		file, found := packageResult.Sources.Lookup(result.Path)
		if !found {
			return analysis.Result{}, nil, newLintPackageValidationError(
				"post-fix typed source %q is missing",
				result.Path,
			)
		}
		if file.Digest() != formatted.Digest() {
			return analysis.Result{}, nil, newLintPackageValidationError(
				"post-fix typed source %q does not match the validated overlay",
				result.Path,
			)
		}
		return result, file, nil
	}
	return analysis.Result{}, nil, newLintPackageValidationError(
		"post-fix typed result %q is missing",
		formatted.Path(),
	)
}

func runUncachedPackageAnalysis(
	ctx context.Context,
	registry *rules.Registry,
	task lintPackageTask,
	overlay map[string][]byte,
) (analysis.PackageResult, error) {
	return analysis.RunPackages(
		ctx,
		registry,
		task.options.analysis,
		packageLoadOptions(task, overlay),
	)
}

func packageLoadOptions(
	task lintPackageTask,
	overlay map[string][]byte,
) analysis.PackageLoadOptions {
	selection := task.options.buildSelection
	return analysis.PackageLoadOptions{
		Dir: task.root,
		Patterns: task.patterns,
		Tests: true,
		BuildTags: slices.Clone(selection.BuildTags),
		ModuleMode: analysis.ModuleReadonly,
		Env: packageAnalysisEnvironment(selection.CGOEnabled),
		Overlay: overlay,
		GOOS: selection.GOOS,
		GOARCH: selection.GOARCH,
	}
}

func validateLintPackagePrerequisites(result analysis.PackageResult) error {
	if len(result.SourceProblems) > 0 {
		problem := result.SourceProblems[0]
		return newLintPackageValidationError(
			"typed package source %q failed validation: %s",
			problem.Path,
			problem.Message,
		)
	}
	if len(result.LoadDiagnostics) > 0 {
		diagnostic := result.LoadDiagnostics[0]
		if diagnostic.Position != "" {
			return newLintPackageValidationError(
				"typed package validation failed at %s: %s",
				diagnostic.Position,
				diagnostic.Message,
			)
		}
		return newLintPackageValidationError(
			"typed package validation failed: %s",
			diagnostic.Message,
		)
	}
	return nil
}

func newLintPackageValidationError(format string, arguments ...any) error {
	return &lintPackageValidationError{err: fmt.Errorf(format, arguments...)}
}

func prepareLintFixExecutions(
	ctx context.Context,
	tasks []lintTask,
	registry *rules.Registry,
	selectionOptions fixengine.SelectionOptions,
) ([]lintFixExecution, int, error) {
	executions := make([]lintFixExecution, 0, len(tasks))
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			return executions, ExitCanceled, err
		}
		if task.file.TraversesSymlink {
			return executions, ExitFilesystemError, fmt.Errorf(
				"refusing to fix symlink %q",
				task.file.Path,
			)
		}
		snapshot, err := filesystem.ReadWithin(task.root, task.file.Path)
		if err != nil {
			return executions, exitCodeForError(ExitFilesystemError, err), err
		}
		file, err := source.Load(snapshot.Path(), snapshot.Bytes())
		if err != nil {
			return executions, ExitSourceError, err
		}
		if file.Metadata().Generated {
			return executions, ExitFilesystemError, fmt.Errorf(
				"refusing to fix generated file %q",
				file.Path(),
			)
		}
		analyzed, err := analysis.Run(ctx, file, registry, task.options.analysis)
		if err != nil {
			return executions, exitCodeForError(ExitInternalError, err), err
		}
		selections, err := fixengine.Select(analyzed.Diagnostics, selectionOptions)
		if err != nil {
			return executions, ExitInternalError, err
		}
		executions = append(
			executions,
			lintFixExecution{
				file: file,
				resultFile: file,
				result: analyzed,
				outcome: goxreport.LintFixOutcome{
					Path: file.Path(),
					SourceDigest: file.Digest(),
					Status: goxreport.LintFilePending,
				},
				selections: selections,
				task: task,
				snapshot: snapshot,
			},
		)
	}
	return executions, ExitSuccess, nil
}

func recordLintFixTransaction(
	execution *lintFixExecution,
	postResult analysis.Result,
	postFile *source.File,
	transaction fixengine.Transaction,
	transactionErr error,
) {
	execution.outcome.Applied = append([]fixengine.Applied(nil), transaction.Result.Applied...)
	execution.outcome.Rejected = append(
		[]fixengine.Rejection(nil),
		transaction.Result.Rejected...,
	)
	if errors.Is(transactionErr, filesystem.ErrStale) {
		for _, applied := range transaction.Result.Applied {
			execution.outcome.Rejected = append(
				execution.outcome.Rejected,
				fixengine.Rejection{
					RuleID: applied.RuleID,
					FixName: applied.FixName,
					Range: applied.Range,
					Reason: fixengine.RejectionStaleSource,
					Message: "source changed before atomic replacement",
				},
			)
		}
		execution.outcome.Status = goxreport.LintFileConflict
		return
	}
	execution.result = postResult
	execution.resultFile = postFile
	execution.outcome.Status = lintFixFileStatus(transaction)
}

func lintFixFileStatus(transaction fixengine.Transaction) goxreport.LintFileStatus {
	if transaction.Status == fixengine.WritePossiblyCompleted {
		return goxreport.LintFilePossiblyFixed
	}
	if transaction.Status == fixengine.WriteCompleted {
		return goxreport.LintFileFixed
	}
	for _, rejected := range transaction.Result.Rejected {
		if rejected.Reason == fixengine.RejectionConflict {
			return goxreport.LintFileConflict
		}
	}
	return goxreport.LintFileUnchanged
}

func lintFixExitCode(executions []lintFixExecution) int {
	exitCode := ExitSuccess
	results := make([]analysis.Result, len(executions))
	for index, execution := range executions {
		results[index] = execution.result
		if execution.outcome.Status == goxreport.LintFileConflict {
			exitCode = moreSevereExitCode(exitCode, ExitConflict)
		}
		for _, rejected := range execution.outcome.Rejected {
			if rejected.Reason == fixengine.RejectionConflict {
				exitCode = moreSevereExitCode(exitCode, ExitConflict)
			} else {
				exitCode = moreSevereExitCode(exitCode, ExitFindings)
			}
		}
	}
	return moreSevereExitCode(exitCode, lintResultExitCode(results))
}

func lintResultExitCode(results []analysis.Result) int {
	for _, result := range results {
		if len(result.Diagnostics) > 0 ||
			len(result.SuppressionProblems) > 0 ||
			len(result.UnusedSuppressions) > 0 {
			return ExitFindings
		}
	}
	return ExitSuccess
}

func lintPackageResultExitCode(result analysis.PackageResult) int {
	exitCode := lintResultExitCode(result.Files)
	if len(result.LoadDiagnostics) > 0 || len(result.SourceProblems) > 0 {
		exitCode = moreSevereExitCode(exitCode, ExitSourceError)
	}
	return exitCode
}

func reportLintFailure(
	invocation lintInvocation,
	stdout, stderr io.Writer,
	exitCode int,
	results []analysis.Result,
	err error,
) int {
	if invocation.reporter == goxreport.JSON {
		return reportLintJSON(stdout, stderr, "check", exitCode, false, results, err)
	}
	return report(stderr, exitCode, "gox lint: %v\n", err)
}

func reportLintPackageFailure(
	invocation lintInvocation,
	stdout, stderr io.Writer,
	exitCode int,
	result analysis.PackageResult,
	err error,
) int {
	if invocation.reporter == goxreport.JSON {
		return reportLintPackageJSON(stdout, stderr, "check", exitCode, false, result, err)
	}
	return report(stderr, exitCode, "gox lint: %v\n", err)
}

func reportLintFixFailure(
	invocation lintInvocation,
	stdout, stderr io.Writer,
	exitCode int,
	executions []lintFixExecution,
	err error,
) int {
	if invocation.reporter == goxreport.JSON {
		return reportLintFixJSON(stdout, stderr, exitCode, false, executions, err)
	}
	paths, possibly := completedLintFixPaths(executions)
	if len(paths) == 0 {
		return report(stderr, exitCode, "gox lint: %v\n", err)
	}
	heading := "files fixed before failure"
	if possibly {
		heading = "files fixed or possibly fixed before failure"
	}
	return report(
		stderr,
		exitCode,
		"gox lint: %v\ngox lint: %s:\n%s\n",
		err,
		heading,
		strings.Join(paths, "\n"),
	)
}

func reportLintJSON(
	stdout, stderr io.Writer,
	mode string,
	exitCode int,
	complete bool,
	results []analysis.Result,
	err error,
) int {
	errors_ := []goxreport.Error{}
	if err != nil {
		errors_ = append(errors_, goxreport.Error{Message: err.Error()})
	}
	result, buildErr := goxreport.NewLintResult(
		mode,
		exitCategory(exitCode),
		exitCode,
		complete,
		results,
		errors_,
	)
	if buildErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"gox lint: build JSON report: %v\n",
			buildErr,
		)
	}
	encoded, encodeErr := goxreport.MarshalLintJSON(result)
	if encodeErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"gox lint: encode JSON report: %v\n",
			encodeErr,
		)
	}
	if writeErr := write(stdout, encoded); writeErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitFilesystemError),
			"gox lint: write JSON report: %v\n",
			writeErr,
		)
	}
	return exitCode
}

func reportLintPackageJSON(
	stdout, stderr io.Writer,
	mode string,
	exitCode int,
	complete bool,
	packageResult analysis.PackageResult,
	err error,
) int {
	errors_ := []goxreport.Error{}
	if err != nil {
		errors_ = append(errors_, goxreport.Error{Message: err.Error()})
	}
	result, buildErr := goxreport.NewPackageLintResult(
		mode,
		exitCategory(exitCode),
		exitCode,
		complete,
		packageResult,
		errors_,
	)
	if buildErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"gox lint: build typed JSON report: %v\n",
			buildErr,
		)
	}
	encoded, encodeErr := goxreport.MarshalLintJSON(result)
	if encodeErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"gox lint: encode typed JSON report: %v\n",
			encodeErr,
		)
	}
	if writeErr := write(stdout, encoded); writeErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitFilesystemError),
			"gox lint: write typed JSON report: %v\n",
			writeErr,
		)
	}
	return exitCode
}

func reportLintFixJSON(
	stdout, stderr io.Writer,
	exitCode int,
	complete bool,
	executions []lintFixExecution,
	err error,
) int {
	results := make([]analysis.Result, len(executions))
	outcomes := make([]goxreport.LintFixOutcome, len(executions))
	for index, execution := range executions {
		results[index] = execution.result
		outcomes[index] = execution.outcome
	}
	errors_ := []goxreport.Error{}
	if err != nil {
		errors_ = append(errors_, goxreport.Error{Message: err.Error()})
	}
	result, buildErr := goxreport.NewLintFixResult(
		exitCategory(exitCode),
		exitCode,
		complete,
		results,
		outcomes,
		errors_,
	)
	if buildErr != nil {
		return reportLintFixReportingFailure(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"build fix JSON report",
			buildErr,
			executions,
		)
	}
	encoded, encodeErr := goxreport.MarshalLintJSON(result)
	if encodeErr != nil {
		return reportLintFixReportingFailure(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"encode fix JSON report",
			encodeErr,
			executions,
		)
	}
	if writeErr := write(stdout, encoded); writeErr != nil {
		return reportLintFixReportingFailure(
			stderr,
			moreSevereExitCode(exitCode, ExitFilesystemError),
			"write fix JSON report",
			writeErr,
			executions,
		)
	}
	return exitCode
}

func reportLintFixReportingFailure(
	stderr io.Writer,
	exitCode int,
	action string,
	err error,
	executions []lintFixExecution,
) int {
	paths, possibly := completedLintFixPaths(executions)
	if len(paths) == 0 {
		return report(stderr, exitCode, "gox lint: %s: %v\n", action, err)
	}
	heading := "files fixed before reporting failure"
	if possibly {
		heading = "files fixed or possibly fixed before reporting failure"
	}
	return report(
		stderr,
		exitCode,
		"gox lint: %s: %v\ngox lint: %s:\n%s\n",
		action,
		err,
		heading,
		strings.Join(paths, "\n"),
	)
}

func completedLintFixPaths(executions []lintFixExecution) ([]string, bool) {
	paths := make([]string, 0)
	possibly := false
	for _, execution := range executions {
		switch execution.outcome.Status {
		case goxreport.LintFileFixed:
			paths = append(paths, execution.outcome.Path)
		case goxreport.LintFilePossiblyFixed:
			paths = append(paths, execution.outcome.Path)
			possibly = true
		}
	}
	sort.Strings(paths)
	return paths, possibly
}
