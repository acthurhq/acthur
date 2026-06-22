# Capability-based adapters, not a monolithic adapter contract

**Status:** accepted (supersedes the monolithic `Adapter` interface in PRD §9.2)

An adapter implements a minimal core (`Name`, `Category`, `Detect`, `EnvVars`) plus whatever narrow **capability interfaces** match the resource it bridges — `Scaffolder`, `Runnable`, `Containerized`, and later `Migratable`, `Deployable`. The kernel discovers behavior by type-asserting against these interfaces; it never calls a method an adapter cannot honestly implement.

## Why

Adapters bridge many kinds of runtime resource — application frameworks, databases, caches, queues, and external services (Stripe, Temporal, NATS). A single monolithic interface forces every adapter to implement methods that are meaningless for its resource: `db:postgres` would carry no-op `Scaffold`/`BuildCommand`/`TestCommand`. Those no-ops spread through the ecosystem and push "is this *actually* runnable?" checks into every caller.

Capability interfaces are Go's form of Composition over Inheritance (Principle 4). They let the Phase 3 orchestrator branch on *what an adapter can do* — "does it satisfy `Runnable`? `Containerized`?" — rather than on `node.Type` strings, which is the graph-first instinct applied to adapters.

## Consequences

- The monolithic `Adapter` interface (PRD §9.2) is deprecated. `go:fiber` = core + `Scaffolder` + `Runnable`; `db:postgres` = core + `Containerized`.
- A `Capability` enum mirrors the capability interfaces 1:1 and powers tooling (`acthur adapter inspect <name>`). Capabilities are **derived** from interface satisfaction, never hand-declared, so the advertised set cannot lie about behavior (Principle 7, SSOT).
- `Migratable` and `Deployable` may be defined but left inert until their phase ships (Phase 6 / deploy), mirroring the authored-vs-materialized precedent in [[0004-authored-vs-materialized-nodes]].
- The `Resolver` ([[0005-graph-engine-depends-on-resolver-abstraction]]) returns the core adapter; capability discovery is a type assertion at the call site.
