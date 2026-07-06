package mcp

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
)

// fakePoller is an injectable HealthPoller — no real network/process calls.
type fakePoller struct {
	fail map[string]error
}

func (f *fakePoller) Poll(node *graph.Node) error {
	if err, ok := f.fail[node.ID]; ok {
		return err
	}
	return nil
}

func healthTestGraph() *graph.Graph {
	return graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
		"web": {ID: "web", Type: config.NodeTypeService, Adapter: "ui:next", Port: 3000},
		"db":  {ID: "db", Type: config.NodeTypeInfra, Adapter: "db:postgres"},
	})
}

func TestServiceHealthTool_AllServices(t *testing.T) {
	poller := &fakePoller{fail: map[string]error{"web": errors.New("connection refused")}}
	tool := NewServiceHealthTool(healthTestGraph(), poller)

	result, err := tool.Handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries := result.([]serviceHealthEntry)
	if len(entries) != 2 {
		t.Fatalf("expected 2 service entries (infra excluded), got %d: %+v", len(entries), entries)
	}
	byNode := map[string]serviceHealthEntry{}
	for _, e := range entries {
		byNode[e.Node] = e
	}
	if !byNode["api"].Healthy {
		t.Fatalf("expected api healthy, got %+v", byNode["api"])
	}
	if byNode["web"].Healthy || byNode["web"].Error == "" {
		t.Fatalf("expected web unhealthy with error, got %+v", byNode["web"])
	}
}

func TestServiceHealthTool_SingleNode(t *testing.T) {
	poller := &fakePoller{}
	tool := NewServiceHealthTool(healthTestGraph(), poller)

	result, err := tool.Handler(json.RawMessage(`{"node":"api"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries := result.([]serviceHealthEntry)
	if len(entries) != 1 || entries[0].Node != "api" {
		t.Fatalf("expected only api, got %+v", entries)
	}
}

func TestServiceHealthTool_UnknownNode(t *testing.T) {
	tool := NewServiceHealthTool(healthTestGraph(), &fakePoller{})
	_, err := tool.Handler(json.RawMessage(`{"node":"nope"}`))
	if err == nil {
		t.Fatal("expected error for unknown node")
	}
}

func TestServiceHealthTool_NonServiceNode(t *testing.T) {
	tool := NewServiceHealthTool(healthTestGraph(), &fakePoller{})
	_, err := tool.Handler(json.RawMessage(`{"node":"db"}`))
	if err == nil {
		t.Fatal("expected error for non-service node")
	}
}
