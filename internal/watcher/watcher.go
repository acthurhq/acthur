// Package watcher implements a file system watcher that triggers
// graph-aware hot reload cascades when source files change.
// When a file changes in a service node's directory, Acthur:
//  1. Signals that service's process (the adapter's hot reload handles the rebuild)
//  2. Traverses PropagateFrom() to find downstream nodes (callers)
//  3. Notifies downstream nodes that their dependency changed
//  4. If a contract file changes, regenerates the client for all callers
package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/output"
)

// ---------------------------------------------------------------------------
// Event
// ---------------------------------------------------------------------------

// ChangeType classifies a file system change.
type ChangeType string

const (
	ChangeModified ChangeType = "modified"
	ChangeCreated  ChangeType = "created"
	ChangeDeleted  ChangeType = "deleted"
)

// Event describes a file system change.
type Event struct {
	Path       string
	ChangeType ChangeType
	NodeID     string // which graph node owns this file
	Timestamp  time.Time
}

// ---------------------------------------------------------------------------
// Watcher
// ---------------------------------------------------------------------------

// Handler is called when a watched file changes.
type Handler func(event Event)

// Watcher watches directories mapped to graph nodes and fires handlers
// when files change. It uses polling (works on all platforms including
// network filesystems and Docker volumes) rather than inotify.
//
// For performance, it only hashes files that have changed mtime —
// no unnecessary reads on unchanged files.
type Watcher struct {
	g         *graph.Graph
	rootDir   string
	interval  time.Duration
	handlers  []Handler
	mu        sync.RWMutex
	done      chan struct{}
	snapshots map[string]fileSnapshot // path → snapshot
}

// fileSnapshot holds the last-seen state of a file.
type fileSnapshot struct {
	ModTime time.Time
	Size    int64
}

// New creates a watcher for the given graph rooted at rootDir.
// interval controls how often the filesystem is polled.
func New(g *graph.Graph, rootDir string, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = 300 * time.Millisecond
	}
	return &Watcher{
		g:         g,
		rootDir:   rootDir,
		interval:  interval,
		done:      make(chan struct{}),
		snapshots: make(map[string]fileSnapshot),
	}
}

// OnChange registers a handler called on every file change event.
func (w *Watcher) OnChange(h Handler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, h)
}

// Start begins polling in the background. Returns immediately.
func (w *Watcher) Start() {
	// Take initial snapshot
	w.snapshot()
	go w.poll()
	output.Debug("watcher", "watching %s (interval: %s)", w.rootDir, w.interval)
}

// Stop halts the polling loop.
func (w *Watcher) Stop() {
	close(w.done)
}

// ---------------------------------------------------------------------------
// Polling loop
// ---------------------------------------------------------------------------

func (w *Watcher) poll() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.check()
		}
	}
}

func (w *Watcher) check() {
	current := make(map[string]fileSnapshot)

	// Walk all service node directories
	for _, node := range w.g.NodesByType("service") {
		dir := w.nodeDir(node)
		if dir == "" {
			continue
		}
		w.walkDir(dir, node.ID, current)
	}

	// Also watch contract files
	contractsDir := filepath.Join(w.rootDir, "contracts")
	if _, err := os.Stat(contractsDir); err == nil {
		w.walkDir(contractsDir, "__contracts__", current)
	}

	w.mu.Lock()
	old := w.snapshots
	w.snapshots = current
	w.mu.Unlock()

	// Detect changes
	for path, snap := range current {
		oldSnap, exists := old[path]
		if !exists {
			nodeID := w.nodeForPath(path)
			w.fire(path, nodeID, ChangeCreated)
			continue
		}
		if snap.ModTime != oldSnap.ModTime || snap.Size != oldSnap.Size {
			nodeID := w.nodeForPath(path)
			w.fire(path, nodeID, ChangeModified)
		}
	}

	// Detect deletions
	for path := range old {
		if _, exists := current[path]; !exists {
			nodeID := w.nodeForPath(path)
			w.fire(path, nodeID, ChangeDeleted)
		}
	}
}

func (w *Watcher) walkDir(dir, nodeID string, into map[string]fileSnapshot) {
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if w.shouldIgnore(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		into[path] = fileSnapshot{ModTime: info.ModTime(), Size: info.Size()}
		return nil
	})
}

func (w *Watcher) snapshot() {
	current := make(map[string]fileSnapshot)
	for _, node := range w.g.NodesByType("service") {
		dir := w.nodeDir(node)
		if dir != "" {
			w.walkDir(dir, node.ID, current)
		}
	}
	contractsDir := filepath.Join(w.rootDir, "contracts")
	if _, err := os.Stat(contractsDir); err == nil {
		w.walkDir(contractsDir, "__contracts__", current)
	}
	w.mu.Lock()
	w.snapshots = current
	w.mu.Unlock()
}

