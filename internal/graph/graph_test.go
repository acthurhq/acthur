package graph_test

import (
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
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
	errs := g.Validate(graph.EmptyResolver{})
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

	// Find positions of key nodes.
	// vetangle has: infra(db, cache, storage, queue, proxy), service(api, web, backoffice, worker)
	// web and backoffice have no depends_on to db — they should still come after infra nodes.
	positions := map[string]int{}
	for i, n := range order {
		positions[n.ID] = i
	}

	// api depends_on db (explicit edge) — db must come before api
	if positions["db"] == 0 && positions["api"] == 0 {
		t.Fatal("api or db node not found in startup order")
	}
	if positions["db"] >= positions["api"] {
		t.Errorf("expected db (pos %d) to come before api (pos %d)", positions["db"], positions["api"])
	}

	// web has no depends_on to any infra node — tie-breaker must still put infra first.
	for _, infraID := range []string{"db", "cache", "storage", "queue", "proxy"} {
		if positions[infraID] >= positions["web"] {
			t.Errorf("expected %s (infra, pos %d) to come before web (service, pos %d) — tie-breaker failed",
				infraID, positions[infraID], positions["web"])
		}
	}

	// backoffice also has no depends_on to infra — same constraint.
	for _, infraID := range []string{"db", "cache", "storage", "queue", "proxy"} {
		if positions[infraID] >= positions["backoffice"] {
			t.Errorf("expected %s (infra, pos %d) to come before backoffice (service, pos %d) — tie-breaker failed",
				infraID, positions[infraID], positions["backoffice"])
		}
	}
}

func TestStartupOrder_ContainsAllNodes(t *testing.T) {
	g := buildTestGraph(t)
	order := g.StartupOrder()
	if len(order) != len(g.Nodes()) {
		t.Errorf("startup order has %d nodes, expected %d", len(order), len(g.Nodes()))
	}
}

// TestStartupOrder_FrontendAfterInfra verifies that a frontend/service node with
// no depends_on is ordered after infra nodes, even when its name sorts
// alphabetically before the infra node name.
func TestStartupOrder_FrontendAfterInfra(t *testing.T) {
	// "web" sorts alphabetically between "db" and "redis" — but all three are
	// at the same topological rank (no depends_on edges between them). The
	// tie-breaker must place both infra nodes before the service node.
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"db":    {Type: config.NodeTypeInfra, Adapter: "db:postgres"},
				"redis": {Type: config.NodeTypeInfra, Adapter: "cache:redis"},
				"web":   {Type: config.NodeTypeService, Adapter: "ui:astro", Role: config.NodeRoleServer},
			},
			Edges: []config.EdgeConfig{
				// web has a data_flow to db but NO depends_on — same rank.
				{From: "web", To: "db", Type: config.EdgeDataFlow,
					Contracts: []string{"contracts/dummy.contract.yml"}},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	order := g.StartupOrder()
	webPos, dbPos, redisPos := -1, -1, -1
	for i, n := range order {
		switch n.ID {
		case "web":
			webPos = i
		case "db":
			dbPos = i
		case "redis":
			redisPos = i
		}
	}
	if webPos == -1 || dbPos == -1 || redisPos == -1 {
		t.Fatal("expected web, db, redis nodes in startup order")
	}
	if dbPos >= webPos {
		t.Errorf("expected db (infra, pos %d) before web (service, pos %d)", dbPos, webPos)
	}
	if redisPos >= webPos {
		t.Errorf("expected redis (infra, pos %d) before web (service, pos %d)", redisPos, webPos)
	}
}

