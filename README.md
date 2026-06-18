# Acthur

**Runtime Graph Operating System with Pluggable Infrastructure Nodes**

Acthur orchestrates your entire application — backend, frontend, and infrastructure — as a live directed graph. It generates framework-native code, manages your dev environment, enforces API contracts between services, and deploys your system with one command.

---

## What Acthur Is

```
You define the graph.  Acthur runs it.

acthur.yml             →  graph of nodes and relationships
acthur dev             →  starts everything, proxies traffic, hot reloads
acthur add auth        →  generates full auth system into your project  
acthur deploy          →  ships to production from the same graph
```

Acthur is not a framework. It is not a monorepo tool. It is not a scaffolder that generates code and steps back. It is a **running system** — alive during development and deployment — that manages your entire stack through its graph model.

---

## Quick Start

```bash
# Install
curl -fsSL https://install.acthur.dev | sh

# Create a new project
acthur new my-saas

# Start development
cd my-saas
acthur dev
# → http://my-saas.test:4000
```

---

## The Five Concepts

**Graph** — Every service, database, cache, and queue is a node. Every relationship is a typed edge. The kernel makes every decision — startup order, hot reload cascade, deploy topology — by traversing this graph.

**Contracts** — Every edge between services has a typed contract defining what can flow between them. Breaking changes are detected before they reach production. Code is generated from contracts.

**Adapters** — Adapters bridge abstract graph nodes to concrete technologies. `go:fiber`, `rust:axum`, `ui:astro`, `db:postgres` — each is an adapter. Adapters know how to start, build, and scaffold their framework. They know nothing about plugins.

**Plugins** — Plugins install capabilities into the kernel. `auth`, `migrations`, `rbac`, `multitenancy` — each is a plugin. Plugins register hooks, commands, and generators via the kernel API. They generate framework-native code into your project. Your deployed application has zero runtime dependency on Acthur.

**Dev = Deploy** — The same graph that runs `acthur dev` is the source of truth for `acthur deploy`. Docker Compose files, CI pipelines, and Dockerfiles are all derived from the graph. Never manually written.

---

## Project Structure

```
my-saas/
├── acthur.yml            ← the graph definition (the source of truth)
├── contracts/            ← API contracts between services
│   ├── users.contract.yml
│   └── appointments.contract.yml
├── services/
│   ├── api/              ← go:fiber backend (generated)
│   └── worker/           ← background worker (generated)
├── web/                  ← ui:astro frontend (generated)
├── backoffice/           ← ui:next backoffice (generated)
├── db/migrations/        ← database migrations (generated)
└── .acthur/              ← kernel runtime state (gitignored)
```

---

## `acthur.yml` — The Graph

```yaml
project: my-saas
version: "1"

identifiers:
  strategy: ulid

graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080

    web:
      type: service
      adapter: ui:astro
      port: 3000

    db:
      type: infra
      adapter: db:postgres

    cache:
      type: infra
      adapter: cache:redis

  edges:
    - from: api
      to: db
      type: depends_on

    - from: web
      to: api
      type: data_flow
      contracts: [contracts/users.contract.yml]

plugins:
  - name: migrations
  - name: auth
    config:
      strategy: jwt
      algorithm: RS256
```

---

## Supported Runtimes

| Runtime | Frameworks |
|---------|------------|
| **Go** | Fiber, Chi, Gin, Echo |
| **Rust** | Axum, Actix, Rocket |
| **Node.js** | Fastify, NestJS, Express |
| **Bun** | Elysia, Hono |
| **Python** | FastAPI, Django, Flask |
| **PHP** | Laravel |

| Frontend | Adapter |
|----------|---------|
| Astro | `ui:astro` |
| Next.js | `ui:next` |
| Nuxt | `ui:nuxt` |
| SvelteKit | `ui:svelte` |
| Vue (Vite) | `ui:vue` |

---

## The Three-Layer Boundary

```
Plugin  → installs capability into Acthur kernel
           (auth, migrations, rbac, multitenancy, ...)

Adapter → bridges a graph node to a real framework  
           (go:fiber, ui:astro, db:postgres, ...)

Generated code → lives in your project, zero Acthur imports
                  pure Go / Rust / TypeScript
```

These three never cross into each other. An adapter never knows a plugin exists. A plugin never calls an adapter directly. Your generated code never imports Acthur at runtime.

