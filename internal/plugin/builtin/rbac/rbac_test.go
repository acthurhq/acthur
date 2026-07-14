package rbac_test

import (
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/plugin"
	_ "github.com/acthurhq/acthur/internal/plugin/builtin/rbac"
)

// stubAuthPlugin stands in for the real "auth" plugin, which is being built
// in a parallel worktree slice (#44). rbac.DependsOn() requires "auth" to be
// present in the plugin registry for plugin.Load's topological resolver to
// succeed; this fixture satisfies that without importing the real auth
// package or any of its generated code.
type stubAuthPlugin struct{}

func (stubAuthPlugin) Name() string        { return "auth" }
func (stubAuthPlugin) Version() string     { return "0.0.0-stub" }
func (stubAuthPlugin) DependsOn() []string { return nil }
func (stubAuthPlugin) Register(k plugin.KernelAPI) error {
	k.RegisterGenerator("auth", stubAuthGenerator{})
	return nil
}

type stubAuthGenerator struct{}

func (stubAuthGenerator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	return nil, nil
}
func (stubAuthGenerator) SupportedAdapters() []string { return []string{"go:fiber"} }

func registerStubAuth(t *testing.T) {
	t.Helper()
	plugin.Unregister("auth")
	plugin.Register(stubAuthPlugin{})
	t.Cleanup(func() { plugin.Unregister("auth") })
}

func TestRBACPlugin_DependsOnAuth(t *testing.T) {
	p, err := plugin.Resolve("rbac")
	if err != nil {
		t.Fatalf("resolve rbac plugin: %v", err)
	}
	deps := p.DependsOn()
	if len(deps) != 1 || deps[0] != "auth" {
		t.Fatalf("expected DependsOn() == [\"auth\"], got %v", deps)
	}
}

// TestRBACPlugin_LoadOrder_AuthBeforeRBAC drives rbac through the real
// plugin.Load path (no plugin-kernel mocks): the loader must resolve
// DependsOn into a topological order that loads "auth" before "rbac", even
// when the input names list is given in the opposite order.
func TestRBACPlugin_LoadOrder_AuthBeforeRBAC(t *testing.T) {
	registerStubAuth(t)

	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, nil, nil, nil)

	loaded, err := plugin.Load([]string{"rbac", "auth"}, bus, k)
	if err != nil {
		t.Fatalf("plugin.Load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded plugins, got %d", len(loaded))
	}
	if loaded[0].Plugin.Name() != "auth" || loaded[1].Plugin.Name() != "rbac" {
		t.Fatalf("expected load order [auth rbac], got [%s %s]",
			loaded[0].Plugin.Name(), loaded[1].Plugin.Name())
	}
}

// TestRBACPlugin_LoadWithoutAuth_FailsCleanly asserts the loader's existing
// DependsOn validation surfaces a pointed error when "auth" is missing from
// the requested plugin list — rbac does not special-case this itself.
func TestRBACPlugin_LoadWithoutAuth_FailsCleanly(t *testing.T) {
	registerStubAuth(t)

	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, nil, nil, nil)

	_, err := plugin.Load([]string{"rbac"}, bus, k)
	if err == nil {
		t.Fatal("expected error loading rbac without auth in the plugin list")
	}
	if !strings.Contains(err.Error(), "auth") {
		t.Errorf("expected error to mention missing dependency %q, got: %v", "auth", err)
	}
}

func TestRBACGenerator_SupportedAdapters(t *testing.T) {
	gen := generatorFromRegistry(t)
	adapters := gen.SupportedAdapters()
	if len(adapters) != 1 || adapters[0] != "go:fiber" {
		t.Fatalf("expected SupportedAdapters() == [\"go:fiber\"], got %v", adapters)
	}
}

func TestRBACGenerator_UnsupportedAdapter_Errors(t *testing.T) {
	gen := generatorFromRegistry(t)
	_, err := gen.Generate("rust:axum", plugin.GeneratorContext{})
	if err == nil {
		t.Fatal("expected error generating for an unsupported adapter")
	}
}

