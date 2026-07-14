//go:build windows

package process

import "os/exec"

const processOwnerEnv = "ACTHUR_PROCESS_OWNER"
const managerOwnerEnv = "ACTHUR_MANAGER_OWNER"

// Process groups are a Unix concept; on Windows we operate on the direct
// child only.
func setProcessGroup(cmd *exec.Cmd) {}

func terminateTree(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func killTree(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func snapshotDescendants(rootPID int) []ownedProcess     { return nil }
func snapshotOwned(ownerID string) []ownedProcess        { return nil }
func snapshotManagerOwned(ownerID string) []ownedProcess { return nil }
func terminateOwned(processes []ownedProcess)            {}
func killOwned(processes []ownedProcess)                 {}
