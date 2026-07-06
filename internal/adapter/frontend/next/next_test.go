package next_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/adapter"
	_ "github.com/acthur/acthur/internal/adapter/frontend/next" // register ui:next
)

func mustResolve(t *testing.T) adapter.Adapter {
	t.Helper()
	a, err := adapter.Resolve("ui:next")
	if err != nil {
		t.Fatalf("expected ui:next to be registered, got: %v", err)
	}
	return a
}

// ---------------------------------------------------------------------------
// Core interface
// ---------------------------------------------------------------------------

func TestNext_Registered(t *testing.T) {
	mustResolve(t)
}

func TestNext_Name(t *testing.T) {
	a := mustResolve(t)
	if a.Name() != "ui:next" {
		t.Errorf("expected ui:next, got %q", a.Name())
	}
}

func TestNext_Category(t *testing.T) {
	a := mustResolve(t)
	if a.Category() != adapter.CategoryFrontend {
		t.Errorf("expected CategoryFrontend, got %q", a.Category())
	}
}

func TestNext_Detect_True(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{
  "name": "app",
  "dependencies": { "next": "^14.2.13" }
}`)
	a := mustResolve(t)
	if !a.Detect(dir) {
		t.Error("expected Detect to return true for package.json with next")
	}
}

func TestNext_Detect_False(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{
  "name": "app",
  "dependencies": { "astro": "^4.16.0" }
}`)
	a := mustResolve(t)
	if a.Detect(dir) {
		t.Error("expected Detect to return false for package.json without next")
	}
}

func TestNext_Detect_NoPackageJSON(t *testing.T) {
	dir := t.TempDir()
	a := mustResolve(t)
	if a.Detect(dir) {
		t.Error("expected Detect to return false when package.json is absent")
	}
}

func TestNext_EnvVars_NotEmpty(t *testing.T) {
	a := mustResolve(t)
	if len(a.EnvVars()) == 0 {
		t.Error("expected non-empty EnvVars")
	}
}

func TestNext_EnvVars_ContainsAppPort(t *testing.T) {
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
func TestNext_EnvVars_DoesNotDeclareDatabaseURL(t *testing.T) {
	a := mustResolve(t)
	for _, e := range a.EnvVars() {
		if e.Key == "DATABASE_URL" {
			t.Error("ui:next should not declare DATABASE_URL — it is a frontend node")
		}
	}
}

// ---------------------------------------------------------------------------
// Runnable
// ---------------------------------------------------------------------------

func TestNext_DevCommand_DefaultsToPort3000(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.DevCommand(nil)
	if cmd.Bin != "npm" {
		t.Errorf("expected npm, got %q", cmd.Bin)
	}
	if !containsArg(cmd.Args, "dev") {
		t.Errorf("expected npm run dev invocation, got %v", cmd.Args)
	}
	if !containsArg(cmd.Args, "3000") {
		t.Errorf("expected default port 3000 in args, got %v", cmd.Args)
	}
}

func TestNext_DevCommand_UsesAppPortFromEnv(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.DevCommand(map[string]string{"APP_PORT": "4000"})
	if !containsArg(cmd.Args, "4000") {
		t.Errorf("expected APP_PORT 4000 threaded into dev command args, got %v", cmd.Args)
	}
}

func TestNext_SelfReloads(t *testing.T) {
	a := mustResolve(t)
	sr, ok := a.(adapter.SelfReloader)
	if !ok {
		t.Fatal("ui:next does not implement SelfReloader")
	}
	if !sr.SelfReloads() {
		t.Error("expected ui:next to self-reload via next dev's Fast Refresh")
	}
}

func TestNext_BuildCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.BuildCommand(nil)
	if cmd.Bin != "npm" || !containsArg(cmd.Args, "build") {
		t.Errorf("expected npm run build, got %q %v", cmd.Bin, cmd.Args)
	}
}

func TestNext_TestCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.TestCommand(nil)
	if cmd.Bin != "npm" || !containsArg(cmd.Args, "test") {
		t.Errorf("expected npm test, got %q %v", cmd.Bin, cmd.Args)
	}
}

// ---------------------------------------------------------------------------
// Scaffold
// ---------------------------------------------------------------------------

