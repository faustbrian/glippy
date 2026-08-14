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
	lintTargets := "warnings correctness suspicious performance complexity style pedantic " +
		strings.Join(ruleIDs, " ")
	return `# bash completion for glippy
_glippy_completion() {
	local current previous command
	COMPREPLY=()
	current="${COMP_WORDS[COMP_CWORD]}"
	previous=""
	if (( COMP_CWORD > 0 )); then
		previous="${COMP_WORDS[COMP_CWORD-1]}"
	fi
	command="${COMP_WORDS[1]}"

	if (( COMP_CWORD == 1 )); then
		COMPREPLY=( $(compgen -W "fmt lint check lsp init config rules explain version completion" -- "$current") )
		return
	fi
	if [[ "$command" == "config" && COMP_CWORD -eq 2 ]]; then
		COMPREPLY=( $(compgen -W "check show" -- "$current") )
		return
	fi

	case "$command:$previous" in
		fmt:--reporter)
			COMPREPLY=( $(compgen -W "text json" -- "$current") )
			return
			;;
		lint:--reporter|check:--reporter)
			COMPREPLY=( $(compgen -W "text json github sarif" -- "$current") )
			return
			;;
		lint:--only|lint:--except)
			COMPREPLY=( $(compgen -W "` +
		strings.Join(ruleIDs, " ") +
		`" -- "$current") )
			return
			;;
		lint:-A|lint:-W|lint:-D|lint:-F|lint:--allow|lint:--warn|lint:--deny|lint:--forbid|check:-A|check:-W|check:-D|check:-F|check:--allow|check:--warn|check:--deny|check:--forbid)
			COMPREPLY=( $(compgen -W "` +
		lintTargets +
		`" -- "$current") )
			return
			;;
		rules:--preset)
			COMPREPLY=( $(compgen -W "correctness suspicious performance complexity style pedantic restriction migration" -- "$current") )
			return
			;;
		rules:--tier)
			COMPREPLY=( $(compgen -W "lexical syntax types cfg ssa" -- "$current") )
			return
			;;
		fmt:--config|fmt:--stdin-filepath|lint:--config|check:--config|lsp:--config|config:--config)
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
			COMPREPLY=( $(compgen -W "-A -W -D -F --allow --warn --deny --forbid --fix --fix-suggestions --fix-unsafe --only --except --new-from= --generate-baseline= --reporter --reporter=text --reporter=json --reporter=github --reporter=sarif --config" -- "$current") )
			COMPREPLY+=( $(compgen -f -- "$current") )
			;;
		check)
			COMPREPLY=( $(compgen -W "-A -W -D -F --allow --warn --deny --forbid --new-from= --reporter --reporter=text --reporter=json --reporter=github --reporter=sarif --config" -- "$current") )
			COMPREPLY+=( $(compgen -f -- "$current") )
			;;
		lsp)
			COMPREPLY=( $(compgen -W "--fix-suggestions --fix-unsafe --config" -- "$current") )
			;;
		init)
			COMPREPLY=( $(compgen -d -- "$current") )
			;;
		config)
			COMPREPLY=( $(compgen -W "--config --config=" -- "$current") )
			COMPREPLY+=( $(compgen -f -- "$current") )
			;;
		rules)
			COMPREPLY=( $(compgen -W "--preset --preset= --fixable --tier --tier=" -- "$current") )
			;;
		explain)
			COMPREPLY=( $(compgen -W "--json ` +
		strings.Join(ruleIDs, " ") +
		`" -- "$current") )
			;;
		completion)
			COMPREPLY=( $(compgen -W "bash zsh fish" -- "$current") )
			;;
	esac
}
complete -F _glippy_completion glippy
`
}