// TestStartupOrder_ExplicitDependsOnAlwaysWins verifies that an explicit
// depends_on edge always takes precedence over the type-tier tie-breaker.
// Even if a service node (api) would come before an infra node (metrics) by
// alphabetical order within the same tier, an explicit depends_on from api→db
// means api is ordered after db regardless. Separately, an infra node (metrics)
// with no depends_on to api is still ordered before api because it's infra.
// The strongest case: "infra2" depends on "svc1" (service) — infra2 must come
// AFTER svc1 because the explicit edge wins over the type-tier convention.
func TestStartupOrder_ExplicitDependsOnAlwaysWins(t *testing.T) {
	// infra2 depends_on svc1 — even though infra normally comes first,
	// the explicit edge forces infra2 to start after svc1.
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"svc1":   {Type: config.NodeTypeService, Adapter: "go:fiber"},
				"infra2": {Type: config.NodeTypeInfra, Adapter: "db:postgres"},
			},
			Edges: []config.EdgeConfig{
				// infra2 explicitly depends_on svc1 — topology wins over type tier.
				{From: "infra2", To: "svc1", Type: config.EdgeDependsOn},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	order := g.StartupOrder()
	svc1Pos, infra2Pos := -1, -1
	for i, n := range order {
		switch n.ID {
		case "svc1":
			svc1Pos = i
		case "infra2":
			infra2Pos = i
		}
	}
	if svc1Pos == -1 || infra2Pos == -1 {
		t.Fatal("expected svc1 and infra2 nodes in startup order")
	}
	// infra2 depends on svc1, so svc1 must come first — explicit edge wins.
	if svc1Pos >= infra2Pos {
		t.Errorf("expected svc1 (service, pos %d) before infra2 (infra, pos %d) — explicit depends_on must win", svc1Pos, infra2Pos)
	}
}

// TestStartupOrder_TiebreakerInfraBeforeService verifies that within a
// topological tie (no dependency between them), an infra node is always
// ordered before a service node regardless of alphabetical order.
func TestStartupOrder_TiebreakerInfraBeforeService(t *testing.T) {
	// "zebra" (service) sorts after "aardvark" (infra) alphabetically, but here
	// we use names where the service ("aaa") sorts before the infra ("zzz") to
	// confirm the tie-breaker beats alphabetical order.
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"aaa": {Type: config.NodeTypeService, Adapter: "go:fiber"},
				"zzz": {Type: config.NodeTypeInfra, Adapter: "db:postgres"},
			},
			Edges: []config.EdgeConfig{
				// No depends_on between them — pure tie at rank 0.
				// We need at least one edge to satisfy graph validation.
				{From: "aaa", To: "zzz", Type: config.EdgeDataFlow,
					Contracts: []string{"contracts/dummy.contract.yml"}},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	order := g.StartupOrder()
	// proxy (infra, materialized) + zzz (infra) should precede aaa (service).
	// We specifically need zzz before aaa.
	zzzPos, aaaPos := -1, -1
	for i, n := range order {
		switch n.ID {
		case "zzz":
			zzzPos = i
		case "aaa":
			aaaPos = i
		}
	}
	if zzzPos == -1 || aaaPos == -1 {
		t.Fatal("expected both zzz and aaa nodes in startup order")
	}
	if zzzPos >= aaaPos {
		t.Errorf("expected zzz (infra, pos %d) to come before aaa (service, pos %d) — tie-breaker failed", zzzPos, aaaPos)
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
	errs := g.Validate(graph.EmptyResolver{})
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

	errs := g.Validate(graph.EmptyResolver{})
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
	errs := g.Validate(graph.EmptyResolver{})
	if len(errs) == 0 {
		t.Error("expected orphan node validation error")
	}
}

func TestValidate_CleanGraph(t *testing.T) {
	g := buildTestGraph(t)
	errs := g.Validate(graph.EmptyResolver{})
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

// ---------------------------------------------------------------------------
// Phase 1F — Two-phase lifecycle: Freeze/seal with post-seal mutation guard
// ---------------------------------------------------------------------------

// Behavior 1: Freeze() seals the graph; IsSealed() reflects the change.
func TestFreeze_SetsSealed(t *testing.T) {
	g := buildMinGraph(t)
	if g.IsSealed() {
		t.Error("expected graph to be unsealed before Freeze()")
	}
	g.Freeze()
	if !g.IsSealed() {
		t.Error("expected graph to be sealed after Freeze()")
	}
}

// Behavior 1b: Calling Freeze() a second time is a no-op (not a panic).
func TestFreeze_IsIdempotent(t *testing.T) {
	g := buildMinGraph(t)
	g.Freeze()
	// Should not panic.
	g.Freeze()
	if !g.IsSealed() {
		t.Error("expected graph to remain sealed after second Freeze()")
	}
}

// Behavior 2: AddNode works before seal.
func TestAddNode_BeforeSeal_Succeeds(t *testing.T) {
	cfg := minConfig()
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	g.AddNode(&graph.Node{ID: "new-node", Type: config.NodeTypeService, Adapter: "go:fiber"})
	if g.Node("new-node") == nil {
		t.Error("expected new-node to be added before seal")
	}
}

// Behavior 3: AddNode panics after seal.
func TestAddNode_AfterSeal_Panics(t *testing.T) {
	g := buildMinGraph(t)
	g.Freeze()

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic when adding a node to a sealed graph")
		}
	}()
	g.AddNode(&graph.Node{ID: "too-late", Type: config.NodeTypeService, Adapter: "go:fiber"})
}

