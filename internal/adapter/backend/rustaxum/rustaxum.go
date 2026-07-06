// Package rustaxum implements the rust:axum adapter.
// It knows how to scaffold, start, build, test, and containerize
// a Rust Axum application. It has zero knowledge of plugins or contracts.
package rustaxum

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

//go:embed templates/cargo.toml.tmpl
var tmplCargoToml []byte

//go:embed templates/main.rs.tmpl
var tmplMainRs []byte

//go:embed templates/routes.rs.tmpl
var tmplRoutesRs []byte

//go:embed templates/health.rs.tmpl
var tmplHealthRs []byte

//go:embed templates/config.rs.tmpl
var tmplConfigRs []byte

//go:embed templates/errors.rs.tmpl
var tmplErrorsRs []byte

//go:embed templates/ids_ulid.rs.tmpl
var tmplIDsULID []byte

//go:embed templates/ids_uuid.rs.tmpl
var tmplIDsUUID []byte

//go:embed templates/env.example.tmpl
var tmplEnvExample []byte

//go:embed templates/gitignore.tmpl
var tmplGitignore []byte

//go:embed templates/dockerfile.tmpl
var tmplDockerfile []byte

//go:embed templates/dockerfile.prod.tmpl
var tmplDockerfileProd []byte

// Adapter implements adapter.Adapter for Rust Axum.
type Adapter struct{}

func init() {
	adapter.Register(&Adapter{})
}

func (a *Adapter) Name() string               { return "rust:axum" }
func (a *Adapter) Category() adapter.Category { return adapter.CategoryBackend }

// Detect returns true if a Cargo.toml containing axum is found in dir.
func (a *Adapter) Detect(dir string) bool {
	cargoToml := filepath.Join(dir, "Cargo.toml")
	data, err := os.ReadFile(cargoToml)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "axum")
}

// DevCommand returns the cargo-watch command for hot reload.
func (a *Adapter) DevCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "cargo",
		Args: []string{"watch", "-x", "run"},
		Env:  env,
	}
}

// SelfReloads reports true: cargo-watch rebuilds and reruns the compiled
// binary itself on file change. The dev engine's file watcher must not
// also restart this node's process — that would just race cargo-watch's
// own rebuild for the same port.
func (a *Adapter) SelfReloads() bool { return true }

// BuildCommand returns the cargo build command for production.
func (a *Adapter) BuildCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "cargo",
		Args: []string{"build", "--release"},
		Env:  env,
	}
}

// TestCommand returns the cargo test command.
func (a *Adapter) TestCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "cargo",
		Args: []string{"test"},
		Env:  env,
	}
}

// EnvVars returns the environment variables an Axum app requires.
func (a *Adapter) EnvVars() []adapter.EnvVar {
	return []adapter.EnvVar{
		{Key: "APP_ENV", Description: "application environment", Required: true, Default: "development"},
		{Key: "APP_PORT", Description: "HTTP port", Required: true, Default: "8080"},
		{Key: "APP_SECRET", Description: "application secret key", Required: true, Secret: true, Generate: true},
		{Key: "DATABASE_URL", Description: "PostgreSQL connection string", Required: true, Secret: true},
		{Key: "REDIS_URL", Description: "Redis connection string", Required: false, Default: "redis://localhost:6379"},
	}
}

// Scaffold returns the minimal file set for a new rust:axum service.
func (a *Adapter) Scaffold(ctx adapter.ScaffoldContext) ([]adapter.File, error) {
	cargoToml, err := renderTemplate("cargo.toml.tmpl", tmplCargoToml, ctx)
	if err != nil {
		return nil, fmt.Errorf("Cargo.toml: %w", err)
	}
	mainRs, err := renderTemplate("main.rs.tmpl", tmplMainRs, ctx)
	if err != nil {
		return nil, fmt.Errorf("main.rs: %w", err)
	}
	routesRs, err := renderTemplate("routes.rs.tmpl", tmplRoutesRs, ctx)
	if err != nil {
		return nil, fmt.Errorf("routes.rs: %w", err)
	}
	healthRs, err := renderTemplate("health.rs.tmpl", tmplHealthRs, ctx)
	if err != nil {
		return nil, fmt.Errorf("health.rs: %w", err)
	}
	configRs, err := renderTemplate("config.rs.tmpl", tmplConfigRs, ctx)
	if err != nil {
		return nil, fmt.Errorf("config.rs: %w", err)
	}
	errorsRs, err := renderTemplate("errors.rs.tmpl", tmplErrorsRs, ctx)
	if err != nil {
		return nil, fmt.Errorf("errors.rs: %w", err)
	}
	idsRs, err := a.renderIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("ids.rs: %w", err)
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
		return nil, fmt.Errorf("Dockerfile: %w", err)
	}

	return []adapter.File{
		{Path: "Cargo.toml", Content: cargoToml},
		{Path: ".env.example", Content: envExample},
		{Path: ".gitignore", Content: gitignore},
		{Path: "src/main.rs", Content: mainRs},
		{Path: "src/routes.rs", Content: routesRs},
		{Path: "src/health.rs", Content: healthRs},
		{Path: "src/config.rs", Content: configRs},
		{Path: "src/errors.rs", Content: errorsRs},
		{Path: "src/ids.rs", Content: idsRs},
		{Path: "Dockerfile", Content: dockerfile},
	}, nil
}

// DockerfileFor renders a production-ready, multi-stage Dockerfile for a
// rust:axum service node (Dockerizable capability). It is distinct from the
// scaffold-time Dockerfile Scaffold() writes once into the project: this one
// is (re)rendered by internal/deploy/artifacts on every `acthur deploy`, so
// EXPOSE/HEALTHCHECK always reflect the node's current graph facts —
// omitted entirely when the node has no port (e.g. a queue-worker service).
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
		return renderTemplate("ids_uuid.rs.tmpl", tmplIDsUUID, ctx)
	}
	return renderTemplate("ids_ulid.rs.tmpl", tmplIDsULID, ctx)
}
