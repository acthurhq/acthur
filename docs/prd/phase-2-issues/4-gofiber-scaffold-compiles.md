> **Issue:** [samueloshio/acthur#21](https://github.com/samueloshio/acthur/issues/21) · **Status:** ready-for-agent · **Type:** AFK

## Parent

[PRD — Phase 2: Adapter System](https://github.com/samueloshio/acthur/issues/17) (#17)

## What to build

Make the `go:fiber` scaffold produce code that compiles on the first try — fixing the current bug where the `go.mod` module path disagrees with the import paths in generated source.

Move `go:fiber` templates from inline Go string literals to files embedded in the adapter package via `go:embed` + `text/template`, executed through a small shared kernel-side template helper. Extend `ScaffoldContext` with kernel-resolved facts — `ModulePath`, `ProjectName`, `IDStrategy` — which templates render (`{{.ModulePath}}`, etc.); adapters never read `acthur.yml` directly.

Module-path policy (ADR 0007): one self-contained Go module per service node, path `<module_prefix>/<node-id>`. Add an optional top-level `module_prefix` to `acthur.yml`; when unset, default to a host-less `<project>` prefix. The kernel resolves the final `ModulePath` onto `ScaffoldContext`.

Delete the hardcoded `detectIDStrategy()` stub; the generated `ids` package is driven by `identifiers.strategy` from `acthur.yml`.

Verified at the `Scaffolder` seam — no scaffolding CLI command ships this phase.

## Acceptance criteria

- [ ] `go:fiber` templates are embedded via `go:embed` and rendered with `text/template`; no inline-string scaffolds remain.
- [ ] A single resolved `ModulePath` feeds both `go.mod` and every generated import.
- [ ] `acthur.yml` accepts an optional `module_prefix`; unset defaults to a host-less `<project>` prefix.
- [ ] `ScaffoldContext` carries `ModulePath`, `ProjectName`, and `IDStrategy`, all resolved by the kernel.
- [ ] `detectIDStrategy()` is removed; the emitted `ids` package matches the configured `identifiers.strategy`.
- [ ] A test renders `go:fiber` into a temp dir and `go build` succeeds.
- [ ] A toolchain-free test asserts the `go.mod` module path equals the import prefix in every generated `.go` file.

## Blocked by

- [Phase 2 · Slice 3 — capability-described adapters + adapter list/inspect](https://github.com/samueloshio/acthur/issues/19) (#19) — the `Scaffolder` interface and the `ScaffoldContext` shape are defined there.
