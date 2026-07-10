// Package container projects declarative adapter container specs onto local
// container runtime commands.
package container

import (
	"sort"
	"strconv"

	"github.com/acthur/acthur/internal/adapter"
)

// Name returns the container name for a node, scoped by project so two
// projects on one host never share a container (and with it, volumes and
// database state). An empty project falls back to the legacy node-only name.
func Name(project, nodeID string) string {
	if project == "" {
		return "acthur-" + nodeID
	}
	return "acthur-" + project + "-" + nodeID
}

// ToRunArgs projects a ContainerSpec onto docker run arguments for Local dev.
func ToRunArgs(spec adapter.ContainerSpec, name string) []string {
	args := []string{
		"run", "--rm",
		"--name", name,
	}

	for _, port := range spec.Ports {
		mapping := strconv.Itoa(port) + ":" + strconv.Itoa(port)
		args = append(args, "-p", mapping)
	}

	for _, volume := range spec.Volumes {
		args = append(args, "-v", volume.Name+":"+volume.MountPath)
	}

	envKeys := make([]string, 0, len(spec.Env))
	for key := range spec.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for _, key := range envKeys {
		args = append(args, "-e", key+"="+spec.Env[key])
	}

	args = append(args, imageRef(spec))
	args = append(args, spec.Cmd...)
	return args
}

// ToStopArgs projects a container name onto the `docker stop` arguments that
// stop the container ToRunArgs named. Stopping the container (rather than
// killing the docker-run client) is the only way the containerized process
// actually terminates; with --rm the client then exits and cleans up on its
// own.
func ToStopArgs(name string) []string {
	return []string{"stop", name}
}

func imageRef(spec adapter.ContainerSpec) string {
	if spec.Tag == "" {
		return spec.Image
	}
	return spec.Image + ":" + spec.Tag
}
