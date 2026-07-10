package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
)

// ---------------------------------------------------------------------------
// service logs
// ---------------------------------------------------------------------------

func TestRunServiceLogs_MissingFileReturnsPointedError(t *testing.T) {
	dir := t.TempDir()
	stop := make(chan struct{})
	close(stop)

	err := runServiceLogs(filepath.Join(dir, "nope.log"), &bytes.Buffer{}, stop, time.Millisecond)
	if err == nil {
		t.Fatal("expected an error for a missing log file")
	}
	if !strings.Contains(err.Error(), "acthur dev") {
		t.Fatalf("expected error to point at 'acthur dev', got %q", err.Error())
	}
}

func TestRunServiceLogs_PrintsExistingContentThenStopsOnSignal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.log")
	if err := os.WriteFile(path, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatalf("seed log file: %v", err)
	}

	stop := make(chan struct{})
	close(stop) // stop immediately after the initial read — no appended content expected

	var out bytes.Buffer
	if err := runServiceLogs(path, &out, stop, time.Millisecond); err != nil {
		t.Fatalf("runServiceLogs: %v", err)
	}
	if got := out.String(); got != "line one\nline two\n" {
		t.Fatalf("expected existing content printed, got %q", got)
	}
}

func TestRunServiceLogs_PicksUpAppendedContentBeforeStopping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.log")
	if err := os.WriteFile(path, []byte("line one\n"), 0o644); err != nil {
		t.Fatalf("seed log file: %v", err)
	}

	stop := make(chan struct{})
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runServiceLogs(path, &out, stop, 5*time.Millisecond)
	}()

	// Give the initial read a moment, then append and let a couple of poll
	// ticks pick it up before stopping.
	time.Sleep(20 * time.Millisecond)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	if _, err := f.WriteString("line two\n"); err != nil {
		t.Fatalf("append: %v", err)
	}
	_ = f.Close()

	time.Sleep(30 * time.Millisecond)
	close(stop)

	if err := <-done; err != nil {
		t.Fatalf("runServiceLogs: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Fatalf("expected both lines printed, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// service health
// ---------------------------------------------------------------------------

type fakeHealthPoller struct {
	err map[string]error
}

func (f fakeHealthPoller) Poll(node *graph.Node) error {
	return f.err[node.ID]
}

func TestRunServiceHealth_ReportsHealthyNode(t *testing.T) {
	g := graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"},
	})
	if err := runServiceHealth(g, fakeHealthPoller{}, "api"); err != nil {
		t.Fatalf("expected healthy node to report no error, got %v", err)
	}
}

func TestRunServiceHealth_ReportsUnhealthyNodeAsError(t *testing.T) {
	g := graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"},
	})
	poller := fakeHealthPoller{err: map[string]error{"api": errors.New("connection refused")}}
	if err := runServiceHealth(g, poller, "api"); err == nil {
		t.Fatal("expected an error for an unhealthy node")
	}
}

func TestRunServiceHealth_UnknownNodeIsError(t *testing.T) {
	g := graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"},
	})
	if err := runServiceHealth(g, fakeHealthPoller{}, "does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown node")
	}
}

// ---------------------------------------------------------------------------
// service restart
// ---------------------------------------------------------------------------

func TestRunServiceRestart_MissingPIDFileReturnsPointedError(t *testing.T) {
	dir := t.TempDir()
	err := runServiceRestart(dir, "api", func(pid int) error { return nil })
	if err == nil {
		t.Fatal("expected an error when no pidfile exists")
	}
	if !strings.Contains(err.Error(), "acthur dev") {
		t.Fatalf("expected error to point at 'acthur dev', got %q", err.Error())
	}
}

func TestRunServiceRestart_SignalsTheRecordedPID(t *testing.T) {
	dir := t.TempDir()
	writePIDFileForTest(t, dir, "api", 4242)

	var signaledPID int
	err := runServiceRestart(dir, "api", func(pid int) error {
		signaledPID = pid
		return nil
	})
	if err != nil {
		t.Fatalf("runServiceRestart: %v", err)
	}
	if signaledPID != 4242 {
		t.Fatalf("expected signal to target pid 4242, got %d", signaledPID)
	}
}

