package engine

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/acthurhq/acthur/internal/adapter"
	"github.com/acthurhq/acthur/internal/adapter/backend/gofiber"
	"github.com/acthurhq/acthur/internal/adapter/infra/postgres"
	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/flags"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/health"
	"github.com/acthurhq/acthur/internal/plugin"
	"github.com/acthurhq/acthur/internal/process"
	"github.com/acthurhq/acthur/internal/secrets"
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

// TestStartNode_SkipsKernelMaterializedProxyNode: the graph always materializes
// the kernel proxy node ("kernel:proxy"). The engine runs the proxy itself —
// startNode must not try to resolve kernel-namespaced nodes through the adapter
// resolver (the #33 live witness failed startup on exactly this).
func TestStartNode_SkipsKernelMaterializedProxyNode(t *testing.T) {
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber"},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	proxyNode := g.Node("proxy")
	if proxyNode == nil {
		t.Fatal("expected materialized proxy node in graph")
	}

	pm := &fakeProcessManager{}
	eng := NewDevEngine(cfg, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"go:fiber": fakeAdapter{name: "go:fiber", category: adapter.CategoryBackend},
		},
	})
	eng.pm = pm
	eng.checker = &fakeHealthChecker{}

	if err := eng.startNode(proxyNode); err != nil {
		t.Fatalf("expected kernel proxy node to be skipped, got error: %v", err)
	}
	if pm.bin != "" {
		t.Fatalf("expected no process spawned for kernel proxy node, got %q", pm.bin)
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

// TestShutdown_StopsStartedContainersViaDockerStop: killing the docker-run
// client process does not stop the container it launched. Shutdown must issue
// `docker stop` for every container the engine started (found by the #33
// live witness: teardown reported "stopped" while acthur-db kept running).
func TestShutdown_StopsStartedContainersViaDockerStop(t *testing.T) {
	node := &graph.Node{
		ID:      "db",
		Type:    config.NodeTypeInfra,
		Adapter: "db:custom",
		Config:  config.NodeConfig{Version: "14"},
	}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": node})
	docker := &fakeDockerRunner{}
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"db:custom": fakeContainerAdapter{},
		},
	})
	eng.pm = &fakeProcessManager{}
	eng.checker = &fakeHealthChecker{}
	eng.runDocker = docker.run

	if err := eng.startInfraNode(node); err != nil {
		t.Fatalf("start infra node: %v", err)
	}
	eng.shutdown([]*graph.Node{node})

	want := [][]string{{"stop", "acthur-db"}}
	if !reflect.DeepEqual(docker.calls, want) {
		t.Fatalf("docker stop calls mismatch\nwant: %#v\n got: %#v", want, docker.calls)
	}
}

type fakeDockerRunner struct {
	calls [][]string
}

func (f *fakeDockerRunner) run(args ...string) error {
	f.calls = append(f.calls, append([]string(nil), args...))
	return nil
}

// TestResolveNodeEnv_InjectsConnectableEnvOnlyAcrossDependsOnEdges asserts
// Connectable env (DATABASE_URL, etc.) is injected along depends_on edges
// only. data_flow edges are contract-governed traffic (ADR 0012) — they get
// a proxy flow route URL instead, never a Connectable env, even when they
// target the same infra node a depends_on edge also reaches.
func TestResolveNodeEnv_InjectsConnectableEnvOnlyAcrossDependsOnEdges(t *testing.T) {
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

	secrets := fakeSecretStore{}
	apiEnv, err := resolveNodeEnv(g, resolver, api, secrets, 4000)
	if err != nil {
		t.Fatalf("resolve api env: %v", err)
	}
	if apiEnv["DATABASE_URL"] != "postgres://postgres:postgres@localhost:5432/db_development" {
		t.Fatalf("expected api (depends_on) to receive DATABASE_URL, got %v", apiEnv)
	}
	webEnv, err := resolveNodeEnv(g, resolver, web, secrets, 4000)
	if err != nil {
		t.Fatalf("resolve web env: %v", err)
	}
	if _, ok := webEnv["DATABASE_URL"]; ok {
		t.Fatalf("expected web (data_flow) to receive no Connectable DATABASE_URL, got %v", webEnv)
	}
	workerEnv, err := resolveNodeEnv(g, resolver, worker, secrets, 4000)
	if err != nil {
		t.Fatalf("resolve worker env: %v", err)
	}
	if _, ok := workerEnv["DATABASE_URL"]; ok {
		t.Fatalf("expected unrelated worker to receive no DATABASE_URL, got %v", workerEnv)
	}
}

