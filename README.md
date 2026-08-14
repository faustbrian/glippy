# Glippy

Glippy is a Go-native formatter, linter, and safe fixer built as one cohesive
developer tool. Its formatter expands compressed but valid Go and makes
deterministic width-aware layout decisions. Its linter keeps correctness-focused
defaults, pays only for the analysis tiers enabled rules require, and routes
all source changes through an explicit conflict-safe transaction.

Glippy v0.3 is under development and is not tagged or published. The existing
v0.1.0 release remains Gox under `github.com/faustbrian/gox`; its module tags,
binary, archives, and attestations are immutable historical identities. The
maintainer accepted the documented Glippy ecosystem-collision risk for v0.2;
that product decision is not legal clearance. Untagged commits and locally
built binaries remain unsupported development artifacts.

## Why Glippy

Gofmt intentionally leaves many layout choices alone. Glippy gives hostile valid
source one canonical readable form. For example:

```go
if _,err:=client.Discover(nil);!errors.Is(err,ErrContextRequired){t.Fatal(err)}
```

becomes:

```go
if _, err := client.Discover(nil); !errors.Is(err, ErrContextRequired) {
	t.Fatal(err)
}
```

Long expressions and lists use grammar-aware broken forms. Operators remain
where Go semicolon insertion is safe, and multiline lists receive required
trailing commas. Comments, directives, literal spelling, explicit source
structure, and build intent remain part of the validation contract rather than
being reconstructed from `go/ast` alone.

The formatter owns whitespace and canonical layout. Lint rules own behavioral
diagnostics and named source transformations. Semantic refactors do not hide
inside formatting, and layout policy is not duplicated as lint noise.

## Product Direction

- **Go-native frontend:** Glippy builds on the standard parser, AST, token, type
  checker, package loader, CFG, SSA, and `go/analysis` ecosystem.
- **Deterministic layout:** a document IR selects one flat or canonical broken
  form with bounded fit work.
- **High-signal linting:** correctness is the default preset group;
  suspicious, performance, complexity, style, and pedantic groups are
  composable opt-ins, while restriction rules remain individually selected.
- **Demand-driven analysis:** syntax-only work does not construct types, CFG,
  or SSA; deeper representations are shared within one package run.
- **Safe source changes:** fixes carry source identity and safety class;
  conflicts, stale ranges, and failed validation cannot silently write source.
- **One product, separate engines:** formatting, diagnostics, fix coordination,
  filesystem replacement, and reporting retain distinct owners.

Oxfmt and Oxlint are the product-experience references for a fast, focused,
predictable formatter and linter. Oxc is the reference for shared compiler-style
infrastructure. Go's specification and supported toolchains remain the syntax
and semantic authority. Glippy does not copy ESLint's plugin architecture,
configuration breadth, or default product model.

## Commands

```text
glippy fmt [paths...]
glippy fmt --write [paths...]
glippy fmt --check [paths...]
glippy fmt --diff [paths...]
glippy lint [paths...]
glippy lint --fix [paths...]
glippy lint --fix --diff [paths...]
glippy lint --fix-suggestions --diff [paths...]
glippy lint --fix-unsafe --diff [paths...]
glippy lint -Wperformance -Dwarnings [paths...]
glippy lint --only=<rules> [paths...]
glippy lint --except=<rules> [paths...]
glippy lint --new-from=<git-ref> [paths...]
glippy lint --generate-baseline=<path> [paths...]
glippy lint --reporter=github [paths...]
glippy lint --reporter=sarif [paths...]
glippy lint --reporter=short [paths...]
glippy check [paths...]
glippy check --new-from=<git-ref> [paths...]
glippy lsp [--fix-suggestions] [--fix-unsafe]
glippy init [directory]
glippy config check [path]
glippy config show [path]
glippy rules [--preset=<preset>] [--fixable] [--tier=<tier>]
glippy explain <rule>
glippy explain <rule> --json
glippy version
glippy completion <bash|zsh|fish>
```

`glippy check ./...` is the non-mutating combined CI entry point. Safe fixes are
selected with `lint --fix`; suggestion and unsafe classes require their own
explicit flags. Existing findings can be captured without weakening new-code
policy through a [deterministic lint baseline](docs/baselines.md), or hidden
without a baseline during incremental adoption with
`--new-from=origin/main`. See the
[command reference](docs/command-reference.md) for
inputs, reporters, exit categories, and write behavior.

Lint and combined check support GitHub workflow annotations and SARIF 2.1.0 in
addition to human text and Glippy's versioned JSON envelope. Formatter-only
commands remain `text|json` because GitHub and SARIF report diagnostics rather
than formatted source streams.

Default human diagnostics render bounded physical-source frames with precise
primary underlines, related locations, notes, help, and fix safety. Use
`--reporter=short` for the source-free `path:line:column` form in logs. Machine
reporters remain source-free and never include replacement text.

`glippy lsp` provides full-buffer synchronization, live syntax or typed
diagnostics, document formatting, version-bound individual fixes, and a
whole-document "fix all safe" action over standard input/output. It analyzes
the editor's exact buffer through package overlays, never writes source, and
uses the same configuration, cache, suppression, baseline, formatter, and fix
validation contracts as the command-line paths. Suggestion and unsafe actions
remain hidden unless their corresponding LSP flags are supplied.

