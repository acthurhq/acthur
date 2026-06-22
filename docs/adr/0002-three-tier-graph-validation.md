# Three-tier graph validation: parse, build, validate

**Status:** accepted

Graph correctness is split into three ordered tiers, each with a distinct failure mode, because "can we *construct* the graph?" is a different question from "is the graph *valid*?". Mixing them is the architectural mistake this ADR exists to prevent.

```
Parse  →  Build  →  Validate  →  Execution
```

- **Tier 1 — Parse errors** (`config.Load`): malformed YAML, unknown schema version, missing required scalars, empty edge `from`/`to`, invalid enum values. Returns `error`, stops immediately. Schema-level only — nothing semantic.
- **Tier 2 — Build errors** (`graph.Build`): the graph cannot be *constructed* — an edge references a node that does not exist, a node definition is corrupt, a kernel node cannot be materialized. Returns `error`, stops immediately, because no graph exists to validate.
- **Tier 3 — Validation errors** (`graph.Validate`): the graph exists but is semantically wrong — cycles, orphans, missing contracts on `data_flow`, invalid `applies_to`, unresolved adapters (Phase 2+), contract mismatches. Returns `[]ValidationError` and **continues** so the user sees every problem at once.

## Key consequences

- **Cycles are a validation error, not a build error.** A cyclic `depends_on` graph is fully constructable; the cycle only prevents execution, startup ordering, and deploy planning. So `Build` no longer aborts on a cycle — `Validate` reports it with a `Rule`/`Fix`/`DocsURL`.
- **No duplicated rules.** Each rule lives in exactly one tier. The previous `data_flow`-requires-contract check existed in both `config.Validate` and `graph.Validate`; semantic rules now live only in `Validate`.
- **`ValidationError` carries `Severity` (`error` | `warning` | `info`) from the start.** This lets one report mix hard failures (cycle), warnings (unused infra node), and info (source defaulted to local), and is the shared shape every future consumer needs — `acthur graph validate`, `graph explain`, `doctor`, and the pre-deploy gate.
- Traversal operations (`StartupOrder`, etc.) are only defined on a graph that has passed `Validate`; commands gate on that. See [[0001-relationships-outrank-conventions]] for why the kernel may still apply conventions over a valid-but-silent graph.
