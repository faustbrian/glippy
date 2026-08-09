# Preliminary Naming Audit

Date: 2026-08-09

## Conclusion

`gox` is not suitable as the public product or binary name without accepting
direct ecosystem collisions. The working name may remain during private Phase
0 work, but a replacement name and owner-controlled module path are required
before a public tag. This is a preliminary technical collision audit, not
legal clearance.

The current private module path, `github.com/faustbrian/gox`, follows this
repository's configured remote. It is provisional and must change with the
product name before third parties can depend on it.

## Direct Collisions

| Project | Current evidence | Collision |
| --- | --- | --- |
| [mitchellh/gox](https://github.com/mitchellh/gox) | 4,577 GitHub stars; releases through v1.0.1; installation produces `gox` | Established Go cross-compiler using the exact binary name |
| [mentasystems/gox](https://github.com/mentasystems/gox) | Active in 2026; v0.5.0 released 2026-07-02 | Go linter with overlapping `gox check` and `gox explain` commands |
| [germtb/gox](https://github.com/germtb/gox) | Active Go tool with Go install and Homebrew instructions | Exact binary plus overlapping `fmt` and `version` commands |
| [icza/gox](https://github.com/icza/gox), [8byt/gox](https://github.com/8byt/gox), [topxeq/gox](https://github.com/topxeq/gox) | Existing Go repositories and modules | Repository, module, and search ambiguity |

GitHub repository search for `gox in:name language:Go` returned 620 results.
The Go proxy confirmed published versions for the Mitchell, Menta Systems, and
Icza module paths. The npm registry also contains `gox` 0.1.0, and `gox.io` is
registered and parked for sale. No Homebrew core formula or Arch AUR package
named `gox` was found, but Germ distributes its binary through a tap.

## Search Record And Limits

Queries covered GitHub repository search, pkg.go.dev, the Go module proxy,
Homebrew's formula API, AUR RPC, npm, crates.io, DNS and HTTP probes for likely
domains, and preliminary web searches for software trademarks. USPTO, TMview,
and WIPO search entry points were reachable, but their interactive flows did
not produce a reliable automated exact-mark result.

Before selecting a replacement, repeat repository, module, binary, package
manager, domain, and social-name searches and obtain jurisdiction- and
class-specific trademark advice. A negative automated search must not be
treated as legal clearance.

## Revisit Trigger

Revisit this decision before any public release, package publication,
installation documentation, or external integration references the working
name or current module path.
