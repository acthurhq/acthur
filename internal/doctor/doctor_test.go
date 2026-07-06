package doctor

import (
	"os/exec"
	"testing"
)

// TestHasBlockingFailures_RequiredMissingBlocks asserts a required check that
// is missing (auto-fixable or not) is treated as a blocking failure — the
// dev preflight must abort rather than start processes against a broken
// environment.
func TestHasBlockingFailures_RequiredMissingBlocks(t *testing.T) {
	r := &Result{Checks: []*Check{
		{Name: "git", Required: true, Status: StatusOK},
		{Name: "air", Required: true, Status: StatusMissing},
	}}
	if !HasBlockingFailures(r) {
		t.Fatal("expected a required missing check to block")
	}
}

// TestHasBlockingFailures_RequiredFailedBlocks covers the non-autofixable
// failure case (docker daemon not running, etc.).
func TestHasBlockingFailures_RequiredFailedBlocks(t *testing.T) {
	r := &Result{Checks: []*Check{
		{Name: "docker", Required: true, Status: StatusFailed},
	}}
	if !HasBlockingFailures(r) {
		t.Fatal("expected a required failed check to block")
	}
}

// TestHasBlockingFailures_OptionalMissingDoesNotBlock: an optional check
// (not needed for this stack) must never abort dev.
func TestHasBlockingFailures_OptionalMissingDoesNotBlock(t *testing.T) {
	r := &Result{Checks: []*Check{
		{Name: "bun", Required: false, Status: StatusMissing},
	}}
	if HasBlockingFailures(r) {
		t.Fatal("expected optional missing check not to block")
	}
}

// TestHasBlockingFailures_WarningDoesNotBlock: a required check that is
// merely a warning (e.g. a port already in use) is a runtime condition, not
// missing tooling — it must not abort the dev preflight.
func TestHasBlockingFailures_WarningDoesNotBlock(t *testing.T) {
	r := &Result{Checks: []*Check{
		{Name: "port 8080", Required: true, Status: StatusWarning},
	}}
	if HasBlockingFailures(r) {
		t.Fatal("expected warning-status check not to block")
	}
}

// TestHasBlockingFailures_AllOKDoesNotBlock is the healthy-environment case.
func TestHasBlockingFailures_AllOKDoesNotBlock(t *testing.T) {
	r := &Result{Checks: []*Check{
		{Name: "git", Required: true, Status: StatusOK},
		{Name: "docker", Required: true, Status: StatusOK},
	}}
	if HasBlockingFailures(r) {
		t.Fatal("expected all-OK result not to block")
	}
}

// TestCmdVersion_StderrOnlyTool: some tools (air) print their version banner
// to stderr with an empty stdout — cmdVersion must still see it, or doctor
// reports an installed tool as missing.
func TestCmdVersion_StderrOnlyTool(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	got := cmdVersion("sh", "-c", "echo 'v1.2.3' >&2")
	if got != "v1.2.3" {
		t.Fatalf("expected stderr version output captured, got %q", got)
	}
}
