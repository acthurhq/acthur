# Implementation: Phase 3 — Dev Runtime (reconcile-then-finish)

## Goal
`acthur dev` starts the canonical Go Fiber + Postgres project for real: Postgres from its declared `ContainerSpec` gated on `pg_isready`, the Fiber service booting with an injected `DATABASE_URL`, the proxy routing on `localhost:4000`, crashed services restarting, source edits hot-reloading — no `sudo`.

## Owning Docs
- `docs/prd/phase-3-dev-runtime.md` · ADRs 0005, 0006, 0008, 0010, 0011 · tracker #29, witness #33

## Status
Completed (2026-07-05) — closing commit `7abfd84`.
- 2026-06-24 → 2026-07-01 — main — slices 1–3 landed (`f184707`, `b531e07`, `213e5c7`/`93caebb`).
- 2026-07-05 — main — live witness run on real host (Docker + air); five defects found and fixed TDD-first in `7abfd84`; all four pass criteria verified live; #33/#29 closed.

## Current Decisions
- The engine mirrors graph validation's `kernel:*` exemption in `startNode`; kernel-materialized nodes never resolve through the adapter registry.
- Containers are stopped through the runtime (`container.ToStopArgs` → `docker stop`), never by killing the docker-run client.
- Managed processes run in their own process group; stop = SIGTERM to the group, force-kill = SIGKILL to the group; `Restart` reaps the old group first (crash orphans keep ports bound otherwise).
- go:fiber scaffold: `/api/health` served through the proxy's unstripped prefix; air runs with `rerun`/`send_interrupt`.

## Open Questions
None — deferred items recorded in the PRD's Out of Scope (.env overrides, `--docker` context, `.test` DNS, watcher cascade).

## Files/Modules Expected
`internal/engine`, `internal/container`, `internal/process` (+ `_unix`/`_windows`), `internal/adapter/backend/gofiber/templates/*`.

## Acceptance Criteria
- [x] `acthur dev` brings up Postgres → proxy → api, gated on the declared probe, no APP_SECRET panic
- [x] `curl localhost:4000/api/health` healthy through the proxy
- [x] `.go` edit hot-reloads via air
- [x] Ctrl-C tears down in reverse order; container stopped; named volume persisted; no orphans
- [x] (bonus) SIGKILLed air recovers to healthy in ~6s

## Risks
Process-group reaping has a theoretical PID-reuse race (standard supervisor trade-off); air grandchildren live in air's own pgid — covered by air `rerun`, not by group kill.
