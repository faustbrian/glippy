# v1 Text Contracts

These files are the maintainer-approved Glippy v1 text contracts consumed by
focused CLI tests and every native tagless release-candidate rehearsal.

- `help.txt` freezes top-level CLI help.
- `rules.txt` freezes rule identifiers, default severity, preset membership,
  analysis tier, and fix availability.
- `profiles.txt` freezes resolved default, recommended, strict, and pedantic
  configuration policy using the explicit fixtures in `config/`.

Changes require the formatter or rule compatibility evidence required by the
v1 roadmap. Cross-platform agreement alone does not authorize updating these
files.
