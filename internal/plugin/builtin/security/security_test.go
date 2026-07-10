package security_test

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
	_ "github.com/acthurhq/acthur/internal/plugin/builtin/security"
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
		Plugins: []config.PluginEntry{{Name: "security"}},
	}
}

func loadGenerator(t *testing.T, g *graph.Graph) plugin.Generator {
	t.Helper()
	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, g, nil, nil)
	if _, err := plugin.Load([]string{"security"}, bus, k); err != nil {
		t.Fatalf("load security plugin: %v", err)
	}
	gen, ok := k.Generator("security")
	if !ok {
		t.Fatal("expected security plugin to register a generator named \"security\"")
	}
	return gen
}

func TestGenerator_UnsupportedAdapter_Errors(t *testing.T) {
	g, err := graph.Build(minimalConfig())
	if err != nil {
		t.Fatal(err)
	}
	gen := loadGenerator(t, g)
	if _, err := gen.Generate("rust:axum", plugin.GeneratorContext{}); err == nil {
		t.Fatal("expected error for unsupported adapter")
	}
}

func TestGenerate_NoGraphOrConfig_EmptyAllowlist(t *testing.T) {
	g, err := graph.Build(minimalConfig())
	if err != nil {
		t.Fatal(err)
	}
	gen := loadGenerator(t, g)
	files, err := gen.Generate("go:fiber", plugin.GeneratorContext{NodeID: "api"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(files[0].Content)
	if strings.Contains(src, `"https://`) {
		t.Errorf("expected no origins in allowlist without graph data_flow edges or config, got:\n%s", src)
	}
	if !strings.Contains(src, "helmet.New(") || !strings.Contains(src, "limiter.New(") {
		t.Errorf("expected helmet + limiter middleware wired:\n%s", src)
	}
}

func TestGenerate_DerivesAllowlistFromDataFlowEdges(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080, DevURL: "api.p.test"},
				"web": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 3000, DevURL: "web.p.test"},
			},
			Edges: []config.EdgeConfig{
				{From: "web", To: "api", Type: config.EdgeDataFlow},
			},
		},
		Plugins: []config.PluginEntry{{Name: "security"}},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	gen := loadGenerator(t, g)
	files, err := gen.Generate("go:fiber", plugin.GeneratorContext{
		NodeID: "api",
		Extra:  map[string]any{"graph": g},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(files[0].Content)
	if !strings.Contains(src, `"https://web.p.test"`) {
		t.Errorf("expected allowlist to contain web's dev URL derived from the data_flow edge:\n%s", src)
	}
}

func TestGenerate_FallsBackToConfigAllowedOrigins(t *testing.T) {
	g, err := graph.Build(minimalConfig())
	if err != nil {
		t.Fatal(err)
	}
	gen := loadGenerator(t, g)
	files, err := gen.Generate("go:fiber", plugin.GeneratorContext{
		NodeID: "api",
		Config: map[string]any{"allowed_origins": []any{"https://app.example.com"}},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(files[0].Content)
	if !strings.Contains(src, `"https://app.example.com"`) {
		t.Errorf("expected config-provided origin in allowlist:\n%s", src)
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
		Project:      "sectest",
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

	g, err := graph.Build(minimalConfig())
	if err != nil {
		t.Fatal(err)
	}
	gen := loadGenerator(t, g)
	genFiles, err := gen.Generate("go:fiber", plugin.GeneratorContext{
		NodeID: "api",
		Config: map[string]any{"allowed_origins": []any{"https://app.example.com"}},
	})
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
