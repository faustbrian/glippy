package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/baseline"
	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/changed"
	"github.com/faustbrian/glippy/internal/config"
	"github.com/faustbrian/glippy/internal/discovery"
	glippydiff "github.com/faustbrian/glippy/internal/diff"
	"github.com/faustbrian/glippy/internal/filesystem"
	fixengine "github.com/faustbrian/glippy/internal/fix"
	glippyformat "github.com/faustbrian/glippy/internal/format"
	"github.com/faustbrian/glippy/internal/goversion"
	glippyreport "github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

const lintUsage = "glippy: expected 'lint [--fix] [--fix-suggestions] [--fix-unsafe] [--diff] [-A|--allow <rules-or-groups>] [-W|--warn <rules-or-groups>] [-D|--deny <rules-or-groups>] [-F|--forbid <rules-or-groups>] [--only=<rules>] [--except=<rules>] [--new-from=<git-ref>] [--generate-baseline=<path>] [--reporter=text|short|json|github|sarif] [--config=<path>] [path...]'\n"

type lintInvocation struct {
	configPath string
	fix bool
	fixSuggestions bool
	fixUnsafe bool
	diff bool
	generateBaseline string
	newFrom string
	lintLevels []rules.LintLevelDirective
	only []string
	except []string
	paths []string
	reporter glippyreport.Format
}

type lintTaskOptions struct {
	analysis analysis.RunOptions
	buildSelection config.Analysis
	format glippyformat.Options
	cache config.Cache
	configurationDigest cache.Digest
	sourceGoVersion string
	baseline config.Baseline
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

type lintBaselineFilesystemError struct {
	err error
}

func (e *lintPackageValidationError) Error() string {
	return e.err.Error()
}

func (e *lintPackageValidationError) Unwrap() error {
	return e.err
}

func (e *lintBaselineFilesystemError) Error() string {
	return e.err.Error()
}

func (e *lintBaselineFilesystemError) Unwrap() error {
	return e.err
}

type lintFixExecution struct {
	file *source.File
	resultFile *source.File
	result analysis.Result
	outcome glippyreport.LintFixOutcome
	selections []fixengine.Selection
	task lintTask
	packageTask *lintPackageTask
	snapshot *filesystem.Snapshot
}

func parseLintInvocation(arguments []string) (lintInvocation, bool) {
	if len(arguments) == 0 || arguments[0] != "lint" {
		return lintInvocation{}, false
	}
	result := lintInvocation{reporter: glippyreport.Text}
	reporterSet := false
	fixSet := false
	onlySet := false
	exceptSet := false
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		if directive, consumed, matched, valid := parseLintLevelDirective(arguments, index);
			matched {
			if !valid {
				return lintInvocation{}, false
			}
			result.lintLevels = append(result.lintLevels, directive)
			index += consumed
			continue
		}
		switch {
		case argument == "--fix" && !fixSet:
			result.fix = true
			fixSet = true
		case argument == "--fix-suggestions" && !result.fixSuggestions:
			result.fixSuggestions = true
		case argument == "--fix-unsafe" && !result.fixUnsafe:
			result.fixUnsafe = true
		case argument == "--diff" && !result.diff:
			result.diff = true
		case strings.HasPrefix(argument, "--only=") && !onlySet:
			parsed, valid := parseRuleFilter(strings.TrimPrefix(argument, "--only="))
			if !valid {
				return lintInvocation{}, false
			}
			result.only = parsed
			onlySet = true
		case argument == "--only" &&
			!onlySet &&
			index + 1 < len(arguments) &&
			!strings.HasPrefix(arguments[index + 1], "--"):
			index++
			parsed, valid := parseRuleFilter(arguments[index])
			if !valid {
				return lintInvocation{}, false
			}
			result.only = parsed
			onlySet = true
		case strings.HasPrefix(argument, "--except=") && !exceptSet:
			parsed, valid := parseRuleFilter(strings.TrimPrefix(argument, "--except="))
			if !valid {
				return lintInvocation{}, false
			}
			result.except = parsed
			exceptSet = true
		case argument == "--except" &&
			!exceptSet &&
			index + 1 < len(arguments) &&
			!strings.HasPrefix(arguments[index + 1], "--"):
			index++
			parsed, valid := parseRuleFilter(arguments[index])
			if !valid {
				return lintInvocation{}, false
			}
			result.except = parsed
			exceptSet = true
		case strings.HasPrefix(argument, "--new-from=") && result.newFrom == "":
			result.newFrom = strings.TrimPrefix(argument, "--new-from=")
			if result.newFrom == "" {
				return lintInvocation{}, false
			}
		case argument == "--new-from" &&
			result.newFrom == "" &&
			index + 1 < len(arguments) &&
			!strings.HasPrefix(arguments[index + 1], "--"):
			index++
			result.newFrom = arguments[index]
		case strings.HasPrefix(argument, "--generate-baseline=") &&
			result.generateBaseline == "":
			result.generateBaseline = strings.TrimPrefix(
				argument,
				"--generate-baseline=",
			)
			if !baseline.ValidPath(result.generateBaseline) {
				return lintInvocation{}, false
			}
		case argument == "--generate-baseline" &&
			result.generateBaseline == "" &&
			index + 1 < len(arguments) &&
			!strings.HasPrefix(arguments[index + 1], "--"):
			index++
			result.generateBaseline = arguments[index]
			if !baseline.ValidPath(result.generateBaseline) {
				return lintInvocation{}, false
			}
		case strings.HasPrefix(argument, "--reporter=") && !reporterSet:
			reporter, valid := parseDiagnosticReporter(
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
			reporter, valid := parseDiagnosticReporter(arguments[index])
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
	if result.generateBaseline != "" &&
		(result.fixEnabled() ||
			result.reporter != glippyreport.Text ||
			result.newFrom != "") {
		return lintInvocation{}, false
	}
	if result.diff && (!result.fixEnabled() || result.reporter != glippyreport.Text) {
		return lintInvocation{}, false
	}
	return result, true
}

func parseLintLevelDirective(
	arguments []string,
	index int,
) (rules.LintLevelDirective, int, bool, bool) {
	if index < 0 || index >= len(arguments) {
		return rules.LintLevelDirective{}, 0, false, false
	}
	argument := arguments[index]
	type levelFlag struct {
		short string
		long string
		level rules.LintLevel
	}
	flags := []levelFlag{
		{short: "-A", long: "--allow", level: rules.LintAllow},
		{short: "-W", long: "--warn", level: rules.LintWarn},
		{short: "-D", long: "--deny", level: rules.LintDeny},
		{short: "-F", long: "--forbid", level: rules.LintForbid},
	}
	for _, flag := range flags {
		value := ""
		consumed := 0
		switch {
		case strings.HasPrefix(argument, flag.long + "="):
			value = strings.TrimPrefix(argument, flag.long + "=")
		case strings.HasPrefix(argument, flag.short) && argument != flag.short:
			value = strings.TrimPrefix(argument, flag.short)
		case argument == flag.long || argument == flag.short:
			if index + 1 >= len(arguments) ||
				strings.HasPrefix(arguments[index + 1], "-") {
				return rules.LintLevelDirective{}, 0, true, false
			}
			value = arguments[index + 1]
			consumed = 1
		default:
			continue
		}
		targets, valid := parseRuleFilter(value)
		if !valid {
			return rules.LintLevelDirective{}, 0, true, false
		}
		return rules.LintLevelDirective{
			Level: flag.level,
			Targets: targets,
		}, consumed, true, true
	}
	return rules.LintLevelDirective{}, 0, false, false
}

func cloneLintLevelDirectives(directives []rules.LintLevelDirective) []rules.LintLevelDirective {
	if directives == nil {
		return nil
	}
	result := make([]rules.LintLevelDirective, len(directives))
	for index, directive := range directives {
		result[index] = rules.LintLevelDirective{
			Level: directive.Level,
			Targets: slices.Clone(directive.Targets),
		}
	}
	return result
}

func parseRuleFilter(value string) ([]string, bool) {
	if value == "" {
		return nil, false
	}
	ids := strings.Split(value, ",")
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" || strings.TrimSpace(id) != id {
			return nil, false
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, false
		}
		seen[id] = struct{}{}
	}
	sort.Strings(ids)
	return ids, true
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

func prepareChangedScope(
	ctx context.Context,
	base string,
	plans []lintInputPlan,
) (*changed.Scope, int, error) {
	if base == "" {
		return nil, ExitSuccess, nil
	}
	if len(plans) == 0 {
		return nil, ExitInternalError, errors.New(
			"changed-code planning produced no inputs",
		)
	}
	scope, err := changed.Resolve(ctx, plans[0].anchor, base)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, ExitCanceled, err
		}
		return nil, ExitInvalidInvocation, err
	}
	return scope, ExitSuccess, nil
}

