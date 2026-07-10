//go:build !windows

package process_test

import "syscall"

// pidAlive reports whether pid is still running, using the POSIX kill(2)
// signal-0 probe (no signal is actually sent — only existence/permission is
// checked). Used by process-tree tests to verify a process was really
// terminated, not just that its direct parent exited.
func pidAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// forceKill sends SIGKILL to pid. Used by tests either to force-terminate a
// process that survived a graceful Stop (failure-path cleanup) or to
// simulate an external hard crash of the supervised process.
func forceKill(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
