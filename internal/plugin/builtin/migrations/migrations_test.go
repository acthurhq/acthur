package migrations_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acthur/acthur/internal/adapter"
	_ "github.com/acthur/acthur/internal/adapter/backend/gofiber"
	_ "github.com/acthur/acthur/internal/adapter/infra/postgres"
	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/plugin"
	"github.com/acthur/acthur/internal/plugin/builtin/migrations"
)

// baseConfig returns a minimal go:fiber + db:postgres project used across
// this package's tests.
func baseConfig() *config.Config {
	return &config.Config{
		Project: "p",
		Version: "1",
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
				"db":  {Type: config.NodeTypeInfra, Adapter: "db:postgres", Version: "16"},
			},
			Edges: []config.EdgeConfig{
				{From: "api", To: "db", Type: config.EdgeDependsOn},
			},
		},
	}
}

// loadThroughRealKernel drives the plugin's Register method through the
// real KernelAPIImpl, the same seam cmd/acthur uses in production —
// no plugin-kernel mocks, per the Phase 6 shared conventions.
func loadThroughRealKernel(t *testing.T, cfg *config.Config) (*graph.Graph, *plugin.KernelAPIImpl) {
	t.Helper()
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, g, func(plugin.CLICommand) {}, nil)
	if _, err := plugin.Load([]string{migrations.Name}, bus, k); err != nil {
		t.Fatalf("load migrations plugin: %v", err)
	}
	return g, k
}

func TestMigrationsPlugin_RegistersGeneratorNamedAfterPlugin(t *testing.T) {
	_, k := loadThroughRealKernel(t, baseConfig())

	gen, ok := k.Generator(migrations.Name)
	if !ok {
		t.Fatal("expected generator \"migrations\" to be registered")
	}
	adapters := gen.SupportedAdapters()
	if len(adapters) != 1 || adapters[0] != "*" {
		t.Errorf("expected SupportedAdapters() == [\"*\"], got %v", adapters)
	}
}

func TestMigrationsPlugin_Generate_EmitsKeepAndInitPair(t *testing.T) {
	_, k := loadThroughRealKernel(t, baseConfig())
	gen, ok := k.Generator(migrations.Name)
	if !ok {
		t.Fatal("expected generator to be registered")
	}

	files, err := gen.Generate("go:fiber", plugin.GeneratorContext{
		ProjectName: "p",
		NodeID:      "api",
		RootDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	want := map[string]bool{
		"migrations/.keep":              false,
		"migrations/0001_init.up.sql":   false,
		"migrations/0001_init.down.sql": false,
	}
	for _, f := range files {
		if _, ok := want[f.Path]; !ok {
			t.Errorf("unexpected generated file path %q", f.Path)
			continue
		}
		want[f.Path] = true
		if f.Overwrite {
			t.Errorf("file %q: expected Overwrite=false (seed files must not clobber user edits)", f.Path)
		}
	}
	for path, seen := range want {
		if !seen {
			t.Errorf("expected generated file %q, not found", path)
		}
	}
}

func TestDatabaseURL_FindsPostgresNodeAndDerivesConnectionEnv(t *testing.T) {
	g, err := graph.Build(baseConfig())
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}

	url, err := migrations.DatabaseURL(g)
	if err != nil {
		t.Fatalf("DatabaseURL: %v", err)
	}

	// Cross-check against the postgres adapter's own ConnectionEnv, the same
	// logic the dev engine uses (internal/engine/dev.go resolveNodeEnv) —
	// DatabaseURL must never invent its own derivation.
	a, err := adapter.Resolve("db:postgres")
	if err != nil {
		t.Fatalf("resolve db:postgres adapter: %v", err)
	}
	connectable := a.(adapter.Connectable)
	want := connectable.ConnectionEnv(adapter.ContainerContext{NodeID: "db", Version: "16"})["DATABASE_URL"]

	if url != want {
		t.Errorf("DatabaseURL() = %q, want %q (adapter ConnectionEnv output)", url, want)
	}
}

func TestDatabaseURL_NoPostgresNode_ReturnsPointedError(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Version: "1",
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}

	_, err = migrations.DatabaseURL(g)
	if err == nil {
		t.Fatal("expected an error when the graph has no db:postgres node")
	}
	if err != migrations.ErrNoPostgresNode {
		t.Errorf("expected ErrNoPostgresNode, got: %v", err)
	}
}

func TestCreateNext_WritesNumberedEmptyPairSequentially(t *testing.T) {
	root := t.TempDir()

	up1, down1, err := migrations.CreateNext(root, "add users table")
	if err != nil {
		t.Fatalf("CreateNext: %v", err)
	}
	if filepath.Base(up1) != "0001_add_users_table.up.sql" {
		t.Errorf("expected first migration numbered 0001, got %q", filepath.Base(up1))
	}
	if filepath.Base(down1) != "0001_add_users_table.down.sql" {
		t.Errorf("expected first down migration numbered 0001, got %q", filepath.Base(down1))
	}
	for _, p := range []string{up1, down1} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}

	up2, _, err := migrations.CreateNext(root, "add index")
	if err != nil {
		t.Fatalf("CreateNext (second): %v", err)
	}
	if filepath.Base(up2) != "0002_add_index.up.sql" {
		t.Errorf("expected second migration numbered 0002, got %q", filepath.Base(up2))
	}
}

func TestCreateNext_ContinuesFromExistingHighestNumber(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "migrations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate the migrations plugin's own 0001_init pair already present.
	if err := os.WriteFile(filepath.Join(dir, "0001_init.up.sql"), []byte("--\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0001_init.down.sql"), []byte("--\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	up, _, err := migrations.CreateNext(root, "add posts")
	if err != nil {
		t.Fatalf("CreateNext: %v", err)
	}
	if filepath.Base(up) != "0002_add_posts.up.sql" {
		t.Errorf("expected next migration to continue from 0001, got %q", filepath.Base(up))
	}
}
