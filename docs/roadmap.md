# Acthur Roadmap

> Acthur is a runtime graph operating system. This roadmap tracks what is built,
> what is next, and how the five official repositories grow together.

---

## Repository map

```
github.com/samueloshio/acthur           → kernel (this repo)
github.com/samueloshio/acthur-plugins   → official plugins (migrations, auth, rbac, …)
github.com/samueloshio/acthur-adapters  → official adapters (go:chi, rust:axum, ui:astro, …)
github.com/samueloshio/acthur-examples  → example projects (dogfooding + docs-by-example)
github.com/samueloshio/acthur-docs      → documentation site (built with Acthur + Astro)
```

Community plugins and adapters live in independent repositories and are discovered via
the plugin registry (a JSON manifest hosted at `registry.acthur.dev`).

---

## Current state (kernel, as of Phase 0 close-out)

Legend: ✅ done · 🟡 partial · ❌ not built

| Subsystem | Package | Status | Gap |
|---|---|---|---|
| Boot loader / config | `config` | ✅ | — |
| Scheduler / process table | `graph` | ✅ | — |
| Init system (PID 1) | `engine` | ✅ | watcher not wired into dev |
| Process supervisor | `process` | ✅ | — |
| Diagnostics | `doctor` | ✅ | — |
| Syscall ABI (contracts) | `contract` | 🟡 | proto/gql parsers stub; not enforced on wire |
| Device drivers (adapters) | `adapter` | 🟡 | iface + `go:fiber` only |
| Kernel modules (plugins) | `plugin` | 🟡 | bus + KernelAPI ✅; 0 concrete modules |
| Networking / proxy | `proxy` | 🟡 | edge routing ✅; no WS pass-through, no contract enforcement |
| IPC / event bus | `plugin` (bus) | 🟡 | in-proc bus ✅; event mesh ❌ |
| Interrupts / watcher | `watcher` | 🟡 | poll + cascade ✅; not wired into `dev` |
| Watchdog / health | `health` | 🟡 | HTTP/TCP ✅; exec + query checks ❌ |
| Shell / CLI | `cmd`, `output` | 🟡 | output ✅; 4 of 19 commands wired |
| Compiler + AST | `generator` | ❌ | package absent; only fiber scaffold |
| VFS / persistence | — | ❌ | future plugins/adapters |
| Deploy / image export | `engine` deploy | ❌ | no renderers, no targets |
| AI userland (MCP / agent) | — | ❌ | not built |

---

## Phases

### ✅ Foundation (PRD Phases 0–5) — done

The kernel core is real. All packages build, all tests pass with `-race`.

| PRD Phase | Deliverable | Status |
|---|---|---|
| 0 — CLI Skeleton | Binary compiles; full command tree; `acthur --help` | ✅ |
| 1 — Graph Engine | `acthur graph validate` + `show`; topo sort; cycle detection | ✅ |
| 2 — Adapter System | Adapter interface; `go:fiber` driver; `db:postgres` scaffold | ✅ |
| 3 — Dev Runtime | `acthur dev` starts, proxies, supervises, hot-reloads | 🟡 watcher dark |
| 4 — Contract Engine | YAML contracts; diff engine; request validator | 🟡 not wired |
| 5 — Plugin System | Event bus; `KernelAPI`; topo loader; registry | 🟡 no modules |

**Carry-forward work (low-cost, high-value):**
- Wire `internal/watcher` into `engine/dev.go` → live file-change cascade
- Wire `contract.Validator` into the proxy → enforcement on the wire
- Exec/query health strategies in `internal/health`

---

### Phase A — Generator Engine (compiler) `acthur`

**Why first:** the generator is on the critical path for everything visible —
plugins cannot generate code, adapters cannot scaffold full projects, the docs
site cannot be a real Acthur project, and `acthur-examples` cannot be
demonstrated without it.

**Repo:** `acthur` (kernel) + first entry in `acthur-examples`

**Deliverables:**
1. `internal/generator` — template engine; `system.json` AST builder; AST diff;
   generation regions (`// acthur:gen:start` … `// acthur:gen:end`); `generated.lock`
2. `internal/templates` — embedded `go:fiber` template library (handler, service,
   repository, DTO, migration, test)
3. `acthur generate from-contract <contract.yml>` → compiling Go code with tests
4. `acthur generate model <name>` → full domain module (model, repo, service,
   controller, routes, validator, events)
5. `acthur-examples/todo-go-fiber` — first working example project

**Exit gate:** `acthur generate from-contract users.contract.yml` on a clean checkout
produces compiling Go code with passing tests. Example project in `acthur-examples`
builds and runs.

---

### Phase B — First Kernel Modules `acthur-plugins`

**Why next:** with the generator in place, plugins can emit real code. This phase
delivers the four canonical kernel modules that every serious Acthur project needs.

**Repos:** `acthur-plugins` (plugin implementations) + `acthur` (KernelAPI surface,
if gaps are found)

**Deliverables:**
1. `migrations` plugin — `golang-migrate` integration; `acthur db migrate/rollback/status`
2. `auth` plugin — JWT + OAuth2 + Session + Magic Link for `go:fiber`; generates
   middleware, handlers, token store
3. `rbac` plugin — roles/permissions; contract-derived enforcement; policy generator
4. `multitenancy` plugin — schema-per-tenant strategy; `search_path` management;
   tenant provisioner

**Exit gate:** `acthur add auth` on a fresh `go:fiber` project produces compiling,
runnable Go auth code with migrations. All four modules in `acthur-plugins`.

---

