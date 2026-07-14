# Implementation: Phase 8 — Deploy Runtime

## Goal
`acthur deploy` ships the graph as production: per-adapter Dockerfiles + docker-compose.prod.yml projected from the graph, a pre-deploy gate that refuses to ship a broken system, a compose target (local witness) and a Coolify API target (fake-verified; live VPS pending user infrastructure).

## Owning Docs
- `docs/prd/phase-8-deploy-runtime.md` (spec) · tracker issue #50

## Status
Completed (2026-07-06) — local chain witnessed; live VPS leg pending user Coolify instance (tracked on #50)
- 2026-07-06 — main — PRD + this note created; slices 1 and 3 delegated to Sonnet worktree agents (fallback: inline if session limits kill them again, as happened in Phase 7); slice 2 (gate + deploy command + witness) stays with main.
- 2026-07-06 — subagent slice 3 (#52, Sonnet worktree) — done: `internal/deploy/coolify/` (client.go + target.go), full TDD, `go test ./...` green repo-wide, `gofmt`/`go vet` clean. Client implements EnsureProject, EnsureComposeApp (create-or-update by name match), Deploy, WaitHealthy against Coolify v4's Bearer-auth REST API (`/api/v1/projects`, `/api/v1/applications`, `/api/v1/applications/dockercompose`, `/api/v1/deploy`); every operation tested against an httptest fake (auth header, create-vs-update, deploy trigger, poll-until-healthy, poll-until-failed, timeout, non-2xx body surfaced in error). Assumptions on the create-app body shape and deploy-response shape are pinned in the package doc comment — live-VPS verification still pending per the PRD witness scope. `Target`/`DeployContext` interface defined in `target.go` for slice 2 to wire up (`--target coolify`); did not touch cmd/acthur or internal/deploy root, per instructions.
- 2026-07-06 — subagent slice 1 (#51, Sonnet worktree) — landed. `internal/deploy/artifacts.Project(cfg, g)` emits `deploy/Dockerfile.<nodeID>` per service node whose adapter satisfies the new `adapter.Dockerizable` capability, plus one `deploy/docker-compose.prod.yml`. go:fiber implements `Dockerizable` (`DockerfileFor`): multi-stage `golang:1.22-alpine` builder → non-root `alpine:3.20` runtime, EXPOSE + curl/wget HEALTHCHECK against `/health` derived from the node's port (both omitted for portless nodes, e.g. queue-workers). Compose: build-context services vs. official-image infra (postgres:`<version>` + named volume + pg_isready healthcheck), `depends_on: {condition: service_healthy}` derived from `depends_on` edges (filtered to nodes actually present, so an unimplemented adapter like cache:redis never leaves a dangling reference), one shared `acthur` network, `restart: unless-stopped`, ports published only for nodes with a `proxied_through` edge, env always as `${VAR}` references (no literal secrets anywhere, asserted in tests). Deviation from the PRD's literal capability sketch: an *existing* pinned test (`adapter_test.go`, pre-Phase-8) asserted go:fiber exposes no Dockerfile-shaped capability at all and `CapabilitiesOf` == exactly `{Scaffold, Run}` — reconciled by naming the new method `DockerfileFor` (not the bare `Dockerfile` the old test forbids) and updating that test's expectation to `{Scaffold, Run, Dockerize}` with a comment explaining why. Also extended `generate.WriteFiles`'s routing convention: a `"deploy/"`-prefixed path now lands at the project root exactly like `"migrations/"` already did, since deploy artifacts span the whole graph, not one node. Tested against the real `testdata/vetangle/acthur.yml` fixture (graph.Build succeeds even with adapters not yet implemented — cache:redis, storage:minio, queue:nats, ui:astro, ui:next; Project() skips those nodes rather than erroring). `go test ./...` full suite green, `gofmt`/`go vet` clean on touched files. No Docker daemon required.

- 2026-07-06 — main — Slice 2 + live witness. Gate (build/test per node + every `${VAR}` the compose projection references must resolve; failures aggregated), compose target (up/down/status/WaitHealthy over an injectable docker runner), `acthur deploy --env/--target/--dry-run` chaining gate → artifacts (write engine) → target, coolify wired via slice 3's DeployContext. Witness on a fresh scaffolded api+db project caught three real bugs, each fixed with a pinned test: (1) compose resolves `build.context` relative to the compose file's directory — `./api` pointed at `deploy/api` (now `../api`); (2) the Dockerfile built `./...` into a single `-o` output — fails the moment the module has >1 package (now builds the root main package); (3) healthchecks probed `localhost`, which alpine resolves to ::1 while the app listens IPv4-only — permanently-unhealthy running container (now 127.0.0.1, in both the Dockerfile template and the compose projection). Host gap: docker compose + buildx CLI plugins were absent — installed to /usr/local/lib/docker/cli-plugins. After fixes: gate ✓, images built, `stack is healthy`, api 200 from the host on the published port, db unpublished (internal network only), clean `compose down`. Full suite green.

## Current Decisions
- Deploy artifacts flow through `[]plugin.GeneratedFile` + the Phase 7 write engine — generated.lock protects user-edited Dockerfiles.
- The witness scope on this host is the local chain (images build, production stack healthy on local Docker; Coolify client against a recorded fake). The VPS leg is tracked as pending user infrastructure, not silently claimed.

## Open Questions
- Coolify API surface to pin (v4 endpoints) — slice 3 records what it implements.

## Files/Modules Expected
`internal/deploy/` (gate, context, compose target), `internal/deploy/artifacts/`, `internal/deploy/coolify/`, `internal/adapter/*` (Dockerfile capability), `cmd/acthur`.

## Acceptance Criteria
- [x] `acthur deploy --env production` (compose target) on the witness project: gate passes, images build, stack healthy on local Docker, teardown clean
- [x] Gate fails with pointed messages on: broken build, failing test, missing env var, invalid graph
- [x] Dockerfile + compose projection written through the write engine (lock semantics observed)
- [x] Coolify client behavior verified against a fake API (ensure app, push compose, trigger, poll)
- [x] `--dry-run` prints the plan without side effects; full suite green
