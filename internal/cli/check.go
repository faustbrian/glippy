package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/faustbrian/gox/internal/analysis"
	goxformat "github.com/faustbrian/gox/internal/format"
	goxreport "github.com/faustbrian/gox/internal/report"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

const checkUsage = "gox: expected 'check [--reporter=text|json] [--config=<path>] [path...]'\n"

type checkInvocation struct {
	configPath string
	paths      []string
	reporter   goxreport.Format
}

type checkExecution struct {
	file          *source.File
	analysis      analysis.Result
	formatChanged bool
}

func parseCheckInvocation(arguments []string) (checkInvocation, bool) {
	if len(arguments) == 0 || arguments[0] != "check" {
		return checkInvocation{}, false
	}
	result := checkInvocation{reporter: goxreport.Text}
	reporterSet := false
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case strings.HasPrefix(argument, "--reporter=") && !reporterSet:
			reporter, valid := parseReporter(strings.TrimPrefix(argument, "--reporter="))
			if !valid {
				return checkInvocation{}, false
			}
			result.reporter = reporter
			reporterSet = true
		case argument == "--reporter" && !reporterSet &&
			index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "--"):
			index++
			reporter, valid := parseReporter(arguments[index])
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
		case argument == "--config" && result.configPath == "" &&
			index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "--"):
			index++
			result.configPath = arguments[index]
			if result.configPath == "" {
				return checkInvocation{}, false
			}
		case !strings.HasPrefix(argument, "-") && !strings.Contains(argument, "..."):
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

func requestsCheckJSONReporter(arguments []string) bool {
	if len(arguments) == 0 || arguments[0] != "check" {
		return false
	}
	for index, argument := range arguments {
		if argument == "--reporter=json" ||
			(argument == "--reporter" && index+1 < len(arguments) && arguments[index+1] == "json") {
			return true
		}
	}
	return false
}

func runCombinedCheck(
	ctx context.Context,
	invocation checkInvocation,
	stdout, stderr io.Writer,
	registry *rules.Registry,
) int {
	if ctx == nil {
		return reportCombinedCheck(invocation, stdout, stderr, ExitInternalError, false, nil, errors.New("context is required"))
	}
	if registry == nil {
		return reportCombinedCheck(invocation, stdout, stderr, ExitInternalError, false, nil, errors.New("rule registry is required"))
	}
	if err := ctx.Err(); err != nil {
		return reportCombinedCheck(invocation, stdout, stderr, ExitCanceled, false, nil, err)
	}
	tasks, exitCode, err := prepareLintTasks(ctx, lintInvocation{
		configPath: invocation.configPath,
		paths:      invocation.paths,
		reporter:   invocation.reporter,
	}, registry)
	if err != nil {
		return reportCombinedCheck(invocation, stdout, stderr, exitCode, false, nil, err)
	}
	executions := make([]checkExecution, 0, len(tasks))
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			return reportCombinedCheck(invocation, stdout, stderr, ExitCanceled, false, executions, err)
		}
		input, err := os.ReadFile(task.file.Path)
		if err != nil {
			return reportCombinedCheck(invocation, stdout, stderr, ExitFilesystemError, false, executions, fmt.Errorf("read %q: %w", task.file.Path, err))
		}
		file, err := source.Load(task.file.Path, input)
		if err != nil {
			return reportCombinedCheck(invocation, stdout, stderr, ExitSourceError, false, executions, err)
		}
		formatted, err := goxformat.File(file, task.options.format)
		if err != nil {
			return reportCombinedCheck(invocation, stdout, stderr, ExitInternalError, false, executions, fmt.Errorf("format %q: %w", task.file.Path, err))
		}
		analyzed, err := analysis.Run(ctx, file, registry, task.options.analysis)
		if err != nil {
			return reportCombinedCheck(invocation, stdout, stderr, exitCodeForError(ExitInternalError, err), false, executions, fmt.Errorf("analyze %q: %w", task.file.Path, err))
		}
		executions = append(executions, checkExecution{
			file:          file,
			analysis:      analyzed,
			formatChanged: !bytes.Equal(input, formatted),
		})
	}
	if invocation.reporter == goxreport.JSON {
		exitCode = ExitSuccess
		for _, execution := range executions {
			if execution.formatChanged || lintResultExitCode([]analysis.Result{execution.analysis}) == ExitFindings {
				exitCode = ExitFindings
			}
		}
		return reportCombinedCheck(invocation, stdout, stderr, exitCode, true, executions, nil)
	}
	var output bytes.Buffer
	exitCode = ExitSuccess
	for _, execution := range executions {
		if execution.formatChanged {
			fmt.Fprintf(&output, "%s: format differs\n", execution.file.Path())
			exitCode = ExitFindings
		}
		lintOutput, err := goxreport.RenderLintText([]goxreport.LintTextInput{{
			File:   execution.file,
			Result: execution.analysis,
		}})
		if err != nil {
			return report(stderr, ExitInternalError, "gox check: render lint report: %v\n", err)
		}
		if len(lintOutput) > 0 {
			output.Write(lintOutput)
		}
		if lintResultExitCode([]analysis.Result{execution.analysis}) == ExitFindings {
			exitCode = ExitFindings
		}
	}
	if err := ctx.Err(); err != nil {
		return reportCombinedCheck(invocation, stdout, stderr, ExitCanceled, false, executions, err)
	}
	if output.Len() > 0 {
		if err := write(stdout, output.Bytes()); err != nil {
			return report(stderr, moreSevereExitCode(exitCode, ExitFilesystemError), "gox check: write standard output: %v\n", err)
		}
	}
	return exitCode
}

