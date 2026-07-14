# PRD — Phase 5: Plugin System (wire-the-kernel)

> **Status:** ready-for-agent · **Phase:** 5 of the implementation plan (PRD §21)
> **Governing ADRs:** 0005 (resolver-style injection at CLI assembly), 0003 (two-phase graph lifecycle — plugins mutate only pre-seal)
> **Implementation note:** `docs/implementation/active/0003-phase-5-plugin-system.md`

## Problem Statement

`internal/plugin` holds a complete-looking plugin system — `Plugin`/`KernelAPI` interfaces, an event `Bus`, a registry, a topological loader — and none of it is reachable from the product. The engine emits no lifecycle events, the CLI never loads the `plugins:` list from `acthur.yml`, no `KernelAPI` implementation exists (the interface has zero implementors), and nothing proves a plugin's hook/command/generator registrations actually fire. Phase 5's promise (PRD §21) is exactly this wiring.

**Done when:** a test plugin registers a hook, a command, and a generator — and all three fire at the correct lifecycle points under the real kernel (live witness: hook lines visible in `acthur dev` output, command visible in `acthur --help` and runnable).

## Scope decisions

- **Events**: the engine owns a `plugin.Bus` and emits, at minimum: `kernel:graph:before_build`/`after_build` (CLI assembly), `kernel:node:before_start`/`after_start`/`after_healthy`/`before_stop`/`after_stop`/`on_failure` (engine lifecycle). Payload carries node ID/type where applicable. Emission is synchronous and must never panic the kernel (recovered per-handler, error surfaced via output).
- **Loading**: at CLI assembly (mirroring the adapter resolver pattern, ADR 0005), `cfg.Plugins` names are resolved against the built-in plugin registry and `Load`ed in topological order; unknown plugin name = pointed error before startup. Plugins mutate the graph only pre-seal (ADR 0003).
- **KernelAPI**: one real implementation in the kernel: `OnEvent`→bus, `RegisterCommand`→cobra root, `RegisterGenerator`/`RegisterSchema`/`RegisterMiddleware`→kernel registries (stored; consumed by later phases), `AddNodeToGraph`/`AddEdgeToGraph`→graph pre-seal, `Graph()`→read-only view.
- **Test plugin**: a built-in `test` plugin (registered but not in any default list) exercising every registration path; used by unit tests and the live witness.
- **Out of scope**: real capability plugins (auth/migrations/… are Phase 6), community/external plugin loading from disk, generator execution (Phase 7 consumes the registry).

## Slices

### Slice 1 — Engine emits kernel lifecycle events (#40)
Engine constructed with an optional `*plugin.Bus`; emits node lifecycle events at the points above with per-handler panic recovery. Behavior tests: a subscribed handler observes the exact event sequence for a fake node start/stop/failure; a panicking handler doesn't break startup.
**Files:** `internal/engine/*`, `internal/plugin/plugin.go` (only if the Bus needs a recover wrapper).

### Slice 2 — Plugin loading + real KernelAPI at CLI assembly (#41)
`internal/plugin/kernelapi.go` (or engine-adjacent) implementing `KernelAPI` against cobra/graph/bus/registries; CLI loads `cfg.Plugins` after config parse, before graph seal; `acthur plugin list` shows loaded plugins + versions; unknown plugin errors cleanly. Behavior tests: a fixture plugin's command appears on the root command; graph mutation lands pre-seal; load order respects `DependsOn`.
**Files:** `internal/plugin/kernelapi.go(+_test)`, `cmd/acthur/*`.

### Slice 3 — Test plugin + live witness (main agent)
Built-in `test` plugin registering one hook (logs on `kernel:node:after_healthy`), one command (`acthur test-plugin`), one generator target; witness project declares `plugins: [test]`; live `acthur dev` shows the hook line, the command runs.
