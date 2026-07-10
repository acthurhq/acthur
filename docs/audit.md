# Acthur PRD End-to-End Audit

Date: 2026-07-06

## Update — 2026-07-10 (findings closed since this audit)

Re-verified live against a real scaffolded project (`final-witness`: go:fiber +
db:postgres + migrations/auth/feature-flags/observability/security plugins),
real Docker, real Postgres — not simulated.

- **P0 wrong-container health check (Finding 1 below): fixed.** `startInfraNode`
  (`internal/engine/dev.go`) now builds both the docker-run name and the exec
  healthcheck target from the same `container.Name(project, nodeID)` call.
  Regression test: `TestStartInfraNode_HealthcheckTargetsProjectScopedContainerName`
  (`internal/engine/dev_test.go`). Live-witnessed: `acthur dev` brought a
  project-scoped Postgres to healthy, `acthur db migrate/seed/reset --yes` all
  passed against the real container.
- **P1 migration-range collision (Finding 3 below): fixed.** `renumberMigrations`
  (`cmd/acthur/generate.go`) now errors when a contract-generated migration's
  description collides with an existing migration *outside* the 0400-0499
  contract range, instead of silently reusing that plugin-reserved number.
  Regression tests: `TestRenumberMigrations_DescriptionCollisionOutsideContractRange_Errors`,
  `TestRenumberMigrations_ReusesNumberWithinContractRange` (`cmd/acthur/generate_test.go`).
- **New finding + fix, found while re-verifying Phase 8: deploy Dockerfile Go
  version drift.** The go:fiber production Dockerfile hardcoded
  `golang:1.22-alpine`. A plugin dependency (observability's
  `prometheus/client_golang`) bumps a scaffolded node's `go.mod` `go` directive
  via `go mod tidy` well past 1.22 — reproduced live (`go 1.23.0`/`1.25.0`
  depending on installed toolchain), and `acthur deploy --target compose`
  failed at `go mod download` with "go.mod requires go >= X". Fixed:
  `artifacts.Project` now reads each node's actual `go.mod` directive
  (`nodeGoVersion`, `internal/deploy/artifacts/artifacts.go`) and threads it
  into the Dockerfile template instead of a version fixed at codegen time.
  Regression test: `TestProject_DockerfileUsesNodesActualGoModVersion`.
  Live-witnessed: real `acthur deploy --target compose --env production`
  against `final-witness` (go.mod at `go 1.23.0`) produced a healthy compose
  stack (`docker ps` showed both containers healthy, `GET /health` → 200
  through the deployed container).
- **New finding + fix: `acthur db studio` leaked its adminer container on
  SIGTERM.** `waitForInterrupt` (`cmd/acthur/db.go`) only trapped
  `os.Interrupt`; SIGTERM's default disposition terminates the process
  immediately, skipping the deferred `docker stop`. Reproduced live (`kill
  <pid>` left `acthur-db-studio` running). Fixed by also catching
  `syscall.SIGTERM`. Regression test: `TestWaitForInterrupt_ReturnsOnSIGTERM`.
- **P0 full suite not green (Finding 2 below): still flaky, not truly fixed.**
  `internal/process.TestProcess_OutputCapture` still fails intermittently
  under full-suite parallel load (passes standalone and on rerun every time
  observed across this session and the prior one). Root cause not
  investigated — left as a known flake, not silently ignored.
