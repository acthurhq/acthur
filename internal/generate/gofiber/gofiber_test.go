package gofiber_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gofiberadapter "github.com/acthurhq/acthur/internal/adapter/backend/gofiber"
	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/contract"
	gengofiber "github.com/acthurhq/acthur/internal/generate/gofiber"
	"github.com/acthurhq/acthur/internal/plugin"
	"github.com/acthurhq/acthur/internal/scaffold"
)

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := filepath.Glob(filepath.Join(dir, "go.mod")); err == nil {
			if matches, _ := filepath.Glob(filepath.Join(dir, "go.mod")); len(matches) == 1 {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up")
		}
		dir = parent
	}
}

func usersContract(t *testing.T) *contract.Contract {
	t.Helper()
	c, err := contract.ParseFile(filepath.Join(repoRoot(t), "testdata", "vetangle", "contracts", "users.contract.yml"))
	if err != nil {
		t.Fatalf("parse users contract: %v", err)
	}
	return c
}

func generateUsers(t *testing.T) []plugin.GeneratedFile {
	t.Helper()
	files, err := gengofiber.Generate(usersContract(t), plugin.GeneratorContext{
		ProjectName: "vetangle",
		NodeID:      "api",
		Extra:       map[string]any{"module_path": "github.com/acthur/vetangle/api"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return files
}

func fileByPath(files []plugin.GeneratedFile, path string) *plugin.GeneratedFile {
	for i := range files {
		if files[i].Path == path {
			return &files[i]
		}
	}
	return nil
}

// TestGenerate_EmitsExpectedFileSet: the pipeline turns users.contract.yml
// into the full go:fiber file set — a test beside every source file, plus
// the contract-derived migration pair at the project migrations dir.
func TestGenerate_EmitsExpectedFileSet(t *testing.T) {
	files := generateUsers(t)

	want := []string{
		"internal/users/dto.go",
		"internal/users/dto_test.go",
		"internal/users/handler.go",
		"internal/users/handler_test.go",
		"internal/users/service.go",
		"internal/users/service_test.go",
		"internal/users/repository.go",
		"internal/users/repository_test.go",
		"internal/users/routes.go",
		"internal/users/routes_test.go",
		"migrations/0400_users.up.sql",
		"migrations/0400_users.down.sql",
	}
	got := make(map[string]bool, len(files))
	for _, f := range files {
		got[f.Path] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing expected file %s", w)
		}
	}
	if len(files) != len(want) {
		t.Errorf("expected %d files, got %d: %v", len(want), len(files), keys(got))
	}
}

func keys(m map[string]bool) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// TestGenerate_EveryGoFileParses: every emitted .go file must be valid Go.
func TestGenerate_EveryGoFileParses(t *testing.T) {
	for _, f := range generateUsers(t) {
		if !strings.HasSuffix(f.Path, ".go") {
			continue
		}
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, f.Path, f.Content, parser.AllErrors); err != nil {
			t.Errorf("%s does not parse: %v\n%s", f.Path, err, f.Content)
		}
	}
}

// TestGenerate_TypeMappingAndValidation: spot-check the contract type
// mapping (§Shared conventions) and constraint-derived validation.
func TestGenerate_TypeMappingAndValidation(t *testing.T) {
	files := generateUsers(t)
	dto := fileByPath(files, "internal/users/dto.go")
	if dto == nil {
		t.Fatal("dto.go missing")
	}
	src := string(dto.Content)

	for _, want := range []string{
		"type User struct",                           // types: User
		"CreatedAt time.Time",                        // timestamp → time.Time
		"TenantID string",                            // ulid → string, ID casing
		"type CreateUserInput struct",                // endpoint input struct
		"func (in CreateUserInput) Validate() error", // constraints → Validate
		"type ListUsersOutput struct",                // endpoint output struct
		"Data []User",                                // array($User) → []User
		"*string",                                    // optional string? → pointer
	} {
		if !strings.Contains(src, want) {
			t.Errorf("dto.go missing %q", want)
		}
	}

	// enum + max constraints become validation code
	if !strings.Contains(src, "email_taken") == false && false {
		t.Skip()
	}
	for _, want := range []string{"100", "admin"} {
		if !strings.Contains(src, want) {
			t.Errorf("dto.go validation missing %q", want)
		}
	}
}

// TestGenerate_MigrationFromTypes: the migration derives a table from
// types: (ULID text pk, snake_case columns), in the 0400 contract range.
func TestGenerate_MigrationFromTypes(t *testing.T) {
	files := generateUsers(t)
	up := fileByPath(files, "migrations/0400_users.up.sql")
	if up == nil {
		t.Fatal("up migration missing")
	}
	sql := string(up.Content)
	for _, want := range []string{"CREATE TABLE IF NOT EXISTS users", "id TEXT PRIMARY KEY", "tenant_id TEXT", "created_at TIMESTAMPTZ"} {
		if !strings.Contains(sql, want) {
			t.Errorf("up migration missing %q in:\n%s", want, sql)
		}
	}
	down := fileByPath(files, "migrations/0400_users.down.sql")
	if down == nil || !strings.Contains(string(down.Content), "DROP TABLE IF EXISTS users") {
		t.Error("down migration should drop the users table")
	}
	if up.Overwrite {
		t.Error("migrations must be Overwrite=false (protect history)")
	}
}

// TestGenerate_HandlerRoutesAndErrors: handlers register every endpoint's
// method+path and map declared contract errors to statuses.
func TestGenerate_HandlerRoutesAndErrors(t *testing.T) {
	files := generateUsers(t)
	routes := fileByPath(files, "internal/users/routes.go")
	if routes == nil {
		t.Fatal("routes.go missing")
	}
	rsrc := string(routes.Content)
	for _, want := range []string{
		`app.Post("/api/v1/users"`,
		`app.Get("/api/v1/users/:id"`,
		`app.Put("/api/v1/users/:id"`,
		`app.Delete("/api/v1/users/:id"`,
		"func Mount(app *fiber.App, db *pgxpool.Pool)",
	} {
		if !strings.Contains(rsrc, want) {
			t.Errorf("routes.go missing %q", want)
		}
	}

	handler := fileByPath(files, "internal/users/handler.go")
	if handler == nil {
		t.Fatal("handler.go missing")
	}
	hsrc := string(handler.Content)
	for _, want := range []string{"email_taken", "not_found", "validation_failed", "409", "404"} {
		if !strings.Contains(hsrc, want) {
			t.Errorf("handler.go missing error mapping %q", want)
		}
	}
}

// TestGenerate_GoldStandard_CompilesAndTestsPassInScaffoldedProject: the
// phase's done-when at the package seam — scaffold a real go:fiber project,
// write the generated output in, and require `go build ./...` AND the
// generated tests themselves to pass.
func TestGenerate_GoldStandard_CompilesAndTestsPassInScaffoldedProject(t *testing.T) {
	if testing.Short() {
		t.Skip("gold-standard test builds a real project; skipped in -short")
	}
	dir := t.TempDir()

	cfg := config.Config{
		Project:      "goldwit",
		ModulePrefix: "github.com/acthur/goldwit",
		Identifiers:  config.IdentifierConfig{Strategy: config.StrategyULID},
	}
	sctx := scaffold.ResolveScaffoldContext(cfg, "api")
	scaffolded, err := (&gofiberadapter.Adapter{}).Scaffold(sctx)
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	for _, f := range scaffolded {
		dst := filepath.Join(dir, "api", f.Path)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, f.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := gengofiber.Generate(usersContract(t), plugin.GeneratorContext{
		ProjectName: "goldwit",
		NodeID:      "api",
		RootDir:     dir,
		Extra:       map[string]any{"module_path": sctx.ModulePath},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, f := range files {
		dst := filepath.Join(dir, "api", f.Path)
		if strings.HasPrefix(f.Path, "migrations/") {
			dst = filepath.Join(dir, f.Path)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, f.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	node := filepath.Join(dir, "api")
	for _, args := range [][]string{
		{"mod", "tidy"},
		{"build", "./..."},
		{"test", "./internal/users/..."},
	} {
		cmd := exec.Command("go", args...)
		cmd.Dir = node
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}
