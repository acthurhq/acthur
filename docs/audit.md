# Acthur PRD Implementation Audit

Date: 2026-07-06

This handoff captures an end-to-end audit of the current implementation against
`docs/acthur-prd.md`. It intentionally does not rely on notes under
`docs/implementation/`; it compares the PRD phase definitions to the current
source, command wiring, tests, and smoke checks.

No implementation files were edited during the audit.

## PRD Phase Count

`docs/acthur-prd.md` defines 10 implementation phases total:

- Phase 0: CLI Skeleton
- Phase 1: Graph Engine
- Phase 2: Adapter System
- Phase 3: Dev Runtime
- Phase 4: Contract Engine
- Phase 5: Plugin System
- Phase 6: First Plugins
- Phase 7: Generator Engine
- Phase 8: Deploy Runtime
- Phase 9: Ecosystem Expansion

Source reference: `docs/acthur-prd.md`, section 21, "Implementation Phases".

## Completion Summary

| Phase | Audit Status | Confidence | Summary |
|---|---:|---:|---|
| Phase 0 - CLI Skeleton | Complete | High | CLI tree, config loader, output/errors, GoReleaser config, and cross-platform source builds are present. |
| Phase 1 - Graph Engine | Complete | High | Graph build/validate/show, traversal, state/subscription, materialized proxy handling, and graph tests are implemented. |
| Phase 2 - Adapter System | Complete | High | Adapter registry and first adapters `go:fiber` and `db:postgres` are present; Fiber scaffold is tested to compile. |
| Phase 3 - Dev Runtime | Partial | High | Process/proxy/health pieces exist, but doctor preflight, DNS setup, watcher integration, and service commands are not complete. |
| Phase 4 - Contract Engine | Partial | Medium-High | Native `.contract.yml`, registry, diff, request validation, and proxy enforcement exist; importers and response validation are missing or limited. |
| Phase 5 - Plugin System | Complete | High | Plugin API, event bus, KernelAPI, loader, dependency ordering, test plugin, and registration paths are implemented and tested. |
| Phase 6 - First Plugins | Partial | Medium | Built-in plugin generators exist for migrations/auth/rbac/multitenancy, but several operational commands and end-to-end workflows remain incomplete. |
| Phase 7 - Generator Engine | Complete for PRD done-when | Medium-High | `generate from-contract`, `generate model`, generated.lock semantics, Go Fiber codegen, migrations, and generated tests are implemented. |
| Phase 8 - Deploy Runtime | Partial / In Progress | High | Deploy artifact and target packages exist, but `acthur deploy` command wiring is inconsistent/in-flight and full test suite currently fails with untracked deploy test. |
| Phase 9 - Ecosystem Expansion | Not Complete | High | Most Phase 9 command surfaces still return `notImplemented(9)`; additional adapters/plugins/wizard/AI/CI/docs are not implemented. |

## Phase-by-Phase Findings

### Phase 0 - CLI Skeleton

Confirmed complete.

Evidence:

- Root command and broad command tree are wired in `cmd/acthur/commands.go`.
- `acthur --help` prints the full command tree when run from source.
- Config loading lives in `internal/config`.
- Output/error formatting lives in `internal/output`.
- `.goreleaser.yml`, Makefile release targets, and GitHub Actions GoReleaser wiring exist.
- Cross-platform source builds succeeded for:
  - `GOOS=linux GOARCH=amd64 go build ./cmd/acthur`
  - `GOOS=darwin GOARCH=amd64 go build ./cmd/acthur`
  - `GOOS=windows GOARCH=amd64 go build ./cmd/acthur`

### Phase 1 - Graph Engine

Confirmed complete.

Evidence:

- Graph model, builder, traversal, validation, summary, state, materialized proxy node, and subscription behavior are in `internal/graph/graph.go`.
- `acthur graph validate` and `acthur graph show` are wired in `cmd/acthur/commands.go`.
- Graph package has extensive tests in `internal/graph/*_test.go`.

Important nuance:

- `testdata/vetangle` includes adapters not currently registered (`ui:astro`,
  `ui:next`, `cache:redis`, `storage:minio`, `queue:nats`). Source `graph show`
  can build/show that graph, but `graph validate` correctly fails unresolved
  adapters because only `go:fiber` and `db:postgres` are implemented.

### Phase 2 - Adapter System

Confirmed complete.

Evidence:

- Core adapter and capability interfaces are in `internal/adapter/adapter.go`.
- Registry and capability introspection are implemented.
- `go:fiber` adapter is implemented in `internal/adapter/backend/gofiber`.
- `db:postgres` adapter is implemented in `internal/adapter/infra/postgres`.
- Adapter tests cover registration, scaffold, run commands, Dockerfile support,
  ContainerSpec, Connectable env, and scaffold compilation.

### Phase 3 - Dev Runtime

Partial. This is not blocked by Phase 9.

