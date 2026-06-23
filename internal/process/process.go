// Package process manages child processes for all service nodes in the graph.
// It spawns processes, pipes their output to the log router, supervises
// them (restarting on crash with exponential backoff), and coordinates
// graceful shutdown.
package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/acthur/acthur/internal/output"
)

// ---------------------------------------------------------------------------
// Process
// ---------------------------------------------------------------------------

// State represents the lifecycle state of a managed process.
type State string

const (
	StateIdle     State = "idle"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateStopped  State = "stopped"
	StateFailed   State = "failed"
)

// Process wraps an os/exec.Cmd with supervision, log routing, and state tracking.
type Process struct {
	NodeID  string
	Bin     string
	Args    []string
	Env     map[string]string
	Dir     string

	cmd      *exec.Cmd
	state    State
	mu       sync.RWMutex
	cancel   context.CancelFunc
	restarts int
	lastExit time.Time
	outputWG sync.WaitGroup

	// exited is closed by the background waiter goroutine started in startLocked
	// when cmd.Wait() returns. Stop() and Supervisor.loop() both read from this
	// channel rather than calling cmd.Wait() directly, avoiding a data race.
	exited chan struct{}

	// Callbacks
	onStateChange func(nodeID string, state State)
	onLine        func(nodeID, line string)
}

// NewProcess creates a new Process. It does not start it.
func NewProcess(nodeID, bin string, args []string, env map[string]string, dir string) *Process {
	return &Process{
		NodeID: nodeID,
		Bin:    bin,
		Args:   args,
		Env:    env,
		Dir:    dir,
		state:  StateIdle,
	}
}

// OnStateChange registers a callback for state transitions.
func (p *Process) OnStateChange(fn func(nodeID string, state State)) {
	p.onStateChange = fn
}

// OnLine registers a callback for each line of stdout/stderr output.
func (p *Process) OnLine(fn func(nodeID, line string)) {
	p.onLine = fn
}

// Start spawns the process. Returns immediately — the process runs in the background.
func (p *Process) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	return p.startLocked(ctx)
}

func (p *Process) startLocked(ctx context.Context) error {
	p.cmd = exec.CommandContext(ctx, p.Bin, p.Args...)

	// Build environment: inherit current env + override with process-specific vars
	p.cmd.Env = os.Environ()
	for k, v := range p.Env {
		p.cmd.Env = append(p.cmd.Env, k+"="+v)
	}

	if p.Dir != "" {
		p.cmd.Dir = p.Dir
	}

	// Pipe stdout and stderr
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("process %q: stdout pipe: %w", p.NodeID, err)
	}
	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("process %q: stderr pipe: %w", p.NodeID, err)
	}

	if err := p.cmd.Start(); err != nil {
		p.setState(StateFailed)
		return fmt.Errorf("process %q: failed to start %q: %w", p.NodeID, p.Bin, err)
	}

	// exited is closed by a single background goroutine that is the sole caller
	// of cmd.Wait(). Stop() and Supervisor.loop() both block on this channel
	// rather than calling cmd.Wait() themselves, eliminating the data race.
	p.exited = make(chan struct{})
	go func() {
		p.cmd.Wait() //nolint:errcheck // exit status is not used here
		close(p.exited)
	}()

	p.setState(StateRunning)

	// Pipe output to log router
	p.outputWG.Add(2)
	go p.pipeLines(stdout)
	go p.pipeLines(stderr)

	return nil
}

// Stop sends SIGTERM to the process and waits up to timeout for it to exit.
// Falls back to SIGKILL if it does not exit within the timeout.
func (p *Process) Stop(timeout time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == StateStopped || p.state == StateIdle {
		return nil
	}

	p.setState(StateStopping)

	if p.cancel != nil {
		p.cancel()
	}

	// Wait for the single background waiter goroutine (started in startLocked)
	// to signal exit via p.exited. This avoids a data race from multiple callers
	// calling cmd.Wait() on the same exec.Cmd.
	exited := p.exited

	select {
	case <-exited:
		p.outputWG.Wait()
		p.setState(StateStopped)
		return nil
	case <-time.After(timeout):
		// Force kill
		if p.cmd != nil && p.cmd.Process != nil {
			p.cmd.Process.Kill()
		}
		p.outputWG.Wait()
		p.setState(StateStopped)
		return nil
	}
}

// Restart stops and restarts the process.
func (p *Process) Restart(ctx context.Context) error {
	if err := p.Stop(5 * time.Second); err != nil {
		return fmt.Errorf("restart: stop failed: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.startLocked(ctx)
}

// State returns the current process state.
func (p *Process) State() State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// waitExited returns the channel that is closed when the process exits.
// Safe to call concurrently; uses the read lock.
func (p *Process) waitExited() <-chan struct{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exited
}

// Restarts returns the number of times this process has been restarted.
func (p *Process) Restarts() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.restarts
}

func (p *Process) setState(s State) {
	p.state = s
	if p.onStateChange != nil {
		p.onStateChange(p.NodeID, s)
	}
}

func (p *Process) pipeLines(r io.Reader) {
	defer p.outputWG.Done()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if p.onLine != nil {
			p.onLine(p.NodeID, line)
		} else {
			output.ServiceLog(p.NodeID, line)
		}
	}
}

// ---------------------------------------------------------------------------
// Supervisor
// ---------------------------------------------------------------------------

// SupervisionPolicy defines how a process is restarted on failure.
type SupervisionPolicy struct {
	MaxRestarts   int           // 0 = unlimited
	InitialDelay  time.Duration // delay before first restart
	MaxDelay      time.Duration // cap on exponential backoff
	ResetAfter    time.Duration // reset restart count if stable for this long
}

