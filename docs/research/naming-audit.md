# Naming And Ecosystem Collision Audit

- Initial audit: 2026-08-09
- Candidate refresh: 2026-08-11
- Maintainer development-name decision: 2026-08-12
- Final-candidate technical refresh: 2026-08-13

## Conclusion

`gox` has direct ecosystem collisions. The maintainer has chosen to retain Gox,
binary `gox`, and module path `github.com/faustbrian/gox` during development and
has rejected an immediate rename. This development decision does not constitute
legal clearance or final public-release acceptance of the collisions.

The earlier technical screen recommended **Gofettle**, binary `gofettle`, and
module path `github.com/faustbrian/gofettle`. That proposal came from this
agent-produced collision audit rather than the maintainer and is not the chosen
development identity. It remains historical candidate evidence only.

## Direct Collisions

| Project | Current evidence | Collision |
| --- | --- | --- |
| [mitchellh/gox](https://github.com/mitchellh/gox) | 4,576 GitHub stars; archived but still installed as `gox` | Established Go cross-compiler using the exact binary name |
| [mentasystems/gox](https://github.com/mentasystems/gox) | Active experimental Go linter with direct `go install` instructions | Exact binary plus overlapping `check` and `explain` commands |
| [germtb/gox](https://github.com/germtb/gox) | Active Go/JSX tool distributed through Go install, GitHub releases, and Homebrew | Exact binary plus overlapping `fmt` and `version` commands |
| [icza/gox](https://github.com/icza/gox), [8byt/gox](https://github.com/8byt/gox), [topxeq/gox](https://github.com/topxeq/gox) | Existing Go repositories and modules | Repository, module, and search ambiguity |

The 2026-08-13 final-candidate GitHub repository search for
`gox in:name language:Go` returned 620 results. Exact repository names include
the projects above plus active `icza/gox`, `topxeq/gox`, `doors-dev/gox`, and
others. The Go proxy confirms published versions for the Mitchell, Menta
Systems, and Germ module paths. The exact GitHub user `gox` is occupied.

Package and domain checks add further collisions: npm contains `gox` 0.1.0,
crates.io contains `gox` 0.4.0, and both `gox.io` and `gox.dev` are registered.
No Homebrew core formula, Arch AUR package, PyPI project, or Docker user named
`gox` was found at the refresh time, but Germ distributes its binary through a
tap. These negative results are not reservations and can change independently.

## Replacement Candidate Screen

The refresh screened pronounceable names that communicate code being put into
a clean, sound state. Exact GitHub repository-name searches returned no result
for `gofettle`, `goburnish`, `goquoin`, `goarden`, `gomeld`, or `gosculpt`.
The broader code search and naming review then narrowed the useful candidates:

| Candidate | Technical result | Product concern | Disposition |
| --- | --- | --- | --- |
| `gofettle` | No exact GitHub repository, GitHub user, Docker user, pkg.go.dev result, npm package, PyPI project, crates.io crate, Homebrew formula, or AUR package found; `gofettle.dev` had no RDAP record | `gofettle.com` is registered; legal mark search remains incomplete | Recommended |
| `goburnish` | Same checked technical namespaces were clear; GitHub code search returned no exact use; `goburnish.dev` had no RDAP record | Longer binary and less direct Go pronunciation; `.com` is registered | Reserve alternative |
| `goquoin` | Checked package and repository namespaces were clear | Unfamiliar spelling and pronunciation; `.com` is registered | Rejected |
| `goarden` | Checked package and repository namespaces were clear | Visually confusable with “garden” and noisy in code search; `.com` is registered | Rejected |
| `gomeld` | Checked package and repository namespaces were clear | Generic phrase with high unrelated code-search noise; `.com` is registered | Rejected |
| `gosculpt` | Checked package and repository namespaces were clear | Generic product language with high unrelated code-search noise; `.com` is registered | Rejected |

`Gofettle` is short enough for a frequently typed CLI, retains an obvious Go
association, and describes formatting, linting, and fixing without naming only
one engine. The negative checks are a point-in-time technical screen, not a
reservation. The GitHub repository, module path, package-manager names, social
names, and domain can be taken by another party until they are actually
secured.

## Search Record And Limits

Queries covered authenticated GitHub repository search, GitHub and Docker user
lookups, pkg.go.dev, the Go module proxy, Homebrew's formula API, AUR RPC, npm,
PyPI, crates.io, WHOIS, and RDAP. Search results are point-in-time evidence
dated above. USPTO, TMview, and WIPO entry points still do not provide a
reliable automated exact-mark clearance result for this audit. `Gox` is a very
short mark used by multiple software projects, so absence of an automated
record could not establish legal availability in any jurisdiction or class.

The technical refresh is complete and demonstrates material collision risk.
Before a public release, the maintainer must explicitly accept that risk for
Gox or authorize a coherent rename. Jurisdiction- and class-specific trademark
advice remains the recommended route to legal clearance; a negative automated
search must not be treated as legal advice or clearance.

## Revisit Trigger

Immediately before a public tag or installation contract, refresh the technical
screen and record one final decision: retain Gox with explicit collision and
trademark evidence, or rename the repository, module path, binary, CLI examples,
configuration references, cache namespaces, and release metadata coherently.