// Behavior 4: AddEdge panics after seal.
func TestAddEdge_AfterSeal_Panics(t *testing.T) {
	g := buildMinGraph(t)
	g.Freeze()

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic when adding an edge to a sealed graph")
		}
	}()
	g.AddEdge(&graph.Edge{From: "a", To: "b", Type: config.EdgeDependsOn})
}

// Behavior 4b: AddEdge works before seal.
func TestAddEdge_BeforeSeal_Succeeds(t *testing.T) {
	cfg := minConfig()
	cfg.Graph.Nodes["x"] = config.NodeConfig{Type: config.NodeTypeService, Adapter: "go:fiber"}
	cfg.Graph.Nodes["y"] = config.NodeConfig{Type: config.NodeTypeInfra, Adapter: "db:postgres"}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	g.AddEdge(&graph.Edge{From: "x", To: "y", Type: config.EdgeDependsOn})
	edges := g.EdgesFrom("x")
	if len(edges) == 0 {
		t.Error("expected edge to be present after AddEdge before seal")
	}
}

// Behavior 5: SetState on a sealed graph still works (state is not structural).
func TestSetState_AfterSeal_Succeeds(t *testing.T) {
	g := buildMinGraph(t)
	g.Freeze()
	// Must not panic.
	g.SetState("a", graph.StateHealthy)
	if got := g.GetState("a"); got != graph.StateHealthy {
		t.Errorf("expected healthy after SetState on sealed graph, got %q", got)
	}
}

// Behavior 6: Read traversals work normally on a sealed graph.
func TestReadTraversals_AfterSeal_Succeed(t *testing.T) {
	g := buildMinGraph(t)
	g.Freeze()
	nodes := g.Nodes()
	if len(nodes) == 0 {
		t.Error("expected Nodes() to work on sealed graph")
	}
	edges := g.Edges()
	_ = edges // nil is fine for a min graph with no edges added post-build
	order := g.StartupOrder()
	if len(order) == 0 {
		t.Error("expected StartupOrder() to work on sealed graph")
	}
}

// ---------------------------------------------------------------------------
// Phase 1D — Reject hand-declared contract/plugin nodes (authored vs materialized)
// ---------------------------------------------------------------------------

