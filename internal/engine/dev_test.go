package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/acthur/acthur/internal/adapter"
	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/health"
	"github.com/acthur/acthur/internal/process"
)

type fakeDevResolver struct {
	adapters map[string]adapter.Adapter
}

func (f fakeDevResolver) Resolve(key string) (graph.ResolvedAdapter, bool) {
	a, ok := f.adapters[key]
	if !ok {
		return graph.ResolvedAdapter{}, false
	}
	return graph.ResolvedAdapter{Name: a.Name(), Category: string(a.Category())}, true
}

func (f fakeDevResolver) Names() []string {
	names := make([]string, 0, len(f.adapters))
	for key := range f.adapters {
		names = append(names, key)
	}
	return names
}

func (f fakeDevResolver) Adapter(key string) (adapter.Adapter, bool) {
	a, ok := f.adapters[key]
	return a, ok
}

func TestStart_UnknownAdapterFailsValidationBeforeStartup(t *testing.T) {
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {
					Type:    config.NodeTypeService,
					Adapter: "go:unknown",
				},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}

	eng := NewDevEngine(cfg, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"go:fiber": fakeAdapter{name: "go:fiber", category: adapter.CategoryBackend},
		},
	})

	err = eng.Start()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "graph validation failed") {
		t.Fatalf("expected graph validation error, got %v", err)
	}
	if g.Node("api").State() != graph.StatePending {
		t.Fatalf("expected api to remain pending before startup, got %q", g.Node("api").State())
	}
}

func TestStartInfraNode_UsesResolvedAdapterContainerSpecForDockerArgs(t *testing.T) {
	node := &graph.Node{
		ID:      "db",
		Type:    config.NodeTypeInfra,
		Adapter: "db:custom",
		Config: config.NodeConfig{
			Version: "14",
		},
	}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": node})
	pm := &fakeProcessManager{}
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"db:custom": fakeContainerAdapter{},
		},
	})
	eng.pm = pm
	eng.checker = &fakeHealthChecker{}

	if err := eng.startInfraNode(node); err != nil {
		t.Fatalf("start infra node: %v", err)
	}

	want := []string{
		"run", "--rm",
		"--name", "acthur-db",
		"-p", "15432:15432",
		"-v", "custom-db-data:/data",
		"-e", "CUSTOM_DB=db_development",
		"custom/postgres:14",
	}
	if pm.bin != "docker" {
		t.Fatalf("expected docker bin, got %q", pm.bin)
	}
	if !reflect.DeepEqual(pm.args, want) {
		t.Fatalf("docker args mismatch\nwant: %#v\n got: %#v", want, pm.args)
	}
}

func TestResolveNodeEnv_InjectsConnectableEnvOnlyAcrossConnectionEdges(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"}
	web := &graph.Node{ID: "web", Type: config.NodeTypeService, Adapter: "go:fiber"}
	db := &graph.Node{ID: "db", Type: config.NodeTypeInfra, Adapter: "db:connectable"}
	worker := &graph.Node{ID: "worker", Type: config.NodeTypeService, Adapter: "go:fiber"}
	g := graph.NewTestGraph(map[string]*graph.Node{
		"api":    api,
		"web":    web,
		"db":     db,
		"worker": worker,
	})
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})
	g.AddEdge(&graph.Edge{From: "web", To: "db", Type: config.EdgeDataFlow})
	resolver := fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"go:fiber":       fakeAdapter{name: "go:fiber", category: adapter.CategoryBackend},
			"db:connectable": fakeConnectableAdapter{},
		},
	}

	apiEnv := resolveNodeEnv(g, resolver, api)
	if apiEnv["DATABASE_URL"] != "postgres://postgres:postgres@localhost:5432/db_development" {
		t.Fatalf("expected api to receive DATABASE_URL, got %v", apiEnv)
	}
	webEnv := resolveNodeEnv(g, resolver, web)
	if webEnv["DATABASE_URL"] != "postgres://postgres:postgres@localhost:5432/db_development" {
		t.Fatalf("expected web to receive DATABASE_URL, got %v", webEnv)
	}
	workerEnv := resolveNodeEnv(g, resolver, worker)
	if _, ok := workerEnv["DATABASE_URL"]; ok {
		t.Fatalf("expected unrelated worker to receive no DATABASE_URL, got %v", workerEnv)
	}
}

