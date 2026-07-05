package plugin_test

import (
	"testing"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/plugin"
)

func newTestGraph() *graph.Graph {
	return graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService},
	})
}

func TestKernelAPIImpl_OnEvent_FiresThroughBus(t *testing.T) {
	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, newTestGraph(), nil, nil)

	fired := false
	k.OnEvent(plugin.EventAfterNodeHealthy, func(p plugin.EventPayload) {
		fired = true
	})

	bus.Emit(plugin.EventAfterNodeHealthy, plugin.EventPayload{NodeID: "api"})

	if !fired {
		t.Error("expected handler registered via KernelAPIImpl.OnEvent to fire on bus.Emit")
	}
}

func TestKernelAPIImpl_RegisterCommand_InvokesCallback(t *testing.T) {
	bus := plugin.NewBus()
	var got plugin.CLICommand
	called := false
	addCommand := func(cmd plugin.CLICommand) {
		called = true
		got = cmd
	}
	k := plugin.NewKernelAPI(bus, newTestGraph(), addCommand, nil)

	cmd := plugin.CLICommand{Use: "test-plugin", Short: "test command"}
	k.RegisterCommand(cmd)

	if !called {
		t.Fatal("expected addCommand callback to be invoked")
	}
	if got.Use != "test-plugin" {
		t.Errorf("expected Use=test-plugin, got %q", got.Use)
	}
}

func TestKernelAPIImpl_AddNode_LandsPreSeal(t *testing.T) {
	bus := plugin.NewBus()
	g := newTestGraph()
	k := plugin.NewKernelAPI(bus, g, nil, nil)

	k.AddNode(&graph.Node{ID: "mailpit", Type: config.NodeTypeInfra})

	if g.Node("mailpit") == nil {
		t.Fatal("expected node added via KernelAPIImpl.AddNode to land in the graph")
	}
}

func TestKernelAPIImpl_AddEdge_LandsPreSeal(t *testing.T) {
	bus := plugin.NewBus()
	g := newTestGraph()
	k := plugin.NewKernelAPI(bus, g, nil, nil)

	k.AddNode(&graph.Node{ID: "mailer", Type: config.NodeTypeInfra})
	k.AddEdge(&graph.Edge{From: "api", To: "mailer", Type: config.EdgeDependsOn})

	edges := g.EdgesFrom("api")
	if len(edges) != 1 || edges[0].To != "mailer" {
		t.Fatalf("expected edge api->mailer to land in the graph, got %+v", edges)
	}
}

func TestKernelAPIImpl_AddNode_PanicsPostSeal(t *testing.T) {
	bus := plugin.NewBus()
	g := newTestGraph()
	k := plugin.NewKernelAPI(bus, g, nil, nil)
	g.Freeze()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected AddNode to panic after the graph is sealed")
		}
	}()
	k.AddNode(&graph.Node{ID: "toolate", Type: config.NodeTypeInfra})
}

func TestKernelAPIImpl_Generator_RegisterAndRetrieve(t *testing.T) {
	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, newTestGraph(), nil, nil)

	gen := &fakeGenerator{}
	k.RegisterGenerator("mailer", gen)

	got, ok := k.Generator("mailer")
	if !ok || got != gen {
		t.Fatal("expected registered generator to be retrievable by target name")
	}
	if _, ok := k.Generator("nonexistent"); ok {
		t.Error("expected unregistered target to not be found")
	}
}

func TestKernelAPIImpl_Schema_RegisterAndRetrieve(t *testing.T) {
	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, newTestGraph(), nil, nil)

	schema := plugin.SchemaDefinition{Name: "Widget", Fields: map[string]plugin.FieldDef{
		"name": {Type: "string", Required: true},
	}}
	k.RegisterSchema("Widget", schema)

	got, ok := k.Schema("Widget")
	if !ok || got.Name != "Widget" {
		t.Fatal("expected registered schema to be retrievable by name")
	}
}

func TestKernelAPIImpl_Middleware_RegisterAndRetrieve(t *testing.T) {
	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, newTestGraph(), nil, nil)

	mw := plugin.Middleware{Name: "logger", Priority: 10}
	k.RegisterMiddleware(mw)

	mws := k.Middlewares()
	if len(mws) != 1 || mws[0].Name != "logger" {
		t.Fatalf("expected registered middleware to be retrievable, got %+v", mws)
	}
}

func TestKernelAPIImpl_Graph_ReadOnlyView(t *testing.T) {
	bus := plugin.NewBus()
	g := newTestGraph()
	k := plugin.NewKernelAPI(bus, g, nil, nil)

	reader := k.Graph()
	if reader.Node("api") == nil {
		t.Fatal("expected Graph() to expose the same nodes as the underlying graph")
	}
}

func TestKernelAPIImpl_Log_UsesProvidedLogFunc(t *testing.T) {
	bus := plugin.NewBus()
	var gotLevel plugin.LogLevel
	var gotMsg string
	logFunc := func(level plugin.LogLevel, format string, args ...any) {
		gotLevel = level
		gotMsg = format
	}
	k := plugin.NewKernelAPI(bus, newTestGraph(), nil, logFunc)

	k.Log(plugin.LogInfo, "hello %s", "world")

	if gotLevel != plugin.LogInfo {
		t.Errorf("expected level=info, got %q", gotLevel)
	}
	if gotMsg != "hello %s" {
		t.Errorf("expected format string forwarded, got %q", gotMsg)
	}
}

// fakeGenerator is a minimal Generator for registry tests.
type fakeGenerator struct{}

func (g *fakeGenerator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	return nil, nil
}

func (g *fakeGenerator) SupportedAdapters() []string { return []string{"gofiber"} }
