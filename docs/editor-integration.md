# Editor Integration

The stable boundary and the evidence gate for any future persistent service are
recorded in
[ADR 0013](decisions/0013-editor-integration-architecture.md).

Repositories replacing gofmt, gofumpt, or golines should complete the
[formatter migration workflow](migration-from-go-formatters.md) before enabling
format-on-save. Gox must be the only active formatter for the selected files.

Gox's stable editor boundary is a complete Go file on standard input and the
formatted file on standard output:

```sh
gox fmt --stdin-filepath=/absolute/path/to/source.go
```

`--stdin-filepath` supplies project-root and `gox.toml` discovery context. It
may name a file that does not exist yet, and Gox never writes that path in
standard-input mode. Omit the option when no meaningful project path exists;
the default formatter configuration is then used. Fragment buffers require an
explicit `--fragment=declaration`, `--fragment=statement`, or
`--fragment=expression` argument.

The same path selects the source language according to the
[supported-version contract](supported-go-versions.md). Unsupported or
malformed project language directives fail without formatted output.

On success Gox writes only the complete formatted source to stdout and exits
zero. Parse, configuration, and pre-output cancellation failures write a
diagnostic to stderr, exit nonzero, and produce no formatted source. A stdout
failure can expose a partial stream, so editors must retain the original buffer
on every nonzero exit. Do not combine standard-input formatting with `--write`,
`--check`, `--diff`, or the JSON reporter.

## Conform.nvim

This configuration uses Gox for Go buffers and disables a second LSP formatting
pass:

```lua
require("conform").setup({
  formatters_by_ft = {
    go = { "gox" },
  },
  formatters = {
    gox = {
      command = "gox",
      args = { "fmt", "--stdin-filepath", "$FILENAME" },
      stdin = true,
    },
  },
  format_on_save = {
    timeout_ms = 500,
    lsp_format = "never",
  },
})
```

Conform.nvim sends the buffer on stdin by default and expands `$FILENAME` to
its resolved buffer filename. For an unnamed Go buffer it currently fabricates
an `unnamed_temp.go` path under Neovim's working directory, so configuration
selection follows that directory until the buffer is saved.

The example is based on Conform.nvim
[`619363c`](https://github.com/stevearc/conform.nvim/tree/619363c30309d29ffa631e67c8183f2a72caa373).
Its current help defines custom commands and arguments, stdin formatting,
format-on-save, and the default `lsp_format = "never"` behavior; its runner
defines `$FILENAME` and unnamed-buffer expansion.

## Helix

Add the formatter override to `languages.toml`:

```toml
[[language]]
name = "go"
auto-format = true
formatter = { command = "gox", args = ["fmt", "--stdin-filepath", "%{buffer_name}"] }
```

Helix gives the formatter the current buffer on stdin, replaces the buffer from
stdout after success, and expands `%{buffer_name}` in formatter arguments. Save
an unnamed buffer before relying on project-specific configuration; without a
stable filename, invoke `gox fmt` without `--stdin-filepath` instead.

The example is based on Helix
[`079a789`](https://github.com/helix-editor/helix/tree/079a789e8cb08ead67f19e1971a1b7438b37354b),
whose current language configuration documents `formatter`, `auto-format`, the
stdin/stdout contract, and `%{buffer_name}` expansion.

## Diagnostics And Code Actions

The current binary does not advertise an LSP server, live editor diagnostics,
or editor code actions. Use `gox lint` or `gox check` as an external editor or
task-runner command.

A future Gox lint action must be one source-version-bound transaction: select
one named fix under its safety policy, reject stale or overlapping edits, apply
the edit to the in-memory source, reparse, run the shared formatter, validate
the final source, and return that final replacement. Editors must not apply a
Gox lint edit and then invoke an unrelated formatter over the intermediate
buffer. Suggestion and unsafe actions remain explicit opt-ins.