func TestStartInfraNode_UsesDeclaredHealthcheckForReadiness(t *testing.T) {
	node := &graph.Node{
		ID:      "db",
		Type:    config.NodeTypeInfra,
		Adapter: "db:custom",
	}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": node})
	checker := &fakeHealthChecker{}
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"db:custom": fakeContainerAdapter{
				healthcheck: adapter.Healthcheck{
					Test: []string{"CMD-SHELL", "pg_isready -U postgres"},
				},
			},
		},
	})
	eng.pm = &fakeProcessManager{}
	eng.checker = checker

	if err := eng.startInfraNode(node); err != nil {
		t.Fatalf("start infra node: %v", err)
	}

	if checker.strategyName != "exec" {
		t.Fatalf("expected exec readiness strategy, got %q", checker.strategyName)
	}
}

func TestStartInfraNode_FallsBackToTCPReadinessWithoutHealthcheck(t *testing.T) {
	node := &graph.Node{
		ID:      "db",
		Type:    config.NodeTypeInfra,
		Adapter: "db:custom",
	}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": node})
	checker := &fakeHealthChecker{}
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"db:custom": fakeContainerAdapter{},
		},
	})
	eng.pm = &fakeProcessManager{}
	eng.checker = checker

	if err := eng.startInfraNode(node); err != nil {
		t.Fatalf("start infra node: %v", err)
	}

	if checker.strategyName != "tcp" {
		t.Fatalf("expected tcp readiness strategy, got %q", checker.strategyName)
	}
}

type fakeAdapter struct {
	name     string
	category adapter.Category
}

func (f fakeAdapter) Name() string               { return f.name }
func (f fakeAdapter) Category() adapter.Category { return f.category }
func (f fakeAdapter) Detect(dir string) bool     { return false }
func (f fakeAdapter) EnvVars() []adapter.EnvVar  { return nil }

type fakeContainerAdapter struct {
	healthcheck adapter.Healthcheck
}

func (fakeContainerAdapter) Name() string               { return "db:custom" }
func (fakeContainerAdapter) Category() adapter.Category { return adapter.CategoryDatabase }
func (fakeContainerAdapter) Detect(dir string) bool     { return false }
func (fakeContainerAdapter) EnvVars() []adapter.EnvVar  { return nil }

func (f fakeContainerAdapter) Container(ctx adapter.ContainerContext) adapter.ContainerSpec {
	return adapter.ContainerSpec{
		Image: "custom/postgres",
		Tag:   ctx.Version,
		Ports: []int{15432},
		Volumes: []adapter.Volume{
			{Name: "custom-" + ctx.NodeID + "-data", MountPath: "/data"},
		},
		Env: map[string]string{
			"CUSTOM_DB": ctx.NodeID + "_development",
		},
		Healthcheck: f.healthcheck,
	}
}

type fakeConnectableAdapter struct{}

func (fakeConnectableAdapter) Name() string               { return "db:connectable" }
func (fakeConnectableAdapter) Category() adapter.Category { return adapter.CategoryDatabase }
func (fakeConnectableAdapter) Detect(dir string) bool     { return false }
func (fakeConnectableAdapter) EnvVars() []adapter.EnvVar  { return nil }

func (fakeConnectableAdapter) ConnectionEnv(ctx adapter.ContainerContext) map[string]string {
	return map[string]string{
		"DATABASE_URL": "postgres://postgres:postgres@localhost:5432/" + ctx.NodeID + "_development",
	}
}

type fakeProcessManager struct {
	bin  string
	args []string
}

func (f *fakeProcessManager) Spawn(nodeID, bin string, args []string, env map[string]string, dir string) (*process.Process, error) {
	f.bin = bin
	f.args = append([]string(nil), args...)
	return nil, nil
}

func (f *fakeProcessManager) StopAll(nodeIDs []string) {}

type fakeHealthChecker struct {
	strategyName string
}

func (f *fakeHealthChecker) WaitFor(ctx context.Context, node *graph.Node, timeout time.Duration) error {
	return nil
}

func (f *fakeHealthChecker) WaitForStrategy(ctx context.Context, node *graph.Node, strategy health.Strategy, timeout time.Duration) error {
	f.strategyName = strategy.Name()
	return nil
}
