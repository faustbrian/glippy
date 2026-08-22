package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/changed"
	glippyformat "github.com/faustbrian/glippy/internal/format"
	glippyreport "github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

const checkUsage = "glippy: expected 'check [-A|--allow <rules-or-groups>] [-W|--warn <rules-or-groups>] [-D|--deny <rules-or-groups>] [-F|--forbid <rules-or-groups>] [--new-from=<git-ref>] [--reporter=text|short|json|github|sarif] [--stats[=text|json]] [--config=<path>] [path...]'\n"

type checkInvocation struct {
	configPath string
	newFrom string
	lintLevels []rules.LintLevelDirective
	paths []string
	reporter glippyreport.Format
	statistics lintStatisticsFormat
}

type checkExecution struct {
	file *source.File
	analysis analysis.Result
	formatChanged bool
	formatPreexisting bool
}

func parseCheckInvocation(arguments []string) (checkInvocation, bool) {
	if len(arguments) == 0 || arguments[0] != "check" {
		return checkInvocation{}, false
	}
	result := checkInvocation{reporter: glippyreport.Text}
	reporterSet := false
	statisticsSet := false
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		if directive, consumed, matched, valid := parseLintLevelDirective(arguments, index);
			matched {
			if !valid {
				return checkInvocation{}, false
			}
			result.lintLevels = append(result.lintLevels, directive)
			index += consumed
			continue
		}
		switch {
		case argument == "--stats" && !statisticsSet:
			result.statistics = lintStatisticsText
			statisticsSet = true
		case strings.HasPrefix(argument, "--stats=") && !statisticsSet:
			value := lintStatisticsFormat(strings.TrimPrefix(argument, "--stats="))
			if value != lintStatisticsText && value != lintStatisticsJSON {
				return checkInvocation{}, false
			}
			result.statistics = value
			statisticsSet = true
		case strings.HasPrefix(argument, "--new-from=") && result.newFrom == "":
			result.newFrom = strings.TrimPrefix(argument, "--new-from=")
			if result.newFrom == "" {
				return checkInvocation{}, false
			}
		case argument == "--new-from" &&
			result.newFrom == "" &&
			index + 1 < len(arguments) &&
			!strings.HasPrefix(arguments[index + 1], "--"):
			index++
			result.newFrom = arguments[index]
		case strings.HasPrefix(argument, "--reporter=") && !reporterSet:
			reporter, valid := parseDiagnosticReporter(
				strings.TrimPrefix(argument, "--reporter="),
			)
			if !valid {
				return checkInvocation{}, false
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
				return checkInvocation{}, false
			}
			result.reporter = reporter
			reporterSet = true
		case strings.HasPrefix(argument, "--config=") && result.configPath == "":
			result.configPath = strings.TrimPrefix(argument, "--config=")
			if result.configPath == "" {
				return checkInvocation{}, false
			}
		case argument == "--config" &&
			result.configPath == "" &&
			index + 1 < len(arguments) &&
			!strings.HasPrefix(arguments[index + 1], "--"):
			index++
			result.configPath = arguments[index]
			if result.configPath == "" {
				return checkInvocation{}, false
			}
		case !strings.HasPrefix(argument, "-"):
			result.paths = append(result.paths, argument)
		default:
			return checkInvocation{}, false
		}
	}
	if len(result.paths) == 0 {
		result.paths = []string{"."}
	}
	return result, true
}

func classifyChangedFormat(
	scope *changed.Scope,
	file *source.File,
	formatted []byte,
) (bool, bool, error) {
	if file == nil {
		return false, false, errors.New("format classification requires a source file")
	}
	if bytes.Equal(file.Bytes(), formatted) {
		return false, false, nil
	}
	if scope == nil {
		return true, false, nil
	}
	owned, err := scope.OwnsTransformation(file, formatted)
	if err != nil {
		return false, false, err
	}
	if owned {
		return true, false, nil
	}
	return false, true, nil
}

