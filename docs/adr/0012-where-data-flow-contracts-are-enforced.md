# Where `data_flow` contracts are enforced (proxy interception vs. generated middleware)

**Status:** proposed — open question, blocks Phase 4 (Contract Engine) implementation

Phase 4's done-criterion is *"a contract violation shows in the proxy log."* Contracts are declared on `data_flow` edges (e.g. `web → api`, `contracts: [users]`). For the proxy to log a violation it must sit in the path of that traffic — but the Phase 3 dev runtime wires service-to-service discovery to go **direct**, bypassing the proxy. This ADR records the conflict and the options; it does **not** yet pick one.

## Why this is load-bearing

`resolveNodeEnv` injects each consumer a direct URL to its `data_flow` / `depends_on` targets — `web` is handed `API_URL=http://localhost:8080` and calls `api` straight. The dev proxy on `:4000` only routes *inbound* traffic selected by `proxied_through` edges (browser → service, by path prefix). It is therefore never on the `web → api` path and has **no interception point** for a `data_flow` contract. The proxy package doc already claims it "enforces contracts on data_flow edges" — that is aspirational; `dispatch()` performs no validation and the traffic never reaches it. So the PRD's "proxy layer enforcement" (§11.3) and the runtime's actual topology contradict each other today. This must be resolved before any Phase 4 enforcement code is written, and it is hard to reverse once generated clients or env-injection conventions ship.

## Considered Options

- **A — Route `data_flow` traffic through the proxy.** Change discovery so a consumer reaches a `data_flow` target via the proxy (`API_URL=http://localhost:4000/api`), making the proxy a true east-west interception point. Matches PRD §11.3 ("traffic blocked at proxy layer") and keeps enforcement in one kernel-owned place. Cost: every internal call takes an extra hop; the proxy must distinguish a `data_flow` route (contract-checked) from a `proxied_through` route (plain forward); breaks the "direct localhost" mental model Phase 3 established.
- **B — Enforce in generated client/server middleware, not the proxy.** The Generator (Phase 7) emits contract-checking middleware into each service; the proxy stays a dumb inbound router. Keeps the hot path direct and per-language-honest. Cost: contradicts the Phase 4 done-criterion as literally written ("shows in **proxy** log"), pushes real enforcement to Phase 7, and spreads logic across N adapters instead of one kernel.
- **C — Hybrid: proxy enforces, but only for edges explicitly opted through it.** Default direct; a `data_flow` edge may declare it routes via the proxy, and only those are contract-checked in dev. Cost: two discovery modes to reason about; partial coverage.

## Consequences (once decided — recorded here so the choice is deliberate)

- The choice fixes *where* `ContractFor(from, to)` is consulted at runtime and what `data_flow` discovery URLs look like — a public-ish convention consumers bake into their code.
- It determines whether Phase 4 can satisfy its own done-criterion or whether that criterion must be reworded (proxy log → service log).
- Resolve this **on paper before** the #33 dev-runtime witness closes; #33 verifies the Phase 3 loop (proxy carries inbound traffic, services boot, restart, hot reload), and implementation of the chosen option here waits until that witness is green.

See [[0011-local-dev-projects-containerspec-zero-privilege]] (Local-only execution context) and [[0010-connectable-capability-for-cross-node-connection-env]] (the existing single discovery-env seam this would extend).