PRD requires `acthur dev` to:

1. Load and validate graph.
2. Run `acthur doctor`.
3. Resolve startup order.
4. Start infra nodes.
5. Wait for infra health.
6. Load/init plugins.
7. Start services.
8. Wait for `GET /health -> 200`.
9. Start proxy.
10. Set up DNS.
11. Start file watcher / hot reload cascade.
12. Print ready output.

Current `acthur dev` path:

- `cmd/acthur/commands.go` calls `loadGraph()`, `contract.LoadDir(...)`,
  `engine.NewDevEngine(...)`, then `eng.Start()`.
- `internal/engine/dev.go` validates graph, starts nodes, waits for health,
  starts proxy, prints ready, waits for shutdown.

Implemented Phase 3 pieces:

- Process manager and supervision in `internal/process`.
- Docker run args and container stop args in `internal/container`.
- Dev orchestrator in `internal/engine/dev.go`.
- Health checks in `internal/health`.
- Dev proxy in `internal/proxy`.
- Watcher package exists in `internal/watcher` and has tests.

Missing Phase 3 integration:

- `acthur dev` does not call `doctor.Run`.
- DNS / `.test` setup is not implemented or wired.
- `internal/watcher` is not constructed or started by `DevEngine.Start`.
- Service commands still return `notImplemented(3)`:
  - `acthur service add`
  - `acthur service logs`
  - `acthur service restart`
  - `acthur service health`

Conclusion:

Phase 3 is blocked by unfinished Phase 3 integration, not by Phase 9.
Phase 9 adds broader ecosystem support, but the Phase 3 done-when is narrower:
Go Fiber + Postgres starts, proxies correctly, restarts crashed services, and
hot reloads on file change.

### Phase 4 - Contract Engine

Partial.

Implemented:

- Contract types, parser, registry, diff engine, and runtime validator in
  `internal/contract`.
- CLI commands:
  - `acthur contract validate`
  - `acthur contract list`
  - `acthur contract show`
  - `acthur contract diff`
- Proxy can enforce contracts on `data_flow` flow routes via `/_flow/<from>/<to>/*`.
- `acthur dev --strict` can block contract violations with 422.

Limitations against PRD:

- OpenAPI, Protobuf, and GraphQL importers are explicitly unsupported by
  `contract.ParseFile`.
- Runtime proxy enforcement currently validates requests, not full
  request/response behavior.
- Contract enforcement is implemented on synthetic flow routes rather than all
  possible inter-service calls.

### Phase 5 - Plugin System

Confirmed complete.

Evidence:

- Plugin interface, event bus, registry, loader, and dependency ordering are
  in `internal/plugin/plugin.go`.
- KernelAPI implementation is in `internal/plugin/kernelapi.go`.
- CLI plugin loading and command registration are in `cmd/acthur/commands.go`.
- Built-in test plugin exists in `internal/plugin/builtin/testplugin`.
- Tests exercise:
  - event bus behavior
  - panic recovery
  - dependency ordering
  - plugin command registration
  - generator registration
  - schema registration
  - middleware registration
  - graph mutation before seal

### Phase 6 - First Plugins

Partial.

Implemented:

- `migrations` plugin generator and database migration helpers.
- `auth` plugin generator with JWT/session/magic-link/OAuth provider registry
  templates and migrations.
- `rbac` plugin generator with auth dependency.
- `multitenancy` plugin generator with schema-per-tenant templates, migration,
  pool hooks, and provisioner.
- `acthur add <plugin>` workflow exists and is tested.

Missing or incomplete:

- Several Phase 6 command surfaces still return `notImplemented(6)`:
  - `acthur db migrate:create`
  - `acthur db seed`
  - `acthur db reset`
  - `acthur db studio`
  - `acthur test`
- The PRD done-when says `acthur add auth` on a fresh `go:fiber` project should
  produce compiling, runnable auth code with migrations. There is strong unit
  coverage for generated code, but the full fresh-project runtime workflow was
  not confirmed in this audit.

### Phase 7 - Generator Engine

Complete for the PRD done-when.

Implemented:

- Write engine and `generated.lock` semantics in `internal/generate`.
- Go Fiber contract generator in `internal/generate/gofiber`.
- `acthur generate from-contract <file>`.
- `acthur generate model <Name> ...`.
- Contract-derived handlers, DTOs, service/repository skeletons, migrations,
  and tests are generated.
- Tests validate generated output and generated test/build behavior.

Remaining caveat:

- This is currently Go Fiber focused. That matches the Phase 7 done-when, but
  not the broader long-term multi-adapter PRD vision.

### Phase 8 - Deploy Runtime

Partial / in progress.

Implemented:

- Deploy preflight gate in `internal/deploy/gate.go`.
- Compose target in `internal/deploy/compose.go`.
- Dockerfile and docker-compose artifact projection in
  `internal/deploy/artifacts`.
