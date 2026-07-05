# Implementation: Phase 4 — Contract Engine (enforce-on-the-edge)

## Goal
Contracts enforce safety on `data_flow` edges during dev: a violation shows in the proxy log (dev mode) or blocks with 422 (`--strict`), and `acthur contract diff` correctly classifies breaking changes per PRD §11.5.

## Owning Docs
- `docs/prd/phase-4-contract-engine.md` · ADR 0012 (accepted: Option A — proxy interception via `/_flow/<from>/<to>`) · tracker #35, slices #36 #37 #38

## Status
Completed (2026-07-05) — closing commit `c606d76`.
- 2026-07-05 — main — ADR 0012 resolved (Option A), PRD written, slices filed.
- 2026-07-05 — subagent slice 3 (#38, Sonnet worktree) — §11.5 rules verified row-by-row; constraint-tightening (max/min) detection added; importers now error honestly. Merged `22c6b07`.
- 2026-07-05 — subagent slice 1 (#36, Sonnet worktree) — `contract.LoadDir`, `graph.ContractResolver` (`unloadable-contract` rule), `contract validate|list|show|diff` CLI. Merged `1f6e870`.
- 2026-07-05 — subagent slice 2 (#37, Sonnet worktree) — `/_flow` proxy routes + enforcement (dev-log / strict-422), `resolveNodeEnv` flow URLs, `--strict` flag. Merged.
- 2026-07-05 — main — dev↔registry integration (`c606d76`); live witness passed both done-criteria.

## Current Decisions
- `data_flow` discovery env is a proxy flow URL (`http://localhost:<devPort>/_flow/<from>/<to>`); `depends_on` keeps direct URL + `Connectable` env. A `data_flow` edge never receives Connectable env.
- Contract files live at `contracts/<name>.contract.yml`; bare names and qualified relative paths both resolve.
- `contract diff` exits `ExitContractBreak` (10) on breaking changes.
- Nil registry = enforcement off; flow routes still forward.

## Open Questions
- Enforcement depth: `ValidateRequest` checks auth header + required input fields only — no response validation, no type/constraint checking. Deepen when the validator grows (candidate for a Phase 4.5 slice or alongside Phase 7 generated middleware).
- Multi-contract edges: enforcement uses the first contract (`ContractFor` semantics).

## Files/Modules Expected
`internal/contract` (loader, diff), `internal/proxy` (flow routes, enforcement), `internal/engine` (env injection, options), `internal/graph` (ContractResolver), `cmd/acthur`.

## Acceptance Criteria
- [x] Contract violation shows in the proxy log on a live `acthur dev` run (`[proxy] ⚠ contract violation: web→api contract "users": …`)
- [x] `--strict` returns structured 422 without forwarding; conforming requests pass byte-identical
- [x] `acthur contract diff` identifies breaking changes (exit 10 on breaking fixture pair)

## Risks
Proxy-hop enforcement is dev-only by design (ADR 0012); production-path enforcement waits for Phase 7 generated middleware — do not assume prod traffic is checked.
