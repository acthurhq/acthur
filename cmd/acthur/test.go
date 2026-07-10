package main

import (
	"fmt"
	"io"

	"github.com/acthur/acthur/internal/deploy"
	"github.com/acthur/acthur/internal/graph"
)

// runTest runs `go test ./...`, streamed live to out, in every buildable
// service node under root, or just the named one. It reuses
// buildableServiceNodes (cmd/acthur/deploy.go) — the same node-selection
// logic `acthur deploy`'s gate uses — and deploy.GoStream for the actual
// per-node go invocation, so `acthur test` adds no new process-invocation
// plumbing of its own.
func runTest(root string, g *graph.Graph, service string, out io.Writer) error {
	nodes, err := testTargetNodes(root, g, service)
	if err != nil {
		return err
	}

	var failed []string
	for _, node := range nodes {
		_, _ = fmt.Fprintf(out, "--- go test ./... (%s) ---\n", node)
		if err := deploy.GoStream(root, node, out, nil, "test", "./..."); err != nil {
			failed = append(failed, node)
			_, _ = fmt.Fprintf(out, "FAIL %s: %v\n", node, err)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("tests failed in: %v", failed)
	}
	return nil
}

// testTargetNodes resolves which service node(s) `acthur test` should run
// against: the single named service (validated as buildable), or every
// buildable service node when service is empty.
func testTargetNodes(root string, g *graph.Graph, service string) ([]string, error) {
	all := buildableServiceNodes(root, g)
	if service == "" {
		if len(all) == 0 {
			return nil, fmt.Errorf("no service nodes with a go.mod found under %s", root)
		}
		return all, nil
	}
	for _, n := range all {
		if n == service {
			return []string{n}, nil
		}
	}
	return nil, fmt.Errorf("service %q not found (or has no go.mod) among the graph's service nodes", service)
}
