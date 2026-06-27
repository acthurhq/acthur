// Package engine contains the dev and deploy orchestration engines.
// The DevEngine is the runtime core of `acthur dev` — it starts services
// in graph-topological order, manages their health, runs the proxy,
// and coordinates graceful shutdown.
package engine

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/acthur/acthur/internal/adapter"
	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/container"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/health"
	"github.com/acthur/acthur/internal/output"
	"github.com/acthur/acthur/internal/process"
	"github.com/acthur/acthur/internal/proxy"
)

// ---------------------------------------------------------------------------
// DevEngine
// ---------------------------------------------------------------------------

// AdapterResolver is the dev runtime's adapter knowledge boundary.
// It validates graph adapter keys through graph.Resolver and returns concrete
// adapters only at runtime capability call sites.
type AdapterResolver interface {
	graph.Resolver
	Adapter(key string) (adapter.Adapter, bool)
}

type processManager interface {
	Spawn(nodeID, bin string, args []string, env map[string]string, dir string) (*process.Process, error)
	StopAll(nodeIDs []string)
}

type healthChecker interface {
	WaitFor(ctx context.Context, node *graph.Node, timeout time.Duration) error
	WaitForStrategy(ctx context.Context, node *graph.Node, strategy health.Strategy, timeout time.Duration) error
}

