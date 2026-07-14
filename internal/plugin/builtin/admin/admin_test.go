package admin_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/adapter"
	_ "github.com/acthurhq/acthur/internal/adapter/backend/gofiber"
	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/plugin"
	_ "github.com/acthurhq/acthur/internal/plugin/builtin/admin"
	"github.com/acthurhq/acthur/internal/scaffold"
)

func minimalConfig() *config.Config {
	return &config.Config{
		Project: "p",
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
			},
		},
		Plugins: []config.PluginEntry{{Name: "admin"}},
	}
}

func loadGenerator(t *testing.T) plugin.Generator {
	t.Helper()
	g, err := graph.Build(minimalConfig())
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, g, nil, nil)
	if _, err := plugin.Load([]string{"admin"}, bus, k); err != nil {
		t.Fatalf("load admin plugin: %v", err)
	}
	gen, ok := k.Generator("admin")
	if !ok {
		t.Fatal("expected admin plugin to register a generator named \"admin\"")
	}
	return gen
}

func TestGenerator_UnsupportedAdapter_Errors(t *testing.T) {
	gen := loadGenerator(t)
	if _, err := gen.Generate("rust:axum", plugin.GeneratorContext{}); err == nil {
		t.Fatal("expected error for unsupported adapter")
	}
}

func TestGenerate_EmitsDashboardRoutes(t *testing.T) {
	gen := loadGenerator(t)
	files, err := gen.Generate("go:fiber", plugin.GeneratorContext{NodeID: "api"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(files) != 1 || files[0].Path != "internal/admin/admin.go" {
		t.Fatalf("unexpected files: %+v", files)
	}
	src := string(files[0].Content)
	for _, want := range []string{
		`ServiceName = "api"`,
		`app.Get("/admin"`,
		`app.Get("/admin/api/status"`,
		`app.Get("/admin/api/flags"`,
		"func Mount(",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("expected generated admin.go to contain %q:\n%s", want, src)
		}
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
		Project:      "admintest",
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
	for _, f := range scaffoldFiles {
		writeFile(t, dir, f.Path, f.Content, f.Mode)
	}

	gen := loadGenerator(t)
	genFiles, err := gen.Generate("go:fiber", plugin.GeneratorContext{NodeID: "api"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, f := range genFiles {
		writeFile(t, dir, f.Path, f.Content, f.Mode)
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, out)
	}

	build := exec.Command("go", "build", "./...")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
}

func writeFile(t *testing.T, dir, relPath string, content []byte, fileMode uint32) {
	t.Helper()
	path := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	mode := os.FileMode(fileMode)
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
