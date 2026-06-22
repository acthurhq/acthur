# One self-contained Go module per service node

**Status:** accepted

Each service node scaffolds as its own self-contained Go module, rooted at the node's `source` directory, with module path `<module_prefix>/<node-id>`. The kernel resolves the final module path and places it on `ScaffoldContext`; the adapter only renders `{{.ModulePath}}`. There is no project-wide module and no shared local package between services.

## Why

A service node's `source` may be a remote git URL — nodes can be cross-repo (see `CONTEXT.md`, "Cross-repo node"). A node that may live in its own repository must build standalone, which means it must be its own Go module. A single project-wide module would break the instant any node went remote. Per-service modules are the only granularity consistent with the location-agnostic graph.

Keeping module-path *policy* in the kernel (resolved onto `ScaffoldContext`) rather than in the adapter preserves the four-layer boundary: the adapter knows its framework, not the project's naming scheme.

## Consequences

- Cross-service code sharing happens via **generation** (Principle: Generation Over Dependencies), not relative imports or a shared local package. Each service receives its own generated copy of shared types/clients — which is also what contract-derived client generation (Phase 4+) already implies.
- `acthur.yml` gains an optional `module_prefix`; when unset it defaults to a host-less `<project>` prefix (e.g. `vetangle/api`), which still compiles. The placeholder `github.com/yourorg/...` is retired.
- The scaffold's previous compile bug — `go.mod` module path disagreeing with import paths — is structurally impossible once a single resolved `ModulePath` feeds both `go.mod` and every import via `text/template` (see [[0006-capability-based-adapters]] for the `Scaffolder` capability).