func filterChangedResult(scope *changed.Scope, file *source.File, result *analysis.Result) error {
	if scope == nil {
		return nil
	}
	if file == nil || result == nil {
		return errors.New("changed-code filtering requires a source and analysis result")
	}
	if !scope.Contains(file.Path()) {
		return fmt.Errorf(
			"changed-code source %q is outside Git root %q",
			file.Path(),
			scope.Root(),
		)
	}
	visible, preexisting, err := scope.FilterDiagnostics(file, result.Diagnostics)
	if err != nil {
		return err
	}
	result.Diagnostics = visible
	result.PreexistingDiagnostics = append(result.PreexistingDiagnostics, preexisting...)
	return nil
}

func filterChangedPackageResult(scope *changed.Scope, result *analysis.PackageResult) error {
	if scope == nil {
		return nil
	}
	if result == nil {
		return errors.New("changed-code filtering requires a package result")
	}
	for index := range result.Files {
		file, found := result.Sources.Lookup(result.Files[index].Path)
		if !found {
			return fmt.Errorf(
				"changed-code source %q is missing",
				result.Files[index].Path,
			)
		}
		if err := filterChangedResult(scope, file, &result.Files[index]); err != nil {
			return err
		}
	}
	return nil
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
	changedScope, exitCode, err := prepareChangedScope(ctx, invocation.newFrom, plans)
	if err != nil {
		return reportLintFailure(invocation, stdout, stderr, exitCode, nil, err)
	}
	packageTask, packageMode, exitCode, err := prepareLintPackageTask(plans)
	if err != nil {
		return reportLintFailure(invocation, stdout, stderr, exitCode, nil, err)
	}
	if packageMode {
		return runLintPackageCheck(
			ctx,
			invocation,
			stdout,
			stderr,
			registry,
			packageTask,
			changedScope,
		)
	}
	tasks, exitCode, err := prepareLintTasksFromPlans(
		ctx,
		plans,
		invocation.configPath,
		registry,
		invocation.only,
		invocation.except,
		invocation.lintLevels,
	)
	if err != nil {
		return reportLintFailure(invocation, stdout, stderr, exitCode, nil, err)
	}

	inputs := make([]glippyreport.LintTextInput, 0, len(tasks))
	results := make([]analysis.Result, 0, len(tasks))
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			return reportLintFailure(
				invocation,
				stdout,
				stderr,
				ExitCanceled,
				inputs,
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
				inputs,
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
				inputs,
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
				inputs,
				err,
			)
		}
		results = append(results, analyzed)
		inputs = append(inputs, glippyreport.LintTextInput{File: file, Result: analyzed})
	}
	if err := applyConfiguredBaselines(tasks, inputs, results, registry); err != nil {
		return reportLintFailure(
			invocation,
			stdout,
			stderr,
			lintBaselineErrorExitCode(err),
			inputs,
			err,
		)
	}
	for index := range inputs {
		if err := filterChangedResult(changedScope, inputs[index].File, &results[index]);
			err != nil {
			return reportLintFailure(
				invocation,
				stdout,
				stderr,
				ExitInvalidInvocation,
				inputs,
				err,
			)
		}
		inputs[index].Result = results[index]
	}
	if err := ctx.Err(); err != nil {
		return reportLintFailure(invocation, stdout, stderr, ExitCanceled, inputs, err)
	}
	exitCode = lintResultExitCode(results)
	if invocation.reporter == glippyreport.JSON {
		return reportLintJSON(stdout, stderr, "check", exitCode, true, results, nil)
	}
	if isIntegrationReporter(invocation.reporter) {
		return reportIntegrationOutput(
			"lint",
			invocation.reporter,
			stdout,
			stderr,
			exitCode,
			glippyreport.IntegrationInput{Files: inputs, Registry: registry},
		)
	}
	output, err := renderLintText(invocation.reporter, inputs)
	if err != nil {
		return report(
			stderr,
			ExitInternalError,
			"glippy lint: render text report: %v\n",
			err,
		)
	}
	if len(output) > 0 {
		if err := write(stdout, output); err != nil {
			return report(
				stderr,
				ExitFilesystemError,
				"glippy lint: write standard output: %v\n",
				err,
			)
		}
	}
	return exitCode
}

