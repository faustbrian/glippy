package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestV1TextContractsMatchApprovedGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		golden string
	}{
		{name: "help", args: []string{"--help"}, golden: "help.txt"},
		{name: "rules", args: []string{"rules"}, golden: "rules.txt"},
	}

	for _, test := range tests {
		test := test
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				want, err := os.ReadFile(
					filepath.Join(
						"..",
						"..",
						"testdata",
						"contracts",
						"v1",
						test.golden,
					),
				)
				if err != nil {
					t.Fatal(err)
				}

				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := Run(test.args, bytes.NewReader(nil), &stdout, &stderr)
				if exitCode != ExitSuccess || stderr.Len() != 0 {
					t.Fatalf(
						"Run(%v) exit = %d, stderr = %q",
						test.args,
						exitCode,
						stderr.String(),
					)
				}
				if !bytes.Equal(stdout.Bytes(), want) {
					t.Fatalf(
						"Run(%v) output does not match %s",
						test.args,
						test.golden,
					)
				}
			},
		)
	}
}

func TestV1CommandHelpContractMatchesApprovedGolden(t *testing.T) {
	t.Parallel()

	contractRoot := filepath.Join("..", "..", "testdata", "contracts", "v1")
	want, err := os.ReadFile(filepath.Join(contractRoot, "commands.txt"))
	if err != nil {
		t.Fatal(err)
	}

	commands := []string{
		"fmt",
		"lint",
		"check",
		"lsp",
		"init",
		"config",
		"rules",
		"explain",
		"version",
		"completion",
		"help",
	}
	var listedCommands []string
	inCommands := false
	for _, line := range strings.Split(topLevelHelp, "\n") {
		if line == "Commands:" {
			inCommands = true
			continue
		}
		if inCommands && line == "" {
			break
		}
		if inCommands {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				listedCommands = append(listedCommands, fields[0])
			}
		}
	}
	if strings.Join(commands, "\n") != strings.Join(listedCommands, "\n") {
		t.Fatalf("command contract does not match top-level help: %v", listedCommands)
	}

	var got bytes.Buffer
	for _, command := range commands {
		got.WriteString("## " + command + "\n")
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run([]string{"help", command}, bytes.NewReader(nil), &stdout, &stderr)
		if exitCode != ExitSuccess || stderr.Len() != 0 {
			t.Fatalf(
				"help %s exit = %d, stderr = %q",
				command,
				exitCode,
				stderr.String(),
			)
		}
		got.Write(stdout.Bytes())
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("command help output does not match commands.txt")
	}
}

func TestV1CompletionContractsMatchApprovedDigests(t *testing.T) {
	t.Parallel()

	contractRoot := filepath.Join("..", "..", "testdata", "contracts", "v1")
	want, err := os.ReadFile(filepath.Join(contractRoot, "completions.txt"))
	if err != nil {
		t.Fatal(err)
	}

	var got bytes.Buffer
	for _, shell := range []string{"bash", "zsh", "fish"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(
			[]string{"completion", shell},
			bytes.NewReader(nil),
			&stdout,
			&stderr,
		)
		if exitCode != ExitSuccess || stderr.Len() != 0 {
			t.Fatalf(
				"completion %s exit = %d, stderr = %q",
				shell,
				exitCode,
				stderr.String(),
			)
		}
		fmt.Fprintf(
			&got,
			"%s\t%x\t%d\t%d\n",
			shell,
			sha256.Sum256(stdout.Bytes()),
			stdout.Len(),
			bytes.Count(stdout.Bytes(), []byte("\n")),
		)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("completion output does not match completions.txt")
	}
}

