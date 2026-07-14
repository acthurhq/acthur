package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/output"
	"github.com/acthurhq/acthur/internal/plugin"
)

// ---------------------------------------------------------------------------
// fixtureKernelPlugin — a test-local plugin whose Register method exercises
// every KernelAPI method: OnEvent, RegisterCommand, RegisterGenerator,
// RegisterSchema, RegisterMiddleware, AddNode, AddEdge, Graph, Log.
// It stashes the KernelAPI handle it received so tests can introspect the
// kernel-side registries afterward.
// ---------------------------------------------------------------------------

type fixtureKernelPlugin struct {
	name      string
	version   string
	dependsOn []string

	kernelAPI  plugin.KernelAPI
	hookFired  bool
	registered bool
}

func (p *fixtureKernelPlugin) Name() string        { return p.name }
func (p *fixtureKernelPlugin) Version() string     { return p.version }
func (p *fixtureKernelPlugin) DependsOn() []string { return p.dependsOn }

func (p *fixtureKernelPlugin) Register(k plugin.KernelAPI) error {
	p.registered = true
	p.kernelAPI = k

	k.OnEvent(plugin.EventAfterNodeHealthy, func(payload plugin.EventPayload) {
		p.hookFired = true
	})

	k.RegisterCommand(plugin.CLICommand{
		Use:   p.name + "-cmd",
		Short: "fixture command for " + p.name,
		Run:   func(args []string) error { return nil },
	})

	k.RegisterGenerator(p.name+"-target", &fixtureGenerator{})
	k.RegisterSchema(p.name+"-schema", plugin.SchemaDefinition{
		Name:   p.name + "-schema",
		Fields: map[string]plugin.FieldDef{"id": {Type: "string", Required: true}},
	})
	k.RegisterMiddleware(plugin.Middleware{Name: p.name + "-mw", Priority: 5})

	k.AddNode(&graph.Node{ID: p.name + "-node", Type: config.NodeTypeInfra})
	k.AddEdge(&graph.Edge{From: "api", To: p.name + "-node", Type: config.EdgeDependsOn})

	k.Log(plugin.LogInfo, "fixture plugin %s registered", p.name)

	return nil
}

type fixtureGenerator struct{}

func (g *fixtureGenerator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	return nil, nil
}
func (g *fixtureGenerator) SupportedAdapters() []string { return []string{"gofiber"} }

// vetangleConfig loads the shared vetangle fixture config, used across
// existing cmd/acthur tests, as a base for plugin-loading tests.
func vetangleConfig(t *testing.T) *config.Config {
	t.Helper()
	root := repoRoot(t)
	cfg, err := config.LoadFile(filepath.Join(root, "testdata", "vetangle", "acthur.yml"))
	if err != nil {
		t.Fatalf("load vetangle fixture: %v", err)
	}
	return cfg
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestLoadPlugins_FixturePlugin_ExercisesEveryKernelAPIMethod(t *testing.T) {
	cleanup := registerFixturePlugins(t, &fixtureKernelPlugin{name: "kernel-fixture", version: "0.1.0"})
	defer cleanup()

	cfg := vetangleConfig(t)
	cfg.Plugins = []config.PluginEntry{{Name: "kernel-fixture"}}

	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}

	if err := loadPlugins(cfg, g); err != nil {
		t.Fatalf("loadPlugins: %v", err)
	}
	t.Cleanup(func() { removeRootCommand(t, "kernel-fixture-cmd") })

	if len(loadedPlugins) != 1 || loadedPlugins[0].Plugin.Name() != "kernel-fixture" {
		t.Fatalf("expected kernel-fixture in loadedPlugins, got %+v", loadedPlugins)
	}

	// Command reachable from Root.
	found := false
	for _, c := range Root.Commands() {
		if c.Use == "kernel-fixture-cmd" {
			found = true
		}
	}
	if !found {
		t.Error("expected plugin command 'kernel-fixture-cmd' to be attached to Root")
	}

	// Node + edge landed in the graph (pre-seal).
	if g.Node("kernel-fixture-node") == nil {
		t.Error("expected plugin-added node to land in the graph")
	}
	edges := g.EdgesFrom("api")
	edgeLanded := false
	for _, e := range edges {
		if e.To == "kernel-fixture-node" {
			edgeLanded = true
		}
	}
	if !edgeLanded {
		t.Error("expected plugin-added edge to land in the graph")
	}

	// OnEvent handler fires when the bus emits.
	fp := registeredFixture(t, "kernel-fixture")
	kernelBus.Emit(plugin.EventAfterNodeHealthy, plugin.EventPayload{NodeID: "kernel-fixture-node"})
	if !fp.hookFired {
		t.Error("expected OnEvent handler to fire on bus.Emit")
	}

	// Generator/schema/middleware retrievable through the KernelAPI handle
	// the plugin was given.
	impl, ok := fp.kernelAPI.(*plugin.KernelAPIImpl)
	if !ok {
		t.Fatalf("expected KernelAPI handle to be *plugin.KernelAPIImpl, got %T", fp.kernelAPI)
	}
	if _, ok := impl.Generator("kernel-fixture-target"); !ok {
		t.Error("expected registered generator to be retrievable")
	}
	if _, ok := impl.Schema("kernel-fixture-schema"); !ok {
		t.Error("expected registered schema to be retrievable")
	}
	mws := impl.Middlewares()
	mwFound := false
	for _, m := range mws {
		if m.Name == "kernel-fixture-mw" {
			mwFound = true
		}
	}
	if !mwFound {
		t.Error("expected registered middleware to be retrievable")
	}
}

