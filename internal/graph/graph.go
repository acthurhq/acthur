// Package graph is the kernel's brain.
// It builds a live directed acyclic graph from the acthur.yml config,
// validates the graph structure, and provides all traversal operations
// needed by the dev runtime, deploy engine, and plugin system.
package graph

import (
	"fmt"
	"sync"

	"github.com/acthur/acthur/internal/config"
)

// ---------------------------------------------------------------------------
// Node
// ---------------------------------------------------------------------------

// NodeState represents the live runtime state of a graph node.
type NodeState string

const (
	StatePending  NodeState = "pending"
	StateStarting NodeState = "starting"
	StateHealthy  NodeState = "healthy"
	StateDegraded NodeState = "degraded"
	StateStopped  NodeState = "stopped"
	StateFailed   NodeState = "failed"
)

// Node is a vertex in the Acthur graph.
// It corresponds to one entry in acthur.yml → graph.nodes.
type Node struct {
	ID      string
	Type    config.NodeType
	Adapter string
	Port    int
	Role    config.NodeRole
	DevURL  string
	Config  config.NodeConfig

	// Runtime state — updated by the process manager and health checker
	state NodeState
	mu    sync.RWMutex
}

func (n *Node) State() NodeState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state
}

func (n *Node) SetState(s NodeState) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.state = s
}

func (n *Node) IsService() bool  { return n.Type == config.NodeTypeService }
func (n *Node) IsInfra() bool    { return n.Type == config.NodeTypeInfra }
func (n *Node) IsPlugin() bool   { return n.Type == config.NodeTypePlugin }

// ---------------------------------------------------------------------------
// Edge
// ---------------------------------------------------------------------------

// Edge is a directed relationship between two nodes.
type Edge struct {
	From      string
	To        string
	Type      config.EdgeType
	Contracts []string
	Transport config.EdgeTransport
	Events    []string
}

// ---------------------------------------------------------------------------
// Graph
// ---------------------------------------------------------------------------

// Graph is the live relational model of the entire application system.
// Every decision the Acthur kernel makes is derived from this structure.
type Graph struct {
	nodes    map[string]*Node
	edges    []*Edge
	adjOut   map[string][]*Edge // from → edges
	adjIn    map[string][]*Edge // to   → edges

	// State subscription — plugins and monitor subscribe to state changes
	subscribers []func(nodeID string, state NodeState)
	subMu       sync.RWMutex
}

// ---------------------------------------------------------------------------
// Builder
// ---------------------------------------------------------------------------

// Build constructs a Graph from the parsed acthur.yml config.
// Returns a validated, traversal-ready Graph or an error.
func Build(cfg *config.Config) (*Graph, error) {
	g := &Graph{
		nodes:  make(map[string]*Node),
		adjOut: make(map[string][]*Edge),
		adjIn:  make(map[string][]*Edge),
	}

	// Build nodes
	for id, nc := range cfg.Graph.Nodes {
		devURL := nc.DevURL
		if devURL == "" && nc.Type == config.NodeTypeService {
			// Auto-generate .test subdomain from node id and project name
			devURL = id + "." + cfg.Dev.Domain
		}

		g.nodes[id] = &Node{
			ID:      id,
			Type:    nc.Type,
			Adapter: nc.Adapter,
			Port:    nc.Port,
			Role:    nc.Role,
			DevURL:  devURL,
			Config:  nc,
			state:   StatePending,
		}
	}

	// Build edges
	for _, ec := range cfg.Graph.Edges {
		edge := &Edge{
			From:      ec.From,
			To:        ec.To,
			Type:      ec.Type,
			Contracts: ec.Contracts,
			Transport: ec.Transport,
			Events:    ec.Events,
		}
		g.edges = append(g.edges, edge)
		g.adjOut[ec.From] = append(g.adjOut[ec.From], edge)
		g.adjIn[ec.To] = append(g.adjIn[ec.To], edge)
	}

	// Detect cycles in depends_on edges
	if err := g.detectCycle(); err != nil {
		return nil, err
	}

	return g, nil
}

// ---------------------------------------------------------------------------
// Accessors
// ---------------------------------------------------------------------------

// Node returns a node by ID. Returns nil if not found.
func (g *Graph) Node(id string) *Node {
	return g.nodes[id]
}

// Nodes returns all nodes as a slice.
func (g *Graph) Nodes() []*Node {
	ns := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		ns = append(ns, n)
	}
	return ns
}