func runCombinedCheck(
	ctx context.Context,
	invocation checkInvocation,
	stdout, stderr io.Writer,
	registry *rules.Registry,
) int {
	if ctx == nil {
		return reportCombinedCheck(
			invocation,
			stdout,
			stderr,
			ExitInternalError,
			false,
			nil,
			errors.New("context is required"),
		)
	}
	if registry == nil {
		return reportCombinedCheck(
			invocation,
			stdout,
			stderr,
			ExitInternalError,
			false,
			nil,
			errors.New("rule registry is required"),
		)
	}
	if err := ctx.Err(); err != nil {
		return reportCombinedCheck(
			invocation,
			stdout,
			stderr,
			ExitCanceled,
			false,
			nil,
			err,
		)
	}
	plans, exitCode, err := prepareLintInputPlans(
		ctx,
		lintInvocation{
			configPath: invocation.configPath,
			newFrom: invocation.newFrom,
			lintLevels: invocation.lintLevels,
			paths: invocation.paths,
			reporter: invocation.reporter,
		},
		registry,
	)
	if err != nil {
		return reportCombinedCheck(invocation, stdout, stderr, exitCode, false, nil, err)
	}
	changedScope, exitCode, err := prepareChangedScope(ctx, invocation.newFrom, plans)
	if err != nil {
		return reportCombinedCheck(invocation, stdout, stderr, exitCode, false, nil, err)
	}
	packageTask, packageMode, exitCode, err := prepareLintPackageTask(plans)
	if err != nil {
		return reportCombinedCheck(invocation, stdout, stderr, exitCode, false, nil, err)
	}
	var statistics *analysis.Statistics
	if invocation.statistics != lintStatisticsNone {
		statistics = analysis.NewStatistics()
	}
	if packageMode {
		packageTask.options.analysis.Statistics = statistics
		return runCombinedPackageCheck(
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
		nil,
		nil,
		invocation.lintLevels,
	)
	if err != nil {
		return reportCombinedCheck(invocation, stdout, stderr, exitCode, false, nil, err)
	}
	for index := range tasks {
		tasks[index].options.analysis.Statistics = statistics
	}
	executions := make([]checkExecution, 0, len(tasks))
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			return reportCombinedCheck(
				invocation,
				stdout,
				stderr,
				ExitCanceled,
				false,
				executions,
				err,
			)
		}
		input, err := source.ReadFile(task.file.Path)
		if err != nil {
			return reportCombinedCheck(
				invocation,
				stdout,
				stderr,
				exitCodeForError(ExitFilesystemError, err),
				false,
				executions,
				fmt.Errorf("read %q: %w", task.file.Path, err),
			)
		}
		file, err := source.Load(task.file.Path, input)
		if err != nil {
			return reportCombinedCheck(
				invocation,
				stdout,
				stderr,
				ExitSourceError,
				false,
				executions,
				err,
			)
		}
		formatted, err := glippyformat.File(file, task.options.format)
		if err != nil {
			return reportCombinedCheck(
				invocation,
				stdout,
				stderr,
				ExitInternalError,
				false,
				executions,
				fmt.Errorf("format %q: %w", task.file.Path, err),
			)
		}
		formatChanged, formatPreexisting, err := classifyChangedFormat(
			changedScope,
			file,
			formatted,
		)
		if err != nil {
			return reportCombinedCheck(
				invocation,
				stdout,
				stderr,
				ExitInvalidInvocation,
				false,
				executions,
				err,
			)
		}
		analyzed, err := analysis.Run(ctx, file, registry, task.options.analysis)
		if err != nil {
			return reportCombinedCheck(
				invocation,
				stdout,
				stderr,
				exitCodeForError(ExitInternalError, err),
				false,
				executions,
				fmt.Errorf("analyze %q: %w", task.file.Path, err),
			)
		}
		executions = append(
			executions,
			checkExecution{
				file: file,
				analysis: analyzed,
				formatChanged: formatChanged,
				formatPreexisting: formatPreexisting,
			},
		)
	}
	baselineInputs := make([]glippyreport.LintTextInput, len(executions))
	baselineResults := make([]analysis.Result, len(executions))
	for index, execution := range executions {
		baselineInputs[index] = glippyreport.LintTextInput{
			File: execution.file,
			Result: execution.analysis,
		}
		baselineResults[index] = execution.analysis
	}
	if err := applyConfiguredBaselines(tasks, baselineInputs, baselineResults, registry);
		err != nil {
		return reportCombinedCheck(
			invocation,
			stdout,
			stderr,
			lintBaselineErrorExitCode(err),
			false,
			executions,
			err,
		)
	}
	for index := range executions {
		executions[index].analysis = baselineResults[index]
		if err := filterChangedResult(
			changedScope,
			executions[index].file,
			&executions[index].analysis,
		);
			err != nil {
			return reportCombinedCheck(
				invocation,
				stdout,
				stderr,
				ExitInvalidInvocation,
				false,
				executions,
				err,
			)
		}
	}
	if invocation.reporter == glippyreport.JSON {
		exitCode = ExitSuccess
		for _, execution := range executions {
			if execution.formatChanged ||
				lintResultExitCode([]analysis.Result{execution.analysis}) ==
					ExitFindings {
				exitCode = ExitFindings
			}
		}
		reported := reportCombinedCheck(
			invocation,
			stdout,
			stderr,
			exitCode,
			true,
			executions,
			nil,
		)
		if reported != exitCode {
			return reported
		}
		return reportLintStatistics(
			stderr,
			invocation.statistics,
			"check",
			statistics,
			checkAnalysisResults(executions),
			exitCode,
		)
	}
	if isIntegrationReporter(invocation.reporter) {
		exitCode = ExitSuccess
		inputs := make([]glippyreport.LintTextInput, len(executions))
		formats := make([]glippyreport.CheckFormatOutcome, len(executions))
		for index, execution := range executions {
			inputs[index] = glippyreport.LintTextInput{
				File: execution.file,
				Result: execution.analysis,
			}
			formats[index] = glippyreport.CheckFormatOutcome{
				Path: execution.file.Path(),
				Digest: execution.file.Digest(),
				Different: execution.formatChanged,
				Preexisting: execution.formatPreexisting,
			}
			if execution.formatChanged ||
				lintResultExitCode([]analysis.Result{execution.analysis}) ==
					ExitFindings {
				exitCode = ExitFindings
			}
		}
		reported := reportIntegrationOutput(
			"check",
			invocation.reporter,
			stdout,
			stderr,
			exitCode,
			glippyreport.IntegrationInput{
				Files: inputs,
				Formats: formats,
				Registry: registry,
			},
		)
		if reported != exitCode {
			return reported
		}
		return reportLintStatistics(
			stderr,
			invocation.statistics,
			"check",
			statistics,
			checkAnalysisResults(executions),
			exitCode,
		)
	}
	var output bytes.Buffer
	exitCode = ExitSuccess
	for _, execution := range executions {
		if execution.formatChanged {
			fmt.Fprintf(&output, "%s: format differs\n", execution.file.Path())
			exitCode = ExitFindings
		}
		lintOutput, err := renderLintText(
			invocation.reporter,
			[]glippyreport.LintTextInput{
				{File: execution.file, Result: execution.analysis},
			},
		)
		if err != nil {
			return report(
				stderr,
				ExitInternalError,
				"glippy check: render lint report: %v\n",
				err,
			)
		}
		if len(lintOutput) > 0 {
			output.Write(lintOutput)
		}
		if lintResultExitCode([]analysis.Result{execution.analysis}) == ExitFindings {
			exitCode = ExitFindings
		}
	}
	if err := ctx.Err(); err != nil {
		return reportCombinedCheck(
			invocation,
			stdout,
			stderr,
			ExitCanceled,
			false,
			executions,
			err,
		)
	}
	if output.Len() > 0 {
		if err := write(stdout, output.Bytes()); err != nil {
			return report(
				stderr,
				moreSevereExitCode(exitCode, ExitFilesystemError),
				"glippy check: write standard output: %v\n",
				err,
			)
		}
	}
	return reportLintStatistics(
		stderr,
		invocation.statistics,
		"check",
		statistics,
		checkAnalysisResults(executions),
		exitCode,
	)
}