func TestLoadPlugins_UnknownPlugin_ErrorsCleanlyBeforeAnythingStarts(t *testing.T) {
	cleanup := registerFixturePlugins(t, &fixtureKernelPlugin{name: "known-plugin", version: "1.0.0"})
	defer cleanup()

	cfg := vetangleConfig(t)
	cfg.Plugins = []config.PluginEntry{{Name: "does-not-exist"}}

	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}

	err = loadPlugins(cfg, g)
	if err == nil {
		t.Fatal("expected error for unknown plugin, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("expected error to name the unknown plugin, got: %v", err)
	}
	if !strings.Contains(err.Error(), "known-plugin") {
		t.Errorf("expected error to list available plugins, got: %v", err)
	}
	if len(loadedPlugins) != 0 {
		t.Error("expected no plugins to be loaded when an unknown plugin is requested")
	}
}

func TestLoadPlugins_DependsOn_LoadOrderRespected(t *testing.T) {
	var order []string
	base := &orderTrackingPlugin{name: "base", version: "1.0.0", order: &order}
	dependent := &orderTrackingPlugin{name: "dependent", version: "1.0.0", dependsOn: []string{"base"}, order: &order}

	cleanup := registerFixturePlugins(t, base, dependent)
	defer cleanup()

	cfg := vetangleConfig(t)
	cfg.Plugins = []config.PluginEntry{{Name: "dependent"}, {Name: "base"}}

	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}

	if err := loadPlugins(cfg, g); err != nil {
		t.Fatalf("loadPlugins: %v", err)
	}

	if len(order) != 2 || order[0] != "base" || order[1] != "dependent" {
		t.Fatalf("expected load order [base dependent], got %v", order)
	}
}

// orderTrackingPlugin records its own name into *order when Register runs,
// used to assert DependsOn-respecting load order through Load.
type orderTrackingPlugin struct {
	name      string
	version   string
	dependsOn []string
	order     *[]string
}

