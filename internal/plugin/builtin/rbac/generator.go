package rbac

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"github.com/acthurhq/acthur/internal/plugin"
)

//go:embed templates/models.go.tmpl
var tmplModels []byte

//go:embed templates/policy.go.tmpl
var tmplPolicy []byte

//go:embed templates/middleware.go.tmpl
var tmplMiddleware []byte

//go:embed templates/seed.go.tmpl
var tmplSeed []byte

// generator is the rbac plugin's Generator, registered under the target name
// "rbac" (per the Phase 6 shared convention: one generator, named after the
// plugin). It emits internal/rbac/ for go:fiber projects plus migrations
// 0200-0202 at the project root.
type generator struct{}

// SupportedAdapters reports that rbac only generates for go:fiber (adapter
// support for generator targets becomes a modeled capability once the
// generator engine lands — Phase 7).
func (generator) SupportedAdapters() []string { return []string{"go:fiber"} }

// Generate renders internal/rbac/{models,policy,middleware,seed}.go and the
// 0200-0202 migration pairs. Paths are relative to the target node's
// directory, except the migrations/ files, which land at the project root
// per the shared convention.
func (generator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	if adapterName != "go:fiber" {
		return nil, fmt.Errorf("rbac generator: unsupported adapter %q (supports go:fiber)", adapterName)
	}

	data := templateData{ModulePath: resolveModulePath(ctx)}

	models, err := render("models.go.tmpl", tmplModels, data)
	if err != nil {
		return nil, fmt.Errorf("internal/rbac/models.go: %w", err)
	}
	policy, err := render("policy.go.tmpl", tmplPolicy, data)
	if err != nil {
		return nil, fmt.Errorf("internal/rbac/policy.go: %w", err)
	}
	middleware, err := render("middleware.go.tmpl", tmplMiddleware, data)
	if err != nil {
		return nil, fmt.Errorf("internal/rbac/middleware.go: %w", err)
	}
	seed, err := render("seed.go.tmpl", tmplSeed, data)
	if err != nil {
		return nil, fmt.Errorf("internal/rbac/seed.go: %w", err)
	}

	return []plugin.GeneratedFile{
		{Path: "internal/rbac/models.go", Content: models, Mode: 0o644, Overwrite: true},
		{Path: "internal/rbac/policy.go", Content: policy, Mode: 0o644, Overwrite: true},
		{Path: "internal/rbac/middleware.go", Content: middleware, Mode: 0o644, Overwrite: true},
		{Path: "internal/rbac/seed.go", Content: seed, Mode: 0o644, Overwrite: true},

		// Migrations own range 0200-0299 (shared convention). Overwrite=false:
		// once a migration has run, its file must not be silently rewritten.
		{Path: "migrations/0200_roles.up.sql", Content: []byte(migRolesUp), Mode: 0o644, Overwrite: false},
		{Path: "migrations/0200_roles.down.sql", Content: []byte(migRolesDown), Mode: 0o644, Overwrite: false},
		{Path: "migrations/0201_permissions.up.sql", Content: []byte(migPermissionsUp), Mode: 0o644, Overwrite: false},
		{Path: "migrations/0201_permissions.down.sql", Content: []byte(migPermissionsDown), Mode: 0o644, Overwrite: false},
		{Path: "migrations/0202_user_roles.up.sql", Content: []byte(migUserRolesUp), Mode: 0o644, Overwrite: false},
		{Path: "migrations/0202_user_roles.down.sql", Content: []byte(migUserRolesDown), Mode: 0o644, Overwrite: false},
	}, nil
}

// templateData is the data set every rbac template renders against.
type templateData struct {
	// ModulePath is the target node's fully-resolved Go module path (e.g.
	// "github.com/acme/api"), used to import its sibling internal/ids
	// package for ID generation in seed.go.
	ModulePath string
}

// resolveModulePath reads ctx.Extra["module_path"] per the shared convention.
// Falls back to "<ProjectName>/<NodeID>" (the same formula the gofiber
// adapter's scaffold uses to compute ModulePath) if it is absent, so
// generation never panics or emits an unparseable import path.
func resolveModulePath(ctx plugin.GeneratorContext) string {
	if v, ok := ctx.Extra["module_path"].(string); ok && v != "" {
		return v
	}
	return ctx.ProjectName + "/" + ctx.NodeID
}

// render executes the named template against data.
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

// ---------------------------------------------------------------------------
// Migrations 0200-0202 (rbac's slice of the shared range 0200-0299)
// ---------------------------------------------------------------------------

const migRolesUp = `CREATE TABLE IF NOT EXISTS roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

const migRolesDown = `DROP TABLE IF EXISTS roles;
`

const migPermissionsUp = `CREATE TABLE IF NOT EXISTS permissions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id TEXT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);
`

const migPermissionsDown = `DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
`

// user_roles.user_id intentionally carries no foreign key: the users table
// is owned by the "auth" plugin's own migration range (0100-0199), and rbac
// must not depend on auth's schema landing first beyond the plugin load
// order enforced by DependsOn (see rbac.go).
const migUserRolesUp = `CREATE TABLE IF NOT EXISTS user_roles (
    user_id TEXT NOT NULL,
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);
`

const migUserRolesDown = `DROP TABLE IF EXISTS user_roles;
`
