//go:build !windows

package process

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const processOwnerEnv = "ACTHUR_PROCESS_OWNER"
const managerOwnerEnv = "ACTHUR_MANAGER_OWNER"

// setProcessGroup places the child in its own process group so signals can
// reach the entire tree it spawns (air's compiled binary, shell wrappers).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateTree delivers SIGTERM to the child's whole process group,
// falling back to signalling just the direct child.
func terminateTree(cmd *exec.Cmd) error {
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	return nil
}

// killTree delivers SIGKILL to the child's whole process group,
// falling back to killing just the direct child.
func killTree(cmd *exec.Cmd) error {
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

// snapshotDescendants discovers the complete current descendant tree through
// Linux procfs. On Unix systems without procfs it safely returns no entries;
// process-group cleanup remains the fallback there.
func snapshotDescendants(rootPID int) []ownedProcess {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	type procInfo struct {
		ppid     int
		identity string
	}
	all := make(map[int]procInfo)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		ppid, identity, ok := procIdentity(pid)
		if ok {
			all[pid] = procInfo{ppid: ppid, identity: identity}
		}
	}
	owned := map[int]bool{rootPID: true}
	var out []ownedProcess
	for changed := true; changed; {
		changed = false
		for pid, info := range all {
			if !owned[pid] && owned[info.ppid] {
				owned[pid] = true
				out = append(out, ownedProcess{pid: pid, identity: info.identity})
				changed = true
			}
		}
	}
	return out
}

// snapshotOwned finds workload processes through the opaque marker inherited
// from their managed supervisor. This remains stable after reparenting and
// process-group/session changes.
func snapshotOwned(ownerID string) []ownedProcess {
	return snapshotByEnv(processOwnerEnv, ownerID)
}

func snapshotManagerOwned(ownerID string) []ownedProcess {
	return snapshotByEnv(managerOwnerEnv, ownerID)
}

func snapshotByEnv(key, ownerID string) []ownedProcess {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	want := []byte(key + "=" + ownerID)
	var out []ownedProcess
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		environ, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "environ"))
		if err != nil {
			continue
		}
		matched := false
		for _, item := range bytes.Split(environ, []byte{0}) {
			if bytes.Equal(item, want) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		_, identity, ok := procIdentity(pid)
		if ok {
			out = append(out, ownedProcess{pid: pid, identity: identity})
		}
	}
	return out
}

func procIdentity(pid int) (ppid int, identity string, ok bool) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, "", false
	}
	// comm may contain spaces or parentheses; fields following the final ')'
	// begin with state (field 3), ppid (4), ... starttime (22).
	end := strings.LastIndexByte(string(b), ')')
	if end < 0 {
		return 0, "", false
	}
	fields := strings.Fields(string(b[end+1:]))
	if len(fields) <= 19 {
		return 0, "", false
	}
	ppid, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, "", false
	}
	return ppid, fields[19], true
}

func ownedStillMatches(p ownedProcess) bool {
	_, identity, ok := procIdentity(p.pid)
	return ok && identity == p.identity
}

func terminateOwned(processes []ownedProcess) {
	for _, p := range processes {
		if ownedStillMatches(p) {
			_ = syscall.Kill(p.pid, syscall.SIGTERM)
		}
	}
}

func killOwned(processes []ownedProcess) {
	// Kill children before parents as best effort; snapshot order is ancestral.
	for i := len(processes) - 1; i >= 0; i-- {
		if ownedStillMatches(processes[i]) {
			_ = syscall.Kill(processes[i].pid, syscall.SIGKILL)
		}
	}
}