// TestResolveNodeEnv_DataFlowAndDependsOnEdgesDiverge is the direct assertion
// that the two edge kinds resolve to different discovery URLs for the same
// devPort/target port (ADR 0012): data_flow points at the proxy's flow
// route, depends_on keeps the direct node-port URL.
func TestResolveNodeEnv_DataFlowAndDependsOnEdgesDiverge(t *testing.T) {
	web := &graph.Node{ID: "web", Type: config.NodeTypeService, Adapter: "go:fiber"}
	worker := &graph.Node{ID: "worker", Type: config.NodeTypeService, Adapter: "go:fiber"}
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080}
	g := graph.NewTestGraph(map[string]*graph.Node{
		"web":    web,
		"worker": worker,
		"api":    api,
	})
	g.AddEdge(&graph.Edge{From: "web", To: "api", Type: config.EdgeDataFlow, Contracts: []string{"api"}})
	g.AddEdge(&graph.Edge{From: "worker", To: "api", Type: config.EdgeDependsOn})

	resolver := fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"go:fiber": fakeAdapter{name: "go:fiber", category: adapter.CategoryBackend},
		},
	}
	secrets := fakeSecretStore{}

	webEnv, err := resolveNodeEnv(g, resolver, web, secrets, 4000)
	if err != nil {
		t.Fatalf("resolve web env: %v", err)
	}
	if got := webEnv["API_URL"]; got != "http://localhost:4000/_flow/web/api" {
		t.Fatalf("expected data_flow edge to resolve to the proxy flow route, got %q", got)
	}

	workerEnv, err := resolveNodeEnv(g, resolver, worker, secrets, 4000)
	if err != nil {
		t.Fatalf("resolve worker env: %v", err)
	}
	if got := workerEnv["API_URL"]; got != "http://localhost:8080" {
		t.Fatalf("expected depends_on edge to resolve to the direct node URL, got %q", got)
	}
}

// TestResolveNodeEnv_WitnessGraphBootsWithRealAdapters is the #33 witness at the
// unit level: it composes the real go:fiber + db:postgres adapters exactly as the
// Go Fiber + Postgres witness project does and asserts the service receives an env
// that won't panic on boot — APP_SECRET synthesized, DATABASE_URL with sslmode.
func TestResolveNodeEnv_WitnessGraphBootsWithRealAdapters(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080}
	db := &graph.Node{ID: "db", Type: config.NodeTypeInfra, Adapter: "db:postgres"}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api, "db": db})
	g.AddEdge(&graph.Edge{From: "api", To: "db", Type: config.EdgeDependsOn})

	resolver := fakeDevResolver{adapters: map[string]adapter.Adapter{
		"go:fiber":    &gofiber.Adapter{},
		"db:postgres": &postgres.Adapter{},
	}}

	env, err := resolveNodeEnv(g, resolver, api, fakeSecretStore{}, 4000)
	if err != nil {
		t.Fatalf("resolve api env: %v", err)
	}

	if env["APP_SECRET"] == "" {
		t.Fatal("expected APP_SECRET to be synthesized (config.Load mustGetEnv would panic otherwise)")
	}
	if env["APP_ENV"] != "development" {
		t.Fatalf("expected APP_ENV default, got %q", env["APP_ENV"])
	}
	if env["APP_PORT"] != "8080" {
		t.Fatalf("expected APP_PORT from node port, got %q", env["APP_PORT"])
	}
	if got := env["DATABASE_URL"]; !strings.Contains(got, "sslmode=disable") {
		t.Fatalf("expected DATABASE_URL to disable sslmode for local dev, got %q", got)
	}
}

