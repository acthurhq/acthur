# Implementation: Production Readiness and First Release

## Goal

Turn the implementation on `dev` into a defensible end-to-end production
release: close every retained PRD requirement, make all supported-platform
quality gates green, prove local and remote deployment behavior, promote it
through `staging`, publish the release, and fast-forward `main` only after the
evidence is complete.

## Owning Docs

- `docs/acthur-prd.md`
- `docs/prd/phase-3-dev-runtime.md`
- `docs/prd/phase-8-deploy-runtime.md`
- `docs/production-roadmap-handoff.md`
- GitHub PR #66
- GitHub issue #69

## Status

In-flight

- 2026-07-14 — main agent — audited the PRD, code, CI, release path, GitHub
  state, and surviving worktrees; established the production gate.
- 2026-07-14 — main + CI agent — fixed short-lived process output loss by
  starting pipe readers before the sole waiter; race stress is green.
- 2026-07-14 — main agent — stopped retaining durable-log file handles,
  corrected Windows permission assertions and the Windows wait helper, and
  cross-compiled the process test binary for Windows.
- 2026-07-14 — main agent — corrected Makefile and GoReleaser version
  injection to target `main.Version`; black-box release-version check green.
- 2026-07-14 — deploy agent — changed artifact projection to fail closed for
  every declared node that cannot be projected; package race tests green.
- 2026-07-14 — graph-gate TDD agent — made `RunGate` own explicit semantic
  validation of the already-built production graph with an injected adapter
  resolver; captured red at the public gate seam, then wired deploy to the
  real registry resolver. Valid graphs now record a passing `graph` check and
  unresolved adapters block with their rule and adapter key. Deploy/package
  race tests and focused vet are green.
- 2026-07-14 — generated-freshness TDD agent — captured red at the public
  deploy-gate seam, then added a read-only `generated.lock` verifier and wired
  it as the named `generated artifacts` check. Missing and content-diverged
  tracked files now block deployment with pointed paths; absent and empty
  locks remain valid. Generate/deploy race tests and focused vet are green.
- 2026-07-14 — deploy secret-delivery TDD agent — captured red at the CLI
  deploy seam: a required variable absent from the process environment but
  present only in `.acthur/secrets/kv` passed every gate, wrote artifacts, and
  entered the target despite never being delivered there. Removed the
  validation-only `SecretResolver` fallback and its command wiring/help claim.
  Required variables must now exist in the actual deploy process environment;
  local-store-only values block before artifact writes and target calls, while
  exported values pass. Deploy/CLI race suites and focused vet are green.
- 2026-07-14 — migrations-gate TDD agent — captured red at the public gate
  seam for the missing `MigrationState`/status callback and at the migrations
  package seam for missing latest-up-file discovery. Added the named
  `migrations` check: projects without migrations pass; dirty databases and
  applied versions behind the highest `*.up.sql` version block pointedly;
  fully applied clean state passes. Command assembly detects configured or
  present migrations and supplies live `migrations.Status` using the exported
  production `DATABASE_URL` (never graph-derived development coordinates).
  A CLI behavior test proves missing delivered database configuration blocks
  before artifact writes and target execution. Focused race tests and vet are
  green.
- 2026-07-14 — security-gate TDD agent — captured red first at the public
  deploy-gate callback seam and then at the CLI deploy seam. Added the named
  `security` preflight without coupling deploy to plugin internals; projects
  without the plugin pass explicitly, while the command wires builtin
  production validation whenever `security` is configured. Wildcard,
  malformed, or non-string CORS origins, nonpositive or non-integer supplied
  rate-limit values, and service adapters unsupported by the generated
  middleware now block before artifact writes or target execution. Safe
  go:fiber configuration passes. Focused race suites are green.
- 2026-07-14 — installer-integrity TDD agent — captured a black-box Unix
  installer red in which a mismatched release archive reached extraction.
  Both Unix and PowerShell installers now download the versioned GoReleaser
  checksum manifest, require an exact entry for the selected archive, and
  verify SHA-256 before extraction (`sha256sum`/`shasum` on Unix and
  `Get-FileHash` on Windows). Missing entries and mismatches fail closed. The
  Unix installer test also proves that a valid archive installs a binary that
  reports the requested release version; the installers enforce that version
  check themselves. Black-box installer tests, POSIX shell syntax, and
  `git diff --check` are green. PowerShell execution/parse verification remains
  pending because this host has no PowerShell runtime.
