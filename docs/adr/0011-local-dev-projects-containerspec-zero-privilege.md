# Local dev projects ContainerSpec through a pure projector and never requires root

**Status:** accepted

Phase 3's dev runtime targets the **Local** execution context only (native service processes + `docker run` infra on `localhost`). It projects each infra node's `ContainerSpec` onto `docker run` args via a pure `internal/container` projector — deleting the engine's hardcoded `dockerImageFor`/`buildDockerArgs` — and `acthur dev` runs with zero elevated privileges. The `--docker` full-containerization context and the `.test` local-domain DNS strategies are deferred.

## Why

Most of the dev runtime predates the Phase 2 adapter model and bypasses it: the engine carried a hardcoded image map and per-adapter `docker run` flags, and validated the graph with `EmptyResolver` (resolving nothing). Reconciling means the engine consumes the contracts Phase 2 built — the production resolver and the `ContainerSpec` — rather than a parallel hardcoded copy. Putting the projection in a pure `ToRunArgs(spec, nodeID)` function (not inline in the orchestrator, which spawns processes, waits on health, and traps signals) makes it table-testable and gives Phase 8 a home for `ToComposeService`/`ToK8s` beside it — the same reasoning as [[0009-scaffold-seam-resolves-context-from-config]].

Two scoping calls keep the phase honest. **`acthur dev` must not require `sudo`:** the proxy already routes by path prefix on `localhost:4000`, so the PRD's "proxies correctly" is met with no DNS at all. The `.test` per-host domains (`api.<project>.test`) are cosmetic, every `DNSStrategy` (`hosts-file`, `local-dns`) needs root, and baking privileged `/etc/hosts`/dnsmasq mutation into the core dev loop is bad UX and OS-specific fragility. And **Local-only** matches the PRD done-criteria verbatim (Go Fiber native + Postgres container); full `--docker` networking roughly doubles the surface for no Phase 3 deliverable.

## Consequences

- `internal/container` is a pure projector: `ContainerSpec` in, `docker run` args out. The engine resolves the infra adapter, calls `Container(ctx)`, and hands the spec to the projector.
- Projecting the spec faithfully fixes latent bugs for free: the named volume now persists Postgres data (the old `buildDockerArgs` ran ephemeral), and infra readiness uses the spec's declared probe (`pg_isready` via `docker exec`, reusing the unused `ExecStrategy`) instead of the TCP port-open check that reports Postgres "healthy" before it accepts queries.
- To keep `internal/health` a leaf, the engine builds the readiness probe from the spec and hands it to the checker; `health` does not import `adapter`.
- `NewDevEngine` takes the assembled resolver ([[0005-graph-engine-depends-on-resolver-abstraction]]) and uses it for both validation and instance lookup — `EmptyResolver` and the global `adapter.Resolve` leave the dev path.
- **Deferred (tracked, not built):** `--docker` execution context; the `.test` DNS strategies as an opt-in `acthur dns setup`; graph-aware hot-reload cascade (Phase 4 — see CONTEXT "Execution context"). Phase 3 hot reload is adapter-native (`air`).