func TestResolveNodeEnv_AppliesAdapterDefaultsAndSynthesizesSecrets(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:env", Port: 8080}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	resolver := fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"go:env": fakeEnvAdapter{vars: []adapter.EnvVar{
				{Key: "APP_ENV", Default: "development"},
				{Key: "APP_PORT", Default: "8080"}, // must not clobber the node's real port
				{Key: "APP_SECRET", Required: true, Secret: true, Generate: true},
				{Key: "DATABASE_URL", Required: true, Secret: true}, // no default, not generated → left unset
			}},
		},
	}

	secrets := fakeSecretStore{}
	env, err := resolveNodeEnv(g, resolver, api, secrets, 4000)
	if err != nil {
		t.Fatalf("resolve env: %v", err)
	}

	if env["APP_ENV"] != "development" {
		t.Fatalf("expected APP_ENV default applied, got %q", env["APP_ENV"])
	}
	if env["APP_PORT"] != "8080" {
		t.Fatalf("expected APP_PORT to stay the node port, got %q", env["APP_PORT"])
	}
	if env["APP_SECRET"] != "secret::api::APP_SECRET" {
		t.Fatalf("expected synthesized APP_SECRET, got %q", env["APP_SECRET"])
	}
	if _, ok := env["DATABASE_URL"]; ok {
		t.Fatalf("expected DATABASE_URL unset (no default, not generated), got %q", env["DATABASE_URL"])
	}
}

// fakeProjectSecretStore is an in-memory projectSecretStore for tests that
// don't want to touch disk.
type fakeProjectSecretStore map[string]string

func (f fakeProjectSecretStore) All() (map[string]string, error) { return map[string]string(f), nil }

// fakeProjectFlagStore is an in-memory projectFlagStore for tests.
type fakeProjectFlagStore []flags.Flag

func (f fakeProjectFlagStore) List() ([]flags.Flag, error) { return []flags.Flag(f), nil }

func TestBuildEnv_MergesProjectSecretsWithoutClobberingEngineKeys(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:env", Port: 8080}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	resolver := fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"go:env": fakeEnvAdapter{vars: []adapter.EnvVar{
				{Key: "APP_ENV", Default: "development"},
			}},
		},
	}

	eng := NewDevEngine(&config.Config{Dev: config.DevConfig{Port: 4000}}, g, resolver)
	eng.projectSecrets = fakeProjectSecretStore{
		"STRIPE_KEY": "sk_test_123",
		// A project secret sharing a name with an engine-owned key must never
		// win — PORT/APP_PORT are set by the engine, not the secret store.
		"PORT": "9999",
	}

	env, err := eng.buildEnv(api)
	if err != nil {
		t.Fatalf("buildEnv: %v", err)
	}
	if env["STRIPE_KEY"] != "sk_test_123" {
		t.Fatalf("expected project secret injected, got %q", env["STRIPE_KEY"])
	}
	if env["PORT"] != "8080" {
		t.Fatalf("expected engine-owned PORT to win over project secret, got %q", env["PORT"])
	}
}

func TestBuildEnv_InjectsFeatureFlagsAsEnv(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:env", Port: 8080}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	resolver := fakeDevResolver{
		adapters: map[string]adapter.Adapter{"go:env": fakeEnvAdapter{}},
	}

	eng := NewDevEngine(&config.Config{Dev: config.DevConfig{Port: 4000}}, g, resolver)
	eng.projectFlags = fakeProjectFlagStore{
		{Name: "new-booking-flow", Enabled: true},
		{Name: "old-flow", Enabled: false},
	}

	env, err := eng.buildEnv(api)
	if err != nil {
		t.Fatalf("buildEnv: %v", err)
	}
	if env["ACTHUR_FLAG_NEW_BOOKING_FLOW"] != "true" {
		t.Fatalf("expected enabled flag env = true, got %q", env["ACTHUR_FLAG_NEW_BOOKING_FLOW"])
	}
	if env["ACTHUR_FLAG_OLD_FLOW"] != "false" {
		t.Fatalf("expected disabled flag env = false, got %q", env["ACTHUR_FLAG_OLD_FLOW"])
	}
}

