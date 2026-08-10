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
	"github.com/faustbrian/gox/internal/discovery"
	"github.com/faustbrian/gox/internal/filesystem"
	goxformat "github.com/faustbrian/gox/internal/format"
	"github.com/faustbrian/gox/internal/source"
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

const formatUsage = "gox: expected 'fmt [--write|--check] [--config=<path>] [--stdin-filepath=<path>] [--fragment=declaration|statement|expression] [path...]'\n"

const maximumFormatWorkers = 32

type formatInvocation struct {
	fragmentKind  source.FragmentKind
	stdinFilepath string
	configPath    string
	paths         []string
	check         bool
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
	if ctx == nil {
		return report(stderr, ExitInternalError, "gox: context is required\n")
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox: %v\n", err)
	}
	invocation, valid := parseFormatInvocation(arguments)
	if !valid {
		return report(stderr, ExitInvalidInvocation, formatUsage)
	}
	if len(invocation.paths) > 0 {
		if invocation.fragmentKind != 0 || invocation.stdinFilepath != "" {
			return report(stderr, ExitInvalidInvocation, formatUsage)
		}
		if invocation.write {
			return runFormatWrite(ctx, invocation, stderr, replaceFormatSnapshot)
		}
		if invocation.check {
			return runFormatCheck(ctx, invocation, stdout, stderr)
		}
		if len(invocation.paths) != 1 {
			return report(stderr, ExitInvalidInvocation, formatUsage)
		}
		return runFormatFile(ctx, invocation, stdout, stderr)
	}
	if invocation.check || invocation.write {
		return report(stderr, ExitInvalidInvocation, formatUsage)
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
	exitCode := exitCodeForError(ExitInternalError, err)
	var taskError *formatTaskError
	if exitCode != ExitCanceled && errors.As(err, &taskError) {
		exitCode = taskError.exitCode
	}
	return report(stderr, exitCode, "gox fmt: %v\n", err)
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
		return reportFormatTaskError(stderr, err)
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "gox fmt: %v\n", err)
	}
	findings := make([]string, 0)
	for _, item := range prepared {
		if item.changed {
			findings = append(findings, item.path)
		}
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
	tasks, exitCode, err := prepareFormatTasks(ctx, invocation)
	if err != nil {
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
		return reportFormatTaskError(stderr, err)
	}
	replaced := make([]string, 0, len(prepared))
	for _, item := range prepared {
		if err := ctx.Err(); err != nil {
			return reportFormatWriteFailure(stderr, ExitCanceled, err, replaced, "")
		}
		if err := replace(item.snapshot, item.output); err != nil {
			if errors.Is(err, filesystem.ErrStale) {
				return reportFormatWriteFailure(stderr, ExitConflict, err, replaced, "")
			}
			possiblyReplaced := ""
			if item.changed {
				possiblyReplaced = item.path
			}
			return reportFormatWriteFailure(stderr, ExitFilesystemError, err, replaced, possiblyReplaced)
		}
		if item.changed {
			replaced = append(replaced, item.path)
		}
	}
	if err := ctx.Err(); err != nil {
		return reportFormatWriteFailure(stderr, ExitCanceled, err, replaced, "")
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
	var result formatInvocation
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
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
	if result.check && result.write {
		return formatInvocation{}, false
	}
	return result, true
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