- 2026-07-14 — main agent — GitHub's Go 1.23 Ubuntu PR run reproduced
  short-lived output loss after the first pipe-ordering fix. Replaced
  independently drained `StdoutPipe`/`StderrPipe` readers with line writers
  owned by `exec.Cmd`, whose `Wait` contract includes completed output copies.
  Restart stress then exposed that the waiter closure still referenced mutable
  `p.cmd`/`p.exited`; bound each waiter to its own command and exit channel.
  Green evidence: output capture under `-race -count=100`, orphan-restart under
  `-race -count=10`, full `go test -race -count=1 ./...`, full `go vet ./...`,
  and `git diff --check`. The refreshed matrix also exposed the retired
  `macos-13` runner label; CI now uses GitHub's supported `macos-15-intel`
  label, retaining the required x86_64 race-detector path.
- 2026-07-14 — contract/CI-gate TDD agent — captured compile-time reds at
  the public contract and deploy-gate seams: no deploy baseline could be
  defined or checked, and the gate exposed no runner seam through which its
  test policy could be proven. Added the explicit
  `.acthur/contracts.lock.yml` baseline, public
  `UpdateDeployBaseline`/`CheckDeployBaseline` operations, and the user-facing
  `acthur contract baseline update` command. Deploy now records a named
  `contract compatibility` check; an absent baseline passes, while removed
  contracts and breaking diffs block with the contract, diff field, and
  change description. Added the narrow `GateInput.GoRunner` process boundary
  and made every deploy test invocation exactly `go test ./... -race
  -count=1`, matching `acthur test --ci` and preventing cached/non-race
  results from satisfying production preflight. Green evidence: focused race
  tests for contract, deploy, and CLI behavior.
- 2026-07-14 — canonical-scaffold TDD agent — captured a black-box CLI red
  proving `acthur new widgets --adapter go:fiber --module
  example.com/widgets` failed outside a terminal by demanding `--db`, even
  though omission was the only natural way to select no database. Made
  adapter and module the complete required non-interactive answer set and
  treated `--db` as an opt-in. A second red during the cycle showed the
  resolver still consumed non-interactive EOF as the interactive default
  “yes”; database prompting is now restricted to interactive sessions.
  Interactive prompting and its default-yes behavior remain unchanged. The
  black-box regression builds and invokes the real CLI, loads the generated
  `acthur.yml`, and proves no database node exists. Focused race tests are
  green.
- 2026-07-14 — Coolify-env TDD agent — verified against Coolify's official
  API reference that application variables are upserted with `PATCH
  /api/v1/applications/{uuid}/envs` using `key`, `value`, and optional literal
  flags, and that write plus deploy token permissions are required. Captured
  reds showing both the missing client operation and the projection gate's
  blanket rejection. Added a narrow `WorkloadEnv() map[string]string` target
  context populated only from Compose-referenced keys after the existing env
  gate passes. The target sorts and upserts those keys before triggering the
  deployment, logs only the count, keeps literal values out of Compose, and
  sanitizes upsert failures to key plus HTTP status because an API response
  may reflect submitted values. Coolify now passes remote projection env
  validation; Fly, Railway, and Render remain fail-closed. Client, target,
  projection, and command behavior tests prove exact key/value delivery,
  ordering before deploy, and absence of values from Compose/logs/errors.
- 2026-07-14 — Coolify-service API TDD agent — followed the adjacent
  production blocker exposed by official Coolify v4.1 release notes: the
  deprecated Docker Compose application endpoint was removed and current
  clients must use `POST /api/v1/services`. Captured a compile-time red for
  the missing typed service operation, then migrated the entire lifecycle to
  `GET/POST/PATCH /api/v1/services`, service environment upserts,
  `POST /services/{uuid}/start`, and `GET /services/{uuid}` health polling.
  Creation now carries the officially required project UUID, server UUID,
  environment name, stack name, and raw Compose; updates remain idempotent by
  project/name and redeploy through the same service UUID. Added
  `environments.<name>.server_uuid` to the typed config and target context,
  with a pointed failure when absent. Removed production legacy application
  paths and updated client, target, and command black-box tests to accept
  only current service routes, assert exact create/update/env/start/status
  ordering, and retain secret-safe errors. Updated the PRD configuration and
  deployment projection description to match shipped behavior. Live-instance
  verification remains required because current English service endpoint
  detail pages were not discoverable through Coolify's official docs index;
  the official changelog and deprecated endpoint page do explicitly mandate
  the service migration.
