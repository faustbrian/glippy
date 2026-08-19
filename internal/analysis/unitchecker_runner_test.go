package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/faustbrian/glippy/internal/source"
)

func TestDecodeUnitcheckerDiagnosticsPreservesRangesAndFixes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	input := []byte(
		"package sample\n\nimport \"fmt\"\n\nfunc bad(format string) {\n\tfmt.Printf(format)\n}\n",
	)
	file, err := source.Load(path, input)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(input), "format)")
	end := start + len("format")
	payloadDiagnostic := unitcheckerJSONDiagnostic{
		Posn: path + ":6:13",
		End: path + ":6:19",
		Message: "non-constant format string in call to fmt.Printf",
		SuggestedFixes: []unitcheckerJSONSuggestedFix{
			{
				Message: "Insert \"%s\" format string",
				Edits: []unitcheckerJSONEdit{
					{Filename: path, Start: start, End: start, New: "\"%s\", "},
				},
			},
		},
	}
	payload, err := json.Marshal(
		map[string]map[string][]unitcheckerJSONDiagnostic{
			"example.com/sample": {"printf": {payloadDiagnostic}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, payload...)
	diagnostics, err := decodeUnitcheckerDiagnostics(
		payload,
		"printf",
		PackageSourceSet{paths: []string{path}, files: map[string]*source.File{path: file}},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []PackageFactAnalyzerDiagnostic{
		{
			Analyzer: "printf",
			Path: path,
			Range: source.Range{Start: start, End: end},
			Message: "non-constant format string in call to fmt.Printf",
			SuggestedFixes: []PackageFactAnalyzerSuggestedFix{
				{
					Message: "Insert \"%s\" format string",
					Edits: []PackageFactAnalyzerEdit{
						{
							Path: path,
							Range: source.Range{
								Start: start,
								End: start,
							},
							NewText: "\"%s\", ",
						},
					},
				},
			},
		},
	}
	if !reflect.DeepEqual(diagnostics, want) {
		t.Fatalf("diagnostics = %#v, want %#v", diagnostics, want)
	}
}

func TestUnitcheckerPositionRejectsColumnsBeyondPhysicalLine(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sample.go")
	input := []byte("package sample\nfunc run() {}\n")
	file, err := source.Load(path, input)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = unitcheckerPosition(
		path + ":1:16",
		PackageSourceSet{paths: []string{path}, files: map[string]*source.File{path: file}},
	)
	if err == nil || !strings.Contains(err.Error(), "position column is outside") {
		t.Fatalf("unitcheckerPosition() error = %v", err)
	}
}

func TestDecodeUnitcheckerDiagnosticsRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	_, err := decodeUnitcheckerDiagnostics([]byte("{"), "printf", PackageSourceSet{})
	if err == nil || !strings.Contains(err.Error(), "decode external printf diagnostics") {
		t.Fatalf("decodeUnitcheckerDiagnostics() error = %v", err)
	}
}

func TestBoundedCommandBufferRejectsOversizedOutput(t *testing.T) {
	t.Parallel()

	buffer := &boundedCommandBuffer{limit: 3}
	written, err := buffer.Write([]byte("four"))
	if written != 3 || err == nil || buffer.String() != "fou" {
		t.Fatalf("bounded write = %d, %v, %q", written, err, buffer.String())
	}
}

func TestUnitcheckerFactAnalyzerRunnerUsesBoundedExactGoInvocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Glippy does not support Windows runtime execution")
	}

	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	input := []byte("package sample\nfunc run() {}\n")
	file, err := source.Load(path, input)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(input), "run")
	argumentsPath := filepath.Join(root, "arguments")
	environmentPath := filepath.Join(root, "environment")
	goBinary := filepath.Join(root, "go")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$GLIPPY_TEST_ARGUMENTS\"\n" +
		"printf '%s\\n' \"$" +
		UnitcheckerModeEnvironment +
		"\" > \"$GLIPPY_TEST_ENVIRONMENT\"\n" +
		"printf '{\"example.com/sample\":{\"printf\":[{\"posn\":\"%s:2:6\",\"end\":\"%s:2:9\",\"message\":\"external diagnostic\"}]}}' \"$GLIPPY_TEST_SOURCE\" \"$GLIPPY_TEST_SOURCE\"\n" +
		"exit 1\n"
	if err := os.WriteFile(goBinary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner, err := NewUnitcheckerFactAnalyzerRunner(
		UnitcheckerFactAnalyzerRunnerOptions{
			Executable: "/opt//glippy",
			GoBinary: goBinary,
			Parallelism: 2,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := runner.RunPackageFactAnalyzer(
		context.Background(),
		PackageFactAnalyzerRequest{
			Identity: UnitcheckerPrintfIdentity,
			Analyzer: "printf",
			LoadOptions: PackageLoadOptions{
				Dir: root,
				Patterns: []string{"./..."},
				Tests: true,
				BuildTags: []string{"integration"},
				ModuleMode: ModuleReadonly,
				Env: append(
					os.Environ(),
					"GLIPPY_TEST_ARGUMENTS=" + argumentsPath,
					"GLIPPY_TEST_ENVIRONMENT=" + environmentPath,
					"GLIPPY_TEST_SOURCE=" + path,
				),
			},
			Sources: PackageSourceSet{
				paths: []string{path},
				files: map[string]*source.File{path: file},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 ||
		diagnostics[0].Range != (source.Range{Start: start, End: start + 3}) {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	wantArguments := "vet\n-json\n-p=2\n-vettool=/opt/glippy\n-mod=readonly\n-tags=integration\n./...\n"
	if string(arguments) != wantArguments {
		t.Fatalf("arguments = %q, want %q", arguments, wantArguments)
	}
	environment, err := os.ReadFile(environmentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(environment) != UnitcheckerPrintfIdentity + "\n" {
		t.Fatalf("unitchecker environment = %q", environment)
	}
}

func TestUnitcheckerFactAnalyzerRunnerDefersKnownPackageErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Glippy does not support Windows runtime execution")
	}

	root := t.TempDir()
	goBinary := filepath.Join(root, "go")
	if err := os.WriteFile(
		goBinary,
		[]byte("#!/bin/sh\nprintf '%s\\n' 'package failed' >&2\nexit 1\n"),
		0o700,
	);
		err != nil {
		t.Fatal(err)
	}
	runner, err := NewUnitcheckerFactAnalyzerRunner(
		UnitcheckerFactAnalyzerRunnerOptions{Executable: "/opt/glippy", GoBinary: goBinary},
	)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := runner.RunPackageFactAnalyzer(
		context.Background(),
		PackageFactAnalyzerRequest{
			Identity: UnitcheckerPrintfIdentity,
			Analyzer: "printf",
			PackageErrors: true,
			LoadOptions: PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				Tests: true,
			},
		},
	)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("known package failure = %#v, %v", diagnostics, err)
	}
}

func TestUnitcheckerFactAnalyzerRunnerHonorsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Glippy does not support Windows runtime execution")
	}

	root := t.TempDir()
	goBinary := filepath.Join(root, "go")
	if err := os.WriteFile(goBinary, []byte("#!/bin/sh\nexec sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	runner, err := NewUnitcheckerFactAnalyzerRunner(
		UnitcheckerFactAnalyzerRunnerOptions{Executable: "/opt/glippy", GoBinary: goBinary},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100 * time.Millisecond)
	defer cancel()
	_, err = runner.RunPackageFactAnalyzer(
		ctx,
		PackageFactAnalyzerRequest{
			Identity: UnitcheckerPrintfIdentity,
			Analyzer: "printf",
			LoadOptions: PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				Tests: true,
			},
		},
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("canceled unitchecker error = %v", err)
	}
}

func TestUnitcheckerFactAnalyzerRunnerRemovesOverlayFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Glippy does not support Windows runtime execution")
	}

	root := t.TempDir()
	argumentsPath := filepath.Join(root, "arguments")
	goBinary := filepath.Join(root, "go")
	if err := os.WriteFile(
		goBinary,
		[]byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$GLIPPY_TEST_ARGUMENTS\"\n"),
		0o700,
	);
		err != nil {
		t.Fatal(err)
	}
	runner, err := NewUnitcheckerFactAnalyzerRunner(
		UnitcheckerFactAnalyzerRunnerOptions{Executable: "/opt/glippy", GoBinary: goBinary},
	)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	_, err = runner.RunPackageFactAnalyzer(
		context.Background(),
		PackageFactAnalyzerRequest{
			Identity: UnitcheckerPrintfIdentity,
			Analyzer: "printf",
			LoadOptions: PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				Tests: true,
				Overlay: map[string][]byte{path: []byte("package sample\n")},
				Env: append(os.Environ(), "GLIPPY_TEST_ARGUMENTS=" + argumentsPath),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := ""
	for _, argument := range strings.Split(string(arguments), "\n") {
		if strings.HasPrefix(argument, "-overlay=") {
			overlayPath = strings.TrimPrefix(argument, "-overlay=")
			break
		}
	}
	if overlayPath == "" {
		t.Fatalf("unitchecker arguments have no overlay: %q", arguments)
	}
	if _, err := os.Stat(overlayPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unitchecker overlay still exists: %v", err)
	}
}

func TestUnitcheckerFactAnalyzerRunnerExecutesExactPrintfFacts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Glippy does not support Windows runtime execution")
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(path, contents string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "go.mod"), "module example.com/external\n\ngo 1.25\n")
	write(
		filepath.Join(root, "wrapped", "wrapped.go"),
		"package wrapped\n\nimport \"fmt\"\n\nfunc Warnf(format string, arguments ...any) { fmt.Printf(format, arguments...) }\n",
	)
	path := filepath.Join(root, "app", "app.go")
	input := "package app\n\nimport \"example.com/external/wrapped\"\n\nfunc run(format string) {\n\twrapped.Warnf(\"%d\", \"text\")\n\twrapped.Warnf(format)\n}\n"
	write(path, input)
	file, err := source.Load(path, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "glippy")
	build := exec.Command("go", "build", "-o", executable, "./cmd/glippy")
	build.Dir = filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Glippy vettool: %v: %s", err, output)
	}
	runner, err := NewUnitcheckerFactAnalyzerRunner(
		UnitcheckerFactAnalyzerRunnerOptions{Executable: executable},
	)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := runner.RunPackageFactAnalyzer(
		context.Background(),
		PackageFactAnalyzerRequest{
			Identity: UnitcheckerPrintfIdentity,
			Analyzer: "printf",
			LoadOptions: PackageLoadOptions{
				Dir: root,
				Patterns: []string{"./app"},
				Tests: true,
				ModuleMode: ModuleReadonly,
				Env: append(
					os.Environ(),
					"GOWORK=off",
					"GOENV=off",
					"CGO_ENABLED=0",
				),
			},
			Sources: PackageSourceSet{
				paths: []string{path},
				files: map[string]*source.File{path: file},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("printf diagnostics = %#v", diagnostics)
	}
	if got := input[diagnostics[0].Range.Start:diagnostics[0].Range.End]; got != "%d" {
		t.Fatalf("first printf range = %q", got)
	}
	if got := input[diagnostics[1].Range.Start:diagnostics[1].Range.End]; got != "format" {
		t.Fatalf("second printf range = %q", got)
	}
	if len(diagnostics[1].SuggestedFixes) != 1 ||
		diagnostics[1].SuggestedFixes[0].Message != "Insert \"%s\" format string" {
		t.Fatalf("printf suggestion = %#v", diagnostics[1].SuggestedFixes)
	}
}