func TestBuildEnv_NilProjectStoresAreNoOp(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:env", Port: 8080}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	resolver := fakeDevResolver{
		adapters: map[string]adapter.Adapter{"go:env": fakeEnvAdapter{}},
	}

	// cfg.RootDir == "" (the common unit-test shape) leaves projectSecrets and
	// projectFlags nil — buildEnv must not panic or error.
	eng := NewDevEngine(&config.Config{Dev: config.DevConfig{Port: 4000}}, g, resolver)
	if _, err := eng.buildEnv(api); err != nil {
		t.Fatalf("buildEnv with nil project stores: %v", err)
	}
}

func TestNewDevEngine_WiresRealProjectStoresFromRootDir(t *testing.T) {
	root := t.TempDir()
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:env", Port: 8080}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	resolver := fakeDevResolver{
		adapters: map[string]adapter.Adapter{"go:env": fakeEnvAdapter{}},
	}

	realSecrets := secrets.New(root)
	if err := realSecrets.Set("STRIPE_KEY", "sk_live_xyz"); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
	realFlags := flags.New(root)
	if err := realFlags.Create("new-booking-flow", ""); err != nil {
		t.Fatalf("seed flag: %v", err)
	}
	if err := realFlags.Enable("new-booking-flow"); err != nil {
		t.Fatalf("enable flag: %v", err)
	}

	eng := NewDevEngine(&config.Config{RootDir: root, Dev: config.DevConfig{Port: 4000}}, g, resolver)
	env, err := eng.buildEnv(api)
	if err != nil {
		t.Fatalf("buildEnv: %v", err)
	}
	if env["STRIPE_KEY"] != "sk_live_xyz" {
		t.Fatalf("expected real secrets.Store wired from RootDir, got %q", env["STRIPE_KEY"])
	}
	if env["ACTHUR_FLAG_NEW_BOOKING_FLOW"] != "true" {
		t.Fatalf("expected real flags.Store wired from RootDir, got %q", env["ACTHUR_FLAG_NEW_BOOKING_FLOW"])
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

func TestStartInfraNode_HealthcheckTargetsProjectScopedContainerName(t *testing.T) {
	node := &graph.Node{
		ID:      "db",
		Type:    config.NodeTypeInfra,
		Adapter: "db:custom",
	}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": node})
	checker := &fakeHealthChecker{}
	eng := NewDevEngine(&config.Config{Project: "shop"}, g, fakeDevResolver{
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

	exec, ok := checker.strategy.(*health.ExecStrategy)
	if !ok {
		t.Fatalf("expected exec strategy, got %T", checker.strategy)
	}
	for _, arg := range exec.Args {
		if arg == "acthur-db" {
			t.Fatalf("exec healthcheck targeted unscoped container name %q, want project-scoped %q; args=%v", arg, "acthur-shop-db", exec.Args)
		}
	}
	found := false
	for _, arg := range exec.Args {
		if arg == "acthur-shop-db" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected exec healthcheck to target %q, got args=%v", "acthur-shop-db", exec.Args)
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

// fakeSecretStore returns deterministic values so env resolution is assertable
// without touching disk.
type fakeSecretStore struct{}

func (fakeSecretStore) Secret(nodeID, key string) (string, error) {
	return "secret::" + nodeID + "::" + key, nil
}

type fakeEnvAdapter struct {
	vars []adapter.EnvVar
}

func (fakeEnvAdapter) Name() string                { return "go:env" }
func (fakeEnvAdapter) Category() adapter.Category  { return adapter.CategoryBackend }
func (fakeEnvAdapter) Detect(dir string) bool      { return false }
func (f fakeEnvAdapter) EnvVars() []adapter.EnvVar { return f.vars }

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
	bin        string
	args       []string
	spawnErr   error
	stopped    []string
	restarted  []string
	restartErr error
}

func (f *fakeProcessManager) Spawn(nodeID, bin string, args []string, env map[string]string, dir string) (*process.Process, error) {
	if f.spawnErr != nil {
		return nil, f.spawnErr
	}
	f.bin = bin
	f.args = append([]string(nil), args...)
	return nil, nil
}

func (f *fakeProcessManager) Restart(nodeID string) error {
	if f.restartErr != nil {
		return f.restartErr
	}
	f.restarted = append(f.restarted, nodeID)
	return nil
}

func (f *fakeProcessManager) StopAll(nodeIDs []string) {
	f.stopped = append([]string(nil), nodeIDs...)
}

type fakeHealthChecker struct {
	strategyName string
	strategy     health.Strategy
	err          error
}

func (f *fakeHealthChecker) WaitFor(ctx context.Context, node *graph.Node, timeout time.Duration) error {
	return f.err
}

func (f *fakeHealthChecker) WaitForStrategy(ctx context.Context, node *graph.Node, strategy health.Strategy, timeout time.Duration) error {
	f.strategyName = strategy.Name()
	f.strategy = strategy
	return f.err
}

// ---------------------------------------------------------------------------
// Plugin bus lifecycle event tests (#40)
// ---------------------------------------------------------------------------

// recordingBus subscribes to every node lifecycle event and records the
// sequence of (event, nodeID) pairs observed, in order.
func recordingBus() (*plugin.Bus, *[]string) {
	bus := plugin.NewBus()
	seq := []string{}
	record := func(e plugin.Event) func(plugin.EventPayload) {
		return func(p plugin.EventPayload) {
			seq = append(seq, string(e)+":"+p.NodeID)
		}
	}
	for _, e := range []plugin.Event{
		plugin.EventBeforeNodeStart,
		plugin.EventAfterNodeStart,
		plugin.EventAfterNodeHealthy,
		plugin.EventBeforeNodeStop,
		plugin.EventAfterNodeStop,
		plugin.EventOnNodeFailure,
	} {
		bus.On(e, record(e))
	}
	return bus, &seq
}

// TestStartInfraNode_HealthyEmitsBeforeStartAfterStartAfterHealthy asserts the
// exact event sequence for a successful infra start: before_start (entry),
// after_start (process spawned), after_healthy (health wait succeeded).
func TestStartInfraNode_HealthyEmitsBeforeStartAfterStartAfterHealthy(t *testing.T) {
	node := &graph.Node{
		ID:      "db",
		Type:    config.NodeTypeInfra,
		Adapter: "db:custom",
		Config:  config.NodeConfig{Version: "14"},
	}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": node})
	bus, seq := recordingBus()
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{"db:custom": fakeContainerAdapter{}},
	}, WithBus(bus))
	eng.pm = &fakeProcessManager{}
	eng.checker = &fakeHealthChecker{}

	if err := eng.startInfraNode(node); err != nil {
		t.Fatalf("start infra node: %v", err)
	}

	want := []string{
		"kernel:node:before_start:db",
		"kernel:node:after_start:db",
		"kernel:node:after_healthy:db",
	}
	if !reflect.DeepEqual(*seq, want) {
		t.Fatalf("event sequence mismatch\nwant: %#v\n got: %#v", want, *seq)
	}
}

// TestStartInfraNode_HealthFailureEmitsOnFailureNotAfterHealthy asserts a
// failed health wait emits on_failure instead of after_healthy — after_start
// still fires because the process really was spawned.
func TestStartInfraNode_HealthFailureEmitsOnFailureNotAfterHealthy(t *testing.T) {
	node := &graph.Node{
		ID:      "db",
		Type:    config.NodeTypeInfra,
		Adapter: "db:custom",
		Config:  config.NodeConfig{Version: "14"},
	}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": node})
	bus, seq := recordingBus()
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{"db:custom": fakeContainerAdapter{}},
	}, WithBus(bus))
	eng.pm = &fakeProcessManager{}
	eng.checker = &fakeHealthChecker{err: fmt.Errorf("health check timed out")}

	if err := eng.startInfraNode(node); err == nil {
		t.Fatal("expected health failure error")
	}

	want := []string{
		"kernel:node:before_start:db",
		"kernel:node:after_start:db",
		"kernel:node:on_failure:db",
	}
	if !reflect.DeepEqual(*seq, want) {
		t.Fatalf("event sequence mismatch\nwant: %#v\n got: %#v", want, *seq)
	}
}

