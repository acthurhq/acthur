# PRD — Phase 4: Contract Engine (enforce-on-the-edge)

> **Status:** ready-for-agent · **Phase:** 4 of the implementation plan (PRD §21)
> **Governing ADRs:** [0012](../adr/0012-where-data-flow-contracts-are-enforced.md) (accepted: Option A — proxy interception in dev), [0010](../adr/0010-connectable-capability-for-cross-node-connection-env.md), [0005](../adr/0005-graph-engine-depends-on-resolver-abstraction.md)

## Problem Statement

`internal/contract` already holds a native `.contract.yml` parser, a versioned registry, a diff engine, and a request validator — but none of it is reachable from the product. Nothing loads contract files from a project, the `acthur contract` CLI commands are stubs, the proxy never consults a contract (its package doc claiming so is aspirational), and `data_flow` traffic bypasses the proxy entirely, so there is no interception point. The graph validation rule "every `data_flow` edge has at least one contract" checks only that a *name* is present — the file behind it may not exist.

**Done when (PRD §21):** a contract violation shows in the proxy log, and `acthur contract diff` correctly identifies breaking changes per §11.5.

## Conventions fixed by this PRD

- **Contract files live at `contracts/<name>.contract.yml`** relative to the project root. An edge's `contracts: [users]` resolves to `contracts/users.contract.yml`.
- **East-west URLs**: in the Local dev context, a consumer's `data_flow` discovery env points at the proxy flow route: `API_URL=http://localhost:4000/_flow/<from>/<to>`. The proxy derives the edge (and therefore the contract) from the path, strips the `/_flow/<from>/<to>` prefix, and forwards the remainder to the target's port. `depends_on`-only edges (infra) keep the direct `Connectable` path — contracts govern `data_flow` only.
- **Modes** (§11.3): dev mode logs violations (prefixed `[proxy]`, severity `contract`) and passes traffic; `acthur dev --strict` returns 422 with a structured body and blocks.

## Slices (independently grabbable, disjoint file sets)

### Slice 1 — Contract loading + `acthur contract` CLI
`internal/contract` gains a project loader (`LoadDir(root)` → Registry) using the `contracts/` convention; graph validation tier 3 extends: a `data_flow` edge whose named contract has no parseable file is an error (named + loadable = valid). CLI: `acthur contract validate|list|show <name>|diff <old> <new>` wired to the registry/loader, formatted with `internal/output`.
**Files:** `internal/contract/loader.go(+_test)`, `cmd/acthur/commands.go`, graph validation touchpoint.
**Behaviors under test:** loading a fixture project registers its contracts; a missing/broken contract file fails `graph validate` with a pointed message; `contract diff` on §11.5 fixture pairs exits non-zero for breaking, zero for non-breaking.

### Slice 2 — East-west flow routes + proxy enforcement (ADR 0012 Option A)
`resolveNodeEnv` injects `_flow` proxy URLs for `data_flow` targets. The proxy builds flow routes from `data_flow` edges: path `/_flow/<from>/<to>/*` → strip prefix → forward to target port; consults the registry (`ContractFor(from, to)`) and validates request/response against the endpoint definition; logs violations in dev mode, 422s in strict mode. `proxied_through` routes stay plain forwards.
**Files:** `internal/proxy/*`, `internal/engine/dev.go` (env injection only), wiring in the engine startup.
**Behaviors under test:** a consumer env gets the `_flow` URL; a request through a flow route reaches the target with the prefix stripped; a request violating the contract produces a violation line in the proxy log stream (dev) and a 422 (strict); a conforming request passes untouched.

### Slice 3 — Diff engine vs §11.5 + honest importers
Table-verify every §11.5 rule (breaking: field removed/renamed, required input added, path/method changed, stricter constraint, auth added; non-breaking: optional fields added, new endpoint, looser constraint) with one test row each; fix gaps. `parseProto`/`parseGraphQL`/OpenAPI importers either work or return an explicit `not yet supported: <format> (Phase 4 imports native contracts only)` error — no silent stubs.
**Files:** `internal/contract/contract.go(+_test)` only.

## Out of Scope
- OpenAPI/proto/GraphQL import implementations (§11.1) — explicit errors this phase.
- Generated client/server contract middleware — Phase 7 (ADR 0012 Option B complement).
- Contract versioning `extends:` inheritance and `Sunset` headers (§11.4) — after enforcement is real.
- Multi-contract edges: enforcement uses the first contract (`ContractFor` semantics today); n-contract merge later.

## Testing Decisions
Behavior-first per the repo's practice: loader and diff assert on values (fixture files → registry contents / DiffResult); proxy enforcement is tested through `httptest` servers behind real proxy routes — no process spawning; the engine keeps thin glue. A final live witness on the vetangle-style fixture proves the proxy-log done-criterion.
