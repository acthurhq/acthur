package deploy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/graph"
)

// ValidateRemoteProjection verifies that a remote target can faithfully
// execute the declared production graph. A target must fail closed when it
// cannot deliver workload configuration or would deploy only the service
// subset of a graph.
func ValidateRemoteProjection(target string, g *graph.Graph, workloadEnv []string) error {
	if target == "compose" {
		return nil
	}
	if target == "coolify" {
		return nil // Coolify receives Compose topology and environment via API.
	}
	if len(workloadEnv) > 0 {
		vars := append([]string(nil), workloadEnv...)
		sort.Strings(vars)
		return fmt.Errorf("target %q cannot deliver workload environment %s; use compose or add provider environment delivery before deploying", target, strings.Join(vars, ", "))
	}
	if target != "fly" && target != "railway" && target != "render" {
		return nil // Unknown targets are diagnosed by command dispatch.
	}

	var infra []string
	for _, node := range g.Nodes() {
		if node.Type == config.NodeTypeInfra && !strings.HasPrefix(node.Adapter, "kernel:") {
			infra = append(infra, node.ID)
		}
	}
	sort.Strings(infra)
	if len(infra) > 0 {
		return fmt.Errorf("target %q cannot preserve infrastructure nodes %s; use compose/coolify or add provider infrastructure projection", target, strings.Join(infra, ", "))
	}

	var dependencies []string
	for _, edge := range g.EdgesOfType(config.EdgeDependsOn) {
		dependencies = append(dependencies, edge.From+" -> "+edge.To)
	}
	sort.Strings(dependencies)
	if len(dependencies) > 0 {
		return fmt.Errorf("target %q cannot preserve dependency topology %s; use compose/coolify or add provider dependency projection", target, strings.Join(dependencies, ", "))
	}
	return nil
}
