//go:build windows

package process

import "os/exec"

// Process groups are a Unix concept; on Windows we operate on the direct
// child only.
func setProcessGroup(cmd *exec.Cmd) {}

func terminateTree(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func killTree(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
