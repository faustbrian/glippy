package cli

import (
	"context"
	"io"
	"strings"
)

const topLevelHelp = `Glippy formats, lints, and safely fixes Go source.

Usage:
  glippy <command> [options]

Commands:
  fmt         Format Go source
  lint        Report diagnostics and apply selected fixes
  check       Check formatting and diagnostics without mutation
  lsp         Serve editor integrations over stdio
  init        Create a starter configuration
  config      Validate or show configuration
  rules       List compiled lint rules
  explain     Explain one lint rule
  version     Print version information
  completion  Generate shell completion
  help        Show command help

Run 'glippy help <command>' for command usage.
`

const helpUsage = "glippy: expected 'help [command]'\n"

func parseHelpInvocation(arguments []string) (string, bool, bool) {
	switch len(arguments) {
	case 0:
		return "", true, true
	case 1:
		if arguments[0] == "help" || isHelpFlag(arguments[0]) {
			return "", true, true
		}
	case 2:
		if arguments[0] == "help" {
			if isHelpFlag(arguments[1]) {
				return "help", true, true
			}
			_, valid := helpTopicUsage(arguments[1])
			return arguments[1], true, valid
		}
		if isHelpFlag(arguments[1]) {
			_, valid := helpTopicUsage(arguments[0])
			return arguments[0], true, valid
		}
	default:
		if arguments[0] == "help" {
			return "", true, false
		}
	}
	for _, argument := range arguments {
		if isHelpFlag(argument) {
			return "", true, false
		}
	}
	return "", false, false
}

func isHelpFlag(argument string) bool {
	return argument == "--help" || argument == "-h"
}

func runHelp(
	ctx context.Context,
	topic string,
	stdout, stderr io.Writer,
) int {
	if ctx == nil {
		return report(stderr, ExitInternalError, "glippy help: context is required\n")
	}
	if err := ctx.Err(); err != nil {
		return report(stderr, ExitCanceled, "glippy help: %v\n", err)
	}
	output := topLevelHelp
	if topic != "" {
		usage, _ := helpTopicUsage(topic)
		output = "Usage:\n  glippy " + usage + "\n"
	}
	if err := write(stdout, []byte(output)); err != nil {
		return report(
			stderr,
			ExitFilesystemError,
			"glippy help: write standard output: %v\n",
			err,
		)
	}
	return ExitSuccess
}

func helpTopicUsage(topic string) (string, bool) {
	var usage string
	switch topic {
	case "fmt":
		usage = formatUsage
	case "lint":
		usage = lintUsage
	case "check":
		usage = checkUsage
	case "lsp":
		usage = lspUsage
	case "init":
		usage = initUsage
	case "config":
		usage = configUsage
	case "rules":
		usage = rulesUsage
	case "explain":
		usage = explainUsage
	case "version":
		usage = versionUsage
	case "completion":
		usage = completionUsage
	case "help":
		usage = helpUsage
	default:
		return "", false
	}
	return strings.TrimSuffix(
		strings.TrimPrefix(usage, "glippy: expected '"),
		"'\n",
	), true
}
