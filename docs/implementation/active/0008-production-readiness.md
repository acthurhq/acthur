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

## Open Questions

- Which remote provider will be the first live production witness?
- What signing identity/trust policy will be used for release artifacts?
- Will Homebrew/Scoop distribution ship in the first release or move to a
  later explicitly scoped milestone?
- Which internally conflicting PRD scope and binary-size claims are retained
  for the first release?

## Files/Modules Expected

- `internal/process/`
- `internal/deploy/` and `internal/deploy/artifacts/`
- `cmd/acthur/`
- `.github/workflows/ci.yml`, `.goreleaser.yml`, `Makefile`, installers
- PRD, roadmap, audit, and release documentation

## Acceptance Criteria

- [ ] `go test ./... -race -count=1` is green on every supported OS/Go pair.
- [ ] Lint, vet, snapshot, and release-dry-run gates are green.
- [ ] A canonical generated project completes scaffold → validate → dev →
      hot reload/restart → contracts/generate/test → deploy.
- [ ] Deployment refuses every incomplete graph projection.
- [ ] The full pre-deploy gate required by the retained PRD runs and aggregates
      pointed failures.
- [ ] At least one remote target is live-witnessed end-to-end, including
      infrastructure, dependency environment, health, redeploy, and cleanup.
- [ ] Release artifacts verify integrity and report the correct version on
      supported platforms.
- [ ] PRD and roadmap clearly distinguish shipped, experimental, and future
      behavior with no contradictory first-release claims.
- [ ] All campaign tasks are complete/closed; PR #66 is reviewed and green.
- [ ] `main` is fast-forwarded and pushed, and the first production release is
      tagged, published, installed, and verified.

## Risks

- Provider witnesses require external credentials and disposable resources.
- Distribution repositories and signing policy may require owner decisions.
- The expanded PRD contains mutually inconsistent version and size claims;
  release truthfulness requires explicit reconciliation rather than silently
  treating aspirational sections as delivered.
