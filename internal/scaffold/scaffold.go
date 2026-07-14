// Package scaffold resolves adapter scaffold inputs from project config.
package scaffold

import (
	"github.com/acthurhq/acthur/internal/adapter"
	"github.com/acthurhq/acthur/internal/config"
)

// ResolveScaffoldContext assembles the config-backed context passed to adapters.
func ResolveScaffoldContext(cfg config.Config, nodeID string) adapter.ScaffoldContext {
	prefix := cfg.ModulePrefix
	if prefix == "" {
		prefix = cfg.Project
	}

	return adapter.ScaffoldContext{
		ProjectName: cfg.Project,
		NodeID:      nodeID,
		ModulePath:  prefix + "/" + nodeID,
		IDStrategy:  string(cfg.Identifiers.Strategy),
	}
}
