# Implementation: Phase 5 — Plugin System (wire-the-kernel)

## Goal
The dormant `internal/plugin` system becomes real: the engine emits kernel lifecycle events, the CLI loads the `plugins:` list from `acthur.yml` through a real `KernelAPI`, and a test plugin's hook, command, and generator all fire at the correct lifecycle points under a live `acthur dev`.

## Owning Docs
- `docs/prd/phase-5-plugin-system.md` (spec) · ADRs 0003, 0005 · tracker issue #39

## Status
In-flight
- 2026-07-05 — main — PRD + this note created; slices being filed and delegated to Sonnet worktree agents.
- 2026-07-05 — subagent slice 1 (#40, Sonnet worktree) — DevEngine gains `WithBus(*plugin.Bus)`; startInfraNode/startServiceNode emit before_start/after_start/after_healthy/on_failure at the documented points, shutdown emits before_stop/after_stop in reverse order for real (non-`kernel:`) nodes only; kernel-materialized nodes (proxy) emit nothing on either path. Bus.Emit's panic recovery now logs via `output.Warn` (was a raw `fmt.Printf`) instead of adding a new recovery wrapper — recovery already existed. Nil bus is a no-op (zero overhead). 12 new behavior tests at the engine seam plus 1 in internal/plugin covering the output.Warn routing; full suite green.

## Current Decisions
- Event emission is synchronous with per-handler panic recovery; a broken plugin handler must never take down `acthur dev`.
- Plugins load at CLI assembly (ADR 0005 pattern), mutate the graph pre-seal only (ADR 0003).
- Generator/schema/middleware registrations are stored in kernel registries this phase; Phase 7 consumes them.

## Open Questions
- Where middleware registrations attach in dev (proxy chain?) — parked until a consumer exists.

## Files/Modules Expected
`internal/engine` (bus emission), `internal/plugin` (KernelAPI impl, test plugin), `cmd/acthur` (loading, plugin list command).

## Acceptance Criteria
- [ ] Test plugin's hook fires at `kernel:node:after_healthy` during a live `acthur dev` (line visible in output)
- [ ] Test plugin's command appears in `acthur --help` and runs
- [ ] Test plugin's generator target is registered and introspectable
- [ ] Unknown plugin name in `acthur.yml` fails before startup with a pointed error
- [ ] Full suite green; no plugin handler can panic the kernel

## Risks
The plugin package predates Phases 2–4 conventions (like the old engine did) — expect small honest-reconciliation fixes (e.g. global registry vs injected resolver) rather than drop-in wiring.
