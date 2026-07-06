package auth_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/adapter"
	_ "github.com/acthur/acthur/internal/adapter/backend/gofiber"
	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/plugin"
	_ "github.com/acthur/acthur/internal/plugin/builtin/auth"
	"github.com/acthur/acthur/internal/scaffold"
)

// ---------------------------------------------------------------------------
// Test helpers — drive the generator through the real KernelAPIImpl load
// path (plugin.NewKernelAPI + plugin.Load), never a mock of the kernel.
// ---------------------------------------------------------------------------

func minimalConfig() *config.Config {
	return &config.Config{
		Project: "p",
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
			},
		},
		Plugins: []config.PluginEntry{{Name: "auth"}},
	}
}

func loadAuthGenerator(t *testing.T) plugin.Generator {
	t.Helper()
	cfg := minimalConfig()
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, g, nil, nil)
	if _, err := plugin.Load([]string{"auth"}, bus, k); err != nil {
		t.Fatalf("load auth plugin: %v", err)
	}
	gen, ok := k.Generator("auth")
	if !ok {
		t.Fatal("expected the auth plugin to register a generator named \"auth\"")
	}
	return gen
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func TestAuthPlugin_Load_RegistersGeneratorNamedAuth(t *testing.T) {
	gen := loadAuthGenerator(t)
	adapters := gen.SupportedAdapters()
	if len(adapters) != 1 || adapters[0] != "go:fiber" {
		t.Fatalf("expected SupportedAdapters() == [go:fiber], got %v", adapters)
	}
}

// ---------------------------------------------------------------------------
// Generate — error paths
// ---------------------------------------------------------------------------

func TestGenerate_UnsupportedAdapter_ReturnsError(t *testing.T) {
	gen := loadAuthGenerator(t)
	_, err := gen.Generate("rust:axum", plugin.GeneratorContext{
		Extra: map[string]any{"module_path": "github.com/acme/api"},
	})
	if err == nil {
		t.Fatal("expected error for unsupported adapter, got nil")
	}
}

func TestGenerate_MissingModulePath_ReturnsError(t *testing.T) {
	gen := loadAuthGenerator(t)
	_, err := gen.Generate("go:fiber", plugin.GeneratorContext{})
	if err == nil {
		t.Fatal("expected error when ctx.Extra[\"module_path\"] is missing, got nil")
	}
}

// ---------------------------------------------------------------------------
// Generate — happy path: file set + go/parser validity
// ---------------------------------------------------------------------------

