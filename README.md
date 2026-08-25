# Glippy

Glippy is a Go-native formatter, linter, and safe fixer built as one cohesive
developer tool. Its formatter expands compressed but valid Go and makes
deterministic width-aware layout decisions. Its linter keeps correctness-focused
defaults, pays only for the analysis tiers enabled rules require, and routes
all source changes through an explicit conflict-safe transaction.

Glippy v0.8 is under development and is not tagged or published. The current
catalog contains 127 rules, including 19 rules with safe or suggestion fixes.
The current candidate is an unreleased engineering snapshot. Aggregate
process-tree memory and process containment remain unproven until a
non-disruptive evidence policy exists.
The existing
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
  suspicious, performance, complexity, style, pedantic, and nursery groups are
  composable opt-ins. Nursery rules are explicitly unstable validation
  candidates, while restriction rules remain individually selected.
- **Demand-driven analysis:** syntax-only work does not construct types, CFG,
  or SSA; deeper representations are shared within one package run. CFG and
  SSA consumers share demand-driven no-return and nil/error return summaries,
  while lifecycle rules consume versioned parameter and receiver effects for
  root modules plus reachable workspace and local replacement modules, without
  retaining dependency syntax as lint targets. Lock
  rules share one bounded fixed-point state transition over each function CFG,
  distinguish read and write modes, and consume configured blocking-call
  contracts without repeating propagation per rule. Closed-resource analysis
  uses the same bounded worklist contract and consumes proven close effects
  without treating ownership borrowing as proof of unchanged resource state.
  Exact project contracts can also require individual call results to be used;
  ignored contracted results report without duplicating the broader
  `discarded-error` diagnostic. Exact returned-alias contracts preserve
  outstanding lifecycle obligations when ownership returns to the same tracked
  value.
- **Safe source changes:** fixes carry source identity and safety class;
  conflicts, stale ranges, and failed validation cannot silently write source.
  Accepted fixes may declare exact required imports, while the coordinator
  owns collision checks, fix-caused cleanup, and deterministic add/remove
  provenance without becoming a general import organizer.
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
glippy lint --stats[=text|json] [paths...]
glippy check [paths...]
glippy check --new-from=<git-ref> [paths...]
glippy check --stats[=text|json] [paths...]
glippy lsp [--fix-suggestions] [--fix-unsafe]
glippy init [--profile=<profile>] [directory]
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

`lint` and combined `check` also provide opt-in execution statistics through
`--stats` or `--stats=json`. Diagnostics keep their selected standard-output
format, while statistics use standard error so machine consumers can capture
the two versioned documents independently. Statistics explain tier and
dependency loading, per-rule findings and cost, suppression and baseline
counts, and persistent-cache outcomes. They are intentionally absent from
ordinary runs.

`glippy lsp` provides full-buffer synchronization, live syntax or typed
diagnostics, document formatting, version-bound individual fixes, and a
whole-document "fix all safe" action over standard input/output. It analyzes
the editor's exact buffer through package overlays, never writes source, and
uses the same configuration, cache, suppression, baseline, formatter, and fix
validation contracts as the command-line paths. Suggestion and unsafe actions
remain hidden unless their corresponding LSP flags are supplied.

Document analysis runs outside the protocol loop. Rapid replacements coalesce
behind a short bounded debounce, cancel an active older snapshot, and publish
only results whose complete document versions are still current. Code actions
wait for the matching analysis; a later edit rejects the queued request as
content-modified instead of applying stale state.

Typed workspace analysis retains at most eight validated package results per
session within a deterministic 128 MiB accounted-memory budget. Format-capable
source is charged at sixteen times its exact bytes and compact dependency source
at twice its bytes; this is a stable eviction weight, not an operating-system
RSS measurement. An edit reloads its package and open reverse dependants while
unaffected packages reuse their exact prior result against the current complete
overlay. Captured disk sources, Go-file directory membership, module/workspace
control files, baselines, configuration identity, and document digests all
invalidate reuse. A separate eight-entry, 128 MiB typed graph session retains
compact dependency types without dependency syntax or type-value maps. For a
clean package or coherent base, internal-test, and external-test package family
whose imports, source membership, build selection, and project controls remain
compatible, Glippy reparses and re-typechecks every retained variant without
invoking the primary package loader, then rebuilds required CFG and SSA state.
External tests bind to the freshly checked internal variant rather than stale
retained types. This applies to nested packages as well as a package at the
project root. Changed active source in the main module, an active workspace
module, or a local filesystem replacement reparses and re-typechecks its
retained reverse dependency closure and rebuilds selected-module effect facts
before root diagnostics. Exact overlays may replace already selected local
dependency source. Newly direct root imports already present in the retained
graph are admitted without a full load. Imports added by a changed local
dependency reuse the retained package identity when available or use the same
bounded exact types loader. Newly loaded mutable local layers are rechecked
against compatible retained transitive types before reverse dependency and
effect reconstruction. Uncertain or malformed test package families, unresolved
dependency imports, cgo-generated sources, changed source or build selection,
module or workspace control changes, parse or type errors, immutable dependency
edits, and external file notifications fall back to a complete load. The
ordinary aggregate retained weight is therefore 256 MiB; it remains a
deterministic eviction model rather than an RSS promise.

