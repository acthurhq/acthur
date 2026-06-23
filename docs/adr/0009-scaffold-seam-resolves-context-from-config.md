# `internal/scaffold` is the kernel seam that resolves ScaffoldContext from config

`module_prefix` and `identifiers.strategy` live in `acthur.yml` but, until now, never reached the adapter's `ScaffoldContext` — the config fields were inert and only unit tests that hand-built a context could prove scaffolding honored them, so `acthur.yml` was not yet the single source of truth for generated code (PRD Principle 7). We introduce a dedicated `internal/scaffold` package whose one pure function, `ResolveScaffoldContext(cfg, nodeID) adapter.ScaffoldContext`, assembles the resolved facts the kernel hands an adapter (`ModulePath`, `ProjectName`, `IDStrategy`), implementing the host-less `<project>` default when `module_prefix` is unset (ADR 0007). The adapter never computes these.

## Considered Options

- **`internal/engine` (rejected).** It already imports both `config` and `adapter`, so it would add no new dependency edge — but per the repo layout `engine` is the *Dev Orchestrator + Deploy Engine* (Phase 3/8 runtime). Parking a pure, side-effect-free Phase-2 resolution function there mislabels the concern and transitively couples it to `process`/`proxy`/`health`. A future reader would reasonably ask why scaffold resolution lives in the dev orchestrator.
- **`internal/adapter` or `internal/config` (rejected).** Both are clean leaves (`adapter` imports only `fmt`/`sync`; `config` imports nothing internal). Placing the function in either would force one leaf to import the other, costing `adapter` its leaf status and pointing the dependency the wrong way — an individual adapter must not read `config`.

## Consequences

- `adapter` and `config` stay leaves; `scaffold` is the only package that bridges them, importing both and nothing heavier.
- No CLI command calls the resolver in Phase 2 (same posture as `Scaffold` itself: exercised at its seam, wired by a later phase). Its unit tests — `module_prefix` set → org path, unset → host-less `<project>/<nodeID>`, `identifiers.strategy` → `IDStrategy` — are what let User Stories #12 and #13 be closed honestly, through kernel code rather than a fabricated struct.
- "Resolver" (CONTEXT.md) remains reserved for adapter-key resolution by the graph engine; this is *scaffold-context resolution*, a pure assembly function, not a `Resolver`.