// TestStartInfraNode_SpawnFailureEmitsOnFailureOnly asserts a process spawn
// failure emits on_failure without ever emitting after_start.
func TestStartInfraNode_SpawnFailureEmitsOnFailureOnly(t *testing.T) {
	node := &graph.Node{
		ID:      "db",
		Type:    config.NodeTypeInfra,
		Adapter: "db:custom",
		Config:  config.NodeConfig{Version: "14"},
	}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": node})
	bus, seq := recordingBus()
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{"db:custom": fakeContainerAdapter{}},
	}, WithBus(bus))
	eng.pm = &fakeProcessManager{spawnErr: fmt.Errorf("docker not found")}
	eng.checker = &fakeHealthChecker{}

	if err := eng.startInfraNode(node); err == nil {
		t.Fatal("expected spawn failure error")
	}

	want := []string{
		"kernel:node:before_start:db",
		"kernel:node:on_failure:db",
	}
	if !reflect.DeepEqual(*seq, want) {
		t.Fatalf("event sequence mismatch\nwant: %#v\n got: %#v", want, *seq)
	}
}

// TestStartServiceNode_HealthyEmitsFullLifecycle mirrors the infra happy path
// for a native service process using the real go:fiber adapter.
func TestStartServiceNode_HealthyEmitsFullLifecycle(t *testing.T) {
	node := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": node})
	bus, seq := recordingBus()
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{"go:fiber": &gofiber.Adapter{}},
	}, WithBus(bus))
	eng.pm = &fakeProcessManager{}
	eng.checker = &fakeHealthChecker{}

	if err := eng.startServiceNode(node); err != nil {
		t.Fatalf("start service node: %v", err)
	}

	want := []string{
		"kernel:node:before_start:api",
		"kernel:node:after_start:api",
		"kernel:node:after_healthy:api",
	}
	if !reflect.DeepEqual(*seq, want) {
		t.Fatalf("event sequence mismatch\nwant: %#v\n got: %#v", want, *seq)
	}
}

