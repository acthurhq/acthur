# Cross-node connection env comes from a `Connectable` capability, not a kernel scheme map

**Status:** accepted

A service node that depends on an infra node needs that resource's connection coordinates — `DATABASE_URL` for `db:postgres`, `REDIS_URL` for `cache:redis`. We add an optional `Connectable` capability — `ConnectionEnv(ctx ContainerContext) map[string]string` — that the provider adapter implements; the kernel type-asserts it on the target of a dependency edge and injects the returned env into the consumer. The URL format and variable name stay owned by the adapter that knows the resource, and are derived from the same declaration the resource runs with.

## Why

This closes the cross-node env injection deferred by [[0008-infra-adapters-declare-containerspec]] and is the gating fact for Phase 3's done-criteria: the scaffolded `go:fiber` app calls `mustGetEnv("DATABASE_URL")` and **panics on boot** if it is unset, so `acthur dev` cannot start a Go Fiber + Postgres project without it.

The credentials, port, and database name must be single-sourced. At decision time the tree held **three divergent Postgres credential sets** — the `ContainerSpec` (`postgres`/`postgres`/`<nodeID>_development`), the engine's hardcoded `buildDockerArgs` (`acthur`/`acthur`/`acthur_dev`), and the fiber `env.example` template (`postgres`/`postgres`/`app_development`). If the consumer's connection string is assembled anywhere other than the provider that also produces its `ContainerSpec`, they drift — which they already had.

## Considered Options

- **Kernel derives the URL from `ContainerSpec` + a category→scheme map (rejected).** Re-introduces a hardcoded `db:postgres → postgres://` adapter map in the engine — the exact anti-pattern [[0008-infra-adapters-declare-containerspec]] removed from `buildDockerArgs`. The kernel would again own per-resource URL formats.
- **Add `Exports map[string]string` to `ContainerSpec` (rejected).** Overloads one struct with two roles — the container's *internal* env and the env *exported* to consumers — and pushes credential templating into a declarative value where a method is clearer.

## Consequences

- A new capability in the [[0006-capability-based-adapters]] set, type-asserted like `Scaffolder`/`Runnable`/`Containerized`. `db:postgres` becomes `Containerized` + `Connectable`; `CapabilitiesOf` reports it.
- The host is `localhost` for now — Phase 3 is the **Local** execution context only ([[0011-local-dev-projects-containerspec-zero-privilege]]). A containerized (`--docker`) context will need the kernel to rewrite the host to the node name; `ConnectionEnv` returns the URL, the kernel owns the host rewrite.
- Injection fires along a consumer's `data_flow` / `depends_on` edges to a `Connectable` target, reusing the existing `buildEnv` edge walk.
- **Out of scope:** disambiguating two databases that both want `DATABASE_URL` (single-DB dev assumption); the `pool:` block (Phase 6).
