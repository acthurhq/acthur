> **Issue:** [samueloshio/acthur#22](https://github.com/samueloshio/acthur/issues/22) · **Status:** ready-for-agent · **Type:** AFK

## Parent

[PRD — Phase 2: Adapter System](https://github.com/samueloshio/acthur/issues/17) (#17)

## What to build

Add the `db:postgres` adapter — the first `Containerized` adapter — so a user can declare a Postgres infra node and have the kernel know how to run it.

`db:postgres` implements core + `Containerized` only (no `Scaffolder`/`Runnable`). `Container(ctx)` returns a declarative `ContainerSpec` describing the resource; it is **not** a `docker run` command — the kernel projects the spec onto an execution model in a later phase (ADR 0008).

Postgres spec: image `postgres`, tag from the node's `version:` field (default `16`), port `5432`, a named volume for data persistence, env `POSTGRES_USER`/`POSTGRES_PASSWORD`/`POSTGRES_DB`, and a `pg_isready` healthcheck.

Out of scope here: the `pool:` block (Phase 6) and wiring `DATABASE_URL` into consuming service nodes (Phase 3).

## Acceptance criteria

- [ ] `db:postgres` is registered, with category database, satisfying core + `Containerized` and no other capability.
- [ ] `Container(ctx)` returns a `ContainerSpec` with image `postgres`, port 5432, a named data volume, the Postgres env vars, and a `pg_isready` healthcheck.
- [ ] The image tag comes from the node's `version:` and defaults to `16` when unset.
- [ ] `acthur adapter inspect db:postgres` shows ✓ Container and ✗ for the rest.
- [ ] An `infra` node using `db:postgres` passes `acthur graph validate`.
- [ ] Tests assert the `ContainerSpec` values, including tag defaulting and `version:` override, via the adapter package's public API.

## Blocked by

- [Phase 2 · Slice 3 — capability-described adapters + adapter list/inspect](https://github.com/samueloshio/acthur/issues/19) (#19) — the `Containerized` interface, `ContainerSpec` type, and `Capability` enum are defined there.
