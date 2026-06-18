# Acthur — Product Requirements Document

**Version:** 1.0.0  
**Status:** Draft  
**Classification:** Open Source — Public  
**Last Updated:** May 2026

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Product Definition](#2-product-definition)
3. [Architecture Philosophy](#3-architecture-philosophy)
4. [The Four-Layer Model](#4-the-four-layer-model)
5. [System Architecture](#5-system-architecture)
6. [Installer and Environment](#6-installer-and-environment)
7. [Interactive Wizard](#7-interactive-wizard)
8. [Graph Engine](#8-graph-engine)
9. [Adapter System — Full Runtime Matrix](#9-adapter-system--full-runtime-matrix)
10. [Plugin System](#10-plugin-system)
11. [Contract Engine](#11-contract-engine)
12. [Dev Runtime](#12-dev-runtime)
13. [Deploy Runtime](#13-deploy-runtime)
14. [Generator Engine — Rich Scaffolding](#14-generator-engine)
15. [Plugin Catalogue](#15-plugin-catalogue)
16. [Analytics](#16-analytics)
17. [AI and Agentic Coding Integration](#17-ai-and-agentic-coding-integration)
18. [Security Architecture — Four-Layer Model](#18-security-architecture--four-layer-model)
19. [CLI Command Reference](#19-cli-command-reference)
20. [Non-Functional Requirements](#20-non-functional-requirements)
21. [Implementation Phases](#21-implementation-phases)
22. [Open Source Strategy](#22-open-source-strategy)
23. [Glossary](#23-glossary)
24. [Microservices Architecture](#24-microservices-architecture)
25. [GraphQL — First-Class Transport](#25-graphql--first-class-transport)
26. [gRPC and Protocol Buffers](#26-grpc-and-protocol-buffers)
27. [Traceability — Distributed Tracing](#27-traceability--distributed-tracing)
28. [Observability — Three Pillars](#28-observability--three-pillars)
29. [Multiple Backend Configurations](#29-multiple-backend-configurations)
30. [Testing Infrastructure](#30-testing-infrastructure)
31. [Connection Pool Algorithm](#31-connection-pool-algorithm)
32. [Complete acthur.yml Schema Reference](#32-complete-acthuryml-schema-reference)
33. [CLI Identity — ASCII Art, FIGlet Banner, and UX Standards](#33-cli-identity--ascii-art-figlet-banner-and-ux-standards)
34. [i18n and l10n](#34-i18n-and-l10n)
35. [CI/CD Pipeline Generation](#35-cicd-pipeline-generation)
36. [Documentation Generation](#36-documentation-generation)
37. [Feature Flags](#37-feature-flags)
38. [Starter Templates — Production-Ready SaaS Applications](#38-starter-templates--production-ready-saas-applications)
39. [Dokploy — Self-Hosted PaaS Integration](#39-dokploy--self-hosted-paas-integration)
40. [Roadmap](#40-roadmap)
41. [Constraints and Non-Goals](#41-constraints-and-non-goals)
42. [Compilation Pipeline and system.json AST](#42-compilation-pipeline--systemjson-ast)
43. [Capability System and Dependency Graph](#43-capability-system--dependency-graph)
44. [Internal Capability Contracts](#44-internal-capability-contracts)
45. [Asynchronous Task Processing](#45-asynchronous-task-processing)
46. [Event Mesh and Domain Events](#46-event-mesh--domain-events)
47. [Caching Architecture](#47-caching-architecture)
48. [Real-Time Broadcasting](#48-real-time-broadcasting)
49. [Notification System](#49-notification-system)
50. [Payments System](#50-payments-system)
51. [Validation System](#51-validation-system)
52. [Admin System](#52-admin-system)
53. [SDK and Client Generation](#53-sdk--client-generation)
54. [Competitive Positioning](#54-competitive-positioning)
55. [Success Metrics](#55-success-metrics)
56. [Architectural Decision Records (ADRs)](#56-architectural-decision-records-adrs)
---

## 1. Executive Summary

Acthur is a **runtime graph operating system with pluggable infrastructure nodes**. It orchestrates the entire lifecycle of a software application — from interactive project creation through local development to production deployment — treating backend services, frontend applications, and infrastructure as first-class, interconnected nodes in a live directed graph.

Unlike scaffolding tools that generate code and step back, or monorepo tools that reason about file structure, Acthur is alive for the entire duration of development and deployment. It is the system that starts your services, manages their processes, enforces their contracts, generates their code, and ships them to production — all from one unified, graph-aware runtime.

### What Makes Acthur Different

| Tool | What It Does | What It Misses |
|------|-------------|----------------|
| Docker Compose | Manages container topology | No code generation, no contracts, no DX |
| Nx / Turborepo | Manages monorepo builds | File-centric, not relationship-centric |
| Kubernetes | Manages production topology | Not a dev tool, no code generation |
| Rails / Laravel | Generates and runs a single runtime | Single-language, no polyglot support |
| Encore / Wasp | Contracts + code generation | Language-locked, cloud-only |
| **Acthur** | **All of the above, language-agnostic** | — |

### The Formal Definition

```
Acthur is a runtime graph operating system
with pluggable infrastructure nodes.

Runtime     → alive during development AND deployment, not just scaffolding
Graph       → execution, ordering, health, cascade, and topology are all
              derived from one live relational model
Operating   → manages processes, resources, contracts, and provides a
System        stable interface to all layers above it
Pluggable   → capabilities are installed, versioned, and removable
Infrastructure → infra is a first-class graph node, not a side-concern
Nodes
```

---

## 2. Product Definition

### 2.1 Vision

Every developer and team should be able to go from idea to a fully operational, production-ready application — regardless of their chosen technology stack — in a single unified workflow, without sacrificing architectural correctness, security, or observability.

### 2.2 Mission

Acthur eliminates the accidental complexity of building and operating multi-service applications by making the relationships between services, infrastructure, and interfaces the primary unit of configuration, generation, and orchestration.

### 2.3 Core Value Propositions

**For Individual Developers:**
- Start a new project with correct architecture from day one
- Never manually configure CORS, connection pools, auth middleware, or deployment manifests
- AI coding agents that understand your entire system, not just individual files

**For Teams:**
- Shared, machine-readable architecture definition in `acthur.yml`
- Contracts that prevent breaking changes from reaching production
- Generated CI/CD pipelines that reflect actual system topology

**For Existing Projects:**
- Non-destructive adoption via `acthur init`
- Incremental plugin installation without restructuring existing code
- Living documentation generated from what actually exists

### 2.4 Target Users

**Primary:**
- Full-stack developers building SaaS products (solo or small team)
- Backend developers building microservice systems
- Technical founders who need production-grade architecture without a platform team

**Secondary:**
- Development teams at agencies building client products
- Open source project maintainers wanting a standard project skeleton
- DevOps engineers who want app topology as code

### 2.5 Positioning

Acthur occupies the space that no existing tool covers: **runtime-agnostic, graph-based, full-lifecycle orchestration** that is equally correct for a solo developer's side project and a team's production microservice system.


### 2.3 Core Design Principles

These principles are architectural axioms. Every decision in Acthur — from the graph model to the adapter interface to the CLI output format — is justified by at least one of these.

**Principle 1 — Dependency Inversion (Inside-Out Architecture)**
Business logic depends on abstract contracts. Adapters implement contracts. Nothing in the domain layer imports an external library directly. The kernel wires them together.

**Principle 2 — Convention Over Configuration, Configuration Over Magic**
Sensible defaults cover 90% of projects. Every default is overridable. Nothing happens implicitly that isn't declared in `acthur.yml` or explicit in generated code.

**Principle 3 — Compile-Time Safety Over Runtime Discovery**
Type mismatches, missing contracts, and broken edges are caught before the process starts — not after. In the Rust runtime, tenant context violations are caught at **compile time**.

**Principle 4 — Composition Over Inheritance**
Capabilities are assembled through modular composition. No capability carries hard dependencies on other capabilities unless declared explicitly in the capability DAG.

**Principle 5 — Modular Monolith as the Canonical Architecture**
Generated applications default to a disciplined event-driven modular monolith — not premature microservices. Modules communicate through typed domain events and defined API contracts. Microservice extraction is an optional, deliberate migration path when load patterns justify it.

**Principle 6 — Observability and Security as First-Class Citizens**
Structured logging, distributed tracing, metrics, health endpoints, and security-hardened middleware are baked into every generated resource and route. They are not add-ons.

**Principle 7 — Single Source of Truth (SSOT)**
`acthur.yml` is the authoritative source for every generated artifact: database schemas, API routes, type definitions, RBAC policies, frontend pages, admin views, and client SDKs. A change in the manifest propagates across the entire stack through the generation pipeline.

**Principle 8 — Minimal Runtime Footprint**
Target memory at idle:
```
Go/Fiber + PostgreSQL + Redis:   < 15 MB
Rust/Axum + PostgreSQL + Redis:  < 8 MB
```
Web server, queue workers, cron schedulers, and pub/sub engines coexist in a **single binary process** — eliminating the RAM overhead of running multiple isolated process managers on resource-constrained infrastructure.
---

## 3. Architecture Philosophy

### 3.1 Graph Over Files

Monorepo tools reason about projects by reading folder structure. Acthur reasons about systems by understanding relationships. The co-location of code is irrelevant. What matters is what depends on what, what talks to what, and what must exist before what else can start.

```
Monorepo thinking:     Graph thinking:
  apps/api/     ←         [api] ──depends_on──▶ [db]
  apps/web/       folder       ──data_flow──▶ [web]
  packages/db/    structure    ──emits──────▶ [worker]

Structure is truth.    Relationships are truth.
```

### 3.2 Contract-First Development

The contract is the source of truth — not the code. Code is generated from the contract. The runtime enforces the contract. Breaking changes are detected before they reach production.

```
Developer defines contract
         ↓
Acthur generates server implementation stub
Acthur generates typed client for callers
Acthur generates migration for new types
Acthur validates at runtime that contract is honored
Acthur blocks deploys that break the contract without a version bump
```

### 3.3 Runtime = Dev = Deploy

The same graph that powers `acthur dev` is the source of truth for `acthur deploy`. Docker Compose, Kubernetes manifests, Coolify configuration — all are projections of the graph onto a specific execution model. They are never manually written.

### 3.4 Generation Over Dependencies

Acthur generates framework-native code into your project. Once generated, that code belongs entirely to you. There is no runtime dependency on Acthur. No `import "@acthur/auth"`. No phone-home. The generated Go auth handler is pure Go. The generated Rust middleware is pure Rust. Acthur exits. Your code remains.

### 3.5 Non-Destructive Adoption

`acthur init` on an existing project never modifies a file that existed before it ran. It reads, detects, and proposes. The developer confirms. Acthur writes only `acthur.yml` and the `.acthur/` state directory.

### 3.6 Kernel Analogy

Acthur Core is architected as a kernel. This is not metaphorical:

| OS Kernel Concept | Acthur Equivalent |
|---|---|
| Process Management | Service lifecycle (start, stop, restart, supervise) |
| IPC / Message Bus | Internal proxy + event bus between services |
| Device Drivers | Runtime Adapters |
| System Calls | Contracts — the defined interface a service must satisfy |
| Scheduler | Job runner, cron, queue workers |
| Virtual File System | Storage abstraction |
| Networking Stack | Reverse proxy, service mesh, TLS termination |
| Kernel Modules | Plugins |
| Shell | The `acthur` CLI |
| Init System | `acthur dev` / `acthur start` — PID 1 of your app |

---

## 4. The Four-Layer Model

Everything in Acthur belongs to exactly one of four layers. No layer reaches upward. No layer bleeds into another. This is the foundational rule the entire system is built on.

```
┌─────────────────────────────────────────────────────────────┐
│  LAYER 4 · PLUGINS                  capability layer        │
│                                                             │
│  What:  capability installation into the kernel             │
│  Who:   Acthur core team + trusted contributors             │
│  Owns:  hooks, commands, generators, schemas, middleware     │
│  Calls: kernel registration API only                        │
│  Does NOT know: how adapters run, what users' apps look like │
├─────────────────────────────────────────────────────────────┤
│  LAYER 3 · CONTRACTS                safety layer            │
│                                                             │
│  What:  typed rules on every graph edge                     │
│  Who:   application architects (you)                        │
│  Owns:  the interface between any two nodes                 │
│  Is:    passive law — enforced, not executing               │
│  Does NOT know: which framework implements it               │
├─────────────────────────────────────────────────────────────┤
│  LAYER 2 · ADAPTERS                 infrastructure layer    │
│                                                             │
│  What:  bridges abstract graph nodes to concrete tools      │
│  Who:   framework maintainers + community                   │
│  Owns:  how to run, build, scaffold one specific framework  │
│  Calls: the framework's own toolchain                       │
│  Does NOT know: what plugins are installed                  │
├─────────────────────────────────────────────────────────────┤
│  LAYER 1 · GRAPH                    execution model         │
│                                                             │
│  What:  the live relational model of the entire system      │
│  Who:   the kernel itself                                   │
│  Owns:  all nodes, all edges, all runtime state             │
│  Calls: adapters to execute, plugins to extend              │
│  Is:    the single source of truth                          │
└─────────────────────────────────────────────────────────────┘
```

### 4.1 The Communication Law

```
Graph      ──calls──▶  Adapters      (kernel tells adapter: run this node)
Graph      ──calls──▶  Plugins       (kernel fires hooks on lifecycle events)
Plugins    ──writes──▶ Graph         (plugins mutate graph: add node, inject edge)
Contracts  ──enforce─▶ Graph edges   (contracts validate what flows on an edge)

Plugins    ✗  never call  → Adapters directly
Adapters   ✗  never know  → Plugins exist
Contracts  ✗  never execute → they are passive law
```

### 4.2 Layer Decision Rule

```
Ask of anything being designed:

  Does it define execution relationships?   → Graph
  Does it enforce interface safety?         → Contract
  Does it bridge to a real technology?      → Adapter
  Does it install a capability?             → Plugin

  If none of the above → it does not belong in Acthur core.
```

---

## 5. System Architecture

### 5.1 Repository Structure

```
github.com/samueloshio/
├── kernel/               # The Go binary — all core internals
│   ├── cmd/acthur/       # CLI entrypoint (cobra)
│   ├── internal/
│   │   ├── graph/        # Node, Edge, DAG, State Manager
│   │   ├── contract/     # Parser, Registry, Validator, Diff
│   │   ├── adapter/      # Interface + Registry
│   │   ├── plugin/       # Interface + Loader + Registry
│   │   ├── engine/       # Dev Orchestrator + Deploy Engine
│   │   ├── process/      # Process Manager + Supervisor
│   │   ├── proxy/        # Dev Reverse Proxy
│   │   ├── watcher/      # File watcher + hot reload cascade
│   │   ├── generator/    # Template engine + code generation
│   │   ├── health/       # Health check strategies per node type
│   │   ├── config/       # acthur.yml loader + validator
│   │   ├── output/       # CLI output formatter
│   │   ├── errors/       # Typed error system
│   │   └── doctor/       # Environment checker + auto-fixer
│   └── templates/        # Embedded scaffolding templates (go:embed)
│
├── plugins/              # Official plugins (separate packages)
│   ├── auth/
│   ├── migrations/
│   ├── rbac/
│   ├── multitenancy/
│   ├── feature-flags/
│   ├── admin/
│   ├── https/
│   ├── secrets/
│   ├── observability/
│   ├── testing/
│   ├── ci-cd/
│   ├── docs/
│   ├── i18n/
│   ├── security/
│   ├── product-analytics/
│   ├── web-analytics/
│   ├── business-analytics/
│   └── ai/
│
├── adapters/             # Official adapters (separate packages)
│   ├── backend/
│   │   ├── go-fiber/
│   │   ├── go-chi/
│   │   ├── go-gin/
│   │   ├── go-echo/
│   │   ├── rust-axum/
│   │   ├── rust-actix/
│   │   ├── rust-rocket/
│   │   ├── node-fastify/
│   │   ├── node-nest/
│   │   ├── node-express/
│   │   ├── bun-elysia/
│   │   ├── bun-hono/
│   │   ├── python-fastapi/
│   │   ├── python-django/
│   │   └── php-laravel/
│   ├── frontend/
│   │   ├── astro/
│   │   ├── next/
│   │   ├── vue/
│   │   ├── svelte/
│   │   ├── nuxt/
│   │   └── remix/
│   ├── mobile/
│   │   └── flutter/
│   ├── desktop/
│   │   ├── tauri/
│   │   └── electron/
│   ├── database/
│   │   ├── postgres/
│   │   ├── mysql/
│   │   └── sqlite/
│   ├── cache/
│   │   └── redis/
│   ├── storage/
│   │   ├── minio/
│   │   └── s3/
│   ├── queue/
│   │   └── nats/
│   ├── secrets/
│   │   ├── vault/
│   │   ├── doppler/
│   │   └── infisical/
│   ├── analytics/
│   │   ├── posthog/
│   │   ├── plausible/
│   │   └── metabase/
│   └── deploy/
│       ├── coolify/
│       ├── fly/
│       ├── railway/
│       ├── render/
│       └── docker/
│
└── examples/
    ├── go-fiber-astro-saas/
    ├── rust-axum-next-app/
    ├── node-fastify-vue-api/
    └── go-fiber-flutter-mobile/
```

### 5.2 The `acthur.yml` Graph DSL

`acthur.yml` is not a configuration file. It is a graph definition language. Every key represents a node or an edge. The kernel builds its live in-memory DAG from this file.

```yaml
# acthur.yml — complete annotated example

project: vetangle
version: "1"

# Global identifier strategy — applied everywhere Acthur generates IDs
identifiers:
  strategy: ulid              # ulid | uuid-v4 | cuid2 | nanoid | sequential
  public_strategy: nanoid     # optional: different strategy for public-facing IDs

# Dev environment configuration
dev:
  domain: vetangle.test       # auto-derived from project name
  https: false                # true if https plugin active
  port: 4000                  # unified proxy port
  dns_strategy: auto          # auto | hosts-file | local-dns | pac-proxy

# Execution environments
environments:
  dev:
    context: local            # local | docker
  staging:
    context: docker
    target: coolify
    host: staging.vetangle.com
  production:
    context: cloud
    target: coolify
    host: vetangle.com

# The graph
graph:
  nodes:
    # ── Service Nodes ──────────────────────────────────────────
    api:
      type: service
      adapter: go:fiber
      port: 8080
      hot_reload: true
      dev_url: api.vetangle.test

    web:
      type: service
      adapter: ui:astro
      port: 3000
      hot_reload: true
      dev_url: vetangle.test

    backoffice:
      type: service
      adapter: ui:next
      port: 3001
      hot_reload: true
      dev_url: backoffice.vetangle.test

    worker:
      type: service
      adapter: go:fiber
      role: queue-worker

    # ── Infra Nodes ────────────────────────────────────────────
    db:
      type: infra
      adapter: db:postgres
      version: 16
      pool:
        max_conns: auto
        min_conns: auto
        max_conn_lifetime: 1h
        max_conn_lifetime_jitter: 5m
        max_conn_idle_time: 30m
        health_check_period: 30s
        connect_timeout: 5s
        before_acquire: ping
        after_release: reset_search_path  # injected by multitenancy plugin

    cache:
      type: infra
      adapter: cache:redis

    storage:
      type: infra
      adapter: storage:minio

    queue:
      type: infra
      adapter: queue:nats

  edges:
    # Dependency edges — startup ordering
    - api       → db         : depends_on
    - api       → cache      : depends_on
    - api       → storage    : depends_on
    - worker    → db         : depends_on
    - worker    → queue      : depends_on

    # Data flow edges — contract-bound service communication
    - web       → api        : data_flow
                               contracts: [users, appointments]
    - backoffice → api       : data_flow
                               contracts: [admin, vets]

    # Event edges
    - api       → worker     : emits
                               events: [appointment.booked, payment.received]
    - worker    → queue      : subscribes_to
                               events: [appointment.booked, payment.received]

    # Proxy edges — traffic routing
    - web        → proxy     : proxied_through
    - api        → proxy     : proxied_through
    - backoffice → proxy     : proxied_through

    # Ownership edges
    - api  → db      : migrates
    - api  → storage : migrates

# Plugins — installed capabilities
plugins:
  - name: migrations
  - name: auth
    config:
      strategy: jwt
      algorithm: RS256
      access_token_ttl: 15m
      refresh_token_ttl: 7d
      refresh_rotation: true
      revocation: redis
      oauth2:
        providers: [google, github]
      mfa:
        factors: [totp]
  - name: rbac
    config:
      roles: [admin, vet, agent, support, user]
  - name: multitenancy
    config:
      strategy: schema
      identification: subdomain
  - name: observability
    config:
      signals: [traces, metrics, logs]
      trace_backend: jaeger
      metrics_backend: prometheus
  - name: security
  - name: i18n
    config:
      default_locale: en-US
      locales: [en-US, fr-FR, es-ES]
  - name: ai
    config:
      provider: claude
      model: claude-sonnet-4-20250514

# AI configuration
ai:
  provider: claude
  api_key: ${ANTHROPIC_API_KEY}
  context_strategy: graph-first
```

---

## 6. Installer and Environment

### 6.1 Installation Methods

```bash
# macOS / Linux
curl -fsSL https://install.acthur.dev | sh

# Windows (PowerShell)
irm https://install.acthur.dev/win | iex

# Homebrew (macOS)
brew install acthur

# Scoop (Windows)
scoop install acthur

# Direct binary download
# Available at: github.com/acthur/acthur/releases
```

The installer:
1. Detects OS and architecture (Linux x86_64/arm64, macOS x86_64/arm64, Windows x86_64)
2. Downloads the correct pre-built binary from GitHub Releases
3. Places it in `~/.acthur/bin` (Unix) or `%APPDATA%\acthur\bin` (Windows)
4. Adds the binary directory to `PATH`
5. Runs `acthur doctor` automatically on first invocation

The installer has zero dependencies — it requires only `curl`/`wget` on Unix or `irm` on Windows.

### 6.2 `acthur doctor` — Environment Health System

`acthur doctor` is the living environment health check. It runs automatically before `acthur new` and `acthur dev`. It is also runnable at any time.

```bash
acthur doctor          # check environment
acthur doctor --fix    # auto-fix all resolvable issues
```

**What Doctor Checks:**

```
Core Requirements (always checked):
  git                  any version
  docker               ≥ 24.0, daemon running
  docker compose       ≥ 2.0

Per-Adapter (checked only if adapter in acthur.yml):
  go                   ≥ 1.21  (go:* adapters)
  air                  any     (go:* adapters, hot reload)
  golang-migrate       any     (migrations plugin + go:* adapter)
  rustup / cargo       any     (rust:* adapters)
  cargo-watch          any     (rust:* adapters, hot reload)
  node                 ≥ 20    (node:*, ui:* adapters)
  pnpm                 ≥ 8     (frontend adapters)
  bun                  ≥ 1.0   (bun:* adapters)
  python               ≥ 3.11  (python:* adapters)

System:
  port availability    all ports declared in graph nodes
  disk space           ≥ 2GB recommended
  DNS resolver         for *.test domain support
```

**Auto-Fix Capability:**

```
Can auto-install via package managers:
  golang-migrate     → go install
  air                → go install
  cargo-watch        → cargo install
  sqlx-cli           → cargo install
  pnpm               → npm install -g
  uv                 → curl -LsSf https://astral.sh/uv/install.sh | sh (macOS/Linux)
                       irm https://astral.sh/uv/install.ps1 | iex (Windows)
  docker-compose     → brew / apt / winget

Guides but cannot auto-install:
  docker desktop     → provides OS-specific download link
  go toolchain       → golang.org/dl
  rust / rustup      → rustup.rs
  node               → nodejs.org
  python             → python.org
```

**Dependency Matrix:**

Doctor checks only what your specific `acthur.yml` requires. A project using `go:fiber` never sees Rust or Bun in doctor output.

```
Adapter               Required Tools
──────────────────────────────────────────────────────────────────────
go:fiber              go ≥1.21, air, golang-migrate
go:chi                go ≥1.21, air, golang-migrate
go:gin                go ≥1.21, air, golang-migrate
go:echo               go ≥1.21, air, golang-migrate
rust:axum             rustup, cargo, cargo-watch, sqlx-cli
rust:actix            rustup, cargo, cargo-watch, sqlx-cli
rust:rocket           rustup, cargo, cargo-watch, sqlx-cli
node:fastify          node ≥20, pnpm
node:nest             node ≥20, pnpm
node:express          node ≥20, pnpm
bun:elysia            bun ≥1.0
bun:hono              bun ≥1.0
python:fastapi        uv ≥0.4.0  (uv manages Python version internally)
python:django         uv ≥0.4.0
python:flask          uv ≥0.4.0
php:laravel           php ≥8.2, composer

ui:astro              node ≥20, pnpm
ui:next               node ≥20, pnpm
ui:vue                node ≥20, pnpm
ui:svelte             node ≥20, pnpm
ui:nuxt               node ≥20, pnpm

mobile:flutter        flutter ≥3.x, dart
mobile:react-native   node ≥20, pnpm, expo-cli
desktop:tauri         rustup, tauri-cli, node ≥20, pnpm
desktop:electron      node ≥20, pnpm

db:postgres           docker (or external DSN)
cache:redis           docker (or external DSN)
queue:nats            docker (or external DSN)
```

### 6.3 Execution Contexts

Acthur supports three execution contexts. The graph definition does not change between them — only the execution strategy for each node changes.

**Context 1: Local (default)**
- Service nodes run as native OS child processes
- Infra nodes (db, redis, minio) run as Docker containers
- Best for: individual developers, fast feedback loop
- Requires: Docker for infra, runtime toolchain for services

**Context 2: Docker**
- All nodes run as Docker containers
- Service nodes built on change, source volume-mounted for hot reload
- Best for: production parity, CI/CD, teams without uniform toolchains
- Requires: Docker only
- Activated via: `acthur dev --docker`

**Context 3: Cloud**
- Graph projected onto target platform's native format
- Best for: staging and production environments
- Activated via: `acthur deploy --target <provider>`

---

## 7. Interactive Wizard

### 7.1 `acthur new` — New Project Wizard

```bash
acthur new <project-name>
```

The wizard collects all decisions needed to build the initial graph, then generates the entire project in one pass. Every selection is remembered in `acthur.yml` and drives all future generation.

**Step 1: Backend Runtime**
```
Which backend runtime?
❯ Go
  Rust
  Node.js
  Bun
  Python
  PHP
```

**Step 2: Backend Framework**
```
[ Go selected ]
Which framework / router?
❯ Fiber   — Express-inspired, high performance, large ecosystem
  Chi     — Lightweight, 100% stdlib-compatible routing
  Gin     — Battle-tested, widely used, rich middleware
  Echo    — Minimalist, extensible, low overhead

[ Rust selected ]
❯ Axum    — Tower-based, ergonomic, excellent async
  Actix   — Extremely high performance, actor model
  Rocket  — Convention over configuration, easy start

[ Node.js selected ]
❯ Fastify — Fast, schema-based, great plugin system
  NestJS  — Opinionated, Angular-inspired, enterprise-grade
  Express — Ubiquitous, maximum flexibility

[ Bun selected ]
❯ Elysia  — Bun-native, TypeScript-first, fast
  Hono    — Lightweight, multi-runtime compatible

[ Python selected ]
❯ FastAPI — Modern, async, automatic OpenAPI docs
  Django  — Batteries-included, ORM, admin built-in
  Flask   — Minimal, flexible, large ecosystem
```

**Step 3: Frontend**
```
Frontend?  (multi-select, or none for API-only)
❯ [ ] Astro    — Content + islands architecture, multi-framework
  [ ] Next.js  — Full-stack React, SSR + SSG
  [ ] Nuxt     — Full-stack Vue, SSR + SSG
  [ ] SvelteKit — Svelte, minimal overhead
  [ ] Remix    — Full-stack React, web standards focus
  [ ] Vue      — Vite + Vue 3 SPA
  [ ] None     — API only

Mobile?
  [ ] Flutter       — Dart, iOS + Android, single codebase
  [ ] React Native  — TypeScript, iOS + Android, Expo managed

Desktop?
  [ ] Tauri         — Rust backend, web frontend, lightweight
  [ ] Electron      — Node.js, web frontend, large ecosystem
```

> Note: All generated frontends and mobile apps are fully functional
> working applications — not empty scaffolds. Selected plugins
> (auth, rbac, multitenancy) determine what is pre-built.
> A project with the auth plugin will have complete login, register,
> and dashboard screens ready to run on `acthur dev`.

**Step 4: Database**
```
Database?
❯ PostgreSQL 16  — recommended
  MySQL 8
  SQLite

Cache?
❯ Redis 7   None

Object Storage?
❯ MinIO (self-hosted)   S3   R2   None

Message Queue?
❯ NATS   Redis Streams   None
```

**Step 5: Identifier Strategy**
```
How should entity IDs be generated?
❯ ULID       — sortable, URL-safe, no collision (recommended)
  UUID v4    — universal standard, maximum interoperability
  CUID2      — secure, monotonic, collision-resistant
  NanoID     — compact, URL-safe, configurable length
  Sequential — auto-increment (simple projects only)

Use a different strategy for public-facing IDs?
❯ Yes — internal: ULID · public: NanoID
  No  — same strategy everywhere
```

**Step 6: Plugins**
```
Which plugins to install?
❯ [x] migrations     — always recommended
  [x] auth           — authentication system
  [ ] rbac           — roles and permissions
  [ ] multitenancy   — multi-tenant architecture
  [ ] feature-flags  — feature flag system
  [ ] admin          — auto-generated admin panel
  [ ] observability  — OpenTelemetry + metrics + logs
  [ ] security       — CORS, CSP, rate limiting, OWASP headers
  [ ] i18n           — internationalization + localization
  [ ] analytics      — product + web analytics
  [ ] https          — TLS for local dev + production
  [ ] ci-cd          — CI/CD pipeline generation
  [ ] docs           — living documentation generation
  [ ] ai             — AI coding agent context + MCP server
```

**Step 7: Auth Configuration**
*(shown only if auth plugin selected)*
```
Primary auth strategy?
❯ JWT          — stateless, token-based, best for APIs
  Session      — server-side, instant revocation, best for web
  Both         — JWT for API, Session for web

JWT Algorithm?  (JWT selected)
❯ RS256  — asymmetric, recommended for production
  HS256  — symmetric, simpler for single-service

Identity sources?  (multi-select)
❯ [x] Email + Password
  [ ] Google OAuth2
  [ ] GitHub OAuth2
  [ ] Microsoft OAuth2 / OIDC
  [ ] Magic Link (passwordless)
  [ ] Custom OIDC provider

Second factor (MFA)?
❯ None
  TOTP (Authenticator app — Google Authenticator / Authy)
  SMS OTP
  Email OTP
  Passkey / WebAuthn
```

**Step 8: Multi-Tenancy**
*(shown only if multitenancy plugin selected)*
```
Tenancy isolation model?
❯ Schema isolation
    Each tenant gets a dedicated PostgreSQL schema.
    Strong data isolation. Required for some compliance standards.
    → best for: B2B SaaS, HIPAA, SOC2

  Row-level isolation
    All tenants share tables with tenant_id column.
    Simpler to operate. Fast queries with correct indexes.
    → best for: high-volume SaaS, consumer products

  Database isolation
    Each tenant gets a dedicated database.
    Maximum isolation. Complex connection management.
    → best for: large enterprise, strict data residency

Tenant identification?
❯ Subdomain   (tenant.yourapp.com)
  Path prefix (yourapp.com/t/tenant)
  Header      (X-Tenant-ID)
  Custom domain
```

**Step 9: Deploy Target**
```
Where will this deploy?
❯ Coolify  — self-hosted PaaS, recommended
  Fly.io   — global edge deployment
  Railway  — simple cloud deployment
  Render   — managed cloud
  Docker   — self-managed compose
  Skip for now
```

**Step 10: Review and Generate**
```
  Project:         vetangle
  Backend:         Go · Fiber
  Frontend:        Astro (web) · Next.js (backoffice)
  Mobile:          Flutter
  Database:        PostgreSQL 16 · Redis
  IDs:             ULID (internal) · NanoID (public)
  Plugins:         migrations, auth (JWT + Google, TOTP MFA)
                   rbac, multitenancy (schema)
  Deploy:          Coolify

  Running acthur doctor...
  ✓ go 1.22.3 · air v1.52.0 · golang-migrate 4.17.1
  ✓ node 22.2.0 · pnpm 9.1.0
  ✓ flutter 3.24.0 · dart 3.5.0
  ✓ docker 26.1.0 (daemon running)
  ✓ All requirements satisfied

  Scaffolding...
  ✓ acthur.yml written
  ✓ go:fiber scaffolded
  ✓ ui:astro scaffolded with auth + dashboard (login, register, dashboard, admin)
  ✓ ui:next scaffolded with auth + dashboard (backoffice)
  ✓ mobile:flutter scaffolded with auth + dashboard (login, home, profile)
  ✓ db:postgres configured
  ✓ migrations plugin installed → migrations generated
  ✓ auth plugin installed → JWT + Google OAuth2 + TOTP MFA generated
  ✓ rbac plugin installed → roles [admin, vet, agent, support, user] generated
  ✓ multitenancy (schema) installed → tenant middleware + provisioner generated
  ✓ typed API clients generated from contracts
  ✓ docker-compose.dev.yml generated
  ✓ .env.example written

  Ready. All services start with complete working UI.
  cd vetangle && acthur dev
  → http://vetangle.test:4000         (web — login page)
  → http://backoffice.vetangle.test   (backoffice — login page)
  → flutter run (in mobile/)          (mobile app — login screen)
```

### 7.2 `acthur init` — Initialize on Existing Project

`acthur init` adopts an existing project into Acthur without modifying any existing file.

```bash
cd my-existing-project
acthur init
```

**The Non-Destructive Guarantee:**
`acthur init` will never modify, overwrite, or delete any file that existed before it was run. The only files it creates are `acthur.yml` and the `.acthur/` directory.

**Init Phases:**

1. **Scan** — detect runtime from lock files / manifests, detect framework from dependencies, detect database from env vars, detect existing migration tool, detect existing auth patterns

2. **Confirm** — display the detected graph to the user, allow corrections, warn about ambiguities

3. **Write** — write `acthur.yml` only; write `.acthur/` state directory

4. **Reconcile** — register existing migrations, map existing env vars to graph node config, register existing Dockerfile if present

5. **Verify** — run `acthur graph validate`, `acthur doctor`, dry-run `acthur dev`

---

## 8. Graph Engine

### 8.1 Node Types

```
NodeType
├── Service   — something that runs: api, web, worker, cron
├── Infra     — something that exists: postgres, redis, minio
├── Plugin    — cross-cutting system: auth, tenant, rbac, flags
└── Contract  — interface definition: users.contract, payments.contract
```

### 8.2 Edge Types

```
EdgeType
├── depends_on       — startup ordering; B cannot start until A is healthy
├── data_flow        — A calls B; a contract must exist on this edge
├── proxied_through  — traffic from A routes via B
├── satisfies        — this service implements this contract
├── consumes         — this service calls this contract endpoint
├── migrates         — this service owns this infra node's schema
├── emits            — this service produces these events
└── subscribes_to    — this service consumes these events
```

### 8.3 Graph Operations

All runtime decisions are graph traversal operations:

```
Startup ordering      → topological sort on depends_on edges
Shutdown ordering     → reverse of startup order
Hot reload cascade    → propagate change along reverse data_flow edges
Contract enforcement  → find contract on data_flow edge, validate traffic
Deploy topology       → project graph onto target execution model
Health dependency     → find all nodes affected by a failed node
Plugin application    → find all nodes targeted by applies_to edges
```

### 8.4 Graph Validation Rules

```
No unknown node references in edges
No self-referencing edges
No cycles in depends_on edges
All adapter names resolve to registered adapters
No orphan nodes (every node has at least one edge)
Every data_flow edge has at least one contract
Plugin applies_to targets must be service nodes
```

### 8.5 Cross-Repo Nodes

Nodes do not need to be co-located. The graph is location-agnostic:

```yaml
nodes:
  api:
    type: service
    adapter: go:fiber
    source: ./                        # local

  web:
    type: service
    adapter: ui:astro
    source: github.com/org/web        # remote repo

  payments:
    type: service
    adapter: rust:axum
    source: git@gitlab.com:org/pay    # private repo
```

---

## 9. Adapter System — Full Runtime Matrix

### 9.1 Core Principle

An adapter is an implementation bridge. It knows exactly one thing: how to operate its specific framework. It has zero knowledge of plugins, contracts, or other adapters. The kernel calls adapters — adapters never call back.

### 9.2 Adapter Interface

```go
type Adapter interface {
    Name()      string      // "go:fiber" — runtime:framework compound key
    Category()  Category    // Backend | Frontend | Database | Cache | Storage | Queue

    Detect(dir string) bool
    Scaffold(ctx ScaffoldContext) ([]File, error)
    DevCommand(env Env)    Command
    BuildCommand(env Env)  Command
    TestCommand(env Env)   Command
    GeneratorTargets() []string
    Dockerfile(cfg Config) string
    EnvVars() []EnvVar
}
```

### 9.3 Multiple Backends Per Project — Core Design

Acthur fully supports multiple backend runtimes within one project. Every service node independently selects its adapter. The graph connects them through typed contracts regardless of language.

```yaml
# acthur.yml — three backends, one system
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber         # core business API
      port: 8080

    payments:
      type: service
      adapter: rust:axum        # performance-critical payment service
      port: 8081

    ml-inference:
      type: service
      adapter: python:fastapi   # ML model serving
      port: 8082

    notifications:
      type: service
      adapter: node:fastify     # notification service
      port: 8083
```

`acthur dev` starts all four services using the right toolchain for each. `acthur doctor` checks only what each specific adapter needs. Contracts enforce type safety across language boundaries.

### 9.4 Adapter Naming Convention

```
runtime:framework

Go Backends:      go:fiber    go:chi      go:gin      go:echo
Rust Backends:    rust:axum   rust:actix  rust:rocket
Node Backends:    node:fastify node:nest  node:express node:hapi
Bun Backends:     bun:elysia  bun:hono
Python Backends:  python:fastapi python:django python:flask
PHP Backends:     php:laravel

Frontends:        ui:astro    ui:next     ui:vue      ui:svelte
                  ui:nuxt     ui:remix    ui:solid

Mobile:           mobile:flutter  mobile:react-native
Desktop:          desktop:tauri   desktop:electron

Databases:        db:postgres  db:mysql   db:sqlite
Cache:            cache:redis
Storage:          storage:minio  storage:s3  storage:r2
Queue:            queue:nats
Secrets:          secrets:vault  secrets:doppler  secrets:infisical
Analytics:        analytics:posthog  analytics:plausible  analytics:metabase
Observability:    obs:jaeger  obs:prometheus  obs:grafana  obs:loki  obs:tempo
Deploy:           deploy:coolify  deploy:fly  deploy:railway  deploy:render
```

### 9.5 Full Runtime Support Matrix

#### Backend Adapters

| Runtime | Framework | Adapter Key | Hot Reload | Package Manager | Migration Tool |
|---------|-----------|-------------|------------|-----------------|----------------|
| Go | Fiber | `go:fiber` | air | go modules | golang-migrate |
| Go | Chi | `go:chi` | air | go modules | golang-migrate |
| Go | Gin | `go:gin` | air | go modules | golang-migrate |
| Go | Echo | `go:echo` | air | go modules | golang-migrate |
| Rust | Axum | `rust:axum` | cargo-watch | cargo | sqlx-migrate |
| Rust | Actix | `rust:actix` | cargo-watch | cargo | sqlx-migrate |
| Rust | Rocket | `rust:rocket` | cargo-watch | cargo | sqlx-migrate |
| Node.js | Fastify | `node:fastify` | tsx watch | pnpm | knex / prisma |
| Node.js | NestJS | `node:nest` | nest --watch | pnpm | TypeORM / Prisma |
| Node.js | Express | `node:express` | tsx watch | pnpm | knex |
| Bun | Elysia | `bun:elysia` | bun --hot | bun | drizzle |
| Bun | Hono | `bun:hono` | bun --hot | bun | drizzle |
| **Python** | **FastAPI** | **`python:fastapi`** | **uvicorn --reload** | **uv** | **alembic** |
| **Python** | **Django** | **`python:django`** | **runserver** | **uv** | **django migrate** |
| **Python** | **Flask** | **`python:flask`** | **flask --debug** | **uv** | **flask-migrate** |
| PHP | Laravel | `php:laravel` | artisan serve | composer | artisan migrate |

#### Frontend Adapters

| Framework | Adapter Key | Package Manager | Dev Command | SSR | SSG |
|-----------|-------------|-----------------|-------------|-----|-----|
| Astro | `ui:astro` | pnpm | astro dev | ✓ | ✓ |
| Next.js | `ui:next` | pnpm | next dev | ✓ | ✓ |
| Nuxt | `ui:nuxt` | pnpm | nuxt dev | ✓ | ✓ |
| SvelteKit | `ui:svelte` | pnpm | vite dev | ✓ | ✓ |
| Remix | `ui:remix` | pnpm | remix dev | ✓ | ✓ |
| Vue (Vite) | `ui:vue` | pnpm | vite | — | ✓ |

#### Mobile / Desktop Adapters

| Platform | Adapter Key | Package Manager | Hot Reload | Auth UI |
|----------|-------------|-----------------|------------|---------|
| Flutter | `mobile:flutter` | pub / flutter | flutter run --hot | Login · Register · Dashboard |
| React Native | `mobile:react-native` | pnpm + expo | expo start | Login · Register · Dashboard |
| Tauri | `desktop:tauri` | pnpm + cargo | tauri dev | Login · Register · Dashboard |
| Electron | `desktop:electron` | pnpm | electron . | Login · Register · Dashboard |

### 9.6 Python Adapter — UV as the Toolchain

UV is the default and only supported Python package manager for all `python:*` adapters. It replaces pip, poetry, pyenv, and virtualenv in one binary.

**Why UV is the correct choice:**

```
UV is written in Rust by Astral (creators of Ruff).
It is 10–100× faster than pip.
It manages Python versions (replaces pyenv).
It manages virtual environments (replaces venv/virtualenv).
It has lock files (uv.lock — replaces poetry.lock / requirements.txt).
It is a single binary — one install, everything included.
```

**How UV is used in Python adapters:**

```bash
# acthur doctor checks for uv
uv --version    ≥ 0.4.0

# acthur new — project initialization
uv init my-service
uv python install 3.12      # pin Python version
uv add fastapi uvicorn alembic psycopg[async] redis

# Generated pyproject.toml
[project]
name = "my-service"
version = "0.1.0"
requires-python = ">=3.12"
dependencies = [
    "fastapi>=0.115.0",
    "uvicorn[standard]>=0.32.0",
    "alembic>=1.13.0",
    "psycopg[async]>=3.2.0",
    "redis>=5.2.0",
    "opentelemetry-sdk>=1.28.0",
    "opentelemetry-instrumentation-fastapi>=0.49b0",
]

[tool.uv]
dev-dependencies = [
    "pytest>=8.3.0",
    "pytest-asyncio>=0.24.0",
    "httpx>=0.27.0",
    "testcontainers[postgres,redis]>=4.8.0",
]

# Dev command (generated by python:fastapi adapter)
DevCommand: uv run uvicorn app.main:app --reload --port 8080

# Build command (production)
BuildCommand: uv run python -m compileall app/

# Test command
TestCommand: uv run pytest

# uv.lock is ALWAYS committed to git
# Ensures reproducible installs across all environments
```

**Generated Dockerfile (python:fastapi + UV):**

```dockerfile
# syntax=docker/dockerfile:1
# Generated by Acthur for python:fastapi adapter

# ── Build stage ──────────────────────────────────────────────────────────────
FROM python:3.12-slim AS builder

# Install UV
COPY --from=ghcr.io/astral-sh/uv:0.4.0 /uv /uvx /bin/

WORKDIR /app

# Install dependencies (cached layer — only reinstalls when pyproject.toml changes)
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev --no-install-project

# Install project
COPY . .
RUN uv sync --frozen --no-dev

# ── Run stage ─────────────────────────────────────────────────────────────────
FROM python:3.12-slim

COPY --from=ghcr.io/astral-sh/uv:0.4.0 /uv /bin/
COPY --from=builder /app /app

WORKDIR /app

ENV PATH="/app/.venv/bin:$PATH"

EXPOSE 8080
USER nobody

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8080"]
```

**acthur doctor for Python adapters:**

```bash
acthur doctor

  Python Toolchain
  ✓  uv                 0.4.18   (package manager + python version manager)
  ✓  python             3.12.7   (managed by uv)
  ○  pyenv              not found  (not needed — uv manages Python versions)

  # If uv not found:
  ✗  uv                 not found
     → required for python:* adapters
     → fix: acthur doctor --fix uv
```

```bash
# Auto-fix installs uv via the official installer
curl -LsSf https://astral.sh/uv/install.sh | sh   (macOS/Linux)
irm https://astral.sh/uv/install.ps1 | iex         (Windows)
```


### 9.6.1 ORM Adapters Per Runtime

Acthur generates all database access through an ORM or query-builder layer. The adapter is selected in `acthur.yml` alongside the backend adapter.

**Go ORM Adapters:**

| Adapter | Style | Notes |
|---------|-------|-------|
| `orm:gorm` | Active-Record, struct-tag driven | Rich feature set; most familiar |
| `orm:sqlc` | SQL-first, compile-time type-safe | Best for teams that write SQL directly |
| `orm:ent` | Graph-based, schema-as-code | Strongest type safety; code-generated traversal |
| `orm:sqlx` | Thin wrapper over database/sql | Maximum control, minimal magic |

**Rust ORM Adapters:**

| Adapter | Style | Notes |
|---------|-------|-------|
| `orm:sqlx-rs` | Compile-time verified SQL | async-native; zero runtime query construction |
| `orm:diesel` | Strongly typed, zero-cost | No runtime query construction; Rust-only |
| `orm:seaorm` | Async-first, migration support | Closest to ActiveRecord feel in Rust |

**ORM–Runtime Compatibility:**

| ORM | Go | Rust | Notes |
|-----|----|----|-------|
| gorm | ✓ | ✗ | Go only |
| sqlc | ✓ | ✗ | Go only |
| ent | ✓ | ✗ | Go only |
| sqlx | ✓ | ✗ | Go only |
| sqlx-rs | ✗ | ✓ | Rust only |
| diesel | ✗ | ✓ | Rust only — rejects with `go:*` adapter |
| seaorm | ✗ | ✓ | Rust only |

`acthur doctor` validates ORM–runtime compatibility at project init time. Selecting `orm:diesel` with `go:fiber` is rejected with a clear error before any code is generated.

### 9.7 Multi-Runtime Dev Doctor Matrix

`acthur doctor` checks only what your specific graph requires:

```
Adapter in acthur.yml         Required tools checked
────────────────────────────────────────────────────────────────────
go:*                          go ≥1.21, air, golang-migrate
rust:*                        rustup, cargo, cargo-watch, sqlx-cli
node:*, ui:*                  node ≥20, pnpm ≥8
bun:*                         bun ≥1.0
python:*                      uv ≥0.4.0   (uv manages Python itself)
php:laravel                   php ≥8.2, composer
mobile:flutter                flutter ≥3.x, dart
mobile:react-native           node ≥20, pnpm, expo-cli
desktop:tauri                 rust toolchain, tauri-cli
db:postgres                   docker (or external DSN)
cache:redis                   docker (or external DSN)
queue:nats                    docker (or external DSN)
```

## 10. Plugin System

### 10.1 Plugin Interface

```go
type Plugin interface {
    Name()      string
    Version()   string
    DependsOn() []string       // other plugin names required first

    Register(k KernelAPI) error
}

type KernelAPI interface {
    OnEvent(event Event, handler func(any))
    RegisterCommand(cmd CLICommand)
    RegisterGenerator(target string, gen Generator)
    RegisterSchema(name string, schema Schema)
    RegisterMiddleware(m Middleware)
    AddNodeToGraph(node *graph.Node)
    AddEdgeToGraph(edge *graph.Edge)
    Graph() GraphReader          // read-only
}
```

### 10.2 Kernel Event Bus

Plugins attach to kernel lifecycle events. This is the only mechanism through which plugins modify system behavior.

```
kernel:graph:before_build       kernel:graph:after_build
kernel:node:before_start        kernel:node:after_start
kernel:node:after_healthy       kernel:node:before_stop
kernel:node:after_stop          kernel:node:on_failure
kernel:proxy:before_request     kernel:proxy:after_request
kernel:proxy:on_error
kernel:db:before_migrate        kernel:db:after_migrate
kernel:db:before_seed           kernel:db:after_seed
kernel:deploy:before            kernel:deploy:after
kernel:deploy:preflight
kernel:contract:registered      kernel:contract:violated
kernel:plugin:loaded            kernel:plugin:error
```

### 10.3 Plugin Loading Order

Plugins are loaded in topological order based on their `DependsOn()` declarations. The kernel validates that no circular dependencies exist between plugins.

---

## 11. Contract Engine

### 11.1 Contract Formats

Acthur accepts contracts in four formats. All are compiled to Acthur's internal contract representation.

```
.contract.yml    → Acthur native format (canonical)
.proto           → Protocol Buffers (imported natively)
.openapi.yml     → OpenAPI 3.x (imported and compiled)
.graphql         → GraphQL SDL (imported and compiled)
```

### 11.2 Native Contract Format

```yaml
contract: users
version: "1"
transport: http            # http | grpc | ws | queue

endpoints:
  - id: create_user
    method: POST
    path: /api/v1/users
    auth: required
    roles: [admin]
    rate_limit:
      max: 100
      window: 1m
    input:
      name:     string(required, max:100)
      email:    email(required, unique)
      role:     enum(admin, member, viewer)
    output:
      id:         ulid
      name:       string
      email:      email
      created_at: timestamp
    errors:
      - 409: email_taken
      - 422: validation_failed

events:
  - id: user.created
    payload: $User
  - id: user.deleted
    payload: { id: ulid }

types:
  User:
    id:         ulid
    name:       string
    email:      email
    role:       enum(admin, member, viewer)
    tenant_id:  ulid?
    created_at: timestamp
```

### 11.3 Contract Runtime Enforcement

```
Dev mode (default):
  Contract violations logged with full request/response detail
  Traffic passes through — development is not blocked
  Violations aggregated in acthur monitor

Strict mode (--strict flag / always in pre-deploy):
  Contract violations return 422 with structured error body
  Traffic blocked at proxy layer
  Deploy gate fails if violations exist in staging

Pre-deploy gate (always runs before acthur deploy):
  All contracts validated structurally
  Breaking changes detected via diff engine
  Unversioned breaking changes block the deploy
```

### 11.4 API Versioning

API versioning is managed at the contract level:

```yaml
contract: users
version: "2"
extends: users@v1     # inherit v1 definition, override below

endpoints:
  - id: get_user
    path: /api/v2/users/:id
    output:
      id:           ulid
      name:         string
      email:        email
      avatar_url:   url?         # new — non-breaking
      display_name: string?      # new — non-breaking
```

Multiple contract versions run simultaneously. The proxy routes to the correct version handler. Deprecated versions emit `Sunset` headers. Breaking changes block the deploy unless a new version is declared.

### 11.5 Contract Diff Rules

```
NON-BREAKING (allowed without version bump):
  ✓ New optional output field added
  ✓ New optional input field added
  ✓ New endpoint added
  ✓ Looser validation constraint (max:100 → max:200)

BREAKING (requires version bump):
  ✗ Field removed from output
  ✗ Field renamed
  ✗ Required field added to input
  ✗ Endpoint path changed
  ✗ HTTP method changed
  ✗ Stricter validation constraint (max:200 → max:100)
  ✗ Auth requirement added
```

---

## 12. Dev Runtime

### 12.1 `acthur dev` Startup Sequence

```
1. Load and validate acthur.yml → build live graph
2. Run acthur doctor → verify environment
3. Resolve startup order → topological sort on depends_on edges
4. Start infra nodes first → docker run for db, cache, queue, storage
5. Wait for infra health checks → TCP connect + ping per infra type
6. Load and initialize all plugins → in DependsOn order
7. Start service nodes in dependency order → spawn child processes
8. Wait for service health checks → GET /health → 200
9. Start dev proxy → unified port (default :4000)
10. Set up DNS → write /etc/hosts or configure local DNS resolver
11. Start file watcher → graph-aware hot reload cascade
12. Print ready message with all service URLs
```

### 12.2 Local .test Domain

Every Acthur project gets its own `.test` domain with subdomain routing for all UI nodes:

```
http://vetangle.test             → main web UI
http://api.vetangle.test         → API service
http://backoffice.vetangle.test  → backoffice UI
http://mail.vetangle.test        → Mailpit (email preview)
http://storage.vetangle.test     → MinIO console
http://analytics.vetangle.test   → PostHog UI (if analytics active)
http://traces.vetangle.test      → Jaeger UI (if observability active)
http://metrics.vetangle.test     → Grafana (if observability active)
```

DNS strategies (Acthur selects automatically via doctor):
1. Local DNS resolver (dnsmasq/Acrylic) — cleanest, one-time setup
2. /etc/hosts injection — works everywhere, requires sudo once
3. PAC proxy — fallback, no system changes required

### 12.3 Hot Reload Cascade

File changes propagate through the graph. When a contract file changes, all nodes with data_flow edges to the changed contract's service are notified:

```
contracts/users.contract.yml changes
         │
         ▼ (satisfies edge)
    api regenerates handler     ← adapter-level rebuild
         │
         ▼ (data_flow edge)
    web regenerates TS client   ← contract client update
         │
         ▼ (data_flow edge)
    backoffice regenerates      ← contract client update
```

### 12.4 Process Supervision

All service processes are supervised by Acthur's process manager:
- Crashed processes are restarted with exponential backoff
- Restart count is surfaced in the terminal and monitor
- Persistent crashes (> 5 restarts) prompt the developer
- Graceful shutdown on Ctrl+C (SIGTERM cascade in dependency reverse order)

---

## 13. Deploy Runtime

### 13.1 Core Principle

The same graph that runs `acthur dev` is the source of truth for `acthur deploy`. All deployment manifests — Docker Compose, Helm charts, platform configs — are projections of the graph onto the target execution model. They are never manually written.

### 13.2 Pre-Deploy Gate

Every `acthur deploy` runs these checks first. Any single failure blocks the deploy:

```
1. acthur graph validate        → no broken or invalid graph edges
2. acthur contract validate     → all contracts structurally valid
3. acthur contract diff         → no unversioned breaking changes
4. service builds               → all service nodes build cleanly
5. test suite                   → acthur test --ci (if not skipped with --skip-tests)
6. migration status             → no unapplied migrations without a plan
7. secret references            → all ${VAR} references resolvable in target environment
8. security scan                → CSP, CORS config valid (security plugin active)
```

### 13.3 Deploy Targets

```
deploy:coolify    → Coolify API + webhook (self-hosted PaaS)
deploy:dokploy    → Dokploy API (self-hosted PaaS, Docker Swarm + Traefik)
deploy:fly        → fly.toml + flyctl deploy
deploy:railway    → railway.json + railway CLI
deploy:render     → render.yaml + Render API
deploy:docker     → docker-compose.prod.yml + remote compose up
deploy:k8s        → Helm chart + kubectl apply
```

### 13.4 Coolify Deploy Target

```yaml
# Coolify — self-hosted PaaS (recommended for most teams)
environments:
  production:
    context: cloud
    target: coolify
    host: yourserver.com
    api_key: ${COOLIFY_API_KEY}
    team_id: ${COOLIFY_TEAM_ID}
```

Acthur generates Coolify resources from the graph:
```
Service nodes  → Coolify Applications (Docker image per service)
Infra nodes    → Coolify Services (managed Postgres, Redis, etc.)
data_flow edges → Service-to-service environment variable wiring
migrates edges  → Migration run ordering in Coolify
```

### 13.5 Dokploy Deploy Target

Dokploy is an open-source self-hosted PaaS built on Docker Swarm and Traefik. It is MIT-licensed and requires no vendor account — run it on any VPS.

```yaml
# acthur.yml
environments:
  production:
    context: cloud
    target: dokploy
    host: yourserver.com          # VPS where Dokploy is installed
    api_key: ${DOKPLOY_API_KEY}
    project_name: ${PROJECT_NAME}
```

What Acthur generates for Dokploy:

```
Service nodes    → Dokploy Applications (each with its Dockerfile)
Infra nodes      → Dokploy Databases / Services (Postgres, Redis, etc.)
proxied_through  → Traefik routing rules (domain, path, TLS)
data_flow edges  → Environment variable injection between services
migrates edges   → Pre-deploy migration commands
```

Generated Dokploy configuration (derived from graph):

```json
// .acthur/deploy/dokploy.json — generated, never manually written
{
  "project": "vetangle",
  "services": [
    {
      "name": "api",
      "type": "application",
      "source": { "type": "dockerfile", "path": "services/api/Dockerfile" },
      "domains": [
        { "host": "api.vetangle.com", "port": 8080, "https": true }
      ],
      "env": [
        { "key": "DATABASE_URL",  "value": "${DATABASE_URL}" },
        { "key": "REDIS_URL",     "value": "${REDIS_URL}" },
        { "key": "APP_SECRET",    "value": "${APP_SECRET}" }
      ],
      "health_check": { "path": "/health", "interval": 30 }
    },
    {
      "name": "web",
      "type": "application",
      "source": { "type": "dockerfile", "path": "web/Dockerfile" },
      "domains": [
        { "host": "vetangle.com",  "port": 3000, "https": true },
        { "host": "www.vetangle.com", "port": 3000, "https": true }
      ]
    },
    {
      "name": "db",
      "type": "database",
      "engine": "postgres",
      "version": "16"
    },
    {
      "name": "cache",
      "type": "database",
      "engine": "redis",
      "version": "7"
    }
  ],
  "swarm": {
    "replicas": { "api": 2, "web": 2 },
    "update_config": { "parallelism": 1, "delay": "10s" },
    "rollback_config": { "parallelism": 1 }
  }
}
```

Deployment command:

```bash
acthur deploy --target dokploy
  [acthur] running preflight checks...
  [acthur] ✓ graph valid
  [acthur] ✓ contracts valid · no breaking changes
  [acthur] ✓ all services build cleanly
  [acthur] building images...
  [acthur] ✓ api image pushed  → registry.vetangle.com/api:v1.2.3
  [acthur] ✓ web image pushed  → registry.vetangle.com/web:v1.2.3
  [acthur] deploying to Dokploy at yourserver.com...
  [acthur] ✓ db service created
  [acthur] ✓ cache service created
  [acthur] ✓ running migrations on db...
  [acthur] ✓ api deployed (2 replicas, rolling update)
  [acthur] ✓ web deployed (2 replicas, rolling update)
  [acthur] ✓ Traefik routes configured
  [acthur] ✓ TLS certificates provisioned
  [acthur] deployed → https://vetangle.com
```

Docker Swarm features Acthur uses via Dokploy:
```
Rolling updates   → zero-downtime deploys
Replica scaling   → configurable per service node
Health checks     → derived from /health endpoint contract
Rollback          → automatic on health check failure
Secrets           → Docker Swarm secrets (not env vars)
Networks          → isolated internal overlay network per project
```

### 13.6 Fly.io Deploy Target

```yaml
environments:
  production:
    context: cloud
    target: fly
    region: lhr                  # fly.io region
```

Generated `fly.toml` (per service node):

```toml
# services/api/fly.toml — generated from graph
app = "vetangle-api"
primary_region = "lhr"

[build]
  dockerfile = "Dockerfile"

[env]
  PORT = "8080"

[[services]]
  protocol = "tcp"
  internal_port = 8080

  [[services.ports]]
    port = 443
    handlers = ["tls", "http"]

  [services.concurrency]
    type = "connections"
    hard_limit = 25
    soft_limit = 20

  [[services.tcp_checks]]
    interval = "15s"
    timeout = "2s"
    grace_period = "1s"

  [[services.http_checks]]
    interval = "10s"
    timeout = "2s"
    grace_period = "5s"
    method = "get"
    path = "/health"
    protocol = "http"
```

### 13.7 Docker Deploy Target

```bash
acthur deploy --target docker
```

Generates `docker-compose.prod.yml` from the graph, builds and pushes images to a registry, transfers the compose file to the remote server, and runs `docker compose up -d`. Hardened Dockerfiles, network isolation, non-root users — all generated.

### 13.8 Kubernetes (Helm Chart) Deploy Target

```bash
acthur deploy --target k8s
```

Generates a Helm chart from the graph:

```
charts/vetangle/
├── Chart.yaml
├── values.yaml                   ← configurable values derived from graph
├── templates/
│   ├── api/
│   │   ├── deployment.yaml       ← go:fiber service
│   │   ├── service.yaml
│   │   ├── hpa.yaml              ← horizontal pod autoscaler
│   │   └── configmap.yaml
│   ├── web/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── ingress.yaml              ← derived from proxied_through edges
│   ├── secrets.yaml              ← external-secrets or sealed-secrets
│   └── NOTES.txt
```

### 13.9 Multi-Environment Deploy

```bash
acthur deploy                    # → production (default)
acthur deploy --env staging      # → staging environment
acthur deploy --env preview      # → ephemeral preview environment
acthur deploy --dry-run          # → show plan without executing
acthur deploy --skip-tests       # → skip test suite (not recommended)
acthur deploy --target dokploy   # → override target
```

### 13.10 Deploy Target Comparison

| Target | Hosting | Setup Complexity | Docker Swarm | K8s | Cost |
|--------|---------|-----------------|--------------|-----|------|
| `deploy:coolify` | Self-hosted | Low | No | No | VPS cost only |
| `deploy:dokploy` | Self-hosted | Low | ✓ | No | VPS cost only |
| `deploy:fly` | Cloud | Very low | No | No | Pay per use |
| `deploy:railway` | Cloud | Very low | No | No | Pay per use |
| `deploy:render` | Cloud | Low | No | No | Pay per use |
| `deploy:docker` | Self-hosted | Medium | No | No | VPS cost only |
| `deploy:k8s` | Any | High | No | ✓ | Varies |


---

## 14. Generator Engine

### 14.1 Generation Philosophy — Ready to Use, Not Empty Shells

**The fundamental principle:** Every frontend, mobile, and desktop application scaffolded by Acthur is a fully working, functional application from the moment `acthur dev` is run. Not a blank template. Not a hello-world placeholder.

The level of completeness maps exactly to which plugins are installed. Acthur knows your stack at generation time — the auth strategy, the rbac roles, the multitenancy strategy, the identifier type, the contracts. There is no reason to generate empty files.

```
No plugins:
  Frontend   → layout, nav, home page, API health indicator

With auth plugin:
  Frontend   → login · register · forgot password · protected routes
               auth context/store · JWT refresh logic · logout

With auth + OAuth2:
  Frontend   → social login buttons · OAuth callback handling

With auth + rbac:
  Frontend   → role-based navigation · permission-gated components
               admin pages visible only to admin role

With auth + multitenancy:
  Frontend   → tenant-aware API client · tenant context headers
               subdomain-based tenant routing

With auth + rbac + admin plugin:
  Frontend   → full admin dashboard · user management table
               role assignment UI · system statistics · audit log viewer

Mobile (any config):
  Auth stack     → login screen · register screen · forgot password screen
  App stack      → home screen · profile screen · settings screen
  Role-based nav → bottom tabs or drawer differ by user role
  Deep linking   → magic link / OAuth callback handling
  Biometric auth → if MFA + passkey selected
```

### 14.2 Generation Principles

- **Contract-first:** generators take a contract as input, produce framework-native code
- **Idempotent:** re-running a generator merges, never overwrites; tracked in `.acthur/generated.lock`
- **Adapter-aware:** the same generator target produces different output per adapter
- **Zero Acthur imports:** all generated code is pure framework-native; no runtime dependency on Acthur
- **Ready to run:** all generated code compiles and passes tests with no additional work required

### 14.3 Rich Frontend Scaffolding — Per Adapter

#### `ui:astro` — Auth + RBAC + Dashboard

When `auth` + `rbac` plugins are active, Acthur generates a complete Astro site:

```
web/
├── src/
│   ├── layouts/
│   │   ├── Base.astro              base HTML, OTel, analytics injection
│   │   ├── Auth.astro              unauthenticated layout (login/register)
│   │   └── Dashboard.astro         authenticated layout with sidebar + topnav
│   │
│   ├── pages/
│   │   ├── index.astro             → redirects to /dashboard or /login
│   │   │
│   │   ├── auth/
│   │   │   ├── login.astro         email/password login form
│   │   │   ├── register.astro      registration form with validation
│   │   │   ├── forgot-password.astro
│   │   │   ├── reset-password.astro
│   │   │   ├── verify-email.astro
│   │   │   └── mfa.astro           MFA challenge (if mfa plugin active)
│   │   │
│   │   ├── dashboard/
│   │   │   ├── index.astro         main dashboard — real API data, stats cards
│   │   │   └── profile.astro       user profile — edit name, email, avatar
│   │   │
│   │   ├── admin/                  (if rbac + roles include admin)
│   │   │   ├── index.astro         admin overview — system stats
│   │   │   ├── users.astro         user management table — CRUD
│   │   │   └── roles.astro         role assignment UI
│   │   │
│   │   └── [tenant]/               (if multitenancy: subdomain active)
│   │       └── dashboard.astro     tenant-scoped dashboard
│   │
│   ├── components/
│   │   ├── auth/
│   │   │   ├── LoginForm.astro     form with validation, error states
│   │   │   ├── RegisterForm.astro
│   │   │   ├── OAuthButtons.astro  Google/GitHub buttons (if oauth2 active)
│   │   │   └── MFAForm.astro
│   │   │
│   │   ├── ui/
│   │   │   ├── Button.astro
│   │   │   ├── Input.astro         validated input with error display
│   │   │   ├── DataTable.astro     sortable, paginated table
│   │   │   ├── StatsCard.astro     dashboard metric card
│   │   │   ├── Sidebar.astro       role-based nav items
│   │   │   ├── TopNav.astro        user menu, notifications
│   │   │   ├── Toast.astro         success/error notifications
│   │   │   └── LoadingSpinner.astro
│   │   │
│   │   └── guards/
│   │       ├── RequireAuth.astro   redirects to /auth/login if not authenticated
│   │       └── RequireRole.astro   renders 403 if role insufficient
│   │
│   ├── lib/
│   │   ├── auth.ts                 JWT management, refresh, storage
│   │   ├── api.ts                  typed API client generated from contracts
│   │   ├── tenant.ts               tenant resolution from subdomain
│   │   └── permissions.ts          client-side RBAC helpers
│   │
│   └── middleware.ts               Astro middleware — auth check on protected routes
│
└── package.json                    pnpm dependencies (all pre-configured)
```

The generated `login.astro` is a complete, working login page — form validation, error handling, JWT storage, redirect on success, loading state:

```astro
---
// src/pages/auth/login.astro — Generated by Acthur
// auth plugin: jwt + RS256 + Google OAuth2
import AuthLayout from '@/layouts/Auth.astro'
import LoginForm from '@/components/auth/LoginForm.astro'
import OAuthButtons from '@/components/auth/OAuthButtons.astro'

// Redirect if already authenticated
const token = Astro.cookies.get('auth_token')
if (token) return Astro.redirect('/dashboard')
---

<AuthLayout title="Sign in to VetAngle">
  <div class="auth-card">
    <h1>Welcome back</h1>
    <LoginForm redirectTo="/dashboard" />
    <div class="divider">or continue with</div>
    <OAuthButtons providers={['google', 'github']} />
    <p>
      Don't have an account? <a href="/auth/register">Sign up</a>
    </p>
  </div>
</AuthLayout>
```

The generated dashboard has real data from the contracts:

```astro
---
// src/pages/dashboard/index.astro — Generated by Acthur
// Fetches data from users.contract.yml + appointments.contract.yml
import DashboardLayout from '@/layouts/Dashboard.astro'
import StatsCard from '@/components/ui/StatsCard.astro'
import { api } from '@/lib/api'
import { requireAuth } from '@/lib/auth'

const user = await requireAuth(Astro)    // redirects to login if not authenticated
const stats = await api.getStats()       // typed call from contracts
---

<DashboardLayout title="Dashboard" {user}>
  <div class="stats-grid">
    <StatsCard label="Appointments Today" value={stats.appointmentsToday} />
    <StatsCard label="Active Vets" value={stats.activeVets} />
    <StatsCard label="Pending Reviews" value={stats.pendingReviews} />
  </div>
  <!-- ... more dashboard content, all from contracts -->
</DashboardLayout>
```

---

#### `ui:next` — App Router with Auth Middleware

```
backoffice/
├── app/
│   ├── (auth)/                     auth route group — no sidebar
│   │   ├── login/
│   │   │   └── page.tsx            login form (next-auth or custom JWT)
│   │   ├── register/
│   │   │   └── page.tsx
│   │   ├── forgot-password/
│   │   │   └── page.tsx
│   │   └── mfa/
│   │       └── page.tsx
│   │
│   ├── (dashboard)/                protected route group
│   │   ├── layout.tsx              sidebar + topnav — reads user from session
│   │   ├── page.tsx                dashboard home — stats cards, recent activity
│   │   ├── profile/
│   │   │   └── page.tsx
│   │   │
│   │   └── admin/                  (if rbac includes admin role)
│   │       ├── page.tsx            admin overview
│   │       ├── users/
│   │       │   ├── page.tsx        users table — sortable, filterable, paginated
│   │       │   └── [id]/
│   │       │       └── page.tsx    user detail — edit, assign roles
│   │       └── settings/
│   │           └── page.tsx
│   │
│   ├── layout.tsx                  root layout — providers, fonts
│   └── api/
│       └── auth/
│           └── [...nextauth]/
│               └── route.ts        NextAuth handler (if oauth2 active)
│
├── components/
│   ├── auth/
│   │   └── ...                     same completeness as Astro
│   ├── ui/
│   │   ├── DataTable.tsx           tanstack-table based, sorting + pagination
│   │   ├── StatsCard.tsx
│   │   ├── Sidebar.tsx             role-based nav, collapses on mobile
│   │   ├── TopNav.tsx
│   │   └── ...
│   └── providers/
│       ├── AuthProvider.tsx        session context
│       ├── TenantProvider.tsx      (if multitenancy active)
│       └── QueryProvider.tsx       TanStack Query setup
│
├── lib/
│   ├── auth.ts                     session utilities, role checks
│   ├── api/                        typed clients generated from contracts
│   │   ├── users.ts                → all endpoints from users.contract.yml
│   │   └── appointments.ts        → all endpoints from appointments.contract.yml
│   └── tenant.ts
│
└── middleware.ts                   Next.js middleware — auth + RBAC on every route
```

Generated Next.js middleware:

```typescript
// middleware.ts — Generated by Acthur
// auth plugin: JWT + RS256 · rbac: [admin, vet, agent, support, user]
import { NextRequest, NextResponse } from 'next/server'
import { verifyToken } from '@/lib/auth'

const PUBLIC_PATHS = ['/login', '/register', '/forgot-password', '/auth/callback']

const ROLE_PATHS: Record<string, string[]> = {
  '/admin': ['admin'],                    // from rbac plugin roles config
  '/admin/users': ['admin', 'support'],
}

export async function middleware(req: NextRequest) {
  const { pathname } = req.nextUrl
  if (PUBLIC_PATHS.some(p => pathname.startsWith(p))) return NextResponse.next()

  const token = req.cookies.get('auth_token')?.value
  if (!token) return NextResponse.redirect(new URL('/login', req.url))

  const payload = await verifyToken(token)
  if (!payload) return NextResponse.redirect(new URL('/login', req.url))

  // RBAC check — derived from roles config in acthur.yml
  const requiredRoles = Object.entries(ROLE_PATHS)
    .find(([path]) => pathname.startsWith(path))?.[1]

  if (requiredRoles && !requiredRoles.includes(payload.role)) {
    return NextResponse.redirect(new URL('/403', req.url))
  }

  return NextResponse.next()
}
```

---

#### `ui:vue` — Pinia Store + Vue Router Guards

```
web/
├── src/
│   ├── router/
│   │   └── index.ts            Vue Router — auth guards on protected routes
│   ├── stores/
│   │   ├── auth.ts             Pinia auth store — user, token, refresh
│   │   └── tenant.ts           Pinia tenant store (if multitenancy)
│   ├── views/
│   │   ├── auth/               login, register, forgot-password, mfa
│   │   ├── dashboard/          home, profile
│   │   └── admin/              (if rbac + admin role)
│   ├── components/
│   │   ├── auth/               LoginForm, RegisterForm, OAuthButtons
│   │   ├── layout/             AppSidebar, AppTopNav, AppLayout
│   │   └── ui/                 DataTable, StatsCard, Toast, LoadingSpinner
│   └── lib/
│       ├── api.ts              typed API client from contracts
│       └── permissions.ts      role check helpers
```

---

#### `mobile:flutter` — GoRouter + Riverpod

```
lib/
├── main.dart                   app entry, GoRouter, provider scope
├── router/
│   └── router.dart             GoRouter config — auth guard, redirect logic
├── features/
│   ├── auth/
│   │   ├── screens/
│   │   │   ├── login_screen.dart      username/email + password form
│   │   │   ├── register_screen.dart   full registration with validation
│   │   │   ├── forgot_password_screen.dart
│   │   │   └── mfa_screen.dart        TOTP code entry (if mfa active)
│   │   ├── providers/
│   │   │   └── auth_provider.dart     Riverpod auth state
│   │   └── services/
│   │       └── auth_service.dart      JWT management, refresh, secure storage
│   │
│   ├── dashboard/
│   │   ├── screens/
│   │   │   ├── home_screen.dart       stats cards, recent activity, real API data
│   │   │   └── profile_screen.dart    user profile, edit, logout
│   │   └── providers/
│   │       └── dashboard_provider.dart
│   │
│   └── admin/                         (if rbac includes admin role)
│       └── screens/
│           └── users_screen.dart      user list + management
│
├── core/
│   ├── api/                           typed API client from contracts
│   │   ├── api_client.dart            Dio HTTP client, auth interceptor
│   │   ├── users_api.dart             generated from users.contract.yml
│   │   └── appointments_api.dart      generated from appointments.contract.yml
│   ├── models/                        typed models from contracts
│   └── widgets/
│       ├── auth_guard.dart            widget that redirects if not authenticated
│       ├── role_guard.dart            widget that shows 403 if role insufficient
│       └── ...
└── pubspec.yaml                       flutter_riverpod, go_router, dio, pre-configured
```

Generated Flutter router with auth guard:

```dart
// lib/router/router.dart — Generated by Acthur
// auth plugin: jwt · rbac: [admin, vet, agent, support, user]
import 'package:go_router/go_router.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

final routerProvider = Provider<GoRouter>((ref) {
  final authState = ref.watch(authProvider);

  return GoRouter(
    redirect: (context, state) {
      final isLoggedIn   = authState.isAuthenticated;
      final isAuthRoute  = state.uri.path.startsWith('/auth');
      final isAdminRoute = state.uri.path.startsWith('/admin');

      if (!isLoggedIn && !isAuthRoute)  return '/auth/login';
      if (isLoggedIn  &&  isAuthRoute)  return '/dashboard';
      if (isAdminRoute && authState.user?.role != 'admin') return '/403';
      return null;
    },
    routes: [
      GoRoute(path: '/auth/login',    builder: (_, __) => const LoginScreen()),
      GoRoute(path: '/auth/register', builder: (_, __) => const RegisterScreen()),
      GoRoute(path: '/auth/mfa',      builder: (_, __) => const MFAScreen()),
      ShellRoute(
        builder: (_, __, child) => AppShell(child: child),  // bottom nav
        routes: [
          GoRoute(path: '/dashboard', builder: (_, __) => const HomeScreen()),
          GoRoute(path: '/profile',   builder: (_, __) => const ProfileScreen()),
          GoRoute(path: '/admin',     builder: (_, __) => const AdminScreen()),
        ],
      ),
    ],
  );
});
```

---

#### `mobile:react-native` — Expo Router + Zustand

```
app/
├── (auth)/
│   ├── login.tsx               login screen — styled, validated form
│   ├── register.tsx
│   ├── forgot-password.tsx
│   └── mfa.tsx
├── (tabs)/                     authenticated tab navigator
│   ├── _layout.tsx             tab bar — role-based tab visibility
│   ├── index.tsx               home/dashboard — stats, recent items
│   ├── profile.tsx             user profile, edit, logout
│   └── admin.tsx               (if rbac includes admin role)
├── _layout.tsx                 root layout — auth redirect logic
└── +not-found.tsx

store/
├── auth.store.ts               Zustand auth store — user, token, refresh
└── tenant.store.ts             (if multitenancy active)

lib/
├── api/
│   ├── client.ts               axios/fetch client — auth interceptor
│   ├── users.ts                generated from users.contract.yml
│   └── appointments.ts
└── auth.ts                     JWT management, expo-secure-store

app.json                        Expo config — deep linking for OAuth/magic link
```

---

### 14.4 Rich Scaffolding — What Is Always Generated

Regardless of adapter, these are always present and always working:

```
Auth pages/screens          ← every login/register/reset flow
Protected route guards      ← middleware/hooks/router guards
Role-based navigation       ← sidebar/tabs/drawer differ by role
Typed API client            ← generated from contracts, zero manual writing
Loading and error states    ← every API call has loading + error handling
Toast/notification system   ← success/error feedback on every action
Responsive layout           ← works on desktop, tablet, mobile
Dark mode support           ← system preference respected
```

### 14.5 What Is NOT Pre-Filled

Generated code makes no assumptions about your business domain:

```
Dashboard stats             ← placeholder stats, developer fills real queries
List pages                  ← table structure from contracts, rows are empty
Domain-specific components  ← VetAngle appointment calendar, not generated
Business logic in services  ← handlers and resolvers are stubs
Email templates             ← structure generated, copy is placeholder
```

The rule: **auth, routing, API clients, guards, and layout are complete. Business domain is left to the developer.**

### 14.6 Generator Targets

```
from-contract    → full CRUD from a .contract.yml file
model            → data model + migration + basic CRUD + list/detail pages
auth             → complete auth system (all pages, guards, stores, API client)
rbac             → role middleware + role-based nav + permission guards
tenant           → tenant middleware + tenant-aware API client
migration        → single migration file
client           → typed HTTP/GraphQL client from a contract (caller service)
docs             → API documentation from contracts
ci               → CI/CD pipeline config from graph
ai-context       → AI tool context files from graph + contracts
skill            → custom AI skill file
```

### 14.7 Contract-to-Code Pipeline

```
users.contract.yml + go:fiber backend + ui:astro frontend

  ↓ generator engine

Backend (go:fiber):
  internal/users/handler.go          HTTP handlers, all endpoints, auth middleware
  internal/users/service.go          service interface + stub
  internal/users/repository.go       DB layer, parameterized queries only
  internal/users/dto.go              input/output structs, validation
  internal/users/handler_test.go     HTTP tests
  internal/users/service_test.go     unit tests

Frontend (ui:astro):
  src/lib/api/users.ts               typed fetch client, all endpoints
  src/pages/dashboard/users.astro    users list page — table, pagination
  src/pages/dashboard/users/[id].astro   user detail/edit page

Database:
  db/migrations/<timestamp>_create_users.sql

All of these compiled, typed, tested, and ready to run.
No manual wiring required.
```

## 15. Plugin Catalogue

### 15.1 Core Plugins

**`migrations`**
Manages database schema versioning. Supports golang-migrate (Go), sqlx-migrate (Rust), Alembic (Python), Prisma (Node). Auto-runs in dev before service start. Acthur db commands wrap the underlying tool.

**`auth`**
Full authentication system. Strategies: JWT (RS256/HS256), Session (Redis/DB backed), OAuth2 (Google, GitHub, Microsoft, custom OIDC), Magic Link, MFA (TOTP, SMS OTP, Email OTP, WebAuthn/Passkey). All strategies are composable. OAuth2 is an identity source that feeds JWT or Session. MFA is a pipeline stage on top of any primary strategy.

**`rbac`**
Role-Based Access Control. Generates roles/permissions schema. Derives enforcement middleware from contract endpoint `roles` declarations. Roles configurable in wizard. DependsOn: auth.

**`multitenancy`**
Three strategies: schema-isolation (PostgreSQL schemas, strong isolation, compliance-grade), row-level (tenant_id FK, simpler, high scale), database-isolation (maximum separation, high complexity). Generates resolver, middleware, provisioner, and migrator. Connection pool `AfterRelease` hook automatically resets `search_path` in schema strategy preventing cross-tenant data leakage.

**`feature-flags`**
Local JSON provider by default. Adapters: Flagsmith (self-hosted), GrowthBook (self-hosted), LaunchDarkly. Generates flag evaluation middleware and typed flag registry.

**`admin`**
Auto-generates admin panel from graph and contracts. Produces a data management interface for all models without manual UI work.

**`https`**
Dev: generates local CA via mkcert, issues certs for `*.APP_NAME.test`, installs CA in system trust store. Production: Caddy/Nginx TLS config, Let's Encrypt ACME, Cloudflare DNS challenge.

**`secrets`**
Generates a `Provider` interface. Adapters: local `.env` (dev only), HashiCorp Vault, Doppler, Infisical, AWS SSM, GCP Secret Manager, Azure Key Vault. `acthur secrets rotate <key>` triggers rotation in provider and restarts affected services.

**`observability`**
Full OpenTelemetry stack. Signals: traces (Jaeger/Tempo), metrics (Prometheus + Grafana), logs (Loki + Grafana). Auto-instruments HTTP handlers, DB queries, and cache operations. Pre-built Grafana dashboards per adapter. All services become OTel nodes in the graph.

**`security`**
Generates CORS allowlist from graph data_flow edges (exact origins, not wildcards). Generates CSP, HSTS, X-Frame-Options, Referrer-Policy headers. Rate limiting per IP, per user, per tenant. Input sanitization in DTOs. SQL injection prevention via parameterized query enforcement in generated repository layer. Audit logging for auth events and admin actions.

**`testing`**
Generates test files alongside every generated source file. Categories: unit (mocked dependencies), integration (testcontainers), contract (service honors its contract), E2E (Playwright for UIs), load (k6 scripts from contracts). `acthur test` command surface wraps all categories.

**`ci-cd`**
Generates CI/CD pipeline config from graph. Targets: GitHub Actions, GitLab CI, CircleCI. Pipeline includes: graph validate, contract validate, per-service tests, contract compliance tests, build all, staging dry-run, production deploy on main branch.

**`docs`**
Generates living documentation from contracts, graph, and plugin config. Targets: Astro docs site, Nextra (Next.js), README update. Includes: API reference per contract, architecture diagram (Mermaid from graph), auth docs, deployment runbook.

**`i18n`**
Locale detection middleware (Accept-Language / cookie / user preference). Translation file structure (JSON per locale). Providers: local files, Tolgee, Crowdin, PhraseApp. Generates typed translation helpers for both backend (error messages, emails) and frontend nodes.

### 15.2 Analytics Plugins

**`product-analytics`**
Tracks user behavior. Adapters: PostHog (self-hosted, recommended), Segment, Mixpanel, Amplitude. Generates typed event structs from auth plugin events and contract definitions. Server-side events for reliability (no ad-block impact). PostHog runs as an infra node when self-hosted.

**`web-analytics`**
Privacy-first page view tracking. Adapters: Plausible (self-hosted), Umami (self-hosted), Fathom, Pirsch. Injects tracking script into all frontend nodes with `data_flow` edge to the analytics node. GDPR compliant by default (cookieless options). Generates cookie consent component for frontend nodes.

**`business-analytics`**
SQL dashboards over your own data. Adapters: Metabase (self-hosted), Apache Superset, Lightdash. Generates read-only analytics DB user with SELECT-only grants. Metabase/Superset runs as infra node.

### 15.3 AI Plugin

See Section 17 for full specification.

---

## 16. Analytics

### 16.1 Three Distinct Analytics Concerns

```
Product Analytics    → what are users doing in the app?
                       events, funnels, retention, feature usage

Web Analytics        → who is visiting the site?
                       pageviews, sessions, referrers, bounce rate

Business Analytics   → how is the business performing?
                       SQL-based BI over your own database
```

Infrastructure analytics (service health, error rates, latency) is covered by the observability plugin and is not part of the analytics plugins.

### 16.2 Graph Integration

All analytics providers are infra nodes in the graph. This means they follow the same lifecycle (startup, health checks, dev URLs) and receive the same operational treatment as Postgres or Redis:

```yaml
nodes:
  analytics:
    type: infra
    adapter: analytics:posthog
    dev_url: analytics.vetangle.test
    port: 8000

  web-analytics:
    type: infra
    adapter: webanalytics:plausible
    dev_url: plausible.vetangle.test

edges:
  - api  → analytics     : data_flow   # server-side events
  - web  → analytics     : data_flow   # frontend events
  - web  → web-analytics : data_flow   # page views
```

### 16.3 Privacy Compliance

Analytics plugin wizard collects:
- GDPR / CCPA / HIPAA compliance requirements
- Cookie consent requirements
- Data retention policy
- Anonymization preferences

Generated code reflects these selections. Cookieless analytics adapters (Plausible, Umami) require no consent banner.

---

## 17. AI and Agentic Coding Integration

### 17.1 The Core Insight

Acthur possesses what AI coding agents lack: a complete, structured, live, typed model of the entire application system. The graph, contracts, plugin configuration, and adapter selection collectively represent the architecture document that AI agents typically must infer by reading individual files. Acthur makes this model directly available to AI tools.

### 17.2 `acthur generate ai-context` — User-Selected Tool Generation

The developer selects which AI tool they use. Acthur generates only for that tool. Context files for other tools are never generated unless explicitly requested. Re-running the command detects previously generated tools from `.acthur/ai.lock` and offers to regenerate for the same tools or add a new one.

**Supported Tools:**

| Tool | Generated File(s) | Format |
|------|-------------------|--------|
| Claude Code | `CLAUDE.md` + `.claude/commands/` + `.claude/skills/` | Markdown |
| Cursor | `.cursorrules` + `.cursor/skills/` | MDC |
| GitHub Copilot | `.github/copilot-instructions.md` | Markdown |
| Windsurf | `.windsurfrules` + `.windsurf/skills/` | Markdown |
| Continue.dev | `.continue/config.json` + `.continue/skills/` | JSON + Markdown |
| Aider | `.aider.conf.yml` + `.aider/conventions/` | YAML + Markdown |
| Zed AI | `.zed/settings.json` | JSON |

### 17.3 Context vs Skills — Strictly Separated

**Context file** answers: *"What is this project?"*
- Stack, architecture, all service relationships
- Layer patterns, naming conventions
- Active plugins and what they provide
- Critical rules the AI must follow
- Static description, read once at conversation start

**Skill files** answer: *"How do I do X in this specific project?"*
- Step-by-step procedures for recurring tasks
- Project-specific rules embedded in each step
- Examples from the actual codebase
- Generated from the intersection of active plugins + selected adapter
- Referenced when a specific task type is triggered

**A project without multitenancy gets no tenant skill. A project using `rust:axum` gets Rust-specific skills. Skills are always accurate because they are derived from the actual graph.**

### 17.4 Generated Skill Set

Skills generated depend on active plugins and selected adapter:

| Skill | Generated When |
|-------|----------------|
| `create-endpoint` | Always (adapter-specific steps) |
| `create-model` | migrations plugin active |
| `add-migration` | migrations plugin active |
| `auth-patterns` | auth plugin active |
| `tenant-patterns` | multitenancy plugin active |
| `rbac-patterns` | rbac plugin active |
| `testing` | Always |
| `error-handling` | Always (adapter-specific patterns) |
| `contract-workflow` | Always |
| `deploy` | Deploy target configured |
| `debug-runtime` | Always |
| `observability-patterns` | observability plugin active |
| `analytics-events` | product-analytics plugin active |
| `i18n-patterns` | i18n plugin active |

Custom skills can be added at any time:
```bash
acthur generate skill <name>
```

### 17.5 Acthur as an MCP Server

```bash
acthur mcp serve     # starts MCP server (stdio or TCP)
```

Exposes the full live Acthur system to any MCP-compatible AI tool (Claude Code, Cursor, Continue.dev, and others).

**MCP Tools Exposed:**

```
acthur_graph_read           → full live graph as structured JSON
acthur_graph_node_files     → source files owned by a node
acthur_contract_read        → parsed contract, structured
acthur_contract_list        → all contracts with endpoints
acthur_node_health          → live health status of all nodes
acthur_generate             → trigger an Acthur generator
acthur_run_migration        → create and run a migration
acthur_test_run             → run tests for a node
acthur_doctor               → environment health status
acthur_logs                 → stream logs from a node
acthur_contract_validate    → validate contracts
acthur_graph_validate       → validate graph structure
```

### 17.6 `acthur agent` Commands

AI-powered commands with full graph + contract context:

```bash
acthur agent explain "<question about the system>"
acthur agent generate "<feature or endpoint description>"
acthur agent diagnose "<description of the problem>"
acthur agent review                      # review current git diff
acthur agent update-contract "<change description>"
acthur agent document "<path>"
```

### 17.7 LLM Provider Configuration

```yaml
ai:
  provider: claude        # claude | openai | groq | ollama
  model: claude-sonnet-4-20250514
  api_key: ${ANTHROPIC_API_KEY}
  context_strategy: graph-first

  # Local model (no API key required)
  # provider: ollama
  # model: codestral
  # base_url: http://localhost:11434
```

### 17.8 Stale Context Detection

When `acthur.yml` changes after context files were last generated, Acthur warns at `acthur dev` start:

```
[acthur] ⚠  AI context files are outdated (graph changed since last generation)
[acthur]    Run: acthur generate ai-context to update
```

The `.acthur/ai.lock` file records a hash of `acthur.yml` at generation time for this comparison.

---

## 18. Security Architecture — Four-Layer Model

Security in Acthur is not a plugin that gets bolted on. It operates simultaneously at four independent layers, each generated automatically from the graph and contract definitions.

```
Layer 1 — Transport Security      TLS, HSTS, certificate management
Layer 2 — Request Security        CORS, CSP, rate limiting, OWASP headers
Layer 3 — Application Security    Auth, RBAC, input validation, SQL injection prevention
Layer 4 — Infrastructure Security Distroless containers, non-root, network isolation
```

Every request passes through all four layers in sequence. Every layer is generated — never manually configured.

### 18.1 Layer 1 — Transport Security

**Development (https plugin):**
```
mkcert generates a local CA
Issues wildcard cert for *.APP_NAME.test
Installs CA in system trust store (browser-trusted)
All .test URLs become HTTPS automatically
No browser warnings, no certificate errors
```

**Production:**
```
Caddy / Nginx TLS config generated from graph
ACME (Let's Encrypt) auto-provisioning
DNS challenge support via Cloudflare adapter
Auto-renewal managed by deploy target
Certificate paths injected as env vars into service nodes
```

**Enforced settings:**
```
TLS 1.2 minimum (TLS 1.3 preferred)
HSTS: max-age=31536000; includeSubDomains; preload
Certificate transparency logging
OCSP stapling
```

### 18.2 Layer 2 — Request Security

Generated by the `security` plugin. Derived from graph edges — origins, paths, and methods are all known from the contract definitions.

**CORS — Exact allowlist from graph:**
```go
// Generated from graph data_flow edges — NEVER wildcards in production
// Origins = all service nodes with proxied_through edges to frontend nodes
app.Use(cors.New(cors.Config{
    AllowOrigins:     "https://vetangle.com,https://backoffice.vetangle.com",
    AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
    AllowHeaders:     "Origin,Content-Type,Authorization,X-Request-ID,X-Tenant-ID",
    AllowCredentials: true,
    MaxAge:           86400,
}))
```

**Security headers (all generated, none manual):**
```
Content-Security-Policy:   default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'
X-Content-Type-Options:    nosniff
X-Frame-Options:           DENY
Referrer-Policy:           strict-origin-when-cross-origin
Permissions-Policy:        camera=(), microphone=(), geolocation=()
Strict-Transport-Security: max-age=31536000; includeSubDomains (https plugin active)
Cross-Origin-Opener-Policy: same-origin
Cross-Origin-Embedder-Policy: require-corp
```

**Rate limiting — three dimensions:**
```go
// Per IP + per authenticated user + per tenant (if multitenancy active)
// Limits configurable per contract endpoint
app.Use(limiter.New(limiter.Config{
    Max:        100,
    Expiration: 1 * time.Minute,
    KeyGenerator: func(c *fiber.Ctx) string {
        tenantID := c.Locals("tenant_id")
        userID   := c.Locals("user_id")
        return fmt.Sprintf("%s:%s:%s", c.IP(), tenantID, userID)
    },
    LimitReached: func(c *fiber.Ctx) error {
        return c.Status(429).JSON(fiber.Map{
            "error": fiber.Map{
                "code":    "rate_limit_exceeded",
                "message": "too many requests",
                "retry_after": c.GetRespHeader("X-RateLimit-Reset"),
            },
        })
    },
}))
```

**GraphQL-specific request security:**
```
Introspection:     disabled in production (always generated)
Query depth limit: configurable, default 10 levels
Query complexity:  configurable, default 200 units
Persisted queries: optional, improves security by blocking arbitrary queries
Field-level auth:  per-operation auth from contract operations config
```

### 18.2.1 Authentication Hardening

In addition to the Layer 2 controls, the auth plugin generates these hardening measures:

**Account Enumeration Prevention:**
The login endpoint returns identical HTTP status codes, response bodies, and artificial timing delays for both "email not found" and "wrong password" states. Attackers cannot probe whether an email address is registered.

**Brute-Force Protection:**
Progressive rate limiting on all auth endpoints. After 5 failed attempts: 30-second lockout. After 10: 10-minute lockout. After 20: account flag + admin alert. Backed by Redis-side counters per IP and per account.

**Session Fixation Prevention:**
Session ID rotates on every privilege elevation (login, MFA challenge, role change).

**Password Hashing:**
Default: `argon2id` (memory=64MB, iterations=3, parallelism=2 — OWASP recommended).
Configurable alternative: `bcrypt` (cost factor 12 minimum) for teams with existing bcrypt infrastructure.

```yaml
plugins:
  - name: auth
    config:
      password_hash: argon2id    # argon2id (default) | bcrypt
      bcrypt_cost: 12            # only used when password_hash: bcrypt
```

### 18.3 Layer 3 — Application Security

**JWT Security (auth plugin):**
```
Algorithm:           RS256 (asymmetric) — default and recommended
                     HS256 available for single-service setups
Key rotation:        JWKS endpoint generated, clients fetch public keys
Token binding:       optional — bind token to client IP or device fingerprint
Expiry:              access token 15m, refresh token 7d (configurable)
Refresh rotation:    every use issues a new refresh token
Revocation:          Redis blacklist checked on every request
Clock skew:          5-second tolerance generated in verifier
```

**Password Security (auth plugin):**
```
Algorithm:    argon2id (not bcrypt — stronger against GPU attacks)
Params:       memory=64MB, iterations=3, parallelism=2 (OWASP recommended)
Salt:         32 random bytes, unique per user, stored alongside hash
Pepper:       optional application-level secret applied before hashing
```

**Input Validation (generated in DTO layer from contracts):**
```go
// Generated DTO — validation derived from contract input schema
type CreateUserInput struct {
    Name  string `validate:"required,max=100,no_html"`
    Email string `validate:"required,email,max=255"`
    Role  string `validate:"required,oneof=admin member viewer"`
}
// Validation runs BEFORE handler logic — handlers receive clean data only
// Invalid input returns 422 with structured field-level errors
// HTML tags stripped from string fields (XSS prevention)
```

**SQL Injection Prevention:**
```
All generated repository layer uses parameterized queries exclusively
Raw SQL string concatenation is never generated under any circumstances
ORM-like query builders used for dynamic queries (pgx named args)
Integration test suite includes OWASP SQL injection test cases
```

**Mass Assignment Prevention:**
```
Every DTO has an explicit allow-list of assignable fields
Request binding maps ONLY declared DTO fields
Extra fields in request body are silently ignored (never panic, never assign)
```

**Secrets Handling:**
```
Never logged — secrets filtered from structured log output
Never in responses — DTO serialization excludes secret fields
Never in URLs — secrets always in headers or request body
Always via secrets plugin in production — never .env files
${VAR} references validated resolvable before deploy
```

**CSRF Protection:**
```
SameSite=Strict cookies generated by session plugin
Double-submit cookie pattern generated for form endpoints
Custom X-CSRF-Token header required for state-changing operations
```

### 18.4 Layer 4 — Infrastructure Security

**Dockerfiles (generated per adapter — always hardened):**
```dockerfile
# Every generated Dockerfile follows this pattern:

# Stage 1: Build (full toolchain, temporary)
FROM golang:1.22-alpine AS builder
# ... build steps ...

# Stage 2: Run (distroless — no shell, no package manager, no OS vulnerabilities)
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/app /app

# Never run as root
USER nonroot:nonroot

# Read-only root filesystem
# (set in docker-compose via read_only: true)

EXPOSE 8080
ENTRYPOINT ["/app"]
```

**Docker Compose (generated from graph — always hardened):**
```yaml
# Generated docker-compose.prod.yml
services:
  api:
    image: registry.example.com/api:${VERSION}
    read_only: true                    # read-only root filesystem
    security_opt:
      - no-new-privileges:true         # prevent privilege escalation
    cap_drop:
      - ALL                            # drop all Linux capabilities
    cap_add:
      - NET_BIND_SERVICE               # only add what's needed
    networks:
      - internal                       # not exposed to external network
    tmpfs:
      - /tmp                           # writable temp dir only
```

**Network Isolation (generated from graph edges):**
```
Each service communicates only with nodes it has edges to
No service can directly reach a service it has no edge to
Infra nodes (db, redis) are on an isolated internal network
Only the proxy/gateway node is exposed to external traffic
```

**Database Security:**
```
App user:       CRUD only on app tables — never DDL in production
Migration user: DDL permissions — used only during migrations
Analytics user: SELECT only — read-only, used by Metabase/Superset
All users:      TLS enforced in DATABASE_URL in production
Connection:     pool reset between tenants (search_path cleared)
```

### 18.5 OWASP Top 10 — Full Coverage

| OWASP Category | Acthur Coverage | Layer |
|---|---|---|
| A01 Broken Access Control | RBAC plugin generates role middleware; contract `roles` field enforced on every endpoint; per-tenant data isolation | 3 |
| A02 Cryptographic Failures | TLS everywhere; RS256 JWT; argon2id passwords; secrets never in code | 1 + 3 |
| A03 Injection | Parameterized queries enforced in generated repo layer; input validation in DTOs; HTML sanitization | 3 |
| A04 Insecure Design | Contract-first development forces explicit interfaces; breaking changes blocked; no implicit trust between services | All |
| A05 Security Misconfiguration | security plugin generates correct defaults; no debug endpoints in production; introspection disabled; error messages sanitized | 2 + 4 |
| A06 Vulnerable Components | `acthur doctor` scans for known CVEs in dependencies (Phase 9); Dockerfile uses minimal distroless base | 4 |
| A07 Identification & Auth Failures | Auth plugin: MFA, rate limiting on auth endpoints, account lockout, secure session management | 3 |
| A08 Software & Data Integrity | Contract diff blocks breaking changes; deploy gate validates all contracts; signed releases via GoReleaser | 3 |
| A09 Security Logging & Monitoring | Structured logs with trace correlation; auth events logged; observability plugin generates audit trail | 3 |
| A10 SSRF | Proxy allowlist derived strictly from graph edges; services cannot call arbitrary URLs outside declared edges | 2 |

### 18.6 Security Audit Trail (audit-log plugin)

```go
// Generated by audit-log plugin when compliance mode is active
// Every state-changing operation is recorded

type AuditEvent struct {
    ID         string    // ulid
    ActorID    string    // user performing action
    TenantID   string    // tenant scope
    Action     string    // "user.created", "appointment.cancelled"
    ResourceID string    // affected entity ID
    Changes    JSON      // before/after for updates
    IP         string
    UserAgent  string
    TraceID    string    // correlates to distributed trace
    Timestamp  time.Time
}
// Immutable — insert only, never update/delete
// Separate DB user with INSERT only (no UPDATE/DELETE)
```

### 18.7 Connection Pool Safety

Pre-tuned pool prevents exhaustion, stale connections, and cross-tenant data leakage:

```
MaxConns              = min((cpu_cores × 2) + 1, per_service_budget)
MinConns              = max(2, cpu_cores ÷ 2)
MaxConnLifetime       = 1h
MaxConnLifetimeJitter = 5m    (stagger rotation, prevent thundering herd)
MaxConnIdleTime       = 30m (services) · 5m (workers)
HealthCheckPeriod     = 30s
BeforeAcquire         = ping (validate before use)
AfterRelease          = reset_search_path  (schema multitenancy — prevents data leakage)
ConnectTimeout        = 5s
```

## 19. CLI Command Reference

### 19.1 Project Commands

```bash
acthur new <name>              # interactive wizard → new project
acthur init                    # adopt existing project (non-destructive)
acthur dev                     # start all services (local context)
acthur dev --docker            # start all services (docker context)
acthur build                   # build all services for production
acthur deploy                  # deploy to configured target
acthur deploy --env <env>      # deploy to named environment
acthur deploy --dry-run        # show deploy plan without executing
acthur deploy --target <t>     # override deploy target
```

### 19.2 Graph Commands

```bash
acthur graph validate          # validate graph structure and adapter refs
acthur graph show              # print nodes and edges
acthur graph visualize         # open graph in browser (Mermaid diagram)
```

### 19.3 Contract Commands

```bash
acthur contract validate       # validate all contracts structurally
acthur contract diff <name>    # show changes vs last committed version
acthur contract list           # list all contracts with endpoint summary
```

### 19.4 Database Commands

```bash
acthur db migrate              # run pending migrations
acthur db migrate:create <n>   # create new migration file
acthur db seed                 # run seeders
acthur db reset                # drop + migrate + seed
acthur db studio               # open DB GUI in browser
```

### 19.5 Generator Commands

```bash
acthur generate from-contract <file>        # full stack from contract
acthur generate model <Name> --fields "..."  # model + migration + CRUD
acthur generate skill <name>                # custom AI skill file
acthur generate ai-context                  # AI tool context files (user selects tool)
acthur generate ci --target <provider>      # CI/CD pipeline
acthur generate docs                        # living documentation
acthur generate ide-config                  # IDE workspace config
```

### 19.6 Plugin Commands

```bash
acthur add <plugin>            # install plugin + generate its code
acthur plugin list             # list installed + available plugins
acthur plugin remove <name>    # remove plugin
```

### 19.7 Service Commands

```bash
acthur service add <infra>     # add infra node (redis, minio, etc.)
acthur service logs <name>     # stream logs from a service
acthur service restart <name>  # restart a specific service
acthur service health          # show health of all nodes
```

### 19.8 Test Commands

```bash
acthur test                    # unit tests for all services
acthur test <service>          # unit tests for one service
acthur test --integration      # integration tests (requires docker)
acthur test --contract         # contract compliance tests
acthur test --e2e              # end-to-end (starts full stack)
acthur test --load             # k6 load tests from contracts
acthur test --coverage         # with coverage report
```

### 19.9 AI Commands

```bash
acthur mcp serve               # start MCP server for AI tools
acthur agent explain "<q>"     # explain part of the system
acthur agent generate "<f>"    # generate a feature with full context
acthur agent diagnose "<p>"    # diagnose a runtime problem
acthur agent review            # review current git diff
acthur agent document "<path>" # generate documentation for a path
```

### 19.10 Secrets Commands

```bash
acthur secrets list            # list secret keys (not values)
acthur secrets rotate <key>    # rotate a secret + restart affected services
```

### 19.11 Environment Commands

```bash
acthur doctor                  # check environment health
acthur doctor --fix            # auto-fix resolvable issues
acthur flag create <name>      # create feature flag
acthur flag list               # list all feature flags
acthur flag toggle <name>      # toggle a feature flag
acthur monitor                 # open monitoring dashboard
```

---

## 20. Non-Functional Requirements

### 20.1 Performance

| Metric | Target |
|--------|--------|
| `acthur dev` cold start time | < 3 seconds to proxy ready |
| `acthur graph validate` | < 200ms for graphs up to 50 nodes |
| `acthur generate from-contract` | < 5 seconds for a 10-endpoint contract |
| CLI binary size | < 30MB (single binary, all templates embedded) |
| Memory usage at rest (`acthur dev`) | < 100MB for kernel process |
| Hot reload trigger to service ready | < adapter's own rebuild time + 500ms |

### 20.2 Reliability

- Acthur kernel crash must not take down managed services
- Service crashes must be isolated — one failing service does not stop others
- File watcher must recover from inotify limit exhaustion
- Proxy must queue requests during service restart, not drop them (with configurable timeout)

### 20.3 Compatibility

| Platform | Supported |
|----------|-----------|
| Linux x86_64 | ✓ Primary |
| Linux arm64 | ✓ Primary |
| macOS x86_64 (Intel) | ✓ Primary |
| macOS arm64 (Apple Silicon) | ✓ Primary |
| Windows x86_64 | ✓ Primary |
| Windows arm64 | Planned |

### 20.4 Security

- Acthur binary communicates with services only via localhost
- MCP server binds to localhost only by default
- No telemetry without explicit opt-in
- No phone-home at runtime
- Generated code contains zero Acthur imports
- All secrets accessed via environment variables; never logged

### 20.5 Extensibility

- New backend adapter: implement 8-method `Adapter` interface + scaffold templates
- New frontend adapter: same interface, simpler scaffold
- New plugin: implement 3-method `Plugin` interface + KernelAPI calls
- New deploy target: implement `DeployTarget` interface + manifest renderer
- All extension points documented with working examples in the repository

---

## 21. Implementation Phases

### Phase 0 — CLI Skeleton
**Deliverable:** `acthur` binary exists, command tree wired, all commands return "not implemented".

Steps:
1. Repository setup (`cmd/`, `internal/`, `templates/`, `go.mod`)
2. Cobra command tree — all commands registered, none implemented
3. Config loader — `acthur.yml` parser to typed Config struct
4. Output formatter — consistent terminal output across all commands
5. Typed error system with exit codes and user-facing hints
6. GoReleaser config — multi-platform binary builds

**Done when:** `acthur --help` prints full command tree. Binary compiles for all three platforms.

---

### Phase 1 — Graph Engine
**Deliverable:** Graph can be parsed, validated, and traversed. No processes run yet.

Steps:
1. Node and edge type definitions
2. Graph builder from `acthur.yml` GraphConfig
3. Graph validator — all structural rules enforced
4. DAG implementation — topological sort, ancestor/descendant traversal
5. Graph state manager — live node state tracking + subscription
6. `acthur graph validate` and `acthur graph show` commands wired

**Done when:** `acthur graph validate` catches bad configs. `acthur graph show` prints correct graph.

---

### Phase 2 — Adapter System
**Deliverable:** Adapters can be registered and resolved. First two adapters: `go:fiber` and `db:postgres`.

Steps:
1. Adapter interface definition
2. Adapter registry
3. `go:fiber` adapter (scaffold, dev command, generator targets)
4. `db:postgres` adapter (scaffold, dev command, env vars)
5. Graph builder extended to resolve adapter references
6. Template embedding (go:embed) infrastructure

**Done when:** `acthur graph validate` validates adapter names. `go:fiber` scaffold produces correct files.

---

### Phase 3 — Dev Runtime
**Deliverable:** `acthur dev` starts a real project, manages processes, proxies traffic.

Steps:
1. Process manager (spawn, stop, restart, supervise)
2. Log router (unified output, per-service prefix + color)
3. Dev orchestrator (startup sequence, health wait, supervision loop)
4. Health check strategies (TCP, HTTP /health, custom)
5. Dev reverse proxy (port 4000, routes from proxied_through edges)
6. File watcher + hot reload cascade (graph-aware)
7. Local .test domain (DNS strategy selection + /etc/hosts fallback)
8. `acthur dev` command fully operational

**Done when:** `acthur dev` starts Go Fiber + Postgres project, proxies correctly, restarts crashed services, hot reloads on file change.

---

### Phase 4 — Contract Engine
**Deliverable:** Contracts enforce safety on data_flow edges during dev.

Steps:
1. Contract type system
2. Native `.contract.yml` parser
3. OpenAPI / Protobuf / GraphQL importers
4. Contract registry
5. Runtime validator (request/response against contract)
6. Contract diff engine (breaking change detection)
7. `acthur contract` commands wired

**Done when:** Contract violation shows in proxy log. `acthur contract diff` correctly identifies breaking changes.

---

### Phase 5 — Plugin System
**Deliverable:** Plugins can register hooks, commands, generators. No actual plugins yet.

Steps:
1. Kernel event bus
2. Plugin interface + KernelAPI interface
3. Plugin loader (topological order, dependency resolution)
4. Plugin registry (built-in + community)
5. Test plugin that exercises all registration paths

**Done when:** Test plugin registers hook, command, and generator. All three fire at correct lifecycle points.

---

### Phase 6 — First Plugins
**Deliverable:** `migrations`, `auth`, `rbac`, `multitenancy` generate real working code.

Steps:
1. `migrations` plugin (golang-migrate integration, `acthur db` commands)
2. `auth` plugin (JWT + OAuth2 + Session + Magic Link, for go:fiber)
3. `rbac` plugin (roles/permissions, contract-derived enforcement)
4. `multitenancy` plugin (schema strategy, schema switch, provisioner)
5. Connection pool tuning (BeforeAcquire ping, AfterRelease search_path reset)

**Done when:** `acthur add auth` on a fresh `go:fiber` project produces compiling, runnable Go auth code with migrations.

---

### Phase 7 — Generator Engine
**Deliverable:** Contracts + plugins → framework-native code via `acthur generate`.

Steps:
1. Template engine (go:embed templates, idempotent merge, generated.lock)
2. Template library for `go:fiber` (handler, service, repository, dto, migration)
3. Contract-to-code pipeline (contract → template data → files)
4. `acthur generate from-contract` command
5. `acthur generate model` command
6. Test generation alongside every source file

**Done when:** `acthur generate from-contract users.contract.yml` produces compiling Go code with tests.

---

### Phase 8 — Deploy Runtime
**Deliverable:** `acthur deploy` ships a production system in one command.

Steps:
1. Deploy execution context (same graph, different strategy)
2. Dockerfile generator per adapter
3. `docker-compose.prod.yml` generator from graph
4. Coolify deploy target (API integration)
5. Pre-deploy gate (all checks, fail with clear messages)
6. `acthur deploy` command fully operational
7. Multi-environment support (`--env staging`, `--env production`)

**Done when:** `acthur deploy` takes a project to running production on a VPS in one command.

---

### Phase 9 — Ecosystem Expansion
**Deliverable:** Ecosystem broad enough for community adoption.

Steps:
1. Additional backend adapters (go:chi, go:gin, rust:axum, node:fastify)
2. Additional frontend adapters (ui:next, ui:vue, ui:svelte)
3. Additional plugins (feature-flags, admin, observability, security, https)
4. Additional deploy targets (fly.io, railway, render)
5. `acthur init` for existing projects
6. `acthur generate ai-context` + AI plugin
7. CI/CD generation plugin
8. Documentation generation plugin
9. Analytics plugins (product, web, business)
10. `acthur new` interactive wizard fully operational

---

## 22. Open Source Strategy

### 22.1 License

Acthur kernel, official plugins, and official adapters are released under the **MIT License**. Fully open source. No open-core model. No paid tiers.

### 22.2 Future Repository Strategy

```
github.com/acthur/acthur          → kernel (monorepo for kernel)
github.com/acthur/plugins         → official plugins
github.com/acthur/adapters        → official adapters
github.com/acthur/examples        → example projects
github.com/acthur/docs            → documentation site (built with Acthur + Astro)
```

Community plugins and adapters live in independent repositories and are discoverable via the plugin registry (a simple JSON manifest hosted at `registry.acthur.dev`).

### 22.3 Distribution

```
brew install acthur             # Homebrew (macOS)
scoop install acthur            # Scoop (Windows)
curl | sh                       # Universal installer
github.com/samueloshio/acthur/releases  # Direct binary download
```

Built with **GoReleaser** + GitHub Actions. New release on every tagged commit to main. Binaries published for all supported platforms.

### 22.4 Community Contribution Paths

**Adding a new backend adapter:**
Implement the `Adapter` interface (8 methods), add scaffold templates for the framework, add tests, submit PR to `adapters/` repository.

**Adding a new plugin:**
Implement the `Plugin` interface (3 methods + KernelAPI calls), add generator templates for at least one adapter, add tests, submit PR to `plugins/` repository.

**Adding a new deploy target:**
Implement the `DeployTarget` interface, add manifest renderer, test against a real target environment, submit PR.

### 22.5 Documentation

The Acthur documentation site is itself an Acthur project: `go:fiber` backend + `ui:astro` frontend. It demonstrates the system while documenting it. All documentation is co-located with the code it documents and is updated as part of the same PR.

### 22.6 Dogfooding Policy

Every Acthur feature must be demonstrable via a working example project in the `examples/` repository. Features without examples are considered incomplete.

---

## 23. Glossary

**Adapter** — A bridge between an abstract graph node and a concrete technology (framework, database, cache). Knows how to start, build, scaffold, and generate code for one specific tool. Has zero knowledge of plugins.

**Agent** — An AI-powered CLI command (`acthur agent`) that uses full graph and contract context to explain, generate, diagnose, or review within the current project.

**Contract** — A typed interface definition on a graph edge. Defines what is allowed to flow between two nodes. Enforced at dev runtime (warnings) and pre-deploy (blocking). Never executes — it is passive law.

**Contract Diff** — Analysis of changes between two versions of a contract, classified as breaking or non-breaking. Breaking changes without a version bump block deployment.

**DAG** — Directed Acyclic Graph. The internal representation of the Acthur graph. All runtime scheduling decisions are topological traversals of this structure.

**Data Flow Edge** — A graph edge indicating that one service calls another. Must have at least one contract. Enforced at the proxy layer.

**Deploy Target** — An implementation of the `DeployTarget` interface that renders the Acthur graph as a specific platform's deployment manifest (docker-compose, fly.toml, etc.).

**Depends-On Edge** — A graph edge indicating startup dependency. The target node must be healthy before the source node starts.

**Execution Context** — The strategy used to run graph nodes: Local (native processes + docker infra), Docker (fully containerized), or Cloud (deployed to target platform).

**Generator** — A code generation unit that takes a contract and adapter as input and produces framework-native source files as output. All output is owned entirely by the developer — no Acthur imports in generated code.

**Graph** — The live, in-memory directed acyclic graph that represents the entire application system. Contains all nodes, edges, and runtime state. The single source of truth for all Acthur decisions.

**Kernel** — The core Acthur binary. Manages the graph, process lifecycle, proxy, event bus, and plugin system. Analogous to an OS kernel.

**KernelAPI** — The restricted interface provided to plugins. The only mechanism through which plugins modify system behavior. Plugins cannot access any kernel internals not exposed by this interface.

**MCP Server** — Model Context Protocol server. Exposes the live Acthur graph, contracts, and execution tools to any MCP-compatible AI coding tool.

**Node** — An entity in the Acthur graph. Types: Service (something that runs), Infra (something that exists), Plugin (a cross-cutting capability), Contract (an interface definition).

**Plugin** — A capability installer for the Acthur kernel. Registers hooks, commands, generators, and schemas via the KernelAPI. Generates framework-native code into the user's project. Has zero runtime presence in the user's deployed application.

**Pre-Deploy Gate** — A sequence of validations that must all pass before any deployment proceeds: graph validity, contract validity, no unversioned breaking changes, clean builds, resolved secrets.

**Proxy** — Acthur's built-in reverse proxy. Routes all dev traffic under a single port based on `proxied_through` edges in the graph. Enforces contracts on `data_flow` edges.

**Scaffold** — The minimal set of files an adapter generates for a new node of its type. Produces a runnable but empty starting point.

**Skill** — A project-specific AI instruction file that describes how to perform a recurring task in the context of the current project's exact stack, plugins, and conventions. Generated by Acthur from the graph and plugin configuration.

**`acthur.yml`** — The graph definition file. The single source of truth for the entire project — stack, services, infrastructure, plugins, and environments. All generated artifacts (docker-compose, CI pipelines, Dockerfiles, AI context) are derived from it.

---

*Acthur — Runtime Graph Operating System with Pluggable Infrastructure Nodes*  
*Version 1.0.0 PRD — Open Source — MIT License*

**AST (Abstract Syntax Tree):** The compiled, machine-readable representation of `acthur.yml` stored as `system.json`. Committed alongside the manifest. Drives all code generation.

**Capability:** A declared application feature (auth, queue, cache, payments) that maps to one or more adapters at code-generation time.

**Capability DAG:** The directed acyclic graph of capability dependencies. Selecting `payments` auto-resolves `auth`, `database`, `queue`, and `notifications`.

**Contract (internal):** A pure Go interface or Rust trait that every capability adapter must implement. Business logic depends only on contracts — never on adapter implementations.

**Contract (external):** A `.contract.yml`, `.proto`, or `.graphql` file that declares the typed API surface between two graph nodes.

**DLQ (Dead-Letter Queue):** The queue where jobs land after exhausting their retry budget. Inspectable and replayable via the admin panel.

**Domain Module:** A generated code unit encapsulating all layers for one domain entity (model, repository, service, controller, routes, validator, events, policies).

**Generation Region:** A code block delimited by `@acthur:generated:start` and `@acthur:generated:end` markers. Acthur regenerates only this region; developer code outside it is preserved.

**Hexagonal Architecture:** An architecture style where the domain core is surrounded by ports (contracts) and adapters. Acthur generates applications in this style.

**Kernel:** The runtime orchestration core. Wires capabilities, manages service lifecycle, bootstraps middleware pipelines. Zero knowledge of specific adapter implementations.

**L1 Cache:** In-process memory cache (BigCache for Go, Moka for Rust). Sub-microsecond latency, bounded heap.

**L2 Cache:** Distributed Redis cache. Sub-millisecond latency, shared across all binary instances.

**Manifest:** The `acthur.yml` file. The single source of truth for the entire application.

**Modular Monolith:** An architecture where modules are strictly separated by domain boundaries but run in a single deployable binary. Acthur's canonical architecture.

**RLS (Row-Level Security):** PostgreSQL feature that enforces per-row access policies at the database level. Optional double-layer for shared-schema multi-tenancy.

**Single Binary:** The execution model where HTTP server, queue workers, scheduler, and pub/sub run as one OS process. Eliminates multi-process overhead.

**SSOT (Single Source of Truth):** The `acthur.yml` manifest drives all generated artifacts. Changing the manifest propagates changes across the entire stack.

**system.json:** The compiled AST artifact produced from `acthur.yml`. Version-controlled, machine-readable, and the input to all code generators.

**ULID:** Universally Unique Lexicographically Sortable Identifier. 26-character base32, time-sortable, URL-safe. Acthur's default identifier strategy.

**Snowflake ID:** A 64-bit integer identifier that encodes timestamp + datacenter ID + machine ID + sequence. Time-ordered, distributed-safe, storage-efficient (BIGINT column).


---

## 24. Microservices Architecture

### 24.1 Microservices as a Graph-Native Pattern

Acthur's graph model makes microservices the natural default rather than an architectural challenge. In conventional tooling, microservices require a service mesh, a service registry, manual environment variable wiring, and complex local development setups. In Acthur, each microservice is a node. The graph IS the service mesh.

There is no special "microservices mode." A monolith is a graph with one service node. A microservices system is a graph with many service nodes. The kernel manages both identically.

### 24.2 Service Communication Patterns

**Synchronous (HTTP / gRPC)**
```yaml
edges:
  - api-gateway → user-service    : data_flow
                  contract: contracts/users.contract.yml
                  transport: http

  - api-gateway → payment-service : data_flow
                  contract: contracts/payments.proto
                  transport: grpc
```

**Asynchronous (Events / Queue)**
```yaml
edges:
  - payment-service → notification-service : emits
                       events: [payment.completed, payment.failed]
                       transport: nats

  - notification-service → queue : subscribes_to
                            events: [payment.completed, payment.failed]
```

**Gateway Pattern**
```yaml
nodes:
  api-gateway:
    type: service
    adapter: go:fiber
    role: gateway          # aggregates downstream services
    port: 8080
```

The gateway role tells the generator to produce aggregation handlers rather than business logic handlers. The gateway knows only contracts — it never reaches directly into downstream service internals.

### 24.3 Service Discovery

Service discovery is fully automated from graph edges. Developers never manually configure service URLs.

```
Dev (local context):
  USER_SERVICE_URL=http://user-service.vetangle.test:8081
  PAYMENT_SERVICE_URL=http://payment-service.vetangle.test:8082
  Injected into each node's environment by the kernel at startup

Dev (docker context):
  USER_SERVICE_URL=http://user-service:8081
  PAYMENT_SERVICE_URL=http://payment-service:8082
  Docker network DNS used

Production (Coolify / Docker Compose):
  Docker Compose internal network DNS
  Service names derived from node IDs

Production (Kubernetes):
  user-service.default.svc.cluster.local
  Helm chart generated from graph
```

### 24.4 Distributed Tracing in Microservices

When the observability plugin is active, all inter-service HTTP and gRPC calls automatically propagate W3C TraceContext headers. The proxy layer injects trace IDs on ingress. Each service node propagates them downstream. Jaeger/Tempo visualizes the complete call tree across all services.

### 24.5 Contract Enforcement Across Services

In a microservices topology, contract enforcement is critical. Every `data_flow` edge has a contract. The Acthur proxy enforces these contracts on all inter-service traffic:

```
api-gateway calls user-service
      │
      ▼
Acthur proxy intercepts
      │
      ▼
Validates request against users.contract.yml@v1
      │
  valid? → forward to user-service
  invalid? → 422 response (strict mode) or log + forward (dev mode)
```

This means every service can trust that what it receives already complies with its contract. Defensive programming at service boundaries is eliminated.

---

## 25. GraphQL — First-Class Transport

GraphQL is not a separate mode or a plugin. It is a transport adapter at the contract layer — equal in status to HTTP REST and gRPC. A single service can simultaneously expose REST, GraphQL, and gRPC, all as contracts, all enforced by the proxy, all generating framework-native code.

### 25.1 Position in the Architecture

```
GRAPH LAYER
  Service node declares adapter (go:fiber, node:fastify, etc.)
  Nothing about the node changes for GraphQL

CONTRACT LAYER  ← GraphQL transport lives here
  transport: graphql
  Schema defined as SDL (inline or external .graphql file)
  Per-operation auth and roles declared
  Acthur validates, enforces, and diffs the schema
  Breaking changes blocked at deploy (removed type, changed field type)

ADAPTER LAYER
  go:fiber       → generates gqlgen resolver stubs + DataLoaders
  node:fastify   → generates mercurius resolvers + DataLoaders
  rust:axum      → generates async-graphql resolvers + DataLoaders
  python:fastapi → generates strawberry resolvers + DataLoaders

PLUGIN LAYER
  No special plugin required for basic GraphQL
  observability   → auto-instruments every resolver field with a span
  auth plugin     → per-operation JWT enforcement
  rbac plugin     → per-operation role enforcement
  security plugin → introspection disabled in prod, depth + complexity limits
  multitenancy    → tenant context injected into resolver context
```

### 25.2 Contract Definition — Three Formats

#### Format 1 — Native Contract (SDL inline)

```yaml
# contracts/veterinary.contract.yml
contract: veterinary
version: "1"
transport: graphql

schema: |
  type Vet {
    id: ID!
    name: String!
    specialization: String!
    available: Boolean!
    appointments: [Appointment!]!
  }

  type Appointment {
    id: ID!
    vet: Vet!
    petName: String!
    startTime: String!
    status: AppointmentStatus!
    notes: String
  }

  enum AppointmentStatus {
    PENDING
    CONFIRMED
    COMPLETED
    CANCELLED
    NO_SHOW
  }

  type Query {
    vet(id: ID!): Vet
    vets(available: Boolean, specialization: String): [Vet!]!
    appointment(id: ID!): Appointment
    appointments(vetId: ID, status: AppointmentStatus, page: Int): AppointmentPage!
  }

  type Mutation {
    bookAppointment(input: BookAppointmentInput!): Appointment!
    cancelAppointment(id: ID!, reason: String): Appointment!
    updateVetAvailability(id: ID!, available: Boolean!): Vet!
  }

  type Subscription {
    appointmentStatusChanged(vetId: ID!): Appointment!
    vetAvailabilityChanged: Vet!
  }

  type AppointmentPage {
    data: [Appointment!]!
    total: Int!
    page: Int!
  }

  input BookAppointmentInput {
    vetId:     ID!
    petName:   String!
    startTime: String!
    notes:     String
  }

# Per-operation auth and roles — enforced by auth + rbac plugins
operations:
  - operation: vets
    type: query
    auth: none                  # public query — no token required

  - operation: vet
    type: query
    auth: none

  - operation: appointment
    type: query
    auth: required

  - operation: appointments
    type: query
    auth: required
    roles: [vet, admin, support]

  - operation: bookAppointment
    type: mutation
    auth: required
    roles: [user, agent]

  - operation: cancelAppointment
    type: mutation
    auth: required
    roles: [user, agent, vet, admin]

  - operation: updateVetAvailability
    type: mutation
    auth: required
    roles: [vet, admin]

  - operation: appointmentStatusChanged
    type: subscription
    auth: required

  - operation: vetAvailabilityChanged
    type: subscription
    auth: none
```

#### Format 2 — External SDL file

```yaml
contract: veterinary
version: "1"
transport: graphql
schema_file: contracts/veterinary.graphql   # SDL in separate file
operations:
  - operation: bookAppointment
    type: mutation
    auth: required
    roles: [user, agent]
```

#### Format 3 — Direct .graphql file in edge

```yaml
edges:
  - from: web
    to: api
    type: data_flow
    contracts: [contracts/veterinary.graphql]   # transport auto-detected from extension
```

### 25.3 Code Generation Per Adapter

#### `go:fiber` → gqlgen

```bash
acthur generate from-contract contracts/veterinary.contract.yml
```

Generated files:
```
internal/veterinary/
├── schema.graphqls              ← SDL from contract (never edit manually)
├── resolver.go                  ← resolver struct, dependencies injected
├── vet.resolvers.go             ← Query + Mutation stubs → developer fills logic
├── subscription.resolvers.go    ← Subscription stubs (WebSocket backed)
├── models_gen.go                ← Go types (never edit — regenerated from schema)
├── generated.go                 ← gqlgen wiring (never edit)
├── dataloader.go                ← N+1 prevention (generated per relationship)
└── resolver_test.go             ← resolver tests with httptest GraphQL client
```

Resolver stub (developer fills the body):
```go
// internal/veterinary/vet.resolvers.go — Generated by Acthur

// Query.vets — auth: none (public)
func (r *queryResolver) Vets(ctx context.Context,
    available *bool, specialization *string,
) ([]*model.Vet, error) {
    // tenant := tenant.FromContext(ctx)  ← injected if multitenancy active
    // db    := r.DB.ForTenant(tenant)
    // TODO: implement
    return nil, nil
}

// Mutation.bookAppointment — auth: required, roles: [user, agent]
// JWT validated + RBAC enforced BEFORE this resolver is called
func (r *mutationResolver) BookAppointment(ctx context.Context,
    input model.BookAppointmentInput,
) (*model.Appointment, error) {
    user := auth.FromContext(ctx)  // injected by auth middleware
    // TODO: implement
    return nil, nil
}

// Subscription.appointmentStatusChanged — WebSocket backed
func (r *subscriptionResolver) AppointmentStatusChanged(ctx context.Context,
    vetID string,
) (<-chan *model.Appointment, error) {
    ch := make(chan *model.Appointment, 1)
    // TODO: implement pub/sub
    return ch, nil
}
```

Route registration (generated in `internal/server/routes.go`):
```go
// GraphQL endpoint registered alongside REST routes
srv := handler.NewDefaultServer(
    veterinary.NewExecutableSchema(veterinary.Config{
        Resolvers: &veterinary.Resolver{DB: db, Cache: cache},
    }),
)
// Security middleware from security plugin
srv.SetIntrospectionFunc(func(ctx context.Context) bool {
    return cfg.IsDevelopment()  // disabled in production
})
srv.Use(extension.FixedComplexityLimit(200))

app.Post("/graphql", adaptor.HTTPHandler(srv))
if cfg.IsDevelopment() {
    app.Get("/graphql", adaptor.HTTPHandler(
        playground.Handler("Acthur", "/graphql"),
    ))
}
```

#### `node:fastify` → Mercurius

```
src/graphql/
├── schema.graphql           ← SDL from contract
├── resolvers/
│   ├── vet.resolvers.ts     ← Query + Mutation stubs
│   ├── subscription.ts      ← Subscription via WebSocket
│   └── index.ts             ← resolver map
├── types.ts                 ← generated TypeScript types (never edit)
├── dataloaders.ts           ← DataLoader for N+1 prevention
└── plugin.ts                ← Mercurius plugin registration
```

```typescript
// src/graphql/resolvers/vet.resolvers.ts — Generated by Acthur
import type { Resolvers } from '../types'

export const vetResolvers: Resolvers = {
  Query: {
    vets: async (_, { available, specialization }, ctx) => {
      // ctx.tenant injected by multitenancy middleware
      // ctx.user injected by auth middleware
      // TODO: implement
      return []
    },
  },
  Mutation: {
    bookAppointment: async (_, { input }, ctx) => {
      // auth: required enforced before this resolver is called
      // TODO: implement
      throw new Error('not implemented')
    },
  },
  Subscription: {
    appointmentStatusChanged: {
      subscribe: async (_, { vetId }, ctx) => {
        // WebSocket subscription
        // TODO: implement
      },
    },
  },
}
```

#### `rust:axum` → async-graphql

```
src/graphql/
├── schema.rs                ← schema wiring
├── query.rs                 ← Query resolver
├── mutation.rs              ← Mutation resolver
├── subscription.rs          ← Subscription resolver (WebSocket)
├── types/
│   ├── vet.rs               ← Vet GraphQL type
│   └── appointment.rs       ← Appointment GraphQL type
└── dataloader.rs            ← DataLoader for N+1
```

```rust
// src/graphql/query.rs — Generated by Acthur
use async_graphql::{Context, Object, Result};

pub struct QueryRoot;

#[Object]
impl QueryRoot {
    /// List vets — auth: none
    async fn vets(
        &self,
        ctx: &Context<'_>,
        available: Option<bool>,
        specialization: Option<String>,
    ) -> Result<Vec<Vet>> {
        let state = ctx.data::<AppState>()?;
        // TODO: implement
        Ok(vec![])
    }

    /// Get single vet
    async fn vet(&self, ctx: &Context<'_>, id: String) -> Result<Option<Vet>> {
        // TODO: implement
        Ok(None)
    }
}
```

#### `python:fastapi` → Strawberry

```python
# app/graphql/query.py — Generated by Acthur
import strawberry
from typing import Optional, List
from app.graphql.types import Vet, AppointmentPage

@strawberry.type
class Query:
    @strawberry.field
    async def vets(
        self,
        info: strawberry.types.Info,
        available: Optional[bool] = None,
        specialization: Optional[str] = None,
    ) -> List[Vet]:
        # info.context["tenant"] injected by middleware
        # TODO: implement
        return []

    @strawberry.field
    async def vet(self, info: strawberry.types.Info, id: str) -> Optional[Vet]:
        # TODO: implement
        return None
```

### 25.4 Multiple Transports — REST + GraphQL + gRPC on One Service

```yaml
edges:
  # Mobile clients use REST
  - from: mobile-app
    to: api
    type: data_flow
    contracts: [contracts/users.contract.yml]
    transport: http

  # Web frontend uses GraphQL
  - from: web
    to: api
    type: data_flow
    contracts: [contracts/veterinary.contract.yml]
    transport: graphql

  # Internal analytics service uses gRPC
  - from: analytics-service
    to: api
    type: data_flow
    contracts: [contracts/analytics.proto]
    transport: grpc
```

Acthur generates all three handler sets for the `api` service. They coexist in the same service process. Routes:
```
POST /api/v1/users        → REST handler
POST /graphql             → GraphQL handler (all operations)
GET  /graphql             → GraphQL playground (dev only)
:50051                    → gRPC server (analytics)
```

### 25.5 N+1 Prevention — DataLoader Auto-Generated

When the contract schema declares relationships (Vet has Appointments), Acthur generates DataLoaders automatically:

```go
// internal/veterinary/dataloader.go — Generated by Acthur
// Detected: Vet.appointments resolves []Appointment → batching needed

type Loaders struct {
    AppointmentsByVetID *dataloader.Loader[string, []*model.Appointment]
}

func NewLoaders(db *pgxpool.Pool, tenant string) *Loaders {
    return &Loaders{
        AppointmentsByVetID: dataloader.NewBatchedLoader(
            func(ctx context.Context, vetIDs []string) ([][]*model.Appointment, []error) {
                // ONE query for ALL vet IDs in this batch — no N+1
                rows, _ := db.Query(ctx,
                    `SELECT * FROM appointments
                     WHERE vet_id = ANY($1)
                     AND tenant_id = $2`,
                    vetIDs, tenant,
                )
                // Group results by vet_id and return in order
                // ...
            },
        ),
    }
}
```

### 25.6 Schema Introspection Security

```go
// Generated by security plugin for all GraphQL endpoints
srv.SetIntrospectionFunc(func(ctx context.Context) bool {
    // Disabled in production — prevents schema discovery by attackers
    // Enabled in dev/staging for playground and tooling
    return cfg.IsDevelopment() || cfg.IsStaging()
})
```

### 25.7 Query Depth and Complexity Limits

```go
// Generated by security plugin
srv.Use(extension.FixedComplexityLimit(200))  // max 200 complexity units
// Depth limit applied via custom rule
srv.Use(queryDepthLimit(10))  // max 10 levels of nesting
```

### 25.8 GraphQL Contract Diff — Breaking Change Detection

```bash
acthur contract diff veterinary

  BREAKING:
  ✗  Type Vet: field 'licenseNumber' removed
     Clients expecting this field will receive null or error
     Fix: add to v2 contract, run both v1 and v2 simultaneously

  BREAKING:
  ✗  Mutation bookAppointment: arg 'petId' type changed String! → ID!
     Fix: bump version to v2

  NON-BREAKING:
  ✓  Type Vet: optional field 'profilePhoto: String' added
  ✓  Query vetsNearLocation added
  ✓  Enum AppointmentStatus: value 'NO_SHOW' added

  → 2 breaking changes found
  → Deploy blocked until version bumped or changes reverted
```

Breaking changes detected for GraphQL:
```
BREAKING:
  Type removed
  Field removed from type
  Field type changed (String → Int, etc.)
  Field made non-nullable (String → String!)
  Argument removed from field
  Argument type changed
  Enum value removed
  Directive removed
  Input field made required

NON-BREAKING:
  New type added
  New optional field added
  New enum value added
  New query/mutation/subscription added
  Optional argument added
  Field deprecated (adds @deprecated — does not break clients)
```

### 25.9 WebSocket Support for Subscriptions

The dev proxy handles WebSocket upgrades for GraphQL subscriptions transparently:

```yaml
# acthur.yml — WebSocket pass-through enabled per node
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
      websocket: true    # proxy passes WS upgrade through to this node
```

In development, GraphQL subscriptions work over the unified `:4000` proxy port alongside all REST and regular GraphQL queries. In production, Caddy/Nginx is configured with WebSocket proxy support derived from the graph.



---

## 26. gRPC and Protocol Buffers

### 26.1 gRPC as a Contract Transport

gRPC is a first-class transport in Acthur. Proto files are valid contract inputs and are compiled into the internal Contract representation. A service can expose HTTP REST, GraphQL, and gRPC simultaneously.

```yaml
# Proto file used directly as a contract on a data_flow edge
edges:
  - from: payment-service
    to: user-service
    type: data_flow
    contracts: [contracts/users.proto]
    transport: grpc

  - from: analytics-service
    to: api
    type: data_flow
    contracts: [contracts/analytics.proto]
    transport: grpc
```

### 26.2 Code Generation Per Adapter

```bash
# From a proto file
acthur generate from-contract contracts/users.proto --target go:grpc-server
acthur generate from-contract contracts/users.proto --target go:grpc-client
acthur generate from-contract contracts/users.proto --target rust:grpc-server
acthur generate from-contract contracts/users.proto --target node:grpc-client
```

Generated output (go:fiber + gRPC server):
```
internal/users/
├── users.pb.go              ← protoc generated (never edit)
├── users_grpc.pb.go         ← protoc generated (never edit)
├── server.go                ← gRPC server implementation stubs
├── client.go                ← typed gRPC client for callers
├── interceptors.go          ← auth + tenant + logging + tracing interceptors
└── server_test.go           ← gRPC server tests
```

gRPC server stub:
```go
// internal/users/server.go — Generated by Acthur
package users

import (
    "context"
    pb "github.com/yourorg/app/internal/users/gen"
)

type UserServiceServer struct {
    pb.UnimplementedUserServiceServer
    db    *pgxpool.Pool
    cache *redis.Client
}

func (s *UserServiceServer) GetUser(ctx context.Context,
    req *pb.GetUserRequest,
) (*pb.User, error) {
    // auth interceptor validated JWT before this is called
    // tenant interceptor set schema search_path
    // trace interceptor started a child span
    // TODO: implement
    return nil, status.Error(codes.Unimplemented, "not implemented")
}
```

### 26.3 Bidirectional Streaming

```yaml
# contracts/events.proto declares streaming endpoints
# Acthur generates appropriate streaming stubs

endpoints:
  - id: watch_appointments
    transport: grpc
    streaming: server              # server_stream | client_stream | bidirectional
    input:
      vet_id: string
    output:
      appointment: $Appointment
```

### 26.4 gRPC Health Protocol

All generated gRPC servers implement the standard gRPC Health Checking Protocol (`grpc.health.v1`). Acthur's health checker automatically uses the gRPC health check for nodes with `transport: grpc`.

### 26.5 Proto-to-Contract Compilation

Acthur can compile native contract files to proto format for interop:

```bash
acthur contract compile users.contract.yml --target proto
→ contracts/users.pb.proto   (generated, committed alongside .contract.yml)
```

### 26.6 Required Tools

`acthur doctor` checks for and auto-installs gRPC toolchain when proto contracts are present:

```
protoc                  → protocol buffer compiler
protoc-gen-go           → Go code generator (go install)
protoc-gen-go-grpc      → Go gRPC generator (go install)
tonic-build (Rust)      → added to build.rs automatically
@grpc/grpc-js (Node)    → added to package.json automatically
grpcio (Python)         → added to requirements.txt automatically
```

---

## 27. Traceability — Distributed Tracing

### 27.1 What Traceability Means in Acthur

Traceability means every request can be followed end-to-end across all services, all languages, and all runtimes — from the moment it enters the proxy to the moment the response leaves. A single trace ID ties together Go, Rust, Python, and Node.js services.

### 27.2 The Trace Flow

```
User request hits proxy
       │
       ▼
Proxy injects W3C TraceContext headers automatically
  traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
  tracestate:  acthur=vetangle
       │
       ▼
Go Fiber handler receives trace context
OTel middleware continues the span as "api: POST /api/v1/appointments"
       │
       ├──▶ DB query → child span: "db.query SELECT appointments"
       │                            duration, row count, sanitized SQL
       │
       ├──▶ Redis call → child span: "cache.get vet:availability"
       │
       ▼
api calls payments via gRPC (data_flow edge)
gRPC interceptor propagates traceparent automatically
       │
       ▼
Rust Axum service receives trace context
Creates child span: "payments: charge"
       │
       ├──▶ DB query → child span: "db.query INSERT payments"
       │
       ▼
payments emits event to queue
NATS message includes trace context in headers
       │
       ▼
Node.js notification service receives message
Continues trace: "notifications: send email"
       │
       ▼
All spans collected in Jaeger/Tempo
One trace ID → complete picture across Go + Rust + Node.js
```

### 27.3 Auto-Instrumented Layers

When the observability plugin is active, every generated service is instrumented at these layers automatically. Developers never manually add trace calls to business logic.

```
LAYER                     SPAN NAME FORMAT                   ATTRIBUTES
─────────────────────────────────────────────────────────────────────────────
HTTP handler              "METHOD /path"                     status, duration, user_id, tenant_id
DB query (pgx/sqlx)       "db.query"                        operation, table, row_count, duration
Redis operation           "cache.get|set|del"                key_pattern, hit/miss, duration
gRPC call (outbound)      "grpc.client/ServiceName/Method"   status, duration
gRPC handler (inbound)    "grpc.server/ServiceName/Method"   status, duration
Queue publish             "queue.publish topic"              topic, message_id
Queue consume             "queue.consume topic"              topic, message_id, processing_time
GraphQL resolver          "graphql.resolver TypeName.field"  complexity, depth
Background job            "job.execute JobName"              job_id, queue, duration
HTTP outbound call        "http.client METHOD url"           status, duration
```

### 27.4 Trace Context in Logs

Every structured log line includes the current trace ID and span ID, enabling instant correlation between logs and traces:

```go
// Generated logger middleware (go:fiber)
logger.Info("appointment booked",
    slog.String("trace_id",   span.SpanContext().TraceID().String()),
    slog.String("span_id",    span.SpanContext().SpanID().String()),
    slog.String("user_id",    user.ID),
    slog.String("tenant_id",  tenant.ID),
    slog.String("appointment_id", appointment.ID),
)
// In Grafana: click trace_id → jump directly to Jaeger trace
```

### 27.5 Trace Sampling

```yaml
# acthur.yml — observability plugin config
plugins:
  - name: observability
    config:
      tracing:
        sampler: parentbased_traceidratio  # or: always_on | always_off | traceidratio
        sample_rate: 0.1                   # 10% sampling in production
        always_sample:                     # always trace these regardless of rate
          - paths: ["/api/v1/payments/*"]  # payment endpoints always traced
          - status: [500, 503]             # all errors always traced
```

### 27.6 Trace Backends

```
Jaeger (default, self-hosted)
  → OTLP HTTP/gRPC export
  → Full UI: search by trace ID, service, operation, duration, tags
  → dev_url: traces.APP_NAME.test

Tempo (Grafana stack)
  → OTLP export
  → Integrated with Grafana dashboards
  → Better at scale than Jaeger

OTLP endpoint (generic)
  → Send to any OTel-compatible backend
  → Datadog, Honeycomb, Lightstep, etc.
```

### 27.7 Cross-Language Propagation Matrix

| Caller | Callee | Propagation Method |
|--------|--------|--------------------|
| Go Fiber | Go Fiber | OTel HTTP middleware + `otelhttp` |
| Go Fiber | Rust Axum | W3C TraceContext headers, Tower middleware |
| Go Fiber | Python FastAPI | W3C TraceContext headers, `opentelemetry-instrumentation-fastapi` |
| Go Fiber | Node Fastify | W3C TraceContext headers, `@opentelemetry/instrumentation-fastify` |
| Any | Any (gRPC) | gRPC metadata propagation, interceptors |
| Any | Any (Queue) | NATS/Redis message headers |
| Proxy | First service | Injected by Acthur proxy on ingress |

### 27.8 Audit Trail (Traceability for Compliance)

When the `audit-log` plugin is active, every state-changing operation produces an immutable audit record correlated to the distributed trace:

```go
// Generated by audit-log plugin
type AuditEvent struct {
    ID         string    // ulid — immutable record identifier
    ActorID    string    // user performing the action
    TenantID   string    // tenant scope
    Action     string    // "appointment.booked", "user.role_changed"
    ResourceID string    // affected entity ID
    Before     JSON      // state before the change
    After      JSON      // state after the change
    TraceID    string    // links to distributed trace for full context
    IP         string
    UserAgent  string
    Timestamp  time.Time
}
// Storage: separate audit schema, INSERT only
// DB user: INSERT permissions only — no UPDATE or DELETE
// Retention: configurable, default 7 years (compliance)
```

---

## 28. Observability — Three Pillars

### 28.1 The Three-Pillar Architecture

Acthur's observability plugin implements the full OpenTelemetry three-pillar model. All three signals are correlated by trace ID.

```
TRACES  → OpenTelemetry SDK → Jaeger or Tempo
METRICS → OpenTelemetry SDK → Prometheus → Grafana
LOGS    → Structured JSON   → Promtail   → Loki → Grafana

Correlation: trace_id appears in logs, metrics exemplars, and trace spans
→ Click a spike in Grafana → see the traces during that spike
→ Click a log line → jump to its distributed trace
→ Click a trace span → see the logs from that span
```

### 28.2 Metrics Catalogue

Every generated service emits the following metrics automatically. No manual instrumentation required.

**HTTP Metrics:**
```
http_requests_total{method, path, status, service, tenant}          counter
http_request_duration_seconds{method, path, service}                histogram (p50/p95/p99)
http_request_size_bytes{method, path, service}                      histogram
http_response_size_bytes{method, path, service}                     histogram
http_active_requests{service}                                       gauge
```

**Database Metrics:**
```
db_query_duration_seconds{operation, table, service}                histogram
db_queries_total{operation, table, status, service}                 counter
db_connections_active{service}                                      gauge
db_connections_idle{service}                                        gauge
db_connections_total{service}                                       gauge
db_pool_acquire_duration_seconds{service}                           histogram
db_pool_acquire_total{status, service}       status=success|timeout counter
```

**Cache Metrics:**
```
cache_operations_total{operation, result, service}  result=hit|miss counter
cache_operation_duration_seconds{operation, service}                histogram
cache_evictions_total{service}                                      counter
```

**Queue Metrics:**
```
queue_messages_published_total{topic, service}                      counter
queue_messages_consumed_total{topic, status, service}               counter
queue_consumer_lag{topic, service}                                  gauge
queue_processing_duration_seconds{topic, service}                   histogram
```

**Runtime Metrics (per language):**
```
Go:
  go_goroutines                                                      gauge
  go_gc_duration_seconds                                             summary
  go_memstats_alloc_bytes                                            gauge
  go_memstats_heap_inuse_bytes                                       gauge

Rust:
  process_cpu_seconds_total                                          counter
  process_resident_memory_bytes                                      gauge
  process_open_fds                                                   gauge

Node.js:
  nodejs_eventloop_lag_seconds                                       histogram
  nodejs_heap_size_bytes{space}                                      gauge
  nodejs_active_handles_total                                        gauge
  nodejs_gc_duration_seconds{kind}                                   histogram

Python:
  python_gc_collections_total{generation}                            counter
  process_resident_memory_bytes                                      gauge
  process_cpu_seconds_total                                          counter
```

**Connection Pool Metrics (critical for production health):**
```
db_pool_connections_acquired{service}                               gauge
db_pool_connections_idle{service}                                   gauge
db_pool_empty_acquire_total{service}   ← requests that waited      counter
db_pool_canceled_acquire_total{service} ← requests that timed out  counter

Alert: pool_utilization > 80% → WARNING: approaching exhaustion
Alert: pool_utilization > 95% → CRITICAL: pool exhausted
Alert: canceled_acquire rising → CRITICAL: user-visible timeouts
```

### 28.3 Pre-Built Grafana Dashboards

Generated per adapter — installed automatically into the Grafana infra node:

**Service Dashboard (all adapters):**
```
Row 1: Request rate · Error rate · P95 latency · Active requests
Row 2: Request duration histogram · Status code breakdown
Row 3: DB query rate · DB P95 latency · Pool utilization
Row 4: Cache hit rate · Cache operation latency
Row 5: Queue lag · Queue throughput
```

**Go-specific Dashboard:**
```
Goroutines over time · GC pause duration · Heap allocation rate
Memory in use · Open file descriptors
```

**Node.js-specific Dashboard:**
```
Event loop lag · Heap used vs total · GC frequency
Active handles · External memory
```

**Infra Dashboard:**
```
PostgreSQL: connections, query rate, cache hit ratio, replication lag
Redis: ops/sec, memory usage, keyspace hits, connected clients
NATS: messages in/out, subscriptions, slow consumers
```

### 28.4 Alert Rules — Generated Prometheus Rules

```yaml
# .acthur/monitoring/alerts.yml — generated by observability plugin
groups:
  - name: acthur.slo
    interval: 30s
    rules:
      # Availability SLO: 99.9% of requests succeed
      - alert: HighErrorRate
        expr: |
          rate(http_requests_total{status=~"5.."}[5m])
          / rate(http_requests_total[5m]) > 0.001
        for: 2m
        severity: critical
        annotations:
          summary: "Error rate above 0.1% SLO threshold"

      # Latency SLO: P95 < 1s
      - alert: HighLatency
        expr: |
          histogram_quantile(0.95,
            rate(http_request_duration_seconds_bucket[5m])) > 1.0
        for: 5m
        severity: warning

      # DB pool exhaustion
      - alert: DBPoolExhaustion
        expr: db_pool_utilization > 0.95
        for: 1m
        severity: critical
        annotations:
          summary: "DB connection pool above 95% — user requests will time out"

      - alert: DBPoolPressure
        expr: db_pool_utilization > 0.80
        for: 5m
        severity: warning

      # Queue backlog
      - alert: QueueConsumerLag
        expr: queue_consumer_lag > 1000
        for: 5m
        severity: warning
        annotations:
          summary: "Queue consumer is falling behind"

      # Service down
      - alert: ServiceDown
        expr: up == 0
        for: 1m
        severity: critical

      # Node.js event loop
      - alert: EventLoopLag
        expr: nodejs_eventloop_lag_seconds > 0.1
        for: 2m
        severity: warning
```

### 28.5 Log Structure

All generated services produce structured JSON logs. In dev, a human-readable format is used instead.

```json
{
  "time":       "2025-05-21T10:23:45.123Z",
  "level":      "info",
  "service":    "api",
  "tenant_id":  "01ARZ3NDEK...",
  "user_id":    "01ARZ4MFEK...",
  "trace_id":   "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id":    "00f067aa0ba902b7",
  "method":     "POST",
  "path":       "/api/v1/appointments",
  "status":     201,
  "latency_ms": 23,
  "message":    "appointment booked"
}
```

Log levels by environment:
```
development:  DEBUG and above (human-readable, colorized)
staging:      INFO and above (JSON)
production:   INFO and above (JSON, shipped to Loki)
```

Sensitive fields automatically redacted from logs:
```
password, token, secret, authorization, cookie,
credit_card, ssn, api_key, private_key
```

### 28.6 Health Endpoint Contract

Every generated service exposes `GET /health` that returns a structured response Acthur's health checker and load balancers parse:

```json
{
  "status":          "healthy",
  "service":         "api",
  "version":         "1.2.3",
  "uptime_seconds":  3600,
  "checks": {
    "database": { "status": "healthy", "latency_ms": 2 },
    "redis":    { "status": "healthy", "latency_ms": 1 },
    "storage":  { "status": "healthy", "latency_ms": 5 }
  }
}
```

Returns `200 OK` when all checks pass, `503 Service Unavailable` when any check fails. Acthur's graph state manager updates node state based on this response.

### 28.7 Dev Observability URLs

When the observability plugin is active, these URLs are available in `acthur dev`:

```
http://traces.APP_NAME.test    → Jaeger UI (distributed traces)
http://metrics.APP_NAME.test   → Grafana (dashboards and alerts)
http://logs.APP_NAME.test      → Grafana Loki (log search)
```

---

## 29. Multiple Backend Configurations

### 29.1 Supported Combinations

Any combination of backend adapters is valid. Examples from simple to complex:

**Single backend (monolith):**
```yaml
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber    # one backend, handles everything
```

**Two backends (Go + Rust):**
```yaml
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber         # business logic API

    payments:
      type: service
      adapter: rust:axum        # performance-critical payment processing
```

**Three backends (Go + Rust + Python):**
```yaml
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber         # core API

    payments:
      type: service
      adapter: rust:axum        # payments (performance)

    ml-inference:
      type: service
      adapter: python:fastapi   # ML model serving
```

**Four backends (Go + Rust + Python + Node.js):**
```yaml
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber

    payments:
      type: service
      adapter: rust:axum

    ml-inference:
      type: service
      adapter: python:fastapi

    notifications:
      type: service
      adapter: node:fastify     # email/SMS/push (rich JS ecosystem)
```

**Gateway + microservices (different runtimes):**
```yaml
graph:
  nodes:
    gateway:
      type: service
      adapter: go:fiber
      role: gateway

    user-service:
      type: service
      adapter: go:fiber

    payment-service:
      type: service
      adapter: rust:axum

    inventory-service:
      type: service
      adapter: python:fastapi

    search-service:
      type: service
      adapter: bun:elysia       # search index (fast JS runtime)

  edges:
    - from: gateway  to: user-service      type: data_flow
                     contracts: [contracts/users.contract.yml]
    - from: gateway  to: payment-service   type: data_flow
                     contracts: [contracts/payments.proto]
                     transport: grpc
    - from: gateway  to: inventory-service type: data_flow
                     contracts: [contracts/inventory.contract.yml]
    - from: gateway  to: search-service    type: data_flow
                     contracts: [contracts/search.contract.yml]
```

### 29.2 What `acthur dev` Does With Multiple Backends

```
1. acthur doctor checks all required toolchains
   → go ≥1.21, air (for go:fiber)
   → rustup, cargo-watch (for rust:axum)
   → python ≥3.11, uvicorn (for python:fastapi)
   → node ≥20, pnpm (for node:fastify)
   → All checked, only missing tools reported

2. Topological sort determines startup order
   → infra first (db, cache, queue)
   → then services in dependency order

3. Each service starts with its adapter's dev command
   → api:         air -c .air.toml
   → payments:    cargo watch -x run
   → ml-inference: uvicorn app.main:app --reload
   → notifications: pnpm tsx watch src/server.ts

4. Each service supervised independently
   → go service crash does not affect rust service
   → each service has its own restart counter and backoff

5. All services proxied through unified :4000
   → /api/*      → go:fiber :8080
   → /payments/* → rust:axum :8081
   → /ml/*       → python:fastapi :8082
   → /notify/*   → node:fastify :8083

6. OTel trace context propagated across all language boundaries
   → W3C TraceContext headers injected by each adapter
   → One Jaeger trace shows the full cross-language call tree
```

### 29.3 Contracts Across Language Boundaries

The contract is the universal language. The type system is language-agnostic:

```yaml
# contracts/payments.contract.yml
contract: payments
version: "1"
transport: http

types:
  Payment:
    id:          ulid
    amount:      decimal
    currency:    string
    status:      enum(pending,completed,failed,refunded)
    created_at:  timestamp
```

Acthur generates:
```
For go:fiber caller:
  type Payment struct {
      ID        string          `json:"id"`
      Amount    decimal.Decimal `json:"amount"`
      Currency  string          `json:"currency"`
      Status    string          `json:"status"`
      CreatedAt time.Time       `json:"created_at"`
  }

For rust:axum server:
  #[derive(Serialize, Deserialize)]
  pub struct Payment {
      pub id:         String,
      pub amount:     Decimal,
      pub currency:   String,
      pub status:     PaymentStatus,
      pub created_at: DateTime<Utc>,
  }

For python:fastapi caller:
  class Payment(BaseModel):
      id: str
      amount: Decimal
      currency: str
      status: PaymentStatus
      created_at: datetime

For node:fastify caller:
  export interface Payment {
      id: string
      amount: number
      currency: string
      status: 'pending' | 'completed' | 'failed' | 'refunded'
      created_at: string
  }
```

The same contract generates the correct types for every language. Breaking changes in the contract (removing a field, changing a type) are caught before deploy regardless of which language broke the contract.

### 29.4 Service Discovery Across Runtimes

Service discovery is automatic. Acthur injects the correct URL into each service's environment based on `data_flow` edges:

```
go:fiber api node has edge to rust:axum payments node
→ Acthur injects PAYMENTS_URL=http://localhost:8081 into api's env

python:fastapi ml-inference has edge to db:postgres
→ Acthur injects DATABASE_URL=postgres://... into ml-inference's env

No manual service URL configuration. Ever.
```

In production (Docker Compose):
```
PAYMENTS_URL=http://payments:8081    (Docker internal DNS)
DATABASE_URL=postgres://db:5432/app  (Docker internal DNS)
```

In production (Kubernetes):
```
PAYMENTS_URL=http://payments.default.svc.cluster.local:8081
DATABASE_URL=postgres://db.default.svc.cluster.local:5432/app
```

All derived from the graph. Never manually written.

---

## 30. Testing Infrastructure

### 30.1 Test Generation Philosophy

Tests are not optional in Acthur. Every generator produces test files alongside source files. A generated `handler.go` without a `handler_test.go` is an incomplete generation. Tests use the same language as the service — no testing framework is imposed.

### 30.2 Test Categories

**Unit Tests**
```
Pure logic, no external dependencies
Repository layer mocked with generated mock interface
Run in milliseconds — suitable for every file save
Coverage: all handler paths, all service methods, all validator rules
```

**Integration Tests**
```
Real database via testcontainers (Postgres/MySQL/SQLite container)
Real Redis via testcontainers
Migrations applied fresh for each test suite
Tests the full service layer against real infrastructure
Command: acthur test --integration
```

**Contract Tests**
```
Verify service responses comply with declared contract
Run against a live service (started by Acthur for the test run)
Catch drift between contract definition and implementation
Run on every contract file change (hot reload cascade)
Command: acthur test --contract
Also runs automatically in pre-deploy gate
```

**End-to-End Tests**
```
Full stack started via acthur dev --docker
Playwright for browser-based UI testing (ui:* nodes)
API E2E via generated test scripts
Command: acthur test --e2e
```

**Load Tests**
```
k6 scripts generated from contract endpoint definitions
Load profiles: smoke (1 VU) · average (10 VU) · stress (50 VU) · spike
Results surfaced in Grafana (observability plugin)
Command: acthur test --load
```

**Cross-Service Contract Tests (polyglot)**
```
For multi-backend projects: verifies that each service honors
its declared contracts regardless of language
A Go:Fiber server must satisfy the same contract as a Rust:Axum server
Language is irrelevant — the contract is the spec
```

### 30.3 Test Utilities Generated Per Adapter

```go
// internal/testutil/testutil.go — go:fiber generated test helpers

// SetupTestDB — testcontainers Postgres, migrations applied, return pool
func SetupTestDB(t *testing.T) *pgxpool.Pool

// SetupTestRedis — testcontainers Redis, return client
func SetupTestRedis(t *testing.T) *redis.Client

// NewTestApp — Fiber app with all middleware, return httptest client
func NewTestApp(t *testing.T, db *pgxpool.Pool) *httptest.Server

// AuthenticatedRequest — request with valid JWT for test user + role
func AuthenticatedRequest(method, path string, body any, role string) *http.Request

// TenantRequest — request scoped to a specific test tenant
func TenantRequest(method, path string, body any, tenantSlug string) *http.Request

// AssertContract — verify response matches contract definition
func AssertContract(t *testing.T, contractFile, endpointID string, resp *http.Response)
```

### 30.4 Test Command Surface

```bash
acthur test                    # unit tests for all services
acthur test api                # unit tests for api service only
acthur test --integration      # integration tests (requires docker)
acthur test --contract         # contract compliance tests
acthur test --e2e              # end-to-end (starts full stack)
acthur test --load             # k6 load tests from contracts
acthur test --coverage         # generate + open coverage report
acthur test --watch            # re-run unit tests on file change
acthur test --ci               # all tests, no interactive output (for CI)
```

---

## 31. Connection Pool Algorithm

### 31.1 The Problem Acthur Solves

Default connection pool settings in every framework are dangerously low or unconfigured, causing:
- **Pool exhaustion** — requests queue, latency spikes, timeouts cascade
- **Stale connections** — idle connections broken by firewalls, causing "broken pipe" errors
- **Cold start latency** — `MinConns: 0` means first burst creates connections under load
- **Cross-tenant data leakage** — schema multitenancy requires `search_path` reset on every connection return

### 31.2 The Tuning Formula

```
Inputs Acthur knows at runtime:
  cpu_cores            = runtime.NumCPU()
  service_count        = nodes with depends_on → db edge
  postgres_max_conns   = queried: SHOW max_connections
  node_role            = server | worker (workers burst then go idle)
  multitenancy_active  = plugin installed + strategy == schema

Formula:
  budget          = floor(postgres_max_conns × 0.80)   # never use >80% of pg max
  per_service     = floor(budget ÷ service_count)

  MaxConns        = min(per_service, (cpu_cores × 2) + 1)
  MinConns        = max(2, floor(cpu_cores ÷ 2))

  MaxConnLifetime       = 1h
  MaxConnLifetimeJitter = 5m     (stagger rotation — prevent thundering herd)
  MaxConnIdleTime       = 30m    (services) · 5m (workers)
  HealthCheckPeriod     = 30s
  BeforeAcquire         = ping   (validate before handing to handler)
  AfterRelease          = reset_search_path  (if multitenancy:schema active)
  ConnectTimeout        = 5s
```

### 31.3 Cross-Tenant Safety

Schema isolation multitenancy introduces a critical pool safety requirement. Without `AfterRelease`, a connection returned to the pool still has `search_path = tenant_abc`. The next request for `tenant_xyz` receives that connection and queries `tenant_abc`'s data — a data breach.

Acthur generates the `AfterRelease` hook automatically when the multitenancy plugin with `strategy: schema` is active:

```go
// Generated in internal/db/pool.go
poolCfg.AfterRelease = func(conn *pgx.Conn) bool {
    // Reset search_path to public before returning to pool
    // Prevents cross-tenant data access through connection reuse
    _, err := conn.Exec(context.Background(), "SET search_path TO public")
    return err == nil
}
```

---

## 32. Complete `acthur.yml` Schema Reference

### 32.1 Top-Level Fields

```yaml
project: string              # REQUIRED. Project name. Used for .test domain, image names.
version: "1"                 # REQUIRED. Schema version. Currently only "1".

identifiers:
  strategy: ulid             # ulid | uuid-v4 | cuid2 | nanoid | snowflake | sequential
  public_strategy: nanoid    # Optional: different strategy for public-facing IDs
  nanoid_length: 21          # Only for nanoid strategy. Range: 6-36.

dev:
  domain: myapp.test         # Default: project + ".test"
  https: false               # Requires https plugin
  port: 4000                 # Unified proxy port
  dns_strategy: auto         # auto | local-dns | hosts-file | pac-proxy

environments:
  dev:
    context: local           # local | docker
  staging:
    context: docker
    target: coolify
    host: staging.example.com
  production:
    context: cloud
    target: coolify          # coolify | fly | railway | render | docker | k8s
    host: example.com
```

### 32.2 Graph Nodes

```yaml
graph:
  nodes:
    <node-id>:
      type: service | infra | plugin | contract   # REQUIRED

      # Service node fields:
      adapter: go:fiber         # REQUIRED. runtime:framework key.
      port: 8080                # Dev port. Auto-assigned if omitted.
      hot_reload: true          # Default: true for service nodes.
      dev_url: api.myapp.test   # Override auto-generated subdomain.
      role: server              # server | queue-worker | cron | gateway
      websocket: false          # Enable WebSocket proxy pass-through.

      # Infra node fields:
      adapter: db:postgres      # REQUIRED.
      version: "16"             # Container image version.
      pool:                     # db:* adapters only
        max_conns: auto         # int or "auto" (Acthur calculates)
        min_conns: auto
        max_conn_lifetime: 1h
        max_conn_lifetime_jitter: 5m
        max_conn_idle_time: 30m
        health_check_period: 30s
        connect_timeout: 5s
        before_acquire: ping    # ping | none
        after_release: reset_search_path  # reset_search_path | none
```

### 32.3 Graph Edges

```yaml
  edges:
    - from: web               # REQUIRED. Source node ID.
      to: api                 # REQUIRED. Target node ID.
      type: data_flow         # REQUIRED. Edge type.
      # data_flow extra:
      contracts: [contracts/users.contract.yml]   # REQUIRED for data_flow
      transport: http         # http | grpc | graphql | ws | queue

      # emits / subscribes_to extra:
      events: [user.created, user.deleted]        # REQUIRED

      # proxied_through extra:
      path_prefix: /api       # Default: derived from node ID
```

Edge types:
```
depends_on       → startup ordering. B waits for A to be healthy before starting.
data_flow        → A calls B. Requires at least one contract.
proxied_through  → traffic from A routes through B (the proxy).
satisfies        → service A implements contract B.
migrates         → service A owns infra node B's schema.
emits            → service A publishes these events.
subscribes_to    → service A consumes these events.
applies_to       → plugin A injects behavior into service B.
```

---

## 33. CLI Identity — ASCII Art, FIGlet Banner, and UX Standards

### 33.1 Acthur ASCII Banner

Acthur uses a distinctive FIGlet-style ASCII banner rendered via `go-figure` on key commands. The banner is rendered in the terminal's full color support — 256-color or true color where available, with graceful fallback to plain ASCII in non-TTY environments (CI/CD, pipes, redirects).

**The Acthur banner — rendered with `go-figure` (doom font variant, custom colorized):**

```
                  ██████╗  ██╗      ██████╗  ██╗   ██╗  ██████╗ 
                 ██╔═══██╗ ██║     ██╔═══██╗ ██║   ██║ ██╔═══██╗
                 ██║████╗  ██║     ██║███╔██║ ██║   ██║ ██║███╔██║
                 ██║╚═══█║ ██║     ██║╚══╝██║ ██║   ██║ ██║╚══╝██║
                 ╚██████╔╝ ███████╗╚██████╔╝ ╚██████╔╝ ╚██████╔╝
                  ╚═════╝  ╚══════╝ ╚═════╝   ╚═════╝   ╚═════╝

                  Runtime Graph Operating System  ·  v0.1.0
                  github.com/acthur/acthur
```

Colorized version:
- `ACTHUR` text: bold cyan (`#00D4FF`)
- Subtitle line: dim gray (`#888888`)
- Version: white
- URL: dim gray, underlined

**Alternative compact banner** (used in `acthur dev` prefix line, not full banner):
```
  ✦  Acthur v0.1.0
```

### 33.2 Banner Rendering Rules

```
Command                    Banner shown
──────────────────────────────────────────────────────────────
acthur new                 Full banner (before wizard)
acthur dev                 Full banner (before startup sequence)
acthur version             Full banner + version table
acthur --help              Compact banner only (no whitespace waste)
acthur deploy              Compact prefix only (not full banner)
acthur doctor              Full banner
acthur init                Full banner

Non-interactive (CI, pipe, redirect):
acthur *                   No banner (color disabled, plain output)
```

### 33.3 Implementation — go-figure + lipgloss

```go
// internal/output/banner.go
package output

import (
    figure "github.com/common-nighthawk/go-figure"
    "github.com/charmbracelet/lipgloss"
    "os"
)

var (
    styleBanner  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D4FF")).Bold(true)
    styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
    styleVersion = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
    styleURL     = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Underline(true)
)

// Banner renders the full Acthur ASCII banner.
// Suppressed in non-TTY environments (CI, pipes).
func Banner(version string) {
    if !isTerminal() {
        return
    }

    fig := figure.NewFigure("ACTHUR", "doom", true)
    ascii := fig.String()

    fmt.Println()
    // Render each line of the ASCII art in brand cyan
    for _, line := range strings.Split(ascii, "
") {
        fmt.Println(styleBanner.Render("  " + line))
    }
    fmt.Println()
    fmt.Printf("  %s  ·  %s
",
        styleDim.Render("Runtime Graph Operating System"),
        styleVersion.Render("v"+version),
    )
    fmt.Println("  " + styleURL.Render("github.com/acthur/acthur"))
    fmt.Println()
}

// CompactBanner renders the one-line compact prefix.
func CompactBanner(version string) {
    fmt.Printf("  ✦  %s %s

",
        styleBanner.Render("Acthur"),
        styleDim.Render("v"+version),
    )
}

func isTerminal() bool {
    fi, err := os.Stdout.Stat()
    if err != nil {
        return false
    }
    return (fi.Mode() & os.ModeCharDevice) != 0
}
```

### 33.4 `acthur dev` Startup Output

The full startup sequence output with consistent formatting:

```
  ██████╗  ██╗      ██████╗  ██╗   ██╗  ██████╗
  ...   (ASCII art in cyan)
  Runtime Graph Operating System  ·  v0.1.0
  github.com/acthur/acthur

  [acthur] ✓  graph loaded — 8 nodes · 14 edges
  [acthur] →  startup order: db → cache → queue → api → worker → web → backoffice

  [db]     →  starting postgres:16...
  [db]     ✓  healthy (port 5432)
  [cache]  →  starting redis:7...
  [cache]  ✓  healthy (port 6379)
  [api]    →  starting go:fiber (air)...
  [api]    ✓  healthy → http://api.vetangle.test
  [web]    →  starting ui:astro...
  [web]    ✓  healthy → http://vetangle.test
  [proxy]  ✓  listening on http://localhost:4000

  ✦  vetangle is ready

     proxy      http://localhost:4000
     web        http://vetangle.test
     api        http://api.vetangle.test
     backoffice http://backoffice.vetangle.test
     db studio  http://db.vetangle.test
     mail       http://mail.vetangle.test

  Press Ctrl+C to stop all services
```

### 33.5 Error Message Format

Every Acthur error states exactly what went wrong, why it is a problem, and what to do:

```
  [acthur] ✗  Error: missing contract on data_flow edge

  Edge:    web → api
  Problem: data_flow edges require at least one contract.
           Without a contract, Acthur cannot enforce type safety
           between these two services.

  Fix:     Add a contract reference:

           edges:
             - from: web
               to: api
               type: data_flow
               contracts: [contracts/api.contract.yml]

           Then create it:
           → acthur generate contract api

  Docs:    https://acthur.dev/docs/contracts
  Exit:    5
```

### 33.6 Exit Codes

```
0   Success
1   General error
2   Graph error          (invalid nodes/edges)
3   Adapter error        (unknown or failed adapter)
4   Plugin error         (plugin failed to load or register)
5   Contract error       (contract validation failed)
6   Environment error    (missing tool, docker not running)
7   Deploy error         (preflight check failed)
8   Config error         (acthur.yml parse or schema error)
9   Process error        (managed service failed to start)
10  Contract break       (breaking change without version bump)
```

### 33.7 Warning vs Error Policy

```
ERROR (blocks execution):
  Graph validation failures · Missing required adapter
  Missing required plugin dependency
  Contract breaking change on deploy
  Process failed to start after max retries
  Secret reference unresolvable in target environment

WARNING (logs, continues):
  Contract violation in dev mode · AI context files stale
  Port in use (if Acthur can use alternative)
  Service slower than expected to become healthy
  Pool utilization above 80%

INFO (always shown):
  Service start/stop events · Migration results · Deploy progress

DEBUG (--verbose flag only):
  Graph traversal decisions · Process spawn commands
  Proxy routing decisions · Plugin hook execution
```


---

## 34. i18n and l10n

### 34.1 Locale Detection Pipeline

```
1. User preference (stored in DB — highest priority)
2. Cookie (acthur_locale)
3. Accept-Language header (browser standard)
4. URL path prefix (/fr/...) if path-based i18n selected
5. Default locale (from acthur.yml i18n.default_locale)
```

### 34.2 Translation Structure

```
locales/
├── en-US/
│   ├── common.json           shared strings
│   ├── auth.json             generated by auth plugin
│   ├── errors.json           generated from contract error codes
│   └── emails.json           generated by mailer plugin
├── fr-FR/
│   └── ...
└── ar-SA/
    ├── ...
    └── rtl: true             RTL stylesheet generation triggered
```

### 34.3 Typed Translation Keys

```typescript
// Generated: src/i18n/keys.ts — no magic strings, full type checking
export const AuthKeys = {
  LoginTitle:         'auth.login.title',
  LoginSubmit:        'auth.login.submit',
  InvalidCredentials: 'auth.errors.invalid_credentials',
} as const

// Usage:
t(AuthKeys.LoginTitle)    // ✓ type-safe
t('auth.logi.ttle')       // ✗ TypeScript error
```

---

## 35. CI/CD Pipeline Generation

### 35.1 Pipeline Structure

All generated pipelines follow the same stage order. Graph topology drives job parallelism.

```
Stage 1: Validate        graph validate · contract validate · contract diff
Stage 2: Test (parallel) one job per service node
Stage 3: Contract        contract compliance tests (needs stage 2)
Stage 4: Build (parallel) one job per service node (needs stage 3)
Stage 5: Security        dependency vulnerability scan
Stage 6: Deploy Staging  on merge to develop
Stage 7: Deploy Prod     on merge to main / version tag
```

### 35.2 Targets

```bash
acthur generate ci --target github-actions  # .github/workflows/acthur-ci.yml
acthur generate ci --target gitlab-ci       # .gitlab-ci.yml
acthur generate ci --target circleci        # .circleci/config.yml
```

---

## 36. Documentation Generation

```bash
acthur generate docs --target astro    # adds to Astro docs site
acthur generate docs --target nextra   # Next.js MDX pages
acthur generate docs --target readme   # updates README.md
acthur generate docs --target openapi  # generates openapi.yml
```

Generated:
```
docs/
├── architecture.mdx    visual graph diagram (Mermaid)
├── api/                one page per contract
├── guides/             auth, multitenancy, feature-flags
└── runbooks/           deployment, incident response, migration
```

---

## 37. Feature Flags

### 37.1 Flag Lifecycle

```bash
acthur flag create new-booking-flow --description "..."
acthur flag list
acthur flag toggle new-booking-flow
acthur flag rollout new-booking-flow --percentage 20
acthur flag retire new-booking-flow
```

### 37.2 Providers

```
local       → JSON file at .acthur/flags.json (dev default, hot-reloaded)
flagsmith   → self-hostable, real-time updates, targeting rules
growthbook  → self-hostable, A/B testing + experimentation
launchdarkly → cloud, enterprise targeting
```

### 37.3 Context-Aware Evaluation

```go
// Generated typed flag constants — no magic strings
const FlagNewBookingFlow = "new_booking_flow"

// Evaluate with full context: user, tenant, environment
enabled := flags.IsEnabled(ctx, FlagNewBookingFlow)
// ctx carries user, tenant, environment — all available to flag rules
```

---


## 38. Starter Templates — Production-Ready SaaS Applications

### 40.1 What Templates Are

A template is not a code scaffold. It is a complete, production-deployable application built on Acthur's stack — fully wired auth, working UI, real database migrations, seeded data, and a passing test suite. Run `acthur dev` and it works. Run `acthur deploy` and it ships.

Templates encode the expertise of building a specific application type. A developer using the SaaS Starter template gets the benefit of every decision that went into building it — auth strategy, RBAC roles, tenant isolation approach, billing integration, admin panel — without making those decisions themselves.

**The rule:** Templates are always built with Acthur. They are `acthur.yml` + source code + contracts. They follow all of Acthur's generation rules. A template is proof that Acthur's own tooling is capable of producing what it claims.

### 40.2 Using Templates

```bash
# List available templates
acthur template list

# Create project from a template
acthur new my-saas --template saas-starter

# The wizard still runs — you choose your adapter stack
# The template adapts to your choice: go:fiber or rust:axum or node:fastify
acthur new my-saas --template saas-pro --backend go:fiber --frontend ui:next

# Preview template before using it
acthur template preview saas-starter
```

The wizard runs normally when using a template. The template provides the application structure and plugin configuration. The developer chooses the adapter stack. Acthur generates the correct implementation for the chosen adapters.

### 40.3 Free Templates

#### `saas-starter` — Minimal SaaS Boilerplate

The minimum viable SaaS. Auth, dashboard, API, deployment config.

```
Stack choice:  any backend · any frontend
Plugins:       migrations · auth (jwt + email) · rbac · security · https
Database:      postgres
Frontend:      login · register · dashboard · profile · settings
Admin:         not included (use saas-pro for admin)
Billing:       not included (use saas-pro for billing)
```

Generated `acthur.yml`:
```yaml
project: my-saas
plugins:
  - name: migrations
  - name: auth
    config:
      strategy: jwt
      algorithm: RS256
      oauth2:
        providers: [google]
  - name: rbac
    config:
      roles: [admin, member]
  - name: security
  - name: https
```

What you get immediately on `acthur dev`:
```
✓ Login page (email + Google)
✓ Register page with email verification
✓ Forgot/reset password flow
✓ Authenticated dashboard with placeholder stats
✓ User profile page (edit name, email, avatar, password)
✓ Account settings (notification preferences, danger zone)
✓ Admin users page (admin role can manage all users)
✓ All API endpoints from contracts
✓ Complete migration history
✓ Seed data (demo users: admin@demo.com / member@demo.com)
```

---

#### `api-platform` — REST/GraphQL API with Auth

API-only project. No frontend generated. Best for mobile-first or third-party UI.

```
Stack choice:  any backend
Plugins:       migrations · auth · rbac · security
Transports:    REST + GraphQL (both generated from contracts)
Docs:          OpenAPI spec generated · GraphQL playground
```

What you get:
```
✓ Complete REST API (auth, users, RBAC)
✓ GraphQL endpoint (same data, different transport)
✓ JWT auth with refresh tokens
✓ Role-based access control
✓ Rate limiting per endpoint
✓ OpenAPI spec at /api/docs
✓ GraphQL playground at /graphql (dev only)
✓ Postman collection generated
✓ All migrations + seed data
```

---

#### `blog` — Content Platform

Publishing platform with public content and admin editorial panel.

```
Plugins:  migrations · auth · rbac · docs
Roles:    admin · editor · author · reader
Features: posts · categories · tags · comments · SEO meta · RSS
```

What you get:
```
✓ Public blog: post list · post detail · category pages · search
✓ Author portal: write · edit · preview · publish (with auth)
✓ Admin panel: manage all content · moderate comments · manage authors
✓ RSS feed · sitemap.xml · OpenGraph meta tags
✓ Image upload via storage plugin
✓ Markdown editor (frontend)
```

---

#### `landing` — Marketing Site + Waitlist

Static marketing site with a working waitlist capture and email confirmation.

```
Frontend:  ui:astro (optimal for static/SSG marketing sites)
Plugins:   auth (magic-link only, for waitlist confirmation)
Features:  hero · features · pricing · FAQ · waitlist form · email confirmation
```

---

### 40.4 Paid / Premium Templates

Premium templates are available in the Acthur Template Marketplace. They are purchased once — you receive full source code ownership with no ongoing fees to Acthur.

#### `saas-pro` — Full SaaS Platform

The complete production SaaS. Everything in `saas-starter` plus multi-tenancy, billing, analytics, and a full admin panel.

```
Plugins:   migrations · auth (jwt + oauth2 + mfa) · rbac · multitenancy (schema)
           feature-flags · admin · observability · security · https
           product-analytics (posthog) · web-analytics (plausible)
Billing:   Stripe integration (subscriptions + usage billing)
Storage:   MinIO / S3 (file uploads + avatars)
Queue:     Background jobs (email, billing webhooks, usage metering)
```

What you get immediately on `acthur dev`:

```
Customer-facing:
  ✓ Full auth flow: email + Google + GitHub + TOTP MFA
  ✓ Onboarding wizard (company name, invite teammates, connect billing)
  ✓ Team dashboard with role-based sections
  ✓ Billing page (plan selection, payment method, invoices via Stripe)
  ✓ Usage dashboard (API calls, storage, seats)
  ✓ Team management (invite, remove, change roles)
  ✓ User profile (avatar upload, 2FA setup, connected accounts)
  ✓ Notification preferences
  ✓ Feature flags (gated features per plan tier)

Admin panel (admin.APP_NAME.test):
  ✓ Tenant overview (all tenants, status, MRR, usage)
  ✓ User management across all tenants
  ✓ Billing management (override plans, apply credits, view invoices)
  ✓ Feature flag management (toggle per tenant or globally)
  ✓ System health dashboard (metrics, logs, alerts)
  ✓ Audit log viewer

Background services:
  ✓ Email worker (welcome, verification, password reset, invoices)
  ✓ Billing webhook processor (Stripe events → DB state)
  ✓ Usage metering worker (collects usage → Stripe metered billing)
  ✓ Tenant provisioning worker (creates schema on new signup)
```

---

#### `marketplace` — Two-Sided Marketplace

Buyers and sellers. Listings, offers, payments, reviews, disputes.

```
Roles:   admin · seller · buyer
Billing: Stripe Connect (platform payments, seller payouts, fees)
Storage: file uploads (listing images, documents)
Queue:   notification worker · payout scheduler · review reminders
```

What you get:
```
Seller-facing:
  ✓ Seller onboarding (Stripe Connect express)
  ✓ Listing creation (title, description, images, pricing, categories)
  ✓ Order management (pending, in-progress, completed, disputed)
  ✓ Payout dashboard (earned, pending, paid out)
  ✓ Review responses

Buyer-facing:
  ✓ Browse and search listings (faceted search, category filters)
  ✓ Listing detail (images, seller profile, reviews)
  ✓ Checkout (Stripe payment intent, escrow)
  ✓ Order tracking
  ✓ Review submission

Admin:
  ✓ Platform overview (GMV, take rate, disputes)
  ✓ Dispute resolution (review, resolve, issue refund)
  ✓ Seller verification queue
  ✓ Listing moderation
```

---

#### `crm` — Customer Relationship Management

Contacts, companies, deals, pipelines, activities, notes.

```
Roles:   admin · manager · rep · viewer
Features: contacts · companies · deals · pipelines · activities
          email integration · calendar sync · reports
```

---

#### `ecommerce` — Online Store

Products, variants, inventory, cart, checkout, orders, fulfillment.

```
Roles:    admin · store-manager · customer
Billing:  Stripe (one-time payments, refunds)
Storage:  product images · exports
Features: product catalog · variants · inventory · discount codes
          cart · checkout · order management · fulfillment · reports
```

---

### 40.5 Template Adapter Flexibility

Templates are adapter-agnostic at the contract level. The same `saas-pro` template generates correct, idiomatic code for any backend adapter:

```bash
acthur new my-saas --template saas-pro --backend go:fiber --frontend ui:next
acthur new my-saas --template saas-pro --backend rust:axum --frontend ui:astro
acthur new my-saas --template saas-pro --backend node:fastify --frontend ui:next
acthur new my-saas --template saas-pro --backend python:fastapi --frontend ui:vue
```

The contracts are identical. The generated backend code is idiomatic for each language. The frontend API clients are regenerated from the same contracts.

### 40.6 Template Registry

```bash
acthur template list
  free:
  ○  saas-starter          Minimal SaaS boilerplate
  ○  api-platform          REST + GraphQL API with auth
  ○  blog                  Content platform with admin
  ○  landing               Marketing site + waitlist

  premium (marketplace.acthur.dev):
  ★  saas-pro              Full SaaS: multi-tenancy, billing, admin, analytics
  ★  marketplace           Two-sided marketplace with Stripe Connect
  ★  crm                   CRM with pipelines and email integration
  ★  ecommerce             Online store with Stripe

acthur template preview saas-starter
  → opens https://saas-starter.demo.acthur.dev
  → demo credentials shown (admin@demo.com / any password)

acthur template install saas-pro
  → verifies purchase license
  → downloads template source
  → runs wizard to select adapters
  → scaffolds into current directory
```

### 40.7 Template Versioning

Templates are versioned alongside Acthur. A template specifies the minimum Acthur version it requires:

```yaml
# template.yml (internal manifest)
name: saas-pro
version: "2.1.0"
acthur_min_version: "1.2.0"
adapters:
  backend: [go:fiber, go:chi, rust:axum, node:fastify, python:fastapi]
  frontend: [ui:astro, ui:next, ui:vue, ui:svelte]
  mobile: [mobile:flutter, mobile:react-native]
plugins_required: [migrations, auth, rbac, multitenancy, security]
plugins_optional: [observability, feature-flags, product-analytics]
```

---

## 39. Dokploy — Self-Hosted PaaS Integration

### 41.1 What Dokploy Is

Dokploy is an open-source (MIT) self-hosted PaaS built on Docker Swarm and Traefik. It provides:

```
Docker Swarm orchestration   → rolling updates, replicas, health checks
Traefik reverse proxy        → automatic TLS, domain routing, load balancing
Git-based deployments        → push to deploy, webhook triggers
Multi-server management      → deploy across a cluster of VPS nodes
Database management          → managed Postgres, MySQL, Redis, Mongo
Environment variable management → per-service, encrypted at rest
Monitoring                   → basic metrics, logs viewer
```

**Dokploy vs Coolify comparison:**

| Feature | Coolify | Dokploy |
|---------|---------|---------|
| Orchestration | Docker standalone / Swarm | Docker Swarm (primary) |
| Reverse proxy | Traefik / Caddy | Traefik |
| Rolling updates | ✓ | ✓ |
| Multi-server | ✓ | ✓ |
| License | Apache 2.0 | MIT |
| API | REST | REST |
| Acthur adapter | `deploy:coolify` | `deploy:dokploy` |

Both are valid choices. The Acthur adapter handles the differences transparently.

### 41.2 Setting Up Dokploy

```bash
# Install Dokploy on a fresh VPS (Ubuntu 22.04 / Debian 12)
curl -sSL https://dokploy.com/install.sh | sh

# Access Dokploy UI at: http://YOUR_SERVER_IP:3000
# Complete initial setup: create admin account, configure server

# Get API key from Dokploy UI → Settings → API Keys
# Add to your environment:
export DOKPLOY_API_KEY=your_api_key_here
export DOKPLOY_URL=https://your-dokploy-server.com
```

### 41.3 Acthur Configuration for Dokploy

```yaml
# acthur.yml
environments:
  production:
    context: cloud
    target: dokploy
    host: your-dokploy-server.com
    api_key: ${DOKPLOY_API_KEY}
    project_name: vetangle
    registry:
      url: registry.your-dokploy-server.com    # Dokploy includes a private registry
      username: ${REGISTRY_USER}
      password: ${REGISTRY_PASSWORD}
    swarm:
      replicas:
        api: 2
        web: 2
        worker: 1
      update_config:
        parallelism: 1
        delay: 10s
        failure_action: rollback
      resources:
        api:
          limits:    { cpus: "0.5", memory: "512m" }
          reservations: { cpus: "0.25", memory: "256m" }
```

### 41.4 What the Dokploy Adapter Generates

From the graph, the `deploy:dokploy` adapter generates:

```
.acthur/deploy/dokploy/
├── project.json               → Dokploy project definition
├── services/
│   ├── api.json               → Application service (api node)
│   ├── web.json               → Application service (web node)
│   ├── worker.json            → Application service (worker node)
│   ├── db.json                → Database service (db node)
│   └── cache.json             → Database service (cache node)
├── compose.swarm.yml          → Docker Swarm compose file
└── traefik/
    └── dynamic.yml            → Traefik routing rules from graph edges
```

### 41.5 Traefik Configuration Derived from Graph

The `proxied_through` and `data_flow` edges in the graph drive Traefik routing:

```yaml
# .acthur/deploy/dokploy/traefik/dynamic.yml — Generated from graph
# api node proxied_through edge → /api/* prefix
# web node proxied_through edge → /* catch-all

http:
  routers:
    api-router:
      rule: "Host(`api.vetangle.com`)"
      service: api-service
      tls:
        certResolver: letsencrypt

    web-router:
      rule: "Host(`vetangle.com`) || Host(`www.vetangle.com`)"
      service: web-service
      tls:
        certResolver: letsencrypt

    backoffice-router:
      rule: "Host(`backoffice.vetangle.com`)"
      service: backoffice-service
      middlewares: [auth-middleware]
      tls:
        certResolver: letsencrypt

  services:
    api-service:
      loadBalancer:
        servers:
          - url: "http://api:8080"
        healthCheck:
          path: /health
          interval: "30s"

  middlewares:
    auth-middleware:
      # Rate limiting from security plugin
      rateLimit:
        average: 100
        burst: 50
```

### 41.6 Docker Swarm Deployment Flow

```bash
acthur deploy --target dokploy --env production

  [acthur] running preflight checks...
  [acthur] ✓ graph valid · 8 nodes · 14 edges
  [acthur] ✓ all contracts valid · no breaking changes
  [acthur] ✓ api builds clean (go:fiber)
  [acthur] ✓ web builds clean (ui:astro)
  [acthur] ✓ worker builds clean (go:fiber)
  [acthur] ✓ all tests pass (142 passed)
  [acthur] ✓ 0 pending migrations

  [acthur] building and pushing images...
  [acthur] ✓ api  → registry.vetangle.com/api:v1.3.0
  [acthur] ✓ web  → registry.vetangle.com/web:v1.3.0
  [acthur] ✓ work → registry.vetangle.com/worker:v1.3.0

  [acthur] deploying to Dokploy (your-dokploy-server.com)...
  [acthur] ✓ db service provisioned (postgres:16)
  [acthur] ✓ cache service provisioned (redis:7)
  [acthur] ✓ running migrations (api → db)...
  [acthur] ✓ 3 migrations applied
  [acthur] ✓ api deployed (2 replicas, rolling update, 0 downtime)
  [acthur] ✓ web deployed (2 replicas, rolling update, 0 downtime)
  [acthur] ✓ worker deployed (1 replica)
  [acthur] ✓ Traefik routes updated
  [acthur] ✓ TLS certificates provisioned (Let's Encrypt)

  [acthur] ✦ deployed in 3m 42s
  [acthur]   https://vetangle.com
  [acthur]   https://api.vetangle.com
  [acthur]   https://backoffice.vetangle.com
```

### 41.7 Rollback Support

Dokploy's Docker Swarm rollback is exposed via Acthur:

```bash
acthur deploy rollback           # rollback all services to previous version
acthur deploy rollback api       # rollback only the api service
acthur deploy status             # show current deployed versions + health
```


## 40. Roadmap

### 38.1 v1.0 — Core System (Phases 0–8)
Go + Astro/Next.js · `go:fiber` + `go:chi` · `db:postgres` + `cache:redis` · migrations, auth (JWT + OAuth2), rbac, multitenancy, security, https · Coolify + Docker deploy · Claude Code + Cursor AI context · MCP server

### 38.2 v1.1 — Rust + Node.js
`rust:axum` · `rust:actix` · `node:fastify` · `node:nest` · `bun:elysia` · `ui:vue` + `ui:svelte` · observability (OTel) · feature-flags · ci-cd · docs generation · Fly.io + Railway deploy

### 38.3 v1.2 — Analytics + Full Observability
PostHog + Plausible + Metabase · full OTel stack (Jaeger + Prometheus + Grafana + Loki) · load testing (k6) · audit-log plugin · traceability across all adapters

### 38.4 v1.3 — Python + PHP + Mobile
`python:fastapi` · `python:django` · `php:laravel` · `mobile:flutter` · `desktop:tauri` · admin panel plugin · Kubernetes (Helm chart) deploy

### 38.5 v2.0 — Enterprise
Vault + Doppler secrets · SAML / enterprise SSO · multi-region deploy · data residency configuration · SLA monitoring · `acthur doctor` CVE scanning · Acthur Cloud (optional managed version)

---

## 41. Constraints and Non-Goals

### 41.1 Non-Goals — What Acthur Is Not

Understanding what Acthur is **not** prevents scope creep and preserves architectural integrity. These are deliberate constraints, not omissions.

| Non-Goal | Rationale |
|---|---|
| **Not a managed hosting platform** | Acthur generates deployment configurations and orchestrates deployments but does not sell compute. It targets your infrastructure. |
| **Not a low-code / no-code tool** | Acthur is code-first. The manifest drives generation but developers own and can modify all generated code. |
| **Not an ORM** | Acthur wraps and generates code for existing ORMs (GORM, SQLx, Diesel, SeaORM) rather than implementing one. |
| **Not a database** | Acthur manages the layer that communicates with storage — it does not manage storage itself. |
| **Not a serverless framework** | Single-binary long-running processes are the execution model. Serverless/FaaS is architecturally incompatible with the queue + scheduler + binary model. |
| **Not a microservices orchestrator by default** | Modular monolith is the canonical architecture. Microservice extraction is optional and deliberate. |
| **Not opinionated about frontend state management** | Acthur scaffolds frontend structure and API clients but does not prescribe Zustand vs. Jotai vs. Redux vs. Pinia. |
| **Not a CI/CD platform** | Acthur generates CI/CD configuration files — it does not run pipelines itself. |
| **Not a code editor or IDE** | `acthur agent` assists via CLI. IDE integration is via the LSP extension (Phase 5), not a bundled editor. |
| **Not a runtime dependency** | All generated code has zero Acthur imports. Generated applications can be built and deployed without the Acthur binary present. |

### 41.2 Deliberate Architectural Constraints

These constraints are non-negotiable — they maintain system integrity:

```
CONSTRAINT                          REASON
──────────────────────────────────────────────────────────────────────────
Adapter layer cannot import domain   Enforces dependency inversion
Domain cannot import adapters        Domain depends on contracts only
Plugin cannot call other plugins     Prevents undeclared coupling
Generator output has no acthur/*     Generated code must be self-contained
Kernel binary < 500 KB              Runtime footprint guarantee
Kernel init < 200 ms                Startup reliability guarantee
Go CLI binary (not JS/Python)       Required for cross-platform single binary
YAML as manifest format             Human-readable, version-control friendly
```

### 41.3 Known Limitations at v1.0

```
Windows local DNS          → uses hosts-file fallback (minor UX difference vs macOS/Linux)
Schema multitenancy        → PostgreSQL only (MySQL uses row-level strategy)
GraphQL subscriptions      → WebSocket setup requires minor manual completion
acthur init detection      → accuracy depends on conventional project structures
MongoDB adapter            → Phase 3 (not v1.0)
Native mobile (Flutter)    → Phase 3 scaffold; no over-the-air update support at launch
Terraform IaC generation   → Phase 4
```

### 41.4 Evolution from Initial Design

The following represent intentional advances beyond the initial Acthur PRD. They are documented here so contributors understand these were deliberate decisions, not accidents:

| Initial Design | Evolution | Rationale |
|---|---|---|
| Single runtime per project | Multiple backends per project (polyglot) | Modern systems mix Go, Rust, Python, Node for different services |
| "Not GraphQL-native" | GraphQL as first-class transport | GraphQL has become standard for frontend-API communication |
| Go/Rust only | Python, Node.js, PHP, Bun, Flutter, React Native | Realistic developer teams use diverse stacks |
| REST primary transport | REST + GraphQL + gRPC equal peers | gRPC for internal services is now standard |
| Modular monolith only | Microservices as equal option | Graph model handles both without special casing |



---

## 42. Compilation Pipeline & system.json AST

### 42.1 Overview

The compilation pipeline transforms the human-readable `acthur.yml` manifest into a machine-optimised `system.json` AST, which then drives all code generation. This is distinct from the runtime graph — the compilation pipeline is a build-time step.

```
acthur.yml
    │
    ▼
Stage 1: YAML Parser
  Parses and validates raw YAML against the Acthur JSON Schema
    │
    ▼
Stage 2: Schema Validator
  Validates field types, relations, adapter compatibility
  Resolves implicit defaults and inferred settings
  Rejects: diesel with go:fiber, schema-tenancy with sqlite, etc.
    │
    ▼
Stage 3: AST Builder → system.json
  Constructs the fully resolved Abstract Syntax Tree
  Includes: resolved relations, RBAC graph, event-handler wiring,
  migration topology, adapter binding map, DI dependency graph
    │
    ▼
Stage 4: Diff Engine
  Compares new AST against previous system.json
  Produces a minimal changeset for incremental generation
  Only regenerates modules touched by the diff
    │
    ▼
Stage 5: Code Generation
  Template engine consumes AST + diff changeset
  Generates: migrations, models, repositories, services,
  controllers, routes, validators, event handlers,
  frontend pages, admin views, TypeScript SDK,
  IaC templates, CI/CD pipelines
```

### 42.2 The system.json AST

The compiled `system.json` is a version-controlled artifact representing the fully resolved, validated application graph. Developers commit this file alongside `acthur.yml`. CI/CD pipelines compare AST diffs to determine deployment scope.

```json
{
  "version": "1.0.0",
  "runtime": { "backend": "go:fiber", "frontend": "ui:astro" },
  "domains": [
    {
      "name": "users",
      "table": "users",
      "identifier": { "type": "ulid", "column": "id" },
      "fields": [...],
      "relations": [...],
      "rbacPolicies": { "create": ["admin"], "read": ["admin", "vet", "user"] },
      "softDelete": true,
      "auditEnabled": true
    }
  ],
  "dependencyGraph": { "nodes": [...], "edges": [...] },
  "adapterBindings": { "orm": "gorm", "queue": "nats", "cache": "redis" },
  "eventWiringMap": { "UserRegistered": ["send-welcome-email", "provision-defaults"] },
  "capabilityDAG": [...],
  "generatedAt": "2025-05-21T10:00:00Z",
  "acthurVersion": "1.0.0"
}
```

### 42.3 Incremental Generation

Acthur does not regenerate the entire codebase on every manifest change. The Diff Engine computes the minimal changeset and regenerates only affected modules, preserving custom developer modifications in adjacent files.

**Generation regions** delimit exactly which code blocks Acthur owns:

```go
// @acthur:generated:start — DO NOT EDIT
// This region is owned by Acthur and overwritten on acthur generate
func (r *UserRepository) FindByID(ctx context.Context, id string) (*User, error) {
    return r.db.WithContext(ctx).First(&User{}, "id = ?", id).Error
}
// @acthur:generated:end

// Developer extensions — PRESERVED across regeneration:
func (r *UserRepository) FindActiveByTenantWithFilter(ctx context.Context, tenantID string, filter Filter) ([]*User, error) {
    // Custom logic — never touched by acthur generate
}
```

Outside generation regions, developer code is always preserved. Inside them, Acthur owns the content and regenerates on `acthur generate`.

### 42.4 Manifest-Driven Change Cascade

When a developer modifies `acthur.yml` (e.g., adds a new field to the `appointments` domain):

```
1. File watcher detects acthur.yml change
2. Incremental generator computes diff (only appointments module affected)
3. Regenerates: appointments model, migration, DTO, validator, admin view, routes
4. TypeScript SDK package is updated
5. Frontend hot-module reloader picks up SDK change
6. Developer sees updated types in IDE within 2–4 seconds
```

---

## 43. Capability System & Dependency Graph

### 43.1 What Capabilities Are

Capabilities declare **what** an application requires, independent of **how** it is fulfilled. Selecting a capability in `acthur.yml` automatically resolves its transitive dependencies.

### 43.2 Capability Dependency Graph

```
http-server     → (no dependencies)
auth            → http-server, database, cache
rbac            → auth
tenancy         → database, rbac
orm             → database
queue           → (optional: cache for Redis backend)
scheduler       → queue, (optional: cache for distributed lock)
websocket       → http-server, (optional: cache for Redis multi-node)
notifications   → queue, mailer
payments        → http-server, database, queue, notifications
storage         → (no required deps)
uploads         → storage, (optional: queue for background processing)
audit           → database, auth
feature-flags   → database, cache
admin           → http-server, auth, rbac, orm
observability   → (no deps — always active)
```

If `payments` is declared in `acthur.yml`, Acthur automatically resolves and includes `auth`, `database`, `queue`, and `notifications` even if not listed. A warning is surfaced informing the developer of implicit inclusions.

### 43.3 Resolution Rules

```
1. Explicit capabilities take precedence over implicit ones
2. Circular dependencies are rejected at manifest validation time
3. Implicit capabilities use default adapter choices (configurable)
4. All resolved capabilities are listed in system.json capabilityDAG
5. acthur graph show --capabilities prints the resolved capability tree
```

### 43.4 Capability Configuration

```yaml
# acthur.yml — explicit capability + adapter selection
plugins:
  - name: auth
    config:
      strategy: jwt
      algorithm: RS256
  - name: queue
    config:
      driver: nats          # nats | redis-asynq | kafka
  - name: cache
    config:
      l1: bigcache          # in-process (Go: bigcache | Rust: moka)
      l2: redis             # distributed
  - name: notifications
    config:
      email: resend
      sms: vonage           # vonage for Africa-strong SMS coverage
      push: fcm
```

---

## 44. Internal Capability Contracts

### 44.1 What Internal Contracts Are

Internal contracts are pure Go interfaces (or Rust traits) that define what every adapter must implement. Business logic depends exclusively on these contracts — never on adapter implementations. This is the foundation of Acthur's hexagonal architecture.

No generated service, handler, or repository ever imports an adapter package directly. It imports only the contract interface.

### 44.2 Contract Inventory

```
contracts/
├── http/
│   └── HttpServer, Router, Middleware, Context, Request, Response
│
├── auth/
│   └── AuthProvider, TokenIssuer, SessionStore, OAuthProvider, MFAProvider
│
├── database/
│   └── DatabaseDriver, Migrator, TransactionManager
│
├── orm/
│   └── Repository[T], QueryBuilder, Paginator, SoftDeleter
│
├── cache/
│   └── CacheStore, TTLPolicy, InvalidationStrategy, TagInvalidator
│
├── queue/
│   └── JobDispatcher, JobHandler, WorkerPool, DLQHandler, RetryPolicy
│
├── events/
│   └── EventBus, EventHandler, DomainEvent, EventStore
│
├── scheduler/
│   └── JobScheduler, CronJob, DistributedLock, SchedulerMonitor
│
├── notifications/
│   └── NotificationChannel, NotificationDispatcher, NotificationTemplate
│
├── tenancy/
│   └── TenantResolver, TenantContext, TenantRepository, TenantProvisioner
│
├── websocket/
│   └── WebSocketServer, Room, Channel, BroadcastAdapter
│
├── storage/
│   └── ObjectStore, PresignedURLProvider, ImageOptimizer, UploadHandler
│
├── payments/
│   └── PaymentGateway, WebhookHandler, SubscriptionManager, RefundManager
│
├── rbac/
│   └── PolicyEnforcer, RoleManager, PermissionResolver, PolicyStore
│
├── audit/
│   └── AuditLogger, AuditRecord, ChangeTracker, AuditQueryService
│
├── secrets/
│   └── SecretProvider, SecretRotator, SecretCache
│
├── flags/
│   └── FeatureFlagProvider, FlagEvaluator, FlagTargetingRule
│
├── observability/
│   └── Logger, Tracer, MetricsRecorder, HealthChecker, RequestID
│
├── validation/
│   └── Validator, ValidationRule, ValidationResult, SanitizerChain
│
├── mailer/
│   └── MailProvider, MailMessage, MailTemplate, AttachmentStore
│
└── deployment/
    └── DeploymentAdapter, InfraTemplate, HealthProbe, RollbackStrategy
```

### 44.3 Contract Design Rules

```
RULE                                      ENFORCEMENT
────────────────────────────────────────────────────────────────────
Contracts have no implementation          Compile-time (interface only)
Contracts have no external imports        Code review + linter rule
Business logic imports contracts only     Linter rejects adapter imports in domain
Adapters import contracts to satisfy them Compile-time interface verification
Plugin KernelAPI is a contract            Plugin cannot reach kernel internals
Generated code has zero acthur/* imports  acthur generate verify --strict
```

---

## 45. Asynchronous Task Processing

### 45.1 Architecture

Queue jobs run inside the same binary process as the HTTP server, eliminating separate worker process management. The binary allocates goroutine/Tokio task pools intelligently — workers consume CPU during low-HTTP-traffic periods.

### 45.2 Job Lifecycle

```
Dispatch → Queue (NATS / Redis / Kafka)
                │
                ▼
         Worker Pool (N workers)
                │
       ┌────────┴──────────┐
       ▼ Success            ▼ Failure
  Complete + Cleanup    Retry with Exponential Backoff
                              │ (max retries exceeded)
                              ▼
                      Dead-Letter Queue (DLQ)
                              │
                      Admin Inspection Panel
                              │
                      Manual Replay / Archive / Discard
```

### 45.3 Queue Backends

```
NATS JetStream    → default; persistent, replicated, low-latency
Redis/Asynq       → Go-native; Redis as the queue broker
Kafka             → high-throughput; multi-consumer; large-scale deployments
In-process        → development default; Go channels / Tokio broadcast
```

### 45.4 Job Definition

```go
// Generated job skeleton — developer fills the Handle() body
type SendWelcomeEmailJob struct {
    UserID   string `json:"user_id"`
    TenantID string `json:"tenant_id"`
}

func (j *SendWelcomeEmailJob) Handle(ctx context.Context) error {
    user, err := userRepo.FindByID(ctx, j.UserID)
    if err != nil { return fmt.Errorf("fetch user: %w", err) }
    return notifications.Send(ctx, notification.Message{
        To:       user,
        Template: "welcome",
        Via:      []channel.Type{channel.Email},
    })
}

func (j *SendWelcomeEmailJob) Config() queue.JobConfig {
    return queue.JobConfig{
        Queue:   "notifications",
        Retries: 3,
        Backoff: queue.Exponential(time.Second, 2.0),
        Timeout: 30 * time.Second,
        DLQ:     "dead-letter",
    }
}
```

### 45.5 Scheduled Jobs (Cron)

```go
// Generated scheduled job — runs on cron schedule
type GenerateWeeklyReportJob struct{}

func (j *GenerateWeeklyReportJob) Handle(ctx context.Context) error {
    // ... generate and email report
    return nil
}

func (j *GenerateWeeklyReportJob) Schedule() scheduler.CronConfig {
    return scheduler.CronConfig{
        Expression:    "0 9 * * 1",   // every Monday at 9am
        Timeout:       5 * time.Minute,
        DistributedLock: true,        // prevents double-execution in multi-node
    }
}
```

### 45.6 CLI Commands

```bash
acthur queue:status             # show queue depth, DLQ count, worker status
acthur queue:flush <queue>      # flush a specific queue
acthur queue:retry <job-id>     # replay a DLQ job
acthur queue:drain              # graceful drain (finish in-flight, stop accepting)
```

---

## 46. Event Mesh & Domain Events

### 46.1 Inter-Module Communication Rule

Modules within the generated application communicate **exclusively** through typed domain events. Direct service-to-service method calls across module boundaries are rejected by the Acthur linter rule `no-cross-module-direct-call`. This enforces true modular decoupling without requiring a microservice split.

```go
// WRONG — rejected by linter (direct cross-module call)
// This code cannot appear in users/ if PostService is in posts/
func (s *UserService) DeleteUser(ctx context.Context, id string) error {
    s.postService.DeleteByAuthor(ctx, id)  // ✗ linter error
}

// CORRECT — emit an event, let posts module handle it
func (s *UserService) DeleteUser(ctx context.Context, id string) error {
    return s.events.Publish(ctx, UserDeletedEvent{UserID: id})  // ✓
}
```

### 46.2 Event Bus Drivers

```
DRIVER              SCOPE         LATENCY    DURABILITY   USE CASE
───────────────────────────────────────────────────────────────────────────
In-process channels  Single node   nanosec    None         Development, single-node
NATS JetStream       Multi-node    <1ms       Persistent   Distributed, at-least-once
Kafka               Multi-node    <5ms       Durable      High-throughput, event sourcing
```

The event bus **contract** remains identical regardless of driver. Switching from in-process to NATS is a config change, not a code change.

### 46.3 Domain Event Definition

```go
// Generated domain event — developer fills payload fields from manifest
type AppointmentBookedEvent struct {
    AppointmentID string    `json:"appointment_id"`
    VetID         string    `json:"vet_id"`
    OwnerID       string    `json:"owner_id"`
    TenantID      string    `json:"tenant_id"`
    StartTime     time.Time `json:"start_time"`
    BookedAt      time.Time `json:"booked_at"`
}

func (e AppointmentBookedEvent) EventName() string { return "appointment.booked" }
func (e AppointmentBookedEvent) AggregateID() string { return e.AppointmentID }
```

### 46.4 Event Handler

```go
// Generated event handler skeleton
type NotifyVetOnBookingHandler struct {
    notifications notifications.NotificationDispatcher
}

func (h *NotifyVetOnBookingHandler) Handle(ctx context.Context, event AppointmentBookedEvent) error {
    return h.notifications.Send(ctx, notification.Message{
        To:       event.VetID,
        Template: "appointment_booked",
        Data:     map[string]any{"appointment_id": event.AppointmentID},
        Via:      []channel.Type{channel.Email, channel.Push},
    })
}
```

---

## 47. Caching Architecture

### 47.1 Multi-Layer Cache Design

```
Service Request
      │
      ▼
 L1 Cache (In-Process)
 Go: BigCache | Rust: Moka
 Sub-microsecond; bounded heap; per-instance
      │ cache miss
      ▼
 L2 Cache (Distributed)
 Redis — sub-millisecond; shared across all binary instances
      │ cache miss
      ▼
 Database (authoritative source)
      │ populate L2 + L1
      ◄──────────────────────────────────
```

### 47.2 Cache Invalidation Strategies

```
TTL-based:       Simple time-based expiry. Default for read-heavy, low-mutation data.
                 Generated: cache.Set(ctx, key, value, 15*time.Minute)

Event-driven:    Domain events trigger targeted invalidation.
                 Generated: on UserUpdatedEvent → invalidate user:{id}
                 Preferred for mutation-sensitive data.

Manual:          cache.Invalidate(ctx, key) in service layer.
                 For complex invalidation scenarios.

Tag-based:       Related cache entries grouped under tags for bulk invalidation.
                 cache.InvalidateTag(ctx, "user:"+userID) clears all user-related keys.
```

### 47.3 Cache Configuration

```yaml
plugins:
  - name: cache
    config:
      l1:
        driver: bigcache        # bigcache (Go) | moka (Rust)
        max_size_mb: 256
        ttl_default: 5m
      l2:
        driver: redis
        ttl_default: 15m
        key_prefix: "vetangle:"
```

### 47.4 Cache-Aside Pattern (Generated)

Every generated repository method is wrapped with cache-aside logic:

```go
// Generated in internal/users/user.repository.go
func (r *UserRepository) FindByID(ctx context.Context, id string) (*User, error) {
    // Check L1 → L2 → DB
    if user, ok := r.cache.L1.Get(cacheKey(id)); ok {
        return user.(*User), nil
    }
    if user, ok := r.cache.L2.Get(ctx, cacheKey(id)); ok {
        r.cache.L1.Set(cacheKey(id), user, 5*time.Minute)
        return user.(*User), nil
    }
    user, err := r.db.WithContext(ctx).First(&User{}, "id = ?", id).Error
    if err != nil { return nil, err }
    r.cache.L2.Set(ctx, cacheKey(id), user, 15*time.Minute)
    r.cache.L1.Set(cacheKey(id), user, 5*time.Minute)
    return user, nil
}
```

---

## 48. Real-Time Broadcasting

### 48.1 WebSocket Hub Architecture

```
Client WebSocket Connections
           │
           ▼
    WebSocket Hub (in-process)
           │
       Room Manager
      /             \
  Room A           Room B
  Subscriber       Subscriber
  Pool             Pool

  (If multi-node deployment)
           │
     Redis Pub/Sub
     Adapter Layer
           │
     Other Binary
     Instances
```

### 48.2 Generated Broadcast API

```go
// Server-side broadcasting — generated by websocket plugin
broadcast.ToRoom(ctx, "appointment-room-123", AppointmentStatusEvent{...})
broadcast.ToUser(ctx, "user-456",              NotificationEvent{...})
broadcast.ToTenant(ctx, "tenant-789",          TenantAlertEvent{...})
broadcast.ToAll(ctx,                            SystemAnnouncementEvent{...})
```

### 48.3 Client Integration (Generated TypeScript)

```typescript
// Generated in packages/sdk/src/realtime.ts
const client = new AcThurClient({ baseURL: 'https://api.vetangle.com' })

client.realtime.subscribe('appointment-updates', (event: AppointmentStatusEvent) => {
    updateAppointmentStatus(event.appointmentId, event.status)
})

client.realtime.join('appointment-room-123')
```

### 48.4 Multi-Node Scaling

When deployed with multiple binary instances, the Redis pub/sub adapter broadcasts messages across all instances. The same `broadcast.ToUser()` call works identically whether there is one instance or ten — the adapter handles fan-out transparently.

```yaml
plugins:
  - name: websocket
    config:
      driver: redis          # redis (multi-node) | in-process (single node)
      max_connections: 10000
      heartbeat_interval: 30s
```

---

## 49. Notification System

### 49.1 Unified Dispatch Interface

A single call routes notifications across multiple channels simultaneously:

```go
// Generated notification dispatch
notifications.Send(ctx, notification.Message{
    To:       user,
    Template: "appointment_booked",
    Data:     map[string]any{
        "vet_name":   vet.Name,
        "start_time": appointment.StartTime,
    },
    Via: []channel.Type{channel.Email, channel.InApp, channel.Push},
})
```

### 49.2 Email Providers

```
adapters/notifications/email/
├── resend/      Modern API-first email delivery; best DX
├── sendgrid/    Enterprise email at scale
├── ses/         AWS Simple Email Service; cost-effective at volume
└── mailgun/     Developer-friendly transactional email
```

### 49.3 SMS Providers

```
adapters/notifications/sms/
├── twilio/      Industry standard; global reach
└── vonage/      Vonage — Competitive alternative; Africa-strong coverage
                 (Essential for Nigerian market — Airtel, MTN, Glo, 9mobile)
```

### 49.4 Push Notification Providers

```
adapters/notifications/push/
├── fcm/         Firebase Cloud Messaging — Android + iOS
├── apns/        Apple Push Notification Service — iOS native
└── onesignal/   Cross-platform unified push; A/B testing built-in
```

### 49.5 In-App Notifications

Generated real-time in-app notification panel using the WebSocket broadcasting layer. Persisted to database for notification history and read/unread state.

### 49.6 Notification Templates

Templates at `backend/notifications/templates/` use Go `text/template` with front-matter metadata. The generator creates template stubs for every event declared in `acthur.yml`:

```
templates/
├── welcome.html              # HTML email template
├── welcome.txt               # Plain-text fallback
├── appointment_booked.html
├── appointment_booked.push   # Push notification payload
└── appointment_booked.sms    # SMS body template
```

---

## 50. Payments System

### 50.1 Payment Gateway Contract

```go
type PaymentGateway interface {
    CreateCheckoutSession(ctx context.Context, opts CheckoutOptions) (*Session, error)
    CapturePayment(ctx context.Context, sessionID string) (*Payment, error)
    RefundPayment(ctx context.Context, paymentID string, amount Decimal) (*Refund, error)
    CreateSubscription(ctx context.Context, opts SubscriptionOptions) (*Subscription, error)
    CancelSubscription(ctx context.Context, subscriptionID string) error
    HandleWebhook(ctx context.Context, payload []byte, signature string) (WebhookEvent, error)
    GetCustomer(ctx context.Context, customerID string) (*Customer, error)
}
```

### 50.2 Payment Gateway Adapters

```
adapters/payments/
├── stripe/        Global standard; subscriptions, one-time, metered billing
├── paystack/      Africa-first; Nigeria, Ghana, Kenya, South Africa
└── flutterwave/   Pan-African; Nigeria, Ghana, Kenya, Uganda, Rwanda + global
```

Paystack and Flutterwave are first-class adapters — not afterthoughts. The Nigerian and pan-African developer market is a primary target for Acthur.

### 50.3 Generated Payment Endpoints

```
POST   /payments/checkout          # Create checkout session
GET    /payments/success            # Post-payment success handler
GET    /payments/cancel             # Cancelled payment handler
POST   /payments/webhooks           # Webhook receiver (signature-verified)
GET    /payments/subscriptions/:id  # Subscription status
DELETE /payments/subscriptions/:id  # Cancel subscription
GET    /billing/invoices            # Invoice history
```

### 50.4 Subscription Billing

```yaml
# acthur.yml
plugins:
  - name: payments
    config:
      gateway: stripe          # stripe | paystack | flutterwave
      currency: NGN            # default currency
      plans:
        - id: starter
          name: Starter
          price: 5000          # kobo / cents (smallest currency unit)
          interval: monthly
          features: [feature_a, feature_b]
        - id: pro
          name: Professional
          price: 15000
          interval: monthly
          features: [feature_a, feature_b, feature_c]
```

---

## 51. Validation System

### 51.1 Per-Runtime Validators

**Go Validators:**
- `go-playground/validator/v10` — Struct tag-based, 70+ built-in rules, custom rule registration
- `ozzo-validation` — Programmatic, chainable rules; better for complex conditional logic

**Rust Validators:**
- `validator` crate — Derive macro-based struct validation; integrates with Serde deserialization

### 51.2 Manifest-to-Validation Code

Field definitions in `acthur.yml` directly generate validation rules:

```yaml
# acthur.yml domain field
- name: email
  type: email
  unique: true
  required: true
```

**Generated Go DTO:**
```go
type CreateUserInput struct {
    Email string `json:"email" validate:"required,email,max=255"`
    Name  string `json:"name"  validate:"required,min=1,max=100"`
    Role  string `json:"role"  validate:"required,oneof=admin vet agent support user"`
}
```

**Generated Rust DTO:**
```rust
#[derive(Deserialize, Validate)]
pub struct CreateUserInput {
    #[validate(required, email, length(max = 255))]
    pub email: String,
    #[validate(required, length(min = 1, max = 100))]
    pub name: String,
}
```

### 51.3 Validation Response Format

Invalid requests return HTTP 422 with structured field-level errors:

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Request validation failed",
    "details": [
      { "field": "email",    "message": "Must be a valid email address" },
      { "field": "password", "message": "Must be at least 8 characters" }
    ]
  },
  "request_id": "01HXYZ..."
}
```

### 51.4 Custom Validation Rules

Developer-defined validation rules go in the extension zone (outside `@acthur:generated` markers) and are preserved across regeneration:

```go
// user.validator.go — extension zone
func validateNigerianPhone(fl validator.FieldLevel) bool {
    phone := fl.Field().String()
    return regexp.MustCompile(`^(\+234|0)[789][01]\d{8}$`).MatchString(phone)
}
```

---

## 52. Admin System

### 52.1 Generated Admin Dashboard

The admin dashboard is a separate frontend application at `frontend/admin-panel/`. It is authentication-protected (requires `admin` role) and uses the generated TypeScript SDK.

### 52.2 Auto-Generated Features Per Domain Model

For every domain declared in `acthur.yml`, the admin panel generates:

```
List view:         Sortable, filterable, paginated table with inline actions
Detail view:       Full record display with relation navigation
Create form:       Type-safe form with all field validations
Edit form:         Pre-populated edit form with optimistic updates
Delete/Archive:    Confirmation dialog with soft-delete or hard-delete
Bulk operations:   Select multiple, bulk delete, bulk status change
Export:            CSV and JSON export of filtered results
```

### 52.3 Admin-Specific Capabilities

```
Tenant management     → Create, suspend, configure, and provision tenants
User management       → Role assignment, account status management
                        Impersonation (with mandatory audit log entry)
Feature flags UI      → Toggle flags, configure targeting rules, percentage rollout
Job queue monitor     → View queued/running/failed/DLQ jobs; manual replay; flush
Scheduler monitor     → Job history, next execution times, manual trigger
System health         → Prometheus metrics dashboard, health probe statuses
Audit log viewer      → Filterable, searchable, immutable audit trail
API keys management   → Create, rotate, revoke API keys for integrations
```

### 52.4 Admin Security

```
All admin routes:    auth: required + roles: [admin]
Impersonation:       Creates mandatory audit_log entry before session swap
Audit log:           INSERT-only — admin cannot delete or modify audit records
Destructive actions: Two-step confirmation with typed confirmation text
API keys:            Shown once on creation, stored as HMAC hash
```

---

## 53. SDK & Client Generation

### 53.1 TypeScript SDK

The generated TypeScript SDK provides fully typed, tree-shakeable access to all API endpoints:

```typescript
import { AcThurClient } from '@vetangle/sdk'

const client = new AcThurClient({
    baseURL: 'https://api.vetangle.com',
    auth: { accessToken: userSession.token }
})

// Fully typed — IntelliSense for all fields, params, responses
const appointment = await client.appointments.book({
    vetId:     'vet_01HABC...',
    petId:     'pet_01HDEF...',
    startTime: '2025-09-15T10:00:00Z',
})

// Pagination
const vets = await client.vets.list({ available: true, page: 1, limit: 20 })

// Real-time subscriptions
client.realtime.subscribe('appointment-updates', (event: AppointmentStatusEvent) => {
    console.log('Appointment status changed:', event)
})
```

Generated from contracts — every endpoint, every type, every error code. TypeScript types are exact mirrors of the backend contract definitions.

### 53.2 Dart / Flutter SDK

When `mobile:flutter` is a graph node, a Dart SDK is generated alongside the TypeScript SDK:

```dart
// Generated packages/sdk-dart/lib/vetangle_sdk.dart
final client = VetAngleClient(
  baseUrl: 'https://api.vetangle.com',
  accessToken: session.accessToken,
);

final appointment = await client.appointments.book(
  BookAppointmentInput(vetId: vetId, petId: petId, startTime: startTime),
);
```

### 53.3 OpenAPI Spec Generation

Acthur generates an `openapi.yaml` from contracts during the pipeline. The spec is served at `/api/docs` in development mode via embedded Swagger UI.

```bash
acthur generate docs --target openapi    # regenerate openapi.yaml
acthur generate sdk --lang python        # generate Python client from spec
acthur generate sdk --lang java          # generate Java client from spec
acthur generate sdk --lang swift         # generate Swift client from spec
```

### 53.4 SDK Versioning

The SDK package version tracks the contract version. When a contract is bumped to v2, the SDK generates `client.usersV2.*` methods alongside existing `client.users.*`, allowing frontend teams to migrate incrementally.

---

## 54. Competitive Positioning

### 54.1 Landscape Analysis

| Framework | Runtime | DX | Performance | Multi-Runtime | Memory | Full-Stack |
|---|---|---|---|---|---|---|
| Laravel | PHP | ★★★★★ | ★★☆☆☆ | ✗ | ~80 MB | Partial |
| NestJS | Node.js | ★★★★☆ | ★★★☆☆ | ✗ | ~60 MB | Partial |
| Django | Python | ★★★★☆ | ★★☆☆☆ | ✗ | ~40 MB | Partial |
| Chi / Axum | Go/Rust | ★★☆☆☆ | ★★★★★ | ✗ | ~3 MB | ✗ |
| Encore.dev | Go | ★★★★☆ | ★★★★☆ | ✗ | ~5 MB | Partial |
| **Acthur** | **Go/Rust/Node/Python** | **★★★★★** | **★★★★★** | **✓** | **< 5 MB** | **✓** |

### 54.2 Key Differentiators

**vs. Laravel:** Same productivity tier (full-stack scaffold, auth, RBAC, tenancy, admin), radically lower runtime memory (< 5 MB vs. ~80 MB), compile-time type safety, multi-runtime.

**vs. NestJS:** No V8 GC overhead. Stronger type guarantees (Rust target). Runtime-agnostic. True single-binary execution. 10–20× lower memory at equivalent load.

**vs. Django:** Python adapter included but Go/Rust available for performance-critical services. Multi-runtime per project. Generated code compiles to static binary.

**vs. Encore.dev:** Not locked to a single cloud provider. MIT-licensed, self-hostable. Multi-runtime (Encore is Go-only). Supports Dokploy, Coolify, Fly, Railway, Render, K8s, Docker.

**vs. Chi/Axum alone:** Full capability ecosystem (auth, queues, RBAC, tenancy, admin, payments, observability) generated from a manifest. Days of boilerplate become minutes.

### 54.3 Performance Targets

```
Go/Fiber (single core, echo benchmark):    > 50,000 RPS
Rust/Axum (single core, echo benchmark):   > 100,000 RPS
Memory at idle (Go + Postgres + Redis):    < 15 MB
Memory at idle (Rust + Postgres + Redis):  < 8 MB
Docker image size (Go, distroless):        ~12 MB
Docker image size (Rust, distroless):      ~6 MB
```

---

## 55. Success Metrics

### 55.1 Performance Benchmarks (Target at v1.0)

```
API throughput:
  Go/Fiber (single core):      > 50,000 RPS
  Rust/Axum (single core):     > 100,000 RPS

Memory at idle:
  Go/Fiber + Postgres + Redis: < 15 MB
  Rust/Axum + Postgres + Redis: < 8 MB

Docker image size:
  Go (distroless):             ~12 MB
  Rust (distroless):           ~6 MB

Binary size:
  Acthur CLI binary:           < 20 MB
  Kernel overhead per app:     < 500 KB
  Kernel init time:            < 200 ms
```

### 55.2 Developer Experience Targets

```
acthur new → running dev server:      < 60 seconds (after deps cached)
acthur generate from-contract:        < 5 seconds per contract
Incremental code generation:          < 3 seconds (for single-domain change)
CI pipeline (test + build + Docker):  < 5 minutes
From acthur new to production-ready API: < 10 minutes
Boilerplate written by developer:     < 50 lines to get to first API endpoint
```

### 55.3 Ecosystem Goals (12-Month)

```
Community adapter plugins:       20+
Starter templates:               10+ (5 free, 5+ paid)
Documentation (acthur.dev):      Complete with interactive examples
Plugin marketplace:              Live with listing and install
Community (Discord):             Active support channel
GitHub stars:                    5,000+ at end of year 1
First-party plugins:             Stripe, Paystack, Flutterwave, Resend, Twilio
```

---

## 56. Architectural Decision Records (ADRs)

ADRs document *why* key architectural decisions were made, not just *what* was decided.

### ADR-001 — Go as CLI and Kernel Language

**Status:** Accepted

**Decision:** The Acthur CLI and kernel are implemented in Go.

**Rationale:**
- Single-binary cross-platform distribution (macOS, Linux, Windows) without a runtime dependency
- Fast compile times (~seconds vs. minutes for Rust)
- Excellent standard library (HTTP, templates, testing)
- `go install` makes distribution trivial
- Sufficient performance for a CLI tool
- Team familiarity

**Rejected alternatives:**
- Rust: Better performance, but compile times and complexity are disproportionate for a CLI tool
- Node.js: Requires Node.js runtime on developer machines; large binary with bundling
- Python: Requires Python interpreter; slow startup for CLI

---

### ADR-002 — YAML as the Manifest Format (acthur.yml)

**Status:** Accepted

**Decision:** `acthur.yml` (YAML) is the single source of truth, not a code-first schema.

**Rationale:**
- Human-readable without programming language knowledge
- Version-control diffable (line-level changes are meaningful)
- Familiar to developers from Docker Compose, Kubernetes, GitHub Actions
- Amenable to IDE autocomplete via JSON Schema
- Lower barrier to understanding for non-Go/Rust developers

**Rejected alternatives:**
- Go structs as config (code-first): Requires language knowledge; not framework-agnostic
- TypeScript config: Requires Node.js for config parsing
- HCL (Terraform): Less familiar; more verbose for complex schemas
- TOML: Less expressive for nested structures

---

### ADR-003 — Modular Monolith as Default Architecture

**Status:** Accepted (with optional microservices path)

**Decision:** Generated applications default to event-driven modular monolith. Microservice extraction is an optional, deliberate choice.

**Rationale:**
- Simpler operational model (one process, one deployment, one log stream)
- Easier local development (no service mesh, no cross-service debugging)
- Lower infrastructure cost at early scale
- Module boundaries via domain events enable future extraction without architectural redesign
- Premature microservices is a well-documented anti-pattern

**Evolution from initial:** The initial PRD declared "not a microservices orchestrator." Our PRD makes microservices an equal option — the graph model handles both without special-casing. The modular monolith remains the recommended default.

---

### ADR-004 — Casbin for RBAC Enforcement

**Status:** Accepted

**Decision:** RBAC policies are enforced by Casbin (Go) / Casbin-RS (Rust).

**Rationale:**
- PERM model (Policy, Effect, Request, Matchers) is expressive and well-tested
- Role inheritance chains via g2 (group-to-group) relationships
- Database-backed policy store with in-memory enforcement cache
- Hot-updatable policies without process restart
- Battle-tested in production (50M+ downloads)
- Supports tenancy-aware domain scoping natively

**Rejected alternatives:**
- Custom RBAC: High implementation cost; security audit surface
- Open Policy Agent (OPA): Rego language adds complexity; separate process
- Oso: Less mature ecosystem

---

### ADR-005 — Single-Binary Execution Model

**Status:** Accepted

**Decision:** HTTP server, queue workers, cron scheduler, and pub/sub engine all run in one OS process.

**Rationale:**
- Eliminates separate worker process management (no `Procfile`, no `supervisord`)
- Shared in-process cache (L1 BigCache / Moka) without inter-process serialisation
- Single deployment artifact, single health check endpoint
- Goroutines (Go) and Tokio tasks (Rust) provide efficient co-scheduling
- Lower infrastructure cost at small-to-medium scale
- Simplifies graceful shutdown coordination

**Trade-off:** At very high scale, queue workers may need horizontal scaling independent of HTTP servers. Acthur addresses this by allowing the `role` field on service nodes (`role: queue-worker`) to deploy workers as separate scaled instances while sharing the same binary.

---

### ADR-006 — Incremental Code Generation Over Full Regeneration

**Status:** Accepted

**Decision:** Acthur uses `@acthur:generated:start/end` region markers and a Diff Engine to regenerate only affected code regions.

**Rationale:**
- Full regeneration on every manifest change would destroy developer customisations
- Region markers are a proven pattern (Django migrations, Rails scaffolding)
- The Diff Engine computes minimal changesets, making generation fast (< 3 seconds)
- Developers can safely run `acthur generate` without fear of overwriting their code
- `system.json` provides a stable, version-controlled baseline for diff computation

**Trade-off:** Region marker syntax is an additional concept developers must understand. Mitigated by clear documentation and IDE highlighting in the LSP extension.

---

### ADR-007 — GraphQL as First-Class Transport (Evolution from Initial)

**Status:** Accepted (Overrides initial "Not GraphQL-native" stance)

**Decision:** GraphQL is a first-class transport adapter equal in status to HTTP REST and gRPC.

**Rationale:**
- Modern frontend teams expect GraphQL — especially for complex dashboards and mobile apps
- The contract layer abstracts transport — adding GraphQL required no architectural change
- Code generation works the same: contract → gqlgen / mercurius / async-graphql / strawberry
- Breaking change detection works identically for GraphQL schemas as for REST contracts
- N+1 DataLoader generation makes GraphQL production-safe out of the box

**Evolution rationale:** The initial PRD said "Not GraphQL-native" because it was Go-only and REST-first. The expansion to polyglot multi-runtime made GraphQL a natural addition.


---

*End of Document*

---

*Acthur — Runtime Graph Operating System with Pluggable Infrastructure Nodes*
*PRD Version 3.0 — Open Source (MIT License) — github.com/acthur/acthur*
