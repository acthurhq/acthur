# Implementation: Phase 8 — Deploy Runtime

## Goal
`acthur deploy` ships the graph as production: per-adapter Dockerfiles + docker-compose.prod.yml projected from the graph, a pre-deploy gate that refuses to ship a broken system, a compose target (local witness) and a Coolify API target (fake-verified; live VPS pending user infrastructure).

## Owning Docs
- `docs/prd/phase-8-deploy-runtime.md` (spec) · tracker issue #50

## Status
In-flight
- 2026-07-06 — main — PRD + this note created; slices 1 and 3 delegated to Sonnet worktree agents (fallback: inline if session limits kill them again, as happened in Phase 7); slice 2 (gate + deploy command + witness) stays with main.
- 2026-07-06 — subagent slice 3 (#52, Sonnet worktree) — done: `internal/deploy/coolify/` (client.go + target.go), full TDD, `go test ./...` green repo-wide, `gofmt`/`go vet` clean. Client implements EnsureProject, EnsureComposeApp (create-or-update by name match), Deploy, WaitHealthy against Coolify v4's Bearer-auth REST API (`/api/v1/projects`, `/api/v1/applications`, `/api/v1/applications/dockercompose`, `/api/v1/deploy`); every operation tested against an httptest fake (auth header, create-vs-update, deploy trigger, poll-until-healthy, poll-until-failed, timeout, non-2xx body surfaced in error). Assumptions on the create-app body shape and deploy-response shape are pinned in the package doc comment — live-VPS verification still pending per the PRD witness scope. `Target`/`DeployContext` interface defined in `target.go` for slice 2 to wire up (`--target coolify`); did not touch cmd/acthur or internal/deploy root, per instructions.

## Current Decisions
- Deploy artifacts flow through `[]plugin.GeneratedFile` + the Phase 7 write engine — generated.lock protects user-edited Dockerfiles.
- The witness scope on this host is the local chain (images build, production stack healthy on local Docker; Coolify client against a recorded fake). The VPS leg is tracked as pending user infrastructure, not silently claimed.

## Open Questions
- Coolify API surface to pin (v4 endpoints) — slice 3 records what it implements.

## Files/Modules Expected
`internal/deploy/` (gate, context, compose target), `internal/deploy/artifacts/`, `internal/deploy/coolify/`, `internal/adapter/*` (Dockerfile capability), `cmd/acthur`.

## Acceptance Criteria
- [ ] `acthur deploy --env production` (compose target) on the witness project: gate passes, images build, stack healthy on local Docker, teardown clean
- [ ] Gate fails with pointed messages on: broken build, failing test, missing env var, invalid graph
- [ ] Dockerfile + compose projection written through the write engine (lock semantics observed)
- [ ] Coolify client behavior verified against a fake API (ensure app, push compose, trigger, poll)
- [ ] `--dry-run` prints the plan without side effects; full suite green