// TestRBACGenerator_EmitsExpectedFiles asserts the emitted file set (models,
// policy, middleware, seed helper under internal/rbac/, plus migrations
// 0200-0202 at project root) and parses every emitted .go file with
// go/parser to confirm it is syntactically valid Go.
func TestRBACGenerator_EmitsExpectedFiles(t *testing.T) {
	gen := generatorFromRegistry(t)

	ctx := plugin.GeneratorContext{
		ProjectName: "acme",
		NodeID:      "api",
		RootDir:     "/tmp/acme",
		Extra:       map[string]any{"module_path": "github.com/acme/api"},
	}

	files, err := gen.Generate("go:fiber", ctx)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got := make([]string, len(files))
	byPath := make(map[string]plugin.GeneratedFile, len(files))
	for i, f := range files {
		got[i] = f.Path
		byPath[f.Path] = f
	}
	sort.Strings(got)

	want := []string{
		"internal/rbac/middleware.go",
		"internal/rbac/models.go",
		"internal/rbac/policy.go",
		"internal/rbac/seed.go",
		"migrations/0200_roles.down.sql",
		"migrations/0200_roles.up.sql",
		"migrations/0201_permissions.down.sql",
		"migrations/0201_permissions.up.sql",
		"migrations/0202_user_roles.down.sql",
		"migrations/0202_user_roles.up.sql",
	}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("expected %d files, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected file set %v, got %v", want, got)
		}
	}

	// Every .go file must parse as valid Go.
	fset := token.NewFileSet()
	for path, f := range byPath {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if _, err := parser.ParseFile(fset, path, f.Content, parser.AllErrors); err != nil {
			t.Errorf("emitted file %s does not parse: %v\n---\n%s", path, err, f.Content)
		}
	}

	// The seed helper must import the ids package under the node's resolved
	// module path, not a hardcoded placeholder.
	seed := string(byPath["internal/rbac/seed.go"].Content)
	if !strings.Contains(seed, `"github.com/acme/api/internal/ids"`) {
		t.Errorf("expected seed.go to import the node's ids package, got:\n%s", seed)
	}

	// Migrations reference their tables and are non-empty.
	rolesUp := string(byPath["migrations/0200_roles.up.sql"].Content)
	if !strings.Contains(rolesUp, "CREATE TABLE") || !strings.Contains(rolesUp, "roles") {
		t.Errorf("expected 0200_roles.up.sql to create the roles table, got:\n%s", rolesUp)
	}
	permsUp := string(byPath["migrations/0201_permissions.up.sql"].Content)
	if !strings.Contains(permsUp, "permissions") || !strings.Contains(permsUp, "role_permissions") {
		t.Errorf("expected 0201_permissions.up.sql to create permissions + role_permissions, got:\n%s", permsUp)
	}
	userRolesUp := string(byPath["migrations/0202_user_roles.up.sql"].Content)
	if !strings.Contains(userRolesUp, "user_roles") {
		t.Errorf("expected 0202_user_roles.up.sql to create user_roles, got:\n%s", userRolesUp)
	}

	// Migration files must not be overwritten once applied.
	for _, path := range want {
		if strings.HasPrefix(path, "migrations/") && byPath[path].Overwrite {
			t.Errorf("expected migration file %s to have Overwrite=false", path)
		}
	}
}

// TestRBACGenerator_ModulePathFallback: when ctx.Extra carries no
// "module_path" (a generator engine bug, or a caller that forgot to set it),
// the generator must still emit valid, parseable Go rather than panicking or
// emitting a broken import path.
func TestRBACGenerator_ModulePathFallback(t *testing.T) {
	gen := generatorFromRegistry(t)

	ctx := plugin.GeneratorContext{ProjectName: "acme", NodeID: "api", RootDir: "/tmp/acme"}
	files, err := gen.Generate("go:fiber", ctx)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	fset := token.NewFileSet()
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".go") {
			continue
		}
		if _, err := parser.ParseFile(fset, f.Path, f.Content, parser.AllErrors); err != nil {
			t.Errorf("emitted file %s does not parse: %v\n---\n%s", f.Path, err, f.Content)
		}
	}
}

// generatorFromRegistry loads rbac (with its auth dependency stubbed) through
// the real KernelAPI and retrieves its registered generator — the same path
// the real CLI's `acthur add rbac` uses.
func generatorFromRegistry(t *testing.T) plugin.Generator {
	t.Helper()
	registerStubAuth(t)

	bus := plugin.NewBus()
	k := plugin.NewKernelAPI(bus, nil, nil, nil)

	if _, err := plugin.Load([]string{"auth", "rbac"}, bus, k); err != nil {
		t.Fatalf("plugin.Load: %v", err)
	}

	gen, ok := k.Generator("rbac")
	if !ok {
		t.Fatal("expected rbac generator to be registered after Load")
	}
	return gen
}