func checkAnalysisResults(executions []checkExecution) []analysis.Result {
	results := make([]analysis.Result, len(executions))
	for index, execution := range executions {
		results[index] = execution.analysis
	}
	return results
}

func runCombinedPackageCheck(
	ctx context.Context,
	invocation checkInvocation,
	stdout, stderr io.Writer,
	registry *rules.Registry,
	task lintPackageTask,
	changedScope *changed.Scope,
) int {
	result, err := runConfiguredPackageAnalysis(ctx, registry, task)
	if err != nil {
		exitCode := packageAnalysisErrorExitCode(err)
		if baselineErr := applyConfiguredPackageBaselineMode(task, &result, registry, true);
			baselineErr != nil {
			exitCode = moreSevereExitCode(
				exitCode,
				lintBaselineErrorExitCode(baselineErr),
			)
			err = errors.Join(
				err,
				fmt.Errorf("apply baseline to partial analysis: %w", baselineErr),
			)
		}
		if changedErr := filterChangedPackageResult(changedScope, &result);
			changedErr != nil {
			exitCode = moreSevereExitCode(exitCode, ExitInvalidInvocation)
			err = errors.Join(
				err,
				fmt.Errorf("filter partial changed-code analysis: %w", changedErr),
			)
		}
		return reportCombinedPackageCheck(
			invocation,
			registry,
			stdout,
			stderr,
			exitCode,
			false,
			result,
			nil,
			err,
		)
	}
	if err := applyConfiguredPackageBaseline(task, &result, registry); err != nil {
		return reportCombinedPackageCheck(
			invocation,
			registry,
			stdout,
			stderr,
			lintBaselineErrorExitCode(err),
			false,
			result,
			nil,
			err,
		)
	}
	if err := filterChangedPackageResult(changedScope, &result); err != nil {
		return reportCombinedPackageCheck(
			invocation,
			registry,
			stdout,
			stderr,
			ExitInvalidInvocation,
			false,
			result,
			nil,
			err,
		)
	}
	executions := make([]checkExecution, 0, len(result.Files))
	for _, analyzed := range result.Files {
		if err := ctx.Err(); err != nil {
			return reportCombinedPackageCheck(
				invocation,
				registry,
				stdout,
				stderr,
				ExitCanceled,
				false,
				result,
				executions,
				err,
			)
		}
		file, found := result.Sources.Lookup(analyzed.Path)
		if !found || file.Digest() != analyzed.Digest {
			err := fmt.Errorf(
				"package analysis source identity is unavailable for %q",
				analyzed.Path,
			)
			return reportCombinedPackageCheck(
				invocation,
				registry,
				stdout,
				stderr,
				ExitInternalError,
				false,
				result,
				executions,
				err,
			)
		}
		formatted, err := glippyformat.File(file, task.options.format)
		if err != nil {
			return reportCombinedPackageCheck(
				invocation,
				registry,
				stdout,
				stderr,
				ExitInternalError,
				false,
				result,
				executions,
				fmt.Errorf("format %q: %w", file.Path(), err),
			)
		}
		formatChanged, formatPreexisting, err := classifyChangedFormat(
			changedScope,
			file,
			formatted,
		)
		if err != nil {
			return reportCombinedPackageCheck(
				invocation,
				registry,
				stdout,
				stderr,
				ExitInvalidInvocation,
				false,
				result,
				executions,
				err,
			)
		}
		executions = append(
			executions,
			checkExecution{
				file: file,
				analysis: analyzed,
				formatChanged: formatChanged,
				formatPreexisting: formatPreexisting,
			},
		)
	}
	if err := ctx.Err(); err != nil {
		return reportCombinedPackageCheck(
			invocation,
			registry,
			stdout,
			stderr,
			ExitCanceled,
			false,
			result,
			executions,
			err,
		)
	}
	exitCode := lintPackageResultExitCode(result)
	for _, execution := range executions {
		if execution.formatChanged {
			exitCode = moreSevereExitCode(exitCode, ExitFindings)
		}
	}
	reported := reportCombinedPackageCheck(
		invocation,
		registry,
		stdout,
		stderr,
		exitCode,
		true,
		result,
		executions,
		nil,
	)
	if reported != exitCode {
		return reported
	}
	return reportLintStatistics(
		stderr,
		invocation.statistics,
		"check",
		task.options.analysis.Statistics,
		result.Files,
		exitCode,
	)
}

