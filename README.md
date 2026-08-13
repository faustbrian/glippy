# Gox

Gox is a Go-native formatter, linter, and safe fixer built as one cohesive
developer tool. Its formatter expands compressed but valid Go and makes
deterministic width-aware layout decisions. Its linter keeps correctness-focused
defaults, pays only for the analysis tiers enabled rules require, and routes
all source changes through an explicit conflict-safe transaction.

Gox v0.1.0 is the first supported release. The maintainer accepted the
documented ecosystem-collision and trademark-risk boundary for the Gox name,
binary, repository, and module path. That decision is not legal clearance for
any jurisdiction or trademark class. Untagged commits and locally built
binaries remain unsupported development artifacts.

## Why Gox

Gofmt intentionally leaves many layout choices alone. Gox gives hostile valid
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

- **Go-native frontend:** Gox builds on the standard parser, AST, token, type
  checker, package loader, CFG, SSA, and `go/analysis` ecosystem.
- **Deterministic layout:** a document IR selects one flat or canonical broken
  form with bounded fit work.
- **High-signal linting:** correctness is the default preset; suspicious,
  performance, complexity, style, and migration policy remain explicit.
- **Demand-driven analysis:** syntax-only work does not construct types, CFG,
  or SSA; deeper representations are shared within one package run.
- **Safe source changes:** fixes carry source identity and safety class;
  conflicts, stale ranges, and failed validation cannot silently write source.
- **One product, separate engines:** formatting, diagnostics, fix coordination,
  filesystem replacement, and reporting retain distinct owners.

Oxfmt and Oxlint are the product-experience references for a fast, focused,
predictable formatter and linter. Oxc is the reference for shared compiler-style
infrastructure. Go's specification and supported toolchains remain the syntax
and semantic authority. Gox does not copy ESLint's plugin architecture,
configuration breadth, or default product model.

## Commands

```text
gox fmt [paths...]
gox fmt --write [paths...]
gox fmt --check [paths...]
gox fmt --diff [paths...]
gox lint [paths...]
gox lint --fix [paths...]
gox check [paths...]
gox explain <rule>
gox version
gox completion <bash|zsh|fish>
```

`gox check ./...` is the non-mutating combined CI entry point. Safe fixes are
selected with `lint --fix`; suggestion and unsafe classes require their own
explicit flags. See the [command reference](docs/command-reference.md) for
inputs, reporters, exit categories, and write behavior.

## Installation

Download the archive for a supported target from the
[v0.1.0 GitHub Release](https://github.com/faustbrian/gox/releases/tag/v0.1.0),
verify its checksum, and extract `gox` into a directory on `PATH`. The release
provides macOS and Linux archives for amd64 and arm64, a checksum file, and a
versioned manifest. GitHub-hosted provenance can be verified with:

```sh
gh attestation verify <downloaded-artifact> --repo faustbrian/gox
```

For source installation, pin the released module version:

```sh
go install github.com/faustbrian/gox/cmd/gox@v0.1.0
gox version
```

To evaluate an untagged checkout with Go 1.26, build a disposable development
binary:

```sh
task_root=$(mktemp -d "${TMPDIR:-/tmp}/gox-eval.XXXXXX")
trap 'find "$task_root" -mindepth 1 -delete; rmdir "$task_root"' EXIT HUP INT TERM
go build -o "$task_root/gox" ./cmd/gox
"$task_root/gox" version
"$task_root/gox" fmt --diff /absolute/path/to/project
```

Pin the source revision used by every developer and CI job. Do not run another
formatter after Gox: documented width, import-order, literal, parentheses,
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

Security reports follow [SECURITY.md](SECURITY.md). Gox is licensed under the
[BSD Zero Clause License](LICENSE), and release archives reproduce that license
and all applicable third-party notices. Ordinary development pushes cannot
create a tag or release.