### Phase C — Driver Expansion `acthur-adapters`

**Why here:** the adapter abstraction is only validated when more than one driver
exists. Adding a second backend and a frontend driver before the abstraction
ossifies around Fiber's shape.

**Repos:** `acthur-adapters` (adapter implementations) + `acthur-examples`
(one example per new adapter)

**Backend adapters:**

| Adapter | Notes |
|---|---|
| `db:postgres` | Promote existing scaffold to a full driver |
| `cache:redis` | Docker infra → full driver |
| `queue:nats` | Docker infra → full driver |
| `go:chi` | Second backend — validates adapter abstraction |
| `go:gin` | Third backend |
| `rust:axum` | Cross-language validation |
| `node:fastify` | Node.js ecosystem |

**Frontend adapters:**

| Adapter | Notes |
|---|---|
| `ui:astro` | First frontend — needed to bootstrap `acthur-docs` |
| `ui:next` | React ecosystem |
| `ui:vue` | Vue ecosystem |
| `ui:svelte` | Svelte ecosystem |

**Exit gate:** `acthur-adapters` contains at least two backend and one frontend
adapter. Each has a corresponding example in `acthur-examples`. `ui:astro` unblocks
`acthur-docs`.

---

### Phase D — Networking Completion `acthur`

**Why here:** these are all wire-ups of existing, tested components — low cost,
high visible impact.

**Deliverables:**
1. Wire `internal/watcher` into `engine/dev.go` — live cascade on file change
2. WebSocket pass-through in the proxy
3. Contract enforcement at the proxy (calls existing `Validator` on every request)
4. `.test` DNS strategies (mDNS → `/etc/hosts` fallback)
5. Finish remaining 15 CLI command stubs

**Exit gate:** file change triggers hot reload end-to-end; proxy rejects a request
that violates a contract; `acthur --help` shows all commands with short descriptions.

---

### Phase E — Deploy Subsystem `acthur`

**Deliverables:**
1. `Dockerfile` generator per adapter (reuses the generator engine from Phase A)
2. `docker-compose.prod.yml` generator from the live graph
3. Helm chart generator (Kubernetes target)
4. Coolify deploy target (API integration)
5. Dokploy deploy target
6. Pre-deploy gate: `graph.Validate` + `contract.Diff` + clean build + resolved secrets
7. `acthur deploy --env production` fully operational
8. Multi-environment support (`staging`, `production`)

**Exit gate:** `acthur deploy` takes a project from local `acthur dev` to running
production on a VPS in one command.

---

### Phase F — Userland / AI `acthur`

**Deliverables:**
1. MCP server over the live graph — exposes graph state, contracts, and execution
   tools to any MCP-compatible AI tool
2. `acthur agent` CLI command — graph-and-contract-aware AI assistant
3. AI context generation (`acthur generate ai-context`)
4. CI/CD generation plugin (in `acthur-plugins`)

**Exit gate:** any MCP-compatible tool (Claude, Cursor, Copilot) can interrogate the
live Acthur graph and call `acthur` commands via the MCP server.

---

### Docs site `acthur-docs`

**Unblocked by:** Phase A (generator) + Phase B (auth plugin) + Phase C (ui:astro adapter).

The docs site is itself an Acthur project: `go:fiber` backend + `ui:astro` frontend.
It demonstrates the system while documenting it. All documentation is co-located with
the code it documents and is updated in the same PR.

**Entry point:** once `ui:astro` adapter exists in `acthur-adapters`, `acthur-docs`
can be bootstrapped as a real Acthur project. The docs site ships alongside
Phase C completion.

---

## Version milestones

| Version | Gates on | What ships |
|---|---|---|
| **v0.1** | Phase A done | `acthur generate` works; `go:fiber` + `db:postgres` full scaffold; first example project |
| **v0.2** | Phase B done | `acthur add auth/migrations/rbac`; first four plugins published |
| **v0.3** | Phase C done | Multi-adapter ecosystem; docs site live; `ui:astro` + 2 backend adapters |
| **v0.4** | Phase D done | Live hot-reload; contract enforcement on wire; all CLI commands |
| **v0.5** | Phase E done | `acthur deploy` to production; Coolify + Dokploy targets |
| **v0.6** | Phase F done | MCP server; `acthur agent` |
| **v1.0** | All phases + hardening | Stable public API; binary on Homebrew/Scoop; full docs site |

---

## Ongoing hardening (all phases)

These run alongside the phase work and are never "done":

- **Proto / GraphQL contract parsers** — complete the stub importers in `internal/contract`
- **Exec + query health checks** — finish `ExecStrategy.Check` and `QueryStrategy.Check`
- **`internal/errors` package** — extract exit codes from `internal/output`
- **Observability plugin** — OpenTelemetry traces + metrics via `KernelAPI`
- **Doctor CVE scanning** — `acthur doctor` checks adapter/plugin dependency CVEs (Phase 9)
- **Junk cleanup** — remove the two empty `{cmd/...}` brace-expansion dirs

---

## Contribution paths

**New backend adapter** — implement the 8-method `Adapter` interface, add scaffold
templates, add tests, submit PR to `acthur-adapters`.

**New plugin** — implement the 3-method `Plugin` interface + `KernelAPI` calls, add
generator templates for at least one adapter, add tests, submit PR to `acthur-plugins`.

**New deploy target** — implement the `DeployTarget` interface, add a manifest
renderer, test against a real target environment, submit PR to `acthur`.

**New example** — every Acthur feature must be demonstrable in `acthur-examples`.
Features without examples are considered incomplete (dogfooding policy).