// NodesByType returns all nodes of a given type.
func (g *Graph) NodesByType(t config.NodeType) []*Node {
	var result []*Node
	for _, n := range g.nodes {
		if n.Type == t {
			result = append(result, n)
		}
	}
	return result
}

// Edges returns all edges as a slice.
func (g *Graph) Edges() []*Edge {
	return g.edges
}

// EdgesFrom returns all edges where From == nodeID.
func (g *Graph) EdgesFrom(nodeID string) []*Edge {
	return g.adjOut[nodeID]
}

// EdgesTo returns all edges where To == nodeID.
func (g *Graph) EdgesTo(nodeID string) []*Edge {
	return g.adjIn[nodeID]
}

// EdgesOfType returns all edges of a specific type.
func (g *Graph) EdgesOfType(t config.EdgeType) []*Edge {
	var result []*Edge
	for _, e := range g.edges {
		if e.Type == t {
			result = append(result, e)
		}
	}
	return result
}

// Edge returns the first edge between from and to with the given type.
// Returns nil if not found.
func (g *Graph) Edge(from, to string, t config.EdgeType) *Edge {
	for _, e := range g.adjOut[from] {
		if e.To == to && e.Type == t {
			return e
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Traversal Operations
// ---------------------------------------------------------------------------

// StartupOrder returns nodes in topological order based on depends_on edges.
// Infra nodes come before service nodes. Within the same level, ordering is
// stable (alphabetical by node ID).
func (g *Graph) StartupOrder() []*Node {
	return g.topoSort(config.EdgeDependsOn)
}

// ShutdownOrder returns the reverse of StartupOrder.
func (g *Graph) ShutdownOrder() []*Node {
	order := g.StartupOrder()
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// DependenciesOf returns all direct depends_on targets of a node.
func (g *Graph) DependenciesOf(nodeID string) []*Node {
	var result []*Node
	for _, e := range g.adjOut[nodeID] {
		if e.Type == config.EdgeDependsOn {
			if n := g.nodes[e.To]; n != nil {
				result = append(result, n)
			}
		}
	}
	return result
}

// DependentsOf returns all nodes that depend on the given node.
func (g *Graph) DependentsOf(nodeID string) []*Node {
	var result []*Node
	for _, e := range g.adjIn[nodeID] {
		if e.Type == config.EdgeDependsOn {
			if n := g.nodes[e.From]; n != nil {
				result = append(result, n)
			}
		}
	}
	return result
}

// PropagateFrom returns all nodes downstream in data_flow from nodeID.
// Used to cascade hot-reload notifications and contract change updates.
func (g *Graph) PropagateFrom(nodeID string) []*Node {
	visited := map[string]bool{nodeID: true}
	var result []*Node
	var walk func(id string)
	walk = func(id string) {
		for _, e := range g.adjIn[id] {
			if e.Type == config.EdgeDataFlow && !visited[e.From] {
				visited[e.From] = true
				if n := g.nodes[e.From]; n != nil {
					result = append(result, n)
				}
				walk(e.From)
			}
		}
	}
	walk(nodeID)
	return result
}

// AffectedByFailure returns all nodes that are affected if nodeID fails.
// Traverses depends_on edges upward from the failing node.
func (g *Graph) AffectedByFailure(nodeID string) []*Node {
	visited := map[string]bool{nodeID: true}
	var result []*Node
	var walk func(id string)
	walk = func(id string) {
		for _, e := range g.adjIn[id] {
			if e.Type == config.EdgeDependsOn && !visited[e.From] {
				visited[e.From] = true
				if n := g.nodes[e.From]; n != nil {
					result = append(result, n)
				}
				walk(e.From)
			}
		}
	}
	walk(nodeID)
	return result
}

// ProxiedNodes returns all nodes that should be routed through the proxy.
func (g *Graph) ProxiedNodes() []*Node {
	var result []*Node
	seen := map[string]bool{}
	for _, e := range g.edges {
		if e.Type == config.EdgeProxiedThrough && !seen[e.From] {
			if n := g.nodes[e.From]; n != nil {
				result = append(result, n)
				seen[e.From] = true
			}
		}
	}
	return result
}

// ContractFor returns the first contract on a data_flow edge from → to.
func (g *Graph) ContractFor(from, to string) string {
	e := g.Edge(from, to, config.EdgeDataFlow)
	if e == nil || len(e.Contracts) == 0 {
		return ""
	}
	return e.Contracts[0]
}

// MigratesEdges returns all migrates edges (service → infra ownership).
func (g *Graph) MigratesEdges() []*Edge {
	return g.EdgesOfType(config.EdgeMigrates)
}

// ---------------------------------------------------------------------------
// State Management
// ---------------------------------------------------------------------------

// SetState updates the runtime state of a node and notifies subscribers.
func (g *Graph) SetState(nodeID string, state NodeState) {
	n := g.nodes[nodeID]
	if n == nil {
		return
	}
	n.SetState(state)
	g.notifySubscribers(nodeID, state)
}

// GetState returns the current state of a node.
func (g *Graph) GetState(nodeID string) NodeState {
	n := g.nodes[nodeID]
	if n == nil {
		return StateFailed
	}
	return n.State()
}

// AllHealthy returns true if every service and infra node is healthy.
func (g *Graph) AllHealthy() bool {
	for _, n := range g.nodes {
		if n.Type == config.NodeTypeService || n.Type == config.NodeTypeInfra {
			if n.State() != StateHealthy {
				return false
			}
		}
	}
	return true
}

// Subscribe registers a callback for node state changes.
// Used by the monitor and plugin system.
func (g *Graph) Subscribe(fn func(nodeID string, state NodeState)) {
	g.subMu.Lock()
	defer g.subMu.Unlock()
	g.subscribers = append(g.subscribers, fn)
}

func (g *Graph) notifySubscribers(nodeID string, state NodeState) {
	g.subMu.RLock()
	defer g.subMu.RUnlock()
	for _, fn := range g.subscribers {
		go fn(nodeID, state)
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// Validate runs all structural validation rules on the graph.
// Returns a slice of human-readable error strings.
// This is called after Build() and can also be called standalone by
// 'acthur graph validate'.
func (g *Graph) Validate() []ValidationError {
	var errs []ValidationError

	// Rule: No orphan nodes (every node must have at least one edge)
	for id := range g.nodes {
		if len(g.adjOut[id]) == 0 && len(g.adjIn[id]) == 0 {
			errs = append(errs, ValidationError{
				Node:    id,
				Rule:    "orphan-node",
				Message: fmt.Sprintf("node %q has no edges — connect it to the graph or remove it", id),
				Fix:     fmt.Sprintf("add an edge involving node %q to acthur.yml graph.edges", id),
			})
		}
	}

	// Rule: data_flow edges must have at least one contract (already checked
	// in config.Validate, but double-checked here for graph-level errors)
	for _, e := range g.edges {
		if e.Type == config.EdgeDataFlow && len(e.Contracts) == 0 {
			errs = append(errs, ValidationError{
				Edge:    fmt.Sprintf("%s→%s", e.From, e.To),
				Rule:    "missing-contract",
				Message: fmt.Sprintf("data_flow edge %s→%s has no contracts", e.From, e.To),
				Fix:     "add contracts: [contracts/<name>.contract.yml] to this edge",
				DocsURL: "https://acthur.dev/docs/contracts",
			})
		}
	}

	// Rule: plugin applies_to targets must be service nodes
	for _, e := range g.edges {
		if e.Type == config.EdgeAppliesTo {
			target := g.nodes[e.To]
			if target != nil && target.Type != config.NodeTypeService {
				errs = append(errs, ValidationError{
					Edge:    fmt.Sprintf("%s→%s", e.From, e.To),
					Rule:    "invalid-applies-to",
					Message: fmt.Sprintf("applies_to edge targets %q which is not a service node", e.To),
					Fix:     "applies_to edges must point at service nodes only",
				})
			}
		}
	}

	return errs
}

// ValidationError represents a single graph validation failure.
type ValidationError struct {
	Node    string
	Edge    string
	Rule    string
	Message string
	Fix     string
	DocsURL string
}

func (e ValidationError) Error() string { return e.Message }

// ---------------------------------------------------------------------------
// Topological Sort
// ---------------------------------------------------------------------------

// topoSort performs a topological sort of nodes based on a specific edge type.
// An edge "from: A, to: B, type: depends_on" means A depends on B, so B must
// start first. Nodes with no dependencies (no outgoing edges of that type)
// have in-degree 0 in this scheme and are processed first.
func (g *Graph) topoSort(edgeType config.EdgeType) []*Node {
	// Build in-degree map: inDegree[node] = number of nodes node depends on.
	// Each "from→to depends_on" edge increments inDegree[from] because
	// "from" cannot start until "to" is ready.
	inDegree := make(map[string]int)
	for id := range g.nodes {
		inDegree[id] = 0
	}
	for _, e := range g.edges {
		if e.Type == edgeType {
			// Only count edges where both endpoints are real nodes
			if _, ok := g.nodes[e.From]; ok {
				if _, ok2 := g.nodes[e.To]; ok2 {
					inDegree[e.From]++
				}
			}
		}
	}

	// Start with all nodes that have in-degree 0 (no dependencies)
	var queue []string
	for id := range g.nodes {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	sortStrings(queue) // stable ordering

	var result []*Node
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if n := g.nodes[id]; n != nil {
			result = append(result, n)
		}
		// After starting id, unblock all nodes that depended on id.
		// Those are nodes with an outgoing edge of edgeType pointing to id,
		// i.e. the "from" side of edges in adjIn[id].
		neighbors := make([]string, 0)
		for _, e := range g.adjIn[id] {
			if e.Type == edgeType {
				if _, ok := g.nodes[e.From]; ok {
					inDegree[e.From]--
					if inDegree[e.From] == 0 {
						neighbors = append(neighbors, e.From)
					}
				}
			}
		}
		sortStrings(neighbors)
		queue = append(queue, neighbors...)
	}

	// Add any remaining nodes not reachable via this edge type
	inResult := map[string]bool{}
	for _, n := range result {
		inResult[n.ID] = true
	}
	var remaining []string
	for id := range g.nodes {
		if !inResult[id] {
			remaining = append(remaining, id)
		}
	}
	sortStrings(remaining)
	for _, id := range remaining {
		result = append(result, g.nodes[id])
	}

	return result
}

// detectCycle checks for cycles in depends_on edges using DFS.
func (g *Graph) detectCycle() error {
	const (
		unvisited = 0
		inStack   = 1
		done      = 2
	)
	color := make(map[string]int)
	var path []string

	var dfs func(id string) bool
	dfs = func(id string) bool {
		color[id] = inStack
		path = append(path, id)
		for _, e := range g.adjOut[id] {
			if e.Type != config.EdgeDependsOn {
				continue
			}
			switch color[e.To] {
			case inStack:
				// Cycle detected — find the cycle in path
				return true
			case unvisited:
				if dfs(e.To) {
					return true
				}
			}
		}
		color[id] = done
		path = path[:len(path)-1]
		return false
	}

	for id := range g.nodes {
		if color[id] == unvisited {
			path = nil
			if dfs(id) {
				return fmt.Errorf(
					"cycle detected in depends_on edges: %v\n"+
						"  Circular dependencies are not allowed — "+
						"review your graph edges",
					path,
				)
			}
		}
	}
	return nil
}

// sortStrings sorts a string slice in-place (bubble sort — small slices only).
func sortStrings(ss []string) {
	for i := 0; i < len(ss); i++ {
		for j := i + 1; j < len(ss); j++ {
			if ss[j] < ss[i] {
				ss[i], ss[j] = ss[j], ss[i]
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Display / Debug
// ---------------------------------------------------------------------------

// Summary returns a human-readable summary of the graph for `acthur graph show`.
func (g *Graph) Summary() string {
	var sb fmt.Stringer
	_ = sb
	out := ""
	out += fmt.Sprintf("  Nodes (%d)\n", len(g.nodes))
	out += fmt.Sprintf("  %s\n", repeatStr("─", 40))

	order := g.StartupOrder()
	for _, n := range order {
		state := ""
		if n.State() != StatePending {
			state = fmt.Sprintf(" [%s]", n.State())
		}
		out += fmt.Sprintf("  %-18s %-10s %s%s\n",
			n.ID, string(n.Type), n.Adapter, state)
	}

	out += fmt.Sprintf("\n  Edges (%d)\n", len(g.edges))
	out += fmt.Sprintf("  %s\n", repeatStr("─", 40))
	for _, e := range g.edges {
		contracts := ""
		if len(e.Contracts) > 0 {
			contracts = fmt.Sprintf(" [%s]", e.Contracts[0])
		}
		out += fmt.Sprintf("  %-18s →  %-18s %-16s%s\n",
			e.From, e.To, string(e.Type), contracts)
	}
	return out
}

func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
