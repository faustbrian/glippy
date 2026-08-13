// Package completion renders deterministic shell completion scripts.
package completion

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Shell identifies one supported completion-script dialect.
type Shell string

const (
	Bash Shell = "bash"
	Zsh Shell = "zsh"
	Fish Shell = "fish"
)

var ruleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// Render returns one complete completion script for shell. Rule IDs are
// validated and ordered before interpolation into shell-owned word lists.
func Render(shell Shell, ruleIDs []string) ([]byte, error) {
	ids := slices.Clone(ruleIDs)
	slices.Sort(ids)
	for index, id := range ids {
		if !ruleIDPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid completion rule ID %q", id)
		}
		if index > 0 && ids[index - 1] == id {
			return nil, fmt.Errorf("duplicate completion rule ID %q", id)
		}
	}

	var script string
	switch shell {
	case Bash:
		script = renderBash(ids)
	case Zsh:
		script = renderZsh(ids)
	case Fish:
		script = renderFish(ids)
	default:
		return nil, fmt.Errorf("unsupported completion shell %q", shell)
	}
	if !strings.HasSuffix(script, "\n") {
		script += "\n"
	}
	return []byte(script), nil
}

func renderBash(ruleIDs []string) string {
	return `# bash completion for gox
_gox_completion() {
	local current previous command
	COMPREPLY=()
	current="${COMP_WORDS[COMP_CWORD]}"
	previous=""
	if (( COMP_CWORD > 0 )); then
		previous="${COMP_WORDS[COMP_CWORD-1]}"
	fi
	command="${COMP_WORDS[1]}"

	if (( COMP_CWORD == 1 )); then
		COMPREPLY=( $(compgen -W "fmt lint check explain version completion" -- "$current") )
		return
	fi

	case "$command:$previous" in
		fmt:--reporter|lint:--reporter|check:--reporter)
			COMPREPLY=( $(compgen -W "text json" -- "$current") )
			return
			;;
		fmt:--config|fmt:--stdin-filepath|lint:--config|check:--config)
			COMPREPLY=( $(compgen -f -- "$current") )
			return
			;;
	esac

	case "$command" in
		fmt)
			COMPREPLY=( $(compgen -W "--write --check --diff --reporter --reporter=text --reporter=json --config --stdin-filepath --fragment=declaration --fragment=statement --fragment=expression" -- "$current") )
			COMPREPLY+=( $(compgen -f -- "$current") )
			;;
		lint)
			COMPREPLY=( $(compgen -W "--fix --fix-suggestions --fix-unsafe --reporter --reporter=text --reporter=json --config" -- "$current") )
			COMPREPLY+=( $(compgen -f -- "$current") )
			;;
		check)
			COMPREPLY=( $(compgen -W "--reporter --reporter=text --reporter=json --config" -- "$current") )
			COMPREPLY+=( $(compgen -f -- "$current") )
			;;
		explain)
			COMPREPLY=( $(compgen -W "` +
		strings.Join(ruleIDs, " ") +
		`" -- "$current") )
			;;
		completion)
			COMPREPLY=( $(compgen -W "bash zsh fish" -- "$current") )
			;;
	esac
}
complete -F _gox_completion gox
`
}

func renderZsh(ruleIDs []string) string {
	return `#compdef gox

_gox() {
	local context state state_descr line
	typeset -A opt_args
	_arguments -C \
		'1:command:(fmt lint check explain version completion)' \
		'*::argument:->arguments'

	case "$line[1]" in
		fmt)
			_arguments \
				'--write[write formatted files in place]' \
				'--check[report files whose formatting differs]' \
				'--diff[print unified formatting differences]' \
				'--reporter=[select reporter]:reporter:(text json)' \
				'--config=[use an explicit configuration]:configuration file:_files' \
				'--stdin-filepath=[supply stdin path context]:source path:_files' \
				'--fragment=[format a source fragment]:fragment kind:(declaration statement expression)' \
				'*:path:_files'
			;;
		lint)
			_arguments \
				'--fix[apply safe fixes]' \
				'--fix-suggestions[apply suggestion fixes]' \
				'--fix-unsafe[apply unsafe fixes]' \
				'--reporter=[select reporter]:reporter:(text json)' \
				'--config=[use an explicit configuration]:configuration file:_files' \
				'*:path:_files'
			;;
		check)
			_arguments \
				'--reporter=[select reporter]:reporter:(text json)' \
				'--config=[use an explicit configuration]:configuration file:_files' \
				'*:path:_files'
			;;
		explain)
			_arguments '1:rule ID:(` +
		strings.Join(ruleIDs, " ") +
		`)'
			;;
		completion)
			_arguments '1:shell:(bash zsh fish)'
			;;
	esac
}

_gox "$@"
`
}

func renderFish(ruleIDs []string) string {
	var output strings.Builder
	output.WriteString(
		`complete -c gox -f
complete -c gox -n '__fish_use_subcommand' -a fmt -d 'Format Go source'
complete -c gox -n '__fish_use_subcommand' -a lint -d 'Lint Go source'
complete -c gox -n '__fish_use_subcommand' -a check -d 'Check formatting and lint diagnostics'
complete -c gox -n '__fish_use_subcommand' -a explain -d 'Explain a lint rule'
complete -c gox -n '__fish_use_subcommand' -a version -d 'Print the Gox version'
complete -c gox -n '__fish_use_subcommand' -a completion -d 'Generate shell completions'
complete -c gox -n '__fish_seen_subcommand_from fmt lint check' -F

complete -c gox -n '__fish_seen_subcommand_from fmt' -l write -d 'Write formatted files in place'
complete -c gox -n '__fish_seen_subcommand_from fmt' -l check -d 'Report files whose formatting differs'
complete -c gox -n '__fish_seen_subcommand_from fmt' -l diff -d 'Print unified formatting differences'
complete -c gox -n '__fish_seen_subcommand_from fmt lint check' -l reporter -r -a 'text json' -d 'Select reporter'
complete -c gox -n '__fish_seen_subcommand_from fmt lint check' -l config -r -F -d 'Use an explicit configuration'
complete -c gox -n '__fish_seen_subcommand_from fmt' -l stdin-filepath -r -F -d 'Supply stdin path context'
complete -c gox -n '__fish_seen_subcommand_from fmt' -a '--fragment=declaration --fragment=statement --fragment=expression' -d 'Format a source fragment'

complete -c gox -n '__fish_seen_subcommand_from lint' -l fix -d 'Apply safe fixes'
complete -c gox -n '__fish_seen_subcommand_from lint' -l fix-suggestions -d 'Apply suggestion fixes'
complete -c gox -n '__fish_seen_subcommand_from lint' -l fix-unsafe -d 'Apply unsafe fixes'

`,
	)
	for _, ruleID := range ruleIDs {
		fmt.Fprintf(
			&output,
			"complete -c gox -n '__fish_seen_subcommand_from explain' -a %s\n",
			ruleID,
		)
	}
	output.WriteString(
		`complete -c gox -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`,
	)
	return output.String()
}
