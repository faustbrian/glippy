# Decision Coverage

The required product decisions are covered as follows. A provisional or
deferred status is an explicit gate, not silent approval.

| Required decision | Record |
| --- | --- |
| Product, binary, and module name | ADR 0001 records immutable Gox v0.1 history; ADR 0016 owns the Glippy v0.2 migration and compatibility window |
| Supported Go versions | ADR 0008 and supported-version policy; Go 1.25 and Go 1.26 accepted |
| Source/trivia representation | ADR 0005 and source model specification |
| Document IR and line breaking | ADR 0002 |
| Width and tabs | ADR 0003 |
| Comment attachment | ADR 0009; initial project-owned corpus proof complete |
| Directive preservation | ADR 0009; initial project-owned corpus proof complete |
| Import organization ownership | Formatter specification; layout only, no organization |
| Gofmt fixed point | ADR 0004; product-wide fixed point rejected with recorded divergences |
| Semantic equivalence | Equivalence specification |
| Invalid-source formatting | Source model specification; diagnostic-only |
| Rule API and `go/analysis` | ADR 0006 and lint specification; native package-wide types rules plus syntax, typed DAG, package-fact, and object-fact adapters implemented |
| Analysis requirement tiers | ADR 0005; ADR 0017 owns cross-package semantic effect facts |
| Suppression syntax and reasons | ADR 0006, lint specification, and public suppression reference; exact-rule grammar implemented |
| Fix safety and conflicts | ADR 0006 and fix specification |
| Configuration discovery | ADR 0007 and schema-version-1 configuration contract; strict discovery and precedence implemented |
| Composable lint policy | ADR 0014; preset unions, warning escalation, and restriction boundaries implemented |
| Deterministic lint baselines | ADR 0015; source-bound generation, stale and expiry policy, fix exclusion, and additive machine reporting implemented |
| Generated, vendor, and testdata behavior | ADR 0007 and schema-version-1 configuration contract; default policies implemented |
| Cache inputs and invalidation | ADR 0008 and cache specification; opt-in typed CLI ownership, canonical configuration identity, and bounded post-run pruning implemented |
| Machine diagnostics | ADR 0011 and public machine-output reference; schema version 1 covers formatter, lint, fix, typed, and combined-check reports |
| Formatter diff output | ADR 0012; bounded text-only unified differences |
| Editor architecture | ADR 0013; stdin/stdout formatter accepted, persistent service evidence-gated |
| Extension mechanism | Syntax and audited typed `go/analysis` adapters implemented; no dynamic plugin API initially |
| Formatter and rule compatibility | ADR 0008 |

The shared frontend and single-file edit model have no open question that
currently requires replacing them. Remaining provisional decisions constrain
compatibility, naming, and later product surfaces rather than the foundational
source or edit representation.
