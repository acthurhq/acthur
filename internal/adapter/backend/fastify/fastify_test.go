package fastify_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/adapter"
	_ "github.com/acthur/acthur/internal/adapter/backend/fastify" // register node:fastify
)

func mustResolve(t *testing.T) adapter.Adapter {
	t.Helper()
	a, err := adapter.Resolve("node:fastify")
	if err != nil {
		t.Fatalf("expected node:fastify to be registered, got: %v", err)
	}
	return a
}

// ---------------------------------------------------------------------------
// Core interface
// ---------------------------------------------------------------------------

func TestFastify_Registered(t *testing.T) {
	mustResolve(t)
}

func TestFastify_Name(t *testing.T) {
	a := mustResolve(t)
	if a.Name() != "node:fastify" {
		t.Errorf("expected node:fastify, got %q", a.Name())
	}
}

func TestFastify_Category(t *testing.T) {
	a := mustResolve(t)
	if a.Category() != adapter.CategoryBackend {
		t.Errorf("expected CategoryBackend, got %q", a.Category())
	}
}

func TestFastify_Detect_True(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{
  "name": "app",
  "dependencies": { "fastify": "^4.28.1" }
}`)
	a := mustResolve(t)
	if !a.Detect(dir) {
		t.Error("expected Detect to return true for package.json with fastify")
	}
}

func TestFastify_Detect_False(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{
  "name": "app",
  "dependencies": { "express": "^4.19.2" }
}`)
	a := mustResolve(t)
	if a.Detect(dir) {
		t.Error("expected Detect to return false for package.json without fastify")
	}
}

func TestFastify_Detect_NoPackageJSON(t *testing.T) {
	dir := t.TempDir()
	a := mustResolve(t)
	if a.Detect(dir) {
		t.Error("expected Detect to return false when package.json is absent")
	}
}

func TestFastify_EnvVars_NotEmpty(t *testing.T) {
	a := mustResolve(t)
	if len(a.EnvVars()) == 0 {
		t.Error("expected non-empty EnvVars")
	}
}

func TestFastify_EnvVars_ContainsDatabaseURL(t *testing.T) {
	a := mustResolve(t)
	found := false
	for _, e := range a.EnvVars() {
		if e.Key == "DATABASE_URL" {
			found = true
		}
	}
	if !found {
		t.Error("expected EnvVars to contain DATABASE_URL")
	}
}

// ---------------------------------------------------------------------------
// Runnable
// ---------------------------------------------------------------------------

func TestFastify_DevCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.DevCommand(nil)
	if cmd.Bin != "node" {
		t.Errorf("expected node, got %q", cmd.Bin)
	}
	if !containsArg(cmd.Args, "--watch") {
		t.Errorf("expected node --watch invocation, got %v", cmd.Args)
	}
}

func TestFastify_SelfReloads(t *testing.T) {
	a := mustResolve(t)
	sr, ok := a.(adapter.SelfReloader)
	if !ok {
		t.Fatal("node:fastify does not implement SelfReloader")
	}
	if !sr.SelfReloads() {
		t.Error("expected node:fastify to self-reload via node --watch")
	}
}

func TestFastify_BuildCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.BuildCommand(nil)
	if cmd.Bin != "npm" || !containsArg(cmd.Args, "ci") {
		t.Errorf("expected npm ci --omit=dev, got %q %v", cmd.Bin, cmd.Args)
	}
}

func TestFastify_TestCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.TestCommand(nil)
	if cmd.Bin != "npm" || !containsArg(cmd.Args, "test") {
		t.Errorf("expected npm test, got %q %v", cmd.Bin, cmd.Args)
	}
}

// ---------------------------------------------------------------------------
// Scaffold
// ---------------------------------------------------------------------------

func TestFastify_Scaffold_ProducesFiles(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{
		ProjectName: "testapi",
		NodeID:      "api",
		IDStrategy:  "ulid",
	})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected non-empty file set")
	}
	want := map[string]bool{
		"package.json": false, "src/index.js": false, "Dockerfile": false,
	}
	for _, f := range files {
		if _, ok := want[f.Path]; ok {
			want[f.Path] = true
		}
	}
	for p, ok := range want {
		if !ok {
			t.Errorf("expected scaffold to produce %s", p)
		}
	}
}

func TestFastify_Scaffold_FilesHaveContent(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapi", NodeID: "api", IDStrategy: "ulid"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	for _, f := range files {
		if len(f.Content) == 0 {
			t.Errorf("file %s has empty content", f.Path)
		}
	}
}

func TestFastify_Scaffold_PackageJSONIsValidAndNameMatchesNodeID(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapi", NodeID: "myservice", IDStrategy: "ulid"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	pkg := fileContent(files, "package.json")
	var parsed struct {
		Name         string            `json:"name"`
		Dependencies map[string]string `json:"dependencies"`
		Scripts      map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(pkg), &parsed); err != nil {
		t.Fatalf("package.json is not valid JSON: %v\n%s", err, pkg)
	}
	if parsed.Name != "myservice" {
		t.Errorf("expected package.json name to match NodeID, got %q", parsed.Name)
	}
	if _, ok := parsed.Dependencies["fastify"]; !ok {
		t.Error("expected package.json to declare a fastify dependency")
	}
	if parsed.Scripts["dev"] == "" || parsed.Scripts["build"] == "" || parsed.Scripts["test"] == "" {
		t.Errorf("expected dev/build/test npm scripts, got %+v", parsed.Scripts)
	}
}