// Behavior 1: A node declared with type "contract" produces a ValidationError
// with Rule "reserved-node-type".
func TestValidate_ContractNode_RejectsWithReservedNodeTypeRule(t *testing.T) {
	nodes := map[string]*graph.Node{
		"auth": {
			ID:      "auth",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"user-contract": {
			ID:      "user-contract",
			Type:    config.NodeTypeContract,
			Adapter: "kernel:contract",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "auth", To: "user-contract", Type: config.EdgeSatisfies})

	errs := g.Validate(graph.EmptyResolver{})

	var found *graph.ValidationError
	for i := range errs {
		if errs[i].Rule == "reserved-node-type" && errs[i].Node == "user-contract" {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected ValidationError with Rule=reserved-node-type for contract node, got: %v", errs)
	}
	if found.Severity != graph.SeverityError {
		t.Errorf("expected SeverityError, got %q", found.Severity)
	}
}

// Behavior 2: The contract node error message references data_flow contracts: syntax.
func TestValidate_ContractNode_FixPointsToDataFlowContracts(t *testing.T) {
	nodes := map[string]*graph.Node{
		"auth": {
			ID:      "auth",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"user-contract": {
			ID:      "user-contract",
			Type:    config.NodeTypeContract,
			Adapter: "kernel:contract",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "auth", To: "user-contract", Type: config.EdgeSatisfies})

	errs := g.Validate(graph.EmptyResolver{})

	var found *graph.ValidationError
	for i := range errs {
		if errs[i].Rule == "reserved-node-type" && errs[i].Node == "user-contract" {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected reserved-node-type error for contract node")
	}
	if found.Fix == "" {
		t.Error("expected a non-empty Fix hint for contract reserved-node-type error")
	}
	if !containsSubstr(found.Fix, "data_flow") && !containsSubstr(found.Fix, "contracts:") {
		t.Errorf("expected Fix to mention data_flow or contracts: syntax, got %q", found.Fix)
	}
	if !containsSubstr(found.Message, "contract") {
		t.Errorf("expected Message to mention 'contract', got %q", found.Message)
	}
}

// Behavior 3: A node declared with type "plugin" produces a ValidationError
// with Rule "reserved-node-type".
func TestValidate_PluginNode_RejectsWithReservedNodeTypeRule(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"my-plugin": {
			ID:      "my-plugin",
			Type:    config.NodeTypePlugin,
			Adapter: "kernel:plugin",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "my-plugin", To: "api", Type: config.EdgeAppliesTo})

	errs := g.Validate(graph.EmptyResolver{})

	var found *graph.ValidationError
	for i := range errs {
		if errs[i].Rule == "reserved-node-type" && errs[i].Node == "my-plugin" {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected ValidationError with Rule=reserved-node-type for plugin node, got: %v", errs)
	}
	if found.Severity != graph.SeverityError {
		t.Errorf("expected SeverityError, got %q", found.Severity)
	}
}

// Behavior 4: The plugin node error message references the top-level plugins: list.
func TestValidate_PluginNode_FixPointsToPluginsList(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"my-plugin": {
			ID:      "my-plugin",
			Type:    config.NodeTypePlugin,
			Adapter: "kernel:plugin",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "my-plugin", To: "api", Type: config.EdgeAppliesTo})

	errs := g.Validate(graph.EmptyResolver{})

	var found *graph.ValidationError
	for i := range errs {
		if errs[i].Rule == "reserved-node-type" && errs[i].Node == "my-plugin" {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected reserved-node-type error for plugin node")
	}
	if found.Fix == "" {
		t.Error("expected a non-empty Fix hint for plugin reserved-node-type error")
	}
	if !containsSubstr(found.Fix, "plugins:") && !containsSubstr(found.Fix, "plugins") {
		t.Errorf("expected Fix to mention plugins: list, got %q", found.Fix)
	}
	if !containsSubstr(found.Message, "plugin") {
		t.Errorf("expected Message to mention 'plugin', got %q", found.Message)
	}
}

// Behavior 5: A service node is NOT rejected — no reserved-node-type error.
func TestValidate_ServiceNode_NotRejectedAsReserved(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"db": {
			ID:      "db",
			Type:    config.NodeTypeInfra,
			Adapter: "db:postgres",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})

	errs := g.Validate(graph.EmptyResolver{})

	for _, e := range errs {
		if e.Rule == "reserved-node-type" {
			t.Errorf("service/infra node got unexpected reserved-node-type error: %v", e)
		}
	}
}

// Behavior 6: An infra node is NOT rejected — no reserved-node-type error.
func TestValidate_InfraNode_NotRejectedAsReserved(t *testing.T) {
	nodes := map[string]*graph.Node{
		"svc": {
			ID:      "svc",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"cache": {
			ID:      "cache",
			Type:    config.NodeTypeInfra,
			Adapter: "cache:redis",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "svc", To: "cache", Type: config.EdgeDependsOn})

	errs := g.Validate(graph.EmptyResolver{})

	for _, e := range errs {
		if e.Rule == "reserved-node-type" {
			t.Errorf("infra node got unexpected reserved-node-type error: %v", e)
		}
	}
}

// containsSubstr is a helper to check substring presence without importing strings.
func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}

// ---------------------------------------------------------------------------
// Phase 1H — Resolver abstraction and unresolved-adapter rule (issue #18)
// ---------------------------------------------------------------------------

// fakeResolver is a map-backed test helper that implements graph.Resolver.
type fakeResolver struct {
	adapters map[string]graph.ResolvedAdapter
}

func (f fakeResolver) Resolve(key string) (graph.ResolvedAdapter, bool) {
	a, ok := f.adapters[key]
	return a, ok
}

func (f fakeResolver) Names() []string {
	names := make([]string, 0, len(f.adapters))
	for k := range f.adapters {
		names = append(names, k)
	}
	return names
}

// Behavior 1: Unknown adapter key → ValidationError with Rule="unresolved-adapter",
// Severity=error, message contains the unknown key name.
func TestValidate_UnresolvedAdapter_UnknownKeyProducesError(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"db": {
			ID:      "db",
			Type:    config.NodeTypeInfra,
			Adapter: "db:postgres",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})

	resolver := fakeResolver{
		adapters: map[string]graph.ResolvedAdapter{
			"db:postgres": {Name: "db:postgres", Category: "infra"},
			// go:fiber intentionally missing
		},
	}

	errs := g.Validate(resolver)

	var found *graph.ValidationError
	for i := range errs {
		if errs[i].Rule == "unresolved-adapter" {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected ValidationError with Rule=unresolved-adapter, got: %v", errs)
	}
	if found.Severity != graph.SeverityError {
		t.Errorf("expected SeverityError, got %q", found.Severity)
	}
	if !containsSubstr(found.Message, "go:fiber") {
		t.Errorf("expected message to contain unknown adapter key %q, got: %q", "go:fiber", found.Message)
	}
}

// Behavior 2: Error message lists available adapters.
func TestValidate_UnresolvedAdapter_MessageListsAvailableAdapters(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"db": {
			ID:      "db",
			Type:    config.NodeTypeInfra,
			Adapter: "db:postgres",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})

	resolver := fakeResolver{
		adapters: map[string]graph.ResolvedAdapter{
			"db:postgres": {Name: "db:postgres", Category: "infra"},
			// go:fiber missing
		},
	}

	errs := g.Validate(resolver)

	var found *graph.ValidationError
	for i := range errs {
		if errs[i].Rule == "unresolved-adapter" && containsSubstr(errs[i].Node, "api") {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected unresolved-adapter error for api node, got: %v", errs)
	}
	if !containsSubstr(found.Message, "db:postgres") {
		t.Errorf("expected message to list available adapter %q, got: %q", "db:postgres", found.Message)
	}
}

// Behavior 3: Known adapter key → no "unresolved-adapter" error.
func TestValidate_UnresolvedAdapter_KnownKeyNoError(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"db": {
			ID:      "db",
			Type:    config.NodeTypeInfra,
			Adapter: "db:postgres",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})

	resolver := fakeResolver{
		adapters: map[string]graph.ResolvedAdapter{
			"go:fiber":    {Name: "go:fiber", Category: "service"},
			"db:postgres": {Name: "db:postgres", Category: "infra"},
		},
	}

	errs := g.Validate(resolver)

	for _, e := range errs {
		if e.Rule == "unresolved-adapter" {
			t.Errorf("expected no unresolved-adapter error when all adapters are known, got: %v", e)
		}
	}
}

// Behavior 4: kernel: namespace keys are NEVER flagged, even when resolver is
// non-empty and doesn't know about them.
func TestValidate_UnresolvedAdapter_KernelPrefixExempt(t *testing.T) {
	nodes := map[string]*graph.Node{
		"proxy": {
			ID:      "proxy",
			Type:    config.NodeTypeInfra,
			Adapter: "kernel:proxy",
		},
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "proxy", Type: config.EdgeDependsOn})

	// Resolver knows go:fiber but NOT kernel:proxy
	resolver := fakeResolver{
		adapters: map[string]graph.ResolvedAdapter{
			"go:fiber": {Name: "go:fiber", Category: "service"},
		},
	}

	errs := g.Validate(resolver)

	for _, e := range errs {
		if e.Rule == "unresolved-adapter" && e.Node == "proxy" {
			t.Errorf("expected kernel:proxy to be exempt from unresolved-adapter rule, got error: %v", e)
		}
	}
}

// ---------------------------------------------------------------------------
// Phase 2 Slice 2 — adapter-category-mismatch rule (issue #20)
// ---------------------------------------------------------------------------

// Behavior 1: A service node using a database-category adapter →
// ValidationError with Rule="adapter-category-mismatch", Severity=error.
func TestValidate_AdapterCategoryMismatch_ServiceWithDatabaseCategory(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "db:postgres",
		},
		"db": {
			ID:      "db",
			Type:    config.NodeTypeInfra,
			Adapter: "db:postgres",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})

	resolver := fakeResolver{
		adapters: map[string]graph.ResolvedAdapter{
			"db:postgres": {Name: "db:postgres", Category: "database"},
		},
	}

	errs := g.Validate(resolver)

	var found *graph.ValidationError
	for i := range errs {
		if errs[i].Rule == "adapter-category-mismatch" && errs[i].Node == "api" {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected ValidationError with Rule=adapter-category-mismatch for service node using database adapter, got: %v", errs)
	}
	if found.Severity != graph.SeverityError {
		t.Errorf("expected SeverityError, got %q", found.Severity)
	}
}

