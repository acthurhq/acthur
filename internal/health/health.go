// Package health provides health check strategies for every node type.
// The dev engine calls WaitFor() after spawning a process and before
// allowing dependent nodes to start.
package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/output"
)

// ---------------------------------------------------------------------------
// Strategy interface
// ---------------------------------------------------------------------------

// Strategy defines how to check whether a node is healthy.
type Strategy interface {
	// Check performs one health check attempt.
	// Returns nil if the node is healthy, an error otherwise.
	Check(ctx context.Context, node *graph.Node) error

	// Name returns a human-readable name for this strategy.
	Name() string
}

// ---------------------------------------------------------------------------
// HTTP Health Check
// ---------------------------------------------------------------------------

// HTTPStrategy checks health by calling GET /health on the service.
type HTTPStrategy struct {
	client *http.Client
}

// NewHTTPStrategy creates an HTTP health check strategy.
func NewHTTPStrategy() *HTTPStrategy {
	return &HTTPStrategy{
		client: &http.Client{
			Timeout: 3 * time.Second,
			// Do not follow redirects — a redirect means the server is up
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (s *HTTPStrategy) Name() string { return "http" }

func (s *HTTPStrategy) Check(ctx context.Context, node *graph.Node) error {
	port := ResolvePort(node)
	if port == 0 {
		return fmt.Errorf("node %q has no port configured", node.ID)
	}
	url := fmt.Sprintf("http://localhost:%d/health", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("not ready: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("not healthy: status %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// TCP Health Check
// ---------------------------------------------------------------------------

// TCPStrategy checks health by opening a TCP connection.
// Used for database and cache infra nodes.
type TCPStrategy struct{}

func (s *TCPStrategy) Name() string { return "tcp" }

func (s *TCPStrategy) Check(ctx context.Context, node *graph.Node) error {
	port := ResolvePort(node)
	if port == 0 {
		return fmt.Errorf("node %q has no port configured", node.ID)
	}
	addr := fmt.Sprintf("localhost:%d", port)

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("TCP connect to %s failed: %w", addr, err)
	}
	conn.Close()
	return nil
}

// ---------------------------------------------------------------------------
// Exec Health Check
// ---------------------------------------------------------------------------

// ExecStrategy runs a command and checks its exit code.
// Used for database nodes where a SELECT 1 or ping is more reliable.
type ExecStrategy struct {
	Cmd    string
	Args   []string
	Runner CommandRunner
}

func (s *ExecStrategy) Name() string { return "exec" }

func (s *ExecStrategy) Check(ctx context.Context, node *graph.Node) error {
	runner := s.Runner
	if runner == nil {
		runner = osExecRunner{}
	}
	if err := runner.Run(ctx, s.Cmd, s.Args...); err != nil {
		return fmt.Errorf("exec health check failed: %w", err)
	}
	return nil
}

// CommandRunner runs a command for ExecStrategy.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

// CommandRunnerFunc adapts a function into a CommandRunner.
type CommandRunnerFunc func(ctx context.Context, name string, args ...string) error

func (f CommandRunnerFunc) Run(ctx context.Context, name string, args ...string) error {
	return f(ctx, name, args...)
}

type osExecRunner struct{}

func (osExecRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// InfraStrategy selects the readiness strategy for an infra container.
func InfraStrategy(containerName string, healthcheck []string, runner CommandRunner) Strategy {
	if len(healthcheck) == 0 {
		return &TCPStrategy{}
	}
	return &ExecStrategy{
		Cmd:    "docker",
		Args:   dockerExecArgs(containerName, healthcheck),
		Runner: runner,
	}
}

func dockerExecArgs(containerName string, healthcheck []string) []string {
	args := []string{"exec", containerName}
	switch healthcheck[0] {
	case "CMD-SHELL":
		if len(healthcheck) > 1 {
			return append(args, "sh", "-c", healthcheck[1])
		}
	case "CMD":
		return append(args, healthcheck[1:]...)
	}
	return append(args, healthcheck...)
}

// ---------------------------------------------------------------------------
// Checker
// ---------------------------------------------------------------------------

// Checker orchestrates health checks for all nodes.
// It selects the right strategy per node type and adapter.
type Checker struct {
	http *HTTPStrategy
	tcp  *TCPStrategy
}

// New creates a health checker.
func New() *Checker {
	return &Checker{
		http: NewHTTPStrategy(),
		tcp:  &TCPStrategy{},
	}
}

// WaitFor polls a node's health check until healthy or timeout.
// It logs progress and returns an error if the timeout expires.
func (c *Checker) WaitFor(ctx context.Context, node *graph.Node, timeout time.Duration) error {
	strategy := c.strategyFor(node)
	return c.WaitForStrategy(ctx, node, strategy, timeout)
}

// WaitForStrategy polls a node with the provided health check strategy until
// healthy or timeout.
func (c *Checker) WaitForStrategy(ctx context.Context, node *graph.Node, strategy Strategy, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	interval := 500 * time.Millisecond
	attempt := 0

	output.Info(node.ID, "waiting for %s health check...", strategy.Name())

	for time.Now().Before(deadline) {
		attempt++
		checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := strategy.Check(checkCtx, node)
		cancel()

		if err == nil {
			output.Success(node.ID, "healthy ✓ (after %d attempt(s))", attempt)
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for %q", node.ID)
		case <-time.After(interval):
			// Increase interval with a cap (adaptive polling)
			if interval < 3*time.Second {
				interval += 250 * time.Millisecond
			}
		}
	}

	return fmt.Errorf(
		"node %q did not become healthy within %s\n"+
			"  Last check: %s strategy\n"+
			"  Check 'acthur service logs %s' for details",
		node.ID, timeout, strategy.Name(), node.ID,
	)
}

// Poll runs a single health check on a node.
func (c *Checker) Poll(node *graph.Node) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return c.strategyFor(node).Check(ctx, node)
}

// strategyFor selects the right health check strategy for a node.
func (c *Checker) strategyFor(node *graph.Node) Strategy {
	switch {
	case node.IsService():
		return c.http // all services must expose GET /health
	case node.IsInfra():
		return c.infraStrategy(node)
	default:
		return c.tcp
	}
}

// infraStrategy picks a strategy based on the infra adapter.
func (c *Checker) infraStrategy(node *graph.Node) Strategy {
	switch node.Adapter {
	case "db:postgres", "db:mysql", "db:sqlite",
		"cache:redis",
		"queue:nats":
		return c.tcp
	default:
		return c.tcp
	}
}

// DefaultPorts provides well-known default ports for infra adapters.
// Used when no port is explicitly set in acthur.yml.
var DefaultPorts = map[string]int{
	"db:postgres":            5432,
	"db:mysql":               3306,
	"db:sqlite":              0, // file-based, no port
	"cache:redis":            6379,
	"storage:minio":          9000,
	"queue:nats":             4222,
	"analytics:posthog":      8000,
	"webanalytics:plausible": 8001,
}

// ResolvePort returns the port for a node, falling back to the default
// for its adapter if no port is configured.
func ResolvePort(node *graph.Node) int {
	if node.Port != 0 {
		return node.Port
	}
	if p, ok := DefaultPorts[node.Adapter]; ok {
		return p
	}
	return 0
}