- **Not addressed — deliberately left for a product decision, not a
  code fix:**
  - Finding 4 (Phase 8 deploy projection silently skips unsupported
    adapters): the existing test suite (`TestProject_NoDockerfileForUnresolvableAdapters`
    et al.) explicitly documents and asserts the current silent-skip
    behavior as intentional ("Slice 1 projects what the adapter registry can
    actually back"). Making this a hard error is a real behavior change with
    product tradeoffs (e.g. it would block deploying a graph that
    legitimately includes an unimplemented adapter like `cache:redis` for a
    node the caller doesn't intend to deploy yet) — flagged to the user
    rather than changed unilaterally.
  - P2 plugin discovery outside a project: unchanged, low severity, not
    revisited this pass.

## Update — 2026-07-10, later same day (Finding 5 resolved)

The user confirmed the canonical GitHub org: **acthurhq**. Finding 5
(installer/release namespace) is now closed: `go.mod`'s module path, every
internal `github.com/acthur/acthur/...` import (~1283 files), `.goreleaser.yml`
(release/homebrew/scoop `owner:` fields and the version ldflags path),
`scripts/install.sh`/`scripts/install.ps1`'s `ACTHUR_REPO`, and `CLAUDE.md`
were all repointed to `github.com/acthurhq/acthur`. `go build ./... && go vet
./... && go test ./...` green after the rename.

Boundary review (repo-split readiness, per project convention): no
`internal/*` package imports `cmd/*`; no adapter package imports a sibling
adapter package across backend/frontend/infra; no plugin imports a sibling
plugin; no deploy target imports a sibling target. All still clean as of this
update.

Scope: fresh audit against `docs/acthur-prd.md` and `docs/prd/phase-*.md`. I did not trust completion notes, comments, or the previous audit. I did not edit implementation code.

## Executive Verdict

Dev's claim that all phase gaps are completely fixed is not supported.

The implementation is significantly more complete than the earlier baseline, but it is not production-ready end-to-end yet. The strongest remaining blockers are:

1. Phase 3 `acthur dev` still has a real Go Fiber + Postgres blocker: the Postgres container is started under one name and health-checked under another.
2. Phase 7 code generation can overwrite/collide with Phase 6 auth migrations because contract migrations are renumbered into an existing plugin range.
3. Phase 8 deploy exists for local compose and remote provider clients, but it is not proven to satisfy the PRD's VPS one-command production deployment. Its artifact projection silently skips unsupported adapters instead of making the pre-deploy gate fail.
4. The installer/release path is not production-ready because scripts and GoReleaser still point at `github.com/acthur/acthur`, an unavailable namespace.
5. `go test ./...` is not green: `internal/process.TestProcess_OutputCapture` failed in this audit.

The PRD has 10 phases altogether, numbered Phase 0 through Phase 9. This audit focuses on the dev claim for Phase 0-8.

## Verification Performed

- Read the top-level phase definitions in `docs/acthur-prd.md`.
- Read the phase PRDs under `docs/prd/`, especially phases 2-8.
- Inspected current CLI wiring, adapters, dev runtime, contracts, plugins, generators, deploy runtime, installers, and release config.
- Built a temporary CLI binary with `go build -o /tmp/acthur-audit ./cmd/acthur`.
- Ran `go test ./...`.
- Ran CLI smoke checks: `acthur --help`, `adapter list`, `adapter inspect db:postgres`, contract diff fixtures.
- Created a fresh temp project with `acthur new demo --adapter go:fiber --db --module example.com/demo`.
- In that temp project verified:
  - `acthur graph validate` succeeds.
  - generated `api` service builds after `go mod tidy`.
  - `acthur add auth --node api` generates auth files and the service builds.
  - `acthur contract validate` succeeds on a new `users.contract.yml`.
  - `acthur generate from-contract users --node api` generates compiling code and tests.
  - `acthur deploy --dry-run --env production` prints a compose plan.

I did not run a live `acthur dev` or `acthur deploy` because the code inspection found a container-name health-check mismatch that would likely start a real Postgres container and wait against the wrong name.

## Phase Status

| Phase | Verdict | Notes |
|---|---:|---|
| Phase 0 - CLI Skeleton | Complete enough | Command tree exists, binary builds locally, help works. Release namespace remains a production packaging issue. |
| Phase 1 - Graph Engine | Complete enough | Fresh project validates; graph commands are wired. |
| Phase 2 - Adapter System | Complete for core, with namespace caveat | Registry and introspection work; `go:fiber` scaffold compiles; `db:postgres` is visible. |
| Phase 3 - Dev Runtime | Not complete | Container health check uses the wrong container name; `go test ./...` also fails in process output capture. |
| Phase 4 - Contract Engine | Substantially complete | Contract CLI and diff behavior work against fixtures; proxy enforcement code exists. Live proxy violation witness was not run. |
| Phase 5 - Plugin System | Substantially complete | Kernel API, event bus, loader, and test plugin exist. `plugin list` requires a project config, so global discoverability is weak but not a phase blocker. |
| Phase 6 - First Plugins | Substantially complete | `acthur add auth` on a fresh go:fiber project produced compiling auth code with migrations. Full runtime auth behavior was not exercised. |
| Phase 7 - Generator Engine | Partially complete, blocker found | `generate from-contract` produces compiling code/tests, but migration numbering can collide with plugin migrations. |
| Phase 8 - Deploy Runtime | Partial / not production-proven | Compose dry-run and artifacts exist; gate builds/tests/env-checks. One-command VPS production deployment is not proven; projection skips unsupported adapters. |

## Findings

### P0 - Phase 3 `acthur dev` Health Checks the Wrong Container

Requirement: Phase 3 is done when `acthur dev` starts Go Fiber + Postgres, gates Postgres on `pg_isready`, boots Fiber with `DATABASE_URL`, proxies correctly, restarts crashed services, and hot-reloads.

Evidence:
- `startInfraNode` starts Docker with `container.Name(e.cfg.Project, node.ID)` at `internal/engine/dev.go:362`.
- `container.Name(project, nodeID)` returns `acthur-<project>-<nodeID>` when a project is set.
- The readiness probe is built with `health.InfraStrategy("acthur-"+node.ID, ...)` at `internal/engine/dev.go:374`.

Impact: for a normal project named `demo` and node `db`, Docker starts `acthur-demo-db`, but the `docker exec` health probe targets `acthur-db`. Postgres can be running correctly and still fail readiness. This blocks the Phase 3 done-when path.

Fix direction: compute the container name once in `startInfraNode`, reuse it for both `ToRunArgs` and `InfraStrategy`, and add a regression test that asserts the same name is used.

### P0 - Full Test Suite Is Not Green

Command: `go test ./...`

Result: failed in `github.com/acthur/acthur/internal/process`.

Failing test: `TestProcess_OutputCapture` expected captured output containing `hello from acthur`, but got an empty slice. The assertion is at `internal/process/process_test.go:107`.

Impact: the process/log-router layer is part of Phase 3's supervision and unified log-output promise. Even if this is flaky, a non-green full test suite blocks a production-ready claim.

### P1 - Phase 7 Contract Migrations Can Overwrite Phase 6 Plugin Migrations

Requirement: Phase 7 contract-derived migrations use range `0400-0499`. Phase 6 plugin ranges reserve auth at `0100-0199`.

Evidence:
- The gofiber contract generator emits `migrations/0400_<package>.up.sql` and `.down.sql` at `internal/generate/gofiber/gofiber.go:46`.
- `renumberMigrations` scans all existing migrations and reuses an existing number when the description matches at `cmd/acthur/generate.go:181`.
- In a fresh temp project, after `acthur add auth`, running `acthur generate from-contract users --node api` wrote `migrations/0100_users.up.sql` and `migrations/0100_users.down.sql`.

Impact: contract generation reused auth's `0100_users` migration name and number. Because `generated.lock` already tracked those files from auth, this can overwrite or replace auth migration content. That violates both Phase 6 plugin migration ranges and Phase 7 contract migration range.

Fix direction: scope renumbering to the contract range only, never reuse an existing migration outside `0400-0499` for contract-generated files, and fail on description collisions with reserved plugin ranges.

### P1 - Phase 8 Deploy Projection Silently Skips Unsupported Adapters

Requirement: the deploy gate refuses to ship a broken system.

Evidence:
- `artifacts.Project` skips nodes when `adapter.Resolve` fails at `internal/deploy/artifacts/artifacts.go:107`.
- It also skips service nodes whose adapter is not `Dockerizable` at `internal/deploy/artifacts/artifacts.go:166`.
- The gate checks build, test, and required env vars only; see `internal/deploy/gate.go:68`.

Impact: a graph can contain a node that cannot be projected to production, but artifact generation may silently omit it. That weakens the Phase 8 "pre-deploy gate refuses to ship a broken system" requirement.

Fix direction: deploy artifact projection should return an error for every service/infra node that is in the deploy graph but lacks required production support, unless the node is explicitly marked out-of-deploy.

### P1 - Phase 8 Is Not Yet Proven Production/VPS Ready

Requirement: `acthur deploy` takes a project to running production on a VPS in one command.

Current evidence:
- `acthur deploy --dry-run --env production` printed a compose plan in a temp project.
- Compose, Coolify, Fly, Railway, and Render target code exists.
- The local gate builds and tests Go service nodes and checks required env vars.

Gap: I did not find a live witness proving one-command VPS production deployment. The phase PRD itself allowed a local compose witness plus fake Coolify client, but the top-level PRD's done-when is stricter. This should remain partial until a real or documented reproducible deploy witness exists.

### P1 - Installer and Release Config Still Point to Unavailable Namespace

Evidence:
- `scripts/install.sh:14` sets `ACTHUR_REPO="acthur/acthur"`.
- `scripts/install.ps1:13` sets `$ACTHUR_REPO = "acthur/acthur"`.
- `.goreleaser.yml:66`, `:77`, and `:89` publish releases/taps/buckets under owner `acthur`.
- `go.mod` module path is still `github.com/acthur/acthur`.

Impact: install scripts will query/download releases from an org the project does not control. Public installation is not production-ready until the canonical namespace is chosen and applied consistently.

Fix direction: pick the canonical org/module path before release, then update `go.mod`, imports, GoReleaser, install scripts, docs, README, CLI source URL, Homebrew/Scoop owners, and CI guards.

### P2 - Plugin Discovery Outside a Project Is Weak

Evidence: `acthur plugin list` calls `loadGraph()` at `cmd/acthur/commands.go:1266`, so it fails outside a directory with `acthur.yml`.

Impact: this does not block Phase 5's project-scoped done-when, but it is awkward for "available plugins" discovery. `adapter list` works globally; `plugin list` does not.

Fix direction: split `plugin list` into global available plugins plus project-loaded plugins, where the project section is skipped when no config exists.

## Positive Confirmations

- `acthur --help` prints the full command tree.
- `adapter list` shows registered backend, frontend, and database adapters.
- `adapter inspect db:postgres` shows `Container` and `Connectable`.
- Fresh `go:fiber` scaffold compiles after `go mod tidy`.
- `module_prefix` flows into generated module path in the fresh project.
- `acthur graph validate` accepts the fresh Go Fiber + Postgres graph.
- Contract diff exits non-zero for breaking fixture changes and zero for non-breaking fixture changes.
- `acthur add auth` generated compiling Go auth code with migrations in the fresh project.
- `acthur generate from-contract` generated compiling Go code and tests in the fresh project.
- `acthur deploy --dry-run` emits a compose deploy plan.

## Phase 9 Ecosystem Expansion Findings

Phase 9 has 10 checklist items in `docs/acthur-prd.md`: extra backend adapters, extra frontend adapters, ecosystem plugins, extra deploy targets, `acthur init`, `generate ai-context` + AI plugin, CI generation, docs generation, analytics plugins, and a fully operational interactive `acthur new` wizard.

Current status:

| Phase 9 Item | Verdict | Evidence / Gap |
|---|---:|---|
| Additional backend adapters | Mostly implemented | `adapter list` shows `go:chi`, `go:gin`, `rust:axum`, and `node:fastify`, in addition to `go:fiber`. |
| Additional frontend adapters | Partial | Implemented: `ui:next`, `ui:astro`. Missing from registry: `ui:nuxt`, `ui:vue`, `ui:svelte`. |
| Additional plugins | Mostly implemented | `feature-flags`, `admin`, `observability`, `security`, and `https` packages exist and targeted tests passed. |
| Additional deploy targets | Implemented in code, not production-witnessed | `fly`, `railway`, and `render` target packages exist and targeted tests passed, but no live provider deployment was verified. |
| `acthur init` existing-project adoption | Partial | Command exists, but implementation shares the new-project scaffold path and does not yet prove non-destructive detection/adoption of existing app stacks. |
| `generate ai-context` + AI plugin | Partial | `acthur generate ai-context` exists; a built-in AI plugin equivalent to the plugin system was not found. |
| CI/CD generation plugin | Partial / command implemented | `acthur generate ci` exists and generator tests passed. It is implemented as a generator command, not clearly as a plugin. |
| Documentation generation plugin | Partial / command implemented | `acthur generate docs` exists and generator tests passed. It is implemented as a generator command, not clearly as a plugin. |
| Analytics plugins | Missing | No product, web, or business analytics plugin package found. |
| `acthur new` interactive wizard fully operational | Partial | Wizard and non-interactive flags exist, but it only covers backend adapter, optional Postgres, and module prefix. It does not yet appear to cover frontend selection, plugin selection, deploy target, CI/docs, or richer community-ready project setup. |

Additional Phase 9 ecosystem gaps found after the first pass:

- **JavaScript package-manager selection is missing.** The current Node/UI adapters hard-code `npm` in `DevCommand`, `BuildCommand`, `TestCommand`, package scripts, and Dockerfiles. There is no wizard choice or config model for `npm`, `yarn`, `pnpm`, or `bun`. This matters because the PRD expects Node/UI projects to use `pnpm` by default and Bun projects to use `bun`.
- **Generated frontend templates are not on current latest releases.** Current templates use `astro ^4.16.0`, `@astrojs/node ^8.3.4`, `next ^14.2.13`, `react ^18.3.1`, and `typescript ^5.6.2`. Current npm latest checked during audit: `astro 7.0.6`, `@astrojs/node 11.0.2`, `next 16.2.10`, `react/react-dom 19.2.7`, `typescript 6.0.3`.
- **Plain React + TypeScript / Vite React is not implemented as an adapter.** The code has `ui:next` and `ui:astro`, but no standalone `ui:react` or `ui:vite-react`.
- **Mobile/desktop adapters are not implemented.** No `mobile:flutter`, `mobile:react-native`, `desktop:tauri`, or `desktop:electron` adapter packages were found.
- **Hot reload is implemented in code for supported adapters, but not end-to-end proven.** Current supported service/frontend adapters implement self-reload via adapter-native tooling: Go adapters use `air`, `rust:axum` uses `cargo-watch`, `node:fastify` uses `node --watch`, `ui:astro` uses Astro/Vite dev reload, and `ui:next` uses Next dev Fast Refresh. The dev engine also starts a polling watcher and restarts non-self-reloading services. However, because Phase 3 `acthur dev` still has the Postgres container health-check blocker, the full "fresh project hot reloads under `acthur dev`" acceptance path remains unverified.
- **Architecture-mode project creation is not implemented.** There are no flags or commands for `--triple`, `--double`, `--single`, `--api`, `--mobile`, or `new-desktop`. Current `acthur new` only accepts `--adapter`, `--db`, and `--module`, then scaffolds one `api` node plus optional Postgres. The proposed modes below are absent:
  - Triple: Web + Admin + API monorepo/Turborepo.
  - Double: Web + API.
  - Single: Go binary with `go:embed` frontend.
  - API: API-only as an explicit mode.
  - Mobile: API + Expo React Native.
  - Desktop: Wails + Go + React + SQLite.

README CLI/reference drift:

| README Claim | Actual Implementation | Audit Result |
|---|---|---|
| Project structure includes `services/api`, `web`, `backoffice`, and `db/migrations` generated. | `acthur new` writes `api/` plus optional root `migrations/` later through plugins; no `web` or `backoffice` is scaffolded by `new`. | README overclaims generated structure. |
| Supported runtimes include Echo, Actix, Rocket, NestJS, Express, Bun Elysia/Hono, Python FastAPI/Django/Flask, PHP Laravel. | Registry currently includes `go:fiber`, `go:chi`, `go:gin`, `rust:axum`, `node:fastify`, `ui:astro`, `ui:next`, `db:postgres`. | README overclaims runtime support. |
| Supported frontends include Nuxt, SvelteKit, Vue. | Registry has `ui:astro` and `ui:next`; `ui:nuxt`, `ui:svelte`, and `ui:vue` are missing. | README overclaims frontend support. |
| `acthur contract diff <n>` | Actual command is `acthur contract diff <old-file> <new-file>`. | README command signature is stale/wrong. |
| `acthur generate model <Name> --fields "..."` | Help advertises `--fields`, but Cobra requires at least one positional `field:type`; command implementation passes `args[1:]` and does not read the `--fields` flag. | README command likely fails as written. |
| `acthur init` scans existing project, detects stack, writes only `acthur.yml` and `.acthur/`. | Current `init` shares scaffold path with `new`, scaffolds generated files into cwd, and only refuses when `acthur.yml` already exists. | README overclaims non-destructive adoption. |
| Deploy targets include Docker. | `deploy --target` supports `compose`, `coolify`, `fly`, `railway`, `render`; there is no literal `docker` target despite config enum containing `TargetDocker`. | README target name is ambiguous/stale. |
| Plugins list includes `secrets`, `i18n`, `analytics`, `ci-cd`, `docs`, `ai`. | `secrets`, `i18n`, and analytics plugins are missing; CI/docs/AI exist as commands/generators, not clearly as plugins. | README plugin table overclaims plugin availability. |
| `acthur new my-saas` quick start leads to `http://my-saas.test:4000`. | DNS `.test` is not automatic; dev uses localhost proxy and DNS setup is deferred/optional. | README quick-start URL overclaims current dev DNS behavior. |

Expanded plugin catalogue status:

| Plugin / Capability Area | Verdict | Evidence / Gap |
|---|---:|---|
| `migrations` | Implemented in code | Built-in plugin package exists with tests. |
| `auth` | Implemented in code, scope-limited | Built-in plugin package exists with tests; advanced PRD auth scope such as MFA/passkeys/full OAuth matrix is not fully proven. |
| `rbac` | Implemented in code, partial vs PRD | Built-in plugin package exists with tests; contract-derived enforcement is still stubbed/limited. |
| `multitenancy` | Implemented in code, partial vs PRD | Built-in plugin package exists with tests; full runtime/data-isolation witness not performed. |
| `observability` | Implemented in code, partial vs PRD | Built-in plugin package exists with tests; full OpenTelemetry/Prometheus/Grafana stack witness not performed. |
| `feature-flags` | Implemented in code, partial vs PRD | Built-in plugin package exists with tests; provider ecosystem beyond local flags is not proven. |
| `admin` | Implemented in code, partial vs PRD | Built-in plugin package exists with tests; full generated admin UI/workflow not proven. |
| `security` | Implemented in code, partial vs PRD | Built-in plugin package exists with tests; full OWASP coverage/CVE scanning not proven. |
| `https` | Implemented in code, partial vs PRD | Built-in plugin package exists with tests; dev cert path exists, but production ACME automation is not proven. |
| `secrets` plugin | Missing as plugin | There is `internal/secrets` and `acthur secrets` command support, but no built-in `secrets` plugin/adapters for Vault, Doppler, Infisical, AWS SSM, GCP Secret Manager, or Azure Key Vault. |
| `i18n` / l10n | Missing | No built-in i18n plugin/package found. |
| Analytics | Missing | No product analytics, web analytics, or business analytics plugin packages found. |
| Payments: Stripe | Missing | No Stripe payments plugin found. |
| Payments: Paystack | Missing | No Paystack plugin found. |
| Payments: Flutterwave | Missing | No Flutterwave plugin found. |
| Notifications | Missing | No in-app, push, SMS, or email notification plugin/channel system found. |
| Video/media | Missing | No upload, FFmpeg processing, or HLS streaming plugin found. |
| Conferencing | Missing | No WebRTC signaling, rooms, or conference plugin found. |

Targeted Phase 9 test run:

```text
go test ./cmd/acthur ./internal/adapter ./internal/deploy/... ./internal/generate/... ./internal/plugin/builtin/...
```

Result: passed. This is useful evidence for the implemented pieces, but it does not close the missing Phase 9 scope above or replace live provider/deployment witnesses.

## What Is Left Before Calling Phase 0-8 Done

1. Fix the Phase 3 container-name mismatch and add a regression test.
2. Make `go test ./...` green, starting with `internal/process.TestProcess_OutputCapture`.
3. Run and record a real Phase 3 witness: fresh Go Fiber + Postgres project, `acthur dev`, Postgres `pg_isready`, Fiber health, proxy route, crash restart, hot reload.
4. Fix Phase 7 migration numbering so contract migrations cannot collide with plugin ranges.
5. Add a regression test for `auth` + `generate from-contract users` proving auth migrations remain untouched and contract migrations land in `0400-0499`.
6. Make Phase 8 fail loudly for unprojectable deploy graph nodes.
7. Run and record the Phase 8 compose witness with real Docker: gate, artifact generation, compose up/build, health wait, status.
8. Decide the canonical GitHub namespace and update installer/release/module/docs references consistently.
9. Run a GoReleaser snapshot after namespace cleanup.

## What Is Left Before Calling Phase 9 Done

1. Add missing `ui:nuxt`, `ui:vue` and `ui:svelte` frontend adapters or formally revise the Phase 9 adapter list.
2. Add analytics plugins for product, web, and business analytics.
3. Decide whether CI/docs/AI generation must be plugin-backed or whether command-backed generators satisfy the PRD; update code or PRD accordingly.
4. Expand `acthur init` from scaffold-into-cwd to real existing-project detection/adoption, with tests proving it does not overwrite existing files.
5. Expand the `acthur new` wizard to cover the Phase 9 ecosystem choices expected for community adoption: frontend, plugins, deploy target, CI/docs, and related defaults.
6. Run live deployment witnesses for `fly`, `railway`, and `render`, or clearly mark those targets experimental until verified.
7. Add a JavaScript package-manager model and wizard choice for `npm`, `yarn`, `pnpm`, and `bun`; adapters must use it consistently for dev/build/test commands, Dockerfiles, lockfile detection, and generated package metadata.
8. Update frontend templates to current supported framework majors or explicitly pin documented LTS versions.
9. Add `ui:react` / Vite React, `mobile:flutter`, `mobile:react-native`, and any other frontend/mobile/desktop adapters the PRD still claims.
10. Add or de-scope the broader plugin catalogue: `secrets`, `i18n`, payments (`stripe`, `paystack`, `flutterwave`), notifications, video/media, and conferencing.
11. Run a live hot-reload witness after the Phase 3 dev-runtime blocker is fixed.
12. Add architecture-mode project creation (`--api`, `--single`, `--double`, `--triple`, `--mobile`) or remove those claims from planning/docs until the scaffolds exist.
13. Reconcile README CLI examples with the actual command tree, especially `contract diff`, `generate model`, `init`, deploy target names, supported runtime tables, plugin table, and quick-start URL.

## Production Readiness

Not production-ready yet.

The CLI is usable for local scaffolding and code generation experiments, but the project should not be marketed as installable or production-deployable until the Phase 3 dev runtime blocker, Phase 7 migration collision, Phase 8 deployment proof, green full test suite, and release namespace are fixed.
