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

	got := container.ToRunArgs(spec, "db")

	want := []string{
		"run", "--rm",
		"--name", "acthur-db",
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