- 2026-07-14 — main-agent Coolify 4.1.2 route audit + Coolify-service agent
  — resolved the remaining API ambiguity against an installed current
  controller: service env `PATCH` updates existing keys only, while `POST`
  creates missing keys, and service start acknowledges queueing with a
  message rather than a deployment UUID. The client now lists service envs,
  chooses POST/PATCH for a true idempotent upsert, sanitizes failures from
  both phases, and accepts the documented start acknowledgement before
  polling service health. Tests cover missing-key POST, existing-key PATCH,
  env-before-start ordering, and the exact acknowledgement shape.
- 2026-07-14 — Coolify-lifecycle TDD agent — audited the installed Coolify
  4.1.2 routes/OpenAPI and captured behavior-first reds at the public config,
  client, target, and CLI seams. Deploy now preserves optional
  `destination_uuid`, explicitly ensures the requested project environment
  before service creation, and exposes the retained read-only `acthur deploy
  status --env <name>` operation for the exact environment-scoped service.
  Added confirmed non-production `acthur deploy cleanup --env <name>
  --confirm`: it deletes only the exact service with configurations, volumes,
  networks, and Docker cleanup enabled; waits for Coolify's queued deletion;
  removes the now-empty environment; treats missing resources/404 as success;
  refuses production; and never deletes the shared project. Provider and CLI
  request-sequence tests prove identity, ordering, read-only status, safety,
  and idempotency.
- 2026-07-14 — remote-topology/secrets TDD agent — captured a black-box CLI
  red proving that locally validated secret values were followed by artifact
  writes and provider calls even though no remote target delivered those
  values to workloads. Added a fail-closed remote projection preflight before
  writes or target side effects: every remote target rejects required workload
  environment until it has a real environment-delivery API, naming keys but
  never values. Fly, Railway, and Render also reject infrastructure nodes and
  `depends_on` topology because their current targets deploy service images
  only; Coolify remains topology-capable through the complete Compose
  projection. Service-only, environment-free provider projections remain
  supported. CLI behavior tests prove no artifact/Docker/provider side effects
  and no literal secret leakage. Focused and full `go test -race -count=1
  ./...`, full `go vet ./...`, and `git diff --check` are green in the
  accumulated workspace.
- 2026-07-14 — release supply-chain TDD agent — added native black-box
  installer CI on Linux, macOS Intel, and Windows PowerShell; the PowerShell
  suite covers checksum mismatch, missing manifest entry, and a valid archive
  whose installed binary reports the requested version. GoReleaser now emits
  SPDX JSON SBOMs, keylessly signs the checksum manifest with a Cosign v3
  Sigstore bundle, and no longer attempts nonexistent Homebrew/Scoop
  repositories. The tag job has least-privilege release/OIDC/attestation
  permissions and GitHub-attests archives, checksums, and SBOMs. Non-tag CI
  validates the config and builds a full unsigned snapshot. Local green
  evidence: Unix installer black-box suite, GoReleaser 2.17 config check and
  five-platform archive/SBOM/checksum snapshot, actionlint 1.7.12, and diff
  check. Native Windows execution remains to be proven by the pushed CI job.
- 2026-07-14 — main agent — live-witnessed the canonical generated project.
  Noninteractive scaffold, graph and contract validation, deploy-baseline
  locking, docs/AI-context generation, and `acthur test --ci` passed. Native
  development served health directly and through the graph proxy, rebuilt
  after a Go source edit, and served health again. The initial shutdown
  witnesses exposed reload descendants; after the fourth behavior-first fix,
  the exact Air v1.63.4 rerun completed Ctrl+C cleanup with no generated-project
  processes and no listeners remaining on service port 18081 or proxy port
  4000.
- 2026-07-14 — main agent — live-witnessed the integrated local production
  Compose path twice (initial deploy and redeploy). Every named gate passed,
  including contract compatibility and exact race-enabled tests; PostgreSQL
  reached healthy state and answered `SELECT 1`; workload environment and
  secret variables were present without printing their values; the secret was
  absent from Compose logs; `/health` passed after both deploys; and teardown
  removed the API, database, network, and volume. An initial attempt on the
  generated port 8080 failed closed because that host port was already owned;
  the isolated witness used port 18080.
