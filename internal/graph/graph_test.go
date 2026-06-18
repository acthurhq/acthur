package graph_test

import (
	"testing"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
)

// ---------------------------------------------------------------------------
// Build tests
// ---------------------------------------------------------------------------

func TestBuild_FromValidConfig(t *testing.T) {
	cfg := newConfig(t, "vetangle")
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(g.Nodes()) != 8 {
		t.Errorf("expected 8 nodes, got %d", len(g.Nodes()))
	}
}

func TestBuild_NodeProperties(t *testing.T) {
	g := buildTestGraph(t)
	api := g.Node("api")
	if api == nil {
		t.Fatal("expected api node to exist")
	}
	if api.Type != config.NodeTypeService {
		t.Errorf("expected service type, got %q", api.Type)
	}
	if api.Adapter != "go:fiber" {
		t.Errorf("expected go:fiber adapter, got %q", api.Adapter)
	}
}

func TestBuild_CycleDetection(t *testing.T) {
	cfg := minConfig()
	// Create a cycle: a → b → a
	cfg.Graph.Nodes["a"] = config.NodeConfig{Type: "service", Adapter: "go:fiber"}
	cfg.Graph.Nodes["b"] = config.NodeConfig{Type: "service", Adapter: "go:fiber"}
	cfg.Graph.Edges = []config.EdgeConfig{
		{From: "a", To: "b", Type: config.EdgeDependsOn},
		{From: "b", To: "a", Type: config.EdgeDependsOn},
	}
	_, err := graph.Build(cfg)
	if err == nil {
		t.Error("expected cycle detection error, got nil")
	}
}

func TestBuild_AutoDevURL(t *testing.T) {
	g := buildTestGraph(t)
	api := g.Node("api")
	if api.DevURL == "" {
		t.Error("expected DevURL to be auto-generated for service nodes")
	}
	// Should be api.vetangle.test
	if api.DevURL != "api.vetangle.test" {
		t.Errorf("expected api.vetangle.test, got %q", api.DevURL)
	}
}

// ---------------------------------------------------------------------------
// Startup order tests
// ---------------------------------------------------------------------------

func TestStartupOrder_InfraBeforeServices(t *testing.T) {
	g := buildTestGraph(t)
	order := g.StartupOrder()

	// Find positions of api and db
	apiPos, dbPos := -1, -1
	for i, n := range order {
		switch n.ID {
		case "api":
			apiPos = i
		case "db":
			dbPos = i
		}
	}
	if dbPos == -1 || apiPos == -1 {
		t.Fatal("api or db node not found in startup order")
	}
	if dbPos >= apiPos {
		t.Errorf("expected db (pos %d) to come before api (pos %d)", dbPos, apiPos)
	}
}

func TestStartupOrder_ContainsAllNodes(t *testing.T) {
	g := buildTestGraph(t)
	order := g.StartupOrder()
	if len(order) != len(g.Nodes()) {
		t.Errorf("startup order has %d nodes, expected %d", len(order), len(g.Nodes()))
	}
}

func TestShutdownOrder_ReverseOfStartup(t *testing.T) {
	g := buildTestGraph(t)
	start := g.StartupOrder()
	stop := g.ShutdownOrder()
	if len(start) != len(stop) {
		t.Fatalf("startup and shutdown orders have different lengths")
	}
	for i := range start {
		j := len(stop) - 1 - i
		if start[i].ID != stop[j].ID {
			t.Errorf("position %d: startup=%q, shutdown[reversed]=%q", i, start[i].ID, stop[j].ID)
		}
	}
}

// ---------------------------------------------------------------------------
// Traversal tests
// ---------------------------------------------------------------------------

func TestDependenciesOf(t *testing.T) {
	g := buildTestGraph(t)
	deps := g.DependenciesOf("api")
	depNames := nodeIDs(deps)
	if !contains(depNames, "db") {
		t.Errorf("expected api to depend on db, got %v", depNames)
	}
	if !contains(depNames, "cache") {
		t.Errorf("expected api to depend on cache, got %v", depNames)
	}
}

func TestDependentsOf(t *testing.T) {
	g := buildTestGraph(t)
	dependents := g.DependentsOf("db")
	names := nodeIDs(dependents)
	if !contains(names, "api") {
		t.Errorf("expected db dependents to include api, got %v", names)
	}
}

func TestAffectedByFailure(t *testing.T) {
	// Simple chain: api → db (depends_on)
	// If db fails, api is affected
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: "service", Adapter: "go:fiber"},
				"db":  {Type: "infra", Adapter: "db:postgres"},
			},
			Edges: []config.EdgeConfig{
				{From: "api", To: "db", Type: config.EdgeDependsOn},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	affected := g.AffectedByFailure("db")
	names := nodeIDs(affected)
	if !contains(names, "api") {
		t.Errorf("expected api to be affected by db failure, got %v", names)
	}
}