// Behavior 2: An infra node using a backend-category adapter →
// ValidationError with Rule="adapter-category-mismatch".
func TestValidate_AdapterCategoryMismatch_InfraWithBackendCategory(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"myinfra": {
			ID:      "myinfra",
			Type:    config.NodeTypeInfra,
			Adapter: "go:fiber",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "myinfra", Type: config.EdgeDependsOn})

	resolver := fakeResolver{
		adapters: map[string]graph.ResolvedAdapter{
			"go:fiber": {Name: "go:fiber", Category: "backend"},
		},
	}

	errs := g.Validate(resolver)

	var found *graph.ValidationError
	for i := range errs {
		if errs[i].Rule == "adapter-category-mismatch" && errs[i].Node == "myinfra" {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected ValidationError with Rule=adapter-category-mismatch for infra node using backend adapter, got: %v", errs)
	}
	if found.Severity != graph.SeverityError {
		t.Errorf("expected SeverityError, got %q", found.Severity)
	}
}

// Behavior 3: A service node using a backend-category adapter → no
// "adapter-category-mismatch" error. (Valid pairing.)
func TestValidate_AdapterCategoryMismatch_ServiceWithBackendCategory_NoError(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"db": {
			ID:      "db",
			Type:    config.NodeTypeInfra,
			Adapter: "db:postgres",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})

	resolver := fakeResolver{
		adapters: map[string]graph.ResolvedAdapter{
			"go:fiber":    {Name: "go:fiber", Category: "backend"},
			"db:postgres": {Name: "db:postgres", Category: "database"},
		},
	}

	errs := g.Validate(resolver)

	for _, e := range errs {
		if e.Rule == "adapter-category-mismatch" && e.Node == "api" {
			t.Errorf("expected no adapter-category-mismatch for service node with backend adapter, got: %v", e)
		}
	}
}

