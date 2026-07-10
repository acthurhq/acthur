package watcher_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/watcher"
)

// ---------------------------------------------------------------------------
// Basic watcher tests
// ---------------------------------------------------------------------------

func TestWatcher_DetectsFileModification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping watcher test in short mode")
	}

	root := t.TempDir()
	apiDir := filepath.Join(root, "services", "api")
	_ = os.MkdirAll(apiDir, 0755)

	// Write initial file
	srcFile := filepath.Join(apiDir, "handler.go")
	_ = os.WriteFile(srcFile, []byte("package api"), 0644)

	g := buildWatcherGraph(t, root)
	w := watcher.New(g, root, 50*time.Millisecond)

	events := make(chan watcher.Event, 10)
	w.OnChange(func(e watcher.Event) {
		events <- e
	})
	w.Start()
	defer w.Stop()

	// Wait for initial snapshot
	time.Sleep(100 * time.Millisecond)

	// Modify the file
	_ = os.WriteFile(srcFile, []byte("package api\n// modified"), 0644)

	select {
	case event := <-events:
		if event.ChangeType != watcher.ChangeModified {
			t.Errorf("expected ChangeModified, got %q", event.ChangeType)
		}
		if event.NodeID != "api" {
			t.Errorf("expected nodeID=api, got %q", event.NodeID)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: expected file change event within 2s")
	}
}

func TestWatcher_DetectsFileCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping watcher test in short mode")
	}

	root := t.TempDir()
	apiDir := filepath.Join(root, "services", "api")
	_ = os.MkdirAll(apiDir, 0755)

	g := buildWatcherGraph(t, root)
	w := watcher.New(g, root, 50*time.Millisecond)

	events := make(chan watcher.Event, 10)
	w.OnChange(func(e watcher.Event) {
		if e.ChangeType == watcher.ChangeCreated {
			events <- e
		}
	})
	w.Start()
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	// Create a new file
	newFile := filepath.Join(apiDir, "service.go")
	_ = os.WriteFile(newFile, []byte("package api"), 0644)

	select {
	case event := <-events:
		if event.ChangeType != watcher.ChangeCreated {
			t.Errorf("expected ChangeCreated, got %q", event.ChangeType)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: expected file creation event within 2s")
	}
}

func TestWatcher_IgnoresNodeModules(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping watcher test in short mode")
	}

	root := t.TempDir()
	apiDir := filepath.Join(root, "services", "api")
	nodeModulesDir := filepath.Join(apiDir, "node_modules", "pkg")
	_ = os.MkdirAll(nodeModulesDir, 0755)

	// Write initial real file
	_ = os.WriteFile(filepath.Join(apiDir, "handler.go"), []byte("package api"), 0644)

	g := buildWatcherGraph(t, root)
	w := watcher.New(g, root, 50*time.Millisecond)

	events := make(chan watcher.Event, 10)
	w.OnChange(func(e watcher.Event) {
		events <- e
	})
	w.Start()
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	// Write to node_modules — should be ignored
	_ = os.WriteFile(filepath.Join(nodeModulesDir, "index.js"), []byte("//ignored"), 0644)

	select {
	case e := <-events:
		t.Errorf("expected no event for node_modules change, got: %v", e)
	case <-time.After(300 * time.Millisecond):
		// Correct — no event fired
	}
}

func TestWatcher_IgnoresNonSourceFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping watcher test in short mode")
	}

	root := t.TempDir()
	apiDir := filepath.Join(root, "services", "api")
	_ = os.MkdirAll(apiDir, 0755)
	_ = os.WriteFile(filepath.Join(apiDir, "handler.go"), []byte("package api"), 0644)

	g := buildWatcherGraph(t, root)
	w := watcher.New(g, root, 50*time.Millisecond)

	events := make(chan watcher.Event, 10)
	w.OnChange(func(e watcher.Event) { events <- e })
	w.Start()
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	// Write binary file — should be ignored
	_ = os.WriteFile(filepath.Join(apiDir, "app.exe"), []byte{0x00, 0x01, 0x02}, 0644)

	select {
	case e := <-events:
		t.Errorf("expected no event for .exe file, got: %v", e)
	case <-time.After(300 * time.Millisecond):
		// Correct
	}
}