`glippy init` creates a strict starter `.glippy.toml` without overwriting an
existing path. `glippy config check` validates discovered or explicit policy,
while `glippy config show` explains the resolved language, presets, rule
severities and reasons, analysis tier, file policies, baseline, suppressions,
and cache settings.

Use `glippy rules` to discover the compiled catalog by preset, fix
availability, or exact analysis tier. `lint --only` and `lint --except` apply
exact comma-separated rule IDs after project policy, with exclusions winning
over inclusions. Both `lint` and `check` accept ordered Clippy-style
`-A/--allow`, `-W/--warn`, `-D/--deny`, and `-F/--forbid` directives targeting
exact rule IDs, selectable groups, or the currently enabled `warnings` set.
Later directives override earlier ones, except that a forbidden rule cannot be
lowered. `explain --json` exposes the same canonical metadata through a
schema-versioned machine contract.

The v0.2 pedantic catalog includes bounded Go-native simplifications for blank
identifiers, direct closures, nil-and-length checks, time helpers, buffer
conversions, constant formatting, and case-normalized comparisons. Five narrow
transformations are available only through `lint --fix-suggestions` and still
pass conflict, formatting, parse, typed-validation, and atomic-write gates.

The opt-in complexity catalog measures structural nesting, logical function
lines, parameter count, and result count with bounded per-rule thresholds.
Test files are excluded by default, and these advisory API or decomposition
findings never enter the correctness preset or offer automatic fixes.

The v0.3 suspicious catalog also checks stream iteration through the shared
control-flow tier. `unchecked-rows-error` and `unchecked-scanner-error` require
every normally returning path after a direct `database/sql.Rows.Next` or
`bufio.Scanner.Scan` loop to observe the matching terminal error. Discarded
results and checks against a reassigned iterator do not satisfy the contract.

The restriction catalog includes `blank-error-discard`, Glippy's Go analogue
to Clippy's `let_underscore_must_use`. Projects can enable it by exact rule ID
to require every explicit blank-identifier error discard to be handled or
suppressed with a reason. Test files remain excluded unless the typed
`include-tests` option is enabled, and the restriction group cannot be enabled
wholesale.

## Installation

No Glippy release is published yet. The historical Gox v0.1.0 release remains
available from its
[GitHub Release](https://github.com/faustbrian/gox/releases/tag/v0.1.0), but it
does not provide the `glippy` command or v0.2 catalog. Its provenance can be
verified with:

```sh
gh attestation verify <downloaded-artifact> --repo faustbrian/gox
```

The corresponding historical source installation is:

```sh
go install github.com/faustbrian/gox/cmd/gox@v0.1.0
gox version
```

To evaluate an untagged checkout with Go 1.26, build a disposable development
binary:

```sh
task_root=$(mktemp -d "${TMPDIR:-/tmp}/glippy-eval.XXXXXX")
trap 'find "$task_root" -mindepth 1 -delete; rmdir "$task_root"' EXIT HUP INT TERM
go build -o "$task_root/glippy" ./cmd/glippy
"$task_root/glippy" version
"$task_root/glippy" fmt --diff /absolute/path/to/project
```

Pin the source revision used by every developer and CI job. Do not run another
formatter after Glippy: documented width, import-order, literal, parentheses,
alignment, and empty-statement choices are not universally gofmt fixed points.
Use the [migration guide](docs/migration-from-go-formatters.md) before changing
a repository's formatter authority.

## Current Target Boundary

- Source language: Go 1.25 and Go 1.26.
- Runtime targets: macOS and Linux on amd64 and arm64.
- Unsupported runtime targets: Windows and all other operating systems.
- Write and fix claims: recorded local-filesystem combinations only; network,
  distributed, and userspace filesystems and forced-power-loss durability are
  outside the guarantee.
- Ordinary operation: no telemetry or source transmission; formatting and
  syntax rules do not invoke the Go command, while typed analysis uses the
  documented `go/packages` boundary with network access disabled by default.

The [support policy](docs/support-policy.md),
[supported Go versions](docs/supported-go-versions.md), and
[CLI filesystem contract](docs/spec/cli.md#filesystem-boundary) define the
precise scope.

## Documentation

- [Formatter rules](docs/formatter-rules.md)
- [Configuration contract](docs/spec/configuration.md)
- [Lint rule catalog](docs/lint-rules.md)
- [Go vet compatibility](docs/go-vet-compatibility.md)
- [Rule roadmap](docs/rule-roadmap.md)
- [Suppression reference](docs/suppressions.md)
- [Lint engine and suppressions](docs/spec/lint-engine.md)
- [Fix safety model](docs/spec/fix-safety.md)
- [Machine output reference](docs/machine-output.md)
- [Machine reporting decision](docs/decisions/0011-machine-reporting-schema.md)
- [Editor integration](docs/editor-integration.md)
- [CI and pre-commit setup](docs/ci-and-precommit.md)
- [Shell completion](docs/shell-completion.md)
- [Compatibility and change policy](docs/compatibility-policy.md)
- [Release artifacts and provenance](docs/release-artifacts.md)
- [Contributor architecture and rule authoring](docs/contributing.md)
- [Architecture decisions](docs/decisions/README.md)
- [Performance methodology and results](benchmarks/README.md)

Security reports follow [SECURITY.md](SECURITY.md). Glippy is licensed under the
[BSD Zero Clause License](LICENSE), and release archives reproduce that license
and all applicable third-party notices. Ordinary development pushes cannot
create a tag or release.
