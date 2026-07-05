# Implementation: Phase 5 — Plugin System (wire-the-kernel)

## Goal
The dormant `internal/plugin` system becomes real: the engine emits kernel lifecycle events, the CLI loads the `plugins:` list from `acthur.yml` through a real `KernelAPI`, and a test plugin's hook, command, and generator all fire at the correct lifecycle points under a live `acthur dev`.

## Owning Docs
- `docs/prd/phase-5-plugin-system.md` (spec) · ADRs 0003, 0005 · tracker issue #39

## Status
In-flight
- 2026-07-05 — main — PRD + this note created; slices being filed and delegated to Sonnet worktree agents.
- 2026-07-05 — subagent slice 2 (#41, Sonnet worktree) — landed `internal/plugin/kernelapi.go` (`KernelAPIImpl`: OnEvent→bus, RegisterCommand→cobra adapter, RegisterGenerator/RegisterSchema/RegisterMiddleware→in-memory kernel registries with accessors, AddNode/AddEdge→graph, Graph()→`*graph.Graph` directly since it already satisfies `GraphReader`, Log→pluggable logFunc). Wired plugin loading into `cmd/acthur`'s `loadGraph()`: `cfg.Plugins` names are checked against the plugin registry up front (unknown name → pointed error naming the plugin + listing every registered plugin, `ExitPluginError`), then `plugin.Load` runs in DependsOn order against a shared package-level `kernelBus`, then `g.Freeze()` seals the graph. Added `acthur plugin list` (loaded vs. available, name+version). Seal-timing finding: `graph.Build` never sealed the graph in production — `Freeze()` was only ever called from `graph_test.go`; this is the first production call site, closing the ADR 0003 gap. Behavior tests in `internal/plugin/kernelapi_test.go` (every KernelAPI method, pre/post-seal AddNode panic) and `cmd/acthur/plugin_loading_test.go` (fixture plugin exercising all registrations end-to-end, unknown-plugin error, DependsOn load order). Full suite green. Deviation: manually ran `acthur plugin list` against `testdata/vetangle` (which declares `plugins: [migrations, auth]`) and confirmed it now fails cleanly with the unknown-plugin error, since no capability plugins are registered yet (Phase 6, out of scope) — expected, not a regression.

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