// DefaultPolicy is the default supervision policy for service nodes.
var DefaultPolicy = SupervisionPolicy{
	MaxRestarts:  10,
	InitialDelay: 500 * time.Millisecond,
	MaxDelay:     30 * time.Second,
	ResetAfter:   5 * time.Minute,
}

// Supervisor watches a Process and restarts it according to a policy.
type Supervisor struct {
	process *Process
	policy  SupervisionPolicy
	onGiveUp func(nodeID string, restarts int)
}

// NewSupervisor creates a supervisor for the given process.
func NewSupervisor(p *Process, policy SupervisionPolicy) *Supervisor {
	return &Supervisor{process: p, policy: policy}
}

// OnGiveUp registers a callback for when the supervisor gives up restarting.
func (s *Supervisor) OnGiveUp(fn func(nodeID string, restarts int)) {
	s.onGiveUp = fn
}

// Watch starts the supervision loop in a goroutine. It waits for the process
// to exit, then decides whether to restart it based on the policy.
func (s *Supervisor) Watch(ctx context.Context) {
	go s.loop(ctx)
}

func (s *Supervisor) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Wait for the process to finish via the single background waiter goroutine.
		// This avoids the data race of calling cmd.Wait() from multiple goroutines.
		if ch := s.process.waitExited(); ch != nil {
			<-ch
		}

		if s.process.State() == StateStopping || s.process.State() == StateStopped {
			return // intentional stop — do not restart
		}

		s.process.mu.Lock()
		s.process.restarts++
		restarts := s.process.restarts
		lastExit := s.process.lastExit
		s.process.lastExit = time.Now()
		s.process.mu.Unlock()

		// Check if we should give up
		if s.policy.MaxRestarts > 0 && restarts >= s.policy.MaxRestarts {
			output.Error(s.process.NodeID,
				"giving up after %d restart(s) — service is persistently failing",
				restarts)
			s.process.setState(StateFailed)
			if s.onGiveUp != nil {
				s.onGiveUp(s.process.NodeID, restarts)
			}
			return
		}

		// Reset restart count if process was stable for a while
		if !lastExit.IsZero() && time.Since(lastExit) > s.policy.ResetAfter {
			s.process.mu.Lock()
			s.process.restarts = 0
			s.process.mu.Unlock()
			restarts = 1
		}

		// Exponential backoff
		delay := s.backoff(restarts)
		output.Warn(s.process.NodeID,
			"process exited unexpectedly (restart %d/%d) — waiting %s...",
			restarts, s.policy.MaxRestarts, delay)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		output.Info(s.process.NodeID, "restarting...")
		if err := s.process.Restart(ctx); err != nil {
			output.Error(s.process.NodeID, "restart failed: %v", err)
		}
	}
}

func (s *Supervisor) backoff(restarts int) time.Duration {
	delay := s.policy.InitialDelay
	for i := 1; i < restarts; i++ {
		delay *= 2
		if delay > s.policy.MaxDelay {
			return s.policy.MaxDelay
		}
	}
	return delay
}

// ---------------------------------------------------------------------------
// Manager
// ---------------------------------------------------------------------------

// Manager tracks all managed processes in the system.
// It is the single place the dev engine creates and monitors processes.
type Manager struct {
	processes map[string]*Process
	supervisors map[string]*Supervisor
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewManager creates a process manager.
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		processes:   make(map[string]*Process),
		supervisors: make(map[string]*Supervisor),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Spawn creates and starts a new supervised process for a graph node.
func (m *Manager) Spawn(nodeID, bin string, args []string, env map[string]string, dir string) (*Process, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processes[nodeID]; exists {
		return nil, fmt.Errorf("process for node %q is already running", nodeID)
	}

	p := NewProcess(nodeID, bin, args, env, dir)

	// Default line handler: route to output package
	p.OnLine(func(id, line string) {
		output.ServiceLog(id, line)
	})

	if err := p.Start(m.ctx); err != nil {
		return nil, err
	}

	// Start supervision
	sup := NewSupervisor(p, DefaultPolicy)
	sup.Watch(m.ctx)

	m.processes[nodeID] = p
	m.supervisors[nodeID] = sup

	return p, nil
}

// Stop stops a specific process.
func (m *Manager) Stop(nodeID string) error {
	m.mu.RLock()
	p, ok := m.processes[nodeID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no process for node %q", nodeID)
	}
	return p.Stop(10 * time.Second)
}

// Restart restarts a specific process.
func (m *Manager) Restart(nodeID string) error {
	m.mu.RLock()
	p, ok := m.processes[nodeID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no process for node %q", nodeID)
	}
	return p.Restart(m.ctx)
}

// StopAll stops all managed processes in reverse startup order.
// Used for graceful shutdown on Ctrl+C.
func (m *Manager) StopAll(nodeIDs []string) {
	m.cancel() // cancel context — stops supervisors

	// Stop in reverse order provided
	for i := len(nodeIDs) - 1; i >= 0; i-- {
		id := nodeIDs[i]
		m.mu.RLock()
		p, ok := m.processes[id]
		m.mu.RUnlock()
		if !ok {
			continue
		}
		output.Info(id, "stopping...")
		if err := p.Stop(10 * time.Second); err != nil {
			output.Warn(id, "stop error: %v", err)
		} else {
			output.Success(id, "stopped")
		}
	}
}

// Get returns the process for a node ID, or nil if not found.
func (m *Manager) Get(nodeID string) *Process {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.processes[nodeID]
}

// All returns all managed processes.
func (m *Manager) All() map[string]*Process {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*Process, len(m.processes))
	for k, v := range m.processes {
		result[k] = v
	}
	return result
}
