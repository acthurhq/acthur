package astro_test

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
	_ "github.com/acthur/acthur/internal/adapter/frontend/astro" // register ui:astro
)

func mustResolve(t *testing.T) adapter.Adapter {
	t.Helper()
	a, err := adapter.Resolve("ui:astro")
	if err != nil {
		t.Fatalf("expected ui:astro to be registered, got: %v", err)
	}
	return a
}

// ---------------------------------------------------------------------------
// Core interface
// ---------------------------------------------------------------------------

func TestAstro_Registered(t *testing.T) {
	mustResolve(t)
}

func TestAstro_Name(t *testing.T) {
	a := mustResolve(t)
	if a.Name() != "ui:astro" {
		t.Errorf("expected ui:astro, got %q", a.Name())
	}
}

func TestAstro_Category(t *testing.T) {
	a := mustResolve(t)
	if a.Category() != adapter.CategoryFrontend {
		t.Errorf("expected CategoryFrontend, got %q", a.Category())
	}
}

func TestAstro_Detect_True(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{
  "name": "app",
  "dependencies": { "astro": "^4.16.0" }
}`)
	a := mustResolve(t)
	if !a.Detect(dir) {
		t.Error("expected Detect to return true for package.json with astro")
	}
}

func TestAstro_Detect_False(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{
  "name": "app",
  "dependencies": { "next": "^14.2.13" }
}`)
	a := mustResolve(t)
	if a.Detect(dir) {
		t.Error("expected Detect to return false for package.json without astro")
	}
}

func TestAstro_Detect_NoPackageJSON(t *testing.T) {
	dir := t.TempDir()
	a := mustResolve(t)
	if a.Detect(dir) {
		t.Error("expected Detect to return false when package.json is absent")
	}
}

func TestAstro_EnvVars_NotEmpty(t *testing.T) {
	a := mustResolve(t)
	if len(a.EnvVars()) == 0 {
		t.Error("expected non-empty EnvVars")
	}
}

func TestAstro_EnvVars_ContainsAppPort(t *testing.T) {
	a := mustResolve(t)
	found := false
	for _, e := range a.EnvVars() {
		if e.Key == "APP_PORT" {
			found = true
		}
	}
	if !found {
		t.Error("expected EnvVars to contain APP_PORT")
	}
}

// A frontend node has no database of its own — it must not declare
// DATABASE_URL/APP_SECRET the way backend adapters do.
func TestAstro_EnvVars_DoesNotDeclareDatabaseURL(t *testing.T) {
	a := mustResolve(t)
	for _, e := range a.EnvVars() {
		if e.Key == "DATABASE_URL" {
			t.Error("ui:astro should not declare DATABASE_URL — it is a frontend node")
		}
	}
}

// ---------------------------------------------------------------------------
// Runnable
// ---------------------------------------------------------------------------

func TestAstro_DevCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.DevCommand(nil)
	if cmd.Bin != "npm" {
		t.Errorf("expected npm, got %q", cmd.Bin)
	}
	if !containsArg(cmd.Args, "dev") {
		t.Errorf("expected npm run dev invocation, got %v", cmd.Args)
	}
}

func TestAstro_SelfReloads(t *testing.T) {
	a := mustResolve(t)
	sr, ok := a.(adapter.SelfReloader)
	if !ok {
		t.Fatal("ui:astro does not implement SelfReloader")
	}
	if !sr.SelfReloads() {
		t.Error("expected ui:astro to self-reload via astro dev's Vite-powered server")
	}
}

func TestAstro_BuildCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.BuildCommand(nil)
	if cmd.Bin != "npm" || !containsArg(cmd.Args, "build") {
		t.Errorf("expected npm run build, got %q %v", cmd.Bin, cmd.Args)
	}
}

func TestAstro_TestCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.TestCommand(nil)
	if cmd.Bin != "npm" || !containsArg(cmd.Args, "test") {
		t.Errorf("expected npm test, got %q %v", cmd.Bin, cmd.Args)
	}
}

// ---------------------------------------------------------------------------
// Scaffold
// ---------------------------------------------------------------------------

func TestAstro_Scaffold_ProducesFiles(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{
		ProjectName: "testapp",
		NodeID:      "web",
	})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected non-empty file set")
	}
	want := map[string]bool{
		"package.json": false, "astro.config.mjs": false,
		"src/pages/index.astro": false, "src/pages/health.js": false,
		"Dockerfile": false,
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

func TestAstro_Scaffold_FilesHaveContent(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapp", NodeID: "web"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	for _, f := range files {
		if len(f.Content) == 0 {
			t.Errorf("file %s has empty content", f.Path)
		}
	}
}

func TestAstro_Scaffold_PackageJSONIsValidAndNameMatchesNodeID(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapp", NodeID: "myweb"})
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
	if parsed.Name != "myweb" {
		t.Errorf("expected package.json name to match NodeID, got %q", parsed.Name)
	}
	if _, ok := parsed.Dependencies["astro"]; !ok {
		t.Error("expected package.json to declare an astro dependency")
	}
	if _, ok := parsed.Dependencies["@astrojs/node"]; !ok {
		t.Error("expected package.json to declare an @astrojs/node dependency for SSR")
	}
	if parsed.Scripts["dev"] == "" || parsed.Scripts["build"] == "" || parsed.Scripts["test"] == "" {
		t.Errorf("expected dev/build/test npm scripts, got %+v", parsed.Scripts)
	}
}

