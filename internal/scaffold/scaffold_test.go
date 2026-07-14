package scaffold_test

import (
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/scaffold"
)

func TestResolveScaffoldContext_UsesConfiguredModulePrefix(t *testing.T) {
	ctx := scaffold.ResolveScaffoldContext(config.Config{
		Project:      "vetangle",
		ModulePrefix: "github.com/acme/vetangle",
		Identifiers: config.IdentifierConfig{
			Strategy: config.StrategyUUIDv4,
		},
	}, "api")

	if ctx.ProjectName != "vetangle" {
		t.Errorf("expected project name vetangle, got %q", ctx.ProjectName)
	}
	if ctx.NodeID != "api" {
		t.Errorf("expected node id api, got %q", ctx.NodeID)
	}
	if ctx.ModulePath != "github.com/acme/vetangle/api" {
		t.Errorf("expected module path github.com/acme/vetangle/api, got %q", ctx.ModulePath)
	}
	if ctx.IDStrategy != string(config.StrategyUUIDv4) {
		t.Errorf("expected id strategy uuid-v4, got %q", ctx.IDStrategy)
	}
}

func TestResolveScaffoldContext_DefaultsModulePrefixToProjectName(t *testing.T) {
	ctx := scaffold.ResolveScaffoldContext(config.Config{
		Project: "vetangle",
		Identifiers: config.IdentifierConfig{
			Strategy: config.StrategyULID,
		},
	}, "api")

	if ctx.ModulePath != "vetangle/api" {
		t.Errorf("expected module path vetangle/api, got %q", ctx.ModulePath)
	}
}