func (p *orderTrackingPlugin) Name() string        { return p.name }
func (p *orderTrackingPlugin) Version() string     { return p.version }
func (p *orderTrackingPlugin) DependsOn() []string { return p.dependsOn }
func (p *orderTrackingPlugin) Register(k plugin.KernelAPI) error {
	*p.order = append(*p.order, p.name)
	return nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

var fixtureRegistry = map[string]plugin.Plugin{}

// registerFixturePlugins registers plugins in the global plugin registry for
// the duration of a test and unregisters them on cleanup.
func registerFixturePlugins(t *testing.T, plugins ...plugin.Plugin) func() {
	t.Helper()
	for _, p := range plugins {
		plugin.Unregister(p.Name())
		plugin.Register(p)
		fixtureRegistry[p.Name()] = p
	}
	return func() {
		for _, p := range plugins {
			plugin.Unregister(p.Name())
			delete(fixtureRegistry, p.Name())
		}
	}
}

// registeredFixture fetches a registered fixtureKernelPlugin by name for
// post-Load introspection.
func registeredFixture(t *testing.T, name string) *fixtureKernelPlugin {
	t.Helper()
	p, ok := fixtureRegistry[name]
	if !ok {
		t.Fatalf("fixture plugin %q not registered", name)
	}
	fp, ok := p.(*fixtureKernelPlugin)
	if !ok {
		t.Fatalf("fixture plugin %q is not a *fixtureKernelPlugin", name)
	}
	return fp
}

// removeRootCommand strips a plugin-registered command from Root after a
// test so later tests see a clean Root.
func removeRootCommand(t *testing.T, use string) {
	t.Helper()
	for _, c := range Root.Commands() {
		if c.Use == use {
			Root.RemoveCommand(c)
			return
		}
	}
	t.Fatalf("expected to find command %q on Root to remove", use)
}

// TestBuiltinTestPlugin_RegistersHookCommandAndGenerator: Phase 5's done-when
// — the built-in "test" plugin registers a hook, a command, and a generator,
// and all are live after loading a project that declares plugins: [test].
func TestBuiltinTestPlugin_RegistersHookCommandAndGenerator(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
			},
		},
		Plugins: []config.PluginEntry{{Name: "test"}},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	if err := loadPlugins(cfg, g); err != nil {
		t.Fatalf("load plugins: %v", err)
	}

	// Command registered on the root
	found := false
	for _, c := range Root.Commands() {
		if c.Use == "test-plugin" {
			found = true
		}
	}
	if !found {
		t.Error("expected test-plugin command on the root command")
	}

	// Generator registered
	if kernelAPI == nil {
		t.Fatal("expected kernelAPI captured by loadPlugins")
	}
	if _, ok := kernelAPI.Generator("test-plugin"); !ok {
		t.Error("expected test-plugin generator registered")
	}

	// Hook fires on kernel:node:after_healthy through the kernel bus
	var buf strings.Builder
	output.SetOutput(&buf, &buf)
	defer output.SetOutput(os.Stdout, os.Stderr)
	kernelBus.Emit(plugin.EventAfterNodeHealthy, plugin.EventPayload{
		NodeID: "api",
		Data:   map[string]any{"node_type": "service"},
	})
	if !strings.Contains(buf.String(), "test-plugin") || !strings.Contains(buf.String(), "api") {
		t.Errorf("expected test-plugin hook line mentioning the node, got %q", buf.String())
	}
}

// TestLoadGraph_AfterBootstrap_DoesNotDoubleRegisterHooks: in the real binary
// bootstrapPlugins loads plugins at Execute, then the dev command's loadGraph
// runs — if loadGraph loads plugins a second time, every hook is registered
// twice on the shared kernelBus and each lifecycle event logs twice (seen live
// in the Phase 5 witness). One process, one plugin load.
func TestLoadGraph_AfterBootstrap_DoesNotDoubleRegisterHooks(t *testing.T) {
	dir := t.TempDir()
	yml := `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
plugins:
  - name: test
`
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	_ = os.Chdir(dir)

	oldBus := kernelBus
	kernelBus = plugin.NewBus()
	bootstrapped = bootstrapResult{}
	defer func() {
		kernelBus = oldBus
		bootstrapped = bootstrapResult{}
	}()

	bootstrapPlugins()
	_, g := loadGraph()
	if g == nil {
		t.Fatal("expected loadGraph to return a graph")
	}

	var buf strings.Builder
	output.SetOutput(&buf, &buf)
	defer output.SetOutput(os.Stdout, os.Stderr)
	kernelBus.Emit(plugin.EventAfterNodeHealthy, plugin.EventPayload{NodeID: "api"})
	if n := strings.Count(buf.String(), "is healthy"); n != 1 {
		t.Fatalf("expected exactly 1 hook line per event, got %d: %q", n, buf.String())
	}
}

// TestPluginCommands_AvailableBeforeCobraDispatch: cobra resolves the command
// word before any RunE executes, so plugin commands must be registered during
// CLI bootstrap (when the cwd holds an acthur.yml with plugins), not inside
// per-command graph loading — otherwise `acthur test-plugin` is an unknown
// command in the real binary even though the plugin registered it.
func TestPluginCommands_AvailableBeforeCobraDispatch(t *testing.T) {
	dir := t.TempDir()
	yml := `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
plugins:
  - name: test
`
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	_ = os.Chdir(dir)

	bootstrapPlugins()

	for _, c := range Root.Commands() {
		if c.Use == "test-plugin" {
			return
		}
	}
	t.Fatal("expected test-plugin command registered at bootstrap, before dispatch")
}
