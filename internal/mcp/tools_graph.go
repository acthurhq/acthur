package mcp

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
)

// nodeInfo is the wire shape of one graph node in graph_query's result.
// State reflects this process's in-memory view only — `acthur mcp serve`
// does not share state with a separately-running `acthur dev` process, so
// a node not currently supervised by this process reports "pending" even
// if it is, in reality, running under a different `acthur dev` invocation.
type nodeInfo struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Adapter string `json:"adapter"`
	Port    int    `json:"port,omitempty"`
	Role    string `json:"role,omitempty"`
	State   string `json:"state"`
}

type edgeInfo struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Type      string   `json:"type"`
	Contracts []string `json:"contracts,omitempty"`
	Transport string   `json:"transport,omitempty"`
	Events    []string `json:"events,omitempty"`
}

type graphQueryResult struct {
	Nodes []nodeInfo `json:"nodes"`
	Edges []edgeInfo `json:"edges"`
}

type graphQueryArgs struct {
	// Type filters nodes by kind: "service", "infra", or "plugin".
	// Empty returns every node.
	Type string `json:"type"`
}

// NewGraphQueryTool exposes the live graph (nodes + edges) as structured
// JSON — the graph_query MCP tool.
func NewGraphQueryTool(g *graph.Graph) Tool {
	return Tool{
		Name:        "graph_query",
		Description: "Return the Acthur graph's nodes and edges as structured JSON. Optionally filter nodes by type (service|infra|plugin).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"description": "Filter nodes by type: service, infra, or plugin. Omit for all nodes.",
					"enum":        []string{"service", "infra", "plugin"},
				},
			},
		},
		Handler: func(raw json.RawMessage) (any, error) {
			var args graphQueryArgs
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &args); err != nil {
					return nil, fmt.Errorf("invalid arguments: %w", err)
				}
			}

			var nodeType config.NodeType
			switch args.Type {
			case "":
				// no filter
			case "service":
				nodeType = config.NodeTypeService
			case "infra":
				nodeType = config.NodeTypeInfra
			case "plugin":
				nodeType = config.NodeTypePlugin
			default:
				return nil, fmt.Errorf("unknown type filter %q (want service, infra, or plugin)", args.Type)
			}

			var nodes []*graph.Node
			if args.Type == "" {
				nodes = g.Nodes()
			} else {
				nodes = g.NodesByType(nodeType)
			}
			sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

			result := graphQueryResult{Nodes: make([]nodeInfo, 0, len(nodes))}
			for _, n := range nodes {
				result.Nodes = append(result.Nodes, nodeInfo{
					ID:      n.ID,
					Type:    string(n.Type),
					Adapter: n.Adapter,
					Port:    n.Port,
					Role:    string(n.Role),
					State:   string(n.State()),
				})
			}

			edges := g.Edges()
			for _, e := range edges {
				result.Edges = append(result.Edges, edgeInfo{
					From:      e.From,
					To:        e.To,
					Type:      string(e.Type),
					Contracts: e.Contracts,
					Transport: string(e.Transport),
					Events:    e.Events,
				})
			}
			return result, nil
		},
	}
}
