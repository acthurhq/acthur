package engine

import (
	"testing"

	"github.com/acthurhq/acthur/internal/adapter"
	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/watcher"
)

// selfReloadingAdapter is a Runnable+SelfReloader fake standing in for
// The go:fiber's real air-backed adapter, without depending on gofiber's
// concrete templates.
type selfReloadingAdapter struct {
	fakeAdapter
}

func (selfReloadingAdapter) SelfReloads() bool { return true }

// TestHandleFileChange_RestartsNodeWhenAdapterDoesNotSelfReload asserts a
// file change in a service node's directory restarts that node's process
// when its adapter has no SelfReloader implementation (the default: a plain
// `go run`/`npm run dev`-style DevCommand does not reload itself).
func TestHandleFileChange_RestartsNodeWhenAdapterDoesNotSelfReload(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	pm := &fakeProcessManager{}
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"go:fiber": fakeAdapter{name: "go:fiber", category: adapter.CategoryBackend},
		},
	})
	eng.pm = pm

	eng.handleFileChange(watcher.Event{NodeID: "api", ChangeType: watcher.ChangeModified})

	if len(pm.restarted) != 1 || pm.restarted[0] != "api" {
		t.Fatalf("expected api to be restarted, got %#v", pm.restarted)
	}
}

// TestHandleFileChange_SkipsRestartWhenAdapterSelfReloads asserts a node
// whose adapter implements SelfReloader (e.g. go:fiber's real air-backed
// adapter) never gets its process restarted by the watcher — air already
// handles its own rebuild+restart, so restarting from the outside would
// just race it for the same port.
func TestHandleFileChange_SkipsRestartWhenAdapterSelfReloads(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	pm := &fakeProcessManager{}
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"go:fiber": selfReloadingAdapter{fakeAdapter{name: "go:fiber", category: adapter.CategoryBackend}},
		},
	})
	eng.pm = pm

	eng.handleFileChange(watcher.Event{NodeID: "api", ChangeType: watcher.ChangeModified})

	if len(pm.restarted) != 0 {
		t.Fatalf("expected no restart for a self-reloading adapter, got %#v", pm.restarted)
	}
}

// TestHandleFileChange_IgnoresContractsAndUnknownAndInfraNodes asserts the
// synthetic node IDs the watcher uses for contract/unattributed changes, and
// infra nodes (which have no user-editable process to restart the same way),
// never trigger a restart attempt.
func TestHandleFileChange_IgnoresContractsAndUnknownAndInfraNodes(t *testing.T) {
	db := &graph.Node{ID: "db", Type: config.NodeTypeInfra, Adapter: "db:custom"}
	g := graph.NewTestGraph(map[string]*graph.Node{"db": db})
	pm := &fakeProcessManager{}
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"db:custom": fakeContainerAdapter{},
		},
	})
	eng.pm = pm

	eng.handleFileChange(watcher.Event{NodeID: "__contracts__", ChangeType: watcher.ChangeModified})
	eng.handleFileChange(watcher.Event{NodeID: "__unknown__", ChangeType: watcher.ChangeModified})
	eng.handleFileChange(watcher.Event{NodeID: "db", ChangeType: watcher.ChangeModified})
	eng.handleFileChange(watcher.Event{NodeID: "missing-node", ChangeType: watcher.ChangeModified})

	if len(pm.restarted) != 0 {
		t.Fatalf("expected no restarts, got %#v", pm.restarted)
	}
}

// TestHandleFileChange_RestartFailureIsNonFatal asserts a Restart error is
// logged, not propagated — handleFileChange runs from a watcher goroutine
// and has no caller to return an error to.
func TestHandleFileChange_RestartFailureIsNonFatal(t *testing.T) {
	api := &graph.Node{ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"}
	g := graph.NewTestGraph(map[string]*graph.Node{"api": api})
	pm := &fakeProcessManager{restartErr: errRestartBoom}
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{
		adapters: map[string]adapter.Adapter{
			"go:fiber": fakeAdapter{name: "go:fiber", category: adapter.CategoryBackend},
		},
	})
	eng.pm = pm

	// Must not panic.
	eng.handleFileChange(watcher.Event{NodeID: "api", ChangeType: watcher.ChangeModified})
}

var errRestartBoom = &boomErr{"restart boom"}

type boomErr struct{ msg string }

func (e *boomErr) Error() string { return e.msg }
