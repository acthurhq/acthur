package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/acthurhq/acthur/internal/deploy"
	"github.com/acthurhq/acthur/internal/graph"
)

type testMode uint8

const (
	testModeUnit testMode = iota
	testModeCI
)

type goTestRunner func(root, node string, out io.Writer, env []string, args ...string) error

// runTest runs `go test ./...`, streamed live to out, in every buildable
// service node under root, or just the named one. It reuses
// buildableServiceNodes (cmd/acthur/deploy.go) — the same node-selection
// logic `acthur deploy`'s gate uses — and deploy.GoStream for the actual
// per-node go invocation, so `acthur test` adds no new process-invocation
// plumbing of its own.
func runTest(root string, g *graph.Graph, service string, out io.Writer) error {
	return runTestMode(root, g, service, out, testModeUnit, deploy.GoStream)
}

// runTestMode projects the user-selected test mode into one Go invocation per
// selected service. CI mode deliberately enables the race detector and
// disables cached results; it otherwise retains unit mode's continue-on-error
// behavior so the final error reports every failing node in one run.
func runTestMode(root string, g *graph.Graph, service string, out io.Writer, mode testMode, runner goTestRunner) error {
	nodes, err := testTargetNodes(root, g, service)
	if err != nil {
		return err
	}

	args := []string{"test", "./..."}
	if mode == testModeCI {
		args = append(args, "-race", "-count=1")
	}

	var failed []string
	for _, node := range nodes {
		_, _ = fmt.Fprintf(out, "--- go %s (%s) ---\n", strings.Join(args, " "), node)
		if err := runner(root, node, out, nil, args...); err != nil {
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