func TestV1FormatterOutputContractsMatchApprovedGoldens(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Join("..", "..")
	contractRoot := filepath.Join(repositoryRoot, "testdata", "contracts", "v1")
	manifest, err := os.ReadFile(filepath.Join(contractRoot, "formatter.txt"))
	if err != nil {
		t.Fatal(err)
	}
	seenNames := make(map[string]struct{})
	seenInputs := make(map[string]struct{})

	for _, line := range strings.Split(strings.TrimSpace(string(manifest)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 5 {
			t.Fatalf("invalid formatter contract entry %q", line)
		}
		name, width := fields[0], fields[1]
		if _, exists := seenNames[name]; exists {
			t.Fatalf("duplicate formatter contract name %q", name)
		}
		seenNames[name] = struct{}{}
		if _, exists := seenInputs[fields[2]]; exists {
			t.Fatalf("duplicate formatter contract input %q", fields[2])
		}
		seenInputs[fields[2]] = struct{}{}
		inputPath := filepath.Join(repositoryRoot, filepath.FromSlash(fields[2]))
		goldenPath := filepath.Join(repositoryRoot, filepath.FromSlash(fields[3]))
		wantDigest := fields[4]

		t.Run(
			name,
			func(t *testing.T) {
				t.Parallel()

				input, err := os.ReadFile(inputPath)
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatal(err)
				}
				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := Run(
					[]string{
						"fmt",
						"--config=" +
							filepath.Join(
								contractRoot,
								"formatter",
								"width-" + width + ".toml",
							),
					},
					bytes.NewReader(input),
					&stdout,
					&stderr,
				)
				if exitCode != ExitSuccess || stderr.Len() != 0 {
					t.Fatalf(
						"formatter contract exit = %d, stderr = %q",
						exitCode,
						stderr.String(),
					)
				}
				if !bytes.Equal(stdout.Bytes(), want) {
					t.Fatalf("formatter output does not match %s", fields[3])
				}
				gotDigest := fmt.Sprintf("%x", sha256.Sum256(stdout.Bytes()))
				if gotDigest != wantDigest {
					t.Fatalf(
						"formatter digest = %s, want %s",
						gotDigest,
						wantDigest,
					)
				}
			},
		)
	}

	expectedInputs := make(map[string]struct{})
	for _, pattern := range
		[]string{"testdata/corpus/hostile/*.go", "testdata/format/motivating/*.input"} {
		matches, err := filepath.Glob(
			filepath.Join(repositoryRoot, filepath.FromSlash(pattern)),
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			t.Fatalf("formatter contract pattern %q is empty", pattern)
		}
		for _, match := range matches {
			relative, err := filepath.Rel(repositoryRoot, match)
			if err != nil {
				t.Fatal(err)
			}
			expectedInputs[filepath.ToSlash(relative)] = struct{}{}
		}
	}
	for input := range expectedInputs {
		if _, exists := seenInputs[input]; !exists {
			t.Errorf("formatter contract does not freeze %s", input)
		}
	}
	for input := range seenInputs {
		if _, exists := expectedInputs[input]; !exists {
			t.Errorf("formatter contract has unexpected input %s", input)
		}
	}
}

type contractFailingWriter struct{}

func (contractFailingWriter) Write([]byte) (int, error) {
	return 0, errors.New("contract stream failure")
}

func TestV1FailureExitContractsMatchApprovedGolden(t *testing.T) {
	t.Parallel()

	contractRoot := filepath.Join("..", "..", "testdata", "contracts", "v1")
	want, err := os.ReadFile(filepath.Join(contractRoot, "failure-exits.txt"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		exitCode int
		run func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{
			name: "filesystem-error",
			exitCode: ExitFilesystemError,
			run: func(_ *bytes.Buffer, stderr *bytes.Buffer) int {
				return Run(
					[]string{"--help"},
					bytes.NewReader(nil),
					contractFailingWriter{},
					stderr,
				)
			},
		},
		{
			name: "internal-error",
			exitCode: ExitInternalError,
			run: func(stdout, stderr *bytes.Buffer) int {
				return runHelp(nil, "", stdout, stderr)
			},
		},
		{
			name: "canceled",
			exitCode: ExitCanceled,
			run: func(stdout, stderr *bytes.Buffer) int {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return RunContext(
					ctx,
					[]string{"--help"},
					bytes.NewReader(nil),
					stdout,
					stderr,
				)
			},
		},
	}

	var got bytes.Buffer
	for _, test := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := test.run(&stdout, &stderr)
		if exitCode != test.exitCode {
			t.Fatalf(
				"%s exit = %d, want %d; stderr = %q",
				test.name,
				exitCode,
				test.exitCode,
				stderr.String(),
			)
		}
		got.WriteString("## " + test.name + "\n")
		got.WriteString("exit: " + strconv.Itoa(exitCode) + "\n")
		got.WriteString("stdout:\n" + stdout.String())
		got.WriteString("stderr:\n" + stderr.String())
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("failure exit codes do not match failure-exits.txt")
	}
}