func TestWatcher_Stop(t *testing.T) {
	root := t.TempDir()
	g := buildWatcherGraph(t, root)
	w := watcher.New(g, root, 50*time.Millisecond)

	fired := false
	w.OnChange(func(e watcher.Event) { fired = true })
	w.Start()
	w.Stop()

	// After stopping, no events should fire
	time.Sleep(200 * time.Millisecond)
	if fired {
		t.Error("expected no events after Stop()")
	}
}

func TestWatcher_MultipleHandlers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping watcher test in short mode")
	}

	root := t.TempDir()
	apiDir := filepath.Join(root, "services", "api")
	_ = os.MkdirAll(apiDir, 0755)
	_ = os.WriteFile(filepath.Join(apiDir, "main.go"), []byte("package main"), 0644)

	g := buildWatcherGraph(t, root)
	w := watcher.New(g, root, 50*time.Millisecond)

	ch1 := make(chan bool, 1)
	ch2 := make(chan bool, 1)
	w.OnChange(func(e watcher.Event) { ch1 <- true })
	w.OnChange(func(e watcher.Event) { ch2 <- true })

	w.Start()
	defer w.Stop()
	time.Sleep(100 * time.Millisecond)

	_ = os.WriteFile(filepath.Join(apiDir, "main.go"), []byte("package main\n//updated"), 0644)

	select {
	case <-ch1:
	case <-time.After(2 * time.Second):
		t.Error("handler 1 was not called")
	}
	select {
	case <-ch2:
	case <-time.After(2 * time.Second):
		t.Error("handler 2 was not called")
	}
}

// ---------------------------------------------------------------------------
// Cascade handler tests
// ---------------------------------------------------------------------------

func TestCascadeHandler_PropagatesDownstream(t *testing.T) {
	// Graph: web → api (data_flow)
	// If api changes, web should be notified (cascade)
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: "service", Adapter: "go:fiber", Port: 8080},
				"web": {Type: "service", Adapter: "ui:astro", Port: 3000},
			},
			Edges: []config.EdgeConfig{
				{From: "web", To: "api", Type: config.EdgeDataFlow,
					Contracts: []string{"contracts/users.contract.yml"}},
			},
		},
	}
	g, _ := graph.Build(cfg)

	cascaded := make(chan string, 5)
	handler := watcher.CascadeHandler(g, func(nodeID string) {
		cascaded <- nodeID
	})

	// Simulate a change in the api service
	handler(watcher.Event{
		NodeID:     "api",
		ChangeType: watcher.ChangeModified,
		Path:       "/project/services/api/handler.go",
		Timestamp:  time.Now(),
	})

	select {
	case nodeID := <-cascaded:
		if nodeID != "web" {
			t.Errorf("expected cascade to web, got %q", nodeID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("expected cascade event within 500ms")
	}
}

func TestCascadeHandler_NoDownstreamNoEvent(t *testing.T) {
	// Graph: api → db (depends_on) — no data_flow edges
	cfg := &config.Config{
		Project: "test",
		Dev:     config.DevConfig{Domain: "test.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: "service", Adapter: "go:fiber"},
				"db":  {Type: "infra", Adapter: "db:postgres"},
			},
			Edges: []config.EdgeConfig{
				{From: "api", To: "db", Type: config.EdgeDependsOn},
			},
		},
	}
	g, _ := graph.Build(cfg)

	cascaded := make(chan string, 1)
	handler := watcher.CascadeHandler(g, func(nodeID string) {
		cascaded <- nodeID
	})

	handler(watcher.Event{
		NodeID:     "api",
		ChangeType: watcher.ChangeModified,
	})

	select {
	case nodeID := <-cascaded:
		t.Errorf("expected no cascade event, got nodeID=%q", nodeID)
	case <-time.After(200 * time.Millisecond):
		// Correct — nothing to cascade to
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func buildWatcherGraph(t *testing.T, root string) *graph.Graph {
	t.Helper()

	apiDir := filepath.Join(root, "services", "api")
	_ = os.MkdirAll(apiDir, 0755)

	cfg := &config.Config{
		Project: "watchtest",
		Dev:     config.DevConfig{Domain: "watchtest.test", Port: 4000},
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: "service", Adapter: "go:fiber", Port: 8080},
				"db":  {Type: "infra", Adapter: "db:postgres"},
			},
			Edges: []config.EdgeConfig{
				{From: "api", To: "db", Type: config.EdgeDependsOn},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}
	return g
}