// DevEngine is the runtime brain of `acthur dev`.
// It owns the full lifecycle: startup → supervision → shutdown.
type DevEngine struct {
	cfg      *config.Config
	graph    *graph.Graph
	resolver AdapterResolver
	pm       processManager
	checker  healthChecker
	proxy    *proxy.Proxy
	secrets  secretStore
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewDevEngine creates a DevEngine. Call Start() to begin.
func NewDevEngine(cfg *config.Config, g *graph.Graph, resolver AdapterResolver) *DevEngine {
	ctx, cancel := context.WithCancel(context.Background())
	return &DevEngine{
		cfg:      cfg,
		graph:    g,
		resolver: resolver,
		pm:       process.NewManager(),
		checker:  health.New(),
		secrets:  fileSecretStore{rootDir: cfg.RootDir},
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start runs the full startup sequence and blocks until shutdown.
//
// Startup sequence:
//  1. Validate graph
//  2. Resolve startup order (topological sort)
//  3. Start infra nodes (docker containers)
//  4. Wait for infra health
//  5. Start service nodes (native processes)
//  6. Wait for service health
//  7. Start dev proxy
//  8. Print ready message
//  9. Wait for SIGINT/SIGTERM
//
// 10. Graceful shutdown in reverse order
func (e *DevEngine) Start() error {
	output.Banner()
	output.Info("", "loading graph for %s...", e.cfg.Project)

	// Validate
	errs := e.graph.Validate(e.resolver)
	if len(errs) != 0 {
		for _, err := range errs {
			output.Error("graph", "%s", err.Error())
		}
		return fmt.Errorf("graph validation failed with %d error(s)", len(errs))
	}
	output.Success("graph", "%d nodes, %d edges — valid",
		len(e.graph.Nodes()), len(e.graph.Edges()))

	// Startup order
	order := e.graph.StartupOrder()
	output.Info("", "startup order: %s", nodeListStr(order))

	// Start each node in order
	for _, node := range order {
		if err := e.startNode(node); err != nil {
			e.shutdown(order)
			return fmt.Errorf("failed to start %q: %w", node.ID, err)
		}
	}

	// Start proxy
	p, err := proxy.New(e.graph, e.cfg.Dev.Port)
	if err != nil {
		e.shutdown(order)
		return fmt.Errorf("proxy setup failed: %w", err)
	}
	e.proxy = p
	if err := p.Start(); err != nil {
		e.shutdown(order)
		return fmt.Errorf("proxy start failed: %w", err)
	}

	// Print ready message
	e.printReady()

	// Block until signal
	e.waitForShutdown()

	// Graceful shutdown
	output.Info("", "shutting down...")
	if e.proxy != nil {
		e.proxy.Stop()
	}
	e.shutdown(order)
	output.Success("", "all services stopped")
	return nil
}

// startNode starts a single graph node according to its type.
func (e *DevEngine) startNode(node *graph.Node) error {
	switch node.Type {
	case config.NodeTypeInfra:
		return e.startInfraNode(node)
	case config.NodeTypeService:
		return e.startServiceNode(node)
	default:
		return nil // plugins and contracts don't start processes
	}
}

// startInfraNode starts an infra node as a Docker container.
func (e *DevEngine) startInfraNode(node *graph.Node) error {
	e.graph.SetState(node.ID, graph.StateStarting)
	output.Info(node.ID, "starting %s (%s)...", node.ID, node.Adapter)

	a, ok := e.resolver.Adapter(node.Adapter)
	if !ok {
		e.graph.SetState(node.ID, graph.StateFailed)
		return fmt.Errorf("adapter %q not found", node.Adapter)
	}
	c, ok := a.(adapter.Containerized)
	if !ok {
		e.graph.SetState(node.ID, graph.StateFailed)
		return fmt.Errorf("adapter %q does not support the Container capability", node.Adapter)
	}
	spec := c.Container(adapter.ContainerContext{
		NodeID:  node.ID,
		Version: node.Config.Version,
	})
	if len(spec.Ports) == 0 {
		// File-based infra (sqlite) — nothing to start
		e.graph.SetState(node.ID, graph.StateHealthy)
		return nil
	}
	args := container.ToRunArgs(spec, node.ID)

	_, err := e.pm.Spawn(node.ID, "docker", args, nil, "")
	if err != nil {
		e.graph.SetState(node.ID, graph.StateFailed)
		return err
	}

	strategy := health.InfraStrategy("acthur-"+node.ID, spec.Healthcheck.Test, nil)
	if err := e.checker.WaitForStrategy(e.ctx, node, strategy, 60*time.Second); err != nil {
		e.graph.SetState(node.ID, graph.StateDegraded)
		return err
	}

	e.graph.SetState(node.ID, graph.StateHealthy)
	return nil
}

// startServiceNode starts a service node using its adapter's dev command.
func (e *DevEngine) startServiceNode(node *graph.Node) error {
	e.graph.SetState(node.ID, graph.StateStarting)
	output.Info(node.ID, "starting %s (%s)...", node.ID, node.Adapter)

	a, ok := e.resolver.Adapter(node.Adapter)
	if !ok {
		e.graph.SetState(node.ID, graph.StateFailed)
		return fmt.Errorf("adapter %q not found", node.Adapter)
	}

	// Build env for this node
	env, err := e.buildEnv(node)
	if err != nil {
		e.graph.SetState(node.ID, graph.StateFailed)
		return err
	}
	r, ok := a.(adapter.Runnable)
	if !ok {
		e.graph.SetState(node.ID, graph.StateFailed)
		return fmt.Errorf("adapter %q does not support the Runnable capability", node.Adapter)
	}
	cmd := r.DevCommand(env)

	nodeDir := nodeDirectory(e.cfg.RootDir, node)
	_, err = e.pm.Spawn(node.ID, cmd.Bin, cmd.Args, cmd.Env, nodeDir)
	if err != nil {
		e.graph.SetState(node.ID, graph.StateFailed)
		return err
	}

	// Wait for HTTP health
	// Give the service time to start before health-checking
	time.Sleep(500 * time.Millisecond)
	if err := e.checker.WaitFor(e.ctx, node, 120*time.Second); err != nil {
		e.graph.SetState(node.ID, graph.StateDegraded)
		return err
	}

	e.graph.SetState(node.ID, graph.StateHealthy)
	return nil
}

// shutdown stops all processes in reverse order.
func (e *DevEngine) shutdown(order []*graph.Node) {
	e.cancel()
	ids := make([]string, len(order))
	for i, n := range order {
		ids[i] = n.ID
	}
	e.pm.StopAll(ids)
}

// buildEnv constructs the environment for a node.
// Injects service discovery URLs from data_flow edges.
func (e *DevEngine) buildEnv(node *graph.Node) (map[string]string, error) {
	return resolveNodeEnv(e.graph, e.resolver, node, e.secrets)
}

func resolveNodeEnv(g *graph.Graph, resolver AdapterResolver, node *graph.Node, secrets secretStore) (map[string]string, error) {
	env := make(map[string]string)

	// Port
	if node.Port != 0 {
		env["PORT"] = fmt.Sprintf("%d", node.Port)
		env["APP_PORT"] = fmt.Sprintf("%d", node.Port)
	}

	// Apply this node's own adapter EnvVars: defaults, and synthesized values
	// for Generate-marked secrets. Engine-set keys (PORT) and edge-injected
	// keys (DATABASE_URL) take precedence, so we never overwrite what's set.
	if a, ok := resolver.Adapter(node.Adapter); ok {
		for _, ev := range a.EnvVars() {
			if _, exists := env[ev.Key]; exists {
				continue
			}
			switch {
			case ev.Default != "":
				env[ev.Key] = ev.Default
			case ev.Generate:
				value, err := secrets.Secret(node.ID, ev.Key)
				if err != nil {
					return nil, fmt.Errorf("synthesize %s for %q: %w", ev.Key, node.ID, err)
				}
				env[ev.Key] = value
			}
		}
	}

	// Service discovery: inject URLs for nodes this one calls
	for _, edge := range g.EdgesFrom(node.ID) {
		if edge.Type == config.EdgeDataFlow || edge.Type == config.EdgeDependsOn {
			target := g.Node(edge.To)
			if target == nil {
				continue
			}
			if target.Port != 0 {
				// e.g. api → USER_SERVICE_URL=http://localhost:8081
				envKey := envKeyFor(edge.To) + "_URL"
				env[envKey] = fmt.Sprintf("http://localhost:%d", target.Port)
			}
			if a, ok := resolver.Adapter(target.Adapter); ok {
				if c, ok := a.(adapter.Connectable); ok {
					for key, value := range c.ConnectionEnv(adapter.ContainerContext{
						NodeID:  target.ID,
						Version: target.Config.Version,
					}) {
						env[key] = value
					}
				}
			}
		}
	}

	return env, nil
}

// printReady outputs the final "ready" message with all service URLs.
func (e *DevEngine) printReady() {
	urls := map[string]string{
		"proxy": fmt.Sprintf("http://localhost:%d", e.cfg.Dev.Port),
	}
	for _, node := range e.graph.NodesByType(config.NodeTypeService) {
		if node.DevURL != "" {
			urls[node.ID] = "http://" + node.DevURL
		} else if node.Port != 0 {
			urls[node.ID] = fmt.Sprintf("http://localhost:%d", node.Port)
		}
	}
	output.Ready(e.cfg.Project, urls)
}

// waitForShutdown blocks until SIGINT or SIGTERM is received.
func (e *DevEngine) waitForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	fmt.Println()
}

func nodeDirectory(rootDir string, node *graph.Node) string {
	// If the project is a monorepo, services live in subdirectories
	// Otherwise, a single-service project lives at the root
	candidates := []string{
		rootDir + "/services/" + node.ID,
		rootDir + "/" + node.ID,
		rootDir,
	}
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	return rootDir
}

func nodeListStr(nodes []*graph.Node) string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return joinStr(ids, " → ")
}

func joinStr(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// envKeyFor converts a node ID to an uppercase env key.
// e.g. "user-service" → "USER_SERVICE"
func envKeyFor(nodeID string) string {
	upper := ""
	for _, ch := range nodeID {
		if ch == '-' || ch == '.' {
			upper += "_"
		} else if ch >= 'a' && ch <= 'z' {
			upper += string(ch - 32)
		} else {
			upper += string(ch)
		}
	}
	return upper
}
