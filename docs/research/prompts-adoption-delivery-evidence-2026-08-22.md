# `pkg/prompts` Adoption Delivery Evidence, 2026-08-22

## Immutable Source Boundary

The integrated adoption commit is
`5eb1b997c2242b456b5f5318547269e5eeca2219` in
`github.com/faustbrian/golib`. A task-owned fetch of the current remote `main`,
then at `6c366188db05f092cb61c4e6f21f803bddb5970a`, proved that the adoption commit
is its ancestor. The remote branch has advanced since the adoption; the
immutable commit, not a mutable branch name, identifies this evidence.

The following read-only Git inventories were evaluated against that commit:

```text
git ls-tree -r --name-only 5eb1b997 -- pkg/prompts
  Go paths: 90
  sorted path-stream SHA-256:
  21668af7c71a546c7fba3b7228c1f3eb1501935e04a18e6668048437e80823b1

git show --format= --name-only 5eb1b997 -- pkg/prompts
  changed paths: 45
  changed Go paths: 42
  path-stream SHA-256:
  5a0db3dc69a1c03c275de2d2f042e820416c028cf4d809edf0c028b2c452a7cf

git diff --name-only 8c9c1e7..5eb1b997 -- pkg/prompts
  changed paths: 92
  changed Go paths: 82
  path-stream SHA-256:
  13da657d237924189eeee7d1f6cd94cd08b28a827caa0898ca9432f150bd96b9
```

The 92-file baseline delta contains intervening behavior and test work. It is
not a formatter-only patch and MUST NOT be used as one.

## Integrated Policy

The Makefile at `5eb1b997` pins Glippy pseudo-version
`v0.1.1-0.20260821090210-724d8a26eec0`, defines `format` and `format-check`
through that binary, and includes `format-check` in the canonical `check`
target. The integration commit removes competing gofmt and goimports authority
from the module's repository runner.

## Verification Boundary

The original approved migration at `d6b0fba8` retains its recorded fixed-point,
test, race, vet, tidy, documentation, lint, nested-module, and maintainer-review
evidence in the earlier dogfood and adoption-review records.

The repository does not retain an immutable command/result artifact for the
later complete-package gate at `5eb1b997`. Earlier v0.5 documents repeated that
claim but did not make it auditable. It is therefore historical, unverified
reporting and is not current release evidence. No broad external gate was rerun
for this record; runtime benchmarks, fuzzing, mutation, signal, interruption,
process-tree, RSS, and Docker work remain prohibited on the shared development
host.
