package visualize_test

import (
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/generate/visualize"
	"github.com/acthur/acthur/internal/graph"
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
				{From: "api", To: "db", Type: config.EdgeDataFlow, Contracts: []string{"users"}},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("graph.Build: %v", err)
	}
	return g
}

func TestMermaid_ContainsNodesAndEdges(t *testing.T) {
	out := visualize.Mermaid(testGraph(t))
	if !strings.HasPrefix(out, "graph TD\n") {
		t.Errorf("expected mermaid flowchart header, got:\n%s", out)
	}
	for _, want := range []string{"api", "go:fiber", "db", "db:postgres", "depends_on", "data_flow", "users", "proxy"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected mermaid output to contain %q, got:\n%s", want, out)
		}
	}
	// infra node rendered as a cylinder shape `[( ... )]`
	if !strings.Contains(out, "[(") {
		t.Errorf("expected an infra node cylinder shape, got:\n%s", out)
	}
}

func TestDot_ContainsNodesAndEdges(t *testing.T) {
	out := visualize.Dot(testGraph(t))
	if !strings.HasPrefix(out, "digraph acthur {") {
		t.Errorf("expected dot digraph header, got:\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), "}") {
		t.Errorf("expected dot digraph to be closed, got:\n%s", out)
	}
	for _, want := range []string{`"api"`, `"db"`, "shape=box", "shape=cylinder", "depends_on", "data_flow"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected dot output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRender_DefaultsToMermaid(t *testing.T) {
	out, err := visualize.Render(testGraph(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "graph TD") {
		t.Errorf("expected default format to be mermaid, got:\n%s", out)
	}
}

func TestRender_Dot(t *testing.T) {
	out, err := visualize.Render(testGraph(t), "dot")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "digraph") {
		t.Errorf("expected dot output, got:\n%s", out)
	}
}

func TestRender_UnsupportedFormat_Errors(t *testing.T) {
	_, err := visualize.Render(testGraph(t), "svg")
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "mermaid") || !strings.Contains(err.Error(), "dot") {
		t.Errorf("expected error to list supported formats, got: %v", err)
	}
}

// TestMermaid_NoStrayEscapes regresses a bug where labels were built with an
// embedded `\n` line-break escape and then re-quoted with fmt's %q verb,
// which escapes the literal backslash again — producing a broken `\\n` in
// the rendered diagram instead of a real line break.
func TestMermaid_NoStrayEscapes(t *testing.T) {
	out := visualize.Mermaid(testGraph(t))
	if strings.Contains(out, `\\`) {
		t.Errorf("expected no double-escaped backslashes in mermaid output, got:\n%s", out)
	}
}

func TestDot_NoStrayEscapes(t *testing.T) {
	out := visualize.Dot(testGraph(t))
	if strings.Contains(out, `\\`) {
		t.Errorf("expected no double-escaped backslashes in dot output, got:\n%s", out)
	}
}

func TestRender_Deterministic(t *testing.T) {
	g := testGraph(t)
	m1, _ := visualize.Render(g, "mermaid")
	m2, _ := visualize.Render(g, "mermaid")
	if m1 != m2 {
		t.Error("expected deterministic mermaid output")
	}
	d1, _ := visualize.Render(g, "dot")
	d2, _ := visualize.Render(g, "dot")
	if d1 != d2 {
		t.Error("expected deterministic dot output")
	}
}