func TestV1ConfigurationProfilesMatchApprovedGolden(t *testing.T) {
	t.Parallel()

	contractRoot := filepath.Join("..", "..", "testdata", "contracts", "v1")
	want, err := os.ReadFile(filepath.Join(contractRoot, "profiles.txt"))
	if err != nil {
		t.Fatal(err)
	}
	configurationRoot := filepath.Join(contractRoot, "config")
	absoluteRoot, err := filepath.Abs(configurationRoot)
	if err != nil {
		t.Fatal(err)
	}

	var got bytes.Buffer
	for _, profile := range []string{"default", "recommended", "strict", "pedantic"} {
		got.WriteString("## " + profile + "\n")
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(
			[]string{
				"config",
				"show",
				"--config",
				filepath.Join(configurationRoot, profile + ".toml"),
				configurationRoot,
			},
			bytes.NewReader(nil),
			&stdout,
			&stderr,
		)
		if exitCode != ExitSuccess || stderr.Len() != 0 {
			t.Fatalf(
				"config show %s exit = %d, stderr = %q",
				profile,
				exitCode,
				stderr.String(),
			)
		}
		got.WriteString(strings.ReplaceAll(stdout.String(), absoluteRoot, "<ROOT>"))
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("curated profile output does not match profiles.txt")
	}
}

func TestV1ConfigurationBoundaryContractMatchesApprovedGolden(t *testing.T) {
	t.Parallel()

	contractRoot := filepath.Join("..", "..", "testdata", "contracts", "v1")
	want, err := os.ReadFile(filepath.Join(contractRoot, "configuration.txt"))
	if err != nil {
		t.Fatal(err)
	}
	absoluteContractRoot, err := filepath.Abs(contractRoot)
	if err != nil {
		t.Fatal(err)
	}
	machineRoot, err := filepath.Abs(filepath.Join(contractRoot, "machine"))
	if err != nil {
		t.Fatal(err)
	}
	initRoot := t.TempDir()

	var got bytes.Buffer
	for _, profile := range []string{"default", "recommended", "strict", "pedantic"} {
		root := filepath.Join(initRoot, profile)
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(
			[]string{"init", "--profile=" + profile, root},
			bytes.NewReader(nil),
			&stdout,
			&stderr,
		)
		if exitCode != ExitSuccess {
			t.Fatalf(
				"init %s exit = %d, stderr = %q",
				profile,
				exitCode,
				stderr.String(),
			)
		}
		configuration, err := os.ReadFile(filepath.Join(root, ".glippy.toml"))
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(root, ".glippy.toml"))
		if err != nil {
			t.Fatal(err)
		}
		got.WriteString("## init-" + profile + "\n")
		got.WriteString("exit: " + strconv.Itoa(exitCode) + "\n")
		got.WriteString("stdout:\n")
		got.WriteString(strings.ReplaceAll(stdout.String(), initRoot, "<INIT>"))
		got.WriteString("stderr:\n")
		got.WriteString(strings.ReplaceAll(stderr.String(), initRoot, "<INIT>"))
		got.WriteString("configuration:\n")
		got.Write(configuration)
		fmt.Fprintf(&got, "mode: %04o\n", info.Mode().Perm())
	}

	for _, caseName := range []string{"invalid-version", "invalid-unknown"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(
			[]string{
				"config",
				"check",
				"--config=" +
					filepath.Join(contractRoot, "config", caseName + ".toml"),
				machineRoot,
			},
			bytes.NewReader(nil),
			&stdout,
			&stderr,
		)
		if exitCode != ExitInvalidInvocation {
			t.Fatalf("%s exit = %d, stderr = %q", caseName, exitCode, stderr.String())
		}
		got.WriteString("## " + caseName + "\n")
		got.WriteString("exit: " + strconv.Itoa(exitCode) + "\n")
		got.WriteString("stdout:\n")
		got.WriteString(
			strings.ReplaceAll(stdout.String(), absoluteContractRoot, "<CONTRACT>"),
		)
		got.WriteString("stderr:\n")
		got.WriteString(
			strings.ReplaceAll(stderr.String(), absoluteContractRoot, "<CONTRACT>"),
		)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("configuration boundary output does not match configuration.txt")
	}
}

