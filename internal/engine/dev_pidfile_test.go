package engine

import (
	"runtime"
	"testing"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/process"
)

func sleepCmdForTest() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "ping", "-n", "10", "127.0.0.1"}
	}
	return "sleep", []string{"10"}
}

// TestWritePIDFile_WritesRealPIDWhenRootDirSet asserts writePIDFile persists
// a real spawned process's PID under .acthur/run/<node>.pid when the engine
// has a real project root — the file `acthur service restart` reads to find
// the process to signal.
func TestWritePIDFile_WritesRealPIDWhenRootDirSet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-process test in short mode")
	}
	root := t.TempDir()
	pm := process.NewManager()
	defer pm.StopAll([]string{"api"})

	bin, args := sleepCmdForTest()
	p, err := pm.Spawn("api", bin, args, nil, "")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	g := graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"},
	})
	eng := NewDevEngine(&config.Config{RootDir: root}, g, fakeDevResolver{})

	eng.writePIDFile("api", p)

	pid, err := process.ReadPIDFile(root, "api")
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}
	if pid != p.Pid() {
		t.Fatalf("expected pidfile to contain %d, got %d", p.Pid(), pid)
	}
}

// TestWritePIDFile_NoopWithoutRootDir asserts writePIDFile never touches disk
// when the engine has no project root — the vast majority of engine unit
// tests construct a bare &config.Config{} and must never see filesystem
// side effects.
func TestWritePIDFile_NoopWithoutRootDir(t *testing.T) {
	g := graph.NewTestGraph(map[string]*graph.Node{
		"api": {ID: "api", Type: config.NodeTypeService, Adapter: "go:fiber"},
	})
	eng := NewDevEngine(&config.Config{}, g, fakeDevResolver{})

	// Must not panic even with a nil process (as fakeProcessManager returns).
	eng.writePIDFile("api", nil)
}
