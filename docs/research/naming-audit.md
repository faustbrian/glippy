# Naming And Ecosystem Collision Audit

- Initial audit: 2026-08-09
- Candidate refresh: 2026-08-11
- Maintainer development-name decision: 2026-08-12
- Final-candidate technical refresh: 2026-08-13
- Maintainer Glippy v0.2 direction: 2026-08-13

## Conclusion

`glippy` has direct ecosystem collisions. The maintainer selected **Glippy**,
binary `glippy`, intended repository `github.com/faustbrian/glippy`, and module
path `github.com/faustbrian/glippy` for v0.2 development after Gox v0.1.0. This
is a maintainer product-risk decision, not legal clearance in any jurisdiction
or trademark class.

The earlier technical screen recommended **Gofettle**, binary `gofettle`, and
module path `github.com/faustbrian/gofettle`. That proposal came from this
agent-produced collision audit rather than the maintainer and is not the chosen
development identity. It remains historical candidate evidence only.

## Direct Collisions

| Project | Current evidence | Collision |
| --- | --- | --- |
| [F1bonacc1/glippy](https://github.com/F1bonacc1/glippy) | Active Go clipboard project; pushed 2026-04-23; Go proxy lists v1.1.0 and v1.2.0 | Exact repository and likely binary/module search ambiguity |
| [quequotion/glippy](https://github.com/quequotion/glippy), [oscard0m/glippy](https://github.com/oscard0m/glippy), and `unkarelian/Glippy` | Existing exact-name repositories in other or unspecified languages | Repository and search ambiguity |
| [GitHub user `glippy`](https://github.com/glippy) | Account created in 2013 and occupied | Exact account namespace unavailable |

The 2026-08-13 unauthenticated GitHub repository search for `glippy in:name`
returned nine results; adding `language:Go` returned the single active
`F1bonacc1/glippy` project. The Go proxy confirms its published v1.1.0 and
v1.2.0 module versions. The exact GitHub user `glippy` is occupied. No broader
package, domain, or trademark search was completed for this rename, so no
negative availability claim is made.

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

The Glippy refresh used GitHub repository and account APIs and the Go module
proxy. Results are point-in-time evidence dated above. Trademark registries and
jurisdiction-specific legal review were not completed. `Glippy` is a short mark
used by multiple software projects, so this audit cannot establish legal
availability in any jurisdiction or class.

The technical refresh demonstrates material collision risk, which the
maintainer accepted for the v0.2 development direction. Gox v0.1.0 remains the
immutable historical release identity.

## Revisit Trigger

Refresh the technical screen before a later stable-identity expansion, package
manager registration, or whenever a material new `glippy` collision appears.
