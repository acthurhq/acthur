package deploy_test

import (
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/deploy"
	"github.com/acthurhq/acthur/internal/graph"
)

func projectionGraph(t *testing.T, nodes map[string]config.NodeConfig, edges []config.EdgeConfig) *graph.Graph {
	t.Helper()
	g, err := graph.Build(&config.Config{Graph: config.GraphConfig{Nodes: nodes, Edges: edges}})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestRemoteProjectionRefusesToDropInfrastructure(t *testing.T) {
	g := projectionGraph(t, map[string]config.NodeConfig{
		"api": {Type: config.NodeTypeService, Adapter: "go:fiber"},
		"db":  {Type: config.NodeTypeInfra, Adapter: "db:postgres"},
	}, []config.EdgeConfig{{From: "api", To: "db", Type: config.EdgeDependsOn}})

	for _, target := range []string{"fly", "railway", "render"} {
		err := deploy.ValidateRemoteProjection(target, g, nil)
		if err == nil || !strings.Contains(err.Error(), "infrastructure nodes db") {
			t.Errorf("%s: expected pointed unsupported-infrastructure error, got %v", target, err)
		}
	}
}

func TestRemoteProjectionAllowsFaithfulProviderShapes(t *testing.T) {
	serviceOnly := projectionGraph(t, map[string]config.NodeConfig{
		"api": {Type: config.NodeTypeService, Adapter: "go:fiber"},
	}, nil)
	for _, target := range []string{"fly", "railway", "render"} {
		if err := deploy.ValidateRemoteProjection(target, serviceOnly, nil); err != nil {
			t.Errorf("%s service-only projection: %v", target, err)
		}
	}

	completeGraph := projectionGraph(t, map[string]config.NodeConfig{
		"api": {Type: config.NodeTypeService, Adapter: "go:fiber"},
		"db":  {Type: config.NodeTypeInfra, Adapter: "db:postgres"},
	}, []config.EdgeConfig{{From: "api", To: "db", Type: config.EdgeDependsOn}})
	if err := deploy.ValidateRemoteProjection("coolify", completeGraph, nil); err != nil {
		t.Fatalf("coolify receives complete Compose topology: %v", err)
	}
}

func TestRemoteProjectionCoolifyCanDeliverWorkloadEnvironment(t *testing.T) {
	g := projectionGraph(t, map[string]config.NodeConfig{"api": {Type: config.NodeTypeService, Adapter: "go:fiber"}}, nil)
	if err := deploy.ValidateRemoteProjection("coolify", g, []string{"APP_SECRET"}); err != nil {
		t.Fatalf("Coolify environment API makes workload env deliverable: %v", err)
	}
}

func TestRemoteProjectionRefusesToDropServiceDependencyTopology(t *testing.T) {
	g := projectionGraph(t, map[string]config.NodeConfig{
		"api":    {Type: config.NodeTypeService, Adapter: "go:fiber"},
		"worker": {Type: config.NodeTypeService, Adapter: "go:fiber"},
	}, []config.EdgeConfig{{From: "worker", To: "api", Type: config.EdgeDependsOn}})

	for _, target := range []string{"fly", "railway", "render"} {
		err := deploy.ValidateRemoteProjection(target, g, nil)
		if err == nil || !strings.Contains(err.Error(), "dependency topology worker -> api") {
			t.Errorf("%s: expected pointed unsupported-dependency error, got %v", target, err)
		}
	}
}
