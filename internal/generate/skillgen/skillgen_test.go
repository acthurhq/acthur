package skillgen_test

import (
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/generate/skillgen"
	"github.com/acthurhq/acthur/internal/graph"
)

func testGraph(t *testing.T) *graph.Graph {
	t.Helper()
	cfg := &config.Config{
		Project: "vetangle",
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
				"db":  {Type: config.NodeTypeInfra, Adapter: "db:postgres"},
			},
			Edges: []config.EdgeConfig{
				{From: "api", To: "db", Type: config.EdgeDependsOn},
			},
		},
		Plugins: []config.PluginEntry{{Name: "auth"}},
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
		Plugins: []config.PluginEntry{{Name: "auth"}},
	}
}

func TestGenerate_WritesSkillFileWithFrontmatter(t *testing.T) {
	file, err := skillgen.Generate(testConfig(), testGraph(t), "create-endpoint")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if file.Path != ".claude/skills/create-endpoint/SKILL.md" {
		t.Errorf("expected .claude/skills/create-endpoint/SKILL.md, got %s", file.Path)
	}

	content := string(file.Content)
	if !strings.HasPrefix(content, "---\nname: create-endpoint\ndescription: ") {
		t.Errorf("expected YAML frontmatter starting with name/description, got:\n%s", content)
	}
	for _, want := range []string{"vetangle", "go:fiber", "auth", "# Create Endpoint"} {
		if !strings.Contains(content, want) {
			t.Errorf("expected content to contain %q, got:\n%s", want, content)
		}
	}
}

func TestGenerate_EmptyName_Errors(t *testing.T) {
	if _, err := skillgen.Generate(testConfig(), testGraph(t), "  "); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestGenerate_InvalidName_Errors(t *testing.T) {
	for _, bad := range []string{"CreateEndpoint", "create_endpoint", "-leading-dash", "trailing-", "with space"} {
		if _, err := skillgen.Generate(testConfig(), testGraph(t), bad); err == nil {
			t.Errorf("expected error for invalid name %q", bad)
		}
	}
}

func TestGenerate_ValidKebabNames(t *testing.T) {
	for _, ok := range []string{"a", "create-endpoint", "tenant-patterns2"} {
		if _, err := skillgen.Generate(testConfig(), testGraph(t), ok); err != nil {
			t.Errorf("expected %q to be accepted, got: %v", ok, err)
		}
	}
}

func TestGenerate_NoPlugins_SaysNone(t *testing.T) {
	cfg := &config.Config{Project: "p"}
	g, err := graph.Build(&config.Config{Graph: config.GraphConfig{
		Nodes: map[string]config.NodeConfig{"api": {Type: config.NodeTypeService, Adapter: "go:fiber"}},
		Edges: []config.EdgeConfig{{From: "api", To: "proxy", Type: config.EdgeProxiedThrough}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	file, err := skillgen.Generate(cfg, g, "debug-runtime")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(file.Content), "**Active plugins**: none") {
		t.Errorf("expected explicit 'none' for a project with no plugins, got:\n%s", file.Content)
	}
}
