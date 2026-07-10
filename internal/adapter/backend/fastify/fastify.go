// Package fastify implements the node:fastify adapter.
// It knows how to scaffold, start, build, test, and containerize
// a Node Fastify application. It has zero knowledge of plugins or contracts.
package fastify

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/acthurhq/acthur/internal/adapter"
)

//go:embed templates/package.json.tmpl
var tmplPackageJSON []byte

//go:embed templates/index.js.tmpl
var tmplIndexJS []byte

//go:embed templates/routes.js.tmpl
var tmplRoutesJS []byte

//go:embed templates/health.js.tmpl
var tmplHealthJS []byte

//go:embed templates/config.js.tmpl
var tmplConfigJS []byte

//go:embed templates/errors.js.tmpl
var tmplErrorsJS []byte

//go:embed templates/ids_ulid.js.tmpl
var tmplIDsULID []byte

//go:embed templates/ids_uuid.js.tmpl
var tmplIDsUUID []byte

//go:embed templates/env.example.tmpl
var tmplEnvExample []byte

//go:embed templates/gitignore.tmpl
var tmplGitignore []byte

//go:embed templates/dockerfile.tmpl
var tmplDockerfile []byte

//go:embed templates/dockerfile.prod.tmpl
var tmplDockerfileProd []byte

// Adapter implements adapter.Adapter for Node Fastify.
type Adapter struct{}

func init() {
	adapter.Register(&Adapter{})
}

func (a *Adapter) Name() string               { return "node:fastify" }
func (a *Adapter) Category() adapter.Category { return adapter.CategoryBackend }

// Detect returns true if a package.json containing fastify is found in dir.
func (a *Adapter) Detect(dir string) bool {
	pkgJSON := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgJSON)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), `"fastify"`)
}

// DevCommand returns the node --watch command for hot reload.
func (a *Adapter) DevCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "node",
		Args: []string{"--watch", "src/index.js"},
		Env:  env,
	}
}

// SelfReloads reports true: node --watch restarts the process itself on
// file change. The dev engine's file watcher must not also restart this
// node's process — that would just race node's own restart for the same
// port.
func (a *Adapter) SelfReloads() bool { return true }

// BuildCommand installs production dependencies deterministically
// (package-lock.json-pinned) ahead of `acthur build`/`acthur deploy`.
// Plain JS has no compile step; this is the closest equivalent to
// The go:fiber's `go build` or rust:axum's `cargo build --release`.
func (a *Adapter) BuildCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "npm",
		Args: []string{"ci", "--omit=dev"},
		Env:  env,
	}
}

// TestCommand returns the npm test command.
func (a *Adapter) TestCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "npm",
		Args: []string{"test"},
		Env:  env,
	}
}

// EnvVars returns the environment variables a Fastify app requires.
func (a *Adapter) EnvVars() []adapter.EnvVar {
	return []adapter.EnvVar{
		{Key: "APP_ENV", Description: "application environment", Required: true, Default: "development"},
		{Key: "APP_PORT", Description: "HTTP port", Required: true, Default: "8080"},
		{Key: "APP_SECRET", Description: "application secret key", Required: true, Secret: true, Generate: true},
		{Key: "DATABASE_URL", Description: "PostgreSQL connection string", Required: true, Secret: true},
		{Key: "REDIS_URL", Description: "Redis connection string", Required: false, Default: "redis://localhost:6379"},
	}
}

// Scaffold returns the minimal file set for a new node:fastify service.
func (a *Adapter) Scaffold(ctx adapter.ScaffoldContext) ([]adapter.File, error) {
	packageJSON, err := renderTemplate("package.json.tmpl", tmplPackageJSON, ctx)
	if err != nil {
		return nil, fmt.Errorf("package.json: %w", err)
	}
	indexJS, err := renderTemplate("index.js.tmpl", tmplIndexJS, ctx)
	if err != nil {
		return nil, fmt.Errorf("index.js: %w", err)
	}
	routesJS, err := renderTemplate("routes.js.tmpl", tmplRoutesJS, ctx)
	if err != nil {
		return nil, fmt.Errorf("routes.js: %w", err)
	}
	healthJS, err := renderTemplate("health.js.tmpl", tmplHealthJS, ctx)
	if err != nil {
		return nil, fmt.Errorf("health.js: %w", err)
	}
	configJS, err := renderTemplate("config.js.tmpl", tmplConfigJS, ctx)
	if err != nil {
		return nil, fmt.Errorf("config.js: %w", err)
	}
	errorsJS, err := renderTemplate("errors.js.tmpl", tmplErrorsJS, ctx)
	if err != nil {
		return nil, fmt.Errorf("errors.js: %w", err)
	}
	idsJS, err := a.renderIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("ids.js: %w", err)
	}
	envExample, err := renderTemplate("env.example.tmpl", tmplEnvExample, ctx)
	if err != nil {
		return nil, fmt.Errorf(".env.example: %w", err)
	}
	gitignore, err := renderTemplate("gitignore.tmpl", tmplGitignore, ctx)
	if err != nil {
		return nil, fmt.Errorf(".gitignore: %w", err)
	}
	dockerfile, err := renderTemplate("dockerfile.tmpl", tmplDockerfile, ctx)
	if err != nil {
		return nil, fmt.Errorf("rendering Dockerfile: %w", err)
	}

	return []adapter.File{
		{Path: "package.json", Content: packageJSON},
		{Path: ".env.example", Content: envExample},
		{Path: ".gitignore", Content: gitignore},
		{Path: "src/index.js", Content: indexJS},
		{Path: "src/routes.js", Content: routesJS},
		{Path: "src/health.js", Content: healthJS},
		{Path: "src/config.js", Content: configJS},
		{Path: "src/errors.js", Content: errorsJS},
		{Path: "src/ids.js", Content: idsJS},
		{Path: "Dockerfile", Content: dockerfile},
	}, nil
}

// DockerfileFor renders a production-ready, multi-stage Dockerfile for a
// node:fastify service node (Dockerizable capability). It is distinct from
// the scaffold-time Dockerfile Scaffold() writes once into the project:
// this one is (re)rendered by internal/deploy/artifacts on every
// `acthur deploy`, so EXPOSE/HEALTHCHECK always reflect the node's current
// graph facts — omitted entirely when the node has no port (e.g. a
// queue-worker service).
func (a *Adapter) DockerfileFor(ctx adapter.DockerfileContext) ([]byte, error) {
	return renderTemplate("dockerfile.prod.tmpl", tmplDockerfileProd, ctx)
}

// ---------------------------------------------------------------------------
// Template rendering
// ---------------------------------------------------------------------------

// renderTemplate executes a pre-loaded template with the given data.
func renderTemplate(name string, tmplBytes []byte, data any) ([]byte, error) {
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

// renderIDs selects the correct ids template based on ctx.IDStrategy.
func (a *Adapter) renderIDs(ctx adapter.ScaffoldContext) ([]byte, error) {
	if ctx.IDStrategy == "uuid-v4" {
		return renderTemplate("ids_uuid.js.tmpl", tmplIDsUUID, ctx)
	}
	return renderTemplate("ids_ulid.js.tmpl", tmplIDsULID, ctx)
}
