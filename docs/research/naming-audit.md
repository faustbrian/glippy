# Naming And Ecosystem Collision Audit

- Initial audit: 2026-08-09
- Candidate refresh: 2026-08-11

## Conclusion

`gox` is not suitable as the public product or binary name without accepting
direct ecosystem collisions. The working name may remain during private
development, but a replacement name and owner-controlled module path are
required before a public tag. This is a technical collision audit, not legal
clearance.

The current private module path, `github.com/faustbrian/gox`, follows this
repository's configured remote. It is provisional and must change with the
product name before third parties can depend on it. The recommended technical
replacement is **Gofettle**, with binary `gofettle` and proposed module path
`github.com/faustbrian/gofettle`, subject to maintainer approval, repository
rename, and professional trademark review.

## Direct Collisions

| Project | Current evidence | Collision |
| --- | --- | --- |
| [mitchellh/gox](https://github.com/mitchellh/gox) | 4,577 GitHub stars; archived but still installed as `gox` | Established Go cross-compiler using the exact binary name |
| [mentasystems/gox](https://github.com/mentasystems/gox) | Active experimental Go linter with direct `go install` instructions | Exact binary plus overlapping `check` and `explain` commands |
| [germtb/gox](https://github.com/germtb/gox) | Active Go/JSX tool distributed through Go install, GitHub releases, and Homebrew | Exact binary plus overlapping `fmt` and `version` commands |
| [icza/gox](https://github.com/icza/gox), [8byt/gox](https://github.com/8byt/gox), [topxeq/gox](https://github.com/topxeq/gox) | Existing Go repositories and modules | Repository, module, and search ambiguity |

The 2026-08-11 GitHub repository search for `gox in:name language:Go`
returned 621 results. The Go proxy confirmed published versions for the
Mitchell, Menta Systems, and Icza module paths. The npm registry also contains
`gox` 0.1.0, and `gox.io` is registered and parked for sale. No Homebrew core
formula or Arch AUR package named `gox` was found, but Germ distributes its
binary through a tap.

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

Queries covered authenticated GitHub repository and code search, GitHub and
Docker user lookups, pkg.go.dev, the Go module proxy, Homebrew's formula API,
AUR RPC, npm, PyPI, crates.io, and RDAP for candidate `.com` and `.dev`
domains. Search results are point-in-time evidence dated above. USPTO, TMview,
and WIPO search entry points did not produce a reliable automated exact-mark
result, so this audit makes no legal-clearance claim.

Before adopting the recommendation, obtain jurisdiction- and class-specific
trademark advice. Immediately before any public rename or reservation, repeat
the repository, module, binary, package-manager, domain, and social-name checks.
A negative automated search must not be treated as legal clearance.

## Revisit Trigger

The maintainer must approve or reject `Gofettle`. After approval and the final
technical refresh, rename the repository, module path, binary, CLI examples,
configuration references, cache namespaces, and release metadata as one
coherent migration before any public release, package publication, installation
documentation, or external integration references the current working name.
