package multitenancy

import (
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/graph"
	"github.com/acthur/acthur/internal/plugin"
)

// buildKernelAPI drives the plugin through the real KernelAPIImpl load path:
// a real graph, a real bus, no plugin-kernel mocks.
func buildKernelAPI(t *testing.T) (*plugin.KernelAPIImpl, *graph.Graph) {
	t.Helper()
	cfg := &config.Config{
		Project: "acme",
		Graph: config.GraphConfig{
			Nodes: map[string]config.NodeConfig{
				"api": {Type: config.NodeTypeService, Adapter: "go:fiber", Port: 8080},
				"db":  {Type: config.NodeTypeInfra, Adapter: "db:postgres"},
			},
			Edges: []config.EdgeConfig{
				{From: "api", To: "db", Type: config.EdgeDependsOn},
			},
		},
	}
	g, err := graph.Build(cfg)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, g, nil, nil)
	return k, g
}

func registerPlugin(t *testing.T, k plugin.KernelAPI) {
	t.Helper()
	p := multitenancyPlugin{}
	if err := p.Register(k); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestRegister_RegistersGeneratorNamedAfterPlugin(t *testing.T) {
	k, _ := buildKernelAPI(t)
	registerPlugin(t, k)

	if _, ok := k.Generator("multitenancy"); !ok {
		t.Fatal("expected a generator registered under \"multitenancy\"")
	}
}

func TestGenerate_UnsupportedAdapter_Errors(t *testing.T) {
	k, _ := buildKernelAPI(t)
	registerPlugin(t, k)
	gen, _ := k.Generator("multitenancy")

	_, err := gen.Generate("rust:axum", plugin.GeneratorContext{
		Extra: map[string]any{"module_path": "github.com/acme/api"},
	})
	if err == nil {
		t.Fatal("expected an error generating for an unsupported adapter")
	}
}

func TestGenerate_GoFiber_EmitsExpectedFileSet(t *testing.T) {
	k, _ := buildKernelAPI(t)
	registerPlugin(t, k)
	gen, ok := k.Generator("multitenancy")
	if !ok {
		t.Fatal("expected multitenancy generator to be registered")
	}

	files, err := gen.Generate("go:fiber", plugin.GeneratorContext{
		ProjectName: "acme",
		NodeID:      "api",
		Extra:       map[string]any{"module_path": "github.com/acme/api"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	sort.Strings(paths)

	want := []string{
		"internal/tenant/pool.go",
		"internal/tenant/provisioner.go",
		"internal/tenant/resolver.go",
		"internal/tenant/schema.go",
		"internal/tenant/store.go",
		"internal/tenant/tenant.go",
		"migrations/0300_tenants.down.sql",
		"migrations/0300_tenants.up.sql",
	}
	sort.Strings(want)

	if len(paths) != len(want) {
		t.Fatalf("expected %d files, got %d: %v", len(want), len(paths), paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("file set mismatch: want %v, got %v", want, paths)
			break
		}
	}
}

func TestGenerate_MissingModulePath_Errors(t *testing.T) {
	k, _ := buildKernelAPI(t)
	registerPlugin(t, k)
	gen, _ := k.Generator("multitenancy")

	_, err := gen.Generate("go:fiber", plugin.GeneratorContext{})
	if err == nil {
		t.Fatal("expected an error when ctx.Extra[\"module_path\"] is missing")
	}
}

// fileByPath is a test helper to fetch generated content by path.
func fileByPath(t *testing.T, files []plugin.GeneratedFile, path string) plugin.GeneratedFile {
	t.Helper()
	for _, f := range files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("expected generated file %q", path)
	return plugin.GeneratedFile{}
}

func generate(t *testing.T) []plugin.GeneratedFile {
	t.Helper()
	k, _ := buildKernelAPI(t)
	registerPlugin(t, k)
	gen, _ := k.Generator("multitenancy")
	files, err := gen.Generate("go:fiber", plugin.GeneratorContext{
		ProjectName: "acme",
		NodeID:      "api",
		Extra:       map[string]any{"module_path": "github.com/acme/api"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return files
}

func TestGenerate_EveryGoFileParses(t *testing.T) {
	files := generate(t)
	fset := token.NewFileSet()
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".go") {
			continue
		}
		if _, err := parser.ParseFile(fset, f.Path, f.Content, parser.AllErrors); err != nil {
			t.Errorf("%s: does not parse: %v\n---\n%s", f.Path, err, f.Content)
		}
	}
}

func TestGenerate_ResolverMiddleware_HeaderFirstSubdomainFallback400(t *testing.T) {
	files := generate(t)
	content := string(fileByPath(t, files, "internal/tenant/resolver.go").Content)

	for _, want := range []string{"X-Tenant-ID", "subdomain", "StatusBadRequest"} {
		if !strings.Contains(content, want) {
			t.Errorf("resolver.go: expected to reference %q, got:\n%s", want, content)
		}
	}
}

func TestGenerate_SchemaSwitch_SetsSearchPath(t *testing.T) {
	files := generate(t)
	content := string(fileByPath(t, files, "internal/tenant/schema.go").Content)

	if !strings.Contains(content, "search_path") {
		t.Errorf("schema.go: expected search_path SET statement, got:\n%s", content)
	}
	if !strings.Contains(content, "func SchemaSwitch(") {
		t.Errorf("schema.go: expected an exported SchemaSwitch function, got:\n%s", content)
	}
}

func TestGenerate_Pool_TunesBeforeAcquireAndAfterRelease(t *testing.T) {
	files := generate(t)
	content := string(fileByPath(t, files, "internal/tenant/pool.go").Content)

	if !strings.Contains(content, "BeforeAcquire") || !strings.Contains(content, "Ping") {
		t.Errorf("pool.go: expected BeforeAcquire ping hook, got:\n%s", content)
	}
	if !strings.Contains(content, "AfterRelease") || !strings.Contains(content, "search_path") {
		t.Errorf("pool.go: expected AfterRelease search_path reset hook, got:\n%s", content)
	}
}

func TestGenerate_Provisioner_CreatesSchemaAndUsesModulePath(t *testing.T) {
	files := generate(t)
	content := string(fileByPath(t, files, "internal/tenant/provisioner.go").Content)

	if !strings.Contains(content, "CREATE SCHEMA") {
		t.Errorf("provisioner.go: expected a CREATE SCHEMA statement, got:\n%s", content)
	}
	if !strings.Contains(content, "github.com/acme/api/internal/ids") {
		t.Errorf("provisioner.go: expected the module path threaded into the ids import, got:\n%s", content)
	}
}

func TestGenerate_Migrations_TenantsTable(t *testing.T) {
	files := generate(t)
	up := string(fileByPath(t, files, "migrations/0300_tenants.up.sql").Content)
	down := string(fileByPath(t, files, "migrations/0300_tenants.down.sql").Content)

	for _, want := range []string{"CREATE TABLE", "tenants", "slug", "UNIQUE", "schema_name", "created_at"} {
		if !strings.Contains(up, want) {
			t.Errorf("0300_tenants.up.sql: expected %q, got:\n%s", want, up)
		}
	}
	if !strings.Contains(down, "DROP TABLE") {
		t.Errorf("0300_tenants.down.sql: expected DROP TABLE, got:\n%s", down)
	}
}

func TestGenerate_MigrationsAreNotOverwritable(t *testing.T) {
	files := generate(t)
	for _, path := range []string{"migrations/0300_tenants.up.sql", "migrations/0300_tenants.down.sql"} {
		f := fileByPath(t, files, path)
		if f.Overwrite {
			t.Errorf("%s: expected Overwrite=false to protect migration history", path)
		}
	}
}

func TestPlugin_NameVersionDependsOn(t *testing.T) {
	p := multitenancyPlugin{}
	if p.Name() != "multitenancy" {
		t.Errorf("expected plugin name \"multitenancy\", got %q", p.Name())
	}
	if p.Version() == "" {
		t.Error("expected a non-empty version")
	}
	if len(p.DependsOn()) != 0 {
		t.Errorf("expected multitenancy to have no dependencies, got %v", p.DependsOn())
	}
}
