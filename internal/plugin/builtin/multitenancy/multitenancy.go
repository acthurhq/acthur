// Package multitenancy is the built-in "multitenancy" plugin (Phase 6,
// Slice 4): schema-per-tenant multitenancy for go:fiber projects backed by
// Postgres. Its generator emits a tenant model, a resolver middleware
// (X-Tenant-ID header first, subdomain fallback), a SchemaSwitch helper
// that sets search_path per request, a pool tuned with BeforeAcquire/
// AfterRelease hooks, and a provisioner that creates a tenant's schema and
// runs migrations into it — plus the migrations/0300_tenants pair.
package multitenancy

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"github.com/acthur/acthur/internal/plugin"
)

//go:embed templates/tenant.go.tmpl
var tmplTenant []byte

//go:embed templates/store.go.tmpl
var tmplStore []byte

//go:embed templates/resolver.go.tmpl
var tmplResolver []byte

//go:embed templates/schema.go.tmpl
var tmplSchema []byte

//go:embed templates/pool.go.tmpl
var tmplPool []byte

//go:embed templates/provisioner.go.tmpl
var tmplProvisioner []byte

//go:embed templates/0300_tenants.up.sql
var migrationUp []byte

//go:embed templates/0300_tenants.down.sql
var migrationDown []byte

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

type multitenancyPlugin struct{}

func (multitenancyPlugin) Name() string        { return "multitenancy" }
func (multitenancyPlugin) Version() string     { return "0.1.0" }
func (multitenancyPlugin) DependsOn() []string { return nil }

func (multitenancyPlugin) Register(k plugin.KernelAPI) error {
	k.RegisterGenerator("multitenancy", generator{})
	return nil
}

func init() {
	plugin.Register(multitenancyPlugin{})
}

// ---------------------------------------------------------------------------
// Generator
// ---------------------------------------------------------------------------

type generator struct{}

func (generator) SupportedAdapters() []string { return []string{"go:fiber"} }

// templateData is the data every generated Go file template renders
// against. Only provisioner.go.tmpl currently uses ModulePath (to import
// the target module's internal/ids package), but all files render through
// the same data type for consistency.
type templateData struct {
	ModulePath string
}

// Generate emits internal/tenant/ for a go:fiber node plus the
// migrations/0300_tenants pair. Migration files are project-root relative
// (the "migrations/" prefix); everything else is relative to the target
// node's directory, per the shared plugin convention.
func (generator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	if adapterName != "go:fiber" {
		return nil, fmt.Errorf("multitenancy: adapter %q is not supported (supported: go:fiber)", adapterName)
	}

	modulePath, _ := ctx.Extra["module_path"].(string)
	if modulePath == "" {
		return nil, fmt.Errorf("multitenancy: ctx.Extra[%q] is required to import the target module's internal/ids package", "module_path")
	}
	data := templateData{ModulePath: modulePath}

	tenantGo, err := render("tenant.go", tmplTenant, data)
	if err != nil {
		return nil, fmt.Errorf("multitenancy: render tenant.go: %w", err)
	}
	storeGo, err := render("store.go", tmplStore, data)
	if err != nil {
		return nil, fmt.Errorf("multitenancy: render store.go: %w", err)
	}
	resolverGo, err := render("resolver.go", tmplResolver, data)
	if err != nil {
		return nil, fmt.Errorf("multitenancy: render resolver.go: %w", err)
	}
	schemaGo, err := render("schema.go", tmplSchema, data)
	if err != nil {
		return nil, fmt.Errorf("multitenancy: render schema.go: %w", err)
	}
	poolGo, err := render("pool.go", tmplPool, data)
	if err != nil {
		return nil, fmt.Errorf("multitenancy: render pool.go: %w", err)
	}
	provisionerGo, err := render("provisioner.go", tmplProvisioner, data)
	if err != nil {
		return nil, fmt.Errorf("multitenancy: render provisioner.go: %w", err)
	}

	return []plugin.GeneratedFile{
		// internal/tenant/*: regenerable scaffolding, safe to overwrite.
		{Path: "internal/tenant/tenant.go", Content: tenantGo, Overwrite: true},
		{Path: "internal/tenant/store.go", Content: storeGo, Overwrite: true},
		{Path: "internal/tenant/resolver.go", Content: resolverGo, Overwrite: true},
		{Path: "internal/tenant/schema.go", Content: schemaGo, Overwrite: true},
		{Path: "internal/tenant/pool.go", Content: poolGo, Overwrite: true},
		{Path: "internal/tenant/provisioner.go", Content: provisionerGo, Overwrite: true},
		// migrations/*: historical record, never clobbered once written.
		{Path: "migrations/0300_tenants.up.sql", Content: migrationUp, Overwrite: false},
		{Path: "migrations/0300_tenants.down.sql", Content: migrationDown, Overwrite: false},
	}, nil
}

func render(name string, tmplBytes []byte, data any) ([]byte, error) {
	t, err := template.New(name).Parse(string(tmplBytes))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var _ plugin.Generator = generator{}
