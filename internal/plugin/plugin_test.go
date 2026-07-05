package plugin_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/output"
	"github.com/acthur/acthur/internal/plugin"
)

// ---------------------------------------------------------------------------
// Fake plugins for testing
// ---------------------------------------------------------------------------

type fakePlugin struct {
	name      string
	version   string
	dependsOn []string
	registered bool
}

func (p *fakePlugin) Name() string       { return p.name }
func (p *fakePlugin) Version() string    { return p.version }
func (p *fakePlugin) DependsOn() []string { return p.dependsOn }
func (p *fakePlugin) Register(k plugin.KernelAPI) error {
	p.registered = true
	return nil
}

// ---------------------------------------------------------------------------
// Event bus tests
// ---------------------------------------------------------------------------

func TestBus_Emit(t *testing.T) {
	bus := plugin.NewBus()
	received := make([]plugin.EventPayload, 0)

	bus.On(plugin.EventAfterNodeHealthy, func(p plugin.EventPayload) {
		received = append(received, p)
	})

	bus.Emit(plugin.EventAfterNodeHealthy, plugin.EventPayload{
		NodeID: "api",
	})

	if len(received) != 1 {
		t.Errorf("expected 1 event, got %d", len(received))
	}
	if received[0].NodeID != "api" {
		t.Errorf("expected nodeID=api, got %q", received[0].NodeID)
	}
	if received[0].Event != plugin.EventAfterNodeHealthy {
		t.Errorf("expected event field to be set, got %q", received[0].Event)
	}
}

func TestBus_MultipleHandlers(t *testing.T) {
	bus := plugin.NewBus()
	count := 0

	bus.On(plugin.EventAfterGraphBuild, func(p plugin.EventPayload) { count++ })
	bus.On(plugin.EventAfterGraphBuild, func(p plugin.EventPayload) { count++ })
	bus.On(plugin.EventAfterGraphBuild, func(p plugin.EventPayload) { count++ })

	bus.Emit(plugin.EventAfterGraphBuild, plugin.EventPayload{})

	if count != 3 {
		t.Errorf("expected 3 handlers called, got %d", count)
	}
}

func TestBus_DifferentEvents_DoNotCross(t *testing.T) {
	bus := plugin.NewBus()
	gotA, gotB := false, false

	bus.On(plugin.EventBeforeNodeStart, func(p plugin.EventPayload) { gotA = true })
	bus.On(plugin.EventAfterNodeStop, func(p plugin.EventPayload) { gotB = true })

	bus.Emit(plugin.EventBeforeNodeStart, plugin.EventPayload{})

	if !gotA {
		t.Error("expected handler A to be called")
	}
	if gotB {
		t.Error("expected handler B NOT to be called")
	}
}

func TestBus_PanicInHandlerDoesNotCrash(t *testing.T) {
	bus := plugin.NewBus()
	after := false

	bus.On(plugin.EventAfterDeploy, func(p plugin.EventPayload) {
		panic("handler panic")
	})
	bus.On(plugin.EventAfterDeploy, func(p plugin.EventPayload) {
		after = true
	})

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Emit should recover from handler panics, got: %v", r)
		}
	}()

	bus.Emit(plugin.EventAfterDeploy, plugin.EventPayload{})

	if !after {
		t.Error("second handler should still run after first panics")
	}
}

// TestBus_PanicInHandlerLogsViaOutputWarn: the recovered panic must surface
// through internal/output.Warn (not a raw fmt.Printf) so it carries the same
// "[plugin] ⚠" formatting as every other kernel warning.
func TestBus_PanicInHandlerLogsViaOutputWarn(t *testing.T) {
	var buf bytes.Buffer
	output.SetOutput(&buf, &buf)
	defer output.SetOutput(os.Stdout, os.Stderr)

	bus := plugin.NewBus()
	bus.On(plugin.EventAfterDeploy, func(p plugin.EventPayload) {
		panic("boom")
	})

	bus.Emit(plugin.EventAfterDeploy, plugin.EventPayload{})

	got := buf.String()
	if !strings.Contains(got, "plugin") {
		t.Fatalf("expected panic warning scoped to plugin, got %q", got)
	}
	if !strings.Contains(got, "boom") {
		t.Fatalf("expected panic message in warning output, got %q", got)
	}
}

func TestBus_TimestampAutoSet(t *testing.T) {
	bus := plugin.NewBus()
	var received plugin.EventPayload

	bus.On(plugin.EventPluginLoaded, func(p plugin.EventPayload) {
		received = p
	})
	bus.Emit(plugin.EventPluginLoaded, plugin.EventPayload{})

	if received.Timestamp.IsZero() {
		t.Error("expected timestamp to be auto-set")
	}
}

// ---------------------------------------------------------------------------
// Dependency resolution tests
// ---------------------------------------------------------------------------

