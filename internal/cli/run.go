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

// Run executes one Gox invocation against explicit process streams.
func Run(arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdin == nil || stdout == nil || stderr == nil {
		if stderr == nil {
			return ExitFilesystemError
		}
		return report(stderr, ExitFilesystemError, "gox: process streams are required\n")
	}
	if len(arguments) != 1 || arguments[0] != "fmt" {
		return report(stderr, ExitInvalidInvocation, "gox: expected 'fmt' with standard input\n")
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: read standard input: %v\n", err)
	}
	file, err := source.Load("stdin.go", input)
	if err != nil {
		return report(stderr, ExitSourceError, "gox fmt: %v\n", err)
	}
	formatted, err := goxformat.File(file, defaultFormatOptions)
	if err != nil {
		return report(stderr, ExitInternalError, "gox fmt: %v\n", err)
	}
	if err := write(stdout, formatted); err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: write standard output: %v\n", err)
	}
	return ExitSuccess
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
