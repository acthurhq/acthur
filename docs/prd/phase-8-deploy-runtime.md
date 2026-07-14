# PRD — Phase 8: Deploy Runtime

> **Status:** ready-for-agent · **Phase:** 8 of the implementation plan (PRD §21)
> **Governing ADRs:** 0005 (CLI-assembly injection) · graph/adapters from Phases 2–3
> **Implementation note:** `docs/implementation/active/0006-phase-8-deploy-runtime.md`

## Problem Statement

`acthur dev` runs the graph live; nothing ships it. `acthur deploy` and `acthur build` are stubs. Phase 8 builds the deploy runtime: the same graph executed with a production strategy — per-adapter Dockerfiles, a `docker-compose.prod.yml` projected from the graph, a pre-deploy gate that refuses to ship a broken system, a Coolify API target, and multi-environment support.

**Done when (PRD §21):** `acthur deploy` takes a project to running production on a VPS in one command.
**Witnessable here:** this host has Docker but no VPS/Coolify instance, so the live witness proves the local end of the chain — `acthur deploy --env production` with the compose target builds real images and brings up the production stack on the local Docker daemon (health-checked), and the Coolify client is verified against a recorded fake API. The VPS leg is recorded as pending user infrastructure in the tracker.

## Shared conventions (all slices)

- Deploy artifacts are generated files: reuse `[]plugin.GeneratedFile` + `internal/generate.WriteFiles` (generated.lock semantics apply — a user-edited Dockerfile is skipped with a warning).
- The deploy context consumes the same sealed `*graph.Graph` the dev engine uses; no parallel model. Environment config comes from `cfg.Environments[env]` (`environments:` in acthur.yml — context/target/host already parse).
- Production env vars follow the dev engine's Connectable convention (DATABASE_URL et al.), with values sourced from the environment/secrets — never hardcoded into images or compose files (compose references `${VAR}`).
- TDD, behavior-first; no Docker daemon required in unit tests (projection/gate/client seams testable in-memory; the daemon is witness territory).
- **Before committing**: append your result line under `## Status` in `docs/implementation/active/0006-phase-8-deploy-runtime.md`.

## Slices

### Slice 1 — Dockerfile + docker-compose.prod.yml generators (#51)
`internal/deploy/artifacts/`: per-adapter Dockerfile generation via a new adapter capability — `DockerfileFor(node)` on the gofiber adapter (multi-stage: build with the node's Go version, distroless/alpine runtime, non-root, EXPOSE from node port, HEALTHCHECK from the adapter's health path); infra nodes (db:postgres) need no Dockerfile (official image). `docker-compose.prod.yml` projected from the graph: one service per node (build context for services, image+volume for postgres), `depends_on` with `condition: service_healthy` from graph edges, healthchecks, restart policies, an internal network, ports exposed only where the graph says so, env via `${VAR}` references. Emitted as GeneratedFiles (`deploy/Dockerfile.<node>`, `deploy/docker-compose.prod.yml`) written through the write engine.
**Files:** `internal/deploy/artifacts/`, `internal/adapter/backend/gofiber/` (Dockerfile capability), `internal/adapter/adapter.go` (optional interface).

### Slice 2 — Pre-deploy gate + deploy context + compose target (main agent)
`internal/deploy/`: the gate runs every check with pointed failures (graph validates; contracts load and diff clean against lock if present; `go build ./...` per service node; `go test ./...` per service node; generated artifacts present/fresh; required env vars for the target env resolvable). `acthur deploy --env <env>`: gate → artifacts → target. Compose target: `docker compose -f deploy/docker-compose.prod.yml up -d --build` + health wait + status report; `acthur deploy --dry-run` prints the plan.
**Files:** `internal/deploy/`, `cmd/acthur`.

### Slice 3 — Coolify target (#52)
`internal/deploy/coolify/`: minimal typed client for the Coolify v4 API (token auth via `COOLIFY_TOKEN`, base URL from `environments.<env>.host`): ensure project/application exists, push compose-based deployment, trigger deploy, poll status. All behavior tested against an httptest fake recording requests; a `--target coolify` wiring in the deploy command. Live VPS verification is out of scope for this phase's witness (needs user credentials/instance) — the tracker records it as the remaining leg.
**Files:** `internal/deploy/coolify/`, small wiring in `cmd/acthur`.

## Out of scope
fly.io/railway/render targets (Phase 9), blue-green/rollbacks, image registries (compose builds locally; Coolify builds server-side from pushed compose), TLS/domain automation.
