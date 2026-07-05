# Where `data_flow` contracts are enforced (proxy interception vs. generated middleware)

**Status:** accepted — Option A (proxy interception in dev); Option B deferred to Phase 7 as the production-path complement

Phase 4's done-criterion is *"a contract violation shows in the proxy log."* Contracts are declared on `data_flow` edges (e.g. `web → api`, `contracts: [users]`). For the proxy to log a violation it must sit in the path of that traffic — but the Phase 3 dev runtime wires service-to-service discovery to go **direct**, bypassing the proxy.

## Why this is load-bearing

`resolveNodeEnv` injects each consumer a direct URL to its `data_flow` / `depends_on` targets — `web` is handed `API_URL=http://localhost:8080` and calls `api` straight. The dev proxy on `:4000` only routes *inbound* traffic selected by `proxied_through` edges (browser → service, by path prefix). It is therefore never on the `web → api` path and has **no interception point** for a `data_flow` contract. The proxy package doc already claims it "enforces contracts on data_flow edges" — that is aspirational; `dispatch()` performs no validation and the traffic never reaches it. So the PRD's "proxy layer enforcement" (§11.3) and the runtime's actual topology contradict each other today. This must be resolved before any Phase 4 enforcement code is written, and it is hard to reverse once generated clients or env-injection conventions ship.

## Considered Options

- **A — Route `data_flow` traffic through the proxy.** Change discovery so a consumer reaches a `data_flow` target via the proxy (`API_URL=http://localhost:4000/api`), making the proxy a true east-west interception point. Matches PRD §11.3 ("traffic blocked at proxy layer") and keeps enforcement in one kernel-owned place. Cost: every internal call takes an extra hop; the proxy must distinguish a `data_flow` route (contract-checked) from a `proxied_through` route (plain forward); breaks the "direct localhost" mental model Phase 3 established.
- **B — Enforce in generated client/server middleware, not the proxy.** The Generator (Phase 7) emits contract-checking middleware into each service; the proxy stays a dumb inbound router. Keeps the hot path direct and per-language-honest. Cost: contradicts the Phase 4 done-criterion as literally written ("shows in **proxy** log"), pushes real enforcement to Phase 7, and spreads logic across N adapters instead of one kernel.
- **C — Hybrid: proxy enforces, but only for edges explicitly opted through it.** Default direct; a `data_flow` edge may declare it routes via the proxy, and only those are contract-checked in dev. Cost: two discovery modes to reason about; partial coverage.

## Decision

**Option A.** In the Local dev context, `data_flow` discovery URLs point at the proxy (`API_URL=http://localhost:4000/api`), making the proxy the single east-west interception point where `ContractFor(from, to)` is consulted. Rationale:

- It is the only option that satisfies Phase 4's done-criterion as written ("violation shows in the **proxy** log") and PRD §11.3 ("traffic blocked at proxy layer" in strict mode).
- Enforcement lives in one kernel-owned place instead of being re-implemented per adapter language; Phase 4 does not have to wait for the Phase 7 generator.
- The extra hop is dev-only. Production traffic never crosses the dev proxy; §11.3's dev/strict modes are dev and pre-deploy concerns.
- The proxy distinguishes route kinds: a `data_flow` route is contract-checked; a `proxied_through` route stays a plain inbound forward. The consumer's caller identity travels on the request (`X-Acthur-From`) so the proxy can pick the right edge contract.

Option B (generated client/server middleware) is **deferred, not rejected** — it is the production-path complement the Phase 7 generator emits, honoring the same contract IR. Option C is rejected: two discovery modes with partial coverage.

## Consequences

- `resolveNodeEnv` changes the URL it injects for `data_flow` targets from the direct node port to the proxy path route. `depends_on`-only edges (infra connections) keep the direct/`Connectable` path — contracts apply to `data_flow` only.
- The proxy gains contract-checked east-west routes derived from `data_flow` edges, consulting a contract registry loaded at graph build.
- Dev mode logs violations and passes traffic; `--strict` returns 422 and blocks (§11.3). The violation line appears in the proxy's own log stream, satisfying the Phase 4 done-criterion.

See [[0011-local-dev-projects-containerspec-zero-privilege]] (Local-only execution context) and [[0010-connectable-capability-for-cross-node-connection-env]] (the existing single discovery-env seam this would extend).
