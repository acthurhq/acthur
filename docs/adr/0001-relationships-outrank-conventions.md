# Relationships outrank conventions, but do not abolish them

**Status:** accepted

"Relationships are truth" is the graph's core axiom, but it means relationships have *higher precedence* than conventions — not that the kernel must ignore all defaults. When the graph states a relationship (a `depends_on` edge, an explicit `dev_url`), that relationship is authoritative and a convention may never override it. When the graph is *silent* (two nodes with no dependency relationship between them), the kernel is free to apply a safe convention.

## Why this matters

A purist "ordering comes only from edges" model forces authors to hand-declare a `depends_on` edge from every frontend to every infra node it transitively needs — exactly the boilerplate Acthur exists to eliminate (Principle 2: convention over configuration). The graph saying "nothing depends on anything" is not saying "start `web` before `db`"; it expresses no ordering at all, so the kernel may bring infra up first.

## Consequences

- `StartupOrder` resolves topological rank from `depends_on` edges first; ties (nodes with no dependency relationship) break by infra-before-service, then alphabetical. The convention only ever fills a gap the graph left open.
- The same precedence rule justifies other "kernel fills the silence" decisions: the materialized `proxy` kernel node ([CONTEXT.md] Proxy) and Phase-4 contract-node materialization both inject graph structure the author did not write, without contradicting anything the author did write.
