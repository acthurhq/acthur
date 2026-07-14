package contract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/contract"
)

func TestContractBaseline_BreakingChangeIsNamed(t *testing.T) {
	root := t.TempDir()
	writeBaselineContract(t, root, `contract: users
version: "1"
transport: http
endpoints:
  - id: get_user
    method: GET
    path: /users/:id
    output:
      id: string
`)
	if err := contract.UpdateDeployBaseline(root); err != nil {
		t.Fatalf("define baseline: %v", err)
	}
	writeBaselineContract(t, root, `contract: users
version: "1"
transport: http
endpoints:
  - id: get_user
    method: GET
    path: /users/:id
    output: {}
`)

	err := contract.CheckDeployBaseline(root)
	if err == nil {
		t.Fatal("expected breaking contract change to fail baseline check")
	}
	for _, want := range []string{"users", "endpoints.get_user.output.id", `output field "id" removed`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected pointed baseline error containing %q, got: %v", want, err)
		}
	}
}

func TestContractBaseline_AbsentBaselinePasses(t *testing.T) {
	if err := contract.CheckDeployBaseline(t.TempDir()); err != nil {
		t.Fatalf("projects without an explicit baseline must pass: %v", err)
	}
}

func writeBaselineContract(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "contracts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "users.contract.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
