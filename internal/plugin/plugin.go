// Package plugin defines the Plugin interface, the KernelAPI that plugins
// are given access to, the kernel event bus, and the plugin loader.
//
// A plugin's entire surface area is kernel registration.
// It gets a KernelAPI handle and installs hooks, commands, generators, and
// schemas through that handle. It NEVER reaches into kernel internals directly.
package plugin

import (
	"fmt"
	"sync"
	"time"

	"github.com/acthur/acthur/internal/graph"
)

// ---------------------------------------------------------------------------
// Plugin interface
// ---------------------------------------------------------------------------

// Plugin is the interface every Acthur plugin must implement.
// Three methods to implement. One method called by the kernel (Register).
type Plugin interface {
	// Name returns the plugin's unique identifier (e.g. "auth", "migrations").
	Name() string

	// Version returns the plugin's semantic version string.
	Version() string

	// DependsOn returns names of other plugins that must be loaded first.
	// The loader resolves these into a topological load order.
	DependsOn() []string

	// Register is called once during kernel boot.
	// The plugin receives a KernelAPI handle and must install everything
	// through it. After Register returns, the plugin is active.
	Register(k KernelAPI) error
}

// ---------------------------------------------------------------------------
// KernelAPI — restricted interface plugins use to modify the kernel
// ---------------------------------------------------------------------------

// KernelAPI is the restricted interface provided to plugins.
// This is the ONLY way plugins can interact with the kernel.
// Plugins cannot access any kernel internals not exposed here.
type KernelAPI interface {
	// Hook into kernel lifecycle events
	OnEvent(event Event, handler func(EventPayload))

	// Register a new CLI command (appears under `acthur` root)
	RegisterCommand(cmd CLICommand)

	// Register a code generator for a specific target name
	// The adapter must declare support for the target via GeneratorTargets()
	RegisterGenerator(target string, gen Generator)

	// Register a type schema (added to contract type registry)
	RegisterSchema(name string, schema SchemaDefinition)

	// Register HTTP middleware to inject at the proxy layer
	RegisterMiddleware(m Middleware)

	// Mutate the live graph (add nodes or edges)
	// Used by plugins that need to inject infrastructure nodes (e.g. a mailer
	// plugin that adds a mailpit infra node in dev)
	AddNode(node *graph.Node)
	AddEdge(edge *graph.Edge)

	// Read-only access to the graph
	Graph() GraphReader

	// Log using the kernel's output system
	Log(level LogLevel, format string, args ...any)
}

// GraphReader provides read-only access to the graph for plugins.
type GraphReader interface {
	Node(id string) *graph.Node
	Nodes() []*graph.Node
	Edges() []*graph.Edge
}

// ---------------------------------------------------------------------------
// Supporting types
// ---------------------------------------------------------------------------

// CLICommand represents a new command that a plugin adds to the Acthur CLI.
type CLICommand struct {
	Use   string
	Short string
	Long  string
	Run   func(args []string) error
	Flags []CLIFlag
}

// CLIFlag is a flag on a plugin-registered CLI command.
type CLIFlag struct {
	Name    string
	Short   string
	Default string
	Usage   string
}

// Generator is a code generation unit provided by a plugin.
// When `acthur generate <target>` is run (or triggered internally),
// the kernel calls Generate() with the resolved adapter name.
type Generator interface {
	// Generate produces files for the given adapter.
	// Returns an error if the adapter is not supported.
	Generate(adapterName string, ctx GeneratorContext) ([]GeneratedFile, error)

	// SupportedAdapters returns which adapters this generator supports.
	SupportedAdapters() []string
}

// GeneratorContext is passed to Generator.Generate().
type GeneratorContext struct {
	ProjectName string
	NodeID      string
	RootDir     string
	Config      map[string]any
	Extra       map[string]any
}

// GeneratedFile is a single file produced by a generator.
type GeneratedFile struct {
	Path        string
	Content     []byte
	Mode        uint32
	Overwrite   bool // if false, skip if file already exists
	MergeMarker string // if set, merge at this marker rather than overwrite
}

