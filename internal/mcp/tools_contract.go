package mcp

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/acthur/acthur/internal/contract"
)

type contractSummary struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Transport string `json:"transport"`
	Endpoints int    `json:"endpoints"`
	Events    int    `json:"events"`
	FilePath  string `json:"file_path"`
}

// NewContractListTool exposes every registered contract's summary metadata
// — the contract_list MCP tool.
func NewContractListTool(reg *contract.Registry) Tool {
	return Tool{
		Name:        "contract_list",
		Description: "List every contract registered under contracts/*.contract.yml with its version, transport, and endpoint/event counts.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(json.RawMessage) (any, error) {
			all := reg.All()
			sort.Slice(all, func(i, j int) bool {
				if all[i].Name != all[j].Name {
					return all[i].Name < all[j].Name
				}
				return all[i].Version < all[j].Version
			})
			out := make([]contractSummary, 0, len(all))
			for _, c := range all {
				out = append(out, contractSummary{
					Name:      c.Name,
					Version:   c.Version,
					Transport: string(c.Transport),
					Endpoints: len(c.Endpoints),
					Events:    len(c.Events),
					FilePath:  c.FilePath,
				})
			}
			return out, nil
		},
	}
}

type contractShowArgs struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// NewContractShowTool exposes one contract's full parsed structure (all
// endpoints, events, and types) — the contract_show MCP tool. Version is
// optional; when omitted, the latest registered version of the named
// contract is returned.
func NewContractShowTool(reg *contract.Registry) Tool {
	return Tool{
		Name:        "contract_show",
		Description: "Show the full parsed structure of one contract by name (endpoints, events, types). Optionally pin a version; defaults to the latest registered version.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string", "description": "Contract name, e.g. \"users\""},
				"version": map[string]any{"type": "string", "description": "Optional exact version, e.g. \"v1\""},
			},
			"required": []string{"name"},
		},
		Handler: func(raw json.RawMessage) (any, error) {
			var args contractShowArgs
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &args); err != nil {
					return nil, fmt.Errorf("invalid arguments: %w", err)
				}
			}
			if args.Name == "" {
				return nil, fmt.Errorf("argument %q is required", "name")
			}

			if args.Version != "" {
				c, err := reg.Get(args.Name, args.Version)
				if err != nil {
					return nil, err
				}
				return c, nil
			}
			c, err := reg.GetLatest(args.Name)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
	}
}
