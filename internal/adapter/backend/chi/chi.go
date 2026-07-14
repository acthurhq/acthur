// Package chi implements the go:chi adapter.
// It knows how to scaffold, start, build, test, and containerize
// a Go net/http + go-chi/chi application. It has zero knowledge of plugins
// or contracts.
package chi

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/acthurhq/acthur/internal/adapter"
)

//go:embed templates/go.mod.tmpl
var tmplGoMod []byte

//go:embed templates/main.go.tmpl
var tmplMainGo []byte

//go:embed templates/server.go.tmpl
var tmplServerGo []byte

//go:embed templates/routes.go.tmpl
var tmplRoutesGo []byte

//go:embed templates/health.go.tmpl
var tmplHealthGo []byte

//go:embed templates/config.go.tmpl
var tmplConfigGo []byte

//go:embed templates/errors.go.tmpl
var tmplErrorsGo []byte

//go:embed templates/ids_ulid.go.tmpl
var tmplIDsULID []byte

//go:embed templates/ids_uuid.go.tmpl
var tmplIDsUUID []byte

//go:embed templates/middleware_logger.go.tmpl
var tmplMiddlewareLogger []byte

//go:embed templates/air.toml.tmpl
var tmplAirToml []byte

//go:embed templates/env.example.tmpl
var tmplEnvExample []byte

//go:embed templates/gitignore.tmpl
var tmplGitignore []byte

//go:embed templates/dockerfile.tmpl
var tmplDockerfile []byte

//go:embed templates/dockerfile.prod.tmpl
var tmplDockerfileProd []byte

// Adapter implements adapter.Adapter for go-chi.
type Adapter struct{}

func init() {
	adapter.Register(&Adapter{})
}

func (a *Adapter) Name() string               { return "go:chi" }
func (a *Adapter) Category() adapter.Category { return adapter.CategoryBackend }

// Detect returns true if a go.mod containing chi is found in dir.
func (a *Adapter) Detect(dir string) bool {
	gomod := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(gomod)
	if err != nil {
		return false
	}
	return contains(string(data), "github.com/go-chi/chi")
}

// DevCommand returns the air command for hot reload.
func (a *Adapter) DevCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "air",
		Args: []string{"-c", ".air.toml"},
		Env:  env,
	}
}

// SelfReloads reports true: air watches this node's own source directory,
// rebuilds, and restarts the compiled binary itself. The dev engine's file
// watcher must not also restart this node's process — that would just race
// air's own rebuild for the same port.
func (a *Adapter) SelfReloads() bool { return true }

// BuildCommand returns the go build command for production.
func (a *Adapter) BuildCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "go",
		Args: []string{"build", "-ldflags", "-s -w", "-o", "bin/app", "./..."},
		Env:  mergeEnv(env, map[string]string{"CGO_ENABLED": "0"}),
	}
}

// TestCommand returns the go test command.
func (a *Adapter) TestCommand(env map[string]string) adapter.Command {
	return adapter.Command{
		Bin:  "go",
		Args: []string{"test", "./...", "-count=1"},
		Env:  env,
	}
}

// EnvVars returns the environment variables a chi app requires.
func (a *Adapter) EnvVars() []adapter.EnvVar {
	return []adapter.EnvVar{
		{Key: "APP_ENV", Description: "application environment", Required: true, Default: "development"},
		{Key: "APP_PORT", Description: "HTTP port", Required: true, Default: "8080"},
		{Key: "APP_SECRET", Description: "application secret key", Required: true, Secret: true, Generate: true},
		{Key: "DATABASE_URL", Description: "PostgreSQL connection string", Required: true, Secret: true},
		{Key: "REDIS_URL", Description: "Redis connection string", Required: false, Default: "redis://localhost:6379"},
	}
}

// Scaffold returns the minimal file set for a new go:chi service.
func (a *Adapter) Scaffold(ctx adapter.ScaffoldContext) ([]adapter.File, error) {
	goMod, err := renderTemplate("go.mod.tmpl", tmplGoMod, ctx)
	if err != nil {
		return nil, fmt.Errorf("go.mod: %w", err)
	}
	mainGo, err := renderTemplate("main.go.tmpl", tmplMainGo, ctx)
	if err != nil {
		return nil, fmt.Errorf("main.go: %w", err)
	}
	serverGo, err := renderTemplate("server.go.tmpl", tmplServerGo, ctx)
	if err != nil {
		return nil, fmt.Errorf("server.go: %w", err)
	}
	routesGo, err := renderTemplate("routes.go.tmpl", tmplRoutesGo, ctx)
	if err != nil {
		return nil, fmt.Errorf("routes.go: %w", err)
	}
	healthGo, err := renderTemplate("health.go.tmpl", tmplHealthGo, ctx)
	if err != nil {
		return nil, fmt.Errorf("health.go: %w", err)
	}
	configGo, err := renderTemplate("config.go.tmpl", tmplConfigGo, ctx)
	if err != nil {
		return nil, fmt.Errorf("config.go: %w", err)
	}
	errorsGo, err := renderTemplate("errors.go.tmpl", tmplErrorsGo, ctx)
	if err != nil {
		return nil, fmt.Errorf("errors.go: %w", err)
	}
	idsGo, err := a.renderIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("ids.go: %w", err)
	}
	loggerGo, err := renderTemplate("middleware_logger.go.tmpl", tmplMiddlewareLogger, ctx)
	if err != nil {
		return nil, fmt.Errorf("middleware_logger.go: %w", err)
	}
	airToml, err := renderTemplate("air.toml.tmpl", tmplAirToml, ctx)
	if err != nil {
		return nil, fmt.Errorf(".air.toml: %w", err)
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
		{Path: "go.mod", Content: goMod},
		{Path: "main.go", Content: mainGo},
		{Path: ".air.toml", Content: airToml},
		{Path: ".env.example", Content: envExample},
		{Path: ".gitignore", Content: gitignore},
		{Path: "internal/server/server.go", Content: serverGo},
		{Path: "internal/server/routes.go", Content: routesGo},
		{Path: "internal/health/handler.go", Content: healthGo},
		{Path: "internal/config/config.go", Content: configGo},
		{Path: "internal/errors/errors.go", Content: errorsGo},
		{Path: "internal/ids/ids.go", Content: idsGo},
		{Path: "internal/middleware/logger.go", Content: loggerGo},
		{Path: "Dockerfile", Content: dockerfile},
	}, nil
}

// DockerfileFor renders a production-ready, multi-stage Dockerfile for a
// The go:chi service node (Dockerizable capability). It is distinct from the
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
		return renderTemplate("ids_uuid.go.tmpl", tmplIDsUUID, ctx)
	}
	return renderTemplate("ids_ulid.go.tmpl", tmplIDsULID, ctx)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mergeEnv(base, extra map[string]string) map[string]string {
	result := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
