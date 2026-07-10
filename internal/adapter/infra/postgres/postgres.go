// Package postgres implements the db:postgres adapter.
// It describes how the kernel should run a PostgreSQL instance as a container.
// It has zero knowledge of plugins, contracts, or other adapters.
package postgres

import (
	"fmt"

	"github.com/acthurhq/acthur/internal/adapter"
)

// Adapter implements adapter.Adapter (core) and adapter.Containerized for db:postgres.
type Adapter struct{}

func init() {
	adapter.Register(&Adapter{})
}

func (a *Adapter) Name() string               { return "db:postgres" }
func (a *Adapter) Category() adapter.Category { return adapter.CategoryDatabase }
func (a *Adapter) Detect(dir string) bool     { return false }
func (a *Adapter) EnvVars() []adapter.EnvVar  { return nil }

// Container returns a declarative ContainerSpec for a Postgres instance.
// The kernel projects this spec onto docker run / compose / k8s in later phases.
func (a *Adapter) Container(ctx adapter.ContainerContext) adapter.ContainerSpec {
	tag := ctx.Version
	if tag == "" {
		tag = "16"
	}

	dbName := "app_development"
	if ctx.NodeID != "" {
		dbName = ctx.NodeID + "_development"
	}

	return adapter.ContainerSpec{
		Image: "postgres",
		Tag:   tag,
		Ports: []int{5432},
		Volumes: []adapter.Volume{
			{
				Name:      volumeName(ctx),
				MountPath: "/var/lib/postgresql/data",
			},
		},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_DB":       dbName,
		},
		Healthcheck: adapter.Healthcheck{
			Test:     []string{"CMD-SHELL", "pg_isready -U postgres"},
			Interval: "5s",
			Timeout:  "5s",
			Retries:  5,
		},
	}
}

// ConnectionEnv returns the connection variables exported to dependent nodes.
// In the local dev context Postgres runs without TLS, so the URL declares
// sslmode=disable to keep drivers from negotiating a connection the container
// can't satisfy. The host port is read from the spec rather than hardcoded.
func (a *Adapter) ConnectionEnv(ctx adapter.ContainerContext) map[string]string {
	spec := a.Container(ctx)
	port := 5432
	if len(spec.Ports) > 0 {
		port = spec.Ports[0]
	}
	return map[string]string{
		"DATABASE_URL": fmt.Sprintf(
			"postgres://%s:%s@localhost:%d/%s?sslmode=disable",
			spec.Env["POSTGRES_USER"],
			spec.Env["POSTGRES_PASSWORD"],
			port,
			spec.Env["POSTGRES_DB"],
		),
	}
}

// volumeName scopes the data volume by project: a host running two acthur
// projects must never share database state (witnessed live — a stale
// schema_migrations version from another project broke db migrate).
func volumeName(ctx adapter.ContainerContext) string {
	if ctx.Project == "" {
		return ctx.NodeID + "-data"
	}
	return ctx.Project + "-" + ctx.NodeID + "-data"
}