func reportCombinedPackageCheck(
	invocation checkInvocation,
	registry *rules.Registry,
	stdout, stderr io.Writer,
	exitCode int,
	complete bool,
	result analysis.PackageResult,
	executions []checkExecution,
	err error,
) int {
	formats := packageCheckFormatOutcomes(result.Files, executions)
	if invocation.reporter == glippyreport.JSON {
		errs := []glippyreport.Error{}
		if err != nil {
			errs = append(errs, glippyreport.Error{Message: err.Error()})
		}
		reportResult, reportErr := glippyreport.NewPackageCheckResult(
			exitCategory(exitCode),
			exitCode,
			complete,
			result,
			formats,
			errs,
		)
		if reportErr != nil {
			return report(
				stderr,
				moreSevereExitCode(exitCode, ExitInternalError),
				"glippy check: construct typed JSON report: %v\n",
				reportErr,
			)
		}
		encoded, reportErr := glippyreport.MarshalCheckJSON(reportResult)
		if reportErr != nil {
			return report(
				stderr,
				moreSevereExitCode(exitCode, ExitInternalError),
				"glippy check: encode typed JSON report: %v\n",
				reportErr,
			)
		}
		if reportErr := write(stdout, encoded); reportErr != nil {
			return report(
				stderr,
				moreSevereExitCode(exitCode, ExitFilesystemError),
				"glippy check: write JSON report: %v\n",
				reportErr,
			)
		}
		return exitCode
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
			"check",
			invocation.reporter,
			stdout,
			stderr,
			exitCode,
			glippyreport.IntegrationInput{
				Files: inputs,
				Formats: formats,
				PackageDiagnostics: result.LoadDiagnostics,
				SourceProblems: result.SourceProblems,
				Errors: integrationError(err),
				Registry: registry,
			},
		)
	}
	if err != nil {
		return report(stderr, exitCode, "glippy check: %v\n", err)
	}
	inputs, err := packageLintTextInputs(result)
	if err != nil {
		return report(
			stderr,
			ExitInternalError,
			"glippy check: prepare typed text report: %v\n",
			err,
		)
	}
	lintOutput, err := renderPackageLintText(
		invocation.reporter,
		inputs,
		result.LoadDiagnostics,
		result.SourceProblems,
	)
	if err != nil {
		return report(
			stderr,
			ExitInternalError,
			"glippy check: render typed text report: %v\n",
			err,
		)
	}
	var output bytes.Buffer
	for _, execution := range executions {
		if execution.formatChanged {
			fmt.Fprintf(&output, "%s: format differs\n", execution.file.Path())
		}
	}
	output.Write(lintOutput)
	if output.Len() > 0 {
		if err := write(stdout, output.Bytes()); err != nil {
			return report(
				stderr,
				moreSevereExitCode(exitCode, ExitFilesystemError),
				"glippy check: write standard output: %v\n",
				err,
			)
		}
	}
	return exitCode
}

