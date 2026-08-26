package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestV1TextContractsMatchApprovedGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		golden string
	}{
		{name: "help", args: []string{"--help"}, golden: "help.txt"},
		{name: "rules", args: []string{"rules"}, golden: "rules.txt"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			want, err := os.ReadFile(filepath.Join(
				"..", "..", "testdata", "contracts", "v1", test.golden,
			))
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
				t.Fatalf("Run(%v) output does not match %s", test.args, test.golden)
			}
		})
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
		name     string
		exitCode int
		run      func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{
			name:     "filesystem-error",
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
			name:     "internal-error",
			exitCode: ExitInternalError,
			run: func(stdout, stderr *bytes.Buffer) int {
				return runHelp(nil, "", stdout, stderr)
			},
		},
		{
			name:     "canceled",
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
				filepath.Join(configurationRoot, profile+".toml"),
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
		name     string
		args     []string
		exitCode int
	}{
		{
			name: "format",
			args: []string{
				"fmt", "--check", "--reporter=json",
				"--config=" + configurationPath, sourcePath,
			},
			exitCode: ExitSuccess,
		},
		{
			name: "lint",
			args: []string{
				"lint", "--reporter=json",
				"--config=" + configurationPath, sourcePath, alphaPath,
			},
			exitCode: ExitFindings,
		},
		{
			name: "check",
			args: []string{
				"check", "--reporter=json",
				"--config=" + configurationPath, sourcePath, alphaPath,
			},
			exitCode: ExitFindings,
		},
		{
			name:     "explain",
			args:     []string{"explain", "self-assignment", "--json"},
			exitCode: ExitSuccess,
		},
		{
			name: "source-error",
			args: []string{
				"fmt", "--check", "--reporter=json",
				"--config=" + configurationPath, invalidPath,
			},
			exitCode: ExitSourceError,
		},
		{
			name:     "invalid-invocation",
			args:     []string{"unknown"},
			exitCode: ExitInvalidInvocation,
		},
		{
			name:     "conflict",
			args:     []string{"init", machineRoot},
			exitCode: ExitConflict,
		},
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
