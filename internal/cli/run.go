// Package cli owns Gox command dispatch and process-facing I/O contracts.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/faustbrian/gox/internal/config"
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

const formatUsage = "gox: expected 'fmt' with standard input and optional --fragment=declaration|statement|expression, --stdin-filepath=<path>, or --config=<path>\n"

type formatInvocation struct {
	fragmentKind  source.FragmentKind
	stdinFilepath string
	configPath    string
}

// Run executes one Gox invocation against explicit process streams.
func Run(arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdin == nil || stdout == nil || stderr == nil {
		if stderr == nil {
			return ExitFilesystemError
		}
		return report(stderr, ExitFilesystemError, "gox: process streams are required\n")
	}
	invocation, valid := parseFormatInvocation(arguments)
	if !valid {
		return report(stderr, ExitInvalidInvocation, formatUsage)
	}
	formatOptions, exitCode, err := resolveFormatOptions(invocation)
	if err != nil {
		return report(stderr, exitCode, "gox fmt: %v\n", err)
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		return report(stderr, ExitFilesystemError, "gox fmt: read standard input: %v\n", err)
	}
	sourcePath := invocation.stdinFilepath
	if sourcePath == "" {
		sourcePath = "stdin.go"
	}
	formatted, exitCode, err := formatStandardInput(input, sourcePath, invocation.fragmentKind, formatOptions)
	if err != nil {
		return report(stderr, exitCode, "gox fmt: %v\n", err)
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
		case argument == "--config" && result.configPath == "" &&
			index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "--"):
			index++
			result.configPath = arguments[index]
			if result.configPath == "" {
				return formatInvocation{}, false
			}
		default:
			return formatInvocation{}, false
		}
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