- 2026-07-14 — reload-descendant shutdown TDD agent — the canonical live
  witness found a hot-reloaded service child still listening after Ctrl+C
  reported all services stopped. The first regression only covered a survivor
  in the supervisor's process group and its fix was disproved by the exact Air
  v1.63.4 witness: Air's rebuilt shell/application escaped that group. Reopened
  TDD with a deterministic Linux regression whose reload child creates its own
  session. The resulting stop-time ancestry snapshot also failed the second
  exact witness: Air had already exited and its shell/application had PPID 1
  before shutdown began, so there was no ancestry left to discover. The third
  red regression makes that ordering load-bearing by exiting the supervisor
  before calling `Stop`. Each managed process now receives a random opaque
  ownership marker inherited through its workload tree; Linux shutdown finds
  exact marker matches through `/proc/*/environ`, captures PID plus kernel
  start time to prevent PID-reuse mistakes, gracefully signals them, and
  force-reaps survivors regardless of PPID, process group, or session changes.
  Ancestry/group cleanup remains the cross-platform fallback. The pre-orphaned
  regression passes under `-race -count=20`. The third exact witness then
  exposed another load-bearing transition: the orphan preceded an Acthur
  supervisor auto-restart, and final shutdown's current-generation owner sweep
  still missed it. A full Manager/Supervisor regression now creates an escaped
  generation-one orphan with a differing process-owner value, waits for
  automatic generation-two restart, then invokes `StopAll`; it was red before
  the fourth fix. Managers now inject a separate opaque manager-lifetime marker
  inherited by every supervised generation, and `StopAll` performs a final
  marker-based sweep after all per-process stops. The auto-restart regression
  passes under `-race -count=10`; escaped-session and existing lifecycle tests,
  Windows cross-compilation, full `go test -race -count=1 ./...`, full vet, and
  diff checks are green. The exact fourth Air witness passed: source rebuild,
  direct and proxy health, graceful Ctrl+C, zero remaining witness processes,
  and zero listeners on ports 18081/4000.
- 2026-07-14 — main agent — pushed the integrated candidate to `dev` and
  closed the Windows-native reds exposed by the new CI gates. The PowerShell
  harness now observes expected nonzero child exits without terminating its
  parent; installer failures throw (so dot-sourced/`iex` use cannot print an
  error and return success); multi-line version output is normalized before
  matching; and the scaffold black-box test executes the native `.exe` path.
  GitHub Actions run 29341764155 is fully green: lint/vet, race-enabled tests
  on Go 1.22 and 1.23 across Ubuntu, macOS Intel, and Windows, installer
  integrity tests on all three operating systems, snapshot binaries, and the
  GoReleaser/Syft release dry run.
- 2026-07-14 — main agent — promoted the reviewed candidate through the
  required `dev` → `staging` tier in PR #71. The pull-request run
  29342222528 and post-merge `staging` push run 29342678221 are fully green,
  including lint/vet, the complete race-enabled supported OS/Go matrix,
  native installer integrity tests, snapshot binaries, and the
  GoReleaser/Syft release dry run. `main` and a release tag remain deliberately
  unchanged until the real remote staging witness succeeds.
- 2026-07-14 — Coolify health-gate TDD agent — reproduced at the public
  `WaitServiceHealthy` seam that Coolify status `running:unhealthy` was
  incorrectly accepted because the running-state check preceded terminal
  health failures. Added a behavior regression proving an unhealthy running
  service fails with its status identified, then gave terminal failure markers
  precedence over running success. Focused `go test -race -count=1
  ./internal/deploy/coolify`, focused `go vet`, and `git diff --check` are green.
- 2026-07-14 — main agent — treated GitHub's native-runner Node 20
  deprecation warnings as the failing CI behavior and upgraded the affected
  official actions to their current Node 24 majors: checkout v6, setup-go v6,
  upload-artifact v7, golangci-lint-action v9, and goreleaser-action v7.
  Actionlint 1.7.12 and `git diff --check` are green; pushed native-runner
  evidence is required before staging promotion.
