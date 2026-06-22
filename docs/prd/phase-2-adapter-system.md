# PRD — Phase 2: Adapter System

> **Status:** ready-for-agent · **Tracker:** [samueloshio/acthur#17](https://github.com/samueloshio/acthur/issues/17)
> **Phase:** 2 of the implementation plan (PRD §21) · **Depends on:** Phase 1 (Graph Engine), shipped.
> **Governing ADRs:** [0005](../adr/0005-graph-engine-depends-on-resolver-abstraction.md), [0006](../adr/0006-capability-based-adapters.md), [0007](../adr/0007-one-go-module-per-service-node.md), [0008](../adr/0008-infra-adapters-declare-containerspec.md)

## Problem Statement

Today an Acthur user can describe their system in `acthur.yml` and the kernel will build, validate, and traverse the graph — but the `adapter:` field on every node is just an unchecked string. A typo (`go:fbier`), a nonsensical pairing (`api: { type: service, adapter: db:postgres }`), or a reference to an adapter that doesn't exist all pass validation silently and only fail much later, if at all. There is no way to ask the tool *what adapters exist* or *what each one can do*.

Worse, the only adapter that exists (`go:fiber`) is pre-Phase-1 skeleton code: its scaffold output **does not compile** (the `go.mod` module path disagrees with the import paths in the generated source), its ID-strategy selection is a hardcoded stub that ignores `acthur.yml`, and it carries a monolithic interface that cannot honestly represent an infrastructure resource like Postgres. There is no `db:postgres` adapter at all.

A user cannot trust `acthur graph validate` to catch adapter mistakes, and cannot trust a scaffold to produce code that builds.

## Solution

Phase 2 makes adapters first-class, capability-described, and verifiable:

- **`acthur graph validate` catches adapter mistakes** — unknown adapter keys and node-type/adapter-category mismatches surface as validation errors with the same severity model as cycles and orphans, before any process runs. Kernel primitives (`kernel:*`) are correctly exempt.
- **The graph engine never learns *how* adapters are discovered** — it depends on an injected `Resolver` abstraction, so the discovery mechanism (a registry today, plugin loaders or remote catalogs later) can evolve without touching Ring 0.
- **Adapters expose only the capabilities their resource genuinely has** — `go:fiber` can scaffold and run; `db:postgres` declares a container. No no-op methods. `acthur adapter list` and `acthur adapter inspect <key>` let a user *see* the registry and each adapter's capabilities.
- **`go:fiber` scaffolds code that compiles** — one kernel-resolved module path feeds both `go.mod` and every import, and the chosen identifier strategy from `acthur.yml` actually drives the generated `ids` package.
- **`db:postgres` exists** — it declares a `ContainerSpec` (image, tag from the node's `version:`, port, volume, healthcheck) that the kernel will project onto an execution model in later phases.

## User Stories

1. As an Acthur user, I want `acthur graph validate` to reject an adapter key that isn't registered, so that a typo in `acthur.yml` fails immediately with a clear message instead of surfacing as a confusing runtime failure.
2. As an Acthur user, I want a misspelled adapter error to tell me which adapters *are* available, so that I can correct it without leaving the terminal.
3. As an Acthur user, I want `acthur graph validate` to reject a `service` node that points at an infra-category adapter (and vice versa), so that structurally nonsensical topologies are caught at compile time (Principle 3).
4. As an Acthur user, I want the kernel `proxy` node to never be flagged as an unknown adapter, so that validation doesn't complain about a node I didn't even author.
5. As an Acthur user, I want to run `acthur adapter list` and see every registered adapter grouped by category, so that I know what I can put in `acthur.yml`.
6. As an Acthur user, I want to run `acthur adapter inspect go:fiber` and see a ✓/✗ table of its capabilities, so that I understand what the adapter can and cannot do before I rely on it.
7. As an Acthur user, I want the inspect table to also show capabilities that aren't built yet (e.g. `Deploy`) as `✗`, so that I can see the roadmap rather than guess at it.
8. As an Acthur user, I want the capability table to reflect what the adapter *actually does*, so that it can never advertise a capability it doesn't have.
9. As an Acthur user, I want `acthur new`-style scaffolding of a `go:fiber` service to produce a project that compiles with `go build` on the first try, so that I'm not debugging generated code before I've written a line of my own.
10. As an Acthur user, I want the generated Go module path to be consistent across `go.mod` and every import, so that the project builds regardless of what I named it.
11. As an Acthur user, I want each service node scaffolded as its own self-contained Go module, so that I can later move it to its own repository (cross-repo node) without restructuring.
12. As an Acthur user, I want to set a `module_prefix` in `acthur.yml` and have it reflected in generated module paths, so that my modules carry my org's import path.
13. As an Acthur user, I want the identifier strategy I chose in `acthur.yml` (`ulid`, `uuid-v4`, …) to actually drive the generated `ids` package, so that the scaffold honors my configuration instead of a hardcoded default.
14. As an Acthur user, I want a `db:postgres` adapter, so that I can declare a Postgres infra node and have the kernel know how to run it.
15. As an Acthur user, I want `db:postgres` to take its image tag from the node's `version:` field, so that `version: 16` actually pins Postgres 16.
16. As an Acthur user, I want `db:postgres` to declare a healthcheck, so that `depends_on` "wait until healthy" works the same way no matter where it runs.
17. As an adapter author, I want to implement only the capability interfaces my resource needs, so that I'm not forced to write meaningless no-op methods.
18. As an adapter author, I want my capabilities to be derived from the interfaces I implement, so that I can't accidentally advertise a capability I forgot to build.
19. As a kernel maintainer, I want the graph engine to depend on a `Resolver` interface rather than the adapter registry, so that adding future discovery mechanisms never forces a refactor of Ring 0.
20. As a kernel maintainer, I want `Validate` to require a non-nil resolver, so that no caller can quietly skip adapter checks or fall back to a hidden global.
21. As a kernel maintainer, I want adapters to register themselves into a registry assembled at the CLI entrypoint, so that the wiring is explicit and the engine stays free of `init()`-order coupling.
22. As a test author, I want to validate structural rules with an explicit empty resolver, so that "this test has no adapter knowledge" is a visible choice, not a hidden default.
23. As a contributor, I want adapter templates embedded in the adapter's own package, so that adding a new adapter is purely additive and never edits the kernel.

## Implementation Decisions

**Resolver abstraction (ADR 0005).**
- The `graph` package defines and owns a `Resolver` interface (consumer-defined). It returns a minimal `ResolvedAdapter` exposing `Name()` and `Category()` — enough for both new rules, and no more. `graph` does **not** import the `adapter` package.
- `graph.Validate` takes a `Resolver` argument: `Validate(resolver) []ValidationError`. `Build(cfg)` stays resolver-free — an unknown adapter is a *validation* fault on a constructable graph, not a *build* fault (consistent with the three-tier model, ADR 0002).
- The resolver is **required and non-nil**. There is no `Validate()` overload and no `nil`-consults-a-global path. Callers with no adapter knowledge pass an explicit empty resolver.
- The production resolver is the adapter registry, wrapped to satisfy `graph.Resolver`, assembled and injected at the CLI entrypoint.

**Two new validation rules (in `graph.Validate`).**
- *adapter-resolves*: every node's adapter key must resolve via the resolver. On failure, a `ValidationError{Rule: "unresolved-adapter", Severity: error}` whose message lists available adapters.
- *type↔category-compatible*: a `service` node's adapter must be a service-shaped category (backend/frontend/mobile/desktop); an `infra` node's adapter must be an infra-shaped category (database/cache/storage/queue). Mismatch → `ValidationError{Rule: "adapter-category-mismatch", Severity: error}`.
- Both rules **skip any adapter key in the `kernel:` namespace** — kernel primitives are provided, not resolved (ADR re: kernel primitives; `CONTEXT.md`).

**Capability-based adapters (ADR 0006).**
- Core interface every adapter implements: `Name() string`, `Category() Category`, `Detect(dir) bool`, `EnvVars() []EnvVar`.
- Capability interfaces, type-asserted by the kernel:
  - `Scaffolder` → `Scaffold(ScaffoldContext) ([]File, error)`
  - `Runnable` → `DevCommand` / `BuildCommand` / `TestCommand`
  - `Containerized` → `Container(ctx) ContainerSpec`
  - `Migratable`, `Deployable` — **defined but inert** this phase (no implementations), mirroring the authored-vs-materialized "defined but inert" precedent (ADR 0004).
- A `Capability` enum mirrors the interfaces 1:1 (`Scaffold`, `Run`, `Container`, `Migrate`, `Deploy`).
- `CapabilitiesOf(a) []Capability` is a free function that derives the set via type assertions — the single source of truth. There is **no** hand-written `Capabilities()` method.
- The monolithic `Adapter` interface (PRD §9.2) is retired.

**`go:fiber` (core + `Scaffolder` + `Runnable`).**
- Templates move from inline Go string literals to files embedded in the adapter package via `go:embed` + `text/template`, executed through a small shared kernel-side template helper.
- `ScaffoldContext` is extended to carry kernel-resolved facts: `ModulePath`, `ProjectName`, `IDStrategy`. Adapters render these (`{{.ModulePath}}`, etc.) and never read `acthur.yml` directly.
- The hardcoded `detectIDStrategy()` stub is deleted; `IDStrategy` comes from `identifiers.strategy`.

**Module-path policy (ADR 0007).**
- One self-contained Go module per service node, rooted at the node's `source` dir.
- Module path = `<module_prefix>/<node-id>`. `acthur.yml` gains an optional top-level `module_prefix`; unset defaults to a host-less `<project>` prefix. The kernel resolves the final `ModulePath` onto `ScaffoldContext`; the adapter never computes it.
- Cross-service code sharing is via generation, not relative imports or a shared local package.

**`db:postgres` (core + `Containerized`) (ADR 0008).**
- `Container(ctx)` returns a declarative `ContainerSpec { Image, Tag, Ports, Volumes, Env, Healthcheck, Cmd }`. It is **not** a `docker run` command — the kernel projects the spec per execution context in later phases.
- Postgres spec: image `postgres`, tag from node `version:` (default `16`), port `5432`, a named volume for data, env `POSTGRES_USER`/`POSTGRES_PASSWORD`/`POSTGRES_DB`, healthcheck `pg_isready`.

**CLI — read-only introspection only.**
- New `adapter` command group, sibling to `graph`:
  - `acthur adapter list` — all registered adapters grouped by `Category()`.
  - `acthur adapter inspect <key>` — name, category, and the ✓/✗ `Capability` table (full set, including inert `Migrate`/`Deploy`), straight from `CapabilitiesOf`.
- The commands are thin projections of the registry + `CapabilitiesOf`. **No** `install` / `remove` / `search` / `update` — adapters are not a package-manager subsystem in this phase.

## Testing Decisions

A good test here asserts **external behavior** — what `validate` reports, what `inspect` shows, whether the scaffold compiles — never internal structure. Prior art: `internal/graph/graph_test.go` (drives `Build` + `Validate` against config fixtures, asserts on `ValidationError.Rule`/`Severity`) and `internal/adapter/adapter_test.go` (drives the registry and `go:fiber` through the package's public API).

- **Graph validation (`graph_test.go` seam, extended).** Drive `graph.Validate(resolver)` with a **controlled fake resolver** (map-backed; never `nil`). Assert: unknown adapter key → `ValidationError` with `Rule="unresolved-adapter"`; `service`-on-infra-adapter → `Rule="adapter-category-mismatch"`; `kernel:proxy` is never flagged by either rule. Same style as the existing cycle/orphan tests.
- **Adapter package API (`adapter_test.go` seam).** Assert `CapabilitiesOf(go:fiber)` == {Scaffold, Run} and `CapabilitiesOf(db:postgres)` == {Container}, with inert ones absent. Assert the registry-as-`graph.Resolver` wrapper resolves known keys and surfaces category.
- **Scaffold correctness (`Scaffolder.Scaffold` seam).** The strongest assertion: render `go:fiber` into a temp dir and run `go build` — compilation *is* the external contract and directly kills the module-path bug. Toolchain-free companions: `go.mod` module path equals the import prefix in every generated `.go` file; the emitted `ids` package matches the `IDStrategy` in the context.
- **Container shape (`Containerized.Container` seam).** Pure value assertion on the `ContainerSpec` for `db:postgres` (image/tag/port/volume/healthcheck), including tag defaulting and `version:` override.
- **CLI.** Logic lives at the adapter-package seam; the `adapter list/inspect` commands get at most a light smoke test, not golden-output tests.

## Out of Scope

- Connection-pool tuning (the `pool:` block) — Phase 6.
- Cross-node env injection (wiring `DATABASE_URL` from `db:postgres` into consuming service nodes) — Phase 3.
- Actually *running* anything: process management, container execution, the dev orchestrator, the proxy — Phase 3.
- Projecting `ContainerSpec` onto `docker run` / compose / k8s — later phases (the spec is declared now, projected later).
- Any adapter beyond `go:fiber` and `db:postgres` (the full matrix in PRD §9 stays unbuilt).
- `Migratable` / `Deployable` implementations — interfaces defined but inert.
- An adapter package-manager subsystem (`install`/`search`/`update`).
- `acthur init` detection flows (the `Detect` method exists on the core interface but its consumer is a later phase).

## Further Notes

- The existing `internal/adapter/adapter.go` and `internal/adapter/backend/gofiber/gofiber.go` are pre-Phase-1 skeleton and are **rebuilt** to this design, not extended — the monolithic interface and inline-string scaffold are removed.
- Deferred design intent: parse adapter keys into a structured `AdapterRef{Namespace, Name}` so the `kernel:` exemption becomes a field comparison instead of string-prefix matching. Explicitly *not* this phase; string-prefix matching is the accepted stand-in.
- Glossary terms introduced for this work live in `CONTEXT.md`: *Adapter*, *Adapter key*, *Kernel primitive*, *Capability*, *Resolver*.
- This PRD is intended to be sliced into vertical, independently-grabbable issues via `/to-issues`.
