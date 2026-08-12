# Shell Completion

Gox generates deterministic completion scripts for Bash, Zsh, and Fish. Each
script contains the command-specific flags and values supported by that binary,
plus the exact rule IDs available to `gox explain`. Regenerate the script after
upgrading Gox so newly admitted rules and CLI options become available.

Generation writes only to standard output. It does not inspect a project,
configuration, source file, package graph, or network resource.

## Bash

Write the script to a directory loaded by `bash-completion`:

```sh
mkdir -p ~/.local/share/bash-completion/completions
gox completion bash > ~/.local/share/bash-completion/completions/gox
```

Start a new shell after installation. A distribution-specific Bash completion
directory may be used instead when it is already configured.

## Zsh

Install the script in a directory on `fpath` before running `compinit`:

```zsh
mkdir -p ~/.zfunc
gox completion zsh > ~/.zfunc/_gox
fpath=(~/.zfunc $fpath)
autoload -Uz compinit && compinit
```

Persist the `fpath` and `compinit` lines in `.zshrc` when they are not already
present.

## Fish

Install the script in Fish's user completion directory:

```fish
mkdir -p ~/.config/fish/completions
gox completion fish > ~/.config/fish/completions/gox.fish
```

Fish loads the generated script automatically in new shell sessions.