// SchemaDefinition defines a type that plugins contribute to the contract registry.
type SchemaDefinition struct {
	Name   string
	Fields map[string]FieldDef
}

// FieldDef defines a single field in a schema.
type FieldDef struct {
	Type     string
	Required bool
	Default  any
}

// Middleware is an HTTP middleware injected at the proxy layer.
type Middleware struct {
	Name     string
	Priority int // lower number runs first; 0=first, 100=last
	Handler  func(req Request, next func(Request) Response) Response
}

// Request and Response are minimal representations for proxy middleware.
// (Full implementations use net/http internally.)
type Request struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    []byte
}

type Response struct {
	Status  int
	Headers map[string]string
	Body    []byte
}

// LogLevel mirrors the output package levels.
type LogLevel string

const (
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
	LogDebug LogLevel = "debug"
)

// ---------------------------------------------------------------------------
// Event Bus
// ---------------------------------------------------------------------------

// Event names published on the kernel event bus.
type Event string

const (
	EventBeforeGraphBuild    Event = "kernel:graph:before_build"
	EventAfterGraphBuild     Event = "kernel:graph:after_build"
	EventBeforeNodeStart     Event = "kernel:node:before_start"
	EventAfterNodeStart      Event = "kernel:node:after_start"
	EventAfterNodeHealthy    Event = "kernel:node:after_healthy"
	EventBeforeNodeStop      Event = "kernel:node:before_stop"
	EventAfterNodeStop       Event = "kernel:node:after_stop"
	EventOnNodeFailure       Event = "kernel:node:on_failure"
	EventBeforeProxyRequest  Event = "kernel:proxy:before_request"
	EventAfterProxyRequest   Event = "kernel:proxy:after_request"
	EventBeforeMigrateRun    Event = "kernel:db:before_migrate"
	EventAfterMigrateRun     Event = "kernel:db:after_migrate"
	EventBeforeSeedRun       Event = "kernel:db:before_seed"
	EventAfterSeedRun        Event = "kernel:db:after_seed"
	EventBeforeDeploy        Event = "kernel:deploy:before"
	EventAfterDeploy         Event = "kernel:deploy:after"
	EventDeployPreflight     Event = "kernel:deploy:preflight"
	EventContractRegistered  Event = "kernel:contract:registered"
	EventContractViolated    Event = "kernel:contract:violated"
	EventPluginLoaded        Event = "kernel:plugin:loaded"
	EventPluginError         Event = "kernel:plugin:error"
)

// EventPayload carries event-specific data to handlers.
type EventPayload struct {
	Event     Event
	NodeID    string
	Timestamp time.Time
	Data      map[string]any
}

// Bus is the kernel event bus. Plugins subscribe to events via OnEvent.
type Bus struct {
	mu       sync.RWMutex
	handlers map[Event][]func(EventPayload)
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[Event][]func(EventPayload)),
	}
}

// On subscribes a handler to a specific event.
func (b *Bus) On(event Event, handler func(EventPayload)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[event] = append(b.handlers[event], handler)
}

// Emit publishes an event to all registered handlers.
// Handlers run synchronously in registration order.
// Panics in handlers are recovered and logged.
func (b *Bus) Emit(event Event, payload EventPayload) {
	b.mu.RLock()
	handlers := b.handlers[event]
	b.mu.RUnlock()

	payload.Event = event
	if payload.Timestamp.IsZero() {
		payload.Timestamp = time.Now()
	}

	for _, h := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[kernel] event handler panic for %s: %v\n", event, r)
				}
			}()
			h(payload)
		}()
	}
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

var (
	regMu    sync.RWMutex
	registry = map[string]Plugin{}
)

// Register adds a plugin to the global registry.
// Called from each plugin package's init() function.
func Register(p Plugin) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, exists := registry[p.Name()]; exists {
		panic(fmt.Sprintf("plugin %q is already registered", p.Name()))
	}
	registry[p.Name()] = p
}