---

## CLI Reference

```bash
# Project
acthur new <name>          interactive wizard → new project
acthur init                adopt existing project (non-destructive)
acthur dev                 start all services + proxy
acthur build               build all services for production
acthur deploy              deploy to configured target

# Graph
acthur graph validate      validate graph structure
acthur graph show          print all nodes and edges

# Contracts
acthur contract validate   validate all contracts
acthur contract diff <n>   show changes + flag breaking

# Database
acthur db migrate          run pending migrations
acthur db seed             run seeders
acthur db reset            drop + migrate + seed

# Code generation
acthur generate from-contract <file>
acthur generate model <Name> --fields "..."
acthur generate ai-context
acthur generate ci --target github-actions

# Plugins
acthur add auth
acthur add rbac
acthur add multitenancy

# Environment
acthur doctor              check environment health
acthur doctor --fix        auto-fix issues

# AI
acthur mcp serve           start MCP server for AI tools
acthur agent explain "<q>" explain part of the system
acthur agent diagnose "<p>" diagnose a runtime problem
```

---

## Plugins

| Plugin | Adds |
|--------|------|
| `migrations` | DB migration management |
| `auth` | JWT, OAuth2, Session, Magic Link, MFA |
| `rbac` | Roles and permissions |
| `multitenancy` | Schema or row-level isolation |
| `feature-flags` | Feature flag system |
| `admin` | Auto-generated admin panel |
| `observability` | OpenTelemetry + Prometheus + Grafana |
| `security` | CORS, CSP, rate limiting, OWASP headers |
| `https` | mkcert for dev, ACME for production |
| `secrets` | Vault, Doppler, Infisical adapters |
| `i18n` | Internationalization + localization |
| `analytics` | PostHog, Plausible, Metabase |
| `ci-cd` | GitHub Actions, GitLab CI |
| `docs` | Living documentation from contracts |
| `ai` | Claude Code / Cursor context + MCP server |

---

## AI Integration

Acthur has something AI coding agents lack: a complete, live, typed model of your entire system. The `ai` plugin exposes this as:

**Context files** — Generated for your specific AI tool. Claude Code gets `CLAUDE.md` + skills. Cursor gets `.cursorrules`. Each file is derived from the actual graph — always accurate.

**Skills** — Project-specific procedural knowledge. `create-endpoint`, `add-migration`, `auth-patterns` — each skill knows your exact stack, adapter, and plugin configuration.

**MCP server** — `acthur mcp serve` exposes the live graph to any MCP-compatible tool. Claude Code can read your graph, trigger generators, run tests, and stream logs directly.

```bash
acthur generate ai-context    # select your tool, generate context + skills
acthur mcp serve              # start MCP server
```

---

## For Existing Projects

```bash
cd my-existing-project
acthur init
```

Acthur scans your project, detects the stack, and writes `acthur.yml`. It never modifies existing files. The non-destructive guarantee: Acthur writes only `acthur.yml` and `.acthur/`.

---

## Deploy

```bash
acthur deploy                    # production
acthur deploy --env staging      # staging
acthur deploy --dry-run          # preview without changes
```

Supported targets: **Coolify**, **Fly.io**, **Railway**, **Render**, **Docker**.

All deployment manifests are derived from the graph. Docker Compose files, Dockerfiles, and CI pipelines are generated — never manually written.

---

## Architecture

For the complete architectural specification, see the [Product Requirements Document](./docs/PRD.md).

Key architectural documents:
- [The Four-Layer Model](./docs/layers.md)
- [Graph Engine Internals](./docs/graph.md)
- [Contract Specification](./docs/contracts.md)
- [Plugin System](./docs/plugins.md)

---

## Contributing

Acthur is MIT-licensed and actively developed.

**Adding a backend adapter:** Implement the `Adapter` interface (8 methods) + scaffold templates for your framework. See `internal/adapter/backend/gofiber/` as reference.

**Adding a plugin:** Implement the `Plugin` interface (3 methods) + `Register(KernelAPI)`. See the plugin system documentation.

```bash
git clone https://github.com/acthur/acthur
cd acthur
make build
make test
./bin/acthur doctor
```

---

## License

MIT — see [LICENSE](./LICENSE)

---

*Acthur — Runtime Graph Operating System*  
*https://acthur.dev · https://github.com/acthur/acthur*
