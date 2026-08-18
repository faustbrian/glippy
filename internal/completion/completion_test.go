package completion_test

import (
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/completion"
)

func TestRenderProducesDeterministicShellCompletions(t *testing.T) {
	t.Parallel()

	ruleIDs := []string{"ineffective-break", "duplicate-condition"}
	tests := []struct {
		shell completion.Shell
		markers []string
		absent []string
	}{
		{
			shell: completion.Bash,
			markers: []string{
				"complete -F _glippy_completion glippy",
				"fmt lint check lsp init config rules explain version completion",
				"--fix --fix-suggestions --fix-unsafe --diff",
				"-A -W -D -F --allow --warn --deny --forbid",
				"--only --except",
				"--generate-baseline=",
				"--new-from=",
				"--preset --preset= --fixable --tier --tier=",
				"correctness suspicious performance complexity style pedantic restriction migration",
				"lexical syntax types cfg ssa",
				"--json",
				"text short json github sarif",
				"--fix-suggestions --fix-unsafe --config",
				"--profile --profile=default --profile=recommended --profile=strict --profile=pedantic",
				"--fragment=declaration --fragment=statement --fragment=expression",
				"duplicate-condition ineffective-break",
			},
			absent: []string{"--fragment --fragment=declaration"},
		},
		{
			shell: completion.Zsh,
			markers: []string{
				"#compdef glippy",
				"fmt lint check lsp init config rules explain version completion",
				"--stdin-filepath",
				"--only",
				"--except",
				"--allow=[set allow lint level]",
				"--forbid=[set forbid lint level]",
				"--generate-baseline",
				"--new-from",
				"--diff[preview validated fixes without writing]",
				"--fixable",
				"--tier",
				"--json",
				"reporter:(text short json github sarif)",
				"lsp)",
				"--fix-suggestions[offer suggestion code actions]",
				"--profile=[select starter lint profile]:profile:(default recommended strict pedantic)",
				"duplicate-condition ineffective-break",
			},
		},
		{
			shell: completion.Fish,
			markers: []string{
				"complete -c glippy -f",
				"__fish_seen_subcommand_from fmt lint check' -F",
				" -l fix-unsafe ",
				"__fish_seen_subcommand_from lint' -l diff",
				" -l only ",
				" -l except ",
				" -l allow ",
				" -l forbid ",
				" -l generate-baseline ",
				" -l new-from ",
				" -a rules -d 'List lint rules'",
				" -a lsp -d 'Serve editor diagnostics and code actions'",
				" -l preset ",
				" -l fixable ",
				" -l tier ",
				" -l json ",
				"__fish_seen_subcommand_from lsp' -l fix-suggestions",
				"__fish_seen_subcommand_from init' -l profile",
				"-a 'text short json github sarif'",
				"-a '--fragment=declaration --fragment=statement --fragment=expression'",
				"duplicate-condition",
				"ineffective-break",
			},
			absent: []string{" -l fragment "},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(
			string(test.shell),
			func(t *testing.T) {
				t.Parallel()

				first, err := completion.Render(test.shell, ruleIDs)
				if err != nil {
					t.Fatal(err)
				}
				second, err := completion.Render(test.shell, ruleIDs)
				if err != nil {
					t.Fatal(err)
				}
				if string(first) != string(second) {
					t.Fatal("Render() is nondeterministic")
				}
				if len(first) == 0 || first[len(first) - 1] != '\n' {
					t.Fatalf(
						"Render() output does not end in one newline: %q",
						first,
					)
				}
				for _, marker := range test.markers {
					if !strings.Contains(string(first), marker) {
						t.Fatalf(
							"Render() output does not contain %q:\n%s",
							marker,
							first,
						)
					}
				}
				for _, marker := range test.absent {
					if strings.Contains(string(first), marker) {
						t.Fatalf(
							"Render() output contains invalid form %q:\n%s",
							marker,
							first,
						)
					}
				}
			},
		)
	}
}

func TestRenderRejectsUnsupportedShellAndUnsafeRuleID(t *testing.T) {
	t.Parallel()

	if _, err := completion.Render(completion.Shell("powershell"), nil); err == nil {
		t.Fatal("Render() accepted an unsupported shell")
	}
	if _, err := completion.Render(completion.Bash, []string{"valid-rule", "$(unsafe)"});
		err == nil {
		t.Fatal("Render() accepted an unsafe rule ID")
	}
}
