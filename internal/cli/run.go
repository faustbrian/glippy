// Package cli owns Gox command dispatch and process-facing I/O contracts.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/faustbrian/gox/internal/config"
	goxdiff "github.com/faustbrian/gox/internal/diff"
	"github.com/faustbrian/gox/internal/discovery"
	"github.com/faustbrian/gox/internal/filesystem"
	goxformat "github.com/faustbrian/gox/internal/format"
	goxreport "github.com/faustbrian/gox/internal/report"
	"github.com/faustbrian/gox/internal/source"
	goxversion "github.com/faustbrian/gox/internal/version"
)

const (
	ExitSuccess           = 0
	ExitFindings          = 1
	ExitSourceError       = 2
	ExitInvalidInvocation = 3
	ExitConflict          = 4
	ExitFilesystemError   = 5
	ExitInternalError     = 6
	ExitCanceled          = 130
)

var defaultFormatOptions = goxformat.Options{
	Width:     100,
	TabWidth:  8,
	FitBudget: 1_000,
}

const formatUsage = "gox: expected 'fmt [--write|--check|--diff] [--reporter=text|json] [--config=<path>] [--stdin-filepath=<path>] [--fragment=declaration|statement|expression] [path...]'\n"
const versionUsage = "gox: expected 'version'\n"

const maximumFormatWorkers = 32

type formatInvocation struct {
	fragmentKind  source.FragmentKind
	stdinFilepath string
	configPath    string
	paths         []string
	reporter      goxreport.Format
	check         bool
	diff          bool
	write         bool
}

// Run executes one Gox invocation against explicit process streams.
func Run(arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return RunContext(context.Background(), arguments, stdin, stdout, stderr)
}

// RunContext executes one Gox invocation and observes cancellation between bounded operations.
func RunContext(ctx context.Context, arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdin == nil || stdout == nil || stderr == nil {
		if stderr == nil {
			return ExitFilesystemError
		}
		return report(stderr, ExitFilesystemError, "gox: process streams are required\n")
	}
	if len(arguments) > 0 && arguments[0] == "version" {
		return runVersion(ctx, arguments, stdout, stderr)
	}
	invocation, valid := parseFormatInvocation(arguments)
	if !valid {
		if requestsJSONReporter(arguments) {
			return reportFormatJSON(
				stdout,
				stderr,
				invalidFormatInvocationMode(arguments),
				ExitInvalidInvocation,
				0,
				0,
				nil,
				errors.New(strings.TrimSpace(formatUsage)),
			)
		}
		return report(stderr, ExitInvalidInvocation, formatUsage)
	}
	if ctx == nil {
		if invocation.reporter == goxreport.JSON {
			return reportFormatJSON(
				stdout,
				stderr,
				formatInvocationMode(invocation),
				ExitInternalError,
				0,
				0,
				nil,
				errors.New("context is required"),
			)
		}
		return report(stderr, ExitInternalError, "gox: context is required\n")
	}
	if err := ctx.Err(); err != nil {
		if invocation.reporter == goxreport.JSON {
			return reportFormatJSON(
				stdout,
				stderr,
				formatInvocationMode(invocation),
				ExitCanceled,
				0,
				0,
				nil,
				err,
			)
		}
		return report(stderr, ExitCanceled, "gox: %v\n", err)
	}
	if len(invocation.paths) > 0 {
		if invocation.fragmentKind != 0 || invocation.stdinFilepath != "" {
			return reportInvalidFormatInvocation(invocation, stdout, stderr)
		}
		if invocation.write {
			return runFormatWriteReported(ctx, invocation, stdout, stderr, replaceFormatSnapshot)
		}
		if invocation.diff {
			if invocation.reporter == goxreport.JSON {
				return reportInvalidFormatInvocation(invocation, stdout, stderr)
			}
			return runFormatDiff(ctx, invocation, stdout, stderr)
		}
		if invocation.check {
			return runFormatCheck(ctx, invocation, stdout, stderr)
		}
		if len(invocation.paths) != 1 {
			return reportInvalidFormatInvocation(invocation, stdout, stderr)
		}
		if invocation.reporter == goxreport.JSON {
			return reportInvalidFormatInvocation(invocation, stdout, stderr)
		}
		return runFormatFile(ctx, invocation, stdout, stderr)
	}
	if invocation.check || invocation.diff || invocation.write {
		return reportInvalidFormatInvocation(invocation, stdout, stderr)
	}
	if invocation.reporter == goxreport.JSON {
		return reportInvalidFormatInvocation(invocation, stdout, stderr)
	}
	formatOptions, exitCode, err := resolveFormatOptions(invocation)
	if err != nil {
		return report(stderr, exitCode, "gox fmt: %v\n", err)
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: read standard input: %v\n", err)
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
	}
	sourcePath := invocation.stdinFilepath
	if sourcePath == "" {
		sourcePath = "stdin.go"
	}
	formatted, exitCode, err := formatStandardInput(input, sourcePath, invocation.fragmentKind, formatOptions)
	if err != nil {
		return report(stderr, exitCode, "gox fmt: %v\n", err)
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
	}
	if err := write(stdout, formatted); err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: write standard output: %v\n", err)
	}
	return ExitSuccess
}

