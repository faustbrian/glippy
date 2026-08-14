# Shell Completion

Glippy generates deterministic completion scripts for Bash, Zsh, and Fish. Each
script contains the command-specific flags and values supported by that binary,
the `config check` and `config show` subcommands, configuration paths, rule
discovery filters, lint selection flags, and the exact rule IDs available to
`glippy explain`, `--only`, and `--except`. Regenerate the script after
upgrading Glippy so newly admitted rules and CLI options become available.
Formatter reporter completion offers `text` and `json`; lint and combined-check
reporter completion additionally offers `github` and `sarif`.

Generation writes only to standard output. It does not inspect a project,
configuration, source file, package graph, or network resource.

For `lint` and `check`, completion includes short and long `allow`, `warn`,
`deny`, and `forbid` flags plus exact rules, selectable groups, and the special
`warnings` target. Restriction and migration are intentionally absent as group
targets because their policy requires exact rule IDs or an explicit migration
target.

## Bash

Write the script to a directory loaded by `bash-completion`:

```sh
mkdir -p ~/.local/share/bash-completion/completions
glippy completion bash > ~/.local/share/bash-completion/completions/glippy
```

Start a new shell after installation. A distribution-specific Bash completion
directory may be used instead when it is already configured.

## Zsh

Install the script in a directory on `fpath` before running `compinit`:

```zsh
mkdir -p ~/.zfunc
glippy completion zsh > ~/.zfunc/_glippy
fpath=(~/.zfunc $fpath)
autoload -Uz compinit && compinit
```

Persist the `fpath` and `compinit` lines in `.zshrc` when they are not already
present.

## Fish

Install the script in Fish's user completion directory:

```fish
mkdir -p ~/.config/fish/completions
glippy completion fish > ~/.config/fish/completions/glippy.fish
```

Fish loads the generated script automatically in new shell sessions.