func renderZsh(ruleIDs []string) string {
	lintTargets := "warnings correctness suspicious performance complexity style pedantic " +
		strings.Join(ruleIDs, " ")
	return `#compdef glippy

_glippy() {
	local context state state_descr line
	typeset -A opt_args
	_arguments -C \
		'1:command:(fmt lint check lsp init config rules explain version completion)' \
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
				'-A[set allow lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'--allow=[set allow lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'-W[set warn lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'--warn=[set warn lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'-D[set deny lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'--deny=[set deny lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'-F[set forbid lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'--forbid=[set forbid lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'--fix[apply safe fixes]' \
				'--fix-suggestions[apply suggestion fixes]' \
				'--fix-unsafe[apply unsafe fixes]' \
				'--only=[run only exact rule IDs]:rule IDs:('` +
		strings.Join(ruleIDs, " ") +
		`')' \
				'--except=[exclude exact rule IDs]:rule IDs:('` +
		strings.Join(ruleIDs, " ") +
		`')' \
				'--new-from=[report findings introduced since a Git ref]:git ref' \
				'--generate-baseline=[write lint baseline]:baseline path:_files' \
				'--reporter=[select reporter]:reporter:(text json github sarif)' \
				'--config=[use an explicit configuration]:configuration file:_files' \
				'*:path:_files'
			;;
		check)
			_arguments \
				'-A[set allow lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'--allow=[set allow lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'-W[set warn lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'--warn=[set warn lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'-D[set deny lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'--deny=[set deny lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'-F[set forbid lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'--forbid=[set forbid lint level]:rule or group:('` +
		lintTargets +
		`')' \
				'--new-from=[report findings introduced since a Git ref]:git ref' \
				'--reporter=[select reporter]:reporter:(text json github sarif)' \
				'--config=[use an explicit configuration]:configuration file:_files' \
				'*:path:_files'
			;;
		lsp)
			_arguments \
				'--fix-suggestions[offer suggestion code actions]' \
				'--fix-unsafe[offer unsafe code actions]' \
				'--config=[use an explicit configuration]:configuration file:_files'
			;;
		init)
			_arguments '1:directory:_directories'
			;;
		config)
			_arguments \
				'1:config command:(check show)' \
				'--config=[use an explicit configuration]:configuration file:_files' \
				'2:path:_files'
			;;
		rules)
			_arguments \
				'--preset=[filter by preset]:preset:(correctness suspicious performance complexity style pedantic restriction migration)' \
				'--fixable[show only rules with fixes]' \
				'--tier=[filter by exact analysis tier]:tier:(lexical syntax types cfg ssa)'
			;;
		explain)
			_arguments \
				'--json[render versioned JSON]' \
				'1:rule ID:(` +
		strings.Join(ruleIDs, " ") +
		`)'
			;;
		completion)
			_arguments '1:shell:(bash zsh fish)'
			;;
	esac
}

_glippy "$@"
`
}

