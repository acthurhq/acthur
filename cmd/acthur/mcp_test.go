package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mcpTestFixture = `project: mcp-fixture
version: "1"

graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
    db:
      type: infra
      adapter: db:postgres

  edges:
    - from: api
      to: db
      type: depends_on
`

func writeMCPFixtureProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte(mcpTestFixture), 0o644); err != nil {
		t.Fatalf("write fixture acthur.yml: %v", err)
	}
	return dir
}

func TestLoadMCPContext_BuildsGraphAndRegistry(t *testing.T) {
	dir := writeMCPFixtureProject(t)
	cfg, g, reg, err := loadMCPContext(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Project != "mcp-fixture" {
		t.Fatalf("got project %q", cfg.Project)
	}
	if g.Node("api") == nil || g.Node("db") == nil {
		t.Fatalf("expected api and db nodes in graph")
	}
	if reg == nil {
		t.Fatal("expected a non-nil (possibly empty) contract registry")
	}
}

func TestLoadMCPContext_MissingProjectErrors(t *testing.T) {
	dir := t.TempDir()
	_, _, _, err := loadMCPContext(dir)
	if err == nil {
		t.Fatal("expected error for a directory with no acthur.yml")
	}
}

// TestRunMCPServe_EndToEnd drives the server through a real multi-message
// JSON-RPC session — initialize, tools/list, then tools/call graph_query —
// over in-memory pipes standing in for stdin/stdout, exactly as `acthur mcp
// serve` would be driven by a real MCP client.
func TestRunMCPServe_EndToEnd(t *testing.T) {
	dir := writeMCPFixtureProject(t)

	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"graph_query","arguments":{}}}` + "\n" +
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"project_info","arguments":{}}}` + "\n"

	var out bytes.Buffer
	if err := runMCPServe(dir, strings.NewReader(input), &out); err != nil {
		t.Fatalf("runMCPServe error: %v", err)
	}

	scanner := bufio.NewScanner(&out)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []map[string]any
	for scanner.Scan() {
		var resp map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON line: %v\n%s", err, scanner.Text())
		}
		lines = append(lines, resp)
	}
	if len(lines) != 4 {
		t.Fatalf("expected 4 responses (init, tools/list, 2x tools/call), got %d", len(lines))
	}

	toolsListResult := lines[1]["result"].(map[string]any)
	tools := toolsListResult["tools"].([]any)
	if len(tools) != 5 {
		t.Fatalf("expected 5 tools registered, got %d", len(tools))
	}

	graphResult := lines[2]["result"].(map[string]any)
	content := graphResult["content"].([]any)[0].(map[string]any)
	text := content["text"].(string)
	if !strings.Contains(text, "\"api\"") || !strings.Contains(text, "\"db\"") {
		t.Fatalf("graph_query result missing expected nodes: %s", text)
	}

	projectResult := lines[3]["result"].(map[string]any)
	projectContent := projectResult["content"].([]any)[0].(map[string]any)
	if !strings.Contains(projectContent["text"].(string), "mcp-fixture") {
		t.Fatalf("project_info result missing project name: %v", projectContent["text"])
	}
}
