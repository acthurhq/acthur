package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/acthur/acthur/internal/deploy"
	"github.com/acthur/acthur/internal/graph"
)

// buildOutputDir is where production binaries land, relative to the
// project root — docs/acthur-prd.md §19.1 "acthur build".
const buildOutputDir = ".acthur/build"

// runBuild compiles every buildable service node (or fails naming why there
// are none) into a static production binary under
// root/.acthur/build/<node>, printing each binary's size as it finishes.
// It reuses buildableServiceNodes (cmd/acthur/deploy.go) — the same
// "which service nodes have a go.mod" logic `acthur deploy`'s gate already
// uses — and deploy.GoStream for the actual per-node go invocation, so this
// command adds no new process-invocation plumbing of its own.
func runBuild(root string, g *graph.Graph, out io.Writer) error {
	nodes := buildableServiceNodes(root, g)
	if len(nodes) == 0 {
		return fmt.Errorf("no service nodes with a go.mod found under %s — nothing to build", root)
	}

	outDir := filepath.Join(root, buildOutputDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating build output directory: %w", err)
	}

	for _, node := range nodes {
		binPath := filepath.Join(outDir, node)
		_, _ = fmt.Fprintf(out, "--- building %s ---\n", node)
		// CGO_ENABLED=0: a fully static binary, no libc dependency in the
		// deploy image. -ldflags="-s -w": strip debug/symbol info to shrink
		// the binary — standard production Go build flags.
		if err := deploy.GoStream(root, node, out, []string{"CGO_ENABLED=0"}, "build", "-ldflags=-s -w", "-o", binPath, "."); err != nil {
			return fmt.Errorf("building node %q: %w", node, err)
		}
		info, err := os.Stat(binPath)
		if err != nil {
			return fmt.Errorf("stat built binary %s: %w", binPath, err)
		}
		_, _ = fmt.Fprintf(out, "%-20s %8.2f MB  %s\n", node, float64(info.Size())/(1024*1024), binPath)
	}
	return nil
}
