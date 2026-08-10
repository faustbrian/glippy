// Package cli owns Gox command dispatch and process-facing I/O contracts.
package cli

import (
	"fmt"
	"io"

	goxformat "github.com/faustbrian/gox/internal/format"
	"github.com/faustbrian/gox/internal/source"
)

const (
	ExitSuccess           = 0
	ExitSourceError       = 2
	ExitInvalidInvocation = 3
	ExitFilesystemError   = 5
	ExitInternalError     = 6
)

var defaultFormatOptions = goxformat.Options{
	Width:     100,
	TabWidth:  8,
	FitBudget: 1_000,
}

const formatUsage = "gox: expected 'fmt' or 'fmt --fragment=declaration|statement|expression' with standard input\n"

// Run executes one Gox invocation against explicit process streams.
func Run(arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdin == nil || stdout == nil || stderr == nil {
		if stderr == nil {
			return ExitFilesystemError
		}
		return report(stderr, ExitFilesystemError, "gox: process streams are required\n")
	}
	fragmentKind, valid := parseFormatInvocation(arguments)
	if !valid {
		return report(stderr, ExitInvalidInvocation, formatUsage)
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: read standard input: %v\n", err)
	}
	formatted, exitCode, err := formatStandardInput(input, fragmentKind)
	if err != nil {
		return report(stderr, exitCode, "gox fmt: %v\n", err)
	}
	if err := write(stdout, formatted); err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: write standard output: %v\n", err)
	}
	return ExitSuccess
}

func formatStandardInput(input []byte, fragmentKind source.FragmentKind) ([]byte, int, error) {
	if fragmentKind != 0 {
		fragment, err := source.LoadFragment("stdin.go", fragmentKind, input)
		if err != nil {
			return nil, ExitSourceError, err
		}
		formatted, err := goxformat.Fragment(fragment, defaultFormatOptions)
		if err != nil {
			return nil, ExitInternalError, err
		}
		return formatted, ExitSuccess, nil
	}
	file, err := source.Load("stdin.go", input)
	if err != nil {
		return nil, ExitSourceError, err
	}
	formatted, err := goxformat.File(file, defaultFormatOptions)
	if err != nil {
		return nil, ExitInternalError, err
	}
	return formatted, ExitSuccess, nil
}

func parseFormatInvocation(arguments []string) (source.FragmentKind, bool) {
	if len(arguments) == 1 && arguments[0] == "fmt" {
		return 0, true
	}
	if len(arguments) != 2 || arguments[0] != "fmt" {
		return 0, false
	}
	switch arguments[1] {
	case "--fragment=declaration":
		return source.FragmentDeclaration, true
	case "--fragment=statement":
		return source.FragmentStatement, true
	case "--fragment=expression":
		return source.FragmentExpression, true
	default:
		return 0, false
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
