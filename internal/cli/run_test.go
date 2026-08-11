package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/faustbrian/gox/internal/discovery"
	"github.com/faustbrian/gox/internal/filesystem"
	goxversion "github.com/faustbrian/gox/internal/version"
)

var errStream = errors.New("stream failure")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errStream
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errStream
}

type shortWriter struct{}

func (shortWriter) Write(value []byte) (int, error) {
	return len(value) - 1, nil
}

type formatJSONReport struct {
	SchemaVersion int    `json:"schema_version"`
	Command       string `json:"command"`
	Mode          string `json:"mode"`
	Outcome       struct {
		Category string `json:"category"`
		ExitCode int    `json:"exit_code"`
	} `json:"outcome"`
	Summary struct {
		Files    int  `json:"files"`
		Changed  int  `json:"changed"`
		Complete bool `json:"complete"`
	} `json:"summary"`
	Files []struct {
		Path   string `json:"path"`
		Status string `json:"status"`
	} `json:"files"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func decodeFormatJSONReport(t *testing.T, output []byte) formatJSONReport {
	t.Helper()
	var report formatJSONReport
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatalf("decode JSON report: %v; output = %q", err, output)
	}
	return report
}

func TestRunReportsResolvedVersion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"version"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit = %d, want %d", exitCode, ExitSuccess)
	}
	want := "gox " + goxversion.Current() + "\n"
	if stdout.String() != want {
		t.Fatalf("Run() stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunVersionRejectsArguments(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"version", "extra"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit = %d, want %d", exitCode, ExitInvalidInvocation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != versionUsage {
		t.Fatalf("Run() stderr = %q, want version usage", stderr.String())
	}
}

func TestRunVersionHonorsCancellationBeforeOutput(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stderr bytes.Buffer

	exitCode := RunContext(ctx, []string{"version"}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitCanceled {
		t.Fatalf("RunContext() exit = %d, want %d", exitCode, ExitCanceled)
	}
	if !strings.Contains(stderr.String(), context.Canceled.Error()) {
		t.Fatalf("RunContext() stderr = %q, want cancellation", stderr.String())
	}
}

func TestRunVersionReportsOutputFailure(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	exitCode := Run([]string{"version"}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "write standard output") {
		t.Fatalf("Run() stderr = %q, want output failure", stderr.String())
	}
}

func TestRunFormatsCompleteFileFromStdinToStdout(t *testing.T) {
	t.Parallel()

	stdin := bytes.NewBufferString("package sample\nfunc run(){if ready{work()}}\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt"}, stdin, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	want := "package sample\n\nfunc run() {\n\tif ready {\n\t\twork()\n\t}\n}\n"
	if stdout.String() != want {
		t.Fatalf("Run() stdout =\n%s\nwant:\n%s", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunContextRefusesCanceledInvocationBeforeReadingInput(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := RunContext(ctx, []string{"fmt"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitCanceled {
		t.Fatalf("RunContext() exit code = %d, want %d", exitCode, ExitCanceled)
	}
	if stdout.Len() != 0 {
		t.Fatalf("RunContext() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), context.Canceled.Error()) || strings.Contains(stderr.String(), errStream.Error()) {
		t.Fatalf("RunContext() stderr = %q, want cancellation without input read", stderr.String())
	}
}

func TestRunContextReportsPreCanceledJSONInvocationAsJSON(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := RunContext(
		ctx,
		[]string{"fmt", "--check", "--reporter=json", "source.go"},
		failingReader{},
		&stdout,
		&stderr,
	)

	if exitCode != ExitCanceled {
		t.Fatalf("RunContext() exit = %d, want %d", exitCode, ExitCanceled)
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.Mode != "check" || report.Outcome.Category != "canceled" ||
		report.Outcome.ExitCode != ExitCanceled || report.Summary.Complete || len(report.Errors) != 1 {
		t.Fatalf("RunContext() report = %#v", report)
	}
	if stderr.Len() != 0 {
		t.Fatalf("RunContext() stderr = %q, want empty", stderr.String())
	}
}

func TestMapFormatTasksBoundsConcurrencyAndPreservesTaskOrder(t *testing.T) {
	t.Parallel()

	tasks := []formatTask{
		{file: discoveryFile("a.go")},
		{file: discoveryFile("b.go")},
		{file: discoveryFile("c.go")},
		{file: discoveryFile("d.go")},
	}
	started := make(chan string, len(tasks))
	release := make(chan struct{}, len(tasks))
	var active atomic.Int64
	var maximum atomic.Int64
	type outcome struct{ path string }
	result := make(chan struct {
		values []outcome
		err    error
	}, 1)
	go func() {
		values, err := mapFormatTasks(context.Background(), tasks, 2, func(_ context.Context, task formatTask) (outcome, error) {
			current := active.Add(1)
			for observed := maximum.Load(); current > observed && !maximum.CompareAndSwap(observed, current); observed = maximum.Load() {
			}
			started <- task.file.Path
			<-release
			active.Add(-1)
			return outcome{path: task.file.Path}, nil
		})
		result <- struct {
			values []outcome
			err    error
		}{values: values, err: err}
	}()

	<-started
	<-started
	if got := maximum.Load(); got != 2 {
		t.Fatalf("mapFormatTasks() maximum concurrency = %d, want 2", got)
	}
	for range tasks {
		release <- struct{}{}
	}
	got := <-result
	if got.err != nil {
		t.Fatalf("mapFormatTasks() error = %v", got.err)
	}
	for index, task := range tasks {
		if got.values[index].path != task.file.Path {
			t.Fatalf("mapFormatTasks() result[%d] = %q, want %q", index, got.values[index].path, task.file.Path)
		}
	}
}

func TestBoundedFormatWorkerLimitUsesEveryResourceBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		resourceLimit int
		taskCount     int
		want          int
	}{
		{name: "resource", resourceLimit: 4, taskCount: 20, want: 4},
		{name: "selection", resourceLimit: 8, taskCount: 3, want: 3},
		{name: "hard ceiling", resourceLimit: 64, taskCount: 100, want: maximumFormatWorkers},
		{name: "empty selection", resourceLimit: 8, taskCount: 0, want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := boundedFormatWorkerLimit(test.resourceLimit, test.taskCount); got != test.want {
				t.Fatalf("boundedFormatWorkerLimit(%d, %d) = %d, want %d", test.resourceLimit, test.taskCount, got, test.want)
			}
		})
	}
}

func TestMapFormatTasksChoosesFirstTaskErrorDeterministically(t *testing.T) {
	t.Parallel()

	firstErr := errors.New("first task failed")
	secondErr := errors.New("second task failed")
	secondFinished := make(chan struct{})
	tasks := []formatTask{{file: discoveryFile("a.go")}, {file: discoveryFile("z.go")}}

	_, err := mapFormatTasks(context.Background(), tasks, 2, func(_ context.Context, task formatTask) (string, error) {
		if task.file.Path == "a.go" {
			<-secondFinished
			return "", firstErr
		}
		close(secondFinished)
		return "", secondErr
	})

	if !errors.Is(err, firstErr) {
		t.Fatalf("mapFormatTasks() error = %v, want first task error", err)
	}
}

func TestMapFormatTasksChoosesSeverityBeforeTaskOrder(t *testing.T) {
	t.Parallel()

	sourceErr := errors.New("source failed")
	filesystemErr := errors.New("filesystem failed")
	tasks := []formatTask{{file: discoveryFile("a.go")}, {file: discoveryFile("z.go")}}

	_, err := mapFormatTasks(context.Background(), tasks, 2, func(_ context.Context, task formatTask) (string, error) {
		if task.file.Path == "a.go" {
			return "", &formatTaskError{exitCode: ExitSourceError, err: sourceErr}
		}
		return "", &formatTaskError{exitCode: ExitFilesystemError, err: filesystemErr}
	})

	if !errors.Is(err, filesystemErr) {
		t.Fatalf("mapFormatTasks() error = %v, want higher-severity filesystem error", err)
	}
}

func discoveryFile(path string) discovery.File {
	return discovery.File{Path: path}
}

func TestRunFormatsOneExplicitFileToStdoutWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gox.toml"),
		[]byte("version = 1\n[format]\nline-width = 30\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){if firstCondition && secondCondition && thirdCondition {work()}}\n")
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	if !strings.Contains(stdout.String(), "firstCondition &&\n") {
		t.Fatalf("Run() stdout =\n%s\nwant configured width break", stdout.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run() mutated stdout-mode file: %q", got)
	}
}

func TestRunWriteFormatsFileAndPreservesPermissions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "package sample\n\nfunc run() {}\n"
	if string(got) != want {
		t.Fatalf("Run() file = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("Run() permissions = %o, want 640", info.Mode().Perm())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("Run() stdout = %q, stderr = %q, want empty", stdout.String(), stderr.String())
	}
}

func TestRunWriteDoesNotTouchFormattedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("Run() touched formatted file: before = %#v, after = %#v", before, after)
	}
}

func TestRunWriteReportsVersionedJSONOutcomesInPathOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(changedPath, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unchangedPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(unchangedPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", "--reporter=json", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.SchemaVersion != 1 || report.Command != "fmt" || report.Mode != "write" ||
		report.Outcome.Category != "success" || report.Outcome.ExitCode != ExitSuccess {
		t.Fatalf("Run() report header = %#v", report)
	}
	if report.Summary.Files != 2 || report.Summary.Changed != 1 || !report.Summary.Complete || len(report.Files) != 2 || len(report.Errors) != 0 {
		t.Fatalf("Run() report totals = %#v", report)
	}
	if report.Files[0].Path != changedPath || report.Files[0].Status != "formatted" ||
		report.Files[1].Path != unchangedPath || report.Files[1].Status != "unchanged" {
		t.Fatalf("Run() file outcomes = %#v", report.Files)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunWriteValidatesEverySourceBeforeAnyReplacement(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	firstInput := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(firstPath, firstInput, 0o600); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(invalidPath, []byte("package sample\nfunc broken(\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSourceError, stderr.String())
	}
	got, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, firstInput) {
		t.Fatalf("Run() replaced %q before validating later source: %q", firstPath, got)
	}
}

func TestRunWriteRefusesGeneratedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "generated.go")
	input := []byte("// Code generated by fixture. DO NOT EDIT.\npackage sample\nfunc run(){}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFilesystemError || !strings.Contains(stderr.String(), "refusing to write generated file") {
		t.Fatalf("Run() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run() changed generated file: %q", got)
	}
}

func TestRunWriteRefusesExplicitSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target.go")
	input := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(target, input, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFilesystemError || !strings.Contains(stderr.String(), "refusing to write symlink") {
		t.Fatalf("Run() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run() changed symlink target: %q", got)
	}
}

func TestRunWriteRefusesPathThroughSymlinkedDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetDirectory := t.TempDir()
	target := filepath.Join(targetDirectory, "source.go")
	input := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(target, input, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(targetDirectory, link); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(link, "source.go")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFilesystemError || !strings.Contains(stderr.String(), "refusing to write symlink") {
		t.Fatalf("Run() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run() changed file through symlinked directory: %q", got)
	}
}

func TestRunWriteValidatesEveryConfigurationBeforeAnyReplacement(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	firstInput := []byte("package root\nfunc run(){}\n")
	if err := os.WriteFile(firstPath, firstInput, 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "go.mod"), []byte("module example.com/nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".gox.toml"), []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "z.go"), []byte("package nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitInvalidInvocation, stderr.String())
	}
	got, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, firstInput) {
		t.Fatalf("Run() replaced %q before validating later configuration: %q", firstPath, got)
	}
}

func TestRunFormatWriteReportsStaleSourceWithoutOverwritingNewBytes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := []byte("package sample\n\nfunc newer() {}\n")
	var stderr bytes.Buffer

	exitCode := runFormatWrite(
		context.Background(),
		formatInvocation{paths: []string{path}, write: true},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if err := os.WriteFile(snapshot.Path(), newer, 0o600); err != nil {
				return err
			}
			return snapshot.Replace(output)
		},
	)

	if exitCode != ExitConflict || !strings.Contains(stderr.String(), filesystem.ErrStale.Error()) {
		t.Fatalf("runFormatWrite() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newer) {
		t.Fatalf("runFormatWrite() source = %q, want newer bytes preserved", got)
	}
}

func TestRunFormatWriteCannotEscapeRootThroughParentSymlinkRace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "nested")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "source.go")
	input := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	externalDirectory := t.TempDir()
	externalPath := filepath.Join(externalDirectory, "source.go")
	if err := os.Link(path, externalPath); err != nil {
		t.Fatal(err)
	}
	movedDirectory := filepath.Join(root, "moved")
	var stderr bytes.Buffer

	exitCode := runFormatWrite(
		context.Background(),
		formatInvocation{paths: []string{path}, write: true},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if err := os.Rename(directory, movedDirectory); err != nil {
				return err
			}
			if err := os.Symlink(externalDirectory, directory); err != nil {
				return err
			}
			return snapshot.Replace(output)
		},
	)

	if exitCode != ExitConflict || !strings.Contains(stderr.String(), filesystem.ErrStale.Error()) {
		t.Fatalf("runFormatWrite() exit = %d, stderr = %q, want stale conflict", exitCode, stderr.String())
	}
	got, err := os.ReadFile(externalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runFormatWrite() changed outside-root hard link: %q", got)
	}
}

func TestRunFormatWriteReportsFilesReplacedBeforeLaterConflict(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\nfunc first(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(secondPath, []byte("package sample\nfunc second(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := []byte("package sample\n\nfunc newer() {}\n")
	var stderr bytes.Buffer

	exitCode := runFormatWrite(
		context.Background(),
		formatInvocation{paths: []string{root}, write: true},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if snapshot.Path() == secondPath {
				if err := os.WriteFile(secondPath, newer, 0o600); err != nil {
					return err
				}
			}
			return snapshot.Replace(output)
		},
	)

	if exitCode != ExitConflict {
		t.Fatalf("runFormatWrite() exit = %d, want %d", exitCode, ExitConflict)
	}
	if !strings.Contains(stderr.String(), "files replaced before failure") || !strings.Contains(stderr.String(), firstPath) {
		t.Fatalf("runFormatWrite() stderr = %q, want prior replacement report", stderr.String())
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "package sample\n\nfunc first() {}\n" {
		t.Fatalf("runFormatWrite() first file = %q, want formatted", first)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, newer) {
		t.Fatalf("runFormatWrite() second file = %q, want newer bytes preserved", second)
	}
}

func TestRunFormatWriteReportsPartialConflictAsJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\nfunc first(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(secondPath, []byte("package sample\nfunc second(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := []byte("package sample\n\nfunc newer() {}\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runFormatWriteReported(
		context.Background(),
		formatInvocation{paths: []string{root}, reporter: "json", write: true},
		&stdout,
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if snapshot.Path() == secondPath {
				if err := os.WriteFile(secondPath, newer, 0o600); err != nil {
					return err
				}
			}
			return snapshot.Replace(output)
		},
	)

	if exitCode != ExitConflict {
		t.Fatalf("runFormatWriteReported() exit = %d, want %d", exitCode, ExitConflict)
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.Outcome.Category != "conflict" || report.Summary.Complete ||
		report.Summary.Changed != 2 || len(report.Errors) != 1 {
		t.Fatalf("runFormatWriteReported() report = %#v", report)
	}
	if report.Files[0].Path != firstPath || report.Files[0].Status != "formatted" ||
		report.Files[1].Path != secondPath || report.Files[1].Status != "conflict" {
		t.Fatalf("runFormatWriteReported() file outcomes = %#v", report.Files)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runFormatWriteReported() stderr = %q, want empty", stderr.String())
	}
}

func TestRunFormatWriteStopsBeforeNextReplacementWhenCanceled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\nfunc first(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "z.go")
	secondInput := []byte("package sample\nfunc second(){}\n")
	if err := os.WriteFile(secondPath, secondInput, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var replacements atomic.Int64
	var stderr bytes.Buffer

	exitCode := runFormatWrite(
		ctx,
		formatInvocation{paths: []string{root}, write: true},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			replacements.Add(1)
			if err := snapshot.Replace(output); err != nil {
				return err
			}
			cancel()
			return nil
		},
	)

	if exitCode != ExitCanceled {
		t.Fatalf("runFormatWrite() exit = %d, want %d", exitCode, ExitCanceled)
	}
	if replacements.Load() != 1 {
		t.Fatalf("runFormatWrite() replacements = %d, want 1", replacements.Load())
	}
	if !strings.Contains(stderr.String(), context.Canceled.Error()) ||
		!strings.Contains(stderr.String(), "files replaced before failure") ||
		!strings.Contains(stderr.String(), firstPath) {
		t.Fatalf("runFormatWrite() stderr = %q, want cancellation and prior replacement", stderr.String())
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, secondInput) {
		t.Fatalf("runFormatWrite() second file = %q, want unchanged", second)
	}
}

func TestRunFormatWriteReportsPartialCancellationAsJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\nfunc first(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "z.go")
	secondInput := []byte("package sample\nfunc second(){}\n")
	if err := os.WriteFile(secondPath, secondInput, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runFormatWriteReported(
		ctx,
		formatInvocation{paths: []string{root}, reporter: "json", write: true},
		&stdout,
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if err := snapshot.Replace(output); err != nil {
				return err
			}
			cancel()
			return nil
		},
	)

	if exitCode != ExitCanceled {
		t.Fatalf("runFormatWriteReported() exit = %d, want %d", exitCode, ExitCanceled)
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.Outcome.Category != "canceled" || report.Summary.Complete ||
		report.Summary.Changed != 2 || len(report.Errors) != 1 {
		t.Fatalf("runFormatWriteReported() report = %#v", report)
	}
	if report.Files[0].Path != firstPath || report.Files[0].Status != "formatted" ||
		report.Files[1].Path != secondPath || report.Files[1].Status != "pending" {
		t.Fatalf("runFormatWriteReported() file outcomes = %#v", report.Files)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runFormatWriteReported() stderr = %q, want empty", stderr.String())
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, secondInput) {
		t.Fatalf("runFormatWriteReported() second file = %q, want unchanged", second)
	}
}

func TestRunFormatWriteReportsUncertainStateAfterReplacementError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := runFormatWrite(
		context.Background(),
		formatInvocation{paths: []string{path}, write: true},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if err := snapshot.Replace(output); err != nil {
				return err
			}
			return errStream
		},
	)

	if exitCode != ExitFilesystemError {
		t.Fatalf("runFormatWrite() exit = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "files replaced or possibly replaced before failure") ||
		!strings.Contains(stderr.String(), path) {
		t.Fatalf("runFormatWrite() stderr = %q, want uncertain replacement report", stderr.String())
	}
}

func TestRunFormatWriteDisclosesPossiblyFormattedFileWhenJSONOutputFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := runFormatWriteReported(
		context.Background(),
		formatInvocation{paths: []string{path}, reporter: "json", write: true},
		failingWriter{},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if err := snapshot.Replace(output); err != nil {
				return err
			}
			return errStream
		},
	)

	if exitCode != ExitFilesystemError {
		t.Fatalf("runFormatWriteReported() exit = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "write JSON report") ||
		!strings.Contains(stderr.String(), "files replaced or possibly replaced before reporting failure") ||
		!strings.Contains(stderr.String(), path) {
		t.Fatalf("runFormatWriteReported() stderr = %q, want reporting failure and uncertain replacement", stderr.String())
	}
}

func TestRunRejectsInvalidWriteModeCombinations(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "source.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := [][]string{
		{"fmt", "--write"},
		{"fmt", "--write", "--check", path},
		{"fmt", "--write", "--diff", path},
		{"fmt", "--check", "--diff", path},
		{"fmt", "--write", "--fragment=statement", path},
		{"fmt", "--write", "--stdin-filepath=source.go", path},
	}
	for _, arguments := range tests {
		arguments := arguments
		t.Run(strings.Join(arguments[1:], "_"), func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := Run(arguments, failingReader{}, &stdout, &stderr)
			if exitCode != ExitInvalidInvocation {
				t.Fatalf("Run(%q) exit = %d, want %d", arguments, exitCode, ExitInvalidInvocation)
			}
			if stdout.Len() != 0 {
				t.Fatalf("Run(%q) stdout = %q, want empty", arguments, stdout.String())
			}
		})
	}
}

func TestRunRefusesExcludedFileInStdoutMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	vendor := filepath.Join(root, "vendor")
	if err := os.Mkdir(vendor, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vendor, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitFilesystemError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
}

func TestRunRejectsDirectoryInStdoutMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitInvalidInvocation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
}

func TestRunCheckReportsDifferencesWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	formattedPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(formattedPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unformattedPath := filepath.Join(root, "z.go")
	unformatted := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(unformattedPath, unformatted, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vendor", "ignored.go"), unformatted, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", "--reporter=text", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitFindings, stderr.String())
	}
	if stdout.String() != unformattedPath+"\n" {
		t.Fatalf("Run() stdout = %q, want changed path", stdout.String())
	}
	got, err := os.ReadFile(unformattedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, unformatted) {
		t.Fatalf("Run() mutated check-mode file: %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunDiffReportsUnifiedDifferenceWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--diff", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitFindings, stderr.String())
	}
	want := "--- " + path + ".orig\n" +
		"+++ " + path + "\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package sample\n" +
		"-func run(){}\n" +
		"+\n" +
		"+func run() {}\n"
	if stdout.String() != want {
		t.Fatalf("Run() stdout = %q, want %q", stdout.String(), want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run() mutated diff-mode file: %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunDiffReportsChangedFilesInPathOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\nfunc first(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unchangedPath := filepath.Join(root, "m.go")
	if err := os.WriteFile(unchangedPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lastPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(lastPath, []byte("package sample\nfunc last(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--diff", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitFindings, stderr.String())
	}
	firstHeader := "--- " + firstPath + ".orig\n"
	lastHeader := "--- " + lastPath + ".orig\n"
	firstIndex := strings.Index(stdout.String(), firstHeader)
	lastIndex := strings.Index(stdout.String(), lastHeader)
	if firstIndex < 0 || lastIndex <= firstIndex {
		t.Fatalf("Run() stdout = %q, want changed files in path order", stdout.String())
	}
	if strings.Contains(stdout.String(), unchangedPath) {
		t.Fatalf("Run() stdout = %q, want unchanged file omitted", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunDiffReturnsSuccessWithoutOutputForFormattedSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--diff", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("Run() stdout = %q, stderr = %q, want both empty", stdout.String(), stderr.String())
	}
}

func TestRunDiffReportsSourceFailureWithoutPartialOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	validPath := filepath.Join(root, "a.go")
	validInput := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(validPath, validInput, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "z.go"), []byte("package sample\nfunc broken(\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--diff", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want no partial difference", stdout.String())
	}
	if !strings.Contains(stderr.String(), "z.go") {
		t.Fatalf("Run() stderr = %q, want source failure", stderr.String())
	}
	got, err := os.ReadFile(validPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, validInput) {
		t.Fatalf("Run() mutated valid file before source failure: %q", got)
	}
}

func TestRunDiffReportsStandardOutputFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--diff", path}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "write standard output") {
		t.Fatalf("Run() stderr = %q, want output failure", stderr.String())
	}
}

func TestRunCheckReportsVersionedJSONOutcomesInPathOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unchangedPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(unchangedPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(changedPath, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", "--reporter", "json", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings {
		t.Fatalf("Run() exit = %d, want %d; stderr = %q", exitCode, ExitFindings, stderr.String())
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.SchemaVersion != 1 || report.Command != "fmt" || report.Mode != "check" ||
		report.Outcome.Category != "findings" || report.Outcome.ExitCode != ExitFindings {
		t.Fatalf("Run() report header = %#v", report)
	}
	if report.Summary.Files != 2 || report.Summary.Changed != 1 || !report.Summary.Complete || len(report.Files) != 2 || len(report.Errors) != 0 {
		t.Fatalf("Run() report totals = %#v", report)
	}
	if report.Files[0].Path != unchangedPath || report.Files[0].Status != "unchanged" ||
		report.Files[1].Path != changedPath || report.Files[1].Status != "different" {
		t.Fatalf("Run() file outcomes = %#v", report.Files)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunCheckReportsSourceFailureAsJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "broken.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc broken(\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", "--reporter=json", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit = %d, want %d; stderr = %q", exitCode, ExitSourceError, stderr.String())
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.Outcome.Category != "source_error" || report.Outcome.ExitCode != ExitSourceError || report.Summary.Complete || len(report.Errors) != 1 {
		t.Fatalf("Run() report = %#v", report)
	}
	if !strings.Contains(report.Errors[0].Message, path) || len(report.Files) != 0 {
		t.Fatalf("Run() report error = %#v", report)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunCheckResolvesConfigurationPerDiscoveredFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gox.toml"), []byte("version = 1\n[format]\nline-width = 30\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	flat := []byte("package sample\n\nfunc run() {\n\tif firstCondition && secondCondition && thirdCondition {\n\t\twork()\n\t}\n}\n")
	outerPath := filepath.Join(root, "outer.go")
	if err := os.WriteFile(outerPath, flat, 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "go.mod"), []byte("module example.com/nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".gox.toml"), []byte("version = 1\n[format]\nline-width = 100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "nested.go"), flat, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings || stdout.String() != outerPath+"\n" {
		t.Fatalf("Run() exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunCheckValidatesAllConfigurationBeforeReporting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "go.mod"), []byte("module example.com/nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".gox.toml"), []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "z.go"), []byte("package nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitInvalidInvocation)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "unsupported configuration version 2") {
		t.Fatalf("Run() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunCheckValidatesConfigurationForEmptySelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gox.toml"), []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitInvalidInvocation)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "unsupported configuration version 2") {
		t.Fatalf("Run() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunUsesDiscoveredConfigurationForStandardInputPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gox.toml"),
		[]byte("version = 1\n[format]\nline-width = 30\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	stdinPath := filepath.Join(root, "new", "source.go")
	input := "package sample\nfunc run(){if firstCondition && secondCondition && thirdCondition {work()}}\n"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--stdin-filepath", stdinPath},
		strings.NewReader(input),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	if !strings.Contains(stdout.String(), "firstCondition &&\n") {
		t.Fatalf("Run() stdout =\n%s\nwant configured width break", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunUsesExplicitConfigurationBeforeReadingStandardInput(t *testing.T) {
	t.Parallel()

	configurationPath := filepath.Join(t.TempDir(), "explicit.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[format]\nline-width = 30\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	input := "package sample\nfunc run(){if firstCondition && secondCondition && thirdCondition {work()}}\n"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--config=" + configurationPath},
		strings.NewReader(input),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSuccess || !strings.Contains(stdout.String(), "firstCondition &&\n") {
		t.Fatalf("Run() exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunAcceptsSeparatedConfigurationFlag(t *testing.T) {
	t.Parallel()

	configurationPath := filepath.Join(t.TempDir(), "explicit.toml")
	if err := os.WriteFile(configurationPath, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--config", configurationPath},
		strings.NewReader("package sample\n"),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
}

func TestRunRejectsInvalidConfigurationBeforeReadingStandardInput(t *testing.T) {
	t.Parallel()

	configurationPath := filepath.Join(t.TempDir(), "invalid.toml")
	if err := os.WriteFile(configurationPath, []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--config=" + configurationPath},
		failingReader{},
		&stdout,
		&stderr,
	)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitInvalidInvocation)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "unsupported configuration version 2") {
		t.Fatalf("Run() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "stream failure") {
		t.Fatalf("Run() read stdin before configuration validation: %q", stderr.String())
	}
}

func TestRunClassifiesDirectoryStdinFilepathAsInvalidInvocation(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		[]string{"fmt", "--stdin-filepath=" + t.TempDir()},
		strings.NewReader("package sample\n"),
		&stdout,
		&stderr,
	)
	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitInvalidInvocation, stderr.String())
	}
}

func TestRunFormatsExplicitStandardInputFragments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		argument string
		input    string
		want     string
	}{
		{
			name:     "declarations",
			argument: "--fragment=declaration",
			input:    "var answer=42\nfunc run(){}",
			want:     "var answer = 42\n\nfunc run() {}\n",
		},
		{
			name:     "statements",
			argument: "--fragment=statement",
			input:    "value:=1;value++",
			want:     "value := 1\nvalue++\n",
		},
		{
			name:     "expression",
			argument: "--fragment=expression",
			input:    "client.call(first,second)\n",
			want:     "client.call(first, second)\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := Run(
				[]string{"fmt", test.argument},
				strings.NewReader(test.input),
				&stdout,
				&stderr,
			)

			if exitCode != ExitSuccess {
				t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
			}
			if stdout.String() != test.want {
				t.Fatalf("Run() stdout = %q, want %q", stdout.String(), test.want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("Run() stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRunDoesNotInferStandardInputFragmentKind(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt"}, strings.NewReader("value++"), &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
}

func TestRunRejectsInvalidFragmentSelections(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{
		{"fmt", "--fragment"},
		{"fmt", "--fragment=unknown"},
		{"fmt", "--fragment=statement", "extra"},
		{"fmt", "--config", "--fragment=statement"},
		{"fmt", "--stdin-filepath", "--config=project.toml"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run(arguments, strings.NewReader("value++"), &stdout, &stderr)

		if exitCode != ExitInvalidInvocation {
			t.Fatalf("Run(%q) exit code = %d, want %d", arguments, exitCode, ExitInvalidInvocation)
		}
		if stdout.Len() != 0 {
			t.Fatalf("Run(%q) stdout = %q, want empty", arguments, stdout.String())
		}
		if !strings.Contains(stderr.String(), "--fragment=declaration|statement|expression") {
			t.Fatalf("Run(%q) stderr = %q, want supported fragment kinds", arguments, stderr.String())
		}
	}
}

func TestRunRejectsInvalidFragmentWithoutPartialOutputOrSyntheticLocations(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--fragment=expression"},
		strings.NewReader("first +"),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stdin.go:1:") {
		t.Fatalf("Run() stderr = %q, want physical fragment location", stderr.String())
	}
	if strings.Contains(stderr.String(), "goxfragment") {
		t.Fatalf("Run() stderr exposed synthetic wrapper: %q", stderr.String())
	}
}

func TestRunRejectsFilePlacementDirectiveInFragment(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--fragment=declaration"},
		strings.NewReader("//go:build linux\nvar value int"),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "requires complete-file placement") {
		t.Fatalf("Run() stderr = %q, want directive boundary diagnostic", stderr.String())
	}
}

func TestRunRejectsInvalidCompleteFileWithoutPartialOutput(t *testing.T) {
	t.Parallel()

	stdin := bytes.NewBufferString("package sample\nfunc")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt"}, stdin, &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stdin.go:2") {
		t.Fatalf("Run() stderr = %q, want stdin source location", stderr.String())
	}
}

func TestRunRejectsUnsupportedInvocation(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{nil, {"lint"}, {"lint", "--reporter=json"}} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run(arguments, bytes.NewReader(nil), &stdout, &stderr)

		if exitCode != ExitInvalidInvocation {
			t.Fatalf("Run(%q) exit code = %d, want %d", arguments, exitCode, ExitInvalidInvocation)
		}
		if stdout.Len() != 0 {
			t.Fatalf("Run(%q) stdout = %q, want empty", arguments, stdout.String())
		}
		if stderr.String() != formatUsage {
			t.Fatalf("Run(%q) stderr = %q", arguments, stderr.String())
		}
	}
}

func TestRunReportsInvalidJSONInvocationAsJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		arguments []string
		mode      string
	}{
		{arguments: []string{"fmt", "--reporter=json"}, mode: "stdout"},
		{arguments: []string{"fmt", "--reporter=json", "source.go"}, mode: "stdout"},
		{arguments: []string{"fmt", "--reporter", "json", "--unsupported"}, mode: "invalid"},
		{arguments: []string{"fmt", "--check", "--reporter=json", "--unsupported"}, mode: "check"},
		{arguments: []string{"fmt", "--diff", "--reporter=json", "source.go"}, mode: "diff"},
		{arguments: []string{"fmt", "--write", "--reporter=json", "--unsupported"}, mode: "write"},
		{arguments: []string{"fmt", "--check", "--write", "--reporter=json"}, mode: "invalid"},
	}
	for _, test := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run(test.arguments, bytes.NewReader(nil), &stdout, &stderr)

		if exitCode != ExitInvalidInvocation {
			t.Fatalf("Run(%q) exit = %d, want %d", test.arguments, exitCode, ExitInvalidInvocation)
		}
		report := decodeFormatJSONReport(t, stdout.Bytes())
		if report.Mode != test.mode || report.Outcome.Category != "invalid_invocation" || report.Outcome.ExitCode != ExitInvalidInvocation ||
			report.Summary.Complete || len(report.Errors) != 1 {
			t.Fatalf("Run(%q) report = %#v, want mode %q", test.arguments, report, test.mode)
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(%q) stderr = %q, want empty", test.arguments, stderr.String())
		}
	}
}

func TestRunReportsJSONOutputFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", "--reporter=json", root}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "write JSON report") {
		t.Fatalf("Run() stderr = %q, want JSON write failure", stderr.String())
	}
}

func TestRunWriteDisclosesReplacementWhenJSONOutputFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", "--reporter=json", root}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "write JSON report") ||
		!strings.Contains(stderr.String(), "files replaced before reporting failure") ||
		!strings.Contains(stderr.String(), path) {
		t.Fatalf("Run() stderr = %q, want JSON failure and replacement disclosure", stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package sample\n\nfunc run() {}\n" {
		t.Fatalf("Run() file = %q, want formatted replacement", got)
	}
}

func TestRunRejectsMissingStreamsWithoutPanicking(t *testing.T) {
	t.Parallel()

	validInput := "package sample\nfunc run(){}\n"
	tests := []struct {
		name   string
		stdin  io.Reader
		stdout io.Writer
		stderr io.Writer
	}{
		{name: "stdin", stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}},
		{name: "stdout", stdin: bytes.NewReader([]byte(validInput)), stderr: &bytes.Buffer{}},
		{name: "stderr", stdin: bytes.NewReader([]byte(validInput)), stdout: &bytes.Buffer{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exitCode := Run([]string{"fmt"}, test.stdin, test.stdout, test.stderr)
			if exitCode != ExitFilesystemError {
				t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitFilesystemError)
			}
		})
	}
}

func TestRunReportsStandardInputReadFailure(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitFilesystemError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "read standard input: stream failure") {
		t.Fatalf("Run() stderr = %q, want read failure", stderr.String())
	}
}

func TestRunReportsStandardOutputWriteFailures(t *testing.T) {
	t.Parallel()

	validInput := "package sample\nfunc run(){}\n"
	tests := []struct {
		name   string
		stdout io.Writer
	}{
		{name: "error", stdout: failingWriter{}},
		{name: "short write", stdout: shortWriter{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer

			exitCode := Run([]string{"fmt"}, strings.NewReader(validInput), test.stdout, &stderr)

			if exitCode != ExitFilesystemError {
				t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitFilesystemError)
			}
			if !strings.Contains(stderr.String(), "write standard output") {
				t.Fatalf("Run() stderr = %q, want write failure", stderr.String())
			}
		})
	}
}

func TestDiagnosticWriteFailureUsesExitSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exitCode int
		want     int
	}{
		{name: "promotes less severe category", exitCode: ExitInvalidInvocation, want: ExitFilesystemError},
		{name: "preserves more severe category", exitCode: ExitInternalError, want: ExitInternalError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := report(failingWriter{}, test.exitCode, "diagnostic\n")
			if got != test.want {
				t.Fatalf("report() exit code = %d, want %d", got, test.want)
			}
		})
	}
}