func TestGenerate_Fiber_EmitsExpectedFileSet(t *testing.T) {
	gen := loadAuthGenerator(t)
	files, err := gen.Generate("go:fiber", plugin.GeneratorContext{
		ProjectName: "p",
		NodeID:      "api",
		Config: map[string]any{
			"strategy":  "jwt",
			"providers": []any{"google", "github"},
		},
		Extra: map[string]any{"module_path": "github.com/acme/api"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantPaths := []string{
		"internal/auth/models.go",
		"internal/auth/password.go",
		"internal/auth/jwt.go",
		"internal/auth/session.go",
		"internal/auth/magiclink.go",
		"internal/auth/providers.go",
		"internal/auth/middleware.go",
		"internal/auth/handlers.go",
		"migrations/0100_users.up.sql",
		"migrations/0100_users.down.sql",
		"migrations/0101_sessions.up.sql",
		"migrations/0101_sessions.down.sql",
	}

	got := make(map[string][]byte, len(files))
	for _, f := range files {
		got[f.Path] = f.Content
	}
	for _, want := range wantPaths {
		if _, ok := got[want]; !ok {
			t.Errorf("expected generated file %q, not found (got paths: %v)", want, keysOf(got))
		}
	}
	if len(got) != len(wantPaths) {
		t.Errorf("expected exactly %d generated files, got %d: %v", len(wantPaths), len(got), keysOf(got))
	}

	// Every .go file must parse.
	for path, content := range got {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, path, content, parser.AllErrors); err != nil {
			t.Errorf("generated file %q does not parse as valid Go: %v\n---\n%s", path, err, content)
		}
	}

	// Migration content sanity: users table has the required columns.
	usersUp := string(got["migrations/0100_users.up.sql"])
	for _, want := range []string{"CREATE TABLE", "email", "UNIQUE", "password_hash", "created_at"} {
		if !strings.Contains(usersUp, want) {
			t.Errorf("expected users migration to contain %q:\n%s", want, usersUp)
		}
	}
	sessionsUp := string(got["migrations/0101_sessions.up.sql"])
	if !strings.Contains(sessionsUp, "sessions") || !strings.Contains(sessionsUp, "REFERENCES users") {
		t.Errorf("expected sessions migration to reference users:\n%s", sessionsUp)
	}
}

func TestGenerate_ProvidersConfig_RegistersEachProvider(t *testing.T) {
	gen := loadAuthGenerator(t)
	files, err := gen.Generate("go:fiber", plugin.GeneratorContext{
		Config: map[string]any{"providers": []any{"google", "github"}},
		Extra:  map[string]any{"module_path": "github.com/acme/api"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var providersGo string
	for _, f := range files {
		if f.Path == "internal/auth/providers.go" {
			providersGo = string(f.Content)
		}
	}
	if providersGo == "" {
		t.Fatal("expected internal/auth/providers.go to be generated")
	}
	for _, want := range []string{`Name: "google"`, `Name: "github"`} {
		if !strings.Contains(providersGo, want) {
			t.Errorf("expected providers.go to register %s:\n%s", want, providersGo)
		}
	}
}

func TestGenerate_DefaultStrategy_IsJWT(t *testing.T) {
	gen := loadAuthGenerator(t)
	files, err := gen.Generate("go:fiber", plugin.GeneratorContext{
		Extra: map[string]any{"module_path": "github.com/acme/api"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var handlersGo string
	for _, f := range files {
		if f.Path == "internal/auth/handlers.go" {
			handlersGo = string(f.Content)
		}
	}
	if !strings.Contains(handlersGo, `Strategy = "jwt"`) {
		t.Errorf("expected default strategy jwt in handlers.go:\n%s", handlersGo)
	}
}

// ---------------------------------------------------------------------------
// Gold-standard: compiles inside a real scaffolded gofiber project.
// ---------------------------------------------------------------------------

func TestGenerate_CompilesInsideScaffoldedGofiberProject(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compilation test in short mode")
	}

	dir := t.TempDir()
	cfg := config.Config{
		Project:      "authtest",
		ModulePrefix: "github.com/acthurtest",
		Identifiers:  config.IdentifierConfig{Strategy: config.StrategyULID},
	}
	sctx := scaffold.ResolveScaffoldContext(cfg, "api")
	sctx.RootDir = dir

	a, err := adapter.Resolve("go:fiber")
	if err != nil {
		t.Fatalf("resolve go:fiber: %v", err)
	}
	scaffolder, ok := a.(adapter.Scaffolder)
	if !ok {
		t.Fatal("go:fiber does not implement Scaffolder")
	}
	scaffoldFiles, err := scaffolder.Scaffold(sctx)
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	writeFiles(t, dir, scaffoldFiles)

	gen := loadAuthGenerator(t)
	genFiles, err := gen.Generate("go:fiber", plugin.GeneratorContext{
		ProjectName: cfg.Project,
		NodeID:      "api",
		Config:      map[string]any{"strategy": "jwt"},
		Extra:       map[string]any{"module_path": sctx.ModulePath},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, f := range genFiles {
		path := filepath.Join(dir, f.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		mode := os.FileMode(f.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(path, f.Content, mode); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "AUTH_JWT_SECRET=test-secret")
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, out)
	}

	build := exec.Command("go", "build", "./...")
	build.Dir = dir
	build.Env = append(os.Environ(), "AUTH_JWT_SECRET=test-secret")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeFiles(t *testing.T, dir string, files []adapter.File) {
	t.Helper()
	for _, f := range files {
		path := filepath.Join(dir, f.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		mode := os.FileMode(f.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(path, f.Content, mode); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
