// Package artifacts projects a sealed graph.Graph onto the production deploy
// artifacts Slice 1 of Phase 8 owns: a Dockerfile per Dockerizable service
// node and one docker-compose.prod.yml wiring the whole graph together.
//
// Project never touches disk itself — it returns []plugin.GeneratedFile for
// internal/generate.WriteFiles to persist, the same write engine and
// generated.lock semantics `acthur add` already uses.
package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/acthurhq/acthur/internal/adapter"
	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
	"github.com/acthurhq/acthur/internal/plugin"

	"gopkg.in/yaml.v3"
)

// networkName is the single internal network every projected service and
// infra node shares. Production has no reverse proxy yet (that's dev-only);
// services reach each other by compose service name on this network.
const networkName = "acthur"

// ---------------------------------------------------------------------------
// docker-compose.prod.yml — Go-typed mirror of the compose schema we emit.
// Field order below is the emission order; gopkg.in/yaml.v3 marshals map
// keys sorted, so the file is byte-identical across runs regardless.
// ---------------------------------------------------------------------------

type composeBuild struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile"`
}

type composeDependsOn struct {
	Condition string `yaml:"condition"`
}

type composeHealthcheck struct {
	Test     []string `yaml:"test"`
	Interval string   `yaml:"interval,omitempty"`
	Timeout  string   `yaml:"timeout,omitempty"`
	Retries  int      `yaml:"retries,omitempty"`
}

type composeService struct {
	Build       *composeBuild               `yaml:"build,omitempty"`
	Image       string                      `yaml:"image,omitempty"`
	Volumes     []string                    `yaml:"volumes,omitempty"`
	Environment map[string]string           `yaml:"environment,omitempty"`
	Ports       []string                    `yaml:"ports,omitempty"`
	DependsOn   map[string]composeDependsOn `yaml:"depends_on,omitempty"`
	Healthcheck *composeHealthcheck         `yaml:"healthcheck,omitempty"`
	Restart     string                      `yaml:"restart,omitempty"`
	Networks    []string                    `yaml:"networks,omitempty"`
}

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
	Volumes  map[string]struct{}       `yaml:"volumes,omitempty"`
	Networks map[string]struct{}       `yaml:"networks,omitempty"`
}

// ---------------------------------------------------------------------------
// Project
// ---------------------------------------------------------------------------

// Project generates the production deploy artifacts for the sealed graph g:
// deploy/Dockerfile.<nodeID> for every service node whose adapter satisfies
// adapter.Dockerizable, and one deploy/docker-compose.prod.yml wiring every
// projectable node together. Every declared service and infra node must be
// projectable: silently skipping one would emit a partial production graph.
//
// Every returned file has Overwrite=true: deploy artifacts are regenerated
// fresh on every `acthur deploy`, and generated.lock (internal/generate)
// still protects a hand-edited Dockerfile by skipping it with a warning
// rather than silently clobbering it.
func Project(cfg *config.Config, g *graph.Graph, root string) ([]plugin.GeneratedFile, error) {
	nodes := g.Nodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	proxied := make(map[string]bool)
	for _, n := range g.ProxiedNodes() {
		proxied[n.ID] = true
	}

	services := make(map[string]composeService)
	volumes := make(map[string]struct{})
	var files []plugin.GeneratedFile

	for _, n := range nodes {
		// Kernel-materialized nodes (the proxy) are never user-declared and
		// never resolve through the adapter registry; production has no
		// reverse proxy in this phase, so they have nothing to project.
		if strings.HasPrefix(n.Adapter, "kernel:") {
			continue
		}
		if n.Type != config.NodeTypeService && n.Type != config.NodeTypeInfra {
			continue
		}

		a, err := adapter.Resolve(n.Adapter)
		if err != nil {
			return nil, fmt.Errorf("projecting node %q with adapter %q: adapter cannot be resolved: %w", n.ID, n.Adapter, err)
		}

		switch n.Type {
		case config.NodeTypeService:
			svc, dockerfile, ok, err := projectService(a, n, proxied[n.ID], nodeGoVersion(root, n.ID))
			if err != nil {
				return nil, fmt.Errorf("node %q: %w", n.ID, err)
			}
			if !ok {
				return nil, fmt.Errorf("projecting node %q with adapter %q: service adapter lacks required Dockerizable production capability", n.ID, n.Adapter)
			}
			services[n.ID] = svc
			files = append(files, plugin.GeneratedFile{
				Path:      fmt.Sprintf("deploy/Dockerfile.%s", n.ID),
				Content:   dockerfile,
				Overwrite: true,
			})

		case config.NodeTypeInfra:
			svc, ok := projectInfra(a, n)
			if !ok {
				return nil, fmt.Errorf("projecting node %q with adapter %q: infra adapter lacks required Containerized production capability", n.ID, n.Adapter)
			}
			services[n.ID] = svc
			for _, v := range svc.Volumes {
				volumes[volumeNameFromMount(v)] = struct{}{}
			}
		}
	}

	wireDependsOn(g, nodes, services)

	compose := composeFile{
		Services: services,
		Networks: map[string]struct{}{networkName: {}},
	}
	if len(volumes) > 0 {
		compose.Volumes = volumes
	}

	composeYAML, err := yaml.Marshal(compose)
	if err != nil {
		return nil, fmt.Errorf("marshaling docker-compose.prod.yml: %w", err)
	}
	files = append(files, plugin.GeneratedFile{
		Path:      "deploy/docker-compose.prod.yml",
		Content:   composeYAML,
		Overwrite: true,
	})

	return files, nil
}