`glippy init` creates a starter `.glippy.toml` without overwriting an existing
path. The `default`, `recommended`, `strict`, and `pedantic` profiles provide
increasing curated policy without requiring projects to assemble groups and
exact rules manually; `glippy init --profile=strict` selects one explicitly.
`glippy config check` validates discovered or explicit policy, while `glippy
config show` explains the resolved profile, presets, rule severities and
reasons, analysis tier, file policies, baseline, suppressions, cache settings,
and configured project semantic contracts. Static versioned
contract files can declare exact project or dependency function effects such as
no-return, required results, ownership transfer, conditional nilness, blocking,
and returned aliases without loading executable plugins. Ordered
`[[lint.overrides]]` entries can adjust exact rule
severities for project-relative glob paths such as tests, fixtures, generated
adapters, or migration trees without introducing nested configuration files.

Use `glippy rules` to discover the compiled catalog by preset, fix
availability, or exact analysis tier. `lint --only` and `lint --except` apply
exact comma-separated rule IDs after project policy, with exclusions winning
over inclusions. Both `lint` and `check` accept ordered Clippy-style
`-A/--allow`, `-W/--warn`, `-D/--deny`, and `-F/--forbid` directives targeting
exact rule IDs, selectable groups, or the currently enabled `warnings` set.
Later directives override earlier ones, except that a forbidden rule cannot be
lowered. `explain --json` exposes the same canonical metadata through a
schema-versioned machine contract.

The `nursery` group is never included by `default`, `recommended`, `strict`,
or `pedantic`. It contains opt-in rules still undergoing broad corpus
validation and may change membership or behavior before promotion. Select it
explicitly with `lint.presets = ["nursery"]`, `-Wnursery`, or
`glippy rules --preset=nursery`.

```toml
[[lint.overrides]]
paths = ["**/*_test.go", "testdata/**"]

[lint.overrides.rules]
discarded-error = "off"
blank-error-discard = "warn"
```

The pedantic catalog includes bounded Go-native simplifications for blank
identifiers, direct closures, nil-and-length checks, time helpers, buffer
conversions, constant formatting, and case-normalized comparisons. Five narrow
transformations plus the constant-format operand replacement are available only
through `lint --fix-suggestions` and still
pass conflict, formatting, parse, typed-validation, and atomic-write gates.
It also reports undocumented empty conditional branches, direct integer
min/max update patterns, and redundant explicit variable types. Only the
redundant-type rule offers a safe fix, and it refuses edits that would remove a
comment.

The opt-in complexity catalog measures structural nesting, logical function
lines, parameter count, and result count with bounded per-rule thresholds.
Test files are excluded by default, and these advisory API or decomposition
findings never enter the correctness preset or offer automatic fixes.

The suspicious catalog also checks stream iteration through the shared
control-flow tier. `unchecked-rows-error` and `unchecked-scanner-error` require
every normally returning path after a direct `database/sql.Rows.Next` or
`bufio.Scanner.Scan` loop to observe the matching terminal error. Discarded
results and checks against a reassigned iterator do not satisfy the contract.

The opt-in error-flow catalog includes `overwritten-error`,
`typed-nil-error-return`, `shadowed-error`, and `nil-error-wrap`. The shadow
rule deliberately does not warn about ordinary nested `if err := ...`
handling: it reports only stale outer errors carried out of loops and deferred
assignments that update a shadowing error instead of the named result.
`nil-error-wrap` reports `%w` operands proven nil directly or through an exact
selected-module sibling result relationship whose dominated state makes a
non-nil error impossible.

The default output-integrity catalog distinguishes three writer failures:
discarded `Flush` or `Close` errors, CSV flushing without observing
`Writer.Error`, and successful tar, gzip, multipart, ascii85, base32, or base64
output paths that use a directly acquired writer without finalizing or
transferring it.

The default correctness catalog checks direct `database/sql` transaction
lifecycles. After a conventional successful `DB.Begin`, `DB.BeginTx`, or
`Conn.BeginTx` guard, `sql-transaction-not-completed` requires every normally
returning path to commit, roll back, or transfer ownership of the transaction.
Conditional cleanup and reassignment cannot silently discharge another open
path.

The opt-in `resource-not-closed` rule applies the same bounded obligation model
to local values with `Close() error`. Cleanup or ownership transfer must cover
every normally returning path, so a close on only one branch and reassignment
before cleanup both report. Exact receiver summaries also recognize direct
terminal methods such as a locally proven `Shutdown` that closes the same
receiver; dynamic, conditional, and promoted receiver behavior remains
conservative. The same receiver facts establish closed state for
`resource-used-after-close`.