func TestFastify_Scaffold_IDStrategy_ULID(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapi", NodeID: "api", IDStrategy: "ulid"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	ids := fileContent(files, "src/ids.js")
	if !strings.Contains(ids, "ULID") {
		t.Errorf("expected ULID strategy content, got:\n%s", ids)
	}
}

func TestFastify_Scaffold_IDStrategy_UUID(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapi", NodeID: "api", IDStrategy: "uuid-v4"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	ids := fileContent(files, "src/ids.js")
	if !strings.Contains(ids, "randomUUID") {
		t.Errorf("expected uuid-v4 strategy content, got:\n%s", ids)
	}
}

// Syntax validity is the strongest assertion available without a network
// `npm install` (skipped per verification scope — see status note): every
// generated .js file must parse under `node --check`. Mirrors the intent of
// go:fiber's compile test and rust:axum's `cargo check` test.
func TestFastify_Scaffold_AllJSFilesPassNodeCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping syntax-check test in short mode")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	sc := mustResolveScaffolder(t)
	dir := t.TempDir()
	files, err := sc.Scaffold(adapter.ScaffoldContext{
		ProjectName: "testapi",
		NodeID:      "api",
		IDStrategy:  "ulid",
	})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	writeFiles(t, dir, files)

	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".js") {
			continue
		}
		check := exec.Command("node", "--check", f.Path)
		check.Dir = dir
		if out, err := check.CombinedOutput(); err != nil {
			t.Errorf("node --check %s failed: %v\n%s", f.Path, err, out)
		}
	}
}

// ---------------------------------------------------------------------------
// Dockerizable
// ---------------------------------------------------------------------------

func TestFastify_SatisfiesDockerizable(t *testing.T) {
	a := mustResolve(t)
	if _, ok := a.(adapter.Dockerizable); !ok {
		t.Error("node:fastify does not satisfy Dockerizable")
	}
}

func TestFastify_DockerfileFor_MultiStageBuild(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "api", Port: 8080})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	content := string(out)
	if strings.Count(content, "FROM ") < 2 {
		t.Error("expected multi-stage build (2+ FROM statements)")
	}
}

func TestFastify_DockerfileFor_NonRootUser(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "api", Port: 8080})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	if !strings.Contains(string(out), "USER node") {
		t.Error("expected non-root USER directive")
	}
}

func TestFastify_DockerfileFor_ExposesNodePort(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "api", Port: 9090})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	if !strings.Contains(string(out), "EXPOSE 9090") {
		t.Errorf("expected EXPOSE 9090, got:\n%s", out)
	}
}

func TestFastify_DockerfileFor_HealthcheckAgainstIPv4Loopback(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "api", Port: 8080})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	content := string(out)
	if !strings.Contains(content, "127.0.0.1:8080/health") {
		t.Errorf("expected HEALTHCHECK against 127.0.0.1 (never localhost — alpine resolves it to ::1), got:\n%s", content)
	}
	if strings.Contains(content, "localhost") {
		t.Error("HEALTHCHECK must never use localhost — alpine resolves it to ::1")
	}
}

func TestFastify_DockerfileFor_NoPort_OmitsExposeAndHealthcheck(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "worker", Port: 0})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	content := string(out)
	if strings.Contains(content, "EXPOSE") || strings.Contains(content, "HEALTHCHECK") {
		t.Errorf("expected no EXPOSE/HEALTHCHECK for a portless node, got:\n%s", content)
	}
}

func TestFastify_DockerfileFor_Deterministic(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	ctx := adapter.DockerfileContext{NodeID: "api", Port: 8080}
	out1, err := a.DockerfileFor(ctx)
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	out2, err := a.DockerfileFor(ctx)
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	if !bytes.Equal(out1, out2) {
		t.Error("expected DockerfileFor to be deterministic across identical calls")
	}
}

func TestFastify_DoesNotSatisfyContainerized(t *testing.T) {
	a := mustResolve(t)
	if _, ok := a.(adapter.Containerized); ok {
		t.Error("node:fastify should not satisfy Containerized — it builds from project source, not an upstream image")
	}
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

func TestCapabilitiesOf_Fastify_ExactlyScaffoldRunAndDockerize(t *testing.T) {
	a := mustResolve(t)
	caps := adapter.CapabilitiesOf(a)
	want := map[adapter.Capability]bool{
		adapter.CapabilityScaffold:  true,
		adapter.CapabilityRun:       true,
		adapter.CapabilityDockerize: true,
	}
	if len(caps) != len(want) {
		t.Fatalf("expected %d capabilities, got %d: %v", len(want), len(caps), caps)
	}
	for _, c := range caps {
		if !want[c] {
			t.Errorf("unexpected capability %v", c)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustResolveScaffolder(t *testing.T) adapter.Scaffolder {
	t.Helper()
	a := mustResolve(t)
	sc, ok := a.(adapter.Scaffolder)
	if !ok {
		t.Fatal("node:fastify does not implement Scaffolder")
	}
	return sc
}

func mustResolveRunnable(t *testing.T) adapter.Runnable {
	t.Helper()
	a := mustResolve(t)
	r, ok := a.(adapter.Runnable)
	if !ok {
		t.Fatal("node:fastify does not implement Runnable")
	}
	return r
}

func fileContent(files []adapter.File, path string) string {
	for _, f := range files {
		if f.Path == path {
			return string(f.Content)
		}
	}
	return ""
}

func writeFiles(t *testing.T, dir string, files []adapter.File) {
	t.Helper()
	for _, f := range files {
		path := filepath.Join(dir, f.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		mode := fs.FileMode(f.Mode)
		if mode == 0 {
			mode = 0644
		}
		if err := os.WriteFile(path, f.Content, mode); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
