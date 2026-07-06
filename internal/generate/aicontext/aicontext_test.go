package aicontext_test

import (
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/contract"
	"github.com/acthur/acthur/internal/generate/aicontext"
	"github.com/acthur/acthur/internal/graph"
)

func testGraph(t *testing.T) *graph.Graph {
	t.Helper()
	cfg := &config.Config{
		Project: "vetangle",
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080, Role: config.NodeRoleServer},
				"db":  {Type: config.NodeTypeInfra, Adapter: "db:postgres"},
			},
			Edges: []config.EdgeConfig{
				{From: "api", To: "db", Type: config.EdgeDependsOn},
				{From: "api", To: "db", Type: config.EdgeDataFlow, Contracts: []string{"users"}},
			},
		},
		Plugins: []config.PluginEntry{{Name: "migrations"}, {Name: "auth"}},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("graph.Build: %v", err)
	}
	return g
}

func testConfig() *config.Config {
	return &config.Config{
		Project: "vetangle",
		Plugins: []config.PluginEntry{{Name: "migrations"}, {Name: "auth"}},
	}
}

func TestGenerate_Claude_DefaultTool_WritesClaudeMD(t *testing.T) {
	cfg := testConfig()
	g := testGraph(t)
	reg := contract.NewRegistry()

	file, err := aicontext.Generate(cfg, g, reg, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if file.Path != "CLAUDE.md" {
		t.Errorf("expected CLAUDE.md, got %s", file.Path)
	}
	content := string(file.Content)
	for _, want := range []string{
		"vetangle", "## Nodes", "api", "go:fiber", "db", "db:postgres",
		"## Edges", "depends_on", "data_flow", "users",
		"## Plugins", "auth", "migrations",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected content to contain %q, got:\n%s", want, content)
		}
	}
}

func TestGenerate_Cursor_WritesCursorrules(t *testing.T) {
	cfg := testConfig()
	g := testGraph(t)

	file, err := aicontext.Generate(cfg, g, nil, "cursor")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if file.Path != ".cursorrules" {
		t.Errorf("expected .cursorrules, got %s", file.Path)
	}
}

func TestGenerate_UnsupportedTool_ErrorsWithSupportedList(t *testing.T) {
	_, err := aicontext.Generate(testConfig(), testGraph(t), nil, "windsurf")
	if err == nil {
		t.Fatal("expected error for unsupported tool")
	}
	if !strings.Contains(err.Error(), "claude") || !strings.Contains(err.Error(), "cursor") {
		t.Errorf("expected error to list supported tools, got: %v", err)
	}
}

func TestGenerate_Deterministic_SameInputSameOutput(t *testing.T) {
	cfg := testConfig()
	g := testGraph(t)
	reg := contract.NewRegistry()

	f1, err := aicontext.Generate(cfg, g, reg, "claude")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := aicontext.Generate(cfg, g, reg, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if string(f1.Content) != string(f2.Content) {
		t.Error("expected deterministic output across repeated runs")
	}
}
