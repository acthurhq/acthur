> **Issue:** [samueloshio/acthur#18](https://github.com/samueloshio/acthur/issues/18) · **Status:** ready-for-agent · **Type:** AFK

## Parent

[PRD — Phase 2: Adapter System](https://github.com/samueloshio/acthur/issues/17) (#17)

## What to build

Make `acthur graph validate` catch adapter keys that don't resolve to a registered adapter.

Introduce a consumer-owned `Resolver` abstraction in the `graph` package (the engine must not import the adapter registry — ADR 0005). The resolver answers "is this adapter key known, and what is its category?" — returning a minimal resolved view exposing name and category, and nothing more. The adapter registry is wrapped to satisfy this interface and injected at the CLI entrypoint as the production resolver.

`graph.Validate` takes the resolver as a required, non-nil argument. There is no zero-arg overload and no `nil`-means-consult-a-global fallback — a caller with no adapter knowledge passes an explicit empty resolver. `Build` stays resolver-free: an unknown adapter is a *validation* fault on a constructable graph, not a build fault.

Add the *unresolved-adapter* validation rule: every node's adapter key must resolve, **except** keys in the `kernel:` namespace, which are kernel primitives that are provided rather than resolved (so the materialized `kernel:proxy` node is never flagged). A failure produces a validation error whose message lists the available adapters.

## Acceptance criteria

- [ ] `graph` defines a `Resolver` interface it owns; `graph` does not import the `adapter` package.
- [ ] The adapter registry is wrapped to satisfy `Resolver` and injected at the CLI entrypoint.
- [ ] `Validate(resolver)` requires a non-nil resolver; there is no zero-arg overload and no global fallback path.
- [ ] An unknown adapter key yields a validation error (severity `error`) whose message lists available adapters.
- [ ] A node whose adapter key is in the `kernel:` namespace (e.g. `kernel:proxy`) is never flagged by this rule.
- [ ] `acthur graph validate` exits non-zero on a config with an unknown adapter and zero on a valid one.
- [ ] Tests drive `Validate` with a controlled fake resolver (map-backed, never `nil`), asserting on the rule name and severity — same style as the existing cycle/orphan tests.

## Blocked by

None - can start immediately.
