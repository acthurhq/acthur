package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/output"
)

// ---------------------------------------------------------------------------
// acthur contract validate|list|show|diff — Phase 4 Slice 1 (issue #36)
// ---------------------------------------------------------------------------

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	output.SetOutput(&buf, &buf)
	defer output.SetOutput(os.Stdout, os.Stderr)
	fn()
	return buf.String()
}

func TestRunContractValidate_ValidProjectFixture_ExitsZero(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "vetangle")

	var code int
	out := captureOutput(t, func() {
		code = runContractValidate(root)
	})

	if code != 0 {
		t.Fatalf("expected exit code 0 for valid contracts, got %d; output: %s", code, out)
	}
	for _, name := range []string{"users", "appointments", "admin"} {
		if !containsSubstr(out, name) {
			t.Errorf("expected output to mention contract %q, got: %s", name, out)
		}
	}
}

func TestRunContractValidate_BrokenContractFile_NonZeroExitNamesFile(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "broken-contract-project")

	var code int
	out := captureOutput(t, func() {
		code = runContractValidate(root)
	})

	if code == 0 {
		t.Fatalf("expected non-zero exit code for a broken contract file, got 0; output: %s", out)
	}
	if !containsSubstr(out, "broken.contract.yml") {
		t.Errorf("expected output to name the broken file, got: %s", out)
	}
}

func TestRunContractValidate_NoContractsDir_ExitsZero(t *testing.T) {
	root := t.TempDir() // no contracts/ subdir

	var code int
	captureOutput(t, func() {
		code = runContractValidate(root)
	})

	if code != 0 {
		t.Fatalf("expected exit code 0 when there is no contracts/ dir, got %d", code)
	}
}

func TestRunContractList_ListsNameVersionTransportAndEndpointCount(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "vetangle")

	var code int
	out := captureOutput(t, func() {
		code = runContractList(root)
	})

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", code, out)
	}
	if !containsSubstr(out, "users") || !containsSubstr(out, "http") {
		t.Errorf("expected output to list name and transport, got: %s", out)
	}
}

func TestRunContractShow_KnownContract_PrintsEndpointDetail(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "vetangle")

	var code int
	out := captureOutput(t, func() {
		code = runContractShow(root, "users")
	})

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", code, out)
	}
	if !containsSubstr(out, "create_user") || !containsSubstr(out, "POST") {
		t.Errorf("expected output to include endpoint id and method, got: %s", out)
	}
}

func TestRunContractShow_UnknownContract_NonZeroExit(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "vetangle")

	var code int
	captureOutput(t, func() {
		code = runContractShow(root, "does-not-exist")
	})

	if code == 0 {
		t.Fatal("expected non-zero exit code for an unknown contract name")
	}
}

func TestRunContractDiff_BreakingPair_NonZeroExit(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "testdata", "contract-diff")

	var code int
	out := captureOutput(t, func() {
		code = runContractDiff(filepath.Join(dir, "breaking-old.contract.yml"), filepath.Join(dir, "breaking-new.contract.yml"))
	})

	if code == 0 {
		t.Fatalf("expected non-zero exit code for a breaking diff, got 0; output: %s", out)
	}
	if !containsSubstr(out, "breaking") {
		t.Errorf("expected output to mention breaking changes, got: %s", out)
	}
}

func TestRunContractDiff_NonBreakingPair_ExitsZero(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "testdata", "contract-diff")

	var code int
	out := captureOutput(t, func() {
		code = runContractDiff(filepath.Join(dir, "nonbreaking-old.contract.yml"), filepath.Join(dir, "nonbreaking-new.contract.yml"))
	})

	if code != 0 {
		t.Fatalf("expected exit code 0 for a non-breaking diff, got %d; output: %s", code, out)
	}
}

func TestRunContractBaselineUpdate_DefinesDeployCompatibilityLock(t *testing.T) {
	root := t.TempDir()
	contractsDir := filepath.Join(root, "contracts")
	if err := os.MkdirAll(contractsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `contract: users
version: "1"
transport: http
endpoints:
  - id: list_users
    method: GET
    path: /users
`
	if err := os.WriteFile(filepath.Join(contractsDir, "users.contract.yml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := runContractBaselineUpdate(root); code != 0 {
		t.Fatalf("baseline update exit = %d, want 0", code)
	}
	lock, err := os.ReadFile(filepath.Join(root, ".acthur", "contracts.lock.yml"))
	if err != nil {
		t.Fatalf("expected explicit deploy baseline: %v", err)
	}
	if !strings.Contains(string(lock), "contract: users") {
		t.Fatalf("baseline does not contain current contract: %s", lock)
	}
}

func containsSubstr(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && bytes.Contains([]byte(s), []byte(substr)))
}