- Coolify client/target packages in `internal/deploy/coolify`.
- `cmd/acthur/deploy.go` contains a `runDeploy(...)` implementation used by
  tests and supports dry-run planning, artifact projection, gate, compose, and
  Coolify target paths.

Blocking issues:

- The Cobra `deployCmd` in `cmd/acthur/commands.go` still calls
  `notImplemented(8)` rather than `runDeploy(...)`.
- Full `go test ./...` currently fails because untracked
  `cmd/acthur/deploy_test.go` expects env vars in
  `TestRunDeploy_ComposeTarget_EndToEnd`, despite the test body using
  `t.Setenv`. This indicates in-flight work that needs reconciliation.

Conclusion:

Do not mark Phase 8 complete until the actual `acthur deploy` command path is
wired and the full test suite passes from a clean checkout/worktree.

### Phase 9 - Ecosystem Expansion

Not complete.

Evidence:

The following surfaces still return `notImplemented(9)`:

- `acthur new`
- `acthur init`
- `acthur generate ai-context`
- `acthur generate ci`
- `acthur generate docs`
- `acthur generate skill`
- `acthur graph visualize`
- `acthur mcp serve`
- `acthur agent ...`
- `acthur secrets ...`
- `acthur flag ...`
- `acthur monitor`

Also missing:

- Additional backend adapters listed by the PRD.
- Additional frontend adapters.
- Additional ecosystem plugins.
- Additional deploy targets beyond compose/Coolify in progress.
- Interactive wizard.
- Existing-project adoption.

## Specific Clarification: Is Phase 9 Blocking `acthur dev`?

No.

`acthur dev` is a Phase 3 deliverable. The current blockers are Phase 3-owned:

- doctor preflight integration
- DNS / `.test` setup
- watcher startup / hot reload cascade integration
- service management commands

Phase 9 can broaden which project shapes `acthur dev` supports, because it adds
more adapters and the wizard, but it is not required to finish the Phase 3
Go Fiber + Postgres done-when.

## Verification Commands Run

These commands were run during the audit:

```bash
go test ./...
GOOS=linux GOARCH=amd64 go build -o /tmp/acthur-linux ./cmd/acthur
GOOS=darwin GOARCH=amd64 go build -o /tmp/acthur-darwin ./cmd/acthur
GOOS=windows GOARCH=amd64 go build -o /tmp/acthur-windows.exe ./cmd/acthur
go run /home/makebot360tech/projects/acthur/cmd/acthur --help
go run /home/makebot360tech/projects/acthur/cmd/acthur graph validate
go run /home/makebot360tech/projects/acthur/cmd/acthur graph show
go run /home/makebot360tech/projects/acthur/cmd/acthur contract validate
go run /home/makebot360tech/projects/acthur/cmd/acthur deploy --dry-run
```

Notes:

- Cross-platform builds passed.
- Internal focused packages passed when run independently.
- `go test ./...` did not pass because of in-progress/untracked deploy test
  work under `cmd/acthur/deploy_test.go`.
- A checked-in `acthur` binary appears stale relative to source behavior. Use
  `go run ./cmd/acthur` or rebuild before trusting CLI smoke tests.

## Current Worktree Notes

At the time of audit, the worktree had untracked/in-progress deploy test work:

- `cmd/acthur/deploy_test.go`

Earlier audit context also observed implementation-in-progress changes in dev
runtime, process management, adapter templates, and container shutdown. Do not
revert or overwrite unrelated in-progress implementation changes without owner
confirmation.

## Suggested Next Work

1. Finish Phase 3 before treating later runtime phases as complete:
   - run doctor preflight from `acthur dev`
   - wire DNS strategy setup or explicitly scope it out with docs/tests
   - start `internal/watcher` from `DevEngine`
   - prove hot reload cascade end-to-end
   - implement or defer service subcommands explicitly

2. Reconcile Phase 8 command wiring:
   - connect Cobra `deployCmd` to `runDeploy(...)`
   - fix full `go test ./...` with the in-flight deploy test
   - avoid relying on untracked test files for completion evidence

3. Decide whether Phase 4 importers are required for completion:
   - current source explicitly rejects OpenAPI/Protobuf/GraphQL importers
   - if intentionally out of scope for Phase 4, update PRD slice docs or audit
     criteria to prevent future false negatives

4. Validate Phase 6 with a fresh-project workflow:
   - scaffold Go Fiber project
   - `acthur add auth`
   - run migrations
   - build and run generated auth endpoints

## Suggested Skills

- `diagnose`: for fixing failing tests and runtime behavior regressions.
- `tdd`: for completing Phase 3 and Phase 8 with red-green-refactor loops.
- `review`: for re-auditing a phase after changes land.
- `request-refactor-plan`: if Phase 3 command/runtime wiring needs to be broken
  into safe incremental issues.
