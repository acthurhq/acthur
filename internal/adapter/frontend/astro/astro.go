// Package astro implements the ui:astro adapter.
// It knows how to scaffold, start, build, test, and containerize an Astro
// application configured for server-side rendering (the @astrojs/node
// adapter in standalone mode) so the kernel's uniform GET /health contract
// works the same way it does for every backend adapter. It has zero
// knowledge of plugins or contracts.
package astro

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/acthur/acthur/internal/adapter"
)

//go:embed templates/package.json.tmpl
var tmplPackageJSON []byte

//go:embed templates/astro.config.mjs.tmpl
var tmplAstroConfig []byte

//go:embed templates/index.astro.tmpl
var tmplIndexAstro []byte

//go:embed templates/health.js.tmpl
var tmplHealthJS []byte

//go:embed templates/env.example.tmpl
var tmplEnvExample []byte

//go:embed templates/gitignore.tmpl
var tmplGitignore []byte

//go:embed templates/dockerfile.tmpl
var tmplDockerfile []byte

//go:embed templates/dockerfile.prod.tmpl
var tmplDockerfileProd []byte

// Adapter implements adapter.Adapter for Astro (SSR via @astrojs/node).
type Adapter struct{}

func init() {
	adapter.Register(&Adapter{})
}

func (a *Adapter) Name() string               { return "ui:astro" }
func (a *Adapter) Category() adapter.Category { return adapter.CategoryFrontend }

// Detect returns true if a package.json declaring astro as a dependency is
// found in dir.
func (a *Adapter) Detect(dir string) bool {
	pkgJSON := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgJSON)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), `"astro"`)
}

// DevCommand returns the astro dev server command. Astro's own dev server
// (Vite underneath) rebuilds and hot-reloads on file change; the port is
// read from the PORT/APP_PORT env var by astro.config.mjs at process start,
// not passed as a CLI flag — matching node:fastify's env-driven config
// pattern rather than go:fiber's flag-driven one.
func (a *Adapter) DevCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "npm",
		Args: []string{"run", "dev"},
		Env:  env,
	}
}

// SelfReloads reports true: astro dev's Vite-powered server rebuilds and
// hot-reloads itself on file change. The dev engine's file watcher must not
// also restart this node's process — that would just race astro's own
// rebuild for the same port.
func (a *Adapter) SelfReloads() bool { return true }

// BuildCommand returns the astro build command (SSR build via the
// @astrojs/node adapter — produces dist/server/entry.mjs + dist/client).
func (a *Adapter) BuildCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "npm",
		Args: []string{"run", "build"},
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

// EnvVars returns the environment variables an Astro SSR app requires.
// Unlike backend adapters, a frontend node has no database/secret of its
// own — it calls a backend API instead, hence PUBLIC_API_URL rather than
// DATABASE_URL/APP_SECRET.
func (a *Adapter) EnvVars() []adapter.EnvVar {
	return []adapter.EnvVar{
		{Key: "APP_ENV", Description: "application environment", Required: true, Default: "development"},
		{Key: "APP_PORT", Description: "HTTP port", Required: true, Default: "4321"},
		{Key: "PUBLIC_API_URL", Description: "base URL of the backend API this frontend calls", Required: false, Default: "http://localhost:8080"},
	}
}

// Scaffold returns the minimal file set for a new ui:astro node.
func (a *Adapter) Scaffold(ctx adapter.ScaffoldContext) ([]adapter.File, error) {
	packageJSON, err := renderTemplate("package.json.tmpl", tmplPackageJSON, ctx)
	if err != nil {
		return nil, fmt.Errorf("package.json: %w", err)
	}
	astroConfig, err := renderTemplate("astro.config.mjs.tmpl", tmplAstroConfig, ctx)
	if err != nil {
		return nil, fmt.Errorf("astro.config.mjs: %w", err)
	}
	indexAstro, err := renderTemplate("index.astro.tmpl", tmplIndexAstro, ctx)
	if err != nil {
		return nil, fmt.Errorf("index.astro: %w", err)
	}
	healthJS, err := renderTemplate("health.js.tmpl", tmplHealthJS, ctx)
	if err != nil {
		return nil, fmt.Errorf("health.js: %w", err)
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
		{Path: "astro.config.mjs", Content: astroConfig},
		{Path: ".env.example", Content: envExample},
		{Path: ".gitignore", Content: gitignore},
		{Path: "src/pages/index.astro", Content: indexAstro},
		{Path: "src/pages/health.js", Content: healthJS},
		{Path: "Dockerfile", Content: dockerfile},
	}, nil
}

// DockerfileFor renders a production-ready, multi-stage Dockerfile for a
// ui:astro service node (Dockerizable capability). It is distinct from the
// scaffold-time Dockerfile Scaffold() writes once into the project: this
// one is (re)rendered by internal/deploy/artifacts on every `acthur deploy`,
// so EXPOSE/HEALTHCHECK always reflect the node's current graph facts —
// omitted entirely when the node has no port.
func (a *Adapter) DockerfileFor(ctx adapter.DockerfileContext) ([]byte, error) {
	return renderTemplate("dockerfile.prod.tmpl", tmplDockerfileProd, ctx)
}

// ---------------------------------------------------------------------------
// Template rendering
// ---------------------------------------------------------------------------

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