func runVersion(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) != 1 {
		return report(stderr, ExitInvalidInvocation, versionUsage)
	}
	if ctx == nil {
		return report(stderr, ExitInternalError, "gox version: context is required\n")
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox version: %v\n", err)
	}
	if err := write(stdout, []byte("gox "+goxversion.Current()+"\n")); err != nil {
		return report(stderr, ExitFilesystemError, "gox version: write standard output: %v\n", err)
	}
	return ExitSuccess
}

func reportInvalidFormatInvocation(invocation formatInvocation, stdout, stderr io.Writer) int {
	if invocation.reporter == goxreport.JSON {
		return reportFormatJSON(
			stdout,
			stderr,
			formatInvocationMode(invocation),
			ExitInvalidInvocation,
			0,
			0,
			nil,
			errors.New(strings.TrimSpace(formatUsage)),
		)
	}
	return report(stderr, ExitInvalidInvocation, formatUsage)
}

func formatInvocationMode(invocation formatInvocation) string {
	switch {
	case invocation.check:
		return "check"
	case invocation.diff:
		return "diff"
	case invocation.write:
		return "write"
	default:
		return "stdout"
	}
}

func requestsJSONReporter(arguments []string) bool {
	if len(arguments) == 0 || arguments[0] != "fmt" {
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

func invalidFormatInvocationMode(arguments []string) string {
	check, diff, write := false, false, false
	for _, argument := range arguments[1:] {
		switch argument {
		case "--check":
			check = true
		case "--write":
			write = true
		case "--diff":
			diff = true
		}
	}
	if boolCount(check, diff, write) != 1 {
		return "invalid"
	}
	if check {
		return "check"
	}
	if diff {
		return "diff"
	}
	return "write"
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

type formatTask struct {
	file    discovery.File
	root    string
	options goxformat.Options
}

type formatTaskError struct {
	exitCode int
	err      error
}

func (e *formatTaskError) Error() string { return e.err.Error() }

func (e *formatTaskError) Unwrap() error { return e.err }

func newFormatTaskError(exitCode int, format string, arguments ...any) error {
	return &formatTaskError{exitCode: exitCode, err: fmt.Errorf(format, arguments...)}
}

func reportFormatTaskError(stderr io.Writer, err error) int {
	exitCode := formatTaskErrorExitCode(err)
	return report(stderr, exitCode, "gox fmt: %v\n", err)
}

func formatTaskErrorExitCode(err error) int {
	exitCode := exitCodeForError(ExitInternalError, err)
	var taskError *formatTaskError
	if exitCode != ExitCanceled && errors.As(err, &taskError) {
		exitCode = taskError.exitCode
	}
	return exitCode
}

func exitCodeForError(fallback int, err error) int {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ExitCanceled
	}
	return fallback
}

func formatWorkerLimit(taskCount int) int {
	return boundedFormatWorkerLimit(runtime.GOMAXPROCS(0), taskCount)
}

func boundedFormatWorkerLimit(resourceLimit, taskCount int) int {
	workers := min(resourceLimit, maximumFormatWorkers, taskCount)
	return max(workers, 1)
}

func mapFormatTasks[T any](
	ctx context.Context,
	tasks []formatTask,
	workerCount int,
	work func(context.Context, formatTask) (T, error),
) ([]T, error) {
	results := make([]T, len(tasks))
	if len(tasks) == 0 {
		return results, ctx.Err()
	}
	workerCount = max(1, min(workerCount, len(tasks)))
	errorsByTask := make([]error, len(tasks))
	jobs := make(chan int)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for index := range jobs {
				if err := ctx.Err(); err != nil {
					errorsByTask[index] = err
					continue
				}
				results[index], errorsByTask[index] = work(ctx, tasks[index])
			}
		}()
	}
	dispatchErr := error(nil)
dispatch:
	for index := range tasks {
		select {
		case jobs <- index:
		case <-ctx.Done():
			dispatchErr = ctx.Err()
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	if dispatchErr != nil {
		return nil, dispatchErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := selectFormatTaskError(errorsByTask); err != nil {
		return nil, err
	}
	return results, nil
}

func selectFormatTaskError(errorsByTask []error) error {
	var selected error
	selectedSeverity := -1
	for _, err := range errorsByTask {
		if err == nil {
			continue
		}
		severity := formatTaskErrorSeverity(err)
		if severity > selectedSeverity {
			selected = err
			selectedSeverity = severity
		}
	}
	return selected
}

func formatTaskErrorSeverity(err error) int {
	var taskError *formatTaskError
	if errors.As(err, &taskError) {
		return taskError.exitCode
	}
	return ExitInternalError
}

type preparedFormatCheck struct {
	path    string
	changed bool
}

func runFormatCheck(
	ctx context.Context,
	invocation formatInvocation,
	stdout, stderr io.Writer,
) int {
	tasks, exitCode, err := prepareFormatTasks(ctx, invocation)
	if err != nil {
		if invocation.reporter == goxreport.JSON {
			return reportFormatJSON(stdout, stderr, "check", exitCode, 0, 0, nil, err)
		}
		return report(stderr, exitCode, "gox fmt: %v\n", err)
	}
	prepared, err := mapFormatTasks(ctx, tasks, formatWorkerLimit(len(tasks)), func(ctx context.Context, task formatTask) (preparedFormatCheck, error) {
		input, err := os.ReadFile(task.file.Path)
		if err != nil {
			return preparedFormatCheck{}, newFormatTaskError(ExitFilesystemError, "read %q: %w", task.file.Path, err)
		}
		if err := ctx.Err(); err != nil {
			return preparedFormatCheck{}, err
		}
		formatted, exitCode, err := formatStandardInput(input, task.file.Path, 0, task.options)
		if err != nil {
			return preparedFormatCheck{}, &formatTaskError{exitCode: exitCode, err: err}
		}
		if err := ctx.Err(); err != nil {
			return preparedFormatCheck{}, err
		}
		return preparedFormatCheck{path: task.file.Path, changed: !bytes.Equal(input, formatted)}, nil
	})
	if err != nil {
		if invocation.reporter == goxreport.JSON {
			exitCode := formatTaskErrorExitCode(err)
			return reportFormatJSON(stdout, stderr, "check", exitCode, len(tasks), 0, nil, err)
		}
		return reportFormatTaskError(stderr, err)
	}
	findings := make([]string, 0)
	files := make([]goxreport.File, 0, len(prepared))
	for _, item := range prepared {
		if item.changed {
			findings = append(findings, item.path)
			files = append(files, goxreport.File{Path: item.path, Status: goxreport.FileDifferent})
		} else {
			files = append(files, goxreport.File{Path: item.path, Status: goxreport.FileUnchanged})
		}
	}
	if err := ctx.Err(); err != nil {
		if invocation.reporter == goxreport.JSON {
			return reportFormatJSON(stdout, stderr, "check", ExitCanceled, len(tasks), len(findings), files, err)
		}
		return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
	}
	if invocation.reporter == goxreport.JSON {
		exitCode := ExitSuccess
		if len(findings) > 0 {
			exitCode = ExitFindings
		}
		return reportFormatJSON(stdout, stderr, "check", exitCode, len(tasks), len(findings), files, nil)
	}
	if len(findings) == 0 {
		return ExitSuccess
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
	}
	if err := write(stdout, []byte(strings.Join(findings, "\n")+"\n")); err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: write standard output: %v\n", err)
	}
	return ExitFindings
}

type preparedFormatDiff struct {
	path      string
	input     []byte
	formatted []byte
}

func runFormatDiff(ctx context.Context, invocation formatInvocation, stdout, stderr io.Writer) int {
	tasks, exitCode, err := prepareFormatTasks(ctx, invocation)
	if err != nil {
		return report(stderr, exitCode, "gox fmt: %v\n", err)
	}
	prepared, err := mapFormatTasks(ctx, tasks, formatWorkerLimit(len(tasks)), func(ctx context.Context, task formatTask) (preparedFormatDiff, error) {
		input, err := os.ReadFile(task.file.Path)
		if err != nil {
			return preparedFormatDiff{}, newFormatTaskError(ExitFilesystemError, "read %q: %w", task.file.Path, err)
		}
		if err := ctx.Err(); err != nil {
			return preparedFormatDiff{}, err
		}
		formatted, exitCode, err := formatStandardInput(input, task.file.Path, 0, task.options)
		if err != nil {
			return preparedFormatDiff{}, &formatTaskError{exitCode: exitCode, err: err}
		}
		return preparedFormatDiff{path: task.file.Path, input: input, formatted: formatted}, nil
	})
	if err != nil {
		return reportFormatTaskError(stderr, err)
	}
	var output strings.Builder
	changed := false
	for _, item := range prepared {
		if err := ctx.Err(); err != nil {
			return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
		}
		difference := goxdiff.Unified(item.path+".orig", item.path, item.input, item.formatted)
		if difference != "" {
			changed = true
			output.WriteString(difference)
		}
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
	}
	if output.Len() > 0 {
		if err := write(stdout, []byte(output.String())); err != nil {
			return report(stderr, ExitFilesystemError, "gox fmt: write standard output: %v\n", err)
		}
	}
	if changed {
		return ExitFindings
	}
	return ExitSuccess
}

func prepareFormatTasks(ctx context.Context, invocation formatInvocation) ([]formatTask, int, error) {
	selected := make(map[string]discovery.File)
	optionsByConfiguration := make(map[string]goxformat.Options)
	for _, input := range invocation.paths {
		if err := ctx.Err(); err != nil {
			return nil, ExitCanceled, err
		}
		selection, err := config.Discover(input, invocation.configPath)
		if err != nil {
			return nil, ExitFilesystemError, err
		}
		options, exitCode, err := formatOptionsForSelection(selection)
		if err != nil {
			return nil, exitCode, err
		}
		optionsByConfiguration[selection.Path] = options
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
	tasks := make([]formatTask, 0, len(paths))
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
			options, exitCode, err = formatOptionsForSelection(selection)
			if err != nil {
				return nil, exitCode, err
			}
			optionsByConfiguration[selection.Path] = options
		}
		tasks = append(tasks, formatTask{file: selected[path], root: selection.Root, options: options})
	}
	return tasks, ExitSuccess, nil
}

type preparedFormatWrite struct {
	snapshot *filesystem.Snapshot
	path     string
	output   []byte
	changed  bool
}

type formatSnapshotReplacer func(*filesystem.Snapshot, []byte) error

func replaceFormatSnapshot(snapshot *filesystem.Snapshot, output []byte) error {
	return snapshot.Replace(output)
}

func runFormatWrite(
	ctx context.Context,
	invocation formatInvocation,
	stderr io.Writer,
	replace formatSnapshotReplacer,
) int {
	return runFormatWriteReported(ctx, invocation, io.Discard, stderr, replace)
}

func runFormatWriteReported(
	ctx context.Context,
	invocation formatInvocation,
	stdout, stderr io.Writer,
	replace formatSnapshotReplacer,
) int {
	tasks, exitCode, err := prepareFormatTasks(ctx, invocation)
	if err != nil {
		if invocation.reporter == goxreport.JSON {
			return reportFormatJSON(stdout, stderr, "write", exitCode, 0, 0, nil, err)
		}
		return report(stderr, exitCode, "gox fmt: %v\n", err)
	}
	prepared, err := mapFormatTasks(ctx, tasks, formatWorkerLimit(len(tasks)), func(ctx context.Context, task formatTask) (preparedFormatWrite, error) {
		if task.file.TraversesSymlink {
			return preparedFormatWrite{}, newFormatTaskError(ExitFilesystemError, "refusing to write symlink %q", task.file.Path)
		}
		snapshot, err := filesystem.ReadWithin(task.root, task.file.Path)
		if err != nil {
			return preparedFormatWrite{}, newFormatTaskError(ExitFilesystemError, "%w", err)
		}
		if err := ctx.Err(); err != nil {
			return preparedFormatWrite{}, err
		}
		input := snapshot.Bytes()
		file, err := source.Load(task.file.Path, input)
		if err != nil {
			return preparedFormatWrite{}, &formatTaskError{exitCode: ExitSourceError, err: err}
		}
		if file.Metadata().Generated {
			return preparedFormatWrite{}, newFormatTaskError(ExitFilesystemError, "refusing to write generated file %q", task.file.Path)
		}
		formatted, err := goxformat.File(file, task.options)
		if err != nil {
			return preparedFormatWrite{}, &formatTaskError{exitCode: ExitInternalError, err: err}
		}
		if err := ctx.Err(); err != nil {
			return preparedFormatWrite{}, err
		}
		return preparedFormatWrite{
			snapshot: snapshot,
			path:     task.file.Path,
			output:   formatted,
			changed:  !bytes.Equal(input, formatted),
		}, nil
	})
	if err != nil {
		if invocation.reporter == goxreport.JSON {
			exitCode := formatTaskErrorExitCode(err)
			return reportFormatJSON(stdout, stderr, "write", exitCode, len(tasks), 0, nil, err)
		}
		return reportFormatTaskError(stderr, err)
	}
	files := make([]goxreport.File, len(prepared))
	changedCount := 0
	for index, item := range prepared {
		files[index] = goxreport.File{Path: item.path, Status: goxreport.FilePending}
		if item.changed {
			changedCount++
		}
	}
	replaced := make([]string, 0, len(prepared))
	for index, item := range prepared {
		if err := ctx.Err(); err != nil {
			if invocation.reporter == goxreport.JSON {
				return reportFormatJSON(stdout, stderr, "write", ExitCanceled, len(tasks), changedCount, files, err)
			}
			return reportFormatWriteFailure(stderr, ExitCanceled, err, replaced, "")
		}
		if err := replace(item.snapshot, item.output); err != nil {
			if errors.Is(err, filesystem.ErrStale) {
				files[index].Status = goxreport.FileConflict
				if invocation.reporter == goxreport.JSON {
					return reportFormatJSON(stdout, stderr, "write", ExitConflict, len(tasks), changedCount, files, err)
				}
				return reportFormatWriteFailure(stderr, ExitConflict, err, replaced, "")
			}
			files[index].Status = goxreport.FileFailed
			possiblyReplaced := ""
			if item.changed {
				possiblyReplaced = item.path
				files[index].Status = goxreport.FilePossiblyFormatted
			}
			if invocation.reporter == goxreport.JSON {
				return reportFormatJSON(stdout, stderr, "write", ExitFilesystemError, len(tasks), changedCount, files, err)
			}
			return reportFormatWriteFailure(stderr, ExitFilesystemError, err, replaced, possiblyReplaced)
		}
		if item.changed {
			replaced = append(replaced, item.path)
			files[index].Status = goxreport.FileFormatted
		} else {
			files[index].Status = goxreport.FileUnchanged
		}
	}
	if err := ctx.Err(); err != nil {
		if invocation.reporter == goxreport.JSON {
			return reportFormatJSON(stdout, stderr, "write", ExitCanceled, len(tasks), changedCount, files, err)
		}
		return reportFormatWriteFailure(stderr, ExitCanceled, err, replaced, "")
	}
	if invocation.reporter == goxreport.JSON {
		return reportFormatJSON(stdout, stderr, "write", ExitSuccess, len(tasks), changedCount, files, nil)
	}
	return ExitSuccess
}

func reportFormatWriteFailure(
	stderr io.Writer,
	exitCode int,
	err error,
	replaced []string,
	possiblyReplaced string,
) int {
	if len(replaced) == 0 && possiblyReplaced == "" {
		return report(stderr, exitCode, "gox fmt: %v\n", err)
	}
	paths := append([]string(nil), replaced...)
	heading := "files replaced before failure"
	if possiblyReplaced != "" {
		paths = append(paths, possiblyReplaced)
		heading = "files replaced or possibly replaced before failure"
	}
	return report(stderr, exitCode, "gox fmt: %v\ngox fmt: %s:\n%s\n", err, heading, strings.Join(paths, "\n"))
}

func reportFormatJSON(
	stdout, stderr io.Writer,
	mode string,
	exitCode, selected, changed int,
	files []goxreport.File,
	err error,
) int {
	errs := []goxreport.Error{}
	if err != nil {
		errs = append(errs, goxreport.Error{Message: err.Error()})
	}
	result := goxreport.NewFormatResult(
		mode,
		exitCategory(exitCode),
		exitCode,
		selected,
		changed,
		err == nil,
		files,
		errs,
	)
	encoded, err := goxreport.MarshalJSON(result)
	if err != nil {
		return report(stderr, moreSevereExitCode(exitCode, ExitInternalError), "gox fmt: encode JSON report: %v\n", err)
	}
	if err := write(stdout, encoded); err != nil {
		outputExitCode := moreSevereExitCode(exitCode, ExitFilesystemError)
		if mode == "write" {
			completed := make([]string, 0)
			possiblyCompleted := false
			for _, file := range files {
				switch file.Status {
				case goxreport.FileFormatted:
					completed = append(completed, file.Path)
				case goxreport.FilePossiblyFormatted:
					completed = append(completed, file.Path)
					possiblyCompleted = true
				}
			}
			if len(completed) > 0 {
				heading := "files replaced before reporting failure"
				if possiblyCompleted {
					heading = "files replaced or possibly replaced before reporting failure"
				}
				return report(
					stderr,
					outputExitCode,
					"gox fmt: write JSON report: %v\ngox fmt: %s:\n%s\n",
					err,
					heading,
					strings.Join(completed, "\n"),
				)
			}
		}
		return report(stderr, outputExitCode, "gox fmt: write JSON report: %v\n", err)
	}
	return exitCode
}

func exitCategory(exitCode int) string {
	switch exitCode {
	case ExitSuccess:
		return "success"
	case ExitFindings:
		return "findings"
	case ExitSourceError:
		return "source_error"
	case ExitInvalidInvocation:
		return "invalid_invocation"
	case ExitConflict:
		return "conflict"
	case ExitFilesystemError:
		return "filesystem_error"
	case ExitCanceled:
		return "canceled"
	default:
		return "internal_error"
	}
}

func moreSevereExitCode(left, right int) int {
	if left == ExitCanceled || right == ExitCanceled {
		return ExitCanceled
	}
	return max(left, right)
}

func runFormatFile(ctx context.Context, invocation formatInvocation, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
	}
	info, err := os.Lstat(invocation.paths[0])
	if err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: inspect %q: %v\n", invocation.paths[0], err)
	}
	if info.IsDir() {
		return report(stderr, ExitInvalidInvocation, formatUsage)
	}
	selection, err := config.Discover(invocation.paths[0], invocation.configPath)
	if err != nil {
		return report(stderr, exitCodeForError(ExitFilesystemError, err), "gox fmt: %v\n", err)
	}
	files, err := discovery.GoFiles(
		ctx,
		invocation.paths,
		discovery.Options{Root: selection.Root},
	)
	if err != nil {
		return report(stderr, exitCodeForError(ExitFilesystemError, err), "gox fmt: %v\n", err)
	}
	if len(files) != 1 {
		return report(stderr, ExitInternalError, "gox fmt: expected one discovered file\n")
	}
	path := files[0].Path
	options, exitCode, err := formatOptionsForSelection(selection)
	if err != nil {
		return report(stderr, exitCode, "gox fmt: %v\n", err)
	}
	input, err := os.ReadFile(path)
	if err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: read %q: %v\n", path, err)
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
	}
	formatted, exitCode, err := formatStandardInput(input, path, 0, options)
	if err != nil {
		return report(stderr, exitCode, "gox fmt: %v\n", err)
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
	}
	if err := write(stdout, formatted); err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: write standard output: %v\n", err)
	}
	return ExitSuccess
}

