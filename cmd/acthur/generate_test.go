package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/generate"
)

// copyUsersContract copies the vetangle users contract into dir/contracts/.
func copyUsersContract(t *testing.T, dir string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata", "vetangle", "contracts", "users.contract.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "contracts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "contracts", "users.contract.yml"), src, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunGenerateFromContract_WritesFullFileSet: `acthur generate
// from-contract users.contract.yml` writes the pipeline's output through the
// write engine — code under the node, migrations at project root, lock file
// recorded.
func TestRunGenerateFromContract_WritesFullFileSet(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	copyUsersContract(t, dir)
	writeTestProject(t, dir)

	results, err := runGenerateFromContract(dir, "users", "")
	if err != nil {
		t.Fatalf("runGenerateFromContract: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}

	for _, p := range []string{
		filepath.Join(dir, "api", "internal", "users", "handler.go"),
		filepath.Join(dir, "api", "internal", "users", "handler_test.go"),
		filepath.Join(dir, "migrations", "0400_users.up.sql"),
		filepath.Join(dir, "generated.lock"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}
}

// TestRunGenerateFromContract_SecondRunKeepsMigrationNumber: regenerating the
// same contract must reuse its migration number, not burn a new one.
func TestRunGenerateFromContract_SecondRunKeepsMigrationNumber(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	copyUsersContract(t, dir)
	writeTestProject(t, dir)

	if _, err := runGenerateFromContract(dir, "users", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := runGenerateFromContract(dir, "users", ""); err != nil {
		t.Fatal(err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "migrations", "*_users.up.sql"))
	if len(matches) != 1 {
		t.Fatalf("expected exactly one users up migration, got %v", matches)
	}
	if !strings.HasSuffix(matches[0], "0400_users.up.sql") {
		t.Errorf("expected number 0400 to be reused, got %s", matches[0])
	}
}

// TestRunGenerateFromContract_AvoidsNumberCollision: a second contract must
// take the next free number in the 0400 range, since golang-migrate rejects
// duplicate versions.
func TestRunGenerateFromContract_AvoidsNumberCollision(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	copyUsersContract(t, dir)
	writeTestProject(t, dir)

	// A second minimal contract.
	pets := `contract: pets
version: "1"
transport: http
endpoints:
  - id: get_pet
    method: GET
    path: /api/v1/pets/:id
    output:
      id: ulid
types:
  Pet:
    fields:
      id: ulid
      name: string
`
	if err := os.WriteFile(filepath.Join(dir, "contracts", "pets.contract.yml"), []byte(pets), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runGenerateFromContract(dir, "users", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := runGenerateFromContract(dir, "pets", ""); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "migrations", "0401_pets.up.sql")); err != nil {
		t.Errorf("expected pets migration at next free number 0401: %v", err)
	}
}

// TestRunGenerateModel_WritesModelMigrationAndTest: `acthur generate model
// Pet name:string(required) age:int?` → model struct + test + migration.
func TestRunGenerateModel_WritesModelMigrationAndTest(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runGenerateModel(dir, "Pet", []string{"name:string(required)", "age:int?"}, ""); err != nil {
		t.Fatalf("runGenerateModel: %v", err)
	}

	model, err := os.ReadFile(filepath.Join(dir, "api", "internal", "models", "pet.go"))
	if err != nil {
		t.Fatalf("model file: %v", err)
	}
	for _, want := range []string{"type Pet struct", "Name string", "Age *int64"} {
		if !strings.Contains(string(model), want) {
			t.Errorf("pet.go missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "api", "internal", "models", "pet_test.go")); err != nil {
		t.Errorf("expected model test beside the model: %v", err)
	}
	up, err := os.ReadFile(filepath.Join(dir, "migrations", "0400_pet.up.sql"))
	if err != nil {
		t.Fatalf("migration: %v", err)
	}
	if !strings.Contains(string(up), "CREATE TABLE IF NOT EXISTS pets") {
		t.Errorf("migration should create pets table, got:\n%s", up)
	}
}

// TestRunGenerateAIContext_WritesClaudeMDAtRoot: `acthur generate ai-context`
// writes CLAUDE.md at the project root (not under a node directory) via the
// write engine, describing the real graph.
func TestRunGenerateAIContext_WritesClaudeMDAtRoot(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	results, err := runGenerateAIContext(dir, "")
	if err != nil {
		t.Fatalf("runGenerateAIContext: %v", err)
	}
	if len(results) != 1 || results[0].Status != generate.StatusWritten {
		t.Fatalf("expected one written result, got %+v", results)
	}

	content, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("expected CLAUDE.md at project root: %v", err)
	}
	if !strings.Contains(string(content), "go:fiber") {
		t.Errorf("expected CLAUDE.md to describe the api node's adapter, got:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(dir, "generated.lock")); err != nil {
		t.Errorf("expected generated.lock to be written: %v", err)
	}
}

// TestRunGenerateAIContext_Cursor_WritesCursorrules: --tool cursor routes to
// .cursorrules instead of CLAUDE.md.
func TestRunGenerateAIContext_Cursor_WritesCursorrules(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runGenerateAIContext(dir, "cursor"); err != nil {
		t.Fatalf("runGenerateAIContext: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursorrules")); err != nil {
		t.Errorf("expected .cursorrules to exist: %v", err)
	}
}

// TestRunGenerateCI_WritesGitHubActionsWorkflow: `acthur generate ci` writes
// a real workflow file at the conventional GitHub Actions path.
func TestRunGenerateCI_WritesGitHubActionsWorkflow(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	results, err := runGenerateCI(dir, "")
	if err != nil {
		t.Fatalf("runGenerateCI: %v", err)
	}
	if len(results) != 1 || results[0].Status != generate.StatusWritten {
		t.Fatalf("expected one written result, got %+v", results)
	}

	content, err := os.ReadFile(filepath.Join(dir, ".github", "workflows", "acthur.yml"))
	if err != nil {
		t.Fatalf("expected .github/workflows/acthur.yml: %v", err)
	}
	if !strings.Contains(string(content), "acthur build") {
		t.Errorf("expected workflow to run acthur build, got:\n%s", content)
	}
}

// TestRunGenerateCI_UnsupportedTarget_Errors: unsupported CI targets fail
// clearly instead of silently emitting something wrong.
func TestRunGenerateCI_UnsupportedTarget_Errors(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runGenerateCI(dir, "circleci"); err == nil {
		t.Fatal("expected error for unsupported ci target")
	}
}

// TestRunGenerateDocs_WritesPerContractPages: `acthur generate docs` writes
// one page per contract plus an index, all under docs/api/.
func TestRunGenerateDocs_WritesPerContractPages(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	copyUsersContract(t, dir)
	writeTestProject(t, dir)

	results, err := runGenerateDocs(dir, "")
	if err != nil {
		t.Fatalf("runGenerateDocs: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected index + users page, got %+v", results)
	}
	for _, p := range []string{
		filepath.Join(dir, "docs", "api", "README.md"),
		filepath.Join(dir, "docs", "api", "users.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}
}

// TestRunGenerateDocs_UnsupportedTarget_Errors: only "markdown" (or empty,
// its default) is implemented today.
func TestRunGenerateDocs_UnsupportedTarget_Errors(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runGenerateDocs(dir, "openapi"); err == nil {
		t.Fatal("expected error for unsupported docs target")
	}
}

// TestRunGenerateSkill_WritesSkillFile: `acthur generate skill <name>` writes
// .claude/skills/<name>/SKILL.md.
func TestRunGenerateSkill_WritesSkillFile(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	results, err := runGenerateSkill(dir, "create-endpoint")
	if err != nil {
		t.Fatalf("runGenerateSkill: %v", err)
	}
	if len(results) != 1 || results[0].Status != generate.StatusWritten {
		t.Fatalf("expected one written result, got %+v", results)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "create-endpoint", "SKILL.md")); err != nil {
		t.Errorf("expected SKILL.md to exist: %v", err)
	}
}

// TestRunGenerateSkill_InvalidName_Errors: propagates skillgen's kebab-case
// validation as a command-level error.
func TestRunGenerateSkill_InvalidName_Errors(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runGenerateSkill(dir, "Not Valid"); err == nil {
		t.Fatal("expected error for invalid skill name")
	}
}
