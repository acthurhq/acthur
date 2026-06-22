# Two-phase graph lifecycle: mutable setup, then sealed runtime

**Status:** accepted

The graph has exactly two lifecycle phases, and topology may only change in the first. This makes the runtime phase lock-free and race-free by construction, rather than by defensive locking.

- **Setup phase** (single-threaded): `Build` constructs nodes/edges, the kernel materializes built-in nodes (proxy now; contract/plugin nodes later — see [[0002-three-tier-graph-validation]]), and plugins register via `AddNodeToGraph`/`AddEdgeToGraph`. All structural mutation happens here. Because it is single-threaded, no lock is needed.
- **Seal** (`Freeze()`): once setup completes and the graph passes `Validate`, topology is sealed. Any structural mutation after seal is a programming error (panic), not a tolerated race.
- **Runtime phase** (concurrent): traversal and topology reads are lock-free because topology is immutable. The only mutable runtime data is per-node `NodeState`, which remains guarded by its own mutex.

## Why not a fully-locked mutable graph

A `RWMutex` around the node/edge/adjacency maps would allow topology mutation at any time, but adds lock complexity and deadlock surface for a capability a dev/deploy orchestrator does not need: by the time execution starts, the topology is known and settled. Sealing matches the `Parse → Build → Validate → Execute` spine — topology is final before execution begins.