// projectService projects a service node onto a compose service plus its
// rendered Dockerfile, if its adapter satisfies Dockerizable. ok is false
// (with a nil error) when the adapter has no Dockerfile capability yet.
var goModDirective = regexp.MustCompile(`(?m)^go\s+(\d+)\.(\d+)(?:\.\d+)?\s*$`)

// nodeGoVersion reads nodeID's own go.mod (if any) under root and returns
// its `go` directive as major.minor (e.g. "1.25") for use as a Docker
// builder image tag. Returns "" when root/nodeID has no go.mod (non-Go
// adapters) or it can't be parsed — the caller's adapter then falls back to
// its own hardcoded floor.
func nodeGoVersion(root, nodeID string) string {
	data, err := os.ReadFile(filepath.Join(root, nodeID, "go.mod"))
	if err != nil {
		return ""
	}
	m := goModDirective.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return fmt.Sprintf("%s.%s", m[1], m[2])
}

func projectService(a adapter.Adapter, n *graph.Node, isProxied bool, goVersion string) (composeService, []byte, bool, error) {
	d, ok := a.(adapter.Dockerizable)
	if !ok {
		return composeService{}, nil, false, nil
	}

	dockerfile, err := d.DockerfileFor(adapter.DockerfileContext{NodeID: n.ID, Port: n.Port, GoVersion: goVersion})
	if err != nil {
		return composeService{}, nil, false, fmt.Errorf("rendering Dockerfile: %w", err)
	}

	svc := composeService{
		Build: &composeBuild{
			// Relative to the compose file's own directory (deploy/):
			// context is the node's source dir one level up; dockerfile is
			// then resolved relative to that context.
			Context:    "../" + n.ID,
			Dockerfile: "../deploy/Dockerfile." + n.ID,
		},
		Environment: envRefs(envVarKeys(a.EnvVars())),
		Restart:     "unless-stopped",
		Networks:    []string{networkName},
	}

	if n.Port != 0 {
		svc.Healthcheck = &composeHealthcheck{
			Test:     []string{"CMD-SHELL", fmt.Sprintf("wget -qO- http://127.0.0.1:%d/health || exit 1", n.Port)},
			Interval: "10s",
			Timeout:  "3s",
			Retries:  5,
		}
		// Production has no reverse proxy (Phase 8 out-of-scope); the
		// public surface is whatever the graph already routes through the
		// dev proxy via proxied_through — that's the API service port.
		if isProxied {
			svc.Ports = []string{fmt.Sprintf("%d:%d", n.Port, n.Port)}
		}
	}

	return svc, dockerfile, true, nil
}

// projectInfra projects an infra node onto a compose service backed by its
// adapter's official image, if the adapter satisfies Containerized. ok is
// false when the adapter has no Container capability (e.g. unimplemented).
func projectInfra(a adapter.Adapter, n *graph.Node) (composeService, bool) {
	c, ok := a.(adapter.Containerized)
	if !ok {
		return composeService{}, false
	}

	spec := c.Container(adapter.ContainerContext{NodeID: n.ID, Version: n.Config.Version})

	svc := composeService{
		Image:    imageRef(spec),
		Restart:  "unless-stopped",
		Networks: []string{networkName},
	}
	for _, v := range spec.Volumes {
		svc.Volumes = append(svc.Volumes, v.Name+":"+v.MountPath)
	}
	if len(spec.Env) > 0 {
		keys := make([]string, 0, len(spec.Env))
		for k := range spec.Env {
			keys = append(keys, k)
		}
		svc.Environment = envRefs(keys)
	}
	if len(spec.Healthcheck.Test) > 0 {
		svc.Healthcheck = &composeHealthcheck{
			Test:     spec.Healthcheck.Test,
			Interval: spec.Healthcheck.Interval,
			Timeout:  spec.Healthcheck.Timeout,
			Retries:  spec.Healthcheck.Retries,
		}
	}
	return svc, true
}

// wireDependsOn adds depends_on: {..., condition: service_healthy} to every
// projected service, derived from the graph's depends_on edges — but only
// for targets that actually landed in the compose file. A depends_on edge
// to a node whose adapter isn't implemented yet (e.g. cache:redis) would
// otherwise reference an undefined service and break `docker compose config`.
func wireDependsOn(g *graph.Graph, nodes []*graph.Node, services map[string]composeService) {
	for _, n := range nodes {
		svc, ok := services[n.ID]
		if !ok {
			continue
		}
		for _, dep := range g.DependenciesOf(n.ID) {
			if _, ok := services[dep.ID]; !ok {
				continue
			}
			if svc.DependsOn == nil {
				svc.DependsOn = make(map[string]composeDependsOn)
			}
			svc.DependsOn[dep.ID] = composeDependsOn{Condition: "service_healthy"}
		}
		services[n.ID] = svc
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// envRefs turns a set of env var keys into ${VAR} references — the
// Connectable convention this PRD requires: compose never carries a literal
// secret, only a reference the target environment resolves.
func envRefs(keys []string) map[string]string {
	if len(keys) == 0 {
		return nil
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = "${" + k + "}"
	}
	return out
}

func envVarKeys(vars []adapter.EnvVar) []string {
	keys := make([]string, len(vars))
	for i, v := range vars {
		keys[i] = v.Key
	}
	return keys
}

func imageRef(spec adapter.ContainerSpec) string {
	if spec.Tag == "" {
		return spec.Image
	}
	return spec.Image + ":" + spec.Tag
}

// volumeNameFromMount extracts the volume name from a compose "name:path"
// volume mount string.
func volumeNameFromMount(mount string) string {
	if i := strings.IndexByte(mount, ':'); i >= 0 {
		return mount[:i]
	}
	return mount
}
