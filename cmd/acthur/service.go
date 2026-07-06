package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/acthur/acthur/internal/adapter"
	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/output"
	"github.com/acthur/acthur/internal/process"
)

// ---------------------------------------------------------------------------
// acthur service logs/health/restart/add
//
// `acthur dev` is a long-running process; every `acthur service <cmd>` here
// is a *separate* CLI invocation with no shared memory with it. Each one
// reads whatever durable, on-disk fact the running dev engine already
// persists under .acthur/ (internal/process/control.go) rather than
// inventing an RPC daemon:
//
//   - logs    tails .acthur/logs/<node>.log, which DevEngine's process
//     Manager keeps appended to via its LogSink.
//   - health  probes the node's own health strategy directly (HTTP for
//     services, TCP for infra) — no dev-engine cooperation needed at all.
//   - restart reads .acthur/run/<node>.pid and signals that process;
//     killing it looks like an ordinary crash to the running Supervisor,
//     which already auto-restarts on unexpected exit.
//   - add     edits acthur.yml's graph.nodes (mirroring acthur add's
//     line-based YAML edit + rollback in cmd/acthur/add.go) and rebuilds
//     the graph to confirm it's still valid.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// service logs <name>
// ---------------------------------------------------------------------------

// runServiceLogs writes path's existing content to out, then polls for
// appended content every pollInterval until stop is closed. It returns a
// pointed error if path does not exist yet (the node has never been started
// under `acthur dev`, or logging hasn't been wired for this project root).
func runServiceLogs(path string, out io.Writer, stop <-chan struct{}, pollInterval time.Duration) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no logs found at %s — is 'acthur dev' running for this node?", path)
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	if _, err := io.Copy(out, f); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-ticker.C:
			if _, err := io.Copy(out, f); err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// service health <name>
// ---------------------------------------------------------------------------

// healthPoller is the seam runServiceHealth depends on — health.Checker.Poll
// in production, a fake in tests.
type healthPoller interface {
	Poll(node *graph.Node) error
}

// runServiceHealth polls nodeID's own health strategy once and reports the
// result. It returns an error (non-zero exit) when the node is unhealthy or
// unknown, so scripting against this command is straightforward.
func runServiceHealth(g *graph.Graph, checker healthPoller, nodeID string) error {
	node := g.Node(nodeID)
	if node == nil {
		return fmt.Errorf("node %q not found in the graph", nodeID)
	}
	if err := checker.Poll(node); err != nil {
		output.Error(nodeID, "unhealthy: %v", err)
		return fmt.Errorf("node %q is unhealthy: %w", nodeID, err)
	}
	output.Success(nodeID, "healthy")
	return nil
}

// ---------------------------------------------------------------------------
// service restart <name>
// ---------------------------------------------------------------------------

// signalFunc delivers a restart signal to pid. The default implementation
// sends SIGTERM — the same graceful-stop signal Process.Stop uses — which
// the running dev engine's Supervisor already treats as an unexpected exit
// and auto-restarts with its normal backoff policy.
type signalFunc func(pid int) error

