// Package engine contains the dev and deploy orchestration engines.
// The DevEngine is the runtime core of `acthur dev` — it starts services
// in graph-topological order, manages their health, runs the proxy,
// and coordinates graceful shutdown.
package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/acthur/acthur/internal/adapter"
	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/container"
	"github.com/acthur/acthur/internal/contract"
	"github.com/acthur/acthur/internal/dns"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/health"
	"github.com/acthur/acthur/internal/output"
	"github.com/acthur/acthur/internal/plugin"
	"github.com/acthur/acthur/internal/process"
	"github.com/acthur/acthur/internal/proxy"
	"github.com/acthur/acthur/internal/watcher"
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
	Restart(nodeID string) error
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

	// bus is the optional kernel event bus. Nil means no emission — plugins
	// are entirely absent from this phase's callers (cmd/acthur, tests that
	// don't need it) and lifecycle emission must cost nothing when unused.
	bus *plugin.Bus

	// registry is the optional contract registry consulted by the proxy to
	// enforce contracts on data_flow flow routes. Nil means no enforcement.
	registry *contract.Registry
	// strict switches proxy contract enforcement from dev mode (log +
	// forward) to strict mode (422 + block). Plumbed from `acthur dev --strict`.
	strict bool

	// runDocker executes a docker CLI command to completion. Seam for tests;
	// used at shutdown to stop containers the engine started (killing the
	// docker-run client alone leaves the container running).
	runDocker func(args ...string) error
	// containers records node IDs whose containers the engine started,
	// so shutdown stops exactly what it created.
	containers []string

	// writeHosts, when true, makes checkDNS attempt to append missing dev
	// hostnames directly to hostsPath instead of only printing instructions.
	// Wired from `acthur dev --write-hosts`.
	writeHosts bool
	// hostsPath is the hosts file checkDNS writes to when writeHosts is set.
	// Defaults to /etc/hosts; overridable in tests.
	hostsPath string
	// dnsLookup resolves a hostname for checkDNS. Defaults to dns.DefaultLookup;
	// overridable in tests so DNS behavior never depends on the test machine's
	// real resolver or /etc/hosts contents.
	dnsLookup dns.LookupFunc

	// watcher polls each service node's directory for file changes and fires
	// handleFileChange. Nil until Start() constructs it (no rootDir to watch
	// in unit tests that call startNode/startInfraNode directly).
	watcher *watcher.Watcher
}

// DevEngineOption configures optional DevEngine behavior at construction time.
type DevEngineOption func(*DevEngine)

// WithStrict enables strict contract enforcement: the proxy blocks (422)
// data_flow requests that violate their edge's contract instead of logging
// and forwarding them. Wired from the `acthur dev --strict` flag.
func WithStrict(strict bool) DevEngineOption {
	return func(e *DevEngine) { e.strict = strict }
}

// WithContractRegistry configures the contract registry the proxy consults
// to enforce contracts on data_flow flow routes. Without this option the
// proxy still serves flow routes, but never checks a contract.
func WithContractRegistry(r *contract.Registry) DevEngineOption {
	return func(e *DevEngine) { e.registry = r }
}

// WithBus attaches a kernel event bus. The engine emits node lifecycle
// events (kernel:node:*) synchronously through it as nodes start, become
// healthy, fail, and stop. Without this option (nil bus) emission is a
// no-op — zero overhead, and every existing caller that doesn't know about
// plugins keeps working unchanged.
func WithBus(bus *plugin.Bus) DevEngineOption {
	return func(e *DevEngine) { e.bus = bus }
}

// WithWriteHosts makes the DNS preflight attempt to append missing dev
// hostnames straight to /etc/hosts instead of only printing copy-pastable
// instructions. Wired from `acthur dev --write-hosts`. The engine still
// never escalates privileges — if the process lacks permission to write
// /etc/hosts, it falls back to printing the same instructions.
func WithWriteHosts(writeHosts bool) DevEngineOption {
	return func(e *DevEngine) { e.writeHosts = writeHosts }
}

