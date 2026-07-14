package aiagent

import (
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/contract"
	"github.com/acthurhq/acthur/internal/graph"
)

func TestBuildContext_IncludesNodesEdgesAndContracts(t *testing.T) {
	cfg := &config.Config{Project: "demo"}
	g := graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
		"db":  {ID: "db", Type: config.NodeTypeInfra, Adapter: "db:postgres"},
	})
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})

	reg := contract.NewRegistry()
	c := &contract.Contract{
		Name:      "users",
		Version:   "v1",
		Transport: contract.TransportHTTP,
		Endpoints: []contract.Endpoint{{ID: "get_user", Method: "GET", Path: "/users/:id"}},
	}
	if err := reg.Register(c); err != nil {
		t.Fatalf("register contract: %v", err)
	}

	out := BuildContext(cfg, g, reg)

	for _, want := range []string{"demo", "api", "db", "go:fiber", "db:postgres", "users@v1", "get_user", "GET /users/:id"} {
		if !strings.Contains(out, want) {
			t.Errorf("context missing %q\n---\n%s", want, out)
		}
	}
}

func TestBuildContext_NeverIncludesAPIKey(t *testing.T) {
	cfg := &config.Config{Project: "demo", AI: config.AIConfig{Provider: config.ProviderClaude, APIKey: "sk-super-secret"}}
	g := graph.NewTestGraph(map[string]*graph.Node{})
	reg := contract.NewRegistry()

	out := BuildContext(cfg, g, reg)
	if strings.Contains(out, "sk-super-secret") {
		t.Fatal("context leaked the api_key")
	}
}
