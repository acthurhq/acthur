# PRD — Phase 3: Dev Runtime (reconcile-then-finish)

> **Status:** ready-for-agent · **Tracker:** [samueloshio/acthur#29](https://github.com/samueloshio/acthur/issues/29) · **Phase:** 3 of the implementation plan (PRD §21)
> **Governing ADRs:** [0005](../adr/0005-graph-engine-depends-on-resolver-abstraction.md), [0006](../adr/0006-capability-based-adapters.md), [0008](../adr/0008-infra-adapters-declare-containerspec.md), [0010](../adr/0010-connectable-capability-for-cross-node-connection-env.md), [0011](../adr/0011-local-dev-projects-containerspec-zero-privilege.md)

## Problem Statement

`acthur dev` is supposed to start a real project — manage processes, run infra, proxy traffic, restart crashed services, hot reload on change. Most of the runtime machinery exists (process manager, health checker, proxy, output router), but it was written *before* the Phase 2 adapter model and bypasses the contracts that model introduced. Concretely, a user who runs `acthur dev` today on the canonical Go Fiber + Postgres project hits three walls:

- **The dev runtime doesn't validate against the real adapter set.** The engine validates the graph with an empty resolver, so adapter problems that `acthur graph validate` would catch slip through `acthur dev` silently.
- **Infra is run from a hardcoded copy, not the adapter's declaration.** The engine carries its own hardcoded Docker image map and per-adapter `docker run` flags that ignore the `ContainerSpec` the `db:postgres` adapter declares. The two have already drifted — there are three different Postgres credential sets in the tree — and a user gets whichever the engine hardcoded, not what the adapter (and the rest of the system) believes.
- **The service can't reach the database, and in fact won't even boot.** Nothing injects a connection string from an infra node into the service that depends on it. The scaffolded Go Fiber app *requires* `DATABASE_URL` and panics on startup when it is absent — so `acthur dev` cannot bring up the very project the phase is defined by.

On top of that, two promised conveniences are half-built: infra is declared "healthy" the moment its port opens (before Postgres can actually accept queries), and the `*.test` vanity domains are printed but never resolve.

## Solution

Close Phase 3 by making the dev runtime *consume the Phase 2 adapter contracts* instead of a parallel hardcoded copy, then finishing the genuinely-missing readiness behavior — all within the **Local** execution context and without ever requiring root.

- **Reconcile the engine with Phase 2.** `acthur dev` validates with the same production resolver the CLI uses, resolves each infra node's `ContainerSpec`, and projects it onto `docker run` through a pure projector — deleting the engine's hardcoded image map and per-adapter flag switch.
- **Make a service reach its dependencies.** A new `Connectable` capability lets an infra adapter export the connection env (`DATABASE_URL`, `REDIS_URL`) a consumer needs, derived from the same declaration the container runs with, so the credentials a service receives can never drift from the resource. The engine injects this along dependency edges, and the Go Fiber + Postgres project boots and connects.
- **Tell the truth about readiness.** Infra readiness runs the probe the adapter declared (`pg_isready`) rather than a bare TCP port check, so a service does not start against a database that is not yet accepting connections.
- **Keep `acthur dev` privilege-free.** The proxy already routes by path on `localhost:4000`; the `.test` DNS strategies and full `--docker` containerization are deferred to later, opt-in work.

**Done when:** `acthur dev` starts the Go Fiber + Postgres project — Postgres runs from its declared `ContainerSpec` and is gated on `pg_isready`, the Fiber service boots with an injected `DATABASE_URL`, the proxy routes correctly on `localhost:4000`, a crashed service restarts, and source changes hot-reload via the adapter's own tooling — with no `sudo`.

## User Stories

1. As an Acthur user, I want `acthur dev` to validate my graph with the same adapter set the rest of the CLI uses, so that a problem `acthur graph validate` would catch does not slip silently through `acthur dev`.
2. As an Acthur user, I want `acthur dev` to reject an unknown adapter before it starts anything, so that I fail fast with a clear message instead of mid-startup.
3. As an Acthur user, I want my `db:postgres` node run from the adapter's declared `ContainerSpec`, so that the image, tag, ports, volumes, env, and healthcheck match what the rest of Acthur believes about that resource.
4. As an Acthur user, I want the container's named volume honored, so that my local Postgres data survives a restart of `acthur dev` instead of vanishing.
5. As an Acthur user, I want the Postgres credentials and database name to come from a single source, so that the value a service connects with always matches the value the container was started with.
6. As a kernel maintainer, I want the engine's hardcoded Docker image map and per-adapter `docker run` flags deleted, so that adapter-specific runtime knowledge lives in the adapter, not the orchestrator.
7. As a kernel maintainer, I want the `ContainerSpec` → `docker run` projection to be a pure, table-tested function, so that I can verify it without spawning processes and reuse the same spec for compose/k8s projection in Phase 8.
8. As an Acthur user, I want a service that depends on `db:postgres` to receive a working `DATABASE_URL`, so that my Go Fiber app boots instead of panicking on a missing required variable.
9. As an Acthur user, I want a service that depends on `cache:redis` to receive a `REDIS_URL`, so that cache-backed code has its connection string without my wiring it by hand.
10. As an adapter author, I want to declare the connection env my resource exports through a `Connectable` capability, so that the URL format and variable name stay owned by the adapter that understands the resource.
11. As a kernel maintainer, I want `Connectable` to be an optional capability type-asserted like the others, so that an adapter advertises connection-export only when it genuinely provides a connection.
12. As an Acthur user, I want `acthur adapter inspect db:postgres` to show its `Connectable` capability, so that I can confirm from the CLI that it exports a connection.
13. As an Acthur user, I want connection env injected only into the consumers that actually depend on a resource, so that an unrelated service is not handed a database URL it never asked for.
14. As an Acthur user, I want infra declared healthy only when it can truly serve, so that my service does not start against a Postgres that has not finished accepting connections.
15. As an Acthur user, I want infra readiness to run the probe the adapter declared (`pg_isready`), so that "healthy" means query-ready, not merely port-open.
16. As an Acthur user, I want a sensible TCP fallback when an infra adapter declares no healthcheck, so that readiness still works for resources without a probe.
17. As an Acthur user, I want a crashed service restarted automatically with backoff, so that a transient failure during development does not leave my environment half-down.
18. As an Acthur user, I want the dev proxy to route all my services under `localhost:4000` by path, so that I have one stable entry point without configuring anything.
19. As an Acthur user, I want unified, per-service prefixed log output, so that I can read interleaved service logs and tell which service each line came from.
20. As an Acthur user, I want source changes to hot-reload the affected service, so that I see my edits without restarting `acthur dev`.
21. As an Acthur user, I want `acthur dev` to run without ever asking for `sudo`, so that starting my environment never depends on privileged DNS or `/etc/hosts` changes.
22. As an Acthur user, I want a clear "ready" summary listing my proxy and service URLs, so that I know where to point my browser once everything is up.
23. As an Acthur user, I want graceful shutdown on Ctrl-C that stops services in reverse order, so that I do not leave orphaned containers or processes behind.
24. As a kernel maintainer, I want connection-env assembly extracted into a pure engine-local helper, so that cross-node injection is unit-tested without binding the test to the full engine lifecycle.
25. As a contributor, I want the deferred work (`--docker` context, `.test` DNS, graph-aware hot-reload cascade) recorded, so that no one assumes it was forgotten rather than scoped out.

## Implementation Decisions

Framing is **reconcile-then-finish** (ADR 0011): make the existing dev runtime honest against the Phase 2 contracts, then finish the missing readiness behavior. Scope is the **Local** execution context only — native service processes plus `docker run` infra on `localhost`.

**Slice 1 — Reconcile the engine: real resolver + `ContainerSpec` projection.**
- `acthur dev` is constructed with the assembled production resolver (ADR 0005) and uses it for both graph validation and adapter-instance lookup. The empty resolver and the global registry reach-through leave the dev path.
- A new `internal/container` package holds a pure projector: `ContainerSpec` in, `docker run` arguments out (ADR 0011, mirroring the `internal/scaffold` precedent of ADR 0009). The engine resolves each infra node's adapter, type-asserts `Containerized`, calls `Container(ctx)`, and hands the spec to the projector.
- The engine's hardcoded image map and per-adapter `docker run` flag switch are deleted. Projecting the spec faithfully restores the declared named volume (persistent data) where the old path ran ephemeral.

**Slice 2 — `Connectable` capability + cross-node connection-env injection (ADR 0010).**
- A new optional capability: `Connectable` with `ConnectionEnv(ctx) map[string]string`, type-asserted by the kernel like `Scaffolder`/`Runnable`/`Containerized` (ADR 0006). `db:postgres` implements it, returning `DATABASE_URL` derived from the same facts as its `ContainerSpec` (user, password, port, database name); host is `localhost` for the Local context.
- The engine injects a dependency target's `ConnectionEnv` into the consumer's environment along its `data_flow` / `depends_on` edges, reusing the existing edge walk. Connection-env assembly is extracted into a pure engine-local helper (`resolveNodeEnv`) that takes graph + resolver + node and returns the env map; the runtime `buildEnv` becomes glue that calls it.
- `CapabilitiesOf(db:postgres)` now reports `Container` + `Connectable`; `acthur adapter inspect` shows it. `go:fiber` does not satisfy `Connectable`.

**Slice 3 — Honest infra readiness from the declared probe.**
- Infra readiness derives its strategy from the resolved `ContainerSpec.Healthcheck`: when a probe is declared, run it inside the container via `docker exec` using the existing (currently unused) `ExecStrategy`; fall back to the TCP strategy when no healthcheck is declared. This replaces the stub that returned TCP for every infra adapter.
- To keep `internal/health` a dependency leaf, the engine builds the readiness probe from the spec and hands it to the checker; `health` does not import `adapter`.

**Slice 4 — End-to-end `acthur dev` witness.**
- The canonical Go Fiber + Postgres project comes up under `acthur dev`: Postgres from its `ContainerSpec`, gated on `pg_isready`; the Fiber service booting with an injected `DATABASE_URL`; the proxy routing on `localhost:4000`; a crashed service restarting; source edits hot-reloading via the adapter's own `air`. No `sudo`.

## Testing Decisions

A good test here asserts **external behavior** — the args the projector emits, the env a node resolves, the capability an adapter advertises, the strategy chosen for a node — never internal structure. Prefer the highest seam; the engine (which spawns processes) is the worst place to assert logic, so logic is pushed out to pure units and the engine keeps only thin glue. Prior art: `internal/scaffold/scaffold_test.go` (pure value assertions), `internal/adapter/adapter_test.go` (`CapabilitiesOf` / `Satisfies*` behavior tests), `internal/health/health_test.go` (per-strategy tests).

- **Projector (`internal/container`, new pure seam).** Table tests on `ToRunArgs`: a `ContainerSpec` projects to the expected image:tag, `-p` port mappings, `-v` named-volume mounts, `-e` env, and container name. Same shape as `scaffold_test.go` — value in, value out, no I/O.
- **`Connectable` (`internal/adapter`, existing seam).** Assert `CapabilitiesOf(db:postgres)` includes `Connectable`; `ConnectionEnv(ctx)` returns a `DATABASE_URL` whose user/password/port/database match the adapter's own `ContainerSpec`-derived facts; `go:fiber` does **not** satisfy `Connectable`. Mirrors the existing `SatisfiesRunnable` / `DoesNotSatisfyContainerized` tests.
- **Cross-node injection (`internal/engine`, new pure helper seam).** Unit-test `resolveNodeEnv` directly over a fixture graph + resolver: a consumer with a `depends_on`/`data_flow` edge to a `Connectable` target gets that target's connection env; an unrelated node does not. No process is spawned and the test does not drive the engine lifecycle.
- **Readiness strategy (`internal/health`, existing seam).** Assert the strategy chosen for a containerized infra node with a declared healthcheck is the exec probe (and that it shells the declared command), and that a node without a healthcheck falls back to TCP. Mirrors the existing strategy-selection and `HTTPStrategy`/`TCPStrategy` tests.
- **End-to-end (`acthur dev`).** A scripted smoke of the done-when criteria — not a golden-output unit test. Logic correctness lives at the seams above; the smoke proves the wiring.

## Out of Scope

- **Full `--docker` execution context** — services containerized on a shared network with host-name service discovery. Local only this phase (ADR 0011); the flag becomes "not yet implemented."
- **`.test` local-domain DNS** — the `hosts-file` / `local-dns` strategies and any `/etc/hosts` or dnsmasq mutation. Deferred to an opt-in `acthur dns setup`; the proxy routes by path on `localhost:4000` without it.
- **Graph-aware hot-reload cascade** — the `internal/watcher` package stays dormant (but tested); Phase 3 hot reload is adapter-native (`air`). Wiring the watcher waits for contract-aware cascade in Phase 4.
- **Connection-pool tuning** (the `pool:` block) — Phase 6.
- **Contract enforcement on `data_flow` edges** — Phase 4.
- **Disambiguating multiple databases that both want `DATABASE_URL`** — single-DB dev assumption (ADR 0010); multi-provider naming is later.
- **Any adapter beyond `go:fiber` and `db:postgres`** for the end-to-end witness; other infra adapters gain `Connectable`/healthchecks as they are built.

## Further Notes

- Each slice is independently grabbable. Slice 1 (resolver + projector) and Slice 3 (readiness) are the reconcile core; Slice 2 (`Connectable`) is what makes the Go Fiber + Postgres project actually boot and is the gating item for the done-when. Slice 4 is the integration witness and depends on 1–3.
- The three divergent Postgres credential sets in the tree at planning time (the `ContainerSpec`, the engine's `buildDockerArgs`, and the fiber `env.example` template) collapse to one once Slice 1 deletes the hardcoded path and Slice 2 derives the URL from the same spec — this single-sourcing is the point of ADR 0010, not a side effect.
- Supervision (restart-on-crash with backoff) is already wired through the process manager; Slice 4 verifies it rather than building it.
- Glossary: this phase introduced **Connection env**, **Execution context**, and **Projection** in `CONTEXT.md`; the PRD uses them as defined there.
