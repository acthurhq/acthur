package mcp

import (
	"encoding/json"
	"sort"

	"github.com/acthur/acthur/internal/aiagent"
	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/contract"
	"github.com/acthur/acthur/internal/graph"
)

type projectInfoResult struct {
	Project       string   `json:"project"`
	NodeCount     int      `json:"node_count"`
	ServiceCount  int      `json:"service_count"`
	InfraCount    int      `json:"infra_count"`
	PluginCount   int      `json:"plugin_count"`
	Adapters      []string `json:"adapters"`
	Plugins       []string `json:"plugins"`
	ContractCount int      `json:"contract_count"`
	AIConfigured  bool     `json:"ai_configured"`
	AIProvider    string   `json:"ai_provider,omitempty"`
}

// NewProjectInfoTool exposes a project-level summary of acthur.yml — the
// project_info MCP tool. It's the cheapest tool to call first: node/plugin
// counts, adapters in use, contract count, and whether an AI provider is
// configured (never the api_key value itself).
func NewProjectInfoTool(cfg *config.Config, g *graph.Graph, reg *contract.Registry) Tool {
	return Tool{
		Name:        "project_info",
		Description: "Summarize the current Acthur project: node/adapter/plugin counts and whether an AI provider is configured.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(json.RawMessage) (any, error) {
			adapterSet := map[string]bool{}
			for _, n := range g.Nodes() {
				if n.Adapter != "" {
					adapterSet[n.Adapter] = true
				}
			}
			adapters := make([]string, 0, len(adapterSet))
			for a := range adapterSet {
				adapters = append(adapters, a)
			}
			sort.Strings(adapters)

			plugins := make([]string, 0, len(cfg.Plugins))
			for _, p := range cfg.Plugins {
				plugins = append(plugins, p.Name)
			}
			sort.Strings(plugins)

			result := projectInfoResult{
				Project:       cfg.Project,
				NodeCount:     len(g.Nodes()),
				ServiceCount:  len(g.NodesByType(config.NodeTypeService)),
				InfraCount:    len(g.NodesByType(config.NodeTypeInfra)),
				PluginCount:   len(g.NodesByType(config.NodeTypePlugin)),
				Adapters:      adapters,
				Plugins:       plugins,
				ContractCount: len(reg.All()),
				AIConfigured:  cfg.AI.Provider != "" && aiagent.ResolveAPIKey(cfg.AI) != "",
				AIProvider:    string(cfg.AI.Provider),
			}
			return result, nil
		},
	}
}