func runLintGenerateBaseline(
	ctx context.Context,
	invocation lintInvocation,
	stdout, stderr io.Writer,
	registry *rules.Registry,
) int {
	if ctx == nil || registry == nil {
		return report(
			stderr,
			ExitInternalError,
			"glippy lint: generate baseline: context and rule registry are required\n",
		)
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "glippy lint: generate baseline: %v\n", err)
	}
	if !baseline.ValidPath(invocation.generateBaseline) {
		return report(
			stderr,
			ExitInvalidInvocation,
			"glippy lint: generate baseline: path must be portable and relative\n",
		)
	}
	plans, exitCode, err := prepareLintInputPlans(ctx, invocation, registry)
	if err != nil {
		return report(stderr, exitCode, "glippy lint: generate baseline: %v\n", err)
	}
	if len(plans) == 0 || plans[0].selection.Root == "" {
		return report(
			stderr,
			ExitInvalidInvocation,
			"glippy lint: generate baseline: a project root is required\n",
		)
	}
	root := plans[0].selection.Root
	configurationPath := plans[0].selection.Path
	for _, plan := range plans[1:] {
		if plan.selection.Root != root || plan.selection.Path != configurationPath {
			return report(
				stderr,
				ExitInvalidInvocation,
				"glippy lint: generate baseline: inputs must resolve to one project root and configuration\n",
			)
		}
	}
	baselinePath := filepath.Join(root, filepath.FromSlash(invocation.generateBaseline))
	var baselineSnapshot *filesystem.Snapshot
	if _, err := os.Lstat(baselinePath); err == nil {
		baselineSnapshot, err = filesystem.ReadWithin(root, baselinePath)
		if err != nil {
			return report(
				stderr,
				exitCodeForError(ExitFilesystemError, err),
				"glippy lint: generate baseline: %v\n",
				err,
			)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return report(
			stderr,
			exitCodeForError(ExitFilesystemError, err),
			"glippy lint: generate baseline: inspect %q: %v\n",
			baselinePath,
			err,
		)
	}
	packageTask, packageMode, exitCode, err := prepareLintPackageTask(plans)
	if err != nil {
		return report(stderr, exitCode, "glippy lint: generate baseline: %v\n", err)
	}
	inputs := make([]baseline.InputFile, 0)
	if packageMode {
		result, err := runPackageAnalysis(ctx, registry, packageTask)
		if err != nil {
			return report(
				stderr,
				packageAnalysisErrorExitCode(err),
				"glippy lint: generate baseline: %v\n",
				err,
			)
		}
		for _, analyzed := range result.Files {
			file, found := result.Sources.Lookup(analyzed.Path)
			if !found {
				return report(
					stderr,
					ExitInternalError,
					"glippy lint: generate baseline: source %q is missing\n",
					analyzed.Path,
				)
			}
			inputs = append(
				inputs,
				baseline.InputFile{File: file, Diagnostics: analyzed.Diagnostics},
			)
		}
	} else {
		tasks, exitCode, err := prepareLintTasksFromPlans(
			ctx,
			plans,
			invocation.configPath,
			registry,
			invocation.only,
			invocation.except,
			invocation.lintLevels,
		)
		if err != nil {
			return report(stderr, exitCode, "glippy lint: generate baseline: %v\n", err)
		}
		for _, task := range tasks {
			input, err := source.ReadFile(task.file.Path)
			if err != nil {
				return report(
					stderr,
					exitCodeForError(ExitFilesystemError, err),
					"glippy lint: generate baseline: read %q: %v\n",
					task.file.Path,
					err,
				)
			}
			file, err := source.Load(task.file.Path, input)
			if err != nil {
				return report(
					stderr,
					ExitSourceError,
					"glippy lint: generate baseline: %v\n",
					err,
				)
			}
			analyzed, err := analysis.Run(ctx, file, registry, task.options.analysis)
			if err != nil {
				return report(
					stderr,
					exitCodeForError(ExitInternalError, err),
					"glippy lint: generate baseline: %v\n",
					err,
				)
			}
			inputs = append(
				inputs,
				baseline.InputFile{File: file, Diagnostics: analyzed.Diagnostics},
			)
		}
	}
	document, err := baseline.Generate(root, inputs)
	if err != nil {
		return report(
			stderr,
			ExitInternalError,
			"glippy lint: generate baseline: %v\n",
			err,
		)
	}
	encoded, err := baseline.Encode(document)
	if err != nil {
		return report(
			stderr,
			ExitInternalError,
			"glippy lint: generate baseline: %v\n",
			err,
		)
	}
	if baselineSnapshot != nil {
		err = baselineSnapshot.Replace(encoded)
	} else {
		err = filesystem.CreateWithin(root, baselinePath, encoded, 0o600)
	}
	if err != nil {
		return report(
			stderr,
			exitCodeForError(ExitFilesystemError, err),
			"glippy lint: generate baseline: %v\n",
			err,
		)
	}
	count := 0
	for _, entry := range document.Entries {
		count += entry.Count
	}
	if err := write(
		stdout,
		[]byte(
			fmt.Sprintf(
				"glippy lint: wrote baseline %s (%d diagnostics)\n",
				baselinePath,
				count,
			),
		),
	);
		err != nil {
		return report(
			stderr,
			ExitFilesystemError,
			"glippy lint: write standard output: %v\n",
			err,
		)
	}
	return ExitSuccess
}