// resolveOrderPublic is a test-only helper since resolveOrder is unexported.
// We test it indirectly through Load behaviour.

func TestResolveOrder_BasicChain(t *testing.T) {
	// Register test plugins
	cleanup := registerFakePlugins(t,
		&fakePlugin{name: "migrations", version: "1.0.0"},
		&fakePlugin{name: "auth", version: "1.0.0", dependsOn: []string{"migrations"}},
		&fakePlugin{name: "rbac", version: "1.0.0", dependsOn: []string{"auth"}},
	)
	defer cleanup()

	// Just test that Load doesn't error — correct order is verified by
	// the fact that rbac depends on auth which depends on migrations
	bus := plugin.NewBus()
	k := &nullKernelAPI{}
	_, err := plugin.Load([]string{"migrations", "auth", "rbac"}, bus, k)
	if err != nil {
		t.Errorf("expected successful load, got: %v", err)
	}
}

func TestResolveOrder_MissingDependency(t *testing.T) {
	cleanup := registerFakePlugins(t,
		&fakePlugin{name: "rbac", version: "1.0.0",
			dependsOn: []string{"auth"}}, // auth not installed
	)
	defer cleanup()

	bus := plugin.NewBus()
	k := &nullKernelAPI{}
	_, err := plugin.Load([]string{"rbac"}, bus, k)
	if err == nil {
		t.Error("expected error for missing dependency, got nil")
	}
}

func TestResolveOrder_AllRegistered(t *testing.T) {
	plugins := []*fakePlugin{
		{name: "migrations", version: "1.0.0"},
		{name: "auth", version: "1.0.0", dependsOn: []string{"migrations"}},
		{name: "rbac", version: "1.0.0", dependsOn: []string{"auth"}},
		{name: "multitenancy", version: "1.0.0", dependsOn: []string{"auth", "migrations"}},
	}
	cleanup := registerFakePlugins(t, plugins...)
	defer cleanup()

	bus := plugin.NewBus()
	k := &nullKernelAPI{}
	loaded, err := plugin.Load([]string{"migrations", "auth", "rbac", "multitenancy"}, bus, k)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(loaded) != 4 {
		t.Errorf("expected 4 loaded plugins, got %d", len(loaded))
	}
	// All plugins should be marked as registered
	for _, p := range plugins {
		if !p.registered {
			t.Errorf("plugin %q was not registered", p.name)
		}
	}
}

func TestLoad_EmitsPluginLoadedEvent(t *testing.T) {
	cleanup := registerFakePlugins(t,
		&fakePlugin{name: "testemit", version: "1.0.0"},
	)
	defer cleanup()

	bus := plugin.NewBus()
	loaded := false
	bus.On(plugin.EventPluginLoaded, func(p plugin.EventPayload) {
		if name, ok := p.Data["plugin"].(string); ok && name == "testemit" {
			loaded = true
		}
	})

	k := &nullKernelAPI{}
	_, err := plugin.Load([]string{"testemit"}, bus, k)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !loaded {
		t.Error("expected EventPluginLoaded to fire")
	}
}

// ---------------------------------------------------------------------------
// Registry tests
// ---------------------------------------------------------------------------

func TestRegistry_ResolveUnknown(t *testing.T) {
	_, err := plugin.Resolve("definitely-not-a-real-plugin-xyz")
	if err == nil {
		t.Error("expected error resolving unknown plugin, got nil")
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// registerFakePlugins registers plugins and returns a cleanup function
// that removes them from the registry (since registry is global in tests).
func registerFakePlugins(t *testing.T, plugins ...*fakePlugin) func() {
	t.Helper()
	for _, p := range plugins {
		// Unregister any pre-existing plugin with this name (from a previous test)
		// before re-registering with the new instance.
		plugin.Unregister(p.Name())
		plugin.Register(p)
	}
	return func() {
		for _, p := range plugins {
			plugin.Unregister(p.Name())
		}
	}
}

// nullKernelAPI is a no-op KernelAPI for testing plugin loading.
type nullKernelAPI struct{}

func (k *nullKernelAPI) OnEvent(_ plugin.Event, _ func(plugin.EventPayload))        {}
func (k *nullKernelAPI) RegisterCommand(_ plugin.CLICommand)                         {}
func (k *nullKernelAPI) RegisterGenerator(_ string, _ plugin.Generator)              {}
func (k *nullKernelAPI) RegisterSchema(_ string, _ plugin.SchemaDefinition)          {}
func (k *nullKernelAPI) RegisterMiddleware(_ plugin.Middleware)                       {}
func (k *nullKernelAPI) AddNode(_ *graph.Node)                                        {}
func (k *nullKernelAPI) AddEdge(_ *graph.Edge)                                        {}
func (k *nullKernelAPI) Graph() plugin.GraphReader                                    { return nil }
func (k *nullKernelAPI) Log(_ plugin.LogLevel, _ string, _ ...any)                   {}
