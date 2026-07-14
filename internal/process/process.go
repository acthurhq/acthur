// Package process manages child processes for all service nodes in the graph.
// It spawns processes, pipes their output to the log router, supervises
// them (restarting on crash with exponential backoff), and coordinates
// graceful shutdown.
package process

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/acthurhq/acthur/internal/output"
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
	NodeID string
	Bin    string
	Args   []string
	Env    map[string]string
	Dir    string

	cmd       *exec.Cmd
	state     State
	mu        sync.RWMutex
	cancel    context.CancelFunc
	restarts  int
	lastExit  time.Time
	ownerID   string
	managerID string

	// exited is closed by the background waiter goroutine started in startLocked
	// when cmd.Wait() returns. Stop() and Supervisor.loop() both read from this
	// channel rather than calling cmd.Wait() directly, avoiding a data race.
	exited chan struct{}

	// Callbacks
	onStateChange func(nodeID string, state State)
	onLine        func(nodeID, line string)
}

// ownedProcess identifies a descendant by PID plus an OS-provided birth
// identity, preventing shutdown cleanup from signalling a recycled PID.
type ownedProcess struct {
	pid      int
	identity string
}

// NewProcess creates a new Process. It does not start it.
func NewProcess(nodeID, bin string, args []string, env map[string]string, dir string) *Process {
	return newProcess(nodeID, bin, args, env, dir, "")
}

