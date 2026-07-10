// Package auth is Acthur's built-in "auth" plugin (Phase 6 Slice 2). For
// The go:fiber service nodes it generates a complete authentication package
// under internal/auth/: JWT issue/verify (HS256, secret from
// AUTH_JWT_SECRET), a DB-backed session store, a magic-link token flow, an
// OAuth2 provider registry, register/login/logout/me route handlers
// mountable via one auth.Mount(app, db) call, and a RequireAuth fiber
// middleware — plus the users/sessions migrations those depend on.
//
// Third-party dependencies (golang-jwt/jwt/v5, jackc/pgx/v5,
// golang.org/x/crypto/bcrypt) reach the target go.mod without a
// MergeMarker: golang-jwt and pgx are already direct requires in the
// The go:fiber adapter's scaffolded go.mod (internal/adapter/backend/gofiber/
// templates/go.mod.tmpl), and bcrypt is a transitive import `go mod tidy`
// resolves and adds on its own. See docs/implementation/active/
// 0004-phase-6-first-plugins.md for the recorded decision.
package auth

import (
	"bytes"
	"embed"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/acthurhq/acthur/internal/plugin"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

var goTemplates = template.Must(template.ParseFS(templatesFS, "templates/*.tmpl"))

// goFileOrder pins template execution (and therefore GeneratedFile) order
// so output is deterministic across runs.
var goFileOrder = []string{
	"models.go.tmpl",
	"password.go.tmpl",
	"jwt.go.tmpl",
	"session.go.tmpl",
	"magiclink.go.tmpl",
	"providers.go.tmpl",
	"middleware.go.tmpl",
	"handlers.go.tmpl",
}

// migrationFiles are the static migration pairs this plugin owns, in the
// 0100-0199 range reserved for auth (see the Phase 6 PRD's shared
// conventions).
var migrationFiles = []string{
	"0100_users.up.sql",
	"0100_users.down.sql",
	"0101_sessions.up.sql",
	"0101_sessions.down.sql",
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

type authPlugin struct{}

func (authPlugin) Name() string        { return "auth" }
func (authPlugin) Version() string     { return "0.1.0" }
func (authPlugin) DependsOn() []string { return nil }

func (authPlugin) Register(k plugin.KernelAPI) error {
	k.RegisterGenerator("auth", generator{})
	return nil
}

func init() {
	plugin.Register(authPlugin{})
}

// ---------------------------------------------------------------------------
// Generator
// ---------------------------------------------------------------------------

type generator struct{}

// templateData is the value passed to every internal/auth/*.go.tmpl template.
type templateData struct {
	ModulePath string
	Strategy   string
	Providers  []string
}

func (generator) SupportedAdapters() []string { return []string{"go:fiber"} }

// Generate renders internal/auth/*.go from templates and attaches the
// users/sessions migrations this plugin owns. Config keys (from the
// plugins: entry in acthur.yml): strategy (default "jwt"), providers
// (list of OAuth2 provider names to pre-register, empty by default).
func (generator) Generate(adapterName string, ctx plugin.GeneratorContext) ([]plugin.GeneratedFile, error) {
	if adapterName != "go:fiber" {
		return nil, fmt.Errorf("auth generator does not support adapter %q (supported: go:fiber)", adapterName)
	}

	data, err := buildTemplateData(ctx)
	if err != nil {
		return nil, err
	}

	var files []plugin.GeneratedFile
	for _, tmplName := range goFileOrder {
		var buf bytes.Buffer
		if err := goTemplates.ExecuteTemplate(&buf, tmplName, data); err != nil {
			return nil, fmt.Errorf("rendering %s: %w", tmplName, err)
		}
		files = append(files, plugin.GeneratedFile{
			Path:      "internal/auth/" + strings.TrimSuffix(tmplName, ".tmpl"),
			Content:   buf.Bytes(),
			Mode:      0o644,
			Overwrite: false,
		})
	}

	for _, name := range migrationFiles {
		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, fmt.Errorf("reading embedded migration %s: %w", name, err)
		}
		files = append(files, plugin.GeneratedFile{
			Path:      "migrations/" + name,
			Content:   content,
			Mode:      0o644,
			Overwrite: false,
		})
	}

	return files, nil
}

// buildTemplateData reads module_path (required), strategy, and providers
// out of a GeneratorContext.
func buildTemplateData(ctx plugin.GeneratorContext) (templateData, error) {
	modulePath, _ := ctx.Extra["module_path"].(string)
	if modulePath == "" {
		return templateData{}, fmt.Errorf(`auth generator requires ctx.Extra["module_path"] to be set`)
	}

	strategy := "jwt"
	if ctx.Config != nil {
		if s, ok := ctx.Config["strategy"].(string); ok && s != "" {
			strategy = s
		}
	}

	var providers []string
	if ctx.Config != nil {
		switch v := ctx.Config["providers"].(type) {
		case []string:
			providers = append(providers, v...)
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					providers = append(providers, s)
				}
			}
		}
	}
	sort.Strings(providers)

	return templateData{ModulePath: modulePath, Strategy: strategy, Providers: providers}, nil
}