The opt-in `http-response-body-not-closed` rule covers the standard-library
gap where `*http.Response` is not itself a closer. After a successful direct
package or `Client` request guard, every normal return must close or transfer
the body. Passing the body to a reader does not count as transfer, so early
status and read-error returns before a later close still report.

The opt-in `http-response-body-used-after-close` rule follows the same direct
request boundary and reports reads, selected `io` consumers, and repeated
closes reached only after every path has closed the body. Conditional closure,
aliases, transfer, asynchronous or deferred execution, and unknown helpers
remain conservative. The rule is suspicious rather than correctness because a
custom `RoundTripper` can supply an `io.ReadCloser` with implementation-specific
post-close behavior.

The restriction catalog includes `blank-error-discard`, `direct-panic`,
`process-exit`, `context-background`, `context-todo`, and
`exported-api-documentation`. Projects can enable these policies only by exact
rule ID to require explicit error handling, keep process termination at an
owned boundary, audit root and placeholder contexts, or enforce documented
exported contracts. Each rule excludes test files unless its typed
`include-tests` option is enabled, and the restriction group cannot be enabled
wholesale.

## Installation

No Glippy release is published yet. The historical Gox v0.1.0 release remains
available from its
[GitHub Release](https://github.com/faustbrian/gox/releases/tag/v0.1.0), but it
does not provide the `glippy` command or current catalog. Its provenance can be
verified with:

```sh
gh attestation verify <downloaded-artifact> --repo faustbrian/gox
```

The corresponding historical source installation is:

```sh
go install github.com/faustbrian/gox/cmd/gox@v0.1.0
gox version
```

To evaluate an untagged checkout with Go 1.27, build a disposable development
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

- Source language: Go 1.25 through Go 1.27.
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
- [Project semantic contracts](docs/spec/project-contracts.md)
- [Lint rule catalog](docs/lint-rules.md)
- [Go vet compatibility](docs/go-vet-compatibility.md)
- [Rule roadmap](docs/rule-roadmap.md)
- [v0.4 exit audit](docs/research/v0.4-exit-audit-2026-08-15.md)
- [v0.4 ecosystem dogfood](docs/research/v0.4-diverse-ecosystem-dogfood-2026-08-15.md)
- [v0.4 fixability sweep](docs/research/v0.4-catalog-fixability-sweep-2026-08-15.md)
- [v0.5 typed-analysis memory reduction](docs/research/v0.5-memory-reduction-2026-08-16.md)
- [v0.5 typed-analysis memory attribution](docs/research/v0.5-typed-memory-attribution-2026-08-19.md)
- [v0.5 exact printf fact execution](docs/research/v0.5-printf-fact-execution-2026-08-19.md)
- [v0.5 interface encoder finalization](docs/research/unchecked-writer-interface-encoder-expansion-2026-08-19.md)
- [v0.5 incremental workspace analysis](docs/research/v0.5-incremental-workspace-analysis-2026-08-16.md)
- [v0.5 memory-aware SSA package waves](docs/research/v0.5-memory-aware-ssa-waves-2026-08-19.md)
- [v0.5 project semantic contracts](docs/research/v0.5-project-semantic-contracts-2026-08-16.md)
- [v0.5 cancellation returned-alias obligations](docs/research/v0.5-returned-alias-cancellation-obligations-2026-08-20.md)
- [v0.5 receiver terminal effects](docs/research/v0.5-receiver-terminal-effects-2026-08-20.md)
- [v0.5 nil-error return facts](docs/research/v0.5-nil-error-return-facts-2026-08-20.md)
- [v0.5 unconditional result-state facts](docs/research/v0.5-unconditional-result-state-facts-2026-08-20.md)
- [v0.5 unconditional nil-error wrapping](docs/research/v0.5-unconditional-nil-error-wrap-2026-08-20.md)
- [v0.5 delegated result-state facts](docs/research/v0.5-delegated-result-state-facts-2026-08-20.md)
- [v0.5 delegated return relationships](docs/research/v0.5-delegated-return-relationships-2026-08-20.md)
- [v0.5 delegated cleanup-managed results](docs/research/v0.5-delegated-cleanup-managed-results-2026-08-21.md)
- [v0.5 authoritative testing cleanup receivers](docs/research/v0.5-authoritative-testing-cleanup-receivers-2026-08-21.md)
- [v0.5 direction exit audit](docs/research/v0.5-direction-exit-audit-2026-08-21.md)
- [v0.5 current-revision arm64 budgets](docs/research/v0.5-current-revision-arm64-budget-evidence-2026-08-21.md)
- [v0.5 native typed-budget calibration](docs/research/v0.5-native-typed-budget-calibration-2026-08-21.md)
- [v0.5 pre-release readiness review](docs/research/v0.5-pre-release-readiness-review-2026-08-21.md)
- [v0.5 required writer finalization](docs/research/writer-not-finalized-rule-admission-2026-08-20.md)
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
