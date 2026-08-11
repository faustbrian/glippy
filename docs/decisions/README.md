# Decision Coverage

The required product decisions are covered as follows. A provisional or
deferred status is an explicit gate, not silent approval.

| Required decision | Record |
| --- | --- |
| Product, binary, and module name | ADR 0001, provisional replacement required |
| Supported Go versions | ADR 0008, prototype only; public range deferred |
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
| Analysis requirement tiers | ADR 0005 |
| Suppression syntax and reasons | ADR 0006 and lint specification; initial Phase 3 grammar accepted |
| Fix safety and conflicts | ADR 0006 and fix specification |
| Configuration discovery | ADR 0007 and configuration draft |
| Generated, vendor, and testdata behavior | ADR 0007 and configuration draft |
| Cache inputs and invalidation | ADR 0008 and cache specification; opt-in typed CLI ownership, canonical configuration identity, and bounded post-run pruning implemented |
| Machine diagnostics | ADR 0011; formatter, lint-check, and lint-fix prototype schema accepted |
| Formatter diff output | ADR 0012; bounded text-only unified differences |
| Editor architecture | Open: stdin/stdout is the prototype boundary; ADR required before Phase 5 integration |
| Extension mechanism | Syntax and audited typed `go/analysis` adapters implemented; no dynamic plugin API initially |
| Formatter and rule compatibility | ADR 0008 |

The shared frontend and single-file edit model have no open question that
currently requires replacing them. Remaining provisional decisions constrain
compatibility, naming, and later product surfaces rather than the foundational
source or edit representation.
