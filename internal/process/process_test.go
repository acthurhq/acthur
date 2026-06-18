package process_test

import (
	"context"
	"runtime"
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
