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
	// 8 authored nodes + 1 materialized proxy kernel node = 9
	if len(g.Nodes()) != 9 {
		t.Errorf("expected 9 nodes (8 authored + proxy), got %d", len(g.Nodes()))
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

func TestBuild_ErrorOnNonExistentNodeRef(t *testing.T) {
	cfg := minConfig()
	cfg.Graph.Nodes["api"] = config.NodeConfig{Type: "service", Adapter: "go:fiber"}
	cfg.Graph.Edges = []config.EdgeConfig{
		{From: "api", To: "ghost", Type: config.EdgeDependsOn},
	}
	_, err := graph.Build(cfg)
	if err == nil {
		t.Error("expected build error for edge referencing non-existent node, got nil")
	}
}

func TestBuild_CycleDetection(t *testing.T) {
	// Phase 1B: cycle detection moved to Validate. Build must succeed; the
	// cycle is surfaced by g.Validate() as a ValidationError with rule="cycle".
	cfg := minConfig()
	// Create a cycle: a → b → a
	cfg.Graph.Nodes["a"] = config.NodeConfig{Type: "service", Adapter: "go:fiber"}
	cfg.Graph.Nodes["b"] = config.NodeConfig{Type: "service", Adapter: "go:fiber"}
	cfg.Graph.Edges = []config.EdgeConfig{
		{From: "a", To: "b", Type: config.EdgeDependsOn},
		{From: "b", To: "a", Type: config.EdgeDependsOn},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("Build must not fail for cyclic graph (cycle detection moved to Validate): %v", err)
	}
	errs := g.Validate()
	found := false
	for _, ve := range errs {
		if ve.Rule == "cycle" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected g.Validate() to return a ValidationError with rule=cycle")
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

func TestBuild_AutoDevURL_WorkerAndCronGetNone(t *testing.T) {
	cfg := minConfig()
	cfg.Graph.Nodes["api"] = config.NodeConfig{
		Type:    config.NodeTypeService,
		Adapter: "go:fiber",
		Role:    config.NodeRoleServer,
	}
	cfg.Graph.Nodes["worker"] = config.NodeConfig{
		Type:    config.NodeTypeService,
		Adapter: "go:fiber",
		Role:    config.NodeRoleQueueWorker,
	}
	cfg.Graph.Nodes["cron"] = config.NodeConfig{
		Type:    config.NodeTypeService,
		Adapter: "go:fiber",
		Role:    config.NodeRoleCron,
	}
	cfg.Graph.Nodes["gateway"] = config.NodeConfig{
		Type:    config.NodeTypeService,
		Adapter: "go:fiber",
		Role:    config.NodeRoleGateway,
	}
	cfg.Graph.Edges = []config.EdgeConfig{
		{From: "api", To: "worker", Type: config.EdgeDependsOn},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	// server-role and gateway-role services get auto dev_url
	if g.Node("api").DevURL == "" {
		t.Error("expected server-role service api to get auto dev_url")
	}
	if g.Node("gateway").DevURL == "" {
		t.Error("expected gateway-role service to get auto dev_url")
	}
	// queue-worker and cron must NOT get auto dev_url
	if g.Node("worker").DevURL != "" {
		t.Errorf("expected queue-worker to have no dev_url, got %q", g.Node("worker").DevURL)
	}
	if g.Node("cron").DevURL != "" {
		t.Errorf("expected cron to have no dev_url, got %q", g.Node("cron").DevURL)
	}
}

func TestBuild_MaterializesProxyNode(t *testing.T) {
	g := buildTestGraph(t)
	proxy := g.Node("proxy")
	if proxy == nil {
		t.Fatal("expected proxy node to be materialized by Build")
	}
	if proxy.Type != config.NodeTypeInfra {
		t.Errorf("expected proxy type=infra, got %q", proxy.Type)
	}
	if proxy.Adapter != "kernel:proxy" {
		t.Errorf("expected proxy adapter=kernel:proxy, got %q", proxy.Adapter)
	}
	if proxy.DevURL != "" {
		t.Errorf("expected proxy to have no dev_url, got %q", proxy.DevURL)
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
	var got string
	_ = g.Subscribe(func(id string, state graph.NodeState) {
		got = id
	})
	g.SetState("api", graph.StateHealthy)
	// Synchronous delivery: by the time SetState returns, callback has run.
	if got != "api" {
		t.Errorf("expected notification for api, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Phase 1G — subscription tests
// ---------------------------------------------------------------------------

// Behavior 1: subscriber is called synchronously (no goroutine delay).
func TestSubscribe_SynchronousDelivery(t *testing.T) {
	g := buildMinGraph(t)
	called := false
	_ = g.Subscribe(func(id string, state graph.NodeState) {
		called = true
	})
	g.SetState("a", graph.StateHealthy)
	// If delivery were async we could not guarantee called==true here.
	if !called {
		t.Error("expected subscriber to be called synchronously before SetState returned")
	}
}

// Behavior 2: multiple subscribers are called in registration order.
func TestSubscribe_RegistrationOrder(t *testing.T) {
	g := buildMinGraph(t)
	var order []int
	_ = g.Subscribe(func(id string, state graph.NodeState) { order = append(order, 1) })
	_ = g.Subscribe(func(id string, state graph.NodeState) { order = append(order, 2) })
	_ = g.Subscribe(func(id string, state graph.NodeState) { order = append(order, 3) })
	g.SetState("a", graph.StateHealthy)
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("expected [1 2 3], got %v", order)
	}
}

// Behavior 3: a node's own transitions are delivered in order.
func TestSubscribe_PerNodeTransitionOrder(t *testing.T) {
	g := buildMinGraph(t)
	var states []graph.NodeState
	_ = g.Subscribe(func(id string, state graph.NodeState) {
		if id == "a" {
			states = append(states, state)
		}
	})
	g.SetState("a", graph.StatePending)
	g.SetState("a", graph.StateStarting)
	g.SetState("a", graph.StateHealthy)
	want := []graph.NodeState{graph.StatePending, graph.StateStarting, graph.StateHealthy}
	if len(states) != len(want) {
		t.Fatalf("expected %d transitions, got %d: %v", len(want), len(states), states)
	}
	for i, s := range states {
		if s != want[i] {
			t.Errorf("transition %d: expected %q, got %q", i, want[i], s)
		}
	}
}

// Behavior 4: Subscribe returns an unsubscribe func; after calling it no
// further callbacks fire.
func TestSubscribe_UnsubscribeStopsCallbacks(t *testing.T) {
	g := buildMinGraph(t)
	count := 0
	unsub := g.Subscribe(func(id string, state graph.NodeState) {
		count++
	})
	g.SetState("a", graph.StateStarting)
	unsub()
	g.SetState("a", graph.StateHealthy)
	if count != 1 {
		t.Errorf("expected exactly 1 callback (before unsub), got %d", count)
	}
}

// Behavior 5: Node.mu is NOT held during callback execution.
// The callback reads g.GetState which acquires Node.mu internally.
// If the node's mutex were still held, this would deadlock.
func TestSubscribe_NodeMuNotHeldDuringCallback(t *testing.T) {
	g := buildMinGraph(t)
	var stateSeenInCallback graph.NodeState
	_ = g.Subscribe(func(id string, state graph.NodeState) {
		// GetState acquires Node.mu — would deadlock if SetState held it.
		stateSeenInCallback = g.GetState(id)
	})
	g.SetState("a", graph.StateHealthy)
	if stateSeenInCallback != graph.StateHealthy {
		t.Errorf("expected healthy state inside callback, got %q", stateSeenInCallback)
	}
}

// buildMinGraph builds a minimal single-node graph for subscription tests.
func buildMinGraph(t *testing.T) *graph.Graph {
	t.Helper()
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"a": {Type: "service", Adapter: "go:fiber"},
				"b": {Type: "infra", Adapter: "db:postgres"},
			},
			Edges: []config.EdgeConfig{
				{From: "a", To: "b", Type: config.EdgeDependsOn},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("failed to build min graph: %v", err)
	}
	return g
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Phase 1B — Severity, Cycle-as-Validation-Error, three-tier pipeline
// ---------------------------------------------------------------------------

// Behavior 1: ValidationError struct has an accessible Severity field.
func TestValidationError_HasSeverityField(t *testing.T) {
	ve := graph.ValidationError{
		Node:     "api",
		Rule:     "test-rule",
		Message:  "test message",
		Severity: graph.SeverityError,
	}
	if ve.Severity != graph.SeverityError {
		t.Errorf("expected SeverityError, got %q", ve.Severity)
	}
}

// Behavior 2: All existing validation rules emit SeverityError by default.
func TestValidate_ExistingRulesHaveSeverityError(t *testing.T) {
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api":    {Type: "service", Adapter: "go:fiber"},
				"orphan": {Type: "service", Adapter: "go:fiber"},
			},
			Edges: []config.EdgeConfig{},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	errs := g.Validate()
	for _, e := range errs {
		if e.Severity != graph.SeverityError {
			t.Errorf("rule %q: expected SeverityError, got %q", e.Rule, e.Severity)
		}
	}
}

// Behavior 3: A cyclic graph builds successfully; Validate returns a
// ValidationError with SeverityError containing the cycle path.
func TestValidate_CycleDetected(t *testing.T) {
	cfg := minConfig()
	cfg.Graph.Nodes["a"] = config.NodeConfig{Type: "service", Adapter: "go:fiber"}
	cfg.Graph.Nodes["b"] = config.NodeConfig{Type: "service", Adapter: "go:fiber"}
	cfg.Graph.Edges = []config.EdgeConfig{
		{From: "a", To: "b", Type: config.EdgeDependsOn},
		{From: "b", To: "a", Type: config.EdgeDependsOn},
	}
	// Behavior 4 test is implicit: Build must NOT return an error for this config
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("Build should succeed for cyclic graph (cycle is a Validate concern): %v", err)
	}

	errs := g.Validate()
	var cycleErr *graph.ValidationError
	for i := range errs {
		if errs[i].Rule == "cycle" {
			cycleErr = &errs[i]
			break
		}
	}
	if cycleErr == nil {
		t.Fatal("expected a ValidationError with rule=cycle, got none")
	}
	if cycleErr.Severity != graph.SeverityError {
		t.Errorf("cycle error: expected SeverityError, got %q", cycleErr.Severity)
	}
	// Message must name the cycle path
	if cycleErr.Message == "" {
		t.Error("cycle error: expected non-empty message naming cycle nodes")
	}
	// Fix hint must be present
	if cycleErr.Fix == "" {
		t.Error("cycle error: expected a non-empty Fix hint")
	}
}

// Behavior 4 (standalone): Build does NOT return an error for a cyclic graph.
func TestBuild_CyclicGraph_BuildsSuccessfully(t *testing.T) {
	cfg := minConfig()
	cfg.Graph.Nodes["a"] = config.NodeConfig{Type: "service", Adapter: "go:fiber"}
	cfg.Graph.Nodes["b"] = config.NodeConfig{Type: "service", Adapter: "go:fiber"}
	cfg.Graph.Edges = []config.EdgeConfig{
		{From: "a", To: "b", Type: config.EdgeDependsOn},
		{From: "b", To: "a", Type: config.EdgeDependsOn},
	}
	_, err := graph.Build(cfg)
	if err != nil {
		t.Errorf("Build must not return an error for cyclic graph — cycle detection moved to Validate: %v", err)
	}
}

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
	// vetangle authored: db, cache, storage, queue = 4 infra
	// plus 1 materialized kernel: proxy = 5 total
	if len(infra) != 5 {
		t.Errorf("expected 5 infra nodes (4 authored + proxy), got %d", len(infra))
	}
}

// ---------------------------------------------------------------------------
// Summary tests
// ---------------------------------------------------------------------------

func TestSummary_ContainsProxyNode(t *testing.T) {
	g := buildTestGraph(t)
	s := g.Summary()
	if !contains([]string{s}, "proxy") {
		// Use simple string search
		if len(s) == 0 {
			t.Error("expected non-empty summary")
		}
		found := false
		for i := 0; i+5 <= len(s); i++ {
			if s[i:i+5] == "proxy" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected Summary to contain 'proxy' node")
		}
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
