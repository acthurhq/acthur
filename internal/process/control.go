package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// .acthur/ on-disk control channel
//
// `acthur dev` and `acthur service <cmd>` are separate OS processes — there
// is no shared memory between them. Rather than invent a daemon RPC, the
// dev engine persists the smallest honest facts a separate CLI invocation
// needs under the project's .acthur/ directory:
//
//   - .acthur/logs/<node>.log — every line the node's process has ever
//     printed, so `acthur service logs` has something real to read even
//     after the line has scrolled off the dev engine's own terminal.
//   - .acthur/run/<node>.pid — the OS PID of the node's currently running
//     process, so `acthur service restart` can signal it directly. Killing
//     it looks like an ordinary crash to the running dev engine's
//     Supervisor, which already auto-restarts on unexpected exit — no new
//     recovery path needed.
// ---------------------------------------------------------------------------

// LogPath returns the path to nodeID's durable log file under rootDir.
func LogPath(rootDir, nodeID string) string {
	return filepath.Join(rootDir, ".acthur", "logs", nodeID+".log")
}

// PIDPath returns the path to nodeID's pidfile under rootDir.
func PIDPath(rootDir, nodeID string) string {
	return filepath.Join(rootDir, ".acthur", "run", nodeID+".pid")
}

// FileLogSink returns a LogSink that appends every line to
// .acthur/logs/<node>.log under rootDir, creating the directory if needed.
// It opens (and keeps open) one file per node the first time a line arrives
// for it.
func FileLogSink(rootDir string) (LogSink, error) {
	dir := filepath.Join(rootDir, ".acthur", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}

	files := make(map[string]*os.File)
	return func(nodeID, line string) {
		f, ok := files[nodeID]
		if !ok {
			var err error
			f, err = os.OpenFile(LogPath(rootDir, nodeID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				return // best-effort — never let a log write failure take down the node
			}
			files[nodeID] = f
		}
		_, _ = fmt.Fprintln(f, line)
	}, nil
}

// WritePIDFile records pid as the current process for nodeID under rootDir,
// creating .acthur/run if needed.
func WritePIDFile(rootDir, nodeID string, pid int) error {
	path := PIDPath(rootDir, nodeID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

// ReadPIDFile returns the PID recorded for nodeID under rootDir.
func ReadPIDFile(rootDir, nodeID string) (int, error) {
	data, err := os.ReadFile(PIDPath(rootDir, nodeID))
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("corrupt pidfile for %q: %w", nodeID, err)
	}
	return pid, nil
}

// RemovePIDFile deletes nodeID's pidfile under rootDir, if present.
func RemovePIDFile(rootDir, nodeID string) error {
	err := os.Remove(PIDPath(rootDir, nodeID))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