func packageCheckFormatOutcomes(
	files []analysis.Result,
	executions []checkExecution,
) []glippyreport.CheckFormatOutcome {
	executionsByPath := make(map[string]checkExecution, len(executions))
	for _, execution := range executions {
		executionsByPath[execution.file.Path()] = execution
	}
	formats := make([]glippyreport.CheckFormatOutcome, len(files))
	for index, analyzed := range files {
		execution, formatted := executionsByPath[analyzed.Path]
		formats[index] = glippyreport.CheckFormatOutcome{
			Path: analyzed.Path,
			Digest: analyzed.Digest,
			Pending: !formatted,
		}
		if formatted {
			formats[index].Different = execution.formatChanged
			formats[index].Preexisting = execution.formatPreexisting
		}
	}
	return formats
}

func reportInvalidCheckInvocation(arguments []string, stdout, stderr io.Writer) int {
	invocation := checkInvocation{reporter: glippyreport.Text}
	reporter, requested := requestedDiagnosticReporter(arguments, "check")
	if !requested {
		return report(stderr, ExitInvalidInvocation, checkUsage)
	}
	invocation.reporter = reporter
	return reportCombinedCheck(
		invocation,
		stdout,
		stderr,
		ExitInvalidInvocation,
		false,
		nil,
		errors.New(strings.TrimSpace(checkUsage)),
	)
}

