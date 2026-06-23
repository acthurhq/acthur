# PRD — Phase 2 Closeout: Honest Adapter/Scaffold Seam

> **Status:** ready-for-agent · **Tracker:** [samueloshio/acthur#24](https://github.com/samueloshio/acthur/issues/24) · **Phase:** 2 of the implementation plan (PRD §21)
> **Parent:** closes the loose ends of [#17 — PRD Phase 2: Adapter System](https://github.com/samueloshio/acthur/issues/17) (merged via PRs #18–#22)
> **Governing ADRs:** [0005](../adr/0005-graph-engine-depends-on-resolver-abstraction.md), [0006](../adr/0006-capability-based-adapters.md), [0007](../adr/0007-one-go-module-per-service-node.md), [0008](../adr/0008-infra-adapters-declare-containerspec.md), [0009](../adr/0009-scaffold-seam-resolves-context-from-config.md)

## Problem Statement

Phase 2's five slices are all merged, but "merged" is not "done." Three gaps between the Phase 2 PRD's intent and the shipped binary mean a user cannot yet trust the things Phase 2 promised:

- **`db:postgres` is invisible to the actual `acthur` binary.** The adapter registers itself in an `init()`, but only the test build imports its package — the CLI entrypoint blank-imports `go:fiber` alone. So `acthur adapter list` shows only `go:fiber`, and `acthur graph validate` on any project that declares a `db:postgres` node reports `unresolved-adapter`. The project's own canonical fixture (`testdata/vetangle`) fails to validate today. Phase 2's headline acceptance — *"graph validate validates adapter names"* — is broken for the one infrastructure adapter the phase delivered.
- **`acthur.yml` is not yet the single source of truth for scaffolding.** `module_prefix` and `identifiers.strategy` exist in config but never reach the adapter — no kernel code maps them onto a `ScaffoldContext`. The only proof that scaffolding honors them is a unit test that *fabricates* the context. Two config fields sit inert, a latent trap for the next contributor who assumes they work.
- **Retired-interface remnants linger.** The monolithic adapter interface was supposed to be retired with no no-op methods, yet `go:fiber` still carries a `GeneratorTargets()` method and a `Dockerfile()` method (plus its `BuildConfig` type) that nothing calls — a third, competing path to an artifact the scaffolded `Dockerfile` already owns, and a bare method the capability model is meant to forbid.

## Solution

Close Phase 2 honestly with three small, independent changes:

- **Wire `db:postgres` into the binary and make registration↔binary drift mechanically impossible to repeat.** Every adapter the repo defines becomes reachable through the assembled CLI resolver, and a completeness guard fails the build if a future adapter falls out of the binary again. The `vetangle` fixture validates clean.
- **Land a tiny `internal/scaffold` kernel seam that resolves a `ScaffoldContext` from config** — implementing the host-less `<project>` module-prefix default — so that `module_prefix` and `identifiers.strategy` provably drive generated output through kernel code, not a hand-built struct. This makes `acthur.yml` the source of truth for scaffolding (PRD Principle 7).
- **Delete the vestigial methods** so the capability model is the whole truth about what an adapter can do.

## User Stories

1. As an Acthur user, I want `acthur adapter list` to show `db:postgres`, so that I can see every adapter the tool actually ships, not just `go:fiber`.
2. As an Acthur user, I want `acthur graph validate` to accept a `db:postgres` node, so that declaring Postgres in `acthur.yml` does not produce a false `unresolved-adapter` error.
3. As an Acthur user, I want the project's own `testdata/vetangle` example to validate cleanly for the adapters Phase 2 ships, so that the canonical example is not self-contradicting.
4. As an Acthur user, I want `acthur adapter inspect db:postgres` to show its `Container` capability, so that I can confirm the adapter is wired and introspectable from the CLI.
5. As a kernel maintainer, I want every adapter package defined in the repo to be reachable through the assembled CLI resolver, so that an adapter cannot be "merged" yet absent from the shipped binary.
6. As a kernel maintainer, I want a mechanical guard that fails when a defined adapter is not reachable through the binary's resolver, so that registration↔binary drift is caught at build time instead of by a confused user.
7. As an Acthur user, I want the `module_prefix` I set in `acthur.yml` to actually appear in a scaffolded service's module path, so that my generated modules carry my org's import path.
8. As an Acthur user, I want an unset `module_prefix` to default to a host-less `<project>` prefix, so that a project scaffolds with a sensible module path without forcing me to configure one.
9. As an Acthur user, I want the `identifiers.strategy` I chose in `acthur.yml` to drive the generated `ids` package, so that the scaffold honors my configuration rather than a default baked into a test.
10. As a kernel maintainer, I want a single pure function that assembles a `ScaffoldContext` from config and a node id, so that the resolution of `ModulePath`/`ProjectName`/`IDStrategy` lives in one testable place and the adapter never computes it (ADR 0007).
11. As a kernel maintainer, I want that resolver to live in its own small package that imports `config` and `adapter`, so that `adapter` and `config` stay dependency leaves and the concern is not mislabeled as dev-runtime (ADR 0009).
12. As a contributor, I want `module_prefix` and `identifiers.strategy` to be live config fields rather than inert ones, so that I do not assume they work and ship a wrong module path.
13. As an adapter author, I want the retired monolithic interface to leave no trace, so that the capability interfaces are the only way an adapter declares what it can do.
14. As an adapter author, I want `go:fiber` to expose only the capabilities it genuinely has (`Scaffold`, `Run`), so that `CapabilitiesOf` cannot be contradicted by stray methods on the concrete type.
15. As a kernel maintainer, I want the unused `Dockerfile()` method and `BuildConfig` type removed, so that there is one owner of the scaffolded `Dockerfile` artifact rather than three competing paths.
16. As a kernel maintainer, I want `GeneratorTargets()` removed now and reintroduced as a proper capability interface when the generator engine lands, so that an inert behavior is modeled as an interface (like `Migratable`/`Deployable`) rather than a bare method.
17. As a contributor reading `plugin.go`, I want comments that reference `GeneratorTargets()` updated, so that documentation does not point at a method that no longer exists.

## Implementation Decisions

**Slice 1 — `db:postgres` reachable + drift guard.**
- The CLI entrypoint blank-imports the `db:postgres` adapter package alongside `go:fiber`, so its `init()` registration runs in the shipped binary. Registration remains via `init()` into the package-level registry assembled at the entrypoint (ADR 0005); no change to the registration mechanism.
- A completeness guard asserts that the set of adapters reachable through the assembled resolver matches the set of adapter packages the repo defines. The guard's intent is to fail when a defined adapter is absent from the binary's resolver — not to enumerate a hardcoded list that itself drifts.
- The `testdata/vetangle` fixture is the integration witness: it must validate without `unresolved-adapter` errors for the adapters Phase 2 ships. (Nodes whose adapters are genuinely out of Phase 2 scope — e.g. `ui:*`, `cache:redis`, `queue:nats`, `storage:minio` — remain legitimately unresolved; the fixture or the test asserts only on the Phase 2 adapters, and this scoping is made explicit so the test is not silently green for the wrong reason.)

**Slice 2 — `internal/scaffold` resolver (ADR 0009).**
- New `internal/scaffold` package with one pure function that takes the loaded config and a node id and returns an `adapter.ScaffoldContext` carrying `ModulePath`, `ProjectName`, and `IDStrategy`.
- Module-path policy (ADR 0007): `ModulePath = <prefix>/<nodeID>`, where `prefix` is `module_prefix` when set, otherwise the bare project name (host-less default).
- `IDStrategy` is taken from `identifiers.strategy`; the adapter continues to select its `ids` template from the context, never reading `acthur.yml` directly.
- The package imports `config` and `adapter` only. `adapter` and `config` remain leaves; neither imports the other. The resolver is not placed in `engine` (the dev/deploy orchestrator) — rejected in ADR 0009.
- The function is not invoked by any CLI command in this phase; it is wired by the first consumer (the dev runtime / `new` wizard) in a later phase. Its value here is that resolution exists in kernel code and is provable.
- Naming: this is *scaffold-context resolution*, a pure assembly function — distinct from the graph engine's `Resolver` (adapter-key resolution), which keeps its CONTEXT.md meaning.

**Slice 3 — delete vestigial methods.**
- Remove `go:fiber`'s `Dockerfile()` method and the `adapter.BuildConfig` type. The scaffolded `Dockerfile` file (rendered from the embedded template in the file set) is unaffected and remains the single owner of that artifact.
- Remove `go:fiber`'s `GeneratorTargets()` method. It is reintroduced as a capability interface (with a matching `Capability` value) when the generator engine is built (Phase 7), not retained as a bare method now. Update the aspirational reference to it in the plugin package's comments.
- After removal, `go:fiber` implements exactly the core interface plus `Scaffolder` and `Runnable`; `CapabilitiesOf(go:fiber)` is unchanged at `{Scaffold, Run}`.

## Testing Decisions

A good test here asserts **external behavior** — what `adapter list`/`graph validate` report, what the resolver returns, what `CapabilitiesOf` reports — never internal structure. Prior art: `internal/graph/graph_test.go` (drives `Build` + `Validate` against config fixtures, asserts on `ValidationError.Rule`/`Severity`) and `internal/adapter/adapter_test.go` (drives the registry, `CapabilitiesOf`, and the `go:fiber` scaffold's `go build` through the package's public API).

- **Registry completeness (new assertion at the `internal/adapter` seam).** Assert that the assembled resolver resolves `db:postgres` and surfaces its category, and that no adapter package defined in the repo is missing from the resolver set. Intent: fail on drift, not enumerate a brittle hardcoded list.
- **Fixture validation (extend the `graph_test.go` style).** Drive `graph.Validate` against `testdata/vetangle/acthur.yml` with the production resolver and assert there is no `unresolved-adapter` error for the Phase 2 adapters (`go:fiber`, `db:postgres`). Same shape as the existing cycle/orphan/category tests.
- **Scaffold-context resolution (new `internal/scaffold` seam).** Pure value assertions on `ResolveScaffoldContext`: `module_prefix` set → `ModulePath` is `<prefix>/<nodeID>`; `module_prefix` unset → `ModulePath` is the host-less `<project>/<nodeID>`; `identifiers.strategy` → `IDStrategy`. No I/O, no disk — the highest possible seam.
- **Capability stability (existing `adapter_test.go` seam).** `CapabilitiesOf(go:fiber)` remains `{Scaffold, Run}` after the method deletions, and the package still compiles — proving the deletions changed no advertised capability.
- **CLI.** At most a light smoke test of `adapter list`/`inspect` showing `db:postgres`; logic lives at the package seams, so no golden-output tests.

## Out of Scope

- Invoking the scaffold resolver or `Scaffold` from a CLI command — the `new` wizard is Phase 9; the dev runtime is Phase 3. The resolver lands and is tested but is wired to a command later.
- Writing scaffolded files to disk.
- Projecting `ContainerSpec` onto `docker run` / compose / k8s — later phases (the spec is declared now, projected later).
- Reintroducing `GeneratorTargets()` as a capability interface — that happens with the generator engine in Phase 7; this PRD only removes the bare method.
- `Migratable` / `Deployable` implementations — interfaces remain defined but inert.
- Any adapter beyond `go:fiber` and `db:postgres`; the rest of the PRD §9 matrix stays unbuilt, and the `vetangle` nodes that reference those adapters remain legitimately unresolved.
- Connection-pool tuning (the `pool:` block) — Phase 6. Cross-node env injection (e.g. `DATABASE_URL`) — Phase 3.

## Further Notes

- Each slice is independently grabbable and can be a separate issue: (1) is actively breaking the `vetangle` fixture and should go first; (2) and (3) have no ordering dependency on each other.
- Deferred design intent unchanged from #17: parse adapter keys into a structured `AdapterRef{Namespace, Name}` so the `kernel:` exemption becomes a field comparison rather than string-prefix matching. Still not this phase.
- ADR 0009 records the placement decision for `internal/scaffold` and the rejected `engine` alternative, so a future reader does not "fix" it by folding the resolver into the dev orchestrator.
- Glossary impact: none. "Resolver" in CONTEXT.md stays scoped to adapter-key resolution by the graph engine; scaffold-context resolution is a separate pure function and introduces no new domain term.
