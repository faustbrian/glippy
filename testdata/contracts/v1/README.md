# v1 Text Contracts

These files are the maintainer-approved Glippy v1 text contracts consumed by
focused CLI tests and every native tagless release-candidate rehearsal.

- `help.txt` freezes top-level CLI help.
- `commands.txt` freezes exact usage for every top-level command.
- `rules.txt` freezes rule identifiers, default severity, preset membership,
  analysis tier, and fix availability.
- `profiles.txt` freezes resolved default, recommended, strict, and pedantic
  configuration policy using the explicit fixtures in `config/`.
- `formatter.txt` freezes the canonical output and digest of the complete
  hostile corpus plus every motivating example at their reviewed widths using
  the explicit configurations in `formatter/`.
- `machine.txt` freezes representative formatter, lint, combined-check, rule
  metadata, source-error, invocation, and conflict output with process exits
  0 through 4 using `machine/`.
- `failure-exits.txt` freezes injected filesystem, internal-invariant, and
  cancellation outcomes with exits 5, 6, and 130.

Changes require the formatter or rule compatibility evidence required by the
v1 roadmap. Cross-platform agreement alone does not authorize updating these
files.