func resolveFormatOptions(invocation formatInvocation) (goxformat.Options, int, error) {
	selection := config.Selection{}
	var err error
	if invocation.stdinFilepath != "" {
		selection, err = config.DiscoverFileContext(invocation.stdinFilepath, invocation.configPath)
	} else if invocation.configPath != "" {
		selection = config.Selection{Path: invocation.configPath, Explicit: true}
	}
	if err != nil {
		return goxformat.Options{}, configurationErrorExitCode(err), err
	}
	return formatOptionsForSelection(selection)
}

func formatOptionsForSelection(selection config.Selection) (goxformat.Options, int, error) {
	loaded, err := config.Load(selection, config.ParseOptions{})
	if err != nil {
		return goxformat.Options{}, configurationErrorExitCode(err), err
	}
	return goxformat.Options{
		Width:     loaded.Format.LineWidth,
		TabWidth:  loaded.Format.TabWidth,
		FitBudget: defaultFormatOptions.FitBudget,
	}, ExitSuccess, nil
}

func configurationErrorExitCode(err error) int {
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return ExitFilesystemError
	}
	return ExitInvalidInvocation
}

func formatStandardInput(
	input []byte,
	sourcePath string,
	fragmentKind source.FragmentKind,
	options goxformat.Options,
) ([]byte, int, error) {
	if fragmentKind != 0 {
		fragment, err := source.LoadFragment(sourcePath, fragmentKind, input)
		if err != nil {
			return nil, ExitSourceError, err
		}
		formatted, err := goxformat.Fragment(fragment, options)
		if err != nil {
			return nil, ExitInternalError, err
		}
		return formatted, ExitSuccess, nil
	}
	file, err := source.Load(sourcePath, input)
	if err != nil {
		return nil, ExitSourceError, err
	}
	formatted, err := goxformat.File(file, options)
	if err != nil {
		return nil, ExitInternalError, err
	}
	return formatted, ExitSuccess, nil
}

