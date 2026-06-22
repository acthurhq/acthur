> **Issue:** [samueloshio/acthur#20](https://github.com/samueloshio/acthur/issues/20) · **Status:** ready-for-agent · **Type:** AFK

## Parent

[PRD — Phase 2: Adapter System](https://github.com/samueloshio/acthur/issues/17) (#17)

## What to build

Make `acthur graph validate` reject structurally nonsensical node/adapter pairings, catching them at validate time rather than at runtime (Principle 3).

Add the *adapter-category-mismatch* validation rule, using the category the resolver already surfaces (from Slice 1). A `service` node's adapter must be a service-shaped category (backend / frontend / mobile / desktop); an `infra` node's adapter must be an infra-shaped category (database / cache / storage / queue). A mismatch — e.g. `api: { type: service, adapter: db:postgres }` — produces a validation error with severity `error`.

Like the resolution rule, this skips any adapter key in the `kernel:` namespace.

## Acceptance criteria

- [ ] A `service` node using an infra-category adapter yields a validation error with rule `adapter-category-mismatch`.
- [ ] An `infra` node using a service-category adapter yields the same error.
- [ ] Valid pairings (service→backend, infra→database, etc.) pass.
- [ ] `kernel:` adapter keys are exempt from this rule.
- [ ] Tests drive `Validate` with a controlled fake resolver returning chosen categories, asserting rule name and severity.

## Blocked by

- [Phase 2 · Slice 1 — graph validate catches unknown adapters](https://github.com/samueloshio/acthur/issues/18) (#18) — the `Resolver` interface and its category surface are introduced there.