// Behavior 4: An infra node using a database-category adapter → no
// "adapter-category-mismatch" error. (Valid pairing.)
func TestValidate_AdapterCategoryMismatch_InfraWithDatabaseCategory_NoError(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
		"db": {
			ID:      "db",
			Type:    config.NodeTypeInfra,
			Adapter: "db:postgres",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})

	resolver := fakeResolver{
		adapters: map[string]graph.ResolvedAdapter{
			"go:fiber":    {Name: "go:fiber", Category: "backend"},
			"db:postgres": {Name: "db:postgres", Category: "database"},
		},
	}

	errs := g.Validate(resolver)

	for _, e := range errs {
		if e.Rule == "adapter-category-mismatch" && e.Node == "db" {
			t.Errorf("expected no adapter-category-mismatch for infra node with database adapter, got: %v", e)
		}
	}
}

// Behavior 5 (#20): kernel: adapter keys are exempt from adapter-category-mismatch,
// even when the fakeResolver is non-empty.
func TestValidate_AdapterCategoryMismatch_KernelPrefixExempt(t *testing.T) {
	nodes := map[string]*graph.Node{
		"proxy": {
			ID:      "proxy",
			Type:    config.NodeTypeInfra,
			Adapter: "kernel:proxy",
		},
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "go:fiber",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "proxy", Type: config.EdgeDependsOn})

	// Resolver knows go:fiber but not kernel:proxy
	resolver := fakeResolver{
		adapters: map[string]graph.ResolvedAdapter{
			"go:fiber": {Name: "go:fiber", Category: "backend"},
		},
	}

	errs := g.Validate(resolver)

	for _, e := range errs {
		if e.Rule == "adapter-category-mismatch" && e.Node == "proxy" {
			t.Errorf("expected kernel:proxy to be exempt from adapter-category-mismatch rule, got: %v", e)
		}
	}
}

