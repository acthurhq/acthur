package main

import (
	"strings"
	"testing"
)

// TestRunGraphVisualize_Mermaid_DefaultFormat: `acthur graph visualize` with
// no --format renders the project graph as a Mermaid flowchart.
func TestRunGraphVisualize_Mermaid_DefaultFormat(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	out, err := runGraphVisualize(dir, "")
	if err != nil {
		t.Fatalf("runGraphVisualize: %v", err)
	}
	if !strings.HasPrefix(out, "graph TD") {
		t.Errorf("expected a mermaid flowchart, got:\n%s", out)
	}
	if !strings.Contains(out, "go:fiber") {
		t.Errorf("expected the api node's adapter to appear, got:\n%s", out)
	}
	if !strings.Contains(out, "proxy") {
		t.Errorf("expected the kernel proxy node to appear, got:\n%s", out)
	}
}

// TestRunGraphVisualize_Dot_Format: --format dot renders a Graphviz digraph.
func TestRunGraphVisualize_Dot_Format(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	out, err := runGraphVisualize(dir, "dot")
	if err != nil {
		t.Fatalf("runGraphVisualize: %v", err)
	}
	if !strings.HasPrefix(out, "digraph acthur {") {
		t.Errorf("expected a dot digraph, got:\n%s", out)
	}
}

// TestRunGraphVisualize_UnsupportedFormat_Errors: an unknown --format value
// fails clearly rather than silently falling back to mermaid.
func TestRunGraphVisualize_UnsupportedFormat_Errors(t *testing.T) {
	resetPluginProcessState(t)
	dir := t.TempDir()
	writeTestProject(t, dir)

	if _, err := runGraphVisualize(dir, "svg"); err == nil {
		t.Fatal("expected error for unsupported visualize format")
	}
}
