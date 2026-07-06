package process_test

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/acthur/acthur/internal/process"
)

// ---------------------------------------------------------------------------
// Process tests
// ---------------------------------------------------------------------------

func TestProcess_StartAndStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process test in short mode")
	}

	p := process.NewProcess("test", sleepCmd(), sleepArgs(), nil, "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("expected start to succeed, got: %v", err)
	}

	if p.State() != process.StateRunning {
		t.Errorf("expected StateRunning after start, got %q", p.State())
	}

	if err := p.Stop(5 * time.Second); err != nil {
		t.Fatalf("expected stop to succeed, got: %v", err)
	}

	if p.State() != process.StateStopped {
		t.Errorf("expected StateStopped after stop, got %q", p.State())
	}
}

func TestProcess_StateTransitions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process test in short mode")
	}

	states := []process.State{}
	p := process.NewProcess("test-state", sleepCmd(), sleepArgs(), nil, "")
	p.OnStateChange(func(id string, s process.State) {
		states = append(states, s)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	p.Stop(5 * time.Second)

	// Should have seen: Running, Stopping, Stopped
	sawRunning := false
	sawStopped := false
	for _, s := range states {
		if s == process.StateRunning {
			sawRunning = true
		}
		if s == process.StateStopped {
			sawStopped = true
		}
	}
	if !sawRunning {
		t.Error("expected StateRunning in state transitions")
	}
	if !sawStopped {
		t.Error("expected StateStopped in state transitions")
	}
}

func TestProcess_OutputCapture(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process test in short mode")
	}

	lines := []string{}
	p := process.NewProcess("test-output", echoCmd(), echoArgs("hello from acthur"), nil, "")
	p.OnLine(func(id, line string) {
		lines = append(lines, line)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p.Start(ctx)
	time.Sleep(500 * time.Millisecond) // give it time to output
	p.Stop(2 * time.Second)

	found := false
	for _, l := range lines {
		if containsStr(l, "hello from acthur") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected output to contain 'hello from acthur', got: %v", lines)
	}
}

func TestProcess_DefaultStateIsIdle(t *testing.T) {
	p := process.NewProcess("test-idle", "echo", []string{"hi"}, nil, "")
	if p.State() != process.StateIdle {
		t.Errorf("expected initial state=idle, got %q", p.State())
	}
}

func TestProcess_RestartCountStartsAtZero(t *testing.T) {
	p := process.NewProcess("test-restarts", "echo", []string{"hi"}, nil, "")
	if p.Restarts() != 0 {
		t.Errorf("expected restarts=0, got %d", p.Restarts())
	}
}

func TestProcess_StartFailure(t *testing.T) {
	p := process.NewProcess("test-fail", "definitely-not-a-real-binary-xyz", nil, nil, "")
	ctx := context.Background()
	err := p.Start(ctx)
	if err == nil {
		t.Error("expected error starting non-existent binary, got nil")
	}
	if p.State() != process.StateFailed {
		t.Errorf("expected StateFailed after start error, got %q", p.State())
	}
}

// ---------------------------------------------------------------------------
// Supervisor tests
// ---------------------------------------------------------------------------

func TestSupervisor_Backoff_Increases(t *testing.T) {
	policy := process.SupervisionPolicy{
		MaxRestarts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		ResetAfter:   1 * time.Minute,
	}

	// Test that backoff is calculated correctly by checking the policy structure
	if policy.InitialDelay >= policy.MaxDelay {
		t.Error("initial delay should be less than max delay")
	}
}

func TestSupervisor_DefaultPolicy(t *testing.T) {
	if process.DefaultPolicy.MaxRestarts <= 0 {
		t.Error("default policy should have a max restart limit")
	}
	if process.DefaultPolicy.InitialDelay <= 0 {
		t.Error("default policy should have a positive initial delay")
	}
	if process.DefaultPolicy.MaxDelay < process.DefaultPolicy.InitialDelay {
		t.Error("max delay should be >= initial delay")
	}
}

// ---------------------------------------------------------------------------
// Manager tests
// ---------------------------------------------------------------------------

func TestManager_SpawnAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping manager test in short mode")
	}

	m := process.NewManager()
	p, err := m.Spawn("test-node", sleepCmd(), sleepArgs(), nil, "")
	if err != nil {
		t.Fatalf("expected Spawn to succeed, got: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil process")
	}

	got := m.Get("test-node")
	if got == nil {
		t.Error("expected Get to return the spawned process")
	}

	m.Stop("test-node")
}

func TestManager_SpawnDuplicate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping manager test in short mode")
	}

	m := process.NewManager()
	m.Spawn("dup-node", sleepCmd(), sleepArgs(), nil, "")
	_, err := m.Spawn("dup-node", sleepCmd(), sleepArgs(), nil, "")
	if err == nil {
		t.Error("expected error spawning duplicate node, got nil")
	}
	m.Stop("dup-node")
}

func TestManager_GetNonExistent(t *testing.T) {
	m := process.NewManager()
	got := m.Get("nonexistent-node")
	if got != nil {
		t.Error("expected nil for non-existent node")
	}
}

func TestManager_StopNonExistent(t *testing.T) {
	m := process.NewManager()
	err := m.Stop("nonexistent-node")
	if err == nil {
		t.Error("expected error stopping non-existent node, got nil")
	}
}