func TestAstro_Scaffold_ConfigUsesServerOutputAndReadsPortFromEnv(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapp", NodeID: "web"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	cfg := fileContent(files, "astro.config.mjs")
	if !strings.Contains(cfg, "output: 'server'") {
		t.Error("expected astro.config.mjs to set output: 'server' so /health can be an SSR endpoint")
	}
	if !strings.Contains(cfg, "@astrojs/node") {
		t.Error("expected astro.config.mjs to import the @astrojs/node adapter")
	}
	if !strings.Contains(cfg, "process.env.PORT") {
		t.Error("expected astro.config.mjs to read the port from process.env.PORT (set by the dev engine)")
	}
}

func TestAstro_Scaffold_HealthEndpointServesKernelContract(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapp", NodeID: "web"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	health := fileContent(files, "src/pages/health.js")
	if !strings.Contains(health, "export async function GET") && !strings.Contains(health, "export function GET") {
		t.Error("expected src/pages/health.js to export a GET handler")
	}
	if !strings.Contains(health, "'status'") && !strings.Contains(health, "status,") {
		t.Error("expected health.js response to include a status field")
	}
	if !strings.Contains(health, "uptime_seconds") {
		t.Error("expected health.js response to include uptime_seconds, matching every other adapter's health contract")
	}
	if !strings.Contains(health, "prerender = false") {
		t.Error("expected health.js to disable prerendering so it runs as a live SSR endpoint")
	}
}

// Syntax validity is the strongest assertion available without a network
// `npm install` (time-boxed per verification scope): every generated .js
// file must parse under `node --check`. Mirrors node:fastify's equivalent.
func TestAstro_Scaffold_AllJSFilesPassNodeCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping syntax-check test in short mode")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	sc := mustResolveScaffolder(t)
	dir := t.TempDir()
	files, err := sc.Scaffold(adapter.ScaffoldContext{
		ProjectName: "testapp",
		NodeID:      "web",
	})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	writeFiles(t, dir, files)

	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".js") && !strings.HasSuffix(f.Path, ".mjs") {
			continue
		}
		check := exec.Command("node", "--check", f.Path)
		check.Dir = dir
		if out, err := check.CombinedOutput(); err != nil {
			t.Errorf("node --check %s failed: %v\n%s", f.Path, err, out)
		}
	}
}

func TestAstro_Scaffold_PackageJSONIsValidJSON(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapp", NodeID: "web"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	var v any
	if err := json.Unmarshal([]byte(fileContent(files, "package.json")), &v); err != nil {
		t.Fatalf("package.json is not valid JSON: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Dockerizable
// ---------------------------------------------------------------------------

func TestAstro_SatisfiesDockerizable(t *testing.T) {
	a := mustResolve(t)
	if _, ok := a.(adapter.Dockerizable); !ok {
		t.Error("ui:astro does not satisfy Dockerizable")
	}
}

func TestAstro_DockerfileFor_MultiStageBuild(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "web", Port: 4321})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	content := string(out)
	if strings.Count(content, "FROM ") < 2 {
		t.Error("expected multi-stage build (2+ FROM statements)")
	}
}

func TestAstro_DockerfileFor_NonRootUser(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "web", Port: 4321})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	if !strings.Contains(string(out), "USER acthur") {
		t.Error("expected non-root USER directive")
	}
}

func TestAstro_DockerfileFor_ExposesNodePort(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "web", Port: 9090})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	if !strings.Contains(string(out), "EXPOSE 9090") {
		t.Errorf("expected EXPOSE 9090, got:\n%s", out)
	}
}

func TestAstro_DockerfileFor_HealthcheckAgainstIPv4Loopback(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "web", Port: 4321})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	content := string(out)
	if !strings.Contains(content, "127.0.0.1:4321/health") {
		t.Errorf("expected HEALTHCHECK against 127.0.0.1 (never localhost — alpine resolves it to ::1), got:\n%s", content)
	}
	if strings.Contains(content, "localhost") {
		t.Error("HEALTHCHECK must never use localhost — alpine resolves it to ::1")
	}
}

func TestAstro_DockerfileFor_NoPort_OmitsExposeAndHealthcheck(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "web", Port: 0})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	content := string(out)
	if strings.Contains(content, "EXPOSE") || strings.Contains(content, "HEALTHCHECK") {
		t.Errorf("expected no EXPOSE/HEALTHCHECK for a portless node, got:\n%s", content)
	}
}

func TestAstro_DockerfileFor_Deterministic(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	ctx := adapter.DockerfileContext{NodeID: "web", Port: 4321}
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

func TestAstro_DoesNotSatisfyContainerized(t *testing.T) {
	a := mustResolve(t)
	if _, ok := a.(adapter.Containerized); ok {
		t.Error("ui:astro should not satisfy Containerized — it builds from project source, not an upstream image")
	}
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

func TestCapabilitiesOf_Astro_ExactlyScaffoldRunAndDockerize(t *testing.T) {
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
		t.Fatal("ui:astro does not implement Scaffolder")
	}
	return sc
}

func mustResolveRunnable(t *testing.T) adapter.Runnable {
	t.Helper()
	a := mustResolve(t)
	r, ok := a.(adapter.Runnable)
	if !ok {
		t.Fatal("ui:astro does not implement Runnable")
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
