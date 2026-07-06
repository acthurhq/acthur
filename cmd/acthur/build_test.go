package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunBuild_BuildsEachServiceNode_ToOutputDir: a project with one
// buildable go:fiber service node produces a static binary under
// .acthur/build/<node> and reports its size.
func TestRunBuild_BuildsEachServiceNode_ToOutputDir(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)
	writeBuildableAPINode(t, dir)

	_, g, err := loadProjectGraph(dir)
	if err != nil {
		t.Fatalf("loadProjectGraph: %v", err)
	}

	var out bytes.Buffer
	if err := runBuild(dir, g, &out); err != nil {
		t.Fatalf("runBuild: %v (output: %s)", err, out.String())
	}

	binPath := filepath.Join(dir, buildOutputDir, "api")
	if _, err := os.Stat(binPath); err != nil {
		t.Errorf("expected built binary at %s: %v", binPath, err)
	}
	if !strings.Contains(out.String(), "api") {
		t.Errorf("expected build output to mention node %q, got: %s", "api", out.String())
	}
}

// TestRunBuild_NoServiceNodes_ReturnsPointedError: a project with no
// buildable service node (no go.mod present) fails naming why there's
// nothing to build, rather than silently succeeding.
func TestRunBuild_NoServiceNodes_ReturnsPointedError(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)

	_, g, err := loadProjectGraph(dir)
	if err != nil {
		t.Fatalf("loadProjectGraph: %v", err)
	}

	var out bytes.Buffer
	err = runBuild(dir, g, &out)
	if err == nil {
		t.Fatal("expected an error when no service node has a go.mod")
	}
	if !strings.Contains(err.Error(), "nothing to build") {
		t.Errorf("expected pointed error, got: %v", err)
	}
}

// TestRunBuild_BuildFailure_ReturnsNodeNamedError: a service node that
// fails to compile surfaces its node ID in the returned error, and no
// binary is left behind for it.
func TestRunBuild_BuildFailure_ReturnsNodeNamedError(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)
	root := filepath.Join(dir, "api")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/api\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { undefinedSymbol() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, g, err := loadProjectGraph(dir)
	if err != nil {
		t.Fatalf("loadProjectGraph: %v", err)
	}

	var out bytes.Buffer
	err = runBuild(dir, g, &out)
	if err == nil {
		t.Fatal("expected build failure for a node that doesn't compile")
	}
	if !strings.Contains(err.Error(), "api") {
		t.Errorf("expected node name in error, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, buildOutputDir, "api")); !os.IsNotExist(statErr) {
		t.Error("expected no binary to be left behind for a failed build")
	}
}
