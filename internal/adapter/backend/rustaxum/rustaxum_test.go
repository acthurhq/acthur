package rustaxum_test

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/adapter"
	_ "github.com/acthurhq/acthur/internal/adapter/backend/rustaxum" // register rust:axum
)

func mustResolve(t *testing.T) adapter.Adapter {
	t.Helper()
	a, err := adapter.Resolve("rust:axum")
	if err != nil {
		t.Fatalf("expected rust:axum to be registered, got: %v", err)
	}
	return a
}

// ---------------------------------------------------------------------------
// Core interface
// ---------------------------------------------------------------------------

func TestRustAxum_Registered(t *testing.T) {
	mustResolve(t)
}

func TestRustAxum_Name(t *testing.T) {
	a := mustResolve(t)
	if a.Name() != "rust:axum" {
		t.Errorf("expected rust:axum, got %q", a.Name())
	}
}

func TestRustAxum_Category(t *testing.T) {
	a := mustResolve(t)
	if a.Category() != adapter.CategoryBackend {
		t.Errorf("expected CategoryBackend, got %q", a.Category())
	}
}

func TestRustAxum_Detect_True(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Cargo.toml"), `[package]
name = "app"
version = "0.1.0"
edition = "2021"

[dependencies]
axum = "0.7"
`)
	a := mustResolve(t)
	if !a.Detect(dir) {
		t.Error("expected Detect to return true for Cargo.toml with axum")
	}
}

func TestRustAxum_Detect_False(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Cargo.toml"), `[package]
name = "app"
version = "0.1.0"
edition = "2021"

[dependencies]
actix-web = "4"
`)
	a := mustResolve(t)
	if a.Detect(dir) {
		t.Error("expected Detect to return false for Cargo.toml without axum")
	}
}

func TestRustAxum_Detect_NoCargoToml(t *testing.T) {
	dir := t.TempDir()
	a := mustResolve(t)
	if a.Detect(dir) {
		t.Error("expected Detect to return false when Cargo.toml is absent")
	}
}

func TestRustAxum_EnvVars_NotEmpty(t *testing.T) {
	a := mustResolve(t)
	if len(a.EnvVars()) == 0 {
		t.Error("expected non-empty EnvVars")
	}
}

func TestRustAxum_EnvVars_ContainsDatabaseURL(t *testing.T) {
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

func TestRustAxum_DevCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.DevCommand(nil)
	if cmd.Bin != "cargo" {
		t.Errorf("expected cargo, got %q", cmd.Bin)
	}
	if !containsArg(cmd.Args, "watch") {
		t.Errorf("expected cargo-watch invocation, got %v", cmd.Args)
	}
}

func TestRustAxum_SelfReloads(t *testing.T) {
	a := mustResolve(t)
	sr, ok := a.(adapter.SelfReloader)
	if !ok {
		t.Fatal("rust:axum does not implement SelfReloader")
	}
	if !sr.SelfReloads() {
		t.Error("expected rust:axum to self-reload via cargo-watch")
	}
}

func TestRustAxum_BuildCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.BuildCommand(nil)
	if cmd.Bin != "cargo" || !containsArg(cmd.Args, "build") || !containsArg(cmd.Args, "--release") {
		t.Errorf("expected cargo build --release, got %q %v", cmd.Bin, cmd.Args)
	}
}

func TestRustAxum_TestCommand(t *testing.T) {
	a := mustResolveRunnable(t)
	cmd := a.TestCommand(nil)
	if cmd.Bin != "cargo" || !containsArg(cmd.Args, "test") {
		t.Errorf("expected cargo test, got %q %v", cmd.Bin, cmd.Args)
	}
}

// ---------------------------------------------------------------------------
// Scaffold
// ---------------------------------------------------------------------------

func TestRustAxum_Scaffold_ProducesFiles(t *testing.T) {
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
		"Cargo.toml": false, "src/main.rs": false, "Dockerfile": false,
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

func TestRustAxum_Scaffold_FilesHaveContent(t *testing.T) {
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

func TestRustAxum_Scaffold_CrateNameMatchesNodeID(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapi", NodeID: "myservice", IDStrategy: "ulid"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	var cargoToml string
	for _, f := range files {
		if f.Path == "Cargo.toml" {
			cargoToml = string(f.Content)
		}
	}
	if !strings.Contains(cargoToml, `name = "myservice"`) {
		t.Errorf("expected Cargo.toml package name to match NodeID, got:\n%s", cargoToml)
	}
}

func TestRustAxum_Scaffold_IDStrategy_ULID(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapi", NodeID: "api", IDStrategy: "ulid"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	ids := fileContent(files, "src/ids.rs")
	if !strings.Contains(ids, "ulid::Ulid") {
		t.Errorf("expected ULID strategy to use the ulid crate, got:\n%s", ids)
	}
}

func TestRustAxum_Scaffold_IDStrategy_UUID(t *testing.T) {
	sc := mustResolveScaffolder(t)
	files, err := sc.Scaffold(adapter.ScaffoldContext{ProjectName: "testapi", NodeID: "api", IDStrategy: "uuid-v4"})
	if err != nil {
		t.Fatalf("scaffold error: %v", err)
	}
	ids := fileContent(files, "src/ids.rs")
	if !strings.Contains(ids, "uuid::Uuid") {
		t.Errorf("expected uuid-v4 strategy to use the uuid crate, got:\n%s", ids)
	}
}

// Compilation is the strongest assertion — mirrors go:fiber's
// TestGoFiber_Scaffold_CompilesWithGoBuild. Requires cargo + network access
// to crates.io (or a warm registry cache); skipped in short mode.
func TestRustAxum_Scaffold_CompilesWithCargoCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compilation test in short mode")
	}
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo not installed")
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

	check := exec.Command("cargo", "check")
	check.Dir = dir
	if out, err := check.CombinedOutput(); err != nil {
		t.Fatalf("cargo check failed: %v\n%s", err, out)
	}
}

// ---------------------------------------------------------------------------
// Dockerizable
// ---------------------------------------------------------------------------

func TestRustAxum_SatisfiesDockerizable(t *testing.T) {
	a := mustResolve(t)
	if _, ok := a.(adapter.Dockerizable); !ok {
		t.Error("rust:axum does not satisfy Dockerizable")
	}
}

func TestRustAxum_DockerfileFor_MultiStageBuild(t *testing.T) {
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

func TestRustAxum_DockerfileFor_NonRootUser(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "api", Port: 8080})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	if !strings.Contains(string(out), "USER acthur") {
		t.Error("expected non-root USER directive")
	}
}

func TestRustAxum_DockerfileFor_ExposesNodePort(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "api", Port: 9090})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	if !strings.Contains(string(out), "EXPOSE 9090") {
		t.Errorf("expected EXPOSE 9090, got:\n%s", out)
	}
}

func TestRustAxum_DockerfileFor_HealthcheckAgainstIPv4Loopback(t *testing.T) {
	a := mustResolve(t).(adapter.Dockerizable)
	out, err := a.DockerfileFor(adapter.DockerfileContext{NodeID: "api", Port: 8080})
	if err != nil {
		t.Fatalf("DockerfileFor error: %v", err)
	}
	content := string(out)
	if !strings.Contains(content, "127.0.0.1:8080/health") {
		t.Errorf("expected HEALTHCHECK against 127.0.0.1 (never localhost — alpine/musl resolves it to ::1), got:\n%s", content)
	}
	if strings.Contains(content, "localhost") {
		t.Error("HEALTHCHECK must never use localhost — alpine/musl resolves it to ::1")
	}
}

func TestRustAxum_DockerfileFor_NoPort_OmitsExposeAndHealthcheck(t *testing.T) {
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

func TestRustAxum_DockerfileFor_Deterministic(t *testing.T) {
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

func TestRustAxum_DoesNotSatisfyContainerized(t *testing.T) {
	a := mustResolve(t)
	if _, ok := a.(adapter.Containerized); ok {
		t.Error("rust:axum should not satisfy Containerized — it builds from project source, not an upstream image")
	}
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

func TestCapabilitiesOf_RustAxum_ExactlyScaffoldRunAndDockerize(t *testing.T) {
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
		t.Fatal("rust:axum does not implement Scaffolder")
	}
	return sc
}

func mustResolveRunnable(t *testing.T) adapter.Runnable {
	t.Helper()
	a := mustResolve(t)
	r, ok := a.(adapter.Runnable)
	if !ok {
		t.Fatal("rust:axum does not implement Runnable")
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
