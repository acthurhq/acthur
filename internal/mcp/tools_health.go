package mcp

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
)

// HealthPoller runs a single live health check on a node. health.Checker
// satisfies this (structurally — see internal/health) without internal/mcp
// needing to import it, keeping this package's dependency surface to
// graph/contract/config only.
type HealthPoller interface {
	Poll(node *graph.Node) error
}

type serviceHealthEntry struct {
	Node    string `json:"node"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

type serviceHealthArgs struct {
	// Node optionally scopes the check to a single node id. Empty checks
	// every service node.
	Node string `json:"node"`
}

// NewServiceHealthTool live-probes service nodes' /health endpoints — the
// service_health MCP tool. A probe failure (connection refused, non-2xx,
// timeout) is reported as healthy: false with the error, not a tool-call
// error — an unhealthy service is an expected, informative result, not a
// protocol failure.
func NewServiceHealthTool(g *graph.Graph, poller HealthPoller) Tool {
	return Tool{
		Name:        "service_health",
		Description: "Live-probe one or all service nodes' /health endpoints and report healthy/unhealthy with the error.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node": map[string]any{
					"type":        "string",
					"description": "Optional service node id to check. Omit to check every service node.",
				},
			},
		},
		Handler: func(raw json.RawMessage) (any, error) {
			var args serviceHealthArgs
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &args); err != nil {
					return nil, fmt.Errorf("invalid arguments: %w", err)
				}
			}

			var targets []*graph.Node
			if args.Node != "" {
				n := g.Node(args.Node)
				if n == nil {
					return nil, fmt.Errorf("node %q not found in graph", args.Node)
				}
				if !n.IsService() {
					return nil, fmt.Errorf("node %q is not a service node (type %q)", args.Node, n.Type)
				}
				targets = []*graph.Node{n}
			} else {
				targets = g.NodesByType(config.NodeTypeService)
			}
			sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })

			results := make([]serviceHealthEntry, 0, len(targets))
			for _, n := range targets {
				entry := serviceHealthEntry{Node: n.ID}
				if err := poller.Poll(n); err != nil {
					entry.Error = err.Error()
				} else {
					entry.Healthy = true
				}
				results = append(results, entry)
			}
			return results, nil
		},
	}
}