func TestNext_Scaffold_ProducesFiles(t *testing.T) {
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
		"package.json": false, "next.config.mjs": false, "tsconfig.json": false,
		"app/layout.tsx": false, "app/page.tsx": false, "app/health/route.ts": false,
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

func TestNext_Scaffold_FilesHaveContent(t *testing.T) {
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

func TestNext_Scaffold_PackageJSONIsValidAndNameMatchesNodeID(t *testing.T) {
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
	if _, ok := parsed.Dependencies["next"]; !ok {
		t.Error("expected package.json to declare a next dependency")
	}
	if parsed.Scripts["dev"] == "" || parsed.Scripts["build"] == "" || parsed.Scripts["test"] == "" {
		t.Errorf("expected dev/build/test npm scripts, got %+v", parsed.Scripts)
	}
}

func TestNext_Scaffold_ConfigUsesStandaloneOutput(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapp", NodeID: "web"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	cfg := fileContent(files, "next.config.mjs")
	if !strings.Contains(cfg, "output: 'standalone'") {
		t.Error("expected next.config.mjs to set output: 'standalone' for a self-contained production server")
	}
}

func TestNext_Scaffold_HealthRouteServesKernelContract(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapp", NodeID: "web"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	health := fileContent(files, "app/health/route.ts")
	if !strings.Contains(health, "export async function GET") {
		t.Error("expected app/health/route.ts to export a GET handler")
	}
	if !strings.Contains(health, "uptime_seconds") {
		t.Error("expected health route response to include uptime_seconds, matching every other adapter's health contract")
	}
	if !strings.Contains(health, "NextResponse") {
		t.Error("expected app/health/route.ts to use NextResponse")
	}
}

func TestNext_Scaffold_PackageJSONIsValidJSON(t *testing.T) {
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

func TestNext_Scaffold_TSConfigIsValidJSON(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapp", NodeID: "web"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	var v any
	if err := json.Unmarshal([]byte(fileContent(files, "tsconfig.json")), &v); err != nil {
		t.Fatalf("tsconfig.json is not valid JSON: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Dockerizable
// ---------------------------------------------------------------------------

func TestNext_SatisfiesDockerizable(t *testing.T) {
	a := mustResolve(t)
	if _, ok := a.(adapter.Dockerizable); !ok {
		t.Error("ui:next does not satisfy Dockerizable")
	}
}

func TestNext_DockerfileFor_MultiStageBuild(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "web", Port: 3000})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	content := string(out)
	if strings.Count(content, "FROM ") < 2 {
		t.Error("expected multi-stage build (2+ FROM statements)")
	}
}

func TestNext_DockerfileFor_StandaloneOutputCopied(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "web", Port: 3000})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	content := string(out)
	if !strings.Contains(content, ".next/standalone") {
		t.Error("expected the runtime stage to copy .next/standalone")
	}
	if !strings.Contains(content, "server.js") {
		t.Error("expected the entrypoint to run the standalone server.js")
	}
}

func TestNext_DockerfileFor_NonRootUser(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "web", Port: 3000})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	if !strings.Contains(string(out), "USER acthur") {
		t.Error("expected non-root USER directive")
	}
}

func TestNext_DockerfileFor_ExposesNodePort(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "web", Port: 9090})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	if !strings.Contains(string(out), "EXPOSE 9090") {
		t.Errorf("expected EXPOSE 9090, got:\n%s", out)
	}
}

func TestNext_DockerfileFor_HealthcheckAgainstIPv4Loopback(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "web", Port: 3000})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	content := string(out)
	if !strings.Contains(content, "127.0.0.1:3000/health") {
		t.Errorf("expected HEALTHCHECK against 127.0.0.1 (never localhost — alpine resolves it to ::1), got:\n%s", content)
	}
	if strings.Contains(content, "localhost") {
		t.Error("HEALTHCHECK must never use localhost — alpine resolves it to ::1")
	}
}

func TestNext_DockerfileFor_NoPort_OmitsExposeAndHealthcheck(t *testing.T) {
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

func TestNext_DockerfileFor_Deterministic(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	ctx := adapter.DockerfileContext{NodeID: "web", Port: 3000}
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

func TestNext_DoesNotSatisfyContainerized(t *testing.T) {
	a := mustResolve(t)
	if _, ok := a.(adapter.Containerized); ok {
		t.Error("ui:next should not satisfy Containerized — it builds from project source, not an upstream image")
	}
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

func TestCapabilitiesOf_Next_ExactlyScaffoldRunAndDockerize(t *testing.T) {
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
		t.Fatal("ui:next does not implement Scaffolder")
	}
	return sc
}

func mustResolveRunnable(t *testing.T) adapter.Runnable {
	t.Helper()
	a := mustResolve(t)
	r, ok := a.(adapter.Runnable)
	if !ok {
		t.Fatal("ui:next does not implement Runnable")
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