func TestPropagateFrom_DataFlow(t *testing.T) {
	// web → api (data_flow), change to api should propagate to web
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"web": {Type: "service", Adapter: "ui:astro"},
				"api": {Type: "service", Adapter: "go:fiber"},
			},
			Edges: []config.EdgeConfig{
				{From: "web", To: "api", Type: config.EdgeDataFlow,
					Contracts: []string{"contracts/users.contract.yml"}},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	downstream := g.PropagateFrom("api")
	names := nodeIDs(downstream)
	if !contains(names, "web") {
		t.Errorf("expected web to be in propagation from api, got %v", names)
	}
}

// ---------------------------------------------------------------------------
// Edge query tests
// ---------------------------------------------------------------------------

func TestEdgesOfType_DependsOn(t *testing.T) {
	g := buildTestGraph(t)
	edges := g.EdgesOfType(config.EdgeDependsOn)
	if len(edges) == 0 {
		t.Error("expected depends_on edges to exist")
	}
	for _, e := range edges {
		if e.Type != config.EdgeDependsOn {
			t.Errorf("expected only depends_on edges, got %q", e.Type)
		}
	}
}

func TestContractFor_DataFlow(t *testing.T) {
	g := buildTestGraph(t)
	contract := g.ContractFor("web", "api")
	if contract == "" {
		t.Error("expected contract on web→api data_flow edge")
	}
}

func TestContractFor_NonExistentEdge(t *testing.T) {
	g := buildTestGraph(t)
	contract := g.ContractFor("web", "db") // no such edge
	if contract != "" {
		t.Errorf("expected empty contract for non-existent edge, got %q", contract)
	}
}

func TestProxiedNodes(t *testing.T) {
	g := buildTestGraph(t)
	proxied := g.ProxiedNodes()
	if len(proxied) == 0 {
		t.Error("expected proxied nodes to exist")
	}
}

// ---------------------------------------------------------------------------
// State management tests
// ---------------------------------------------------------------------------

func TestState_DefaultIsPending(t *testing.T) {
	g := buildTestGraph(t)
	for _, n := range g.Nodes() {
		if n.State() != graph.StatePending {
			t.Errorf("node %q: expected pending state, got %q", n.ID, n.State())
		}
	}
}

func TestState_SetAndGet(t *testing.T) {
	g := buildTestGraph(t)
	g.SetState("api", graph.StateHealthy)
	if got := g.GetState("api"); got != graph.StateHealthy {
		t.Errorf("expected healthy, got %q", got)
	}
}

func TestState_AllHealthy_False(t *testing.T) {
	g := buildTestGraph(t)
	if g.AllHealthy() {
		t.Error("expected AllHealthy to be false when nodes are pending")
	}
}

func TestState_AllHealthy_True(t *testing.T) {
	g := buildTestGraph(t)
	// Set only service and infra nodes to healthy
	for _, n := range g.Nodes() {
		if n.IsService() || n.IsInfra() {
			g.SetState(n.ID, graph.StateHealthy)
		}
	}
	if !g.AllHealthy() {
		t.Error("expected AllHealthy to be true when all service/infra nodes are healthy")
	}
}

func TestState_Subscribe(t *testing.T) {
	g := buildTestGraph(t)
	received := make(chan string, 1)
	g.Subscribe(func(id string, state graph.NodeState) {
		received <- id
	})
	g.SetState("api", graph.StateHealthy)
	got := <-received
	if got != "api" {
		t.Errorf("expected notification for api, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestValidate_OrphanNode(t *testing.T) {
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api":     {Type: "service", Adapter: "go:fiber"},
				"orphan": {Type: "service", Adapter: "go:fiber"}, // no edges
			},
			Edges: []config.EdgeConfig{}, // no edges at all
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	errs := g.Validate()
	if len(errs) == 0 {
		t.Error("expected orphan node validation error")
	}
}

func TestValidate_CleanGraph(t *testing.T) {
	g := buildTestGraph(t)
	errs := g.Validate()
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("unexpected validation error: %s", e.Error())
		}
	}
}

// ---------------------------------------------------------------------------
// NodesByType tests
// ---------------------------------------------------------------------------

func TestNodesByType_Service(t *testing.T) {
	g := buildTestGraph(t)
	services := g.NodesByType(config.NodeTypeService)
	// vetangle has: api, web, backoffice, worker = 4 services
	if len(services) != 4 {
		t.Errorf("expected 4 service nodes, got %d", len(services))
	}
}

func TestNodesByType_Infra(t *testing.T) {
	g := buildTestGraph(t)
	infra := g.NodesByType(config.NodeTypeInfra)
	// vetangle has: db, cache, storage, queue = 4 infra
	if len(infra) != 4 {
		t.Errorf("expected 4 infra nodes, got %d", len(infra))
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newConfig(t *testing.T, fixture string) *config.Config {
	t.Helper()
	cfg, err := config.LoadFile("../../testdata/" + fixture + "/acthur.yml")
	if err != nil {
		t.Fatalf("failed to load fixture %q: %v", fixture, err)
	}
	return cfg
}

func buildTestGraph(t *testing.T) *graph.Graph {
	t.Helper()
	cfg := newConfig(t, "vetangle")
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}
	return g
}

func minConfig() *config.Config {
	return &config.Config{
		Project: "test",
		Version: "1",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{},
			Edges: []config.EdgeConfig{},
		},
	}
}

func nodeIDs(nodes []*graph.Node) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
