package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/plugin"
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
