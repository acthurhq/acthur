package main

import (
	"bytes"
	"os"
	"path/filepath"
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

func containsSubstr(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && bytes.Contains([]byte(s), []byte(substr)))
}