// Behavior 6: Infra node with cache-category adapter → no error (cache is infra-valid).
// Service node with frontend-category adapter → no error (frontend is service-valid).
func TestValidate_AdapterCategoryMismatch_CacheAndFrontend_NoError(t *testing.T) {
	nodes := map[string]*graph.Node{
		"web": {
			ID:      "web",
			Type:    config.NodeTypeService,
			Adapter: "ui:astro",
		},
		"cache": {
			ID:      "cache",
			Type:    config.NodeTypeInfra,
			Adapter: "cache:redis",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "web", To: "cache", Type: config.EdgeDependsOn})

	resolver := fakeResolver{
		adapters: map[string]graph.ResolvedAdapter{
			"ui:astro":    {Name: "ui:astro", Category: "frontend"},
			"cache:redis": {Name: "cache:redis", Category: "cache"},
		},
	}

	errs := g.Validate(resolver)

	for _, e := range errs {
		if e.Rule == "adapter-category-mismatch" {
			t.Errorf("expected no adapter-category-mismatch errors, got: %v", e)
		}
	}
}

// Behavior 5: EmptyResolver (resolver.Names() returns nil/empty) → unresolved-adapter
// rule is skipped entirely (no errors from this rule).
func TestValidate_UnresolvedAdapter_EmptyResolverSkipsRule(t *testing.T) {
	nodes := map[string]*graph.Node{
		"api": {
			ID:      "api",
			Type:    config.NodeTypeService,
			Adapter: "nonexistent:adapter",
		},
		"db": {
			ID:      "db",
			Type:    config.NodeTypeInfra,
			Adapter: "totally:unknown",
		},
	}
	g := graph.NewTestGraph(nodes)
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})

	errs := g.Validate(graph.EmptyResolver{})

	for _, e := range errs {
		if e.Rule == "unresolved-adapter" {
			t.Errorf("expected unresolved-adapter rule to be skipped with EmptyResolver, got: %v", e)
		}
	}
}