func parseFormatInvocation(arguments []string) (formatInvocation, bool) {
	if len(arguments) == 0 || arguments[0] != "fmt" {
		return formatInvocation{}, false
	}
	result := formatInvocation{reporter: goxreport.Text}
	reporterSet := false
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case strings.HasPrefix(argument, "--reporter=") && !reporterSet:
			reporter, valid := parseReporter(strings.TrimPrefix(argument, "--reporter="))
			if !valid {
				return formatInvocation{}, false
			}
			result.reporter = reporter
			reporterSet = true
		case argument == "--reporter" && !reporterSet &&
			index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "--"):
			index++
			reporter, valid := parseReporter(arguments[index])
			if !valid {
				return formatInvocation{}, false
			}
			result.reporter = reporter
			reporterSet = true
		case strings.HasPrefix(argument, "--fragment=") && result.fragmentKind == 0:
			switch strings.TrimPrefix(argument, "--fragment=") {
			case "declaration":
				result.fragmentKind = source.FragmentDeclaration
			case "statement":
				result.fragmentKind = source.FragmentStatement
			case "expression":
				result.fragmentKind = source.FragmentExpression
			default:
				return formatInvocation{}, false
			}
		case strings.HasPrefix(argument, "--stdin-filepath=") && result.stdinFilepath == "":
			result.stdinFilepath = strings.TrimPrefix(argument, "--stdin-filepath=")
			if result.stdinFilepath == "" {
				return formatInvocation{}, false
			}
		case argument == "--stdin-filepath" && result.stdinFilepath == "" &&
			index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "--"):
			index++
			result.stdinFilepath = arguments[index]
			if result.stdinFilepath == "" {
				return formatInvocation{}, false
			}
		case strings.HasPrefix(argument, "--config=") && result.configPath == "":
			result.configPath = strings.TrimPrefix(argument, "--config=")
			if result.configPath == "" {
				return formatInvocation{}, false
			}
		case argument == "--check" && !result.check:
			result.check = true
		case argument == "--diff" && !result.diff:
			result.diff = true
		case argument == "--write" && !result.write:
			result.write = true
		case argument == "--config" && result.configPath == "" &&
			index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "--"):
			index++
			result.configPath = arguments[index]
			if result.configPath == "" {
				return formatInvocation{}, false
			}
		case !strings.HasPrefix(argument, "-"):
			result.paths = append(result.paths, argument)
		default:
			return formatInvocation{}, false
		}
	}
	if boolCount(result.check, result.diff, result.write) > 1 {
		return formatInvocation{}, false
	}
	return result, true
}

func parseReporter(value string) (goxreport.Format, bool) {
	switch value {
	case "text":
		return goxreport.Text, true
	case "json":
		return goxreport.JSON, true
	default:
		return "", false
	}
}

func report(stderr io.Writer, exitCode int, format string, arguments ...any) int {
	if err := write(stderr, []byte(fmt.Sprintf(format, arguments...))); err != nil && exitCode < ExitFilesystemError {
		return ExitFilesystemError
	}
	return exitCode
}

func write(destination io.Writer, value []byte) error {
	written, err := destination.Write(value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}