func reportCombinedCheck(
	invocation checkInvocation,
	stdout, stderr io.Writer,
	exitCode int,
	complete bool,
	executions []checkExecution,
	err error,
) int {
	if isIntegrationReporter(invocation.reporter) {
		inputs := make([]glippyreport.LintTextInput, len(executions))
		formats := make([]glippyreport.CheckFormatOutcome, len(executions))
		for index, execution := range executions {
			inputs[index] = glippyreport.LintTextInput{
				File: execution.file,
				Result: execution.analysis,
			}
			formats[index] = glippyreport.CheckFormatOutcome{
				Path: execution.file.Path(),
				Digest: execution.file.Digest(),
				Different: execution.formatChanged,
				Preexisting: execution.formatPreexisting,
			}
		}
		return reportIntegrationOutput(
			"check",
			invocation.reporter,
			stdout,
			stderr,
			exitCode,
			glippyreport.IntegrationInput{
				Files: inputs,
				Formats: formats,
				Errors: integrationError(err),
			},
		)
	}
	if invocation.reporter != glippyreport.JSON {
		if err == nil {
			return exitCode
		}
		return report(stderr, exitCode, "glippy check: %v\n", err)
	}
	analyses := make([]analysis.Result, len(executions))
	formats := make([]glippyreport.CheckFormatOutcome, len(executions))
	for index, execution := range executions {
		analyses[index] = execution.analysis
		formats[index] = glippyreport.CheckFormatOutcome{
			Path: execution.file.Path(),
			Digest: execution.file.Digest(),
			Different: execution.formatChanged,
			Preexisting: execution.formatPreexisting,
		}
	}
	errs := []glippyreport.Error{}
	if err != nil {
		errs = append(errs, glippyreport.Error{Message: err.Error()})
	}
	result, resultErr := glippyreport.NewCheckResult(
		exitCategory(exitCode),
		exitCode,
		complete,
		analyses,
		formats,
		errs,
	)
	if resultErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"glippy check: construct JSON report: %v\n",
			resultErr,
		)
	}
	encoded, resultErr := glippyreport.MarshalCheckJSON(result)
	if resultErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitInternalError),
			"glippy check: encode JSON report: %v\n",
			resultErr,
		)
	}
	if resultErr := write(stdout, encoded); resultErr != nil {
		return report(
			stderr,
			moreSevereExitCode(exitCode, ExitFilesystemError),
			"glippy check: write JSON report: %v\n",
			resultErr,
		)
	}
	return exitCode
}
