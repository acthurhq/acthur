package contract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acthur/acthur/internal/contract"
)

// ---------------------------------------------------------------------------
// LoadDir tests
// ---------------------------------------------------------------------------

func TestLoadDir_LoadsAllContractsUnderContractsDir(t *testing.T) {
	root := t.TempDir()
	writeProjectContract(t, root, "users", `
contract: users
version: "1"
transport: http
endpoints:
  - id: get_user
    method: GET
    path: /api/v1/users/:id
`)
	writeProjectContract(t, root, "appointments", `
contract: appointments
version: "1"
transport: http
endpoints:
  - id: list_appointments
    method: GET
    path: /api/v1/appointments
`)

	reg, err := contract.LoadDir(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	all := reg.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 contracts loaded, got %d: %v", len(all), all)
	}

	if _, err := reg.Get("users", "1"); err != nil {
		t.Errorf("expected users contract registered: %v", err)
	}
	if _, err := reg.Get("appointments", "1"); err != nil {
		t.Errorf("expected appointments contract registered: %v", err)
	}
}

func TestLoadDir_MissingContractsDir_ReturnsEmptyRegistryNoError(t *testing.T) {
	root := t.TempDir() // no contracts/ subdir created

	reg, err := contract.LoadDir(root)
	if err != nil {
		t.Fatalf("expected no error for missing contracts dir, got: %v", err)
	}
	if reg == nil {
		t.Fatal("expected a non-nil registry")
	}
	if len(reg.All()) != 0 {
		t.Errorf("expected empty registry, got %d contracts", len(reg.All()))
	}
}

func TestLoadDir_BrokenContractFile_ErrorNamesTheFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "contracts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	badPath := filepath.Join(dir, "broken.contract.yml")
	// Missing required 'contract' name field — ParseFile rejects this.
	if err := os.WriteFile(badPath, []byte("version: \"1\"\ntransport: http\nendpoints: []\n"), 0o644); err != nil {
		t.Fatalf("write broken contract: %v", err)
	}

	_, err := contract.LoadDir(root)
	if err == nil {
		t.Fatal("expected an error for a broken contract file")
	}
	if !containsStr(err.Error(), "broken.contract.yml") {
		t.Errorf("expected error to name the broken file, got: %v", err)
	}
}

func TestLoadDir_IgnoresNonContractFiles(t *testing.T) {
	root := t.TempDir()
	writeProjectContract(t, root, "users", `
contract: users
version: "1"
transport: http
endpoints: []
`)
	dir := filepath.Join(root, "contracts")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("not a contract"), 0o644); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	reg, err := contract.LoadDir(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reg.All()) != 1 {
		t.Errorf("expected only the .contract.yml file to be loaded, got %d contracts", len(reg.All()))
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeProjectContract(t *testing.T, root, name, content string) string {
	t.Helper()
	dir := filepath.Join(root, "contracts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir contracts dir: %v", err)
	}
	path := filepath.Join(dir, name+".contract.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write contract %s: %v", name, err)
	}
	return path
}
