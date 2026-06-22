# Authored vs. materialized node types

**Status:** accepted

The four node types split into two groups by *who creates them*. Authored nodes are written by the user under `graph.nodes`; materialized nodes are injected by the kernel from a simpler authored source. A user may not hand-declare a materialized node type.

- **Authored** — `service`, `infra`. Written directly under `graph.nodes` in `acthur.yml`.
- **Materialized** — `proxy` (a kernel node, injected at build — see [[0001-relationships-outrank-conventions]]), `contract` (injected by the Contract Engine in Phase 4 from the file refs on `data_flow` edges), and `plugin` (injected by the Plugin System in Phase 5 from the top-level `plugins:` list, along with its `applies_to` edges).

## Why

Materialized types have a *simpler authoring surface* than their graph representation: a contract is a file path on an edge, a plugin is an entry in a list. Forcing users to also wire the node and its edges by hand would duplicate that source and invite drift. Letting the kernel own materialization keeps a single source of truth (Principle 7) and matches the precedent that the kernel fills in structure where the author was silent.

## Consequences

- Phase 1 validation **rejects** hand-declared `contract` or `plugin` nodes under `graph.nodes`, pointing the author at the `data_flow` `contracts:` list or the `plugins:` list respectively.
- `NodeTypeContract`, `NodeTypePlugin`, `satisfies`, `consumes`, and `applies_to` remain defined but inert until their materializing engine ships (Phase 4 / Phase 5).
- The taxonomy is the lens for any future node type: decide whether it is authored or materialized before adding it.
