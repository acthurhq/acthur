package mcp

import (
	"os"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/contract"
)

func TestProjectInfoTool_Summarizes(t *testing.T) {
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	cfg := &config.Config{
		Project: "demo",
		Plugins: []config.PluginEntry{{Name: "migrations"}, {Name: "auth"}},
	}
	g := healthTestGraph() // api, web (service), db (infra)
	reg := testRegistry(t)

	tool := NewProjectInfoTool(cfg, g, reg)
	result, err := tool.Handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info := result.(projectInfoResult)

	if info.NodeCount != 3 {
		t.Fatalf("got NodeCount %d, want 3", info.NodeCount)
	}
	if info.ServiceCount != 2 {
		t.Fatalf("got ServiceCount %d, want 2", info.ServiceCount)
	}
	if info.InfraCount != 1 {
		t.Fatalf("got InfraCount %d, want 1", info.InfraCount)
	}
	if len(info.Plugins) != 2 {
		t.Fatalf("got Plugins %v, want 2 entries", info.Plugins)
	}
	if info.ContractCount != 2 {
		t.Fatalf("got ContractCount %d, want 2", info.ContractCount)
	}
	if info.AIConfigured {
		t.Fatal("expected AIConfigured false when no ai: block is set")
	}
}

func TestProjectInfoTool_AIConfiguredWhenKeyResolves(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	cfg := &config.Config{Project: "demo", AI: config.AIConfig{Provider: config.ProviderClaude}}
	g := healthTestGraph()
	reg := contract.NewRegistry()

	tool := NewProjectInfoTool(cfg, g, reg)
	result, err := tool.Handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info := result.(projectInfoResult)
	if !info.AIConfigured {
		t.Fatal("expected AIConfigured true")
	}
	if info.AIProvider != "claude" {
		t.Fatalf("got AIProvider %q, want claude", info.AIProvider)
	}
}