func renderFish(ruleIDs []string) string {
	var output strings.Builder
	lintTargets := "warnings correctness suspicious performance complexity style pedantic " +
		strings.Join(ruleIDs, " ")
	output.WriteString(
		`complete -c glippy -f
complete -c glippy -n '__fish_use_subcommand' -a fmt -d 'Format Go source'
complete -c glippy -n '__fish_use_subcommand' -a lint -d 'Lint Go source'
complete -c glippy -n '__fish_use_subcommand' -a check -d 'Check formatting and lint diagnostics'
complete -c glippy -n '__fish_use_subcommand' -a lsp -d 'Serve editor diagnostics and code actions'
complete -c glippy -n '__fish_use_subcommand' -a init -d 'Create a Glippy configuration'
complete -c glippy -n '__fish_use_subcommand' -a config -d 'Inspect Glippy configuration'
complete -c glippy -n '__fish_use_subcommand' -a rules -d 'List lint rules'
complete -c glippy -n '__fish_use_subcommand' -a explain -d 'Explain a lint rule'
complete -c glippy -n '__fish_use_subcommand' -a version -d 'Print the Glippy version'
complete -c glippy -n '__fish_use_subcommand' -a completion -d 'Generate shell completions'
complete -c glippy -n '__fish_seen_subcommand_from fmt lint check' -F
complete -c glippy -n '__fish_seen_subcommand_from config; and __fish_seen_subcommand_from check show' -F
complete -c glippy -n '__fish_seen_subcommand_from init' -a '(__fish_complete_directories)'
complete -c glippy -n '__fish_seen_subcommand_from config; and not __fish_seen_subcommand_from check show' -a 'check show'

complete -c glippy -n '__fish_seen_subcommand_from fmt' -l write -d 'Write formatted files in place'
complete -c glippy -n '__fish_seen_subcommand_from fmt' -l check -d 'Report files whose formatting differs'
complete -c glippy -n '__fish_seen_subcommand_from fmt' -l diff -d 'Print unified formatting differences'
complete -c glippy -n '__fish_seen_subcommand_from fmt' -l reporter -r -a 'text json' -d 'Select reporter'
complete -c glippy -n '__fish_seen_subcommand_from lint check' -l reporter -r -a 'text json github sarif' -d 'Select reporter'
complete -c glippy -n '__fish_seen_subcommand_from fmt lint check lsp' -l config -r -F -d 'Use an explicit configuration'
complete -c glippy -n '__fish_seen_subcommand_from config; and __fish_seen_subcommand_from check show' -l config -r -F -d 'Use an explicit configuration'
complete -c glippy -n '__fish_seen_subcommand_from fmt' -l stdin-filepath -r -F -d 'Supply stdin path context'
complete -c glippy -n '__fish_seen_subcommand_from fmt' -a '--fragment=declaration --fragment=statement --fragment=expression' -d 'Format a source fragment'

complete -c glippy -n '__fish_seen_subcommand_from lint check' -s A -l allow -r -a '` +
			lintTargets +
			`' -d 'Set allow lint level'
complete -c glippy -n '__fish_seen_subcommand_from lint check' -s W -l warn -r -a '` +
			lintTargets +
			`' -d 'Set warn lint level'
complete -c glippy -n '__fish_seen_subcommand_from lint check' -s D -l deny -r -a '` +
			lintTargets +
			`' -d 'Set deny lint level'
complete -c glippy -n '__fish_seen_subcommand_from lint check' -s F -l forbid -r -a '` +
			lintTargets +
			`' -d 'Set forbid lint level'

complete -c glippy -n '__fish_seen_subcommand_from lint' -l fix -d 'Apply safe fixes'
complete -c glippy -n '__fish_seen_subcommand_from lint' -l fix-suggestions -d 'Apply suggestion fixes'
complete -c glippy -n '__fish_seen_subcommand_from lint' -l fix-unsafe -d 'Apply unsafe fixes'
complete -c glippy -n '__fish_seen_subcommand_from lint' -l only -r -a '` +
			strings.Join(ruleIDs, " ") +
			`' -d 'Run only exact rule IDs'
complete -c glippy -n '__fish_seen_subcommand_from lint' -l except -r -a '` +
			strings.Join(ruleIDs, " ") +
			`' -d 'Exclude exact rule IDs'
complete -c glippy -n '__fish_seen_subcommand_from lint' -l generate-baseline -r -F -d 'Write lint baseline'
complete -c glippy -n '__fish_seen_subcommand_from lint check' -l new-from -r -d 'Report findings introduced since a Git ref'

complete -c glippy -n '__fish_seen_subcommand_from lsp' -l fix-suggestions -d 'Offer suggestion code actions'
complete -c glippy -n '__fish_seen_subcommand_from lsp' -l fix-unsafe -d 'Offer unsafe code actions'

complete -c glippy -n '__fish_seen_subcommand_from rules' -l preset -r -a 'correctness suspicious performance complexity style pedantic restriction migration' -d 'Filter by preset'
complete -c glippy -n '__fish_seen_subcommand_from rules' -l fixable -d 'Show only rules with fixes'
complete -c glippy -n '__fish_seen_subcommand_from rules' -l tier -r -a 'lexical syntax types cfg ssa' -d 'Filter by exact analysis tier'
complete -c glippy -n '__fish_seen_subcommand_from explain' -l json -d 'Render versioned JSON'

`,
	)
	for _, ruleID := range ruleIDs {
		fmt.Fprintf(
			&output,
			"complete -c glippy -n '__fish_seen_subcommand_from explain' -a %s\n",
			ruleID,
		)
	}
	output.WriteString(
		`complete -c glippy -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`,
	)
	return output.String()
}
