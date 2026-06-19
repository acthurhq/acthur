// Package postgres implements the db:postgres adapter.
// It describes how the kernel should run a PostgreSQL instance as a container.
// It has zero knowledge of plugins, contracts, or other adapters.
package postgres

import (
	"github.com/acthur/acthur/internal/adapter"
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
				Name:      ctx.NodeID + "-data",
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
