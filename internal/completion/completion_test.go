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
				"fmt lint check init config rules explain version completion",
				"--fix-suggestions",
				"--only --except",
				"--generate-baseline=",
				"--new-from=",
				"--preset --preset= --fixable --tier --tier=",
				"correctness suspicious performance complexity style pedantic restriction migration",
				"lexical syntax types cfg ssa",
				"--json",
				"text json github sarif",
				"--fragment=declaration --fragment=statement --fragment=expression",
				"duplicate-condition ineffective-break",
			},
			absent: []string{"--fragment --fragment=declaration"},
		},
		{
			shell: completion.Zsh,
			markers: []string{
				"#compdef glippy",
				"fmt lint check init config rules explain version completion",
				"--stdin-filepath",
				"--only",
				"--except",
				"--generate-baseline",
				"--new-from",
				"--fixable",
				"--tier",
				"--json",
				"reporter:(text json github sarif)",
				"duplicate-condition ineffective-break",
			},
		},
		{
			shell: completion.Fish,
			markers: []string{
				"complete -c glippy -f",
				"__fish_seen_subcommand_from fmt lint check' -F",
				" -l fix-unsafe ",
				" -l only ",
				" -l except ",
				" -l generate-baseline ",
				" -l new-from ",
				" -a rules -d 'List lint rules'",
				" -l preset ",
				" -l fixable ",
				" -l tier ",
				" -l json ",
				"-a 'text json github sarif'",
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