// Resolve returns the plugin registered under the given name.
func Resolve(name string) (Plugin, error) {
	regMu.RLock()
	defer regMu.RUnlock()
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf(
			"plugin %q is not installed\n"+
				"  Run: acthur add %s",
			name, name,
		)
	}
	return p, nil
}

// All returns all registered plugins.
func All() []Plugin {
	regMu.RLock()
	defer regMu.RUnlock()
	result := make([]Plugin, 0, len(registry))
	for _, p := range registry {
		result = append(result, p)
	}
	return result
}

// ---------------------------------------------------------------------------
// Loader
// ---------------------------------------------------------------------------

// LoadedPlugin wraps a Plugin with its runtime state.
type LoadedPlugin struct {
	Plugin  Plugin
	LoadedAt time.Time
}

// Load resolves and loads plugins in topological dependency order.
// Returns the ordered list of loaded plugins, or an error if any
// dependency is missing or a cycle is detected.
func Load(names []string, bus *Bus, k KernelAPI) ([]*LoadedPlugin, error) {
	ordered, err := resolveOrder(names)
	if err != nil {
		return nil, err
	}

	var loaded []*LoadedPlugin
	for _, name := range ordered {
		p, err := Resolve(name)
		if err != nil {
			return nil, fmt.Errorf("loading plugin %q: %w", name, err)
		}

		if err := p.Register(k); err != nil {
			bus.Emit(EventPluginError, EventPayload{
				Data: map[string]any{"plugin": name, "error": err.Error()},
			})
			return nil, fmt.Errorf("plugin %q registration failed: %w", name, err)
		}

		lp := &LoadedPlugin{Plugin: p, LoadedAt: time.Now()}
		loaded = append(loaded, lp)

		bus.Emit(EventPluginLoaded, EventPayload{
			Data: map[string]any{"plugin": name, "version": p.Version()},
		})
	}
	return loaded, nil
}

// resolveOrder returns plugins in topological order based on DependsOn().
func resolveOrder(names []string) ([]string, error) {
	// Build dependency graph
	deps := make(map[string][]string)
	for _, name := range names {
		p, err := Resolve(name)
		if err != nil {
			return nil, err
		}
		deps[name] = p.DependsOn()
		// Verify all declared dependencies are in the names list
		for _, dep := range p.DependsOn() {
			found := false
			for _, n := range names {
				if n == dep {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf(
					"plugin %q requires plugin %q but it is not installed\n"+
						"  Add %q to your plugins list in acthur.yml",
					name, dep, dep,
				)
			}
		}
	}

	// Topological sort (Kahn's algorithm)
	inDegree := make(map[string]int)
	for _, name := range names {
		if _, ok := inDegree[name]; !ok {
			inDegree[name] = 0
		}
		for _, dep := range deps[name] {
			inDegree[name]++ // name depends on dep, so name's in-degree increases
		}
	}

	var queue []string
	for _, name := range names {
		if inDegree[name] == 0 {
			queue = append(queue, name)
		}
	}
	sortStrings(queue)

	var result []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		result = append(result, n)
		// Find all plugins that depend on n and reduce their in-degree
		for _, other := range names {
			for _, dep := range deps[other] {
				if dep == n {
					inDegree[other]--
					if inDegree[other] == 0 {
						queue = append(queue, other)
						sortStrings(queue)
					}
				}
			}
		}
	}

	if len(result) != len(names) {
		return nil, fmt.Errorf(
			"circular dependency detected in plugins\n" +
				"  Review DependsOn() declarations in your installed plugins",
		)
	}

	return result, nil
}

func sortStrings(ss []string) {
	for i := 0; i < len(ss); i++ {
		for j := i + 1; j < len(ss); j++ {
			if ss[j] < ss[i] {
				ss[i], ss[j] = ss[j], ss[i]
			}
		}
	}
}
