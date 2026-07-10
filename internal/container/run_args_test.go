package container_test

import (
	"reflect"
	"testing"

	"github.com/acthur/acthur/internal/adapter"
	"github.com/acthur/acthur/internal/container"
)

func TestToRunArgs_ProjectsContainerSpecToDockerRunArgs(t *testing.T) {
	spec := adapter.ContainerSpec{
		Image: "postgres",
		Tag:   "16",
		Ports: []int{5432},
		Volumes: []adapter.Volume{
			{Name: "db-data", MountPath: "/var/lib/postgresql/data"},
		},
		Env: map[string]string{
			"POSTGRES_DB":       "db_development",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_USER":     "postgres",
		},
	}

	got := container.ToRunArgs(spec, container.Name("shop", "db"))

	want := []string{
		"run", "--rm",
		"--name", "acthur-shop-db",
		"-p", "5432:5432",
		"-v", "db-data:/var/lib/postgresql/data",
		"-e", "POSTGRES_DB=db_development",
		"-e", "POSTGRES_PASSWORD=postgres",
		"-e", "POSTGRES_USER=postgres",
		"postgres:16",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("docker run args mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

// TestToStopArgs_ProjectsNodeIDToDockerStopArgs: stopping an infra node must
// stop the *container*, not just the docker-run client process — killing the
// client leaves the container running (found by the #33 live witness).
func TestToStopArgs_ProjectsNodeIDToDockerStopArgs(t *testing.T) {
	got := container.ToStopArgs(container.Name("shop", "db"))
	want := []string{"stop", "acthur-shop-db"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stop args mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

// TestName_ScopesByProject: two projects on one host must not share
// containers — a shared name means shared postgres volumes and migration
// state bleeding across projects (witnessed live: a stale schema_migrations
// version from another project broke `acthur db migrate`).
func TestName_ScopesByProject(t *testing.T) {
	if got := container.Name("shop", "db"); got != "acthur-shop-db" {
		t.Errorf("expected acthur-shop-db, got %q", got)
	}
	// No project (defensive) falls back to the legacy node-only name.
	if got := container.Name("", "db"); got != "acthur-db" {
		t.Errorf("expected acthur-db fallback, got %q", got)
	}
}
