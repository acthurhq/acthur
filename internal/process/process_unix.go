//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

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
