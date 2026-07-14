// Package migrations is the built-in "migrations" plugin: it registers a
// generator that scaffolds a project's migrations/ directory with an initial
// golang-migrate-compatible pair, and exposes the database-command logic
// consumed by `acthur db migrate|rollback|status|create` (wired in
// cmd/acthur, since the plugin KernelAPI's CLICommand model registers only
// flat top-level commands — it has no notion of a command group, and
// `acthur db` already exists as a real cobra group in cmd/acthur/commands.go).
//
// Migration numbering: this plugin owns the 0001-0099 range (see
// docs/prd/phase-6-first-plugins.md "Shared conventions") so that migrations,
// auth, rbac, and multitenancy never collide on a filename.
package migrations

import (
	"fmt"

	"github.com/acthurhq/acthur/internal/adapter"
	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/plugin"
)

const (
	// Name is the plugin's registered name and its generator target name
	// (shared convention: one generator per plugin, named after the plugin).
	Name = "migrations"

	// initUpSQL / initDownSQL are the seed migration pair emitted by the
	// generator — an empty, harmless starting point every project can build on.
	initUpSQL   = "-- 0001_init: initial (empty) migration\n"
	initDownSQL = "-- 0001_init: rollback of initial (empty) migration\n"
)

type migrationsPlugin struct{}

func (migrationsPlugin) Name() string        { return Name }
func (migrationsPlugin) Version() string     { return "0.1.0" }
func (migrationsPlugin) DependsOn() []string { return nil }

func (migrationsPlugin) Register(k plugin.KernelAPI) error {
	k.RegisterGenerator(Name, generator{})
	return nil
}

func init() {
	plugin.Register(migrationsPlugin{})
}

// ---------------------------------------------------------------------------
// Generator
// ---------------------------------------------------------------------------

type generator struct{}

// SupportedAdapters returns "*": migrations are project-root files, not
// specific to any one node's framework.
func (generator) SupportedAdapters() []string { return []string{"*"} }

// Generate emits migrations/.keep (so the directory survives in git even
// before the first real migration) plus the 0001_init up/down pair. Every
// file's path starts with "migrations/" so the caller (cmd/acthur's `add`
// command) writes them at the project root rather than under a node's
// directory, per the shared GeneratedFile path convention.
func (generator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	return []plugin.GeneratedFile{
		{
			Path:      "migrations/.keep",
			Content:   []byte(""),
			Overwrite: false,
		},
		{
			Path:      "migrations/0001_init.up.sql",
			Content:   []byte(initUpSQL),
			Overwrite: false,
		},
		{
			Path:      "migrations/0001_init.down.sql",
			Content:   []byte(initDownSQL),
			Overwrite: false,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Database URL derivation — reuses the same Connectable capability the dev
// engine uses to inject DATABASE_URL into consumer services (see
// internal/engine/dev.go resolveNodeEnv), so the coordinates `acthur db`
// commands connect to can never drift from what a running project actually
// exports to its services.
// ---------------------------------------------------------------------------

// ErrNoPostgresNode is returned by DatabaseURL when the graph has no
// db:postgres infra node.
var ErrNoPostgresNode = fmt.Errorf(
	"no db:postgres node found in the graph — add one under graph.nodes to use `acthur db` commands")

// DatabaseURL locates the graph's db:postgres node and derives its
// DATABASE_URL via the adapter's Connectable capability.
func DatabaseURL(g *graph.Graph) (string, error) {
	node := findPostgresNode(g)
	if node == nil {
		return "", ErrNoPostgresNode
	}

	a, err := adapter.Resolve(node.Adapter)
	if err != nil {
		return "", fmt.Errorf("resolving adapter %q for node %q: %w", node.Adapter, node.ID, err)
	}

	connectable, ok := a.(adapter.Connectable)
	if !ok {
		return "", fmt.Errorf("adapter %q does not implement Connectable — cannot derive a DATABASE_URL", node.Adapter)
	}

	env := connectable.ConnectionEnv(adapter.ContainerContext{
		NodeID:  node.ID,
		Version: node.Config.Version,
	})
	url, ok := env["DATABASE_URL"]
	if !ok || url == "" {
		return "", fmt.Errorf("adapter %q did not produce a DATABASE_URL", node.Adapter)
	}
	return url, nil
}

// findPostgresNode returns the first infra node whose adapter is
// "db:postgres", or nil if none exists.
func findPostgresNode(g *graph.Graph) *graph.Node {
	for _, n := range g.NodesByType(config.NodeTypeInfra) {
		if n.Adapter == "db:postgres" {
			return n
		}
	}
	return nil
}
