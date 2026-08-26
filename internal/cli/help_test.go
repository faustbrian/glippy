package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestRunDisplaysTopLevelHelp(t *testing.T) {
	t.Parallel()

	want := "Glippy formats, lints, and safely fixes Go source.\n\n" +
		"Usage:\n" +
		"  glippy <command> [options]\n\n" +
		"Commands:\n" +
		"  fmt         Format Go source\n" +
		"  lint        Report diagnostics and apply selected fixes\n" +
		"  check       Check formatting and diagnostics without mutation\n" +
		"  lsp         Serve editor integrations over stdio\n" +
		"  init        Create a starter configuration\n" +
		"  config      Validate or show configuration\n" +
		"  rules       List compiled lint rules\n" +
		"  explain     Explain one lint rule\n" +
		"  version     Print version information\n" +
		"  completion  Generate shell completion\n" +
		"  help        Show command help\n\n" +
		"Run 'glippy help <command>' for command usage.\n"
	for _, arguments := range [][]string{
		nil,
		{"--help"},
		{"-h"},
		{"help"},
	} {
		arguments := arguments
		t.Run(fmt.Sprintf("%q", arguments), func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := Run(arguments, failingReader{}, &stdout, &stderr)
			if exitCode != ExitSuccess || stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf(
					"Run(%q) = exit %d, stdout %q, stderr %q",
					arguments,
					exitCode,
					stdout.String(),
					stderr.String(),
				)
			}
		})
	}
}

func TestRunRejectsInvalidHelpInvocation(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{
		{"help", "unknown"},
		{"help", "fmt", "extra"},
		{"unknown", "--help"},
		{"fmt", "--help", "--reporter=json"},
		{"lint", "--help", "--reporter=json"},
		{"check", "--help", "--reporter=sarif"},
	} {
		arguments := arguments
		t.Run(fmt.Sprintf("%q", arguments), func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := Run(arguments, failingReader{}, &stdout, &stderr)
			if exitCode != ExitInvalidInvocation || stdout.Len() != 0 ||
				stderr.String() != "glippy: expected 'help [command]'\n" {
				t.Fatalf(
					"Run(%q) = exit %d, stdout %q, stderr %q",
					arguments,
					exitCode,
					stdout.String(),
					stderr.String(),
				)
			}
		})
	}
}

func TestRunDisplaysCommandHelp(t *testing.T) {
	t.Parallel()

	topics := map[string]string{
		"fmt":        "fmt [--write|--check|--diff] [--reporter=text|json] [--config=<path>] [--stdin-filepath=<path>] [--fragment=declaration|statement|expression] [path...]",
		"lint":       "lint [--fix] [--fix-suggestions] [--fix-unsafe] [--diff] [-A|--allow <rules-or-groups>] [-W|--warn <rules-or-groups>] [-D|--deny <rules-or-groups>] [-F|--forbid <rules-or-groups>] [--only=<rules>] [--except=<rules>] [--new-from=<git-ref>] [--generate-baseline=<path>] [--reporter=text|short|json|github|sarif] [--stats[=text|json]] [--config=<path>] [path...]",
		"check":      "check [-A|--allow <rules-or-groups>] [-W|--warn <rules-or-groups>] [-D|--deny <rules-or-groups>] [-F|--forbid <rules-or-groups>] [--new-from=<git-ref>] [--reporter=text|short|json|github|sarif] [--stats[=text|json]] [--config=<path>] [path...]",
		"lsp":        "lsp [--fix-suggestions] [--fix-unsafe] [--config=<path>]",
		"init":       "init [--profile=<profile>] [directory]",
		"config":     "config <check|show> [--config=<path>] [path]",
		"rules":      "rules [--preset=<preset>] [--fixable] [--tier=lexical|syntax|types|cfg|ssa]",
		"explain":    "explain <rule> [--json]",
		"version":    "version",
		"completion": "completion <bash|zsh|fish>",
		"help":       "help [command]",
	}
	for topic, usage := range topics {
		topic := topic
		usage := usage
		invocations := [][]string{
			{"help", topic},
			{topic, "--help"},
			{topic, "-h"},
		}
		for _, arguments := range invocations {
			arguments := arguments
			t.Run(fmt.Sprintf("%s/%q", topic, arguments), func(t *testing.T) {
				t.Parallel()

				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := Run(arguments, failingReader{}, &stdout, &stderr)
				want := "Usage:\n  glippy " + usage + "\n"
				if exitCode != ExitSuccess || stdout.String() != want || stderr.Len() != 0 {
					t.Fatalf(
						"Run(%q) = exit %d, stdout %q, stderr %q",
						arguments,
						exitCode,
						stdout.String(),
						stderr.String(),
					)
				}
			})
		}
	}
}

func TestRunHelpObservesCancellationAndOutputFailure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(ctx, []string{"--help"}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitCanceled || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "context canceled") {
		t.Fatalf(
			"RunContext(canceled help) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	stderr.Reset()
	exitCode = Run([]string{"--help"}, failingReader{}, failingWriter{}, &stderr)
	if exitCode != ExitFilesystemError ||
		!strings.Contains(stderr.String(), "write standard output") {
		t.Fatalf(
			"Run(help output failure) = exit %d, stderr %q",
			exitCode,
			stderr.String(),
		)
	}
}
