// Package generate is the write engine behind acthur's generated code: it
// takes a set of plugin.GeneratedFile values (produced by acthur add's
// plugin generators, and — from Phase 7 slice 2 on — the contract→code
// pipeline) and persists them to disk idempotently, tracking what it wrote
// in a generated.lock file at the project root so repeat runs can tell
// "safe to regenerate" apart from "the user has since edited this by hand".
package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/acthurhq/acthur/internal/plugin"
	"gopkg.in/yaml.v3"
)

// Status classifies what happened to one GeneratedFile on disk.
type Status string

const (
	StatusWritten Status = "written"
	StatusSkipped Status = "skipped"
	StatusMerged  Status = "merged"
)

// Result reports the outcome of writing one plugin.GeneratedFile.
type Result struct {
	NodeID string
	Path   string // the GeneratedFile.Path as given (unrouted)
	Status Status
	// Warning is set when Status is StatusSkipped because the file has
	// diverged from what acthur last generated — the user owns it now.
	Warning string
}

// lockFileName is the generated.lock file's name at the project root.
const lockFileName = "generated.lock"

// lock is the parsed form of generated.lock: repo-relative path -> sha256
// hex digest of the content acthur last wrote for that path.
type lock map[string]string

// WriteFiles persists files to disk under root, following the same routing
// convention `acthur add` established: a path starting with "migrations/"
// or "deploy/" lands at the project root (deploy artifacts span the whole
// graph, not one node), everything else under <root>/<nodeID>/.
//
// It consults and updates generated.lock at the project root to decide,
// per file, whether writing is safe:
//   - absent on disk                                -> write, record hash
//   - present, disk hash == lock hash                -> overwrite (user never touched it), update hash
//   - present, disk hash != lock hash, no MergeMarker -> skip with a warning (user owns it now)
//   - present, disk hash != lock hash, MergeMarker    -> merge below the marker, record the merged hash
//   - present, no entry in the lock (legacy files)    -> respect f.Overwrite
func WriteFiles(root, nodeID string, files []plugin.GeneratedFile) ([]Result, error) {
	l, err := loadLock(root)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", lockFileName, err)
	}

	results := make([]Result, 0, len(files))
	dirty := false

	for _, f := range files {
		key := lockKey(nodeID, f.Path)
		target := filepath.Join(root, key)

		res := Result{NodeID: nodeID, Path: f.Path}

		existing, statErr := os.ReadFile(target)
		exists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}

		lockHash, inLock := l[key]

		switch {
		case !exists:
			if err := writeFile(target, f); err != nil {
				return nil, err
			}
			l[key] = sha256Hex(f.Content)
			res.Status = StatusWritten
			dirty = true

		case !inLock:
			// Legacy path: present on disk but generated.lock never saw
			// it (e.g. written before this engine existed, or by a tool
			// that bypasses it). Fall back to the file's own Overwrite
			// flag, as acthur add always has.
			if f.Overwrite {
				if err := writeFile(target, f); err != nil {
					return nil, err
				}
				l[key] = sha256Hex(f.Content)
				res.Status = StatusWritten
				dirty = true
			} else {
				res.Status = StatusSkipped
			}

		case sha256Hex(existing) == lockHash:
			// Unchanged since we last generated it: safe to regenerate in
			// full, even if it has a MergeMarker (nothing has diverged).
			if err := writeFile(target, f); err != nil {
				return nil, err
			}
			l[key] = sha256Hex(f.Content)
			res.Status = StatusWritten
			dirty = true

		case f.MergeMarker != "":
			merged, err := mergeContent(existing, f)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(target, merged, modeOf(f)); err != nil {
				return nil, err
			}
			l[key] = sha256Hex(merged)
			res.Status = StatusMerged
			dirty = true

		default:
			res.Status = StatusSkipped
			res.Warning = fmt.Sprintf(
				"%s has been edited since it was last generated — skipping (regenerate by hand or delete it to accept the new version)",
				key,
			)
		}

		results = append(results, res)
	}

	if dirty {
		if err := saveLock(root, l); err != nil {
			return nil, fmt.Errorf("saving %s: %w", lockFileName, err)
		}
	}

	return results, nil
}

// lockKey resolves a GeneratedFile.Path to the repo-relative path it is
// tracked under in generated.lock and written to on disk: paths under
// "migrations/", "deploy/", or ".acthur/" (project-scoped state — e.g. the
// https plugin's generated dev-cert README) land at the project root,
// everything else under nodeID/.
func lockKey(nodeID, relPath string) string {
	if strings.HasPrefix(relPath, "migrations/") || strings.HasPrefix(relPath, "deploy/") || strings.HasPrefix(relPath, ".acthur/") {
		return relPath
	}
	return filepath.Join(nodeID, relPath)
}

func modeOf(f plugin.GeneratedFile) os.FileMode {
	if f.Mode != 0 {
		return os.FileMode(f.Mode)
	}
	return 0o644
}

func writeFile(target string, f plugin.GeneratedFile) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, f.Content, modeOf(f))
}

// mergeContent inserts f.Content immediately after the first occurrence of
// f.MergeMarker in existing. If the marker is not found, f.Content is
// appended at the end — a safe fallback that never silently drops
// generated content.
func mergeContent(existing []byte, f plugin.GeneratedFile) ([]byte, error) {
	text := string(existing)
	idx := strings.Index(text, f.MergeMarker)
	if idx == -1 {
		merged := text
		if !strings.HasSuffix(merged, "\n") {
			merged += "\n"
		}
		merged += string(f.Content)
		return []byte(merged), nil
	}
	insertAt := idx + len(f.MergeMarker)
	merged := text[:insertAt] + "\n" + string(f.Content) + text[insertAt:]
	return []byte(merged), nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// loadLock reads generated.lock at root, returning an empty lock (not an
// error) when the file does not exist yet.
func loadLock(root string) (lock, error) {
	path := filepath.Join(root, lockFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return lock{}, nil
		}
		return nil, err
	}
	l := lock{}
	if err := yaml.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if l == nil {
		l = lock{}
	}
	return l, nil
}

// saveLock writes generated.lock atomically: marshal to a temp file in the
// same directory, then rename over the real path, so a crash mid-write
// never leaves a truncated or corrupt lock file behind.
func saveLock(root string, l lock) error {
	path := filepath.Join(root, lockFileName)
	data, err := yaml.Marshal(map[string]string(l))
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(root, ".generated.lock.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()  // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
