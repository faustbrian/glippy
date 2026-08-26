# Suppressions

Glippy suppressions waive one exact lint rule over one explicit physical source
scope. They do not disable formatting, parser or type errors, configuration
errors, other rules, or every rule at once.

Use a suppression only after confirming the diagnostic and recording why the
exception is necessary. Prefer the narrowest scope that owns the intended
diagnostic.

## Syntax

Suppression directives are exact line comments with no space between `//` and
`glippy:`. Each directive names exactly one registered rule ID.

| Directive | Scope |
| --- | --- |
| `//glippy:ignore rule-id [-- reason]` | Immediately following physical line |
| `//glippy:ignore-line rule-id [-- reason]` | Directive's physical line |
| `//glippy:ignore-start rule-id [-- reason]` | Until the matching range end |
| `//glippy:ignore-end rule-id` | Ends the open range for the same rule |
| `//glippy:ignore-file rule-id [-- reason]` | Complete file; must appear before `package` |

There is no list syntax and no disable-all directive. Use a separate comment
for each waived rule.

Through the first stable Glippy release, legacy `//gox:` forms retain the same
scope and suppression behavior but always produce a deterministic
`legacy-directive` finding. Replace the prefix with `//glippy:`; there is no
flag that silently hides this migration finding.

## Examples

Suppress one diagnostic on the following line:

```go
func run(ready bool) {
	for {
		switch {
		case ready:
			//glippy:ignore ineffective-break -- retained for protocol compatibility
			break
		}
	}
}
```

Suppress a diagnostic whose primary location is on the directive's line:

```go
func run(ready bool) {
	for {
		switch {
		case ready:
			break //glippy:ignore-line ineffective-break -- generated decision table
		}
	}
}
```

Suppress one rule within an explicit range:

```go
func run(first, second bool) {
	//glippy:ignore-start ineffective-break -- legacy state machine
	for {
		switch {
		case first:
			break
		}
		switch {
		case second:
			break
		}
	}
	//glippy:ignore-end ineffective-break
}
```

Ranges for the same rule cannot nest. The range end cannot carry a reason;
the matching start owns the waiver metadata.

A file-wide suppression must precede the package clause:

```go
//glippy:ignore-file ineffective-break -- generated compatibility adapter
package adapter
```

## Ownership

A suppression matches only a diagnostic with the same rule ID whose primary
range starts inside the directive's target. Merely overlapping the target is
not sufficient. Matching is also bound to the exact normalized source path and
source digest that produced the diagnostic, so a stale directive index cannot
hide a finding from another file version.

When more than one same-rule directive could own a diagnostic, the first valid
directive in source order owns it. Other matching directives remain unused
unless they suppress a different diagnostic.

Glippy reports every valid suppression that owns no diagnostic as unused. This
makes obsolete and redundant waivers visible in `lint` and `check` output.

## Reasons

`--` separates the rule ID from a human reason:

```go
//glippy:ignore ineffective-break -- required by protocol version 1
```

Reasons are optional by default. A present separator must be followed by a
non-empty reason. Projects can require reasons:

```toml
version = 1

[lint.suppressions]
require-reason = true
```

With this policy, direct scopes and range starts without a reason are invalid
and do not suppress diagnostics. Range ends remain reasonless.

## Expiry

The first reason field can record a calendar expiry:

```go
//glippy:ignore ineffective-break -- expires=2026-09-01 remove after migration
```

The date must be a real `YYYY-MM-DD` date and must be followed by a human
reason. Glippy never reads the wall clock. A project chooses the deterministic
evaluation date in configuration:

```toml
version = 1

[lint.suppressions]
expiry-cutoff = "2026-09-01"
```

A suppression whose expiry is on or before the cutoff is reported as expired
and does not suppress its diagnostic. Omitting `expiry-cutoff` preserves the
structured expiry metadata without deciding that it has expired.

## Problems And Findings

Glippy reports these suppression problems in source order:

- malformed syntax or a missing rule ID;
- an unknown rule ID;
- a missing required reason;
- an invalid or expired date;
- a file suppression after the package clause;
- a nested same-rule range;
- an unmatched range end; and
- an unclosed range start.

Invalid and expired directives do not hide diagnostics. Visible diagnostics,
suppression problems, and unused suppressions produce the findings exit
category. A valid suppression by itself does not.

Text output identifies the physical location and problem kind. JSON output
uses the versioned lint or combined-check schema and keeps visible diagnostics,
suppression problems, unused directives, and suppressed counts separate. It
does not expose suppressed diagnostic bodies by default.

## Formatting And Fixes

The formatter preserves suppression comment text and verifies ownership using
physical token relationships. If formatting would move a structurally valid
directive to a different target, Glippy rejects the complete output instead of
returning or writing altered ownership.

Lint fixes use the same source-versioned suppression index. Suppressed
diagnostics are not selected for fixing. Formatting and reanalysis after an
accepted fix must preserve the remaining suppression ownership before any file
replacement succeeds.

See the [lint engine specification](spec/lint-engine.md) for the normative
ownership contract and the [configuration specification](spec/configuration.md)
for discovery and precedence.