func TestV1DiagnosticReporterContractsMatchApprovedGolden(t *testing.T) {
	t.Parallel()

	contractRoot := filepath.Join("..", "..", "testdata", "contracts", "v1")
	want, err := os.ReadFile(filepath.Join(contractRoot, "reporters.txt"))
	if err != nil {
		t.Fatal(err)
	}
	machineRoot := filepath.Join(contractRoot, "machine")
	absoluteRoot, err := filepath.Abs(machineRoot)
	if err != nil {
		t.Fatal(err)
	}

	var got bytes.Buffer
	for _, reporter := range []string{"text", "short", "github", "sarif"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(
			[]string{
				"lint",
				"--reporter=" + reporter,
				"--config=" + filepath.Join(machineRoot, ".glippy.toml"),
				filepath.Join(machineRoot, "source.go"),
				filepath.Join(machineRoot, "alpha.go"),
			},
			bytes.NewReader(nil),
			&stdout,
			&stderr,
		)
		if exitCode != ExitFindings {
			t.Fatalf(
				"%s reporter exit = %d, stderr = %q",
				reporter,
				exitCode,
				stderr.String(),
			)
		}
		got.WriteString("## " + reporter + "\n")
		got.WriteString("exit: " + strconv.Itoa(exitCode) + "\n")
		got.WriteString("stdout:\n")
		got.WriteString(strings.ReplaceAll(stdout.String(), absoluteRoot, "<ROOT>"))
		got.WriteString("stderr:\n")
		got.WriteString(strings.ReplaceAll(stderr.String(), absoluteRoot, "<ROOT>"))
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("diagnostic reporter output does not match reporters.txt")
	}
}

func TestV1MachineAndExitContractsMatchApprovedGolden(t *testing.T) {
	t.Parallel()

	contractRoot := filepath.Join("..", "..", "testdata", "contracts", "v1")
	want, err := os.ReadFile(filepath.Join(contractRoot, "machine.txt"))
	if err != nil {
		t.Fatal(err)
	}
	machineRoot := filepath.Join(contractRoot, "machine")
	absoluteRoot, err := filepath.Abs(machineRoot)
	if err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(machineRoot, ".glippy.toml")
	alphaPath := filepath.Join(machineRoot, "alpha.go")
	sourcePath := filepath.Join(machineRoot, "source.go")
	invalidPath := filepath.Join(machineRoot, "invalid", "source.go")

	tests := []struct {
		name string
		args []string
		exitCode int
	}{
		{
			name: "format",
			args: []string{
				"fmt",
				"--check",
				"--reporter=json",
				"--config=" + configurationPath,
				sourcePath,
			},
			exitCode: ExitSuccess,
		},
		{
			name: "lint",
			args: []string{
				"lint",
				"--reporter=json",
				"--config=" + configurationPath,
				sourcePath,
				alphaPath,
			},
			exitCode: ExitFindings,
		},
		{
			name: "check",
			args: []string{
				"check",
				"--reporter=json",
				"--config=" + configurationPath,
				sourcePath,
				alphaPath,
			},
			exitCode: ExitFindings,
		},
		{
			name: "explain",
			args: []string{"explain", "self-assignment", "--json"},
			exitCode: ExitSuccess,
		},
		{
			name: "source-error",
			args: []string{
				"fmt",
				"--check",
				"--reporter=json",
				"--config=" + configurationPath,
				invalidPath,
			},
			exitCode: ExitSourceError,
		},
		{
			name: "invalid-invocation",
			args: []string{"unknown"},
			exitCode: ExitInvalidInvocation,
		},
		{name: "conflict", args: []string{"init", machineRoot}, exitCode: ExitConflict},
	}

	var got bytes.Buffer
	for _, test := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(test.args, bytes.NewReader(nil), &stdout, &stderr)
		if exitCode != test.exitCode {
			t.Fatalf(
				"Run(%v) exit = %d, want %d; stderr = %q",
				test.args,
				exitCode,
				test.exitCode,
				stderr.String(),
			)
		}
		got.WriteString("## " + test.name + "\n")
		got.WriteString("exit: " + strconv.Itoa(exitCode) + "\n")
		got.WriteString("stdout:\n")
		got.WriteString(strings.ReplaceAll(stdout.String(), absoluteRoot, "<ROOT>"))
		got.WriteString("stderr:\n")
		got.WriteString(strings.ReplaceAll(stderr.String(), absoluteRoot, "<ROOT>"))
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("machine output and exit codes do not match machine.txt")
	}
}
