package main

import (
	"fmt"
	"io"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/contract"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/health"
	"github.com/acthurhq/acthur/internal/mcp"
)

// loadMCPContext loads acthur.yml, builds the graph, and loads the contract
// registry for a single `acthur mcp serve` run. Unlike loadGraph (used by
// every other command), this never calls output.Fatal / os.Exit and never
// runs plugin hooks: mcp serve is a long-lived process serving many
// requests, and re-running loadPlugins per tool call would re-register
// every plugin hook/command on process-global state. The tradeoff is
// explicit: MCP tools see the declared graph topology, not any
// plugin-injected nodes/edges — read-only introspection doesn't need them.
func loadMCPContext(root string) (*config.Config, *graph.Graph, *contract.Registry, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load acthur.yml: %w", err)
	}
	g, err := graph.Build(cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("graph build failed: %w", err)
	}
	reg, err := contract.LoadDir(root)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load contracts: %w", err)
	}
	g.Freeze()
	return cfg, g, reg, nil
}

// runMCPServe wires acthur's graph/contract/health introspection into an
// MCP server and serves it over in/out until in reaches EOF (the client
// closed stdin) or a read error occurs.
func runMCPServe(root string, in io.Reader, out io.Writer) error {
	cfg, g, reg, err := loadMCPContext(root)
	if err != nil {
		return err
	}

	tools := []mcp.Tool{
		mcp.NewGraphQueryTool(g),
		mcp.NewContractListTool(reg),
		mcp.NewContractShowTool(reg),
		mcp.NewServiceHealthTool(g, health.New()),
		mcp.NewProjectInfoTool(cfg, g, reg),
	}

	server := mcp.NewServer("acthur", Version, tools)
	return server.Serve(in, out)
}