func (w *Watcher) fire(path, nodeID string, ct ChangeType) {
	event := Event{
		Path:       path,
		ChangeType: ct,
		NodeID:     nodeID,
		Timestamp:  time.Now(),
	}

	output.Debug("watcher", "%s: %s (%s)", ct, filepath.Base(path), nodeID)

	w.mu.RLock()
	handlers := w.handlers
	w.mu.RUnlock()

	for _, h := range handlers {
		go h(event)
	}
}

// ---------------------------------------------------------------------------
// Cascade
// ---------------------------------------------------------------------------

// CascadeHandler returns a Handler that propagates change events through
// the graph to downstream nodes. Used by the dev engine to notify frontends
// when their API backend changes a contract.
func CascadeHandler(g *graph.Graph, onCascade func(nodeID string)) Handler {
	return func(event Event) {
		if event.NodeID == "" || event.NodeID == "__unknown__" {
			return
		}

		// Contract change — find all nodes that consume from this contract's service
		if event.NodeID == "__contracts__" {
			contractName := contractNameFromPath(event.Path)
			if contractName == "" {
				return
			}
			// Find which service satisfies this contract and cascade from it
			for _, node := range g.Nodes() {
				downstream := g.PropagateFrom(node.ID)
				for _, d := range downstream {
					output.Debug("watcher", "contract change cascade: %s → %s", node.ID, d.ID)
					if onCascade != nil {
						onCascade(d.ID)
					}
				}
			}
			return
		}

		// Service file change — cascade to downstream nodes
		downstream := g.PropagateFrom(event.NodeID)
		for _, d := range downstream {
			output.Debug("watcher", "file change cascade: %s → %s", event.NodeID, d.ID)
			if onCascade != nil {
				onCascade(d.ID)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// shouldIgnore returns true for files that should not trigger reloads.
func (w *Watcher) shouldIgnore(path string) bool {
	base := filepath.Base(path)

	// Ignore dot files and common non-source artifacts.
	// Apply patterns only against the path relative to rootDir so that
	// ancestor directories (e.g. the OS /tmp prefix) are not matched.
	ignorePatterns := []string{
		".git", ".acthur", "node_modules", "vendor", ".venv",
		"__pycache__", "target", "tmp", "bin", "dist", ".next",
		".astro", "build", ".DS_Store",
	}
	rel, err := filepath.Rel(w.rootDir, path)
	if err != nil {
		rel = path // fallback to absolute path if Rel fails
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		for _, pattern := range ignorePatterns {
			if part == pattern {
				return true
			}
		}
	}

	// Ignore generated files (Acthur marks them in a lock file)
	if strings.HasSuffix(base, ".generated.go") ||
		strings.HasSuffix(base, ".pb.go") ||
		base == "generated.go" {
		return true
	}

	// Only watch source file extensions
	ext := strings.ToLower(filepath.Ext(base))
	watchedExts := map[string]bool{
		".go":    true,
		".rs":    true,
		".ts":    true,
		".tsx":   true,
		".js":    true,
		".jsx":   true,
		".py":    true,
		".php":   true,
		".astro": true,
		".vue":   true,
		".svelte": true,
		".html":  true,
		".css":   true,
		".yml":   true,
		".yaml":  true,
		".toml":  true,
		".env":   true,
	}
	return !watchedExts[ext]
}

// nodeDir returns the directory for a graph node.
func (w *Watcher) nodeDir(node *graph.Node) string {
	candidates := []string{
		filepath.Join(w.rootDir, "services", node.ID),
		filepath.Join(w.rootDir, node.ID),
	}
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	// Single-service project: root is the node's directory
	if len(w.g.NodesByType("service")) == 1 {
		return w.rootDir
	}
	return ""
}

// nodeForPath finds which graph node owns a given file path.
func (w *Watcher) nodeForPath(path string) string {
	for _, node := range w.g.NodesByType("service") {
		dir := w.nodeDir(node)
		if dir != "" && strings.HasPrefix(path, dir) {
			return node.ID
		}
	}
	if strings.Contains(path, "contracts") {
		return "__contracts__"
	}
	return "__unknown__"
}

// contractNameFromPath extracts the contract name from a contract file path.
func contractNameFromPath(path string) string {
	base := filepath.Base(path)
	// e.g. "users.contract.yml" → "users"
	parts := strings.Split(base, ".")
	if len(parts) >= 2 {
		return parts[0]
	}
	return ""
}
