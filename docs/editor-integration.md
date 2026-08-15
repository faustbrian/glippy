# Editor Integration

The stable boundary and the evidence gate for any future persistent service are
recorded in
[ADR 0013](decisions/0013-editor-integration-architecture.md).

Repositories replacing gofmt, gofumpt, or golines should complete the
[formatter migration workflow](migration-from-go-formatters.md) before enabling
format-on-save. Glippy must be the only active formatter for the selected files.

Glippy's stable editor boundary is a complete Go file on standard input and the
formatted file on standard output:

```sh
glippy fmt --stdin-filepath=/absolute/path/to/source.go
```

`--stdin-filepath` supplies project-root and `glippy.toml` discovery context. It
may name a file that does not exist yet, and Glippy never writes that path in
standard-input mode. Omit the option when no meaningful project path exists;
the default formatter configuration is then used. Fragment buffers require an
explicit `--fragment=declaration`, `--fragment=statement`, or
`--fragment=expression` argument.

The same path selects the source language according to the
[supported-version contract](supported-go-versions.md). Unsupported or
malformed project language directives fail without formatted output.

On success Glippy writes only the complete formatted source to stdout and exits
zero. Parse, configuration, and pre-output cancellation failures write a
diagnostic to stderr, exit nonzero, and produce no formatted source. A stdout
failure can expose a partial stream, so editors must retain the original buffer
on every nonzero exit. Do not combine standard-input formatting with `--write`,
`--check`, `--diff`, or the JSON reporter.

## Conform.nvim

This configuration uses Glippy for Go buffers and disables a second LSP formatting
pass:

```lua
require("conform").setup({
  formatters_by_ft = {
    go = { "glippy" },
  },
  formatters = {
    glippy = {
      command = "glippy",
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
formatter = { command = "glippy", args = ["fmt", "--stdin-filepath", "%{buffer_name}"] }
```

Helix gives the formatter the current buffer on stdin, replaces the buffer from
stdout after success, and expands `%{buffer_name}` in formatter arguments. Save
an unnamed buffer before relying on project-specific configuration; without a
stable filename, invoke `glippy fmt` without `--stdin-filepath` instead.

The example is based on Helix
[`079a789`](https://github.com/helix-editor/helix/tree/079a789e8cb08ead67f19e1971a1b7438b37354b),
whose current language configuration documents `formatter`, `auto-format`, the
stdin/stdout contract, and `%{buffer_name}` expansion.

## Diagnostics And Code Actions

Start the stdio language server with:

```sh
glippy lsp
```

Configure it beside `gopls`, leaving Go language intelligence to `gopls` and
assigning Glippy diagnostics, code actions, and formatting to Glippy. Do not
also run the one-shot formatter for the same save event. The client must send
absolute local `file:` URIs and full-document synchronization updates with
strictly increasing versions.

The server publishes syntax diagnostics without package loading. When enabled
rules require types, CFG, or SSA, it loads the containing package with the
current buffer as an overlay and uses the configured persistent cache. Invalid
source or project state is a document diagnostic rather than a fallback to a
different policy. Closing the buffer clears its diagnostics.
Rule diagnostics carry a documentation link to the canonical generated rule
catalog; `glippy explain <rule>` renders the same metadata locally.
When a declared rule fix cannot be offered safely for the exact buffer, the
published diagnostic carries `data.withheld_fixes` entries with stable `name`,
`reason`, and `message` fields. The initial `comments` reason means the rewrite
would discard comments. This is diagnostic provenance, not a code action or an
authorization to edit the document.

Safe quick fixes and a `source.fixAll.glippy` action are available by default.
Launch with one or both explicit flags to broaden the offered action classes:

```sh
glippy lsp --fix-suggestions
glippy lsp --fix-suggestions --fix-unsafe
```

Each action is bound to the exact open document version and returns one
whole-document replacement. Glippy rejects stale, overlapping, ambiguous, or
invalid fixes; reparses, formats, and reanalyzes the complete in-memory result;
and never writes the source file. Suggestion and unsafe authorization are
independent, and the fix-all action always includes safe fixes only. Canceled
requests return the protocol request-canceled error without publishing their
result.

The server rediscovers configuration for each analysis or formatting request,
so later requests observe a changed configuration without weakening an
in-flight request's snapshot. An explicit policy can be pinned for the complete
session with `--config=/absolute/path/to/.glippy.toml`.