// TestStartServiceNode_HealthFailureEmitsOnFailure asserts a service that
// spawns but never turns healthy emits on_failure, not after_healthy.
func TestStartServiceNode_HealthFailureEmitsOnFailure(t *testing.T) {
	node := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": node})
	bus, seq := recordingBus()
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{"go:fiber": &gofiber.Adapter{}},
	}, WithBus(bus))
	eng.pm = &fakeProcessManager{}
	eng.checker = &fakeHealthChecker{err: fmt.Errorf("timed out waiting for health")}

	if err := eng.startServiceNode(node); err == nil {
		t.Fatal("expected health failure error")
	}

	want := []string{
		"kernel:node:before_start:api",
		"kernel:node:after_start:api",
		"kernel:node:on_failure:api",
	}
	if !reflect.DeepEqual(*seq, want) {
		t.Fatalf("event sequence mismatch\nwant: %#v\n got: %#v", want, *seq)
	}
}

// TestStartNode_KernelMaterializedNodeEmitsNoEvents: kernel:* nodes never
// resolve through the adapter registry and must never emit lifecycle events
// either — the engine doesn't "start" them, it runs them itself (the proxy).
func TestStartNode_KernelMaterializedNodeEmitsNoEvents(t *testing.T) {
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber"},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	proxyNode := g.Node("proxy")
	bus, seq := recordingBus()
	eng := NewDevEngine(cfg, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"go:fiber": fakeAdapter{name: "go:fiber", category: adapter.CategoryBackend},
		},
	}, WithBus(bus))
	eng.pm = &fakeProcessManager{}
	eng.checker = &fakeHealthChecker{}

	if err := eng.startNode(proxyNode); err != nil {
		t.Fatalf("expected kernel proxy node to be skipped, got error: %v", err)
	}
	if len(*seq) != 0 {
		t.Fatalf("expected no events for kernel-materialized node, got %#v", *seq)
	}
}

