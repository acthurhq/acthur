// Package container projects declarative adapter container specs onto local
// container runtime commands.
package container

import (
	"sort"
	"strconv"

	"github.com/acthur/acthur/internal/adapter"
)

// ToRunArgs projects a ContainerSpec onto docker run arguments for Local dev.
func ToRunArgs(spec adapter.ContainerSpec, nodeID string) []string {
	args := []string{
		"run", "--rm",
		"--name", "acthur-" + nodeID,
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

func imageRef(spec adapter.ContainerSpec) string {
	if spec.Tag == "" {
		return spec.Image
	}
	return spec.Image + ":" + spec.Tag
}
