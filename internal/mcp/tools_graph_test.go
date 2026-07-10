package mcp

import (
	"encoding/json"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
)

func testGraph() *graph.Graph {
	g := graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
		"db":  {ID: "db", Type: config.NodeTypeInfra, Adapter: "db:postgres"},
	})
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn, Contracts: []string{"users"}})
	return g
}

func callTool(t *testing.T, tool Tool, args string) graphQueryResultRaw {
	t.Helper()
	var raw json.RawMessage
	if args != "" {
		raw = json.RawMessage(args)
	}
	result, err := tool.Handler(raw)
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var out graphQueryResultRaw
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return out
}

// graphQueryResultRaw mirrors graphQueryResult loosely typed for assertions.
type graphQueryResultRaw struct {
	Nodes []map[string]any `json:"nodes"`
	Edges []map[string]any `json:"edges"`
}

func TestGraphQueryTool_NoFilter(t *testing.T) {
	tool := NewGraphQueryTool(testGraph())
	out := callTool(t, tool, "")
	if len(out.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %+v", len(out.Nodes), out.Nodes)
	}
	if len(out.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(out.Edges))
	}
	if out.Edges[0]["contracts"] == nil {
		t.Fatalf("expected contracts on edge, got %+v", out.Edges[0])
	}
}

func TestGraphQueryTool_FilterByType(t *testing.T) {
	tool := NewGraphQueryTool(testGraph())
	out := callTool(t, tool, `{"type":"service"}`)
	if len(out.Nodes) != 1 || out.Nodes[0]["id"] != "api" {
		t.Fatalf("expected only 'api' node, got %+v", out.Nodes)
	}
}

func TestGraphQueryTool_InvalidFilter(t *testing.T) {
	tool := NewGraphQueryTool(testGraph())
	_, err := tool.Handler(json.RawMessage(`{"type":"bogus"}`))
	if err == nil {
		t.Fatal("expected error for invalid type filter")
	}
}

func TestGraphQueryTool_DescriptorHasSchema(t *testing.T) {
	tool := NewGraphQueryTool(testGraph())
	d := tool.descriptor()
	if d.Name != "graph_query" {
		t.Fatalf("got name %q", d.Name)
	}
	if d.InputSchema == nil {
		t.Fatal("expected non-nil input schema")
	}
}
