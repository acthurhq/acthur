# The graph engine depends on a Resolver abstraction, never on adapter discovery

**Status:** accepted

The graph engine validates that every node's adapter name is known, but it never learns *how* adapters are discovered. It depends on a `Resolver` abstraction — "given a name, is this adapter known, and what is it?" — injected at construction. It does not import the adapter registry, and it never triggers `init()`-based self-registration.

## Why

Discovery mechanisms multiply over the project's life: a global registry today, then plugin loaders, generator registries, template sets, and eventually remote adapter catalogs. If the graph engine reached into any one of them directly, every new mechanism would force a refactor of Ring 0 — the one layer that must stay stable. Depending on an abstraction instead means the engine is written once and the discovery mechanisms evolve behind it.

This also preserves the testability bar set in [[0003-two-phase-graph-lifecycle]] and the explicit-construction style of `NewTestGraph`: the "adapter resolves" rule is tested by passing a controlled `Resolver`, with no global mutation and no `init()`-order coupling.

## Consequences

- `Validate` accepts a `Resolver` (`Build` stays resolver-free — an unknown adapter is a validation fault, not a construction failure). The production wiring (the global, `init()`-populated adapter registry) is assembled at the CLI entrypoint and passed in — it survives as *one* implementation of the abstraction, not as a dependency of the engine.
- **No hidden fallback.** `Validate(resolver)` requires a non-nil resolver; there is no `Validate()` overload and no `nil`-means-consult-the-global path. A caller that legitimately has no adapter knowledge (structural-only tests) passes an explicit *empty* resolver, making the absence a visible choice at the call site — the same discipline `NewTestGraph` brings to topology.
- An unknown adapter name is a **validation error** (collected, severity-carrying — see [[0002-three-tier-graph-validation]]), not a build error: the graph is still constructable, it is merely semantically wrong.
- The same rule extends to future discoverable things (plugins, contracts, generators): the engine takes an abstraction, never the discovery mechanism.