// TestShutdown_EmitsBeforeAndAfterStopForRealNodesOnly asserts shutdown emits
// before_stop/after_stop for infra/service nodes in reverse startup order,
// and never for the kernel-materialized proxy node.
func TestShutdown_EmitsBeforeAndAfterStopForRealNodesOnly(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"}
	db := &graph.Node{ID: "db", Type: config.NodeTypeInfra, Adapter: "db:custom"}
	proxyNode := &graph.Node{ID: "proxy", Type: config.NodeTypeInfra, Adapter: "kernel:proxy"}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api, "db": db, "proxy": proxyNode})
	bus, seq := recordingBus()
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{}, WithBus(bus))
	eng.pm = &fakeProcessManager{}
	eng.checker = &fakeHealthChecker{}
	eng.runDocker = func(args ...string) error { return nil }

	order := []*graph.Node{db, api, proxyNode}
	eng.shutdown(order)

	// The kernel proxy node must never appear.
	for _, s := range *seq {
		if strings.Contains(s, ":proxy") {
			t.Fatalf("expected no stop events for kernel-materialized proxy node, got %#v", *seq)
		}
	}
	want := []string{
		"kernel:node:before_stop:api",
		"kernel:node:before_stop:db",
		"kernel:node:after_stop:api",
		"kernel:node:after_stop:db",
	}
	if !reflect.DeepEqual(*seq, want) {
		t.Fatalf("event sequence mismatch\nwant: %#v\n got: %#v", want, *seq)
	}
}

// TestStartInfraNode_PanickingHandlerDoesNotBreakStartup: a subscribed
// handler that panics must not crash the engine or prevent the rest of the
// lifecycle from proceeding normally.
func TestStartInfraNode_PanickingHandlerDoesNotBreakStartup(t *testing.T) {
	node := &graph.Node{
		ID:      "db",
		Type:    config.NodeTypeInfra,
		Adapter: "db:custom",
		Config:  config.NodeConfig{Version: "14"},
	}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": node})
	bus := plugin.NewBus()
	bus.On(plugin.EventBeforeNodeStart, func(p plugin.EventPayload) {
		panic("plugin handler exploded")
	})
	healthyReached := false
	bus.On(plugin.EventAfterNodeHealthy, func(p plugin.EventPayload) {
		healthyReached = true
	})
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{"db:custom": fakeContainerAdapter{}},
	}, WithBus(bus))
	eng.pm = &fakeProcessManager{}
	eng.checker = &fakeHealthChecker{}

	if err := eng.startInfraNode(node); err != nil {
		t.Fatalf("start infra node: %v", err)
	}
	if !healthyReached {
		t.Fatal("expected after_healthy handler to still run despite an earlier handler panicking")
	}
}

// TestNewDevEngine_NilBusEmitsNothingAndDoesNotPanic asserts the zero-value
// case: no WithBus option means no *plugin.Bus, and node lifecycle emission
// is simply a no-op rather than a nil-pointer panic.
func TestNewDevEngine_NilBusEmitsNothingAndDoesNotPanic(t *testing.T) {
	node := &graph.Node{
		ID:      "db",
		Type:    config.NodeTypeInfra,
		Adapter: "db:custom",
		Config:  config.NodeConfig{Version: "14"},
	}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": node})
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{"db:custom": fakeContainerAdapter{}},
	})
	eng.pm = &fakeProcessManager{}
	eng.checker = &fakeHealthChecker{}

	if err := eng.startInfraNode(node); err != nil {
		t.Fatalf("start infra node: %v", err)
	}
}
