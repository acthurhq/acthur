# Acthur

Acthur is a runtime graph operating system: it models an application's backend services, frontends, and infrastructure as nodes in one live directed acyclic graph, and derives execution, ordering, health, and topology from that graph.

## Language

### Graph model

**Node**:
A vertex in the graph — one entry under `graph.nodes` in `acthur.yml`. Every node is exactly one of four kinds: Service, Infra, Plugin, or Contract.

**Edge**:
A directed, typed relationship between two nodes (e.g. `depends_on`, `data_flow`, `proxied_through`). Edges, not file layout, are the source of truth for how the system fits together.

Node types divide into **authored** (`service`, `infra` — written under `graph.nodes`) and **materialized** (`proxy`, `contract`, `plugin` — injected by the kernel). A materialized type is never hand-declared.

**Service node**:
A node that *runs* as a process: api, web, worker, cron.

**Infra node**:
A node that *exists* as a backing resource: postgres, redis, minio, nats.

**Kernel node**:
A built-in node that Acthur injects into the graph itself rather than the user declaring it in `acthur.yml`. Its adapter is namespaced `kernel:*`.
_Avoid_: implicit node, phantom node, magic node.

**Proxy**:
The single kernel node every routed service points at via a `proxied_through` edge. It is the dev reverse proxy (one port, default 4000) and is materialized automatically as a kernel node (adapter `kernel:proxy`) — never declared by the user.
_Avoid_: gateway, router, ingress.

**Contract**:
A typed interface on a `data_flow` edge. Authors reference it as a file path on the edge (`contracts: [...]`); a contract is never hand-declared as a node. The graph-native contract node, and its `satisfies`/`consumes` edges, are *materialized* by the Contract Engine (Phase 4), not authored.

**Materialized node / edge**:
A node or edge the kernel injects into the graph from a simpler authored source, rather than one the user writes in `acthur.yml`. The proxy kernel node and (later) contract nodes are materialized.
_Avoid_: derived node, synthetic node, virtual node.

**Cross-repo node**:
A node whose `source` is a remote git URL rather than a local path, so the graph can span repositories. The graph is location-agnostic; co-location of code is irrelevant to the model. (`source` defaults to `./`; remote fetching is a later-phase concern.)

### Adapters

**Adapter**:
An implementation bridge from an abstract graph node to one concrete runtime resource — a framework, a database, a cache, an external service. It exposes only the **capabilities** that resource actually has, and nothing else. It has zero knowledge of plugins, contracts, or other adapters; the kernel calls adapters, adapters never call back.

**Capability**:
A narrow behavior an adapter may expose, modeled as an optional Go interface the kernel type-asserts against — `Scaffolder`, `Runnable`, `Containerized`, and later `Migratable`, `Deployable`. An adapter composes the capabilities its resource genuinely has: `go:fiber` is `Scaffolder` + `Runnable`; `db:postgres` is `Containerized`. The kernel branches on the capability a node's adapter has, never on the node's type. An adapter's capability set is *derived from* the interfaces it satisfies, so it cannot drift from actual behavior.
_Avoid_: feature, trait, mixin.

**Adapter key**:
An adapter's unique name in `namespace:name` form (`go:fiber`, `rust:axum`, `db:postgres`, `kernel:proxy`). The **namespace** (before the colon) names the runtime or resource category; the **name** identifies the specific framework or resource. It is the value of a node's `adapter:` field and the registry lookup key.
_Avoid_: driver, plugin, backend.

**Kernel primitive**:
A `kernel:*` key (e.g. `kernel:proxy`) attached to a materialized kernel node. Despite sharing the adapter-key form, kernel primitives are *not* adapters: they are provided by the kernel itself, never selected by a user, never placed in the registry, and exempt from registry resolution and node-type/category validation.
_Avoid_: kernel adapter, built-in adapter.

**Resolver**:
The abstraction the graph engine uses to ask whether an adapter key is known. The engine depends on this interface, never on the discovery mechanism behind it (the global registry, and later plugin loaders or remote catalogs). An unknown key surfaces as a validation error, not a build error.
_Avoid_: registry, catalog, loader (those are *implementations* of a Resolver, not the concept).

### Correctness tiers

**Parse error**:
A schema-level failure that stops loading: malformed YAML, unknown version, missing required scalar. Halts immediately.

**Build error**:
A failure to *construct* the graph: an edge references a node that does not exist, or a kernel node cannot be materialized. Halts immediately, because no graph exists yet. Distinct from a validation error — a cycle is *not* a build error, because a cyclic graph is still constructable.

**Validation error**:
A semantic fault in a graph that *was* constructed — cycle, orphan, missing contract, unresolved adapter. Collected (not fatal mid-pass) so every fault surfaces at once, each carrying a severity (`error` | `warning` | `info`).

### Lifecycle

**Setup phase**:
The single-threaded phase in which topology is built, kernel nodes are materialized, and plugins register. The only phase in which the graph's structure may change.

**Seal** (`Freeze`):
The one-way transition that ends the setup phase and makes topology immutable. Structural mutation after seal is a programming error.
_Avoid_: lock, close, finalize.

**Runtime phase**:
Everything after seal. Topology is fixed and read lock-free; only per-node state changes.

**State subscription**:
The mechanism by which consumers (e.g. the monitor) observe node state changes. Delivery is ordered *per node* — a node's own transitions are never observed out of order — but makes no promise about ordering *across* nodes, so parallel execution stays possible. Subscribers must not block the setter.
