package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/config"
	"github.com/faustbrian/gox/internal/discovery"
	goxreport "github.com/faustbrian/gox/internal/report"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

const lintUsage = "gox: expected 'lint [--reporter=text|json] [--config=<path>] [path...]'\n"

type lintInvocation struct {
	configPath string
	paths      []string
	reporter   goxreport.Format
}

type lintTask struct {
	file    discovery.File
	options analysis.RunOptions
}

func parseLintInvocation(arguments []string) (lintInvocation, bool) {
	if len(arguments) == 0 || arguments[0] != "lint" {
		return lintInvocation{}, false
	}
	result := lintInvocation{reporter: goxreport.Text}
	reporterSet := false
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case strings.HasPrefix(argument, "--reporter=") && !reporterSet:
			reporter, valid := parseReporter(strings.TrimPrefix(argument, "--reporter="))
			if !valid {
				return lintInvocation{}, false
			}
			result.reporter = reporter
			reporterSet = true
		case argument == "--reporter" && !reporterSet &&
			index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "--"):
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
		case argument == "--config" && result.configPath == "" &&
			index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "--"):
			index++
			result.configPath = arguments[index]
			if result.configPath == "" {
				return lintInvocation{}, false
			}
		case !strings.HasPrefix(argument, "-") && !strings.Contains(argument, "..."):
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

func requestsLintJSONReporter(arguments []string) bool {
	if len(arguments) == 0 || arguments[0] != "lint" {
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

func runLintCheck(
	ctx context.Context,
	invocation lintInvocation,
	stdout, stderr io.Writer,
	registry *rules.Registry,
) int {
	if ctx == nil {
		return reportLintFailure(invocation, stdout, stderr, ExitInternalError, nil, errors.New("context is required"))
	}
	if registry == nil {
		return reportLintFailure(invocation, stdout, stderr, ExitInternalError, nil, errors.New("rule registry is required"))
	}
	if err := ctx.Err(); err != nil {
		return reportLintFailure(invocation, stdout, stderr, ExitCanceled, nil, err)
	}
	tasks, exitCode, err := prepareLintTasks(ctx, invocation, registry)
	if err != nil {
		return reportLintFailure(invocation, stdout, stderr, exitCode, nil, err)
	}

	inputs := make([]goxreport.LintTextInput, 0, len(tasks))
	results := make([]analysis.Result, 0, len(tasks))
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			return reportLintFailure(invocation, stdout, stderr, ExitCanceled, results, err)
		}
		input, err := os.ReadFile(task.file.Path)
		if err != nil {
			return reportLintFailure(
				invocation,
				stdout,
				stderr,
				ExitFilesystemError,
				results,
				fmt.Errorf("read %q: %w", task.file.Path, err),
			)
		}
		file, err := source.Load(task.file.Path, input)
		if err != nil {
			return reportLintFailure(invocation, stdout, stderr, ExitSourceError, results, err)
		}
		analyzed, err := analysis.Run(ctx, file, registry, task.options)
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
			return report(stderr, ExitFilesystemError, "gox lint: write standard output: %v\n", err)
		}
	}
	return exitCode
}

func prepareLintTasks(
	ctx context.Context,
	invocation lintInvocation,
	registry *rules.Registry,
) ([]lintTask, int, error) {
	selected := make(map[string]discovery.File)
	optionsByConfiguration := make(map[string]analysis.RunOptions)
	for _, input := range invocation.paths {
		if err := ctx.Err(); err != nil {
			return nil, ExitCanceled, err
		}
		selection, err := config.Discover(input, invocation.configPath)
		if err != nil {
			return nil, ExitFilesystemError, err
		}
		if _, exists := optionsByConfiguration[selection.Path]; !exists {
			options, exitCode, err := lintOptionsForSelection(selection, registry)
			if err != nil {
				return nil, exitCode, err
			}
			optionsByConfiguration[selection.Path] = options
		}
		files, err := discovery.GoFiles(ctx, []string{input}, discovery.Options{Root: selection.Root})
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
		selection, err := config.Discover(path, invocation.configPath)
		if err != nil {
			return nil, ExitFilesystemError, err
		}
		options, exists := optionsByConfiguration[selection.Path]
		if !exists {
			var exitCode int
			options, exitCode, err = lintOptionsForSelection(selection, registry)
			if err != nil {
				return nil, exitCode, err
			}
			optionsByConfiguration[selection.Path] = options
		}
		tasks = append(tasks, lintTask{file: selected[path], options: options})
	}
	return tasks, ExitSuccess, nil
}

func lintOptionsForSelection(
	selection config.Selection,
	registry *rules.Registry,
) (analysis.RunOptions, int, error) {
	loaded, err := config.Load(selection, config.ParseOptions{KnownRules: registry.IDs()})
	if err != nil {
		return analysis.RunOptions{}, configurationErrorExitCode(err), err
	}
	return analysis.RunOptions{
		Preset:    loaded.Lint.Preset,
		Overrides: loaded.Lint.Rules,
	}, ExitSuccess, nil
}

func lintResultExitCode(results []analysis.Result) int {
	for _, result := range results {
		if len(result.Diagnostics) > 0 || len(result.SuppressionProblems) > 0 ||
			len(result.UnusedSuppressions) > 0 {
			return ExitFindings
		}
	}
	return ExitSuccess
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