func reportInvalidCheckInvocation(arguments []string, stdout, stderr io.Writer) int {
	invocation := checkInvocation{reporter: goxreport.Text}
	if requestsCheckJSONReporter(arguments) {
		invocation.reporter = goxreport.JSON
	} else {
		return report(stderr, ExitInvalidInvocation, checkUsage)
	}
	return reportCombinedCheck(invocation, stdout, stderr, ExitInvalidInvocation, false, nil, errors.New(strings.TrimSpace(checkUsage)))
}

func reportCombinedCheck(
	invocation checkInvocation,
	stdout, stderr io.Writer,
	exitCode int,
	complete bool,
	executions []checkExecution,
	err error,
) int {
	if invocation.reporter != goxreport.JSON {
		if err == nil {
			return exitCode
		}
		return report(stderr, exitCode, "gox check: %v\n", err)
	}
	analyses := make([]analysis.Result, len(executions))
	formats := make([]goxreport.CheckFormatOutcome, len(executions))
	for index, execution := range executions {
		analyses[index] = execution.analysis
		formats[index] = goxreport.CheckFormatOutcome{
			Path:      execution.file.Path(),
			Digest:    execution.file.Digest(),
			Different: execution.formatChanged,
		}
	}
	errs := []goxreport.Error{}
	if err != nil {
		errs = append(errs, goxreport.Error{Message: err.Error()})
	}
	result, resultErr := goxreport.NewCheckResult(
		exitCategory(exitCode),
		exitCode,
		complete,
		analyses,
		formats,
		errs,
	)
	if resultErr != nil {
		return report(stderr, moreSevereExitCode(exitCode, ExitInternalError), "gox check: construct JSON report: %v\n", resultErr)
	}
	encoded, resultErr := goxreport.MarshalCheckJSON(result)
	if resultErr != nil {
		return report(stderr, moreSevereExitCode(exitCode, ExitInternalError), "gox check: encode JSON report: %v\n", resultErr)
	}
	if resultErr := write(stdout, encoded); resultErr != nil {
		return report(stderr, moreSevereExitCode(exitCode, ExitFilesystemError), "gox check: write JSON report: %v\n", resultErr)
	}
	return exitCode
}