func newProcess(nodeID, bin string, args []string, env map[string]string, dir, managerID string) *Process {
	ownerBytes := make([]byte, 16)
	if _, err := rand.Read(ownerBytes); err != nil {
		ownerBytes = []byte(fmt.Sprintf("%d-%s", time.Now().UnixNano(), nodeID))
	}
	return &Process{
		NodeID:    nodeID,
		Bin:       bin,
		Args:      args,
		Env:       env,
		Dir:       dir,
		state:     StateIdle,
		ownerID:   hex.EncodeToString(ownerBytes),
		managerID: managerID,
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

	// Graceful cancellation: deliver SIGTERM to the whole process group so the
	// full tree can clean up (air stops its compiled binary, docker-run proxies
	// the signal into the container). The default context-cancel behavior is an
	// immediate SIGKILL of only the direct child, which gives it no chance and
	// orphans grandchildren — an orphan keeps its port bound, so a supervised
	// restart could never recover. Stop() still force-kills on timeout.
	setProcessGroup(p.cmd)
	cmd := p.cmd
	p.cmd.Cancel = func() error {
		return terminateTree(cmd)
	}

	// Build environment: inherit current env + override with process-specific vars
	p.cmd.Env = os.Environ()
	for k, v := range p.Env {
		p.cmd.Env = append(p.cmd.Env, k+"="+v)
	}
	// This opaque marker is inherited by the workload tree and survives
	// supervisor exit, reparenting, process-group changes, and new sessions.
	p.cmd.Env = append(p.cmd.Env, processOwnerEnv+"="+p.ownerID)
	if p.managerID != "" {
		p.cmd.Env = append(p.cmd.Env, managerOwnerEnv+"="+p.managerID)
	}

	if p.Dir != "" {
		p.cmd.Dir = p.Dir
	}

	// Let os/exec own the output-copy lifecycle. Wait does not return until
	// writes to non-file Stdout/Stderr destinations complete, so a short-lived
	// command cannot lose its final output when Wait closes its pipes.
	stdout := &lineWriter{emit: p.emitLine}
	stderr := &lineWriter{emit: p.emitLine}
	p.cmd.Stdout = stdout
	p.cmd.Stderr = stderr

	if err := p.cmd.Start(); err != nil {
		p.setState(StateFailed)
		return fmt.Errorf("process %q: failed to start %q: %w", p.NodeID, p.Bin, err)
	}

	p.setState(StateRunning)

	// exited is closed by a single background goroutine that is the sole caller
	// of cmd.Wait(). Stop() and Supervisor.loop() both block on this channel
	// rather than calling cmd.Wait() themselves, eliminating the data race.
	exited := make(chan struct{})
	p.exited = exited
	go func() {
		cmd.Wait() //nolint:errcheck // exit status is not used here
		stdout.flush()
		stderr.flush()
		close(exited)
	}()

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
	var descendants []ownedProcess
	if p.cmd != nil && p.cmd.Process != nil {
		// Snapshot before signalling: reload supervisors may exit and reparent
		// children outside their original process group, after which ancestry is
		// no longer discoverable from the supervisor PID.
		descendants = mergeOwned(snapshotDescendants(p.cmd.Process.Pid), snapshotOwned(p.ownerID))
		terminateOwned(descendants)
	}

	if p.cancel != nil {
		p.cancel()
	}

	// Wait for the single background waiter goroutine (started in startLocked)
	// to signal exit via p.exited. This avoids a data race from multiple callers
	// calling cmd.Wait() on the same exec.Cmd.
	exited := p.exited

	select {
	case <-exited:
		// The supervised process exiting does not prove its process group is
		// empty. Self-reloading tools can leave their current compiled child
		// alive (and holding its port) after the supervisor handles SIGTERM.
		// Reap any survivors before reporting a successful shutdown.
		if p.cmd != nil && p.cmd.Process != nil {
			killTree(p.cmd) //nolint:errcheck // group may already be empty
		}
		killOwned(descendants)
		p.setState(StateStopped)
		return nil
	case <-time.After(timeout):
		// Force kill the whole process group — killing only the direct child
		// would orphan grandchildren (air's compiled binary).
		if p.cmd != nil && p.cmd.Process != nil {
			killTree(p.cmd) //nolint:errcheck // best-effort force kill
		}
		killOwned(descendants)
		p.setState(StateStopped)
		return nil
	}
}

func mergeOwned(groups ...[]ownedProcess) []ownedProcess {
	seen := make(map[string]bool)
	var out []ownedProcess
	for _, group := range groups {
		for _, p := range group {
			key := fmt.Sprintf("%d:%s", p.pid, p.identity)
			if !seen[key] {
				seen[key] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// Restart stops and restarts the process.
func (p *Process) Restart(ctx context.Context) error {
	if err := p.Stop(5 * time.Second); err != nil {
		return fmt.Errorf("restart: stop failed: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// If the process crashed (rather than being stopped by us), the context
	// Cancel never fired — it only runs while the process is alive — so its
	// process group may still hold orphaned grandchildren. Reap them before
	// starting the replacement, or an orphan keeps the port bound and the
	// restarted service can never come up.
	if p.cmd != nil && p.cmd.Process != nil {
		killTree(p.cmd) //nolint:errcheck // best-effort orphan reaping
	}
	return p.startLocked(ctx)
}

// Pid returns the OS process ID of the running process, or 0 if not running.
func (p *Process) Pid() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
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

func (p *Process) emitLine(line string) {
	if p.onLine != nil {
		p.onLine(p.NodeID, line)
	} else {
		output.ServiceLog(p.NodeID, line)
	}
}

// lineWriter turns the byte chunks delivered by os/exec into the line-based
// callback contract exposed by Process.OnLine. Each command stream owns one
// writer, and Wait flushes any final unterminated line before signalling exit.
type lineWriter struct {
	mu      sync.Mutex
	pending []byte
	emit    func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending = append(w.pending, p...)
	for {
		newline := bytes.IndexByte(w.pending, '\n')
		if newline < 0 {
			break
		}
		line := bytes.TrimSuffix(w.pending[:newline], []byte{'\r'})
		w.emit(string(line))
		w.pending = w.pending[newline+1:]
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return
	}
	w.emit(string(bytes.TrimSuffix(w.pending, []byte{'\r'})))
	w.pending = nil
}

// ---------------------------------------------------------------------------
// Supervisor
// ---------------------------------------------------------------------------

// SupervisionPolicy defines how a process is restarted on failure.
type SupervisionPolicy struct {
	MaxRestarts  int           // 0 = unlimited
	InitialDelay time.Duration // delay before first restart
	MaxDelay     time.Duration // cap on exponential backoff
	ResetAfter   time.Duration // reset restart count if stable for this long
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
	process  *Process
	policy   SupervisionPolicy
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

// LogSink receives every line of output from every process this Manager
// spawns, in addition to the standard output.ServiceLog routing. The dev
// engine uses it to persist per-node log files under .acthur/logs/<node>.log
// so `acthur service logs <name>` has something real to read even after the
// line has scrolled off the terminal.
type LogSink func(nodeID, line string)

// Manager tracks all managed processes in the system.
// It is the single place the dev engine creates and monitors processes.
type Manager struct {
	processes   map[string]*Process
	supervisors map[string]*Supervisor
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	logSink     LogSink
	ownerID     string
}

// NewManager creates a process manager.
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	ownerBytes := make([]byte, 16)
	_, _ = rand.Read(ownerBytes)
	return &Manager{
		processes:   make(map[string]*Process),
		supervisors: make(map[string]*Supervisor),
		ctx:         ctx,
		cancel:      cancel,
		ownerID:     hex.EncodeToString(ownerBytes),
	}
}

// SetLogSink registers sink to additionally receive every output line from
// every process this Manager spawns from now on. Nil disables it.
func (m *Manager) SetLogSink(sink LogSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logSink = sink
}

// Spawn creates and starts a new supervised process for a graph node.
func (m *Manager) Spawn(nodeID, bin string, args []string, env map[string]string, dir string) (*Process, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processes[nodeID]; exists {
		return nil, fmt.Errorf("process for node %q is already running", nodeID)
	}

	p := newProcess(nodeID, bin, args, env, dir, m.ownerID)

	// Default line handler: route to output package, and to the log sink
	// (if configured) for durable per-node log files.
	sink := m.logSink
	p.OnLine(func(id, line string) {
		output.ServiceLog(id, line)
		if sink != nil {
			sink(id, line)
		}
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
	owned := snapshotManagerOwned(m.ownerID)
	terminateOwned(owned)
	killOwned(owned)
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