- 2026-07-14 — main agent — the upgraded CI witness exposed dependency drift
  at the generated-project seam on Go 1.22: `go mod tidy` selected Prometheus
  client v1.23.2, which requires Go 1.23, so the retained Go 1.22 project could
  no longer compile with the observability plugin. The existing black-box
  generated-project compilation test was the red. The go:fiber scaffold now
  pins Prometheus client v1.19.1 (whose module requires Go 1.20), preserving
  the supported Go 1.22 floor instead of relying on an unstable latest-module
  resolution.
- 2026-07-14 — main agent — the follow-up Windows Go 1.22 race job exposed a
  concurrent map read/write in the public `output.ServiceLog` path while the
  process manager streamed multiple services. Added a concurrent public-seam
  regression and removed the unnecessary mutable color cache: service colors
  remain the same deterministic name hash with no shared map mutation.
- 2026-07-14 — main agent — pushed the CI/runtime hardening to `dev` at
  `329be4f`, merged PR #72 (`dev` -> `staging`) as merge commit `0754ca7`,
  and verified the post-merge staging push run 29346984175. Dev run
  29345788624, PR #72 run 29346371930, and staging run 29346984175 are fully
  green across lint/vet, Go 1.22 and 1.23 race-enabled tests on Ubuntu,
  macOS Intel, and Windows, native installer integrity tests, snapshot
  binaries, and the GoReleaser/Syft release dry run. `main` and release tags
  remain intentionally unchanged until the disposable remote Coolify witness is
  executed and recorded.

## Current Decisions

- Production artifact projection is fail-closed. A graph node may be excluded
  only through a future explicit deploy-scope declaration, never by silence.
- Supported-platform tests must express platform-native security semantics;
  POSIX mode bits are not used as a proxy for Windows ACLs.
- Release version injection targets the executable symbol `main.Version`.
- The agreed behavior-test seams are CLI exit/output, generated applications
  as black boxes, process/dev lifecycle callbacks, deploy artifact/target
  interfaces, and release binaries/installers.
- Branch promotion is `dev` → `staging` → `main`; CI validates pushes and pull
  requests at every tier, and `main` remains the live production branch.
- The first release distributes through GitHub Releases and the checksum-
  verifying installers. Homebrew and Scoop are deferred until their dedicated
  repositories and credentials exist.
- Release trust is keyless: Cosign signs the checksum manifest with GitHub OIDC
  and a Sigstore bundle, GoReleaser emits per-archive SPDX SBOMs, and GitHub
  attests the archives, checksum manifest, and SBOMs. No long-lived signing key
  is required.

## Open Questions

- Coolify is selected for the first live remote witness. Execution requires a
  disposable remote instance/server, its public route, and a short-lived token
  with `read`, `write`, and `deploy` permissions.

## Files/Modules Expected

- `internal/process/`
- `internal/deploy/` and `internal/deploy/artifacts/`
- `cmd/acthur/`
- `.github/workflows/ci.yml`, `.goreleaser.yml`, `Makefile`, installers
- PRD, roadmap, audit, and release documentation

## Acceptance Criteria

- [x] `go test ./... -race -count=1` is green on every supported OS/Go pair.
- [x] Lint, vet, snapshot, and release-dry-run gates are green.
- [x] A canonical generated project completes scaffold → validate → dev →
      hot reload/restart → contracts/generate/test → deploy.
- [x] Deployment refuses every incomplete graph projection.
- [x] The full pre-deploy gate required by the retained PRD runs and aggregates
      pointed failures.
- [ ] At least one remote target is live-witnessed end-to-end, including
      infrastructure, dependency environment, health, redeploy, and cleanup.
- [ ] Release artifacts verify integrity and report the correct version on
      supported platforms.
- [x] PRD and roadmap clearly distinguish shipped, experimental, and future
      behavior with no contradictory first-release claims.
- [x] PR #66 and the follow-up staging promotion PRs #71 and #72 are merged
      with green supported-platform checks.
- [ ] Campaign issue #69 is reconciled and closed only after remote and public
      release evidence exists.
- [ ] `main` is fast-forwarded and pushed, and the first production release is
      tagged, published, installed, and verified.

## Risks

- Provider witnesses require external credentials and disposable resources.
- The first-release signing policy is decided and keyless; Homebrew/Scoop
  repositories remain explicitly deferred and do not block GitHub Releases.
- The expanded PRD contains mutually inconsistent version and size claims;
  release truthfulness requires explicit reconciliation rather than silently
  treating aspirational sections as delivered.
