# Goal: Build Gox, A Modern Go Formatter And Linter

## Mission

Build `gox` as a modern, fast, cohesive Go developer tool that combines a
width-aware source formatter with a high-signal linter and safe fixer. The
formatter experience SHOULD be comparable in ambition, predictability, and
ergonomics to Oxfmt. The linter experience SHOULD be comparable in focus,
speed, diagnostics, and fix safety to Oxlint.

The project exists because `gofmt` deliberately makes fewer layout decisions
than modern formatters. Valid but hostile-looking Go such as compressed blocks,
semicolon-separated statements, extremely long calls, dense function
literals, and unbroken boolean expressions should be transformed into stable,
readable source without requiring manual cleanup.

The product MUST be developed in Go. It MUST build on the Go toolchain's
parser, syntax tree, token, package-loading, type-checking, analysis, and SSA
capabilities rather than creating a competing Go language frontend without
evidence that the standard frontend cannot satisfy a concrete requirement.

Architecture and correctness come before rule count. Gox MUST first establish
a lossless-enough shared frontend, deterministic formatter, safe edit model,
and scalable analysis engine. A broad rule catalog MAY grow after those
foundations are proven.

`gox` is a working name. Before the first public release, the project MUST
complete a naming, module-path, binary-name, trademark, and ecosystem-collision
audit and either retain the name with evidence or choose a replacement.

## Normative Language

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as
shown here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Goal State

At 100%, Gox MUST provide one installable, versioned Go binary with these
cohesive product surfaces:

```text
gox fmt [paths...]
gox fmt --write [paths...]
gox fmt --check [paths...]
gox lint [paths...]
gox lint --fix [paths...]
gox check [paths...]
gox explain <rule>
gox version
```

The completed product MUST:

- format complete files and standard-input fragments deterministically;
- make width-aware layout decisions for Go expressions and constructs;
- expand compressed statements and blocks into readable source;
- preserve program meaning, comments, directives, literals, and build intent;
- be idempotent across repeated runs;
- report high-signal syntax, type, control-flow, and SSA diagnostics;
- provide explicit safe, suggestion-only, and unsafe fix categories;
- prevent overlapping or stale fixes from silently corrupting source;
- load modules, workspaces, packages, tests, build tags, and generated files
  according to documented policies;
- scale across large repositories through bounded concurrency, tiered loading,
  and deterministic caching;
- support editors, continuous integration, pre-commit workflows, and machine
  diagnostic formats;
- expose stable configuration and suppression contracts;
- document every formatting rule, lint rule, default, exit code, and safety
  boundary; and
- ship with corpus, fuzz, differential, integration, performance, and release
  evidence proportional to its claims.

## Product References

### Primary Product References

Oxfmt and Oxlint are the primary product references for Gox:

- [Oxfmt](https://oxc.rs/docs/guide/usage/formatter.html) is the reference for
  a dedicated, deterministic, high-throughput formatter with a cohesive CLI,
  editor workflow, check mode, and modern layout expectations.
- [Oxlint](https://oxc.rs/docs/guide/usage/linter.html) is the reference for a
  fast linter with correctness-focused defaults, incremental adoption,
  actionable diagnostics, safe fixes, and architecture that can grow without
  making the default experience noisy.
- [Oxc](https://github.com/oxc-project/oxc) is the reference for sharing a
  compiler-style frontend and infrastructure across developer tools while
  keeping formatter and linter responsibilities distinct.

Gox MUST study the current authoritative Oxfmt, Oxlint, and Oxc source and
documentation at the time each corresponding subsystem is designed. Historical
descriptions and assumptions MUST NOT substitute for current source evidence.

### Go Authorities

The Go language specification and the active supported Go toolchains are the
authorities for syntax and semantics. The project SHOULD reuse:

- `go/parser` and `go/ast` for syntax;
- `go/token` for source positions;
- `go/scanner` and original source bytes for lexical reconstruction;
- `golang.org/x/tools/go/packages` for package-aware loading;
- `go/types` for type information;
- `golang.org/x/tools/go/analysis` for analyzer interoperability;
- `golang.org/x/tools/go/ast/inspector` for filtered traversal;
- `golang.org/x/tools/go/cfg` for control flow where required; and
- `golang.org/x/tools/go/ssa` for analyses whose contracts require SSA.

`gofmt`, `go/format`, `go/printer`, and `gofumpt` are compatibility inputs and
differential-test references. They are not the product ceiling for Gox layout.

### Explicitly Rejected Reference Direction

Gox MUST NOT model its architecture, configuration, execution engine, or
default product experience after ESLint. ESLint's breadth MAY inform a later
coverage inventory, but its legacy architecture, plugin overhead, configuration
complexity, and accumulated compatibility surface are not design targets.

Rule breadth MUST NOT be treated as evidence that the Gox foundation is ready.
The project MUST prefer a small set of fast, credible, well-documented rules
over a large catalog built on unstable traversal, fixing, or configuration
contracts.

## Product Principles

### One Product, Separate Engines

Gox MUST ship as one binary and one configuration system, but formatting and
linting MUST remain separate internal engines:

- the formatter owns whitespace and canonical layout;
- lint rules own diagnostics about code, behavior, safety, complexity, and
  maintainability;
- lint fixes own explicit source transformations;
- the fix coordinator owns edit conflicts and transactionality; and
- the formatter normalizes files after successful lint fixes.

A lint rule MUST NOT exist only to enforce layout that the formatter can make
canonical. The formatter MUST NOT silently perform semantic refactors merely
because a lint rule could recommend them.

### Correctness Before Speed

Gox SHOULD be fast enough to run continuously, but optimization MUST follow a
measured, correct implementation. Performance work MUST preserve determinism,
source fidelity, diagnostics, and fix safety.

### Opinionated, Not Arbitrarily Configurable

Formatting MUST have one recognizable output dialect. Configuration SHOULD be
limited to choices that materially affect adoption, such as line width and
width measurement. Individual whitespace rules, brace placement, or dozens of
layout switches MUST NOT fragment the output into incompatible styles.

Lint configuration MAY be broader because projects legitimately differ in
policy, but default presets MUST remain coherent and documented.

### High-Signal Defaults

The default lint preset MUST prioritize code that is incorrect, unsafe,
misleading, ineffective, or highly likely to be defective. Opinionated style,
complexity, performance, and migration rules SHOULD be separate opt-in groups.

### Safe By Default

Gox MUST NOT write a file unless the complete resulting file parses and all
applicable formatter or fix invariants pass. Gox MUST NOT guess how to combine
overlapping fixes. Gox MUST NOT hide partial writes or silently skip malformed
configuration.

### Deterministic Everywhere

The same binary, configuration, source tree, Go language version, build
selection, and environment-independent inputs MUST produce byte-identical
formatted output, ordered diagnostics, and exit status.

## Non-Goals And Hard Prohibitions

The initial product MUST NOT:

- replace the Go parser, type checker, compiler, linker, or module resolver;
- become a build system, test runner, package manager, or code generator;
- copy every lint rule from another ecosystem before the rule engine is proven;
- preserve every user-authored whitespace choice;
- perform arbitrary refactoring under `fmt`;
- claim semantic equivalence from successful parsing alone;
- treat AST equality as sufficient evidence for comment or directive fidelity;
- rewrite syntactically invalid files in place;
- load types, CFG, or SSA for rules that do not require them;
- use unrestricted dynamic Go plugins as the first extension mechanism;
- depend on network access during ordinary formatting or linting;
- send source, diagnostics, or telemetry outside the machine by default;
- hide nondeterminism behind parallel execution;
- make generated files writable by default;
- silently format vendor or module-cache content;
- change language semantics to satisfy print width; or
- promise full gofmt byte compatibility without corpus evidence.

## Observable User Journeys

### Format Hostile But Valid Go

Given:

```go
if _,err:=client.Discover(nil);!errors.Is(err,ErrContextRequired){t.Fatal(err)}
```

Gox MUST produce a stable expanded form equivalent to:

```go
if _, err := client.Discover(nil); !errors.Is(err, ErrContextRequired) {
	t.Fatal(err)
}
```

Given semicolon-separated statements:

```go
ctx,cancel:=context.WithCancel(t.Context());cancel();result:=work(ctx)
```

Gox MUST render separate statements on separate lines. It MAY introduce a
blank line only when a documented structural grouping rule requires one.

### Wrap A Boolean Chain

When a condition fits, it SHOULD remain flat:

```go
if foo && bar && baz {
```

When it does not fit, Gox MUST choose a valid deterministic broken layout:

```go
if foo &&
	bar &&
	baz &&
	somethingReallyLong {
```

Binary-expression breaks MUST respect Go semicolon insertion. In particular,
operators that keep the statement incomplete MUST remain on the preceding
line unless the Go grammar and semicolon rules prove another layout valid.

### Wrap Calls And Literals

When a call group does not fit, Gox SHOULD render one logical argument per
line and MUST emit any trailing comma required by Go syntax:

```go
result, err := client.executeContent(
	ctx,
	OperationInfo,
	http.MethodGet,
	"/",
	nil,
	"application/json",
	200,
)
```

Composite literals, signatures, parameter lists, result lists, index lists,
type arguments, and function literals MUST receive corresponding documented
grouping and breaking behavior.

### Check Without Mutation

`gox check ./...` MUST provide one non-mutating continuous-integration entry
point that:

- identifies files whose formatted output differs;
- reports enabled lint diagnostics;
- uses deterministic ordering;
- distinguishes tool failure from actionable findings; and
- never modifies tracked or untracked files.

### Fix Safely

`gox lint --fix ./...` MUST:

- apply only fixes classified as safe by default;
- refuse conflicting edits;
- reparse each resulting file;
- format changed files through the same formatter engine;
- validate the final source before replacement;
- use atomic replacement where supported; and
- report files or diagnostics left unchanged and why.

## Shared Frontend Architecture

### Source Unit

Define one immutable per-file source representation containing at least:

```go
type SourceFile struct {
	Path       string
	Bytes      []byte
	FileSet    *token.FileSet
	AST        *ast.File
	Tokens     []Token
	Trivia     TriviaIndex
	Directives []Directive
	Metadata   FileMetadata
}
```

The concrete design MAY differ, but it MUST retain enough information to map
between byte offsets, token positions, AST nodes, comments, and edits without
guessing.

The source layer MUST explicitly model:

- UTF-8 and byte offsets;
- byte-order marks if accepted;
- newline style policy;
- tabs and visual width;
- comments and comment groups;
- whitespace between lexical tokens;
- explicit and automatically inserted semicolons;
- parentheses significant to source intent or grammar;
- raw and interpreted literal spelling;
- build constraints;
- compiler and tool directives;
- generated-file markers;
- cgo preambles; and
- incomplete or erroneous parse state.

### AST And Trivia

The formatter MUST NOT assume `go/ast` is a complete concrete syntax tree.
The original source bytes and token gaps MUST remain available throughout
formatting and fix application.

Comment attachment MUST use evidence from positions, token adjacency, AST
ownership, line relationships, and directive classification. A comment MUST
not migrate across a declaration, statement, operand, case, field, or build
boundary merely because the output parses.

### Package Loading Tiers

The engine MUST support increasingly expensive requirement tiers:

| Tier | Provides | Typical consumers |
| --- | --- | --- |
| Lexical | bytes, tokens, trivia, directives | formatter and raw-source rules |
| Syntax | parsed AST and source mapping | formatter and syntax lint rules |
| Types | packages and `go/types` information | type-aware lint rules |
| Control flow | per-function CFG | path-sensitive lint rules |
| SSA | SSA program/package/function data | deeper semantic lint rules |

The scheduler MUST compute the maximum tier required by enabled rules and MUST
NOT build more expensive representations speculatively.

### Parse Failure Policy

The first stable formatter MUST report syntax errors with precise locations and
MUST refuse in-place writes for the affected file. Formatting syntactically
incomplete editor buffers MAY be explored only after valid-file formatting is
proven and MUST have a separate safety contract.

## Formatter Architecture

### Document Intermediate Representation

The formatter MUST lower Go syntax into a layout-oriented document IR rather
than directly concatenating strings or manipulating source lines.

The IR SHOULD support equivalents of:

- text;
- concatenation;
- hard lines;
- soft lines;
- breakable lines;
- groups;
- indentation;
- conditional content when a group breaks;
- line suffixes for comments;
- fill behavior for suitable lists;
- break propagation; and
- source markers for diagnostics and debugging.

An illustrative, non-binding shape is:

```go
type Doc interface{ doc() }

type Text struct{ Value string }
type Concat struct{ Parts []Doc }
type Group struct{ Body Doc }
type Indent struct{ Body Doc }
type Line struct{}
type SoftLine struct{}
type HardLine struct{}
type IfBreak struct {
	Broken Doc
	Flat   Doc
}
```

The renderer MUST decide whether a group fits using bounded work. Adversarially
nested input MUST NOT cause exponential layout search or unbounded memory use.

### Width Model

The formatter MUST define whether print width is measured in bytes, Unicode
code points, terminal cells, or another documented unit. Tabs MUST have a
deterministic configured measurement while indentation output remains
idiomatic Go tabs unless a later compatibility decision explicitly changes it.

Unbreakable tokens longer than the configured width MUST remain intact and MUST
NOT cause failure. Diagnostics about such tokens, if any, belong to an optional
lint rule rather than the formatter.

### Canonical Layout Ownership

The formatter MUST define canonical behavior for at least:

- package clauses;
- import declarations and groups;
- constants, variables, types, and functions;
- receiver, type parameter, parameter, and result lists;
- blocks and statement lists;
- labels;
- simple statements;
- explicit semicolon-separated statements;
- `if` initializers and conditions;
- classic and range `for` clauses;
- `switch`, type switch, and `select` clauses;
- return expressions;
- assignments;
- unary and binary expressions by precedence;
- calls and generic instantiations;
- selectors and call chains;
- indexing and slicing;
- composite literals and keyed elements;
- array, slice, map, struct, interface, function, channel, and union types;
- function literals;
- type assertions;
- embedded fields;
- tags;
- comments in every supported position; and
- malformed nodes in diagnostic-only mode.

### Flat And Broken Forms

Each eligible construct SHOULD define a flat form and one canonical broken
form. The printer MUST NOT accumulate unrelated heuristics that select among
many visually similar layouts without a documented priority.

For example, a call MAY be modeled as:

```text
group(
  callee + "(" +
  indent(softline + join("," + line, arguments)) +
  ifBreak(",", "") +
  softline + ")"
)
```

Actual behavior MUST be proven against Go grammar, comments, trailing commas,
and nested groups before it becomes normative.

### Statement Expansion

Blocks containing statements MUST use hard line boundaries. Source such as:

```go
if err != nil { return err }
```

MUST NOT remain a compressed single-line block after formatting.

Explicit semicolons separating ordinary statements MUST become hard line
boundaries. Semicolons that are part of grammar constructs, including `if`,
classic `for`, and switch initializers, MUST remain structurally represented
and formatted in place.

### Comments And Directives

The formatter MUST preserve comment text unless a separately documented
comment-formatting capability is enabled. Initial releases SHOULD normalize
placement and indentation without reflowing prose.

Special handling MUST be specified and tested for:

- package documentation;
- declaration and field documentation;
- trailing comments;
- comments between operands or arguments;
- comments around braces and delimiters;
- build constraints;
- `//go:generate`;
- `//go:embed`;
- `//go:linkname` and other compiler directives;
- `//line` directives;
- cgo preambles; and
- lint suppression directives.

### Import Behavior

Import grouping, sorting, insertion, and removal MUST be an explicit product
decision. The formatter MUST NOT silently claim missing-import or unused-import
repair as ordinary layout. If Gox supports import organization, it MUST define
whether it belongs to `fmt`, a safe lint fix, or a separate code action and
MUST test initialization, blank import, dot import, cgo, comment, and grouping
behavior.

### Gofmt Compatibility

During Phase 0, the project MUST measure whether desired Gox output can also be
a gofmt fixed point:

```text
gofmt(goxfmt(input)) == goxfmt(input)
```

The decision MUST be corpus-backed and recorded. If strict fixed-point
compatibility is achievable without defeating the product, it SHOULD be an
initial invariant because it reduces adoption conflict. If it is not, Gox MUST
document exact divergence classes and provide a migration path for repositories
whose continuous integration still enforces gofmt.

Gox MUST always maintain its own idempotency invariant regardless of the gofmt
decision.

## Formatter Correctness Contract

For every successfully formatted valid input, verification MUST establish:

1. the output parses under the declared Go language version;
2. formatting the output again produces identical bytes;
3. normalized syntax before and after is equivalent under a documented model;
4. identifiers, operators, keywords, literal values, and declaration structure
   are preserved except for separately specified safe normalizations;
5. comments and directives are neither lost nor duplicated;
6. required comment/directive relative ordering is preserved;
7. the output respects configured line width whenever legal break opportunities
   exist, subject to documented exceptions;
8. generated trailing commas or removed explicit semicolons do not change
   meaning;
9. output ordering is deterministic; and
10. the formatter does not modify the filesystem in check or stdout modes.

AST comparison MUST ignore positions only where safe. It MUST explicitly
account for comments, implicit versus explicit semicolons, redundant
parentheses, import organization, and any normalization that changes raw AST
shape without changing semantics.

## Linter Architecture

### Native Rule Contract

Gox SHOULD expose a small native rule interface whose metadata declares at
least:

- stable rule identifier;
- summary and full documentation;
- default severity or disabled state;
- preset membership;
- required analysis tier;
- node interests where applicable;
- whether generated files are eligible;
- diagnostic categories;
- fix availability and safety classification;
- minimum Go language version;
- configuration schema; and
- deprecation or replacement metadata.

An illustrative shape is:

```go
type Requirement uint8

const (
	RequireSyntax Requirement = iota
	RequireTypes
	RequireControlFlow
	RequireSSA
)

type Rule interface {
	Metadata() Metadata
	Run(*Context) ([]Diagnostic, error)
}
```

The design MUST support adapting suitable `go/analysis.Analyzer` values so Gox
can interoperate with Go's analysis ecosystem without surrendering control of
its scheduling, configuration, reporting, or fix safety.

### Traversal

Syntax rules SHOULD share filtered traversal rather than each walking the full
AST independently. Rules MUST declare node interests where possible. The engine
MAY begin with `ast/inspector` and evolve only from benchmark evidence.

Typed, CFG, and SSA rules MUST reuse package-level representations within a run.
No rule may construct an uncoordinated package loader or duplicate an expensive
analysis behind the scheduler.

### Rule Strategy

The initial rule catalog MUST be selected from real defects, confusing
constructs, unsafe fixes, and repeated review findings observed in Go codebases.
Every proposed rule MUST state:

- the defective or undesirable observable behavior;
- why the compiler, vet, or existing default toolchain does not already make
  the rule redundant;
- expected false-positive boundaries;
- whether type, CFG, or SSA information is actually required;
- examples that must and must not report;
- whether a fix is possible;
- why the fix is safe, suggestive, or unsafe; and
- performance expectations.

The first stable release SHOULD contain a deliberately small correctness set.
It MUST NOT delay formatter maturity in pursuit of a headline rule count.

### Presets

The eventual preset model SHOULD include:

| Preset | Purpose | Default |
| --- | --- | --- |
| `correctness` | incorrect, unsafe, useless, or highly suspicious code | enabled |
| `suspicious` | likely defects with more contextual judgment | opt-in until proven |
| `performance` | measured or structurally credible inefficiencies | opt-in |
| `complexity` | maintainability and control-flow pressure | opt-in |
| `style` | non-layout source conventions | opt-in |
| `migration` | Go-version and API migrations | explicit target only |

Rules MAY move between presets only through a documented compatibility process.
Default enablement changes MUST be called out in release notes.

### Diagnostics

Diagnostics MUST contain:

- rule identifier;
- severity assigned by the driver/configuration;
- concise message;
- precise primary source range;
- optional related ranges;
- optional notes or help;
- zero or more named fixes;
- fix safety classification; and
- stable machine-readable representation.

Human output MUST be concise and actionable. Machine output MUST remain stable
within the documented compatibility policy. Parallel analysis MUST NOT produce
nondeterministically ordered diagnostics.

### Suppressions

Gox MUST support narrow, auditable suppressions. The suppression design MUST:

- identify exact rule IDs;
- support a reason policy;
- define line, next-line, range, file, and generated-file behavior explicitly;
- diagnose unknown, malformed, unused, or expired suppressions where enabled;
- prevent formatter movement from changing suppression ownership;
- preserve directives byte-for-byte unless a safe fixer intentionally changes
  them; and
- avoid one unscoped directive that disables all analysis silently.

### Rule Documentation

`gox explain <rule>` and the published documentation MUST derive from one
canonical metadata source. Every rule MUST include incorrect examples, correct
examples, prerequisites, configuration, fix safety, and known limitations.

## Fix Engine

### Fix Classes

Every fix MUST be classified as:

- **safe**: semantics-preserving under the rule's proven contract and eligible
  for ordinary `--fix`;
- **suggestion**: useful but requires user review and is exposed as a code
  action or explicit selection; or
- **unsafe**: may alter behavior or public contracts and requires an explicit
  unsafe-fix flag.

Severity and fix safety are independent. An error-level diagnostic does not
make its fix safe.

### Edit Model

Text edits MUST use source-file identity and exact byte ranges. Before applying
edits, the engine MUST verify that they target the source version that produced
the diagnostic.

The coordinator MUST:

1. collect edits for one source version;
2. validate all ranges;
3. order edits deterministically;
4. detect overlap and incompatible insertions;
5. reject conflicts without selecting a winner silently;
6. apply accepted edits in a range-safe order;
7. reparse the complete result;
8. format the result;
9. reparse and validate the formatted result;
10. write atomically; and
11. report unapplied fixes.

Multi-file fixes MUST use a separate transaction design and MUST NOT be added
until single-file fix safety is mature. The project MUST define recovery for a
partial filesystem failure before claiming multi-file atomicity.

## Configuration Contract

The configuration file SHOULD use a simple, typed, versioned format. A starting
shape MAY be:

```toml
version = 1

[format]
line-width = 100
tab-width = 4

[lint]
preset = "correctness"

[lint.rules]
complexity = "warn"
no-unexplained-suppression = "error"
```

The final design MUST define:

- filename and discovery order;
- repository-root and module-root behavior;
- inheritance and nested overrides;
- command-line precedence;
- unknown-key handling;
- schema version migration;
- per-file and per-path overrides;
- generated, vendor, testdata, and fixture policies;
- build tags and GOOS/GOARCH selection;
- rule severities and rule options;
- formatter width measurement; and
- configuration diagnostics with source ranges where possible.

Unknown fields and invalid values MUST fail clearly. Gox MUST NOT silently
ignore a misspelled rule or formatter option.

Configuration scope SHOULD remain understandable without reproducing a
general-purpose programming language or legacy cascaded configuration model.

## CLI And Filesystem Contract

### Inputs

Commands MUST define behavior for:

- explicit files;
- directories;
- recursive package patterns such as `./...` where applicable;
- standard input;
- `--stdin-filepath`;
- modules and `go.work` workspaces;
- test packages;
- files excluded by build tags;
- symlinks;
- ignored directories;
- vendor trees;
- generated files; and
- files outside a discovered project root.

Formatting SHOULD be file-oriented. Typed linting SHOULD be package-oriented.
The CLI MUST explain invalid combinations instead of applying surprising
fallbacks.

### Writes

In-place writes MUST:

- preserve file permissions;
- avoid following unsafe symlink changes between discovery and write;
- use same-directory temporary files and atomic replacement where supported;
- avoid changing modification time when output is identical;
- surface fsync or replacement limitations honestly;
- avoid partial content on interruption; and
- retain original content when validation fails.

### Exit Codes

The project MUST assign and document stable exit categories for:

- success with no findings;
- formatting differences or lint findings;
- parse or type errors;
- invalid configuration or invocation;
- fix conflicts;
- filesystem failures; and
- internal tool failures.

Human and machine reporters MUST agree on the outcome category.

## Performance Architecture

### Performance Goals

Benchmarks MUST be defined against representative cold and warm workloads, not
marketing microbenchmarks alone. Measure at least:

- one-file editor formatting latency;
- one-file syntax lint latency;
- medium module cold lint;
- large repository cold lint;
- repeated warm lint;
- type-aware incremental lint;
- peak resident memory;
- allocations per formatted file;
- diagnostic sorting overhead; and
- cache hit, miss, and invalidation behavior.

Oxfmt and Oxlint SHOULD inform the product expectation of continuous,
low-friction use, but Gox MUST publish its own reproducible Go-workload
benchmarks rather than transferring claims from another language ecosystem.

### Concurrency

Independent file formatting MAY run concurrently. Package analysis MAY run
concurrently where dependency and fact requirements permit. Concurrency MUST
be bounded and configurable or automatically derived from explicit resource
policy.

Output order, diagnostics, cache entries, and fixes MUST remain deterministic.
The project MUST test cancellation, worker failure, resource exhaustion, and
large-file behavior.

### Caching

Cache keys MUST include all inputs capable of changing results, including as
applicable:

- tool version;
- Go toolchain or language version;
- source digest;
- configuration digest;
- enabled rules and rule options;
- build tags;
- GOOS and GOARCH;
- dependency export data or facts;
- module/workspace state; and
- formatter compatibility mode.

Cache corruption MUST degrade to recomputation, not incorrect success. Cache
contents MUST be safe to delete and MUST NOT be the only source of diagnostics
or facts.

## Security And Robustness

Gox processes untrusted repositories and generated source. It MUST:

- bound recursion, layout search, concurrency, file size, and memory where
  feasible;
- avoid command execution during formatting and ordinary analysis;
- avoid evaluating generators or build scripts;
- treat configuration and source comments as data, not instructions;
- avoid network access by default;
- prevent path traversal outside authorized roots during writes;
- handle symlink races deliberately;
- avoid exposing source snippets in machine output unless requested;
- document subprocesses invoked for package loading or Go toolchain work;
- fuzz parsers, trivia attachment, document rendering, edit application, and
  configuration decoding; and
- provide a vulnerability reporting and supported-version policy before v1.

The formatter and syntax-only rules SHOULD operate without executing the Go
command. Typed loading MAY invoke standard Go tooling through documented
`go/packages` behavior and MUST preserve cancellation and sanitized errors.

## Repository Shape

The initial repository SHOULD keep public boundaries visible without premature
package fragmentation. A starting layout MAY be:

```text
cmd/gox/
internal/source/
internal/syntax/
internal/format/
internal/format/doc/
internal/format/print/
internal/analysis/
internal/rules/
internal/fix/
internal/config/
internal/discovery/
internal/report/
internal/cache/
internal/cli/
testdata/format/
testdata/lint/
testdata/corpus/
.ai/GOAL.md
```

Packages MUST be extracted from `internal` only when a stable external use case
and compatibility contract exist. The repository MUST NOT expose implementation
packages merely to simulate an ecosystem before the core product is stable.

## Verification Strategy

### Formatter Golden Tests

Every formatting rule MUST have paired input and expected-output fixtures.
Fixtures MUST cover flat, boundary-width, broken, nested, commented, directive,
and invalid variants. Expected files are behavioral contracts, not incidental
snapshots, and reviews MUST inspect their readability.

### Idempotency

Every valid formatting fixture and corpus file MUST satisfy:

```text
format(format(source)) == format(source)
```

Idempotency MUST also be fuzzed. Any unstable layout is a release-blocking
formatter defect.

### Syntax And Semantic Preservation

The project MUST define a normalized equivalence checker and document its blind
spots. It SHOULD combine:

- token/literal preservation;
- normalized AST comparison;
- comment/directive accounting;
- package compilation or type checking over owned fixtures;
- gofmt fixed-point comparison when promised; and
- corpus tests across supported Go versions.

No single comparison MUST be presented as complete semantic proof when it does
not cover the claim.

### Differential Tests

Use gofmt, go/format, gofumpt, and supported Go parser versions as differential
oracles where their contracts overlap. Differences MUST be classified as:

1. intentional Gox layout;
2. accepted upstream version difference;
3. unsupported syntax or version;
4. source-fidelity defect;
5. semantic-risk defect; or
6. unresolved investigation.

Differential tests MUST NOT force Gox to abandon deliberate width-aware output.

### Corpus Tests

The corpus MUST include:

- the Go standard library where licensing and tooling permit;
- representative open-source modules with recorded revisions;
- generated Go;
- cgo;
- build constraints;
- generics and current language features;
- very large files and declarations;
- comment-dense files;
- deliberately minified valid source;
- formatter adversarial cases; and
- internally discovered regressions.

Corpus provenance, license, revision, expected result, and update procedure MUST
be recorded. External source MUST NOT be copied into the repository without
license review.

### Fuzzing

Fuzz at least:

- scanner/trivia reconstruction;
- parser-to-document lowering;
- document rendering;
- idempotency;
- comment and directive preservation;
- edit overlap detection;
- edit application and rollback;
- configuration decoding; and
- suppression parsing.

Crashes, hangs, nondeterminism, invalid output, lost directives, and idempotency
failures MUST become minimized permanent regressions.

### Lint Rule Tests

Every rule MUST test:

- positive diagnostics;
- nearby valid non-diagnostics;
- exact source ranges;
- configuration variants;
- supported language versions;
- type-error behavior;
- generated-file behavior;
- suppression behavior;
- safe and unsafe fix classification;
- fixed output; and
- fix idempotency and interaction with formatting.

Tests MUST exercise the observable rule contract. Source-text assertions alone
MUST NOT substitute for analyzer behavior.

### Integration Tests

Integration coverage MUST exercise:

- all CLI modes;
- stdin and `--stdin-filepath`;
- modules and workspaces;
- check mode non-mutation;
- atomic writes;
- exit codes;
- text and machine reporters;
- configuration discovery and overrides;
- cancellation;
- cache corruption and invalidation;
- fix conflicts; and
- editor-facing protocol behavior when introduced.

## Compatibility And Versioning

### Supported Go Versions

Each Gox release MUST document:

- minimum runtime Go version if installed from source;
- toolchain version used to build official binaries;
- supported source language versions;
- how the language version is derived from `go.mod`, `go.work`, flags, or
  defaults; and
- behavior when encountering newer unsupported syntax.

Compiled releases naturally freeze their standard-library parser and formatter
dependencies at build time. Release evidence MUST record that toolchain so
formatting drift is attributable and reproducible.

### Formatter Stability

Formatting changes are user-visible compatibility changes even when semantics
are preserved. Releases that change canonical output MUST:

- document the affected constructs;
- include before/after examples;
- provide a check-mode migration path;
- distinguish deliberate changes from bug fixes;
- update corpus fingerprints; and
- avoid unrelated layout churn in the same release where practical.

The project SHOULD consider preview channels or explicit unstable modes before
changing high-churn formatter policy.

### Rule Stability

Rule IDs MUST be stable after public release. Renames and replacements MUST use
documented aliases or migration diagnostics. Default preset changes MUST be
versioned and announced.

Machine diagnostic schemas MUST carry a version before external integrations
depend on them.

## Phase 0: Product Contract And Architecture, 0% To 10%

### Objective

Turn the product idea into evidence-backed formatter, linter, frontend, CLI,
and compatibility contracts before committing to implementation architecture.

### Required Work

- Audit current Oxfmt, Oxlint, and relevant Oxc source architecture and public
  behavior.
- Audit Go parser, scanner, AST, token, printer, format, packages, types,
  analysis, inspector, CFG, and SSA capabilities.
- Record what information survives standard parsing and what requires original
  source/trivia retention.
- Build the initial hostile-valid-Go formatting corpus, including the examples
  that motivated the project.
- Define flat and broken layout rules for the first supported constructs.
- Spike document IR alternatives and select one with bounded rendering work.
- Measure gofmt fixed-point feasibility over desired output.
- Decide line-width measurement and indentation policy.
- Define the semantic/source equivalence model.
- Define command names, modes, exit categories, and configuration principles.
- Define analyzer tiers, rule metadata, diagnostics, fix classes, and
  suppression principles.
- Benchmark representative baseline costs for parsing, formatting, package
  loading, type checking, and traversal.
- Record architectural decisions and rejected alternatives.
- Complete the public naming and module-path preliminary audit.

### Deliverables

- formatter specification;
- source/trivia model specification;
- document IR decision record;
- gofmt compatibility decision record;
- lint engine decision record;
- fix safety model;
- CLI contract;
- configuration contract draft;
- initial corpus manifest;
- baseline benchmark harness; and
- risk register.

### Exit Gate: 10%

Phase 0 is complete only when representative examples can be lowered into the
chosen document model, required source fidelity is accounted for, the first
formatter contract is reviewable, and no unresolved architectural question
would plausibly require replacing the frontend or edit model.

## Phase 1: Formatter Core Prototype, 10% To 35%

### Objective

Prove that the selected frontend, trivia representation, document IR, and
renderer can produce correct, deterministic, width-aware Go.

### Required Work

- Implement immutable source loading and position mapping.
- Parse comments and classify directives.
- Construct the token/trivia index.
- Implement document primitives and bounded group-fit rendering.
- Implement file, declaration, statement, block, and expression dispatch.
- Format package clauses and imports without unsafe organization.
- Format functions, parameters, results, and type parameters.
- Expand blocks and explicit statement semicolons.
- Format calls, composite literals, function literals, and lists.
- Format boolean and other binary expressions with precedence-aware groups.
- Preserve required parentheses and literal spelling.
- Attach and render comments and directives.
- Add `fmt` stdin/stdout prototype behavior.
- Implement normalized syntax comparison.
- Establish golden, idempotency, corpus, and fuzz suites.
- Measure performance and eliminate algorithmic pathologies.

### Required Proof Cases

- every motivating compressed example expands readably;
- every implemented construct has flat and broken golden cases;
- boundary-width tests prove deterministic break decisions;
- comments between all significant child nodes remain attached correctly;
- directives remain correctly placed;
- output reparses;
- normalized syntax remains equivalent;
- repeat formatting is byte-identical;
- adversarial nesting has bounded execution; and
- promised gofmt compatibility passes the recorded corpus.

### Exit Gate: 35%

Phase 1 is complete when the formatter core handles the full motivating syntax
surface, survives the initial corpus and fuzz budget, has no known semantic or
directive-loss defect, and produces output readable enough for human approval.

## Phase 2: Production-Usable Formatter, 35% To 55%

### Objective

Turn the formatter engine into a safe daily-use tool for editors, repositories,
and continuous integration.

### Required Work

- Implement file and directory discovery.
- Implement module and workspace root discovery.
- Define ignore, vendor, testdata, symlink, and generated-file behavior.
- Implement versioned configuration parsing and discovery.
- Implement nested path overrides only if justified by user journeys.
- Add `--write`, `--check`, diff, stdin, and `--stdin-filepath` modes.
- Preserve permissions and implement atomic replacement.
- Avoid writes when output is unchanged.
- Add deterministic parallel file formatting.
- Add cancellation and bounded-resource behavior.
- Add text and initial JSON reporting.
- Add editor integration documentation for format-on-save.
- Add release artifacts and reproducible version metadata.
- Expand corpus coverage across supported Go syntax and real repositories.
- Publish formatter rule documentation with examples.
- Benchmark cold and editor-latency workloads.
- Run dogfood migrations and classify every unexpected diff.

### Exit Gate: 55%

Phase 2 is complete when Gox can replace a formatter command in a real Go
repository, check mode is reliably non-mutating, write mode is atomic and
validated, editor latency meets the recorded budget, and the formatter has no
unresolved high-risk corpus findings.

The formatter MAY be released independently at this point. Linter incompleteness
MUST NOT block users from adopting a proven formatter.

## Phase 3: Linter Foundation And Safe Fixes, 55% To 75%

### Objective

Build the modern lint platform on the proven shared frontend without allowing
rule quantity to destabilize architecture.

### Required Work

- Implement rule metadata and registry.
- Implement syntax-tier scheduling and filtered shared traversal.
- Implement deterministic diagnostic collection and sorting.
- Implement severity and preset resolution.
- Implement canonical rule documentation and `gox explain`.
- Implement suppression parsing, ownership, and diagnostics.
- Implement safe, suggestion, and unsafe fix metadata.
- Implement stale-range detection, overlap detection, and conflict reporting.
- Implement transactional single-file fix application.
- Reparse, format, and validate every fixed file.
- Implement text and versioned JSON reporters.
- Add GitHub-oriented output if required by adoption fixtures.
- Implement the `go/analysis` compatibility adapter.
- Select a small initial syntax correctness rule set from real evidence.
- Add complete behavioral rule and fix tests.
- Benchmark one-pass shared traversal against naive per-rule traversal.
- Dogfood default diagnostics and measure noise and false positives.

### Rule Admission Gate

A new built-in rule MUST NOT be admitted unless it has:

- a stable identifier;
- a documented problem statement;
- evidence that it adds value beyond the existing default Go toolchain;
- positive and negative fixtures;
- precise ranges;
- declared analysis cost;
- false-positive analysis;
- fix classification where applicable;
- generated-file and suppression behavior; and
- performance evidence proportional to its expected frequency.

### Exit Gate: 75%

Phase 3 is complete when syntax linting and safe single-file fixing are stable,
default diagnostics have acceptable signal in dogfood repositories, fixes
cannot silently overlap or corrupt source, and formatter normalization after
fixes is deterministic.

## Phase 4: Typed, Control-Flow, And SSA Analysis, 75% To 90%

### Objective

Add deeper correctness analysis while preserving fast syntax-only operation and
paying only for enabled rule requirements.

### Required Work

- Integrate `go/packages` with module, workspace, test, build-tag, GOOS, and
  GOARCH behavior.
- Load and share `go/types` information.
- Support package and object facts where required.
- Add reusable CFG construction behind explicit requirements.
- Add reusable SSA construction behind explicit requirements.
- Schedule package dependencies correctly and deterministically.
- Preserve partial results and clear diagnostics around type errors.
- Add cancellation throughout package loading and analysis.
- Implement cache keys and invalidation for typed results and facts.
- Add bounded package concurrency and memory controls.
- Select the first typed rules from real correctness evidence.
- Select CFG and SSA rules only where those representations materially improve
  precision.
- Test modules, workspaces, replacements, vendoring, build tags, cgo boundaries,
  internal packages, and test variants.
- Benchmark cold and warm typed runs and publish costs by rule tier.

### Typed Rule Discipline

A rule MUST NOT request types, CFG, or SSA merely for implementation
convenience. Its requirement tier MUST be the cheapest representation that can
meet its correctness and false-positive contract.

### Exit Gate: 90%

Phase 4 is complete when typed analysis is correct across representative module
and workspace shapes, syntax-only invocations remain fast and independent,
caches invalidate correctly, and deeper rules demonstrate high signal without
unbounded resource use.

## Phase 5: Ecosystem Integration And Stable Release, 90% To 100%

### Objective

Deliver Gox as a trustworthy, documented, supportable formatter-and-linter
product with a stable adoption path.

### Required Work

- Stabilize CLI, configuration, diagnostics, rule IDs, and exit codes.
- Complete the final binary and module naming audit.
- Provide editor integration, initially through reliable stdin/stdout and
  documented code-action ordering.
- Add an LSP or editor service only if it materially improves latency,
  diagnostics, fixes, or shared cache behavior.
- Provide GitHub and SARIF output where validated consumers require them.
- Document pre-commit and continuous-integration adoption.
- Document migration from gofmt, gofumpt, and golines workflows.
- Publish exact formatter divergences and compatibility expectations.
- Add shell completions and installation paths where supported.
- Produce signed, reproducible release artifacts and checksums.
- Establish vulnerability reporting and supported-version policies.
- Establish formatter change, rule deprecation, and configuration migration
  policies.
- Run final corpus, fuzz, race, integration, and performance gates.
- Run dogfood adoption in multiple representative Go repositories.
- Review the complete product against current Oxfmt and Oxlint experiences,
  explicitly recording deliberate differences caused by Go.
- Publish a roadmap for additional rules only after the foundation audit passes.

### Exit Gate: 100%

Gox reaches 100% only when all final acceptance criteria below are supported by
fresh evidence against the release candidate and no unresolved correctness,
source-fidelity, fix-safety, determinism, or release-blocking performance
finding remains.

## Progress Scale

Progress MUST be measured by proven capability, not files written, rules
started, or elapsed time.

| Progress | Proven state |
| ---: | --- |
| 0% | Mission recorded; no architecture or product contract proven |
| 5% | Reference audit, corpus, risks, and core decisions drafted |
| 10% | Phase 0 architecture and contracts approved |
| 20% | Source/trivia model and document renderer proven on core syntax |
| 35% | Formatter prototype passes motivating and initial corpus gates |
| 45% | Safe filesystem/configuration/CLI workflows proven |
| 55% | Formatter is production-usable and independently releasable |
| 65% | Syntax linter, diagnostics, suppressions, and fix coordinator proven |
| 75% | Linter foundation and safe single-file fixes are stable |
| 82% | Typed loading, facts, and caching work across modules/workspaces |
| 90% | Typed, CFG, and SSA analysis tiers meet correctness and cost gates |
| 95% | Editor/CI/release integrations and migration guidance are complete |
| 100% | Stable release candidate passes every final acceptance gate |

Percentages MUST NOT advance past a phase exit gate because isolated later work
exists. A broad rule catalog cannot compensate for an incomplete formatter,
unsafe fixer, or unstable analysis foundation.

## Decision Records Required

Before the relevant behavior stabilizes, create reviewed decisions for:

1. product and binary name;
2. supported Go versions;
3. source/trivia representation;
4. document IR and line-breaking algorithm;
5. width measurement and tabs;
6. comment attachment;
7. directive preservation;
8. import organization ownership;
9. gofmt fixed-point compatibility;
10. semantic equivalence validation;
11. invalid-source formatting policy;
12. rule API and `go/analysis` interoperability;
13. analysis requirement tiers;
14. suppression syntax and reason policy;
15. fix safety and conflict handling;
16. configuration discovery and inheritance;
17. generated, vendor, and testdata behavior;
18. cache inputs and invalidation;
19. machine diagnostic schema;
20. editor integration architecture;
21. extension mechanism, if any; and
22. formatter and rule compatibility policy.

Decisions MUST include context, evidence, alternatives, consequences, and a
revisit trigger. They MUST NOT merely restate the selected implementation.

## Risk Register

### AST Is Not A Concrete Syntax Tree

**Risk:** comments, explicit semicolons, parentheses, or source intent are lost
when formatting solely from `go/ast`.

**Required mitigation:** retain original bytes, tokens, positions, trivia, and
directives; prove comment and normalized-source preservation over corpus and
fuzz tests.

### Width-Aware Layout Conflicts With Go Syntax

**Risk:** a visually desirable line break triggers semicolon insertion or
requires a trailing comma.

**Required mitigation:** make breaks grammar-aware, generate required commas,
reparse every output, and maintain targeted precedence/semicolon fixtures.

### Gofmt Workflow Conflict

**Risk:** repositories run gofmt after Gox and produce perpetual diffs.

**Required mitigation:** measure fixed-point compatibility early; either enforce
it or publish precise divergence and migration behavior.

### Comment Migration

**Risk:** output parses but a comment changes ownership or directive meaning.

**Required mitigation:** explicit attachment model, comment identity accounting,
directive rules, dense-comment fixtures, and fuzzing.

### Layout Algorithm Pathology

**Risk:** nested groups cause exponential rendering or high memory use.

**Required mitigation:** bounded fit checks, adversarial benchmarks, fuzzing,
resource limits, and algorithmic complexity documentation.

### Type-Aware Latency

**Risk:** typed linting makes save-time workflows too slow.

**Required mitigation:** tiered requirements, shared package loading,
incremental caching, cancellation, and separate syntax-only editor paths.

### Rule Noise

**Risk:** rapid rule growth creates false positives and damages trust.

**Required mitigation:** correctness-first defaults, real-code dogfooding, rule
admission gates, documented false-positive boundaries, and opt-in presets.

### Unsafe Autofixes

**Risk:** seemingly mechanical rewrites change behavior or combine incorrectly.

**Required mitigation:** explicit safety classes, stale-source checks, overlap
detection, reparsing, formatting, validation, and unsafe opt-in.

### Configuration Sprawl

**Risk:** compatibility demands turn Gox into many inconsistent dialects.

**Required mitigation:** few formatter options, typed schemas, explicit
versioning, and rejection of options without a demonstrated adoption need.

### Premature Plugin API

**Risk:** third-party extension compatibility freezes immature internals.

**Required mitigation:** stabilize built-in rules and `go/analysis`
interoperability first; design an external extension boundary only from proven
use cases.

### Name Collision

**Risk:** `gox` conflicts with existing Go tools, module paths, binaries, or
trademarks.

**Required mitigation:** complete the naming audit before public release and
rename while compatibility cost is low if necessary.

## Documentation Requirements

Before stable release, documentation MUST include:

- product philosophy and Oxfmt/Oxlint reference boundaries;
- installation and supported platforms;
- command reference;
- configuration reference and schema;
- complete formatter behavior by syntax construct;
- print-width and tab measurement explanation;
- comments, directives, generated files, and import behavior;
- every lint rule and preset;
- diagnostic and suppression reference;
- fix safety and unsafe-fix policy;
- editor setup;
- CI and pre-commit setup;
- gofmt/gofumpt/golines migration guidance;
- machine output schemas;
- performance methodology and current results;
- supported Go versions;
- compatibility and release policy;
- security reporting; and
- contributor architecture and rule-authoring guidance.

Examples MUST be executable or mechanically checked where their contract allows
it. Documentation MUST be updated in the same change that alters public
behavior.

## Final Acceptance Criteria

### Formatter

- [ ] All documented supported Go syntax has canonical flat and broken layout.
- [ ] Motivating compressed examples become readable without manual edits.
- [ ] Print-width decisions are deterministic and documented.
- [ ] Output reparses under every supported source language version.
- [ ] Formatting is byte-idempotent.
- [ ] Normalized syntax equivalence passes the complete release corpus.
- [ ] Comments and directives are neither lost, duplicated, nor misowned.
- [ ] Required trailing commas and semicolon handling are syntax-correct.
- [ ] Gofmt compatibility matches the published decision and evidence.
- [ ] Check and stdout modes do not mutate the filesystem.
- [ ] Write mode is validated and atomic within documented platform limits.
- [ ] Generated, vendor, ignored, symlink, and external-root behavior is proven.
- [ ] Formatter performance meets published editor and repository budgets.

### Linter

- [ ] Default rules are correctness-focused and dogfood noise is acceptable.
- [ ] Rules execute only the analysis tier they declare.
- [ ] Syntax-only linting does not require typed loading.
- [ ] Typed, CFG, and SSA representations are shared and bounded.
- [ ] Diagnostics have precise, stable locations and deterministic order.
- [ ] Suppressions are narrow, auditable, and formatter-stable.
- [ ] Every rule has complete behavioral tests and canonical documentation.
- [ ] `go/analysis` interoperability satisfies the published boundary.
- [ ] Machine output schemas are versioned.
- [ ] Rule IDs and preset membership follow compatibility policy.

### Fixes

- [ ] Safe, suggestion, and unsafe fixes are distinct.
- [ ] Safe fixes have evidence supporting semantics-preserving claims.
- [ ] Stale edits and overlapping edits cannot apply silently.
- [ ] Every fixed result reparses, formats, and validates before write.
- [ ] Failed validation preserves original source.
- [ ] Fix output is deterministic and idempotent.
- [ ] Multi-file fixes, if present, have a documented failure transaction.

### Product And Operations

- [ ] One binary exposes the documented command surface.
- [ ] Configuration discovery, validation, precedence, and migration are stable.
- [ ] Exit codes distinguish findings from tool failures.
- [ ] Modules, workspaces, tests, build tags, GOOS, and GOARCH are covered.
- [ ] Cancellation and bounded concurrency are proven.
- [ ] Cache keys cover every result-changing input and corruption is recoverable.
- [ ] Editor and CI integrations use stable supported interfaces.
- [ ] Release artifacts are reproducible, signed, and checksummed.
- [ ] Naming and module-path audits are complete.
- [ ] Supported-version and vulnerability policies are published.
- [ ] Final corpus, fuzz, race, integration, and performance gates pass.
- [ ] Multiple real Go repositories have completed documented dogfood adoption.

### Architecture

- [ ] Formatter and linter remain separate engines over a shared frontend.
- [ ] Layout rules are not duplicated as lint rules.
- [ ] Semantic fixes do not hide inside formatting.
- [ ] The standard Go frontend is reused wherever it satisfies requirements.
- [ ] Source fidelity does not rely on AST data alone.
- [ ] Document rendering has bounded complexity.
- [ ] Expensive analysis is demand-driven and shared.
- [ ] No premature plugin or public package surface freezes unstable internals.
- [ ] Current Oxfmt and Oxlint behavior has been reviewed at final readiness.
- [ ] Additional rule expansion begins only after the foundation audit passes.

## Completion Definition

This goal is complete only when Gox is a trustworthy daily-use replacement for
the combination of a conventional Go formatter, width-aware line rewriter, and
high-signal lint/fix runner for its documented scope.

Completion is not a rule-count milestone. It requires a formatter whose output
developers accept without manual cleanup, a linter whose default diagnostics
earn trust, a fixer that cannot silently corrupt source, architecture that can
scale to deeper analysis and more rules, and fresh release evidence for every
public claim.

The product SHOULD feel modern in the same ways Oxfmt and Oxlint do: fast,
focused, cohesive, predictable, easy to adopt, and built on a shared foundation.
It MUST remain Go-native in syntax, semantics, tooling integration, and
implementation.
