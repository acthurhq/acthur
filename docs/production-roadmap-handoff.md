# Production Roadmap Handoff

**Date:** 2026-07-14
**Repository:** `acthurhq/acthur`
**Working branch:** `dev` at `13cd242`
**Target branch:** `main`
**Primary pull request:** [#66 — PRD completion: Phases 3–9 + end-to-end production-readiness verification](https://github.com/acthurhq/acthur/pull/66)

## Purpose

This document hands off the current production-readiness audit and the work needed to turn the implementation on `dev` into a defensible release. It reconciles the PRD, code, tests, GitHub issues and pull requests, and the surviving agent worktrees.

The central conclusion is:

> `dev` is a substantial pre-release implementation, but it is not production-ready or release-ready end-to-end.

It is suitable for continued development, scaffolding experiments, local graph/dev workflows, and local Compose evaluation. It should not yet be described as a production-ready v1 or merged to `main` without stabilizing tests and CI, tightening deployment completeness, validating release artifacts, and reconciling the claimed PRD scope.

## Repository State at Handoff

- The primary worktree is switched to `dev` and is clean against `origin/dev`.
- `dev` is 132 commits ahead of `main`; `main` has no commits absent from `dev`.
- The `main..dev` change contains approximately 343 changed files, 37,504 insertions, and 544 deletions.
- There are no Git tags and no GitHub releases.
- PR #66, which carries `dev` into `main`, remains open.
- PR #66 has no recorded review approval and its latest check set is not green.
- All GitHub issues through the PRD-completion campaign are closed, but issue closure does not represent release acceptance.

## Verification Performed

The audit independently inspected the PRD, roadmap, ADRs, implementation notes, source tree, tests, Git history, remote branches/worktrees, GitHub issues, pull requests, and release configuration.

Commands run on `dev` included:

```text
go test ./...
go build ./...
go vet ./...
go run ./cmd/acthur --help
make build
```

Observed results:

- `go build ./...` passed.
- `go vet ./...` passed.
- The CLI builds and exposes the expected top-level command tree.
- `go test ./...` failed in `internal/process.TestProcess_OutputCapture`.
- `make build` succeeds, but its stale `-X` package path does not inject the version; the resulting binary still reports `Version: dev`.
- A normal unstripped local CLI binary was approximately 17 MB, which conflicts with the PRD's stated `<500 KB` kernel-binary constraint.

## Delivered Capability by PRD Phase

| Phase | Current verdict | Delivered capability | Remaining qualification |
|---|---|---|---|
| 0 — CLI Skeleton | Substantially complete | Buildable CLI, full command tree, help/version surfaces | Release/version stamping and public artifacts are not ready |
| 1 — Graph Engine | Complete enough | Typed graph, validation tiers, topology ordering, lifecycle sealing, materialized nodes, state subscriptions | Keep regression coverage green across supported platforms |
| 2 — Adapter System | Core complete | Fiber/Postgres plus Chi, Gin, Axum, Fastify, Astro, and Next implementations | PRD ecosystem matrix is broader than current registry |
| 3 — Dev Runtime | Substantial, not release-ready | Process supervision, proxy, health checks, connection env, watcher, DNS preflight, scoped containers, logs | Process-output test is unstable/failing; repeatable full E2E evidence is incomplete |
| 4 — Contract Engine | Substantially complete | Native contracts, importers, registry, diffing, CLI, proxy request/response enforcement | Live violation and protocol coverage should become repeatable release tests |
| 5 — Plugin System | Substantially complete | Event bus, `KernelAPI`, loader, hooks, commands, generators | Plugin discovery outside a project remains weak |
| 6 — First Plugins | Implemented for Fiber | Migrations, auth, RBAC, multitenancy, plus ecosystem plugin packages | Advanced auth, runtime isolation, observability, and ACME behavior are not comprehensively proven |
| 7 — Generator Engine | Core implemented | Locking/idempotency, model and contract generation, migration-range safety, tests, CI/docs/context generators | Cross-platform generation and broader adapter output need release gates |
| 8 — Deploy Runtime | Partial | Local Compose path and Coolify/Fly/Railway/Render target implementations | Remote providers are not live-proven; projection may silently omit nodes |
| 9 — Ecosystem Expansion | Partial | Major additional commands, adapters, plugins, AI/MCP, and provider clients | Several adapters, analytics, wizard modes, package-manager choices, and production witnesses remain missing |

## Defects Already Fixed on `dev`

The original audit findings must not be repeated as current blockers where later commits fixed them:

- Project-scoped infrastructure health checks now use the same container name as `docker run` (`11647c5`).
- Contract-generated migrations now fail on collisions with plugin-reserved ranges (`b6676a3`).
- Production Dockerfiles derive the Go toolchain version from the node's actual `go.mod` (`d1a077c`).
- `acthur db studio` catches SIGTERM and cleans up its Adminer container (`0bffb31`).
- The canonical module, release, and installer namespace was migrated to `github.com/acthurhq/acthur` (`826c6e0`).
- Windows-safe generated-lock paths, LF template checkout rules, CI matrix behavior, and Windows process-test compilation received follow-up fixes.

## Current Production Blockers

### P0 — Make the Test and CI Gates Green

Local `go test ./...` fails at `internal/process.TestProcess_OutputCapture`. The test launches an echo process, waits a fixed 500 ms, and sometimes observes an empty captured-output slice. This is directly related to Phase 3 supervision and log routing, so it cannot be dismissed as an unrelated test-only defect.

Required outcome:

- Determine whether the race is in process output draining, process completion, log routing, or test synchronization.
- Replace fixed-time waiting with deterministic lifecycle/output synchronization.
- Run `go test ./... -race -count=1` on all supported Go/OS matrix entries.
- Rerun PR #66 checks and require every mandatory job to finish successfully.

PR #66 currently contains mixed Linux results, failed Windows jobs, macOS jobs cancelled after extended execution, and skipped snapshot/release jobs.

### P0 — Establish an Honest Deploy Completeness Rule

`internal/deploy/artifacts.Project` currently continues when:

- an adapter cannot be resolved;
- a service adapter does not implement `Dockerizable`; or
- an infrastructure adapter does not implement `Containerized`.

This allows deployment artifacts to omit declared graph nodes silently. That conflicts with the PRD promise that the pre-deploy gate refuses to ship a broken system.

Required product decision and implementation:

1. Default to an error for every deploy-scoped node that cannot be projected; and
2. if partial deployment is required, add an explicit manifest concept that marks a node out of the selected deploy projection.

Silence should not represent intent.

### P0 — Prove the Release Path

There are no tags or GitHub releases, so the installer and release declarations have not been exercised as a public delivery chain.

Required outcome:

- Fix the Makefile's obsolete linker symbol path from `github.com/acthur/acthur/...` to `github.com/acthurhq/acthur/...`.
- Verify version injection for both Makefile and GoReleaser builds.
- Produce a release candidate tag and snapshot artifacts for every supported target.
- Exercise Linux/macOS shell installation and Windows PowerShell installation against real release artifacts.
- Verify or create the declared Homebrew tap and Scoop bucket repositories.
- Decide whether the `<500 KB` PRD binary constraint is real; meet it or amend the PRD with an evidence-based target.

### P1 — Live-Witness Remote Production Deployment

The local Docker Compose path has a recorded real Docker/Postgres witness. Fly, Railway, Render, and Coolify have implementations and fake/injectable-client tests, but the repository lacks reproducible evidence of a successful real-provider deployment.

Required outcome:

- Select the first supported production target.
- Deploy a canonical example using documented credentials and least-privilege setup.
- Verify health, routing, secrets, database connectivity, redeploy, status, failure reporting, and teardown.
- Record a sanitised repeatable witness in repository documentation or CI.
- Do not claim all provider targets production-ready based only on client-unit tests.

### P1 — Reconcile PRD Scope with the Actual Product

Notable missing or partial claims include:

- `ui:vue`, `ui:svelte`, and other claimed frontend adapters;
- Redis and broader infrastructure adapters expected by the v1 roadmap;
- analytics plugins;
- full non-destructive `acthur init` stack detection/adoption;
- richer project architecture modes and full-stack wizard selection;
- JavaScript package-manager selection instead of hard-coded npm assumptions;
- advanced auth/runtime proof such as OAuth matrices, MFA/passkeys, and session behavior;
- runtime data-isolation proof for multitenancy;
- a complete observability stack witness;
- production ACME automation;
- deploy rollback/status behavior promised by the expanded PRD;
- comprehensive examples and the external ecosystem repositories described by the roadmap.

For every item, either implement and witness it or explicitly move it to a later version. Avoid keeping aspirational v1 claims marked complete.

### P2 — Correct Documentation Drift

- `CONTRIBUTING.md` links to nonexistent `docs/PRD.md`; the actual file is `docs/acthur-prd.md`.
- `docs/roadmap.md` contains an older Phase 0-era subsystem table and obsolete repository ownership references.
- The PRD, roadmap, and contributing guide retain `samueloshio` namespace references after `acthurhq` became canonical.
- `docs/audit.md` mixes original findings and later updates, making resolved defects look current unless the reader carefully reconciles both sections.
- The issue tracker marks the completion campaign closed while the implementation PR remains unmerged and CI remains unsuccessful.

Required outcome:

- Make this handoff and a revised roadmap the concise current source of truth.
- Preserve the historical audit as history, but add a clearly dated current-status summary.
- Update links and namespace references after deciding which references are historical issue links versus current repository ownership.

## Agent Worktree Disposition

### Completed and Incorporated

`worktree-agent-aeb5f2f5bd72240ea` is fully contained in `dev`. Its tip is the deploy-target wiring associated with issue #64. No merge action is required.

### Obsolete Namespace-Migration Attempts

The following worktrees each contain one unmerged namespace/import migration commit, have no associated pull request, and are substantially behind `dev`:

- `worktree-agent-a066ed7da785f18cc`
- `worktree-agent-a0808583ee16135c5`
- `worktree-agent-a625fae97484cea44`
- `worktree-agent-a8bfa09c63ee47726`
- `worktree-agent-a9fbf93fdc015442c`
- `worktree-agent-aa02558092e19d708`
- `worktree-agent-aaad08f6a74ac851d`
- `worktree-agent-abe2e6029e4ffa560`
- `worktree-agent-abe6004a9981b4288`
- `worktree-agent-ad610227ce25b2ab8`

Do not merge these branches wholesale. `dev` already contains the canonical namespace migration and later fixes. They may be archived/deleted after confirming no unique non-namespace edit is needed.

### CI Scratch Branches

- `ci-fix-scratch-1`
- `ci-fix-scratch-2`

Both carry the same three scratch commits. Their intended changes have equivalents on `dev`. PR #67 is closed, and draft PR #68 explicitly says it should be closed without merging. Close PR #68 and retire both branches after final verification.

## GitHub Issue and PR Interpretation

All listed issues through #65 are closed. In particular, issue #53 was closed after the local production-readiness witness and four targeted fixes. Its closure comment also acknowledged that:

- the process-output test remained flaky;
- unsupported deploy projection remained a product decision; and
- closure represented completion of the implementation campaign.

Therefore, issue status should be interpreted as **campaign work recorded**, not **production release accepted**.

Relevant pull requests:

- PR #66: open, `dev` into `main`, not green, primary release-integration candidate.
- PR #67: closed scratch CI iteration; do not merge.
- PR #68: open draft scratch CI iteration; close without merging after confirming `dev` contains the fixes.
- Earlier PRs #10–#16, #23, #28, and #34 are merged historical phase work on `main`.

## Recommended Execution Order

1. Fix `internal/process.TestProcess_OutputCapture` and its underlying synchronization defect.
2. Run the full race-enabled test matrix and resolve Windows/macOS failures or hangs.
3. Fix Makefile version stamping and prove release-candidate artifacts/installers.
4. Decide and implement explicit deploy projection completeness semantics.
5. Live-witness one remote production target and define which targets are experimental.
6. Update PR #66 from the resulting `dev`, obtain review, and require green checks.
7. Reconcile PRD/roadmap claims with the release candidate; move unfinished scope to explicit later milestones.
8. Merge PR #66 only after the above gates pass.
9. Tag a release candidate, validate installation and rollback paths, then publish the first release.
10. Close/archive obsolete scratch PRs, branches, and worktrees.

## Definition of Production-Ready for the Next Handoff

The next production-readiness review should require all of the following:

- `go test ./... -race -count=1` is green on the supported OS/Go matrix.
- Lint, vet, snapshot build, and release-dry-run jobs finish successfully.
- A fresh canonical project can scaffold, validate, run, hot-reload, generate, test, and deploy through documented commands.
- Deployment fails clearly if any deploy-scoped node cannot be projected.
- At least one real remote target is witnessed end-to-end.
- Release artifacts install and report the correct version on supported platforms.
- The current PRD and roadmap distinguish shipped, experimental, and future scope.
- PR #66 has review approval, green required checks, and no unresolved production blocker.

Until those gates are satisfied, use **pre-release** or **developer preview** language rather than **production-ready**.