func TestRunServiceRestart_PropagatesSignalError(t *testing.T) {
	dir := t.TempDir()
	writePIDFileForTest(t, dir, "api", 4242)

	err := runServiceRestart(dir, "api", func(pid int) error { return errors.New("no such process") })
	if err == nil {
		t.Fatal("expected the signal error to propagate")
	}
}

func writePIDFileForTest(t *testing.T, root, nodeID string, pid int) {
	t.Helper()
	dir := filepath.Join(root, ".acthur", "run")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, nodeID+".pid"), []byte("4242"), 0o644); err != nil {
		t.Fatalf("write pidfile: %v", err)
	}
}

// ---------------------------------------------------------------------------
// service add
// ---------------------------------------------------------------------------

func TestDeriveNodeIDFromAdapterKey(t *testing.T) {
	cases := map[string]string{
		"cache:redis": "redis",
		"db:postgres": "postgres",
		"nocolon":     "",
		"trailing:":   "",
	}
	for key, want := range cases {
		if got := deriveNodeIDFromAdapterKey(key); got != want {
			t.Errorf("deriveNodeIDFromAdapterKey(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestRunServiceAdd_AppendsInfraNodeAndRebuildsGraph(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)

	summary, err := runServiceAdd(dir, "db:postgres", "")
	if err != nil {
		t.Fatalf("runServiceAdd: %v", err)
	}
	if summary.AlreadyHad {
		t.Fatal("expected a fresh node, not already present")
	}
	if summary.NodeID != "postgres" {
		t.Fatalf("expected derived node ID 'postgres', got %q", summary.NodeID)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	node, ok := cfg.Graph.Nodes["postgres"]
	if !ok {
		t.Fatal("expected 'postgres' node in acthur.yml after add")
	}
	if node.Adapter != "db:postgres" || node.Type != config.NodeTypeInfra {
		t.Fatalf("unexpected node config: %#v", node)
	}

	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("graph.Build after add: %v", err)
	}
	if g.Node("postgres") == nil {
		t.Fatal("expected 'postgres' node in the rebuilt graph")
	}
	// The original api node must be untouched.
	if g.Node("api") == nil {
		t.Fatal("expected the original 'api' node to survive the edit")
	}
}

func TestRunServiceAdd_HonorsNameFlag(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)

	summary, err := runServiceAdd(dir, "db:postgres", "primary-db")
	if err != nil {
		t.Fatalf("runServiceAdd: %v", err)
	}
	if summary.NodeID != "primary-db" {
		t.Fatalf("expected --name to override the derived ID, got %q", summary.NodeID)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if _, ok := cfg.Graph.Nodes["primary-db"]; !ok {
		t.Fatal("expected 'primary-db' node in acthur.yml")
	}
}

func TestRunServiceAdd_IdempotentWhenNodeAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runServiceAdd(dir, "db:postgres", ""); err != nil {
		t.Fatalf("first add: %v", err)
	}
	summary, err := runServiceAdd(dir, "db:postgres", "")
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	if !summary.AlreadyHad {
		t.Fatal("expected the second add to report AlreadyHad")
	}
}

// TestRunServiceAdd_LeavesFileNewlineTerminated is a regression test: when
// the node being added is the last thing in graph.nodes (the common single-
// node-project case exercised by writeTestProject), the insertion point can
// land past every real line (the file's trailing newline is itself a
// "continuation" line) — earlier this silently ate the file's final newline.
func TestRunServiceAdd_LeavesFileNewlineTerminated(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runServiceAdd(dir, "db:postgres", ""); err != nil {
		t.Fatalf("runServiceAdd: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatalf("read acthur.yml: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("expected acthur.yml to remain newline-terminated, got:\n%s", data)
	}
}

func TestRunServiceAdd_UnknownAdapterLeavesYAMLUntouched(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)
	before, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatalf("read acthur.yml: %v", err)
	}

	if _, err := runServiceAdd(dir, "cache:redis-not-registered", ""); err == nil {
		t.Fatal("expected an error for an unregistered adapter")
	}

	after, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatalf("read acthur.yml after failed add: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected acthur.yml untouched after a failed add\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
