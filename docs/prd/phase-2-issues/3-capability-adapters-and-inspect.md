> **Issue:** [samueloshio/acthur#19](https://github.com/samueloshio/acthur/issues/19) · **Status:** ready-for-agent · **Type:** AFK

## Parent

[PRD — Phase 2: Adapter System](https://github.com/samueloshio/acthur/issues/17) (#17)

## What to build

Replace the monolithic adapter interface with a capability-based model, and expose it through read-only introspection commands so a user can see what each adapter can do (ADR 0006).

Every adapter implements a minimal **core** (`Name`, `Category`, `Detect`, `EnvVars`). Behavior lives in narrow **capability interfaces** the kernel type-asserts against: `Scaffolder`, `Runnable`, `Containerized` (with its `ContainerSpec` value type), plus `Migratable` and `Deployable` which are *defined but inert* this phase (no implementations). A `Capability` enum mirrors the interfaces 1:1.

Capabilities are **derived**, never declared: a free `CapabilitiesOf(adapter)` function computes the set via type assertions, so an adapter can never advertise a capability it doesn't implement. There is no hand-written `Capabilities()` method.

Reshape the existing `go:fiber` adapter onto core + `Scaffolder` + `Runnable`, preserving its current behavior (the scaffold internals are rewritten in a later slice). The retired monolithic interface is removed.

Add an `adapter` CLI command group, sibling to `graph`:
- `acthur adapter list` — every registered adapter grouped by category.
- `acthur adapter inspect <key>` — name, category, and a ✓/✗ table over the full `Capability` set (including inert `Migrate`/`Deploy`), sourced from `CapabilitiesOf`.

No `install`/`remove`/`search`/`update` — introspection only.

## Acceptance criteria

- [ ] Core interface + `Scaffolder`/`Runnable`/`Containerized` (with `ContainerSpec`) capability interfaces exist; `Migratable`/`Deployable` are defined but unimplemented.
- [ ] `Capability` enum has one constant per capability interface.
- [ ] `CapabilitiesOf` derives the set from interface satisfaction; there is no `Capabilities()` method on adapters.
- [ ] The monolithic adapter interface is removed; `go:fiber` satisfies core + `Scaffolder` + `Runnable` and nothing else.
- [ ] `acthur adapter list` prints registered adapters grouped by category.
- [ ] `acthur adapter inspect go:fiber` shows ✓ Scaffold, ✓ Run, ✗ Container, ✗ Migrate, ✗ Deploy.
- [ ] `acthur adapter inspect` on an unknown key errors clearly.
- [ ] Tests assert `CapabilitiesOf(go:fiber)` is exactly {Scaffold, Run}, via the adapter package's public API.

## Blocked by

None - can start immediately.
