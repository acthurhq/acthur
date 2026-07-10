// Package next implements the ui:next adapter.
// It knows how to scaffold, start, build, test, and containerize a Next.js
// App Router application configured for standalone output, so it produces
// a self-contained production server the same way go:fiber produces a
// static binary. It has zero knowledge of plugins or contracts.
package next

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

//go:embed templates/next.config.mjs.tmpl
var tmplNextConfig []byte

//go:embed templates/tsconfig.json.tmpl
var tmplTSConfig []byte

//go:embed templates/next-env.d.ts.tmpl
var tmplNextEnv []byte

//go:embed templates/layout.tsx.tmpl
var tmplLayoutTSX []byte

//go:embed templates/page.tsx.tmpl
var tmplPageTSX []byte

//go:embed templates/health.route.ts.tmpl
var tmplHealthRouteTS []byte

//go:embed templates/env.example.tmpl
var tmplEnvExample []byte

//go:embed templates/gitignore.tmpl
var tmplGitignore []byte

//go:embed templates/dockerfile.tmpl
var tmplDockerfile []byte

//go:embed templates/dockerfile.prod.tmpl
var tmplDockerfileProd []byte

// Adapter implements adapter.Adapter for Next.js (App Router, standalone output).
type Adapter struct{}

func init() {
	adapter.Register(&Adapter{})
}

func (a *Adapter) Name() string               { return "ui:next" }
func (a *Adapter) Category() adapter.Category { return adapter.CategoryFrontend }

// Detect returns true if a package.json declaring next as a dependency is
// found in dir. Checked as `"next":` (with the trailing colon) rather than
// a bare substring match, since "next" alone is common in unrelated package
// names/descriptions.
func (a *Adapter) Detect(dir string) bool {
	pkgJSON := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgJSON)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), `"next":`)
}

// DevCommand returns the next dev command. The port is threaded explicitly
// via -p (Next's CLI flag) rather than left to Next's own PORT env
// handling, so behavior is identical and version-independent across the
// Next releases this scaffold might be upgraded to.
func (a *Adapter) DevCommand(env map[string]string) adapter.Command {
	port := env["APP_PORT"]
	if port == "" {
		port = env["PORT"]
	}
	if port == "" {
		port = "3000"
	}
	return adapter.Command{
		Bin:  "npm",
		Args: []string{"run", "dev", "--", "-p", port},
		Env:  env,
	}
}

// SelfReloads reports true: next dev's Fast Refresh/webpack (or Turbopack)
// dev server rebuilds and hot-reloads itself on file change. The dev
// engine's file watcher must not also restart this node's process — that
// would just race Next's own rebuild for the same port.
func (a *Adapter) SelfReloads() bool { return true }

// BuildCommand returns the next build command (standalone output — produces
// .next/standalone + .next/static).
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

// EnvVars returns the environment variables a Next.js app requires.
// Unlike backend adapters, a frontend node has no database/secret of its
// own — it calls a backend API instead, hence NEXT_PUBLIC_API_URL rather
// than DATABASE_URL/APP_SECRET. NEXT_PUBLIC_* vars are inlined into the
// client bundle at build time by Next.js itself.
func (a *Adapter) EnvVars() []adapter.EnvVar {
	return []adapter.EnvVar{
		{Key: "APP_ENV", Description: "application environment", Required: true, Default: "development"},
		{Key: "APP_PORT", Description: "HTTP port", Required: true, Default: "3000"},
		{Key: "NEXT_PUBLIC_API_URL", Description: "base URL of the backend API this frontend calls", Required: false, Default: "http://localhost:8080"},
	}
}

// Scaffold returns the minimal file set for a new ui:next node.
func (a *Adapter) Scaffold(ctx adapter.ScaffoldContext) ([]adapter.File, error) {
	packageJSON, err := renderTemplate("package.json.tmpl", tmplPackageJSON, ctx)
	if err != nil {
		return nil, fmt.Errorf("package.json: %w", err)
	}
	nextConfig, err := renderTemplate("next.config.mjs.tmpl", tmplNextConfig, ctx)
	if err != nil {
		return nil, fmt.Errorf("next.config.mjs: %w", err)
	}
	tsConfig, err := renderTemplate("tsconfig.json.tmpl", tmplTSConfig, ctx)
	if err != nil {
		return nil, fmt.Errorf("tsconfig.json: %w", err)
	}
	nextEnv, err := renderTemplate("next-env.d.ts.tmpl", tmplNextEnv, ctx)
	if err != nil {
		return nil, fmt.Errorf("next-env.d.ts: %w", err)
	}
	layoutTSX, err := renderTemplate("layout.tsx.tmpl", tmplLayoutTSX, ctx)
	if err != nil {
		return nil, fmt.Errorf("app/layout.tsx: %w", err)
	}
	pageTSX, err := renderTemplate("page.tsx.tmpl", tmplPageTSX, ctx)
	if err != nil {
		return nil, fmt.Errorf("app/page.tsx: %w", err)
	}
	healthRouteTS, err := renderTemplate("health.route.ts.tmpl", tmplHealthRouteTS, ctx)
	if err != nil {
		return nil, fmt.Errorf("app/health/route.ts: %w", err)
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
		{Path: "next.config.mjs", Content: nextConfig},
		{Path: "tsconfig.json", Content: tsConfig},
		{Path: "next-env.d.ts", Content: nextEnv},
		{Path: ".env.example", Content: envExample},
		{Path: ".gitignore", Content: gitignore},
		{Path: "app/layout.tsx", Content: layoutTSX},
		{Path: "app/page.tsx", Content: pageTSX},
		{Path: "app/health/route.ts", Content: healthRouteTS},
		{Path: "Dockerfile", Content: dockerfile},
	}, nil
}

// DockerfileFor renders a production-ready, multi-stage Dockerfile for a
// ui:next service node (Dockerizable capability). It is distinct from the
// scaffold-time Dockerfile Scaffold() writes once into the project: this
// one is (re)rendered by internal/deploy/artifacts on every `acthur
// deploy`, so EXPOSE/HEALTHCHECK always reflect the node's current graph
// facts — omitted entirely when the node has no port.
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
