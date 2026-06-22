# Infra adapters declare a ContainerSpec, the kernel projects it per context

**Status:** accepted

The `Containerized` capability returns a declarative `ContainerSpec` — `{ Image, Tag, Ports, Volumes, Env, Healthcheck, Cmd }` — describing *what the resource is*. It never returns a `docker run` command. The execution context projects that one spec onto the target runtime: `docker run` (Local), a compose service (Docker), or a k8s/Coolify manifest (Cloud).

## Why

The deploy philosophy is that compose, k8s, and Coolify configs are all *projections of the graph onto an execution model*, never hand-written. That principle has to reach into the adapter layer or it breaks at the first infra node. If `Containerized` returned a `docker run` command, it would hardcode both *docker* and *run*, forcing the same facts (image, ports, volumes, healthcheck) to be re-derived for every other context. A declarative spec is stated once and projected everywhere.

It also unifies the health gate: a node's healthcheck (e.g. `pg_isready`) is declared once in the spec and feeds the `depends_on` "wait until healthy" rule identically across all three contexts.

## Consequences

- This is the `Containerized` capability from [[0006-capability-based-adapters]]; `db:postgres` = core + `Containerized` and implements no `Scaffolder`/`Runnable` methods.
- The container image tag comes from the node's `version:` in `acthur.yml`, resolved by the kernel onto the adapter's context (default `16` for Postgres).
- **Deferred, not part of this capability:** the `pool:` block is the *service's* connection-pool concern (Phase 6), and wiring the resulting `DATABASE_URL` into consuming service nodes is cross-node env injection (Phase 3). `Containerized` describes the container and nothing else.
