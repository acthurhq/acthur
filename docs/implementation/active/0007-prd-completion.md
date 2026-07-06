# Implementation: PRD Completion — audit-gap closure (Phases 3/4/6/8 leftovers + Phase 9)

## Goal
Close every gap `docs/audit.md` (2026-07-06) identified between the PRD and the implementation, in dependency order: Phase 3 integration (doctor-in-dev, watcher wiring, DNS scoping, service commands), Phase 4 (importers, response validation), Phase 6/8 leftovers (db subcommands, acthur test, acthur build), then Phase 9 (new/init wizard, adapters, generate surfaces, secrets/flag/monitor, mcp/agent, deploy targets, ecosystem plugins). Every stub `notImplemented(N)` either gains a real implementation or is explicitly descoped in docs with the PRD updated.

## Owning Docs
- `docs/audit.md` (gap inventory) · `docs/acthur-prd.md` §21 · tracker issue #53

## Status
In-flight
- 2026-07-06 — main — Audit triaged: Phase 8 findings were stale (deploy wired + witnessed in c9e41ce/824fe9c; suite green), rest confirmed. Stale tracked `acthur` binary removed. Wave 1 delegated to parallel Sonnet worktree agents.

## Current Decisions
- Waves: (1) Phase 3+4+6/8 leftovers — three agents; (2) Phase 9 split across parallel agents. Merge + full suite + witness between waves.
- Conflict hotspot `cmd/acthur/commands.go`: each agent owns only its command block; orchestrator resolves import/status-note conflicts on merge.
- Anything genuinely unimplementable here (e.g. live VPS legs) is descoped explicitly in docs + tracker, never silently.

## Open Questions
- DNS auto-setup strategy (hosts file vs resolver) — wave 1 Phase 3 agent decides and records here.

## Acceptance Criteria
- [ ] No `notImplemented(3|4|5|6|8)` remains; each former stub has behavior tests
- [ ] `acthur dev` runs doctor preflight; watcher-driven reload cascade proven; service logs/restart/health work live
- [ ] Contract importers accept .openapi.yml/.proto/.graphql; proxy validates responses
- [ ] Phase 9 surfaces implemented (or explicitly descoped in docs): new/init, adapters, generate ai-context/ci/docs/skill, graph visualize, secrets/flag/monitor, mcp serve, agent, deploy targets, ecosystem plugins
- [ ] Full suite green; live witnesses for dev-runtime changes and new/init
