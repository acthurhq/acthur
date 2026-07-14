package plugin

import (
	"fmt"
	"sync"

	"github.com/acthurhq/acthur/internal/graph"
)

// KernelAPIImpl is the kernel's real, concrete KernelAPI implementation.
// It is constructed once at CLI assembly (ADR 0005 resolver-injection
// pattern) and handed to every plugin's Register method during Load.
//
// Graph mutation (AddNode/AddEdge) is only valid pre-seal — see ADR 0003.
// The underlying *graph.Graph enforces this itself: it panics if a
// structural mutation is attempted after Freeze(). KernelAPIImpl does not
// duplicate that check; it simply forwards to the graph.
type KernelAPIImpl struct {
	bus        *Bus
	graph      *graph.Graph
	addCommand func(CLICommand)
	logFunc    func(level LogLevel, format string, args ...any)

	mu          sync.RWMutex
	generators  map[string]Generator
	schemas     map[string]SchemaDefinition
	middlewares []Middleware
}

// NewKernelAPI constructs a KernelAPIImpl.
//
//   - bus: the kernel event bus plugins subscribe to via OnEvent.
//   - g: the live graph, mutable pre-seal only (ADR 0003).
//   - addCommand: called for every plugin.RegisterCommand — the CLI
//     assembly wires this to cobra Root.AddCommand via a small adapter.
//
// logFunc may be nil, in which case Log falls back to fmt.Printf so the
// implementation stays usable in tests without pulling in internal/output.
func NewKernelAPI(bus *Bus, g *graph.Graph, addCommand func(CLICommand), logFunc func(level LogLevel, format string, args ...any)) *KernelAPIImpl {
	return &KernelAPIImpl{
		bus:        bus,
		graph:      g,
		addCommand: addCommand,
		logFunc:    logFunc,
		generators: make(map[string]Generator),
		schemas:    make(map[string]SchemaDefinition),
	}
}

// OnEvent subscribes handler to event on the kernel bus.
func (k *KernelAPIImpl) OnEvent(event Event, handler func(EventPayload)) {
	k.bus.On(event, handler)
}

// RegisterCommand hands cmd to the CLI assembly's addCommand callback.
func (k *KernelAPIImpl) RegisterCommand(cmd CLICommand) {
	if k.addCommand != nil {
		k.addCommand(cmd)
	}
}

// RegisterGenerator stores gen under target in the kernel's generator
// registry. Consumed by the generator engine (Phase 7).
func (k *KernelAPIImpl) RegisterGenerator(target string, gen Generator) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.generators[target] = gen
}

// Generator returns the generator registered under target, if any.
func (k *KernelAPIImpl) Generator(target string) (Generator, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	gen, ok := k.generators[target]
	return gen, ok
}

// Generators returns every registered target name, sorted for stable output.
func (k *KernelAPIImpl) Generators() map[string]Generator {
	k.mu.RLock()
	defer k.mu.RUnlock()
	out := make(map[string]Generator, len(k.generators))
	for name, gen := range k.generators {
		out[name] = gen
	}
	return out
}

// RegisterSchema stores schema under name in the kernel's contract type
// registry. Consumed by the contract engine.
func (k *KernelAPIImpl) RegisterSchema(name string, schema SchemaDefinition) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.schemas[name] = schema
}

// Schema returns the schema registered under name, if any.
func (k *KernelAPIImpl) Schema(name string) (SchemaDefinition, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	s, ok := k.schemas[name]
	return s, ok
}

// Schemas returns every registered schema, keyed by name.
func (k *KernelAPIImpl) Schemas() map[string]SchemaDefinition {
	k.mu.RLock()
	defer k.mu.RUnlock()
	out := make(map[string]SchemaDefinition, len(k.schemas))
	for name, s := range k.schemas {
		out[name] = s
	}
	return out
}

// RegisterMiddleware appends m to the kernel's proxy middleware chain.
// Consumed by the dev proxy once it grows a middleware chain.
func (k *KernelAPIImpl) RegisterMiddleware(m Middleware) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.middlewares = append(k.middlewares, m)
}

// Middlewares returns every registered middleware, in registration order.
func (k *KernelAPIImpl) Middlewares() []Middleware {
	k.mu.RLock()
	defer k.mu.RUnlock()
	out := make([]Middleware, len(k.middlewares))
	copy(out, k.middlewares)
	return out
}

// AddNode forwards to the live graph. Valid pre-seal only — the graph
// itself panics if called after Freeze() (ADR 0003).
func (k *KernelAPIImpl) AddNode(node *graph.Node) {
	k.graph.AddNode(node)
}

// AddEdge forwards to the live graph. Valid pre-seal only — the graph
// itself panics if called after Freeze() (ADR 0003).
func (k *KernelAPIImpl) AddEdge(edge *graph.Edge) {
	k.graph.AddEdge(edge)
}

// Graph returns a read-only view of the live graph. *graph.Graph already
// satisfies GraphReader (Node/Nodes/Edges), so no adapter type is needed.
func (k *KernelAPIImpl) Graph() GraphReader {
	return k.graph
}

// Log routes through the kernel's output system when logFunc is provided,
// otherwise falls back to fmt.Printf (keeps this type usable standalone).
func (k *KernelAPIImpl) Log(level LogLevel, format string, args ...any) {
	if k.logFunc != nil {
		k.logFunc(level, format, args...)
		return
	}
	fmt.Printf("[%s] %s\n", level, fmt.Sprintf(format, args...))
}

var _ KernelAPI = (*KernelAPIImpl)(nil)