func defaultSignal(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

// runServiceRestart reads nodeID's pidfile under rootDir and signals that
// process. It returns a pointed error if no pidfile exists — the node was
// never started under `acthur dev` for this project root, or dev isn't
// currently running.
func runServiceRestart(rootDir, nodeID string, signal signalFunc) error {
	pid, err := process.ReadPIDFile(rootDir, nodeID)
	if err != nil {
		return fmt.Errorf(
			"no running process found for node %q — is 'acthur dev' running? (%w)", nodeID, err,
		)
	}
	if signal == nil {
		signal = defaultSignal
	}
	if err := signal(pid); err != nil {
		return fmt.Errorf("signal process %d for node %q: %w", pid, nodeID, err)
	}
	output.Success(nodeID, "restart signal sent (pid %d) — the running dev engine will bring it back", pid)
	return nil
}

// ---------------------------------------------------------------------------
// service add <infra>
// ---------------------------------------------------------------------------

// serviceAddSummary is the result of runServiceAdd, used to print the CLI
// summary.
type serviceAddSummary struct {
	NodeID     string
	Adapter    string
	AlreadyHad bool
}

// runServiceAdd adds an infra node named nodeID (or one derived from
// adapterKey when nodeID is empty) to acthur.yml's graph.nodes, rebuilding
// the graph afterward to confirm the result is still valid. A failed add
// never leaves a half-written acthur.yml behind — the original bytes are
// restored on any error after the edit.
func runServiceAdd(root, adapterKey, nodeID string) (*serviceAddSummary, error) {
	if _, err := adapter.Resolve(adapterKey); err != nil {
		return nil, fmt.Errorf("unknown adapter %q: %w", adapterKey, err)
	}

	if nodeID == "" {
		nodeID = deriveNodeIDFromAdapterKey(adapterKey)
	}
	if nodeID == "" {
		return nil, fmt.Errorf("could not derive a node name from adapter %q — pass --name", adapterKey)
	}

	ymlPath, err := config.Find(root)
	if err != nil {
		return nil, err
	}

	original, err := os.ReadFile(ymlPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ymlPath, err)
	}

	alreadyHad, err := addInfraNodeToYAML(ymlPath, nodeID, adapterKey)
	if err != nil {
		return nil, fmt.Errorf("updating %s: %w", ymlPath, err)
	}
	rollback := func() {
		if !alreadyHad {
			_ = os.WriteFile(ymlPath, original, 0o644)
		}
	}

	cfg, err := config.LoadFile(ymlPath)
	if err != nil {
		rollback()
		return nil, fmt.Errorf("reloading %s: %w", ymlPath, err)
	}

	if _, err := graph.Build(cfg); err != nil {
		rollback()
		return nil, fmt.Errorf("building graph: %w", err)
	}

	return &serviceAddSummary{NodeID: nodeID, Adapter: adapterKey, AlreadyHad: alreadyHad}, nil
}

// deriveNodeIDFromAdapterKey derives a graph node ID from an adapter key
// ("cache:redis" -> "redis"). Used when --name is not given.
func deriveNodeIDFromAdapterKey(adapterKey string) string {
	parts := strings.SplitN(adapterKey, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return ""
	}
	return parts[1]
}

// addInfraNodeToYAML inserts a new infra node entry under acthur.yml's
// graph.nodes map at path. Returns true (and does nothing) if a node with
// that ID already exists — the operation is idempotent, mirroring
// addPluginToYAML in add.go.
//
// Like addPluginToYAML, this is a deliberately narrow line-based insertion:
// there is no comment-preserving YAML writer in internal/config, so a full
// yaml.Marshal round-trip would drop every comment in the file. This only
// ever touches the graph.nodes block.
func addInfraNodeToYAML(path, nodeID, adapterKey string) (alreadyPresent bool, err error) {
	cfg, err := config.LoadFile(path)
	if err != nil {
		return false, err
	}
	if _, exists := cfg.Graph.Nodes[nodeID]; exists {
		return true, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	text := string(data)

	lines := strings.Split(text, "\n")
	nodesLine := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t") == "  nodes:" {
			nodesLine = i
			break
		}
	}
	if nodesLine == -1 {
		return false, fmt.Errorf("could not find a 'graph:\n  nodes:' section in %s", path)
	}

	insertAt := nodesLine + 1
	for insertAt < len(lines) && isGraphNodesContinuationLine(lines[insertAt]) {
		insertAt++
	}

	entry := []string{
		fmt.Sprintf("    %s:", nodeID),
		"      type: infra",
		fmt.Sprintf("      adapter: %s", adapterKey),
	}
	newLines := make([]string, 0, len(lines)+len(entry))
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, entry...)
	newLines = append(newLines, lines[insertAt:]...)

	updated := strings.Join(newLines, "\n")
	// insertAt can land past every existing line when the file's trailing
	// "" split element (i.e. its final newline) is itself a continuation
	// line — that would otherwise make the new entry swallow the file's
	// trailing newline. Always leave the file newline-terminated.
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}

	return false, os.WriteFile(path, []byte(updated), 0o644)
}

// isGraphNodesContinuationLine reports whether l is part of the graph.nodes
// map body: a blank line, or a line indented at least 4 spaces (a node key
// or one of its fields). Used to find the insertion point after the last
// existing node entry.
func isGraphNodesContinuationLine(l string) bool {
	if strings.TrimSpace(l) == "" {
		return true
	}
	return strings.HasPrefix(l, "    ")
}