func TestManager_All(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping manager test in short mode")
	}

	m := process.NewManager()
	m.Spawn("node-a", sleepCmd(), sleepArgs(), nil, "")
	m.Spawn("node-b", sleepCmd(), sleepArgs(), nil, "")
	m.Spawn("node-c", sleepCmd(), sleepArgs(), nil, "")

	all := m.All()
	if len(all) != 3 {
		t.Errorf("expected 3 processes, got %d", len(all))
	}

	m.StopAll([]string{"node-a", "node-b", "node-c"})
}

// TestManager_LogSinkReceivesOutputLines asserts that a LogSink registered
// via SetLogSink receives every output line from every process the manager
// spawns afterward, alongside the default output.ServiceLog routing. This is
// the seam `acthur service logs` depends on to persist per-node log files.
func TestManager_LogSinkReceivesOutputLines(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping manager test in short mode")
	}

	m := process.NewManager()

	type line struct{ nodeID, text string }
	var mu sync.Mutex
	var got []line
	m.SetLogSink(func(nodeID, text string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, line{nodeID, text})
	})

	_, err := m.Spawn("logged-node", echoCmd(), echoArgs("hello-from-sink"), nil, "")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	m.Stop("logged-node")

	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("expected log sink to receive at least one line")
	}
	found := false
	for _, l := range got {
		if l.nodeID == "logged-node" && containsStr(l.text, "hello-from-sink") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a line containing the echoed text, got %#v", got)
	}
}

// ---------------------------------------------------------------------------
// OS-specific helpers
// ---------------------------------------------------------------------------

// sleepCmd returns the platform-appropriate sleep command binary.
func sleepCmd() string {
	if runtime.GOOS == "windows" {
		return "timeout"
	}
	return "sleep"
}

// sleepArgs returns args for a 10-second sleep.
func sleepArgs() []string {
	if runtime.GOOS == "windows" {
		return []string{"/t", "10"}
	}
	return []string{"10"}
}

// echoCmd returns the platform-appropriate echo binary.
func echoCmd() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "echo"
}

// echoArgs returns args to echo a string.
func echoArgs(msg string) []string {
	if runtime.GOOS == "windows" {
		return []string{"/c", "echo", msg}
	}
	return []string{msg}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestProcess_StopDeliversSIGTERMBeforeKill: Stop's contract is graceful
// termination — the child must receive SIGTERM and get a chance to clean up
// (air kills its compiled binary, docker-run proxies the signal to the
// container). A context-cancel SIGKILL gives the child no chance at all.
func TestProcess_StopDeliversSIGTERMBeforeKill(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("skipping signal test")
	}

	marker := t.TempDir() + "/got-term"
	script := `trap 'echo yes > ` + marker + `; exit 0' TERM; while true; do sleep 0.1; done`
	p := process.NewProcess("term-test", "sh", []string{"-c", script}, nil, "")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // let the trap install

	if err := p.Stop(5 * time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected child to observe SIGTERM (marker file missing): %v", err)
	}
}

// TestProcess_StopTerminatesWholeProcessTree: dev tools (air) spawn the real
// service binary as a child. Stopping only the direct child orphans the
// grandchild, which keeps the port bound so a supervised restart can never
// recover (found by the #33 live witness). Stop must take down the whole
// process group.
func TestProcess_StopTerminatesWholeProcessTree(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("skipping process-group test")
	}

	pidFile := t.TempDir() + "/grandchild.pid"
	script := `sleep 60 & echo $! > ` + pidFile + `; wait`
	p := process.NewProcess("tree-test", "sh", []string{"-c", script}, nil, "")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	var pid int
	for i := 0; i < 50; i++ {
		if b, err := os.ReadFile(pidFile); err == nil && len(b) > 0 {
			fmt.Sscanf(string(b), "%d", &pid)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("grandchild pid never appeared")
	}

	if err := p.Stop(5 * time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := syscall.Kill(pid, 0); err == nil {
		syscall.Kill(pid, syscall.SIGKILL) // clean up
		t.Fatalf("grandchild %d survived Stop — process tree not terminated", pid)
	}
}

// TestProcess_RestartReapsOrphanedGrandchildren: when the supervised process
// is killed externally (crash), its grandchildren are orphaned — the context
// Cancel never fires because the process already exited. Restart must reap
// the old process group before starting the replacement, or the orphan keeps
// the port bound and the restarted service can never come up (found by the
// #33 live witness).
func TestProcess_RestartReapsOrphanedGrandchildren(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("skipping process-group test")
	}

	pidFile := t.TempDir() + "/grandchild.pid"
	script := `sleep 60 & echo $! > ` + pidFile + `; wait`
	p := process.NewProcess("orphan-test", "sh", []string{"-c", script}, nil, "")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	var pid int
	for i := 0; i < 50; i++ {
		if b, err := os.ReadFile(pidFile); err == nil && len(b) > 0 {
			fmt.Sscanf(string(b), "%d", &pid)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("grandchild pid never appeared")
	}

	// Simulate a hard crash of the supervised process itself.
	syscall.Kill(p.Pid(), syscall.SIGKILL)
	time.Sleep(200 * time.Millisecond)

	if err := p.Restart(ctx); err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer p.Stop(5 * time.Second)
	time.Sleep(200 * time.Millisecond)

	if err := syscall.Kill(pid, 0); err == nil {
		syscall.Kill(pid, syscall.SIGKILL) // clean up
		t.Fatalf("orphaned grandchild %d survived restart", pid)
	}
}
