//go:build windows

package process_test

// pidAlive and forceKill back the process-tree tests (they spawn `sh -c`
// scripts and are skipped at runtime on Windows via t.Skip) — this file
// exists only so the package still compiles on Windows: the syscall
// package there has no Kill/SIGKILL, which are POSIX-only symbols.
func pidAlive(pid int) bool { return false }
func forceKill(pid int)     {}