// NewDevEngine creates a DevEngine. Call Start() to begin.
func NewDevEngine(cfg *config.Config, g *graph.Graph, resolver AdapterResolver, opts ...DevEngineOption) *DevEngine {
	ctx, cancel := context.WithCancel(context.Background())
	pm := process.NewManager()

	// Durable per-node log files under .acthur/logs/<node>.log — the seam
	// `acthur service logs` reads from. Only wired when there's a real
	// project root to write under (cfg.RootDir == "" in most unit tests,
	// which must never touch disk).
	if cfg.RootDir != "" {
		if sink, err := process.FileLogSink(cfg.RootDir); err == nil {
			pm.SetLogSink(sink)
		} else {
			output.Warn("", "could not set up .acthur/logs: %v", err)
		}
	}

	e := &DevEngine{
		cfg:      cfg,
		graph:    g,
		resolver: resolver,
		pm:       pm,
		checker:  health.New(),
		secrets:  fileSecretStore{rootDir: cfg.RootDir},
		ctx:      ctx,
		cancel:   cancel,
		runDocker: func(args ...string) error {
			return exec.Command("docker", args...).Run()
		},
		hostsPath: "/etc/hosts",
		dnsLookup: dns.DefaultLookup,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
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

	// DNS preflight — check whether the dev domain and each service's
	// subdomain resolve to 127.0.0.1. Never blocks startup: localhost:<port>
	// URLs still work even if the friendly hostnames don't resolve yet.
	e.checkDNS()

	// Start proxy
	p, err := proxy.New(e.graph, e.cfg.Dev.Port,
		proxy.WithContractRegistry(e.registry),
		proxy.WithStrict(e.strict),
	)
	if err != nil {
		e.shutdown(order)
		return fmt.Errorf("proxy setup failed: %w", err)
	}
	e.proxy = p
	if err := p.Start(); err != nil {
		e.shutdown(order)
		return fmt.Errorf("proxy start failed: %w", err)
	}

	// Start the file watcher / hot reload cascade. It restarts a node's
	// process on a file change in its directory, unless the node's adapter
	// hot-reloads itself (e.g. air) — see handleFileChange.
	e.watcher = watcher.New(e.graph, e.cfg.RootDir, 0)
	e.watcher.OnChange(e.handleFileChange)
	e.watcher.Start()

	// Print ready message
	e.printReady()

	// Block until signal
	e.waitForShutdown()

	// Graceful shutdown
	output.Info("", "shutting down...")
	if e.watcher != nil {
		e.watcher.Stop()
	}
	if e.proxy != nil {
		e.proxy.Stop()
	}
	e.shutdown(order)
	output.Success("", "all services stopped")
	return nil
}

// startNode starts a single graph node according to its type.
func (e *DevEngine) startNode(node *graph.Node) error {
	// kernel:* nodes are materialized and run by the kernel itself (the proxy
	// is started by the engine after all graph nodes) — they never resolve
	// through the adapter registry, mirroring the graph validation exemption.
	if strings.HasPrefix(node.Adapter, "kernel:") {
		return nil
	}
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
	e.emit(plugin.EventBeforeNodeStart, node, nil)

	a, ok := e.resolver.Adapter(node.Adapter)
	if !ok {
		e.graph.SetState(node.ID, graph.StateFailed)
		err := fmt.Errorf("adapter %q not found", node.Adapter)
		e.emit(plugin.EventOnNodeFailure, node, map[string]any{"error": err.Error()})
		return err
	}
	c, ok := a.(adapter.Containerized)
	if !ok {
		e.graph.SetState(node.ID, graph.StateFailed)
		err := fmt.Errorf("adapter %q does not support the Container capability", node.Adapter)
		e.emit(plugin.EventOnNodeFailure, node, map[string]any{"error": err.Error()})
		return err
	}
	spec := c.Container(adapter.ContainerContext{
		NodeID:  node.ID,
		Version: node.Config.Version,
	})
	if len(spec.Ports) == 0 {
		// File-based infra (sqlite) — nothing to start
		e.graph.SetState(node.ID, graph.StateHealthy)
		e.emit(plugin.EventAfterNodeStart, node, nil)
		e.emit(plugin.EventAfterNodeHealthy, node, nil)
		return nil
	}
	args := container.ToRunArgs(spec, node.ID)

	p, err := e.pm.Spawn(node.ID, "docker", args, nil, "")
	if err != nil {
		e.graph.SetState(node.ID, graph.StateFailed)
		e.emit(plugin.EventOnNodeFailure, node, map[string]any{"error": err.Error()})
		return err
	}
	e.containers = append(e.containers, node.ID)
	e.writePIDFile(node.ID, p)
	e.emit(plugin.EventAfterNodeStart, node, nil)

	strategy := health.InfraStrategy("acthur-"+node.ID, spec.Healthcheck.Test, nil)
	if err := e.checker.WaitForStrategy(e.ctx, node, strategy, 60*time.Second); err != nil {
		e.graph.SetState(node.ID, graph.StateDegraded)
		e.emit(plugin.EventOnNodeFailure, node, map[string]any{"error": err.Error()})
		return err
	}

	e.graph.SetState(node.ID, graph.StateHealthy)
	e.emit(plugin.EventAfterNodeHealthy, node, nil)
	return nil
}

// startServiceNode starts a service node using its adapter's dev command.
func (e *DevEngine) startServiceNode(node *graph.Node) error {
	e.graph.SetState(node.ID, graph.StateStarting)
	output.Info(node.ID, "starting %s (%s)...", node.ID, node.Adapter)
	e.emit(plugin.EventBeforeNodeStart, node, nil)

	a, ok := e.resolver.Adapter(node.Adapter)
	if !ok {
		e.graph.SetState(node.ID, graph.StateFailed)
		err := fmt.Errorf("adapter %q not found", node.Adapter)
		e.emit(plugin.EventOnNodeFailure, node, map[string]any{"error": err.Error()})
		return err
	}

	// Build env for this node
	env, err := e.buildEnv(node)
	if err != nil {
		e.graph.SetState(node.ID, graph.StateFailed)
		e.emit(plugin.EventOnNodeFailure, node, map[string]any{"error": err.Error()})
		return err
	}
	r, ok := a.(adapter.Runnable)
	if !ok {
		e.graph.SetState(node.ID, graph.StateFailed)
		err := fmt.Errorf("adapter %q does not support the Runnable capability", node.Adapter)
		e.emit(plugin.EventOnNodeFailure, node, map[string]any{"error": err.Error()})
		return err
	}
	cmd := r.DevCommand(env)

	nodeDir := nodeDirectory(e.cfg.RootDir, node)
	p, err := e.pm.Spawn(node.ID, cmd.Bin, cmd.Args, cmd.Env, nodeDir)
	if err != nil {
		e.graph.SetState(node.ID, graph.StateFailed)
		e.emit(plugin.EventOnNodeFailure, node, map[string]any{"error": err.Error()})
		return err
	}
	e.writePIDFile(node.ID, p)
	e.emit(plugin.EventAfterNodeStart, node, nil)

	// Wait for HTTP health
	// Give the service time to start before health-checking
	time.Sleep(500 * time.Millisecond)
	if err := e.checker.WaitFor(e.ctx, node, 120*time.Second); err != nil {
		e.graph.SetState(node.ID, graph.StateDegraded)
		e.emit(plugin.EventOnNodeFailure, node, map[string]any{"error": err.Error()})
		return err
	}

	e.graph.SetState(node.ID, graph.StateHealthy)
	e.emit(plugin.EventAfterNodeHealthy, node, nil)
	return nil
}

// writePIDFile records p's OS PID under .acthur/run/<node>.pid so a separate
// `acthur service restart <node>` invocation — a different OS process with
// no shared memory with this one — can find and signal it. p is nil in
// tests that use a fake processManager, and Pid() is 0 before the process
// has actually started; both are silently skipped rather than treated as
// errors, since a missing pidfile only degrades `service restart`, not dev.
func (e *DevEngine) writePIDFile(nodeID string, p *process.Process) {
	if e.cfg.RootDir == "" || p == nil {
		return
	}
	pid := p.Pid()
	if pid == 0 {
		return
	}
	if err := process.WritePIDFile(e.cfg.RootDir, nodeID, pid); err != nil {
		output.Warn(nodeID, "could not write pidfile: %v", err)
	}
}

// shutdown stops all processes in reverse order. Containers the engine
// started are stopped through the container runtime first — stopping only
// the docker-run client process would leave them running.
func (e *DevEngine) shutdown(order []*graph.Node) {
	e.cancel()
	if e.cfg.RootDir != "" {
		for _, n := range order {
			_ = process.RemovePIDFile(e.cfg.RootDir, n.ID)
		}
	}
	for i := len(e.containers) - 1; i >= 0; i-- {
		nodeID := e.containers[i]
		if err := e.runDocker(container.ToStopArgs(nodeID)...); err != nil {
			output.Warn(nodeID, "container stop error: %v", err)
		}
	}
	e.containers = nil

	for i := len(order) - 1; i >= 0; i-- {
		if n := order[i]; isLifecycleNode(n) {
			e.emit(plugin.EventBeforeNodeStop, n, nil)
		}
	}

	ids := make([]string, len(order))
	for i, n := range order {
		ids[i] = n.ID
	}
	e.pm.StopAll(ids)

	for i := len(order) - 1; i >= 0; i-- {
		if n := order[i]; isLifecycleNode(n) {
			e.emit(plugin.EventAfterNodeStop, n, nil)
		}
	}
}

// isLifecycleNode reports whether node participates in kernel:node:* event
// emission. kernel-materialized nodes (e.g. the proxy) are run by the engine
// itself, never resolved through the adapter registry, and never emit —
// mirroring the exemption already applied in startNode.
func isLifecycleNode(node *graph.Node) bool {
	return !strings.HasPrefix(node.Adapter, "kernel:")
}

// emit publishes a node lifecycle event on the engine's bus, if one is
// configured. A nil bus makes this a no-op — plugins are entirely optional.
func (e *DevEngine) emit(event plugin.Event, node *graph.Node, extra map[string]any) {
	if e.bus == nil {
		return
	}
	data := map[string]any{"node_type": string(node.Type)}
	for k, v := range extra {
		data[k] = v
	}
	e.bus.Emit(event, plugin.EventPayload{NodeID: node.ID, Data: data})
}

// buildEnv constructs the environment for a node.
// Injects service discovery URLs from data_flow and depends_on edges.
func (e *DevEngine) buildEnv(node *graph.Node) (map[string]string, error) {
	return resolveNodeEnv(e.graph, e.resolver, node, e.secrets, e.cfg.Dev.Port)
}

// resolveNodeEnv builds the environment for node, injecting a discovery URL
// along each outgoing edge. The two edge kinds diverge (ADR 0012):
//
//   - data_flow: contract-governed traffic. The target's discovery URL points
//     at the dev proxy's flow route (http://localhost:<devPort>/_flow/<from>/<to>)
//     so the proxy is the single east-west interception point where the
//     edge's contract is enforced. No Connectable env is injected here —
//     that is an infra-connection concern, not a data_flow concern.
//   - depends_on: an infra dependency. It keeps today's direct node-port URL
//     plus whatever Connectable env the target's adapter exports
//     (DATABASE_URL, etc.) — contracts do not apply to depends_on.
func resolveNodeEnv(g *graph.Graph, resolver AdapterResolver, node *graph.Node, secrets secretStore, devPort int) (map[string]string, error) {
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
		target := g.Node(edge.To)
		if target == nil {
			continue
		}

		switch edge.Type {
		case config.EdgeDataFlow:
			if target.Port != 0 {
				// Contract-governed traffic is routed through the dev proxy's
				// flow route so the proxy can enforce the edge's contract —
				// e.g. web → API_URL=http://localhost:4000/_flow/web/api
				envKey := envKeyFor(edge.To) + "_URL"
				env[envKey] = fmt.Sprintf("http://localhost:%d/_flow/%s/%s", devPort, node.ID, edge.To)
			}
			// data_flow edges carry no Connectable env — contracts, not
			// connection credentials, govern this traffic.

		case config.EdgeDependsOn:
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

// handleFileChange is the watcher.Handler the dev engine registers to react
// to file changes in a service node's directory. A contract file change or a
// change the watcher couldn't attribute to a node never restarts anything —
// only a real service node's own directory triggers a restart, and only when
// its adapter doesn't already reload itself (see adapter.SelfReloader).
func (e *DevEngine) handleFileChange(ev watcher.Event) {
	if ev.NodeID == "" || ev.NodeID == "__unknown__" || ev.NodeID == "__contracts__" {
		return
	}
	node := e.graph.Node(ev.NodeID)
	if node == nil || !node.IsService() {
		return
	}
	a, ok := e.resolver.Adapter(node.Adapter)
	if !ok {
		return
	}
	if sr, ok := a.(adapter.SelfReloader); ok && sr.SelfReloads() {
		output.Debug(node.ID, "file change — adapter self-reloads, skipping restart")
		return
	}

	output.Info(node.ID, "file change detected — restarting...")
	if err := e.pm.Restart(node.ID); err != nil {
		output.Warn(node.ID, "restart failed: %v", err)
		return
	}
	output.Success(node.ID, "restarted")
}

// checkDNS reports whether the project's dev domain and each service node's
// subdomain resolve to 127.0.0.1. It never blocks or aborts startup — it
// only prints copy-pastable /etc/hosts instructions (or, with --write-hosts,
// attempts to append them itself, still without ever running sudo).
func (e *DevEngine) checkDNS() {
	if e.cfg.Dev.Domain == "" {
		return
	}
	var nodeIDs []string
	for _, node := range e.graph.NodesByType(config.NodeTypeService) {
		nodeIDs = append(nodeIDs, node.ID)
	}
	hosts := dns.Hostnames(e.cfg.Dev.Domain, nodeIDs)
	missing := dns.Missing(e.dnsLookup, hosts)
	if len(missing) == 0 {
		return
	}

	if e.writeHosts {
		if err := dns.WriteHostsEntries(e.hostsPath, missing); err == nil {
			output.Success("dns", "wrote %d hostname(s) to %s", len(missing), e.hostsPath)
			return
		} else {
			output.Warn("dns", "could not write %s: %v", e.hostsPath, err)
		}
	}

	output.Warn("dns", "%d dev hostname(s) don't resolve to 127.0.0.1 yet", len(missing))
	fmt.Print(dns.Instructions(missing))
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