func runLintPackageCheck(
	ctx context.Context,
	invocation lintInvocation,
	stdout, stderr io.Writer,
	registry *rules.Registry,
	task lintPackageTask,
	changedScope *changed.Scope,
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
	if err := applyConfiguredPackageBaseline(task, &result, registry); err != nil {
		return reportLintPackageFailure(
			invocation,
			stdout,
			stderr,
			lintBaselineErrorExitCode(err),
			result,
			err,
		)
	}
	if err := filterChangedPackageResult(changedScope, &result); err != nil {
		return reportLintPackageFailure(
			invocation,
			stdout,
			stderr,
			ExitInvalidInvocation,
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
	if invocation.reporter == glippyreport.JSON {
		return reportLintPackageJSON(stdout, stderr, "check", exitCode, true, result, nil)
	}
	inputs, err := packageLintTextInputs(result)
	if err != nil {
		return report(
			stderr,
			ExitInternalError,
			"glippy lint: prepare typed text report: %v\n",
			err,
		)
	}
	if isIntegrationReporter(invocation.reporter) {
		return reportIntegrationOutput(
			"lint",
			invocation.reporter,
			stdout,
			stderr,
			exitCode,
			glippyreport.IntegrationInput{
				Files: inputs,
				PackageDiagnostics: result.LoadDiagnostics,
				SourceProblems: result.SourceProblems,
				Registry: registry,
			},
		)
	}
	output, err := renderPackageLintText(
		invocation.reporter,
		inputs,
		result.LoadDiagnostics,
		result.SourceProblems,
	)
	if err != nil {
		return report(
			stderr,
			ExitInternalError,
			"glippy lint: render typed text report: %v\n",
			err,
		)
	}
	if len(output) > 0 {
		if err := write(stdout, output); err != nil {
			return report(
				stderr,
				ExitFilesystemError,
				"glippy lint: write standard output: %v\n",
				err,
			)
		}
	}
	return exitCode
}

func applyConfiguredBaselines(
	tasks []lintTask,
	inputs []glippyreport.LintTextInput,
	results []analysis.Result,
	registry *rules.Registry,
) error {
	if len(tasks) != len(inputs) || len(tasks) != len(results) {
		return errors.New("baseline inputs do not match lint results")
	}
	type group struct {
		root string
		policy config.Baseline
		indices []int
	}
	groups := make(map[string]*group)
	keys := make([]string, 0)
	for index, task := range tasks {
		policy := task.options.baseline
		if policy.Path == "" {
			continue
		}
		key := task.root +
			"\x00" +
			policy.Path +
			"\x00" +
			policy.ExpiryCutoff +
			"\x00" +
			fmt.Sprint(policy.ReportStale)
		current, found := groups[key]
		if !found {
			current = &group{root: task.root, policy: policy}
			groups[key] = current
			keys = append(keys, key)
		}
		current.indices = append(current.indices, index)
	}
	sort.Strings(keys)
	for _, key := range keys {
		current := groups[key]
		baselinePath := filepath.Join(current.root, filepath.FromSlash(current.policy.Path))
		snapshot, err := filesystem.ReadWithin(current.root, baselinePath)
		if err != nil {
			return &lintBaselineFilesystemError{
				err: fmt.Errorf("read lint baseline %q: %w", baselinePath, err),
			}
		}
		document, err := baseline.Parse(
			baselinePath,
			snapshot.Bytes(),
			baseline.ParseOptions{KnownRules: registry.IDs()},
		)
		if err != nil {
			return err
		}
		baselineInputs := make([]baseline.InputFile, 0, len(current.indices))
		for _, index := range current.indices {
			baselineInputs = append(
				baselineInputs,
				baseline.InputFile{
					File: inputs[index].File,
					Diagnostics: results[index].Diagnostics,
				},
			)
		}
		applied, err := baseline.Apply(
			current.root,
			document,
			baselineInputs,
			baseline.ApplyOptions{
				ReportStale: current.policy.ReportStale,
				ExpiryCutoff: current.policy.ExpiryCutoff,
			},
		)
		if err != nil {
			return err
		}
		indexByPath := make(map[string]int, len(current.indices))
		for offset, appliedFile := range applied.Files {
			index := current.indices[offset]
			results[index].Diagnostics = appliedFile.Diagnostics
			results[index].Baselined = appliedFile.Baselined
			indexByPath[appliedFile.Path] = index
		}
		for _, problem := range applied.Problems {
			path := filepath.Join(current.root, filepath.FromSlash(problem.Entry.Path))
			index, found := indexByPath[path]
			if !found {
				return fmt.Errorf(
					"baseline problem references unanalyzed path %q",
					path,
				)
			}
			results[index].BaselineProblems = append(
				results[index].BaselineProblems,
				problem,
			)
		}
	}
	return nil
}

func applyConfiguredPackageBaseline(
	task lintPackageTask,
	result *analysis.PackageResult,
	registry *rules.Registry,
) error {
	if task.options.baseline.Path == "" {
		return nil
	}
	inputs := make([]glippyreport.LintTextInput, 0, len(result.Files))
	tasks := make([]lintTask, 0, len(result.Files))
	for _, analyzed := range result.Files {
		file, found := result.Sources.Lookup(analyzed.Path)
		if !found {
			return fmt.Errorf("typed lint result source %q is missing", analyzed.Path)
		}
		inputs = append(inputs, glippyreport.LintTextInput{File: file, Result: analyzed})
		tasks = append(tasks, lintTask{root: task.root, options: task.options})
	}
	return applyConfiguredBaselines(tasks, inputs, result.Files, registry)
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
				invocation.only,
				invocation.except,
				invocation.lintLevels,
			)
			if err != nil {
				return nil, exitCode, err
			}
			resolution, err := options.analysis.RuleResolution()
			if err != nil {
				return nil, ExitInvalidInvocation, err
			}
			selected, err := registry.ResolveOptions(resolution)
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

func packageLintTextInputs(result analysis.PackageResult) ([]glippyreport.LintTextInput, error) {
	inputs := make([]glippyreport.LintTextInput, 0, len(result.Files))
	for _, fileResult := range result.Files {
		file, found := result.Sources.Lookup(fileResult.Path)
		if !found {
			return nil, fmt.Errorf(
				"typed lint result source %q is missing",
				fileResult.Path,
			)
		}
		inputs = append(inputs, glippyreport.LintTextInput{File: file, Result: fileResult})
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
	return prepareLintTasksFromPlans(
		ctx,
		plans,
		invocation.configPath,
		registry,
		invocation.only,
		invocation.except,
		invocation.lintLevels,
	)
}

func prepareLintTasksFromPlans(
	ctx context.Context,
	plans []lintInputPlan,
	configPath string,
	registry *rules.Registry,
	only []string,
	except []string,
	lintLevels []rules.LintLevelDirective,
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
				only,
				except,
				lintLevels,
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
	only []string,
	except []string,
	lintLevels []rules.LintLevelDirective,
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
			Presets: loaded.Lint.Presets,
			WarningsAsErrors: loaded.Lint.WarningsAsErrors,
			Overrides: loaded.Lint.Rules,
			RuleOptions: loaded.Lint.RuleOptions,
			LintLevels: cloneLintLevelDirectives(lintLevels),
			Only: slices.Clone(only),
			Except: slices.Clone(except),
			RequireSuppressionReason: loaded.Lint.Suppressions.RequireReason,
			SuppressionExpiryCutoff: loaded.Lint.Suppressions.ExpiryCutoff,
		},
		buildSelection: loaded.Analysis,
		format: glippyformat.Options{
			Width: loaded.Format.LineWidth,
			TabWidth: loaded.Format.TabWidth,
			FitBudget: defaultFormatOptions.FitBudget,
		},
		cache: loaded.Cache,
		configurationDigest: cache.DigestOf(loaded.CanonicalBytes()),
		sourceGoVersion: sourceGoVersion,
		baseline: loaded.Lint.Baseline,
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
	changedScope, exitCode, err := prepareChangedScope(ctx, invocation.newFrom, plans)
	if err != nil {
		return reportLintFixFailure(invocation, stdout, stderr, exitCode, nil, err)
	}
	packageTask, packageMode, exitCode, err := prepareLintPackageTask(plans)
	if err != nil {
		return reportLintFixFailure(invocation, stdout, stderr, exitCode, nil, err)
	}
	var executions []lintFixExecution
	var preview strings.Builder
	var packageOverlay map[string][]byte
	if packageMode {
		if invocation.diff {
			packageOverlay = make(map[string][]byte)
		}
		executions, exitCode, err = prepareLintPackageFixExecutions(
			ctx,
			packageTask,
			registry,
			invocation.selectionOptions(),
			changedScope,
		)
	} else {
		var tasks []lintTask
		tasks, exitCode, err = prepareLintTasksFromPlans(
			ctx,
			plans,
			invocation.configPath,
			registry,
			invocation.only,
			invocation.except,
			invocation.lintLevels,
		)
		if err == nil {
			executions, exitCode, err = prepareLintFixExecutions(
				ctx,
				tasks,
				registry,
				invocation.selectionOptions(),
				changedScope,
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
				changedScope,
				packageOverlay,
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
			if changedScope != nil {
				owned, err := changedScope.OwnsTransformation(
					execution.file,
					formatted.Bytes(),
				)
				if err != nil {
					return err
				}
				if !owned {
					return errors.New(
						"formatted fix changes lines outside --new-from ownership",
					)
				}
			}
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
				if err == nil {
					inputs := []glippyreport.LintTextInput{
						{File: formatted, Result: analyzed},
					}
					results := []analysis.Result{analyzed}
					err = applyConfiguredBaselines(
						[]lintTask{execution.task},
						inputs,
						results,
						registry,
					)
					analyzed = results[0]
				}
			} else {
				analyzed, analyzedFile, err = validateLintPackageFix(
					ctx,
					registry,
					*execution.packageTask,
					formatted,
					packageOverlay,
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
		var transaction fixengine.Transaction
		var transactionErr error
		if invocation.diff {
			transaction, transactionErr = coordinateLintFixPreview(
				execution.snapshot,
				execution.file,
				execution.selections,
				options,
			)
		} else {
			transaction, transactionErr = fixengine.CoordinateAndReplace(
				execution.snapshot,
				execution.selections,
				options,
			)
		}
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
		if invocation.diff &&
			len(transaction.Result.Applied) > 0 &&
			!bytes.Equal(execution.file.Bytes(), transaction.Result.Bytes) {
			if execution.packageTask != nil {
				packageOverlay[execution.file.Path()] = bytes.Clone(
					transaction.Result.Bytes,
				)
			}
			preview.WriteString(
				glippydiff.Unified(
					execution.file.Path() + ".orig",
					execution.file.Path(),
					execution.file.Bytes(),
					transaction.Result.Bytes,
				),
			)
		}
	}
	if packageMode {
		exitCode, err := refreshFinalLintPackageResults(
			ctx,
			registry,
			packageTask,
			executions,
			packageOverlay,
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
	if !invocation.diff {
		finalChangedScope, finalScopeExitCode, finalScopeErr := prepareChangedScope(
			ctx,
			invocation.newFrom,
			plans,
		)
		if finalScopeErr != nil {
			return reportLintFixFailure(
				invocation,
				stdout,
				stderr,
				finalScopeExitCode,
				executions,
				finalScopeErr,
			)
		}
		for index := range executions {
			if err := filterChangedResult(
				finalChangedScope,
				executions[index].resultFile,
				&executions[index].result,
			);
				err != nil {
				return reportLintFixFailure(
					invocation,
					stdout,
					stderr,
					ExitInvalidInvocation,
					executions,
					err,
				)
			}
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
	if invocation.diff {
		previewInputs := make([]glippyreport.LintFixTextInput, 0)
		for _, execution := range executions {
			if len(execution.outcome.Rejected) == 0 {
				continue
			}
			previewInputs = append(
				previewInputs,
				glippyreport.LintFixTextInput{
					File: execution.file,
					ResultFile: execution.resultFile,
					Result: execution.result,
					Outcome: execution.outcome,
				},
			)
		}
		if len(previewInputs) > 0 {
			rejectedOutput, renderErr := renderLintFixText(
				invocation.reporter,
				previewInputs,
			)
			if renderErr != nil {
				return reportLintFixFailure(
					invocation,
					stdout,
					stderr,
					moreSevereExitCode(exitCode, ExitInternalError),
					executions,
					fmt.Errorf("render fix preview report: %w", renderErr),
				)
			}
			preview.Write(rejectedOutput)
		}
		if preview.Len() > 0 {
			if hasLintFixPreviewChanges(executions) {
				exitCode = moreSevereExitCode(exitCode, ExitFindings)
			}
			if err := write(stdout, []byte(preview.String())); err != nil {
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
	if invocation.reporter == glippyreport.JSON {
		return reportLintFixJSON(stdout, stderr, exitCode, true, executions, nil)
	}
	inputs := make([]glippyreport.LintFixTextInput, len(executions))
	integrationInputs := make([]glippyreport.LintTextInput, len(executions))
	for index, execution := range executions {
		inputs[index] = glippyreport.LintFixTextInput{
			File: execution.file,
			ResultFile: execution.resultFile,
			Result: execution.result,
			Outcome: execution.outcome,
		}
		integrationInputs[index] = glippyreport.LintTextInput{
			File: execution.resultFile,
			Result: execution.result,
		}
	}
	if isIntegrationReporter(invocation.reporter) {
		return reportIntegrationOutput(
			"lint",
			invocation.reporter,
			stdout,
			stderr,
			exitCode,
			glippyreport.IntegrationInput{Files: integrationInputs, Registry: registry},
		)
	}
	output, err := renderLintFixText(invocation.reporter, inputs)
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

func hasLintFixPreviewChanges(executions []lintFixExecution) bool {
	for _, execution := range executions {
		if len(execution.outcome.Applied) > 0 &&
			execution.resultFile != nil &&
			!bytes.Equal(execution.file.Bytes(), execution.resultFile.Bytes()) {
			return true
		}
	}
	return false
}

func coordinateLintFixPreview(
	snapshot *filesystem.Snapshot,
	file *source.File,
	selections []fixengine.Selection,
	options fixengine.Options,
) (fixengine.Transaction, error) {
	if snapshot == nil {
		return fixengine.Transaction{
			Status: fixengine.WriteNotPerformed,
		}, errors.New("fix preview requires a filesystem snapshot")
	}
	result, err := fixengine.Coordinate(file, selections, options)
	transaction := fixengine.Transaction{Result: result, Status: fixengine.WriteNotPerformed}
	if err != nil {
		return transaction, err
	}
	if err := snapshot.Validate(); err != nil {
		return transaction, err
	}
	return transaction, nil
}

func prepareLintPackageFixExecutions(
	ctx context.Context,
	task lintPackageTask,
	registry *rules.Registry,
	selectionOptions fixengine.SelectionOptions,
	changedScope *changed.Scope,
) ([]lintFixExecution, int, error) {
	packageResult, err := runUncachedPackageAnalysis(ctx, registry, task, nil)
	if err != nil {
		return nil, packageAnalysisErrorExitCode(err), err
	}
	if err := applyConfiguredPackageBaseline(task, &packageResult, registry); err != nil {
		return nil, lintBaselineErrorExitCode(err), err
	}
	if err := validateLintPackagePrerequisites(packageResult); err != nil {
		return nil, ExitSourceError, err
	}
	if err := filterChangedPackageResult(changedScope, &packageResult); err != nil {
		return nil, ExitInvalidInvocation, err
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
				outcome: glippyreport.LintFixOutcome{
					Path: file.Path(),
					SourceDigest: file.Digest(),
					Status: glippyreport.LintFilePending,
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
	changedScope *changed.Scope,
	overlay map[string][]byte,
) (int, error) {
	packageResult, err := runUncachedPackageAnalysis(
		ctx,
		registry,
		*execution.packageTask,
		overlay,
	)
	if err != nil {
		return packageAnalysisErrorExitCode(err), err
	}
	if err := applyConfiguredPackageBaseline(*execution.packageTask, &packageResult, registry);
		err != nil {
		return lintBaselineErrorExitCode(err), err
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
		if err := filterChangedResult(changedScope, file, &result); err != nil {
			return ExitInvalidInvocation, err
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
		execution.outcome = glippyreport.LintFixOutcome{
			Path: file.Path(),
			SourceDigest: file.Digest(),
			Status: glippyreport.LintFilePending,
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
	overlay map[string][]byte,
) (int, error) {
	packageResult, err := runUncachedPackageAnalysis(ctx, registry, task, overlay)
	if err != nil {
		return packageAnalysisErrorExitCode(err), err
	}
	if err := applyConfiguredPackageBaseline(task, &packageResult, registry); err != nil {
		return lintBaselineErrorExitCode(err), err
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
	baseOverlay map[string][]byte,
) (analysis.Result, *source.File, error) {
	overlay := make(map[string][]byte, len(baseOverlay) + 1)
	for path, input := range baseOverlay {
		overlay[path] = bytes.Clone(input)
	}
	overlay[formatted.Path()] = formatted.Bytes()
	packageResult, err := runUncachedPackageAnalysis(ctx, registry, task, overlay)
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
	changedScope *changed.Scope,
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
		executions = append(
			executions,
			lintFixExecution{
				file: file,
				resultFile: file,
				result: analyzed,
				outcome: glippyreport.LintFixOutcome{
					Path: file.Path(),
					SourceDigest: file.Digest(),
					Status: glippyreport.LintFilePending,
				},
				selections: nil,
				task: task,
				snapshot: snapshot,
			},
		)
	}
	inputs := make([]glippyreport.LintTextInput, len(executions))
	results := make([]analysis.Result, len(executions))
	for index, execution := range executions {
		inputs[index] = glippyreport.LintTextInput{
			File: execution.file,
			Result: execution.result,
		}
		results[index] = execution.result
	}
	if err := applyConfiguredBaselines(tasks, inputs, results, registry); err != nil {
		return executions, lintBaselineErrorExitCode(err), err
	}
	for index := range executions {
		executions[index].result = results[index]
		if err := filterChangedResult(
			changedScope,
			executions[index].file,
			&executions[index].result,
		);
			err != nil {
			return executions, ExitInvalidInvocation, err
		}
		selections, err := fixengine.Select(
			executions[index].result.Diagnostics,
			selectionOptions,
		)
		if err != nil {
			return executions, ExitInternalError, err
		}
		executions[index].selections = selections
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
		execution.outcome.Status = glippyreport.LintFileConflict
		return
	}
	execution.result = postResult
	execution.resultFile = postFile
	execution.outcome.Status = lintFixFileStatus(transaction)
}

func lintFixFileStatus(transaction fixengine.Transaction) glippyreport.LintFileStatus {
	if transaction.Status == fixengine.WritePossiblyCompleted {
		return glippyreport.LintFilePossiblyFixed
	}
	if transaction.Status == fixengine.WriteCompleted {
		return glippyreport.LintFileFixed
	}
	for _, rejected := range transaction.Result.Rejected {
		if rejected.Reason == fixengine.RejectionConflict {
			return glippyreport.LintFileConflict
		}
	}
	return glippyreport.LintFileUnchanged
}

func lintFixExitCode(executions []lintFixExecution) int {
	exitCode := ExitSuccess
	results := make([]analysis.Result, len(executions))
	for index, execution := range executions {
		results[index] = execution.result
		if execution.outcome.Status == glippyreport.LintFileConflict {
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
			len(result.BaselineProblems) > 0 ||
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

func lintBaselineErrorExitCode(err error) int {
	var filesystemError *lintBaselineFilesystemError
	if errors.As(err, &filesystemError) {
		return exitCodeForError(ExitFilesystemError, err)
	}
	return exitCodeForError(ExitInvalidInvocation, err)
}

func reportLintFailure(
	invocation lintInvocation,
	stdout, stderr io.Writer,
	exitCode int,
	inputs []glippyreport.LintTextInput,
	err error,
) int {
	results := make([]analysis.Result, len(inputs))
	for index, input := range inputs {
		results[index] = input.Result
	}
	if invocation.reporter == glippyreport.JSON {
		return reportLintJSON(stdout, stderr, "check", exitCode, false, results, err)
	}
	if isIntegrationReporter(invocation.reporter) {
		return reportIntegrationOutput(
			"lint",
			invocation.reporter,
			stdout,
			stderr,
			exitCode,
			glippyreport.IntegrationInput{Files: inputs, Errors: integrationError(err)},
		)
	}
	return report(stderr, exitCode, "glippy lint: %v\n", err)
}

func reportLintPackageFailure(
	invocation lintInvocation,
	stdout, stderr io.Writer,
	exitCode int,
	result analysis.PackageResult,
	err error,
) int {
	if invocation.reporter == glippyreport.JSON {
		return reportLintPackageJSON(stdout, stderr, "check", exitCode, false, result, err)
	}
	if isIntegrationReporter(invocation.reporter) {
		inputs, inputErr := packageLintTextInputs(result)
		if inputErr != nil {
			if err == nil {
				err = inputErr
			}
			inputs = nil
		}
		return reportIntegrationOutput(
			"lint",
			invocation.reporter,
			stdout,
			stderr,
			exitCode,
			glippyreport.IntegrationInput{
				Files: inputs,
				PackageDiagnostics: result.LoadDiagnostics,
				SourceProblems: result.SourceProblems,
				Errors: integrationError(err),
			},
		)
	}
	return report(stderr, exitCode, "glippy lint: %v\n", err)
}

func reportLintFixFailure(
	invocation lintInvocation,
	stdout, stderr io.Writer,
	exitCode int,
	executions []lintFixExecution,
	err error,
) int {
	if invocation.reporter == glippyreport.JSON {
		return reportLintFixJSON(stdout, stderr, exitCode, false, executions, err)
	}
	if isIntegrationReporter(invocation.reporter) {
		inputs := make([]glippyreport.LintTextInput, 0, len(executions))
		for _, execution := range executions {
			if execution.resultFile == nil {
				continue
			}
			inputs = append(
				inputs,
				glippyreport.LintTextInput{
					File: execution.resultFile,
					Result: execution.result,
				},
			)
		}
		return reportIntegrationOutput(
			"lint",
			invocation.reporter,
			stdout,
			stderr,
			exitCode,
			glippyreport.IntegrationInput{Files: inputs, Errors: integrationError(err)},
		)
	}
	paths, possibly := completedLintFixPaths(executions)
	if len(paths) == 0 {
		return report(stderr, exitCode, "glippy lint: %v\n", err)
	}
	heading := "files fixed before failure"
	if possibly {
		heading = "files fixed or possibly fixed before failure"
	}
	return report(
		stderr,
		exitCode,
		"glippy lint: %v\nglippy lint: %s:\n%s\n",
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
	errors_ := []glippyreport.Error{}
	if err != nil {
		errors_ = append(errors_, glippyreport.Error{Message: err.Error()})
	}
	result, buildErr := glippyreport.NewLintResult(
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
			"glippy lint: build JSON report: %v\n",
			buildErr,
		)
	}
	encoded, encodeErr := glippyreport.MarshalLintJSON(result)
	if encodeErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"glippy lint: encode JSON report: %v\n",
			encodeErr,
		)
	}
	if writeErr := write(stdout, encoded); writeErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitFilesystemError),
			"glippy lint: write JSON report: %v\n",
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
	errors_ := []glippyreport.Error{}
	if err != nil {
		errors_ = append(errors_, glippyreport.Error{Message: err.Error()})
	}
	result, buildErr := glippyreport.NewPackageLintResult(
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
			"glippy lint: build typed JSON report: %v\n",
			buildErr,
		)
	}
	encoded, encodeErr := glippyreport.MarshalLintJSON(result)
	if encodeErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"glippy lint: encode typed JSON report: %v\n",
			encodeErr,
		)
	}
	if writeErr := write(stdout, encoded); writeErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitFilesystemError),
			"glippy lint: write typed JSON report: %v\n",
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
	outcomes := make([]glippyreport.LintFixOutcome, len(executions))
	for index, execution := range executions {
		results[index] = execution.result
		outcomes[index] = execution.outcome
	}
	errors_ := []glippyreport.Error{}
	if err != nil {
		errors_ = append(errors_, glippyreport.Error{Message: err.Error()})
	}
	result, buildErr := glippyreport.NewLintFixResult(
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
	encoded, encodeErr := glippyreport.MarshalLintJSON(result)
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
		return report(stderr, exitCode, "glippy lint: %s: %v\n", action, err)
	}
	heading := "files fixed before reporting failure"
	if possibly {
		heading = "files fixed or possibly fixed before reporting failure"
	}
	return report(
		stderr,
		exitCode,
		"glippy lint: %s: %v\nglippy lint: %s:\n%s\n",
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
		case glippyreport.LintFileFixed:
			paths = append(paths, execution.outcome.Path)
		case glippyreport.LintFilePossiblyFixed:
			paths = append(paths, execution.outcome.Path)
			possibly = true
		}
	}
	sort.Strings(paths)
	return paths, possibly
}
