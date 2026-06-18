package adapter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acthur/acthur/internal/adapter"
	_ "github.com/acthur/acthur/internal/adapter/backend/gofiber" // register adapter
)

// ---------------------------------------------------------------------------
// Registry tests
// ---------------------------------------------------------------------------

func TestRegistry_GoFiberRegistered(t *testing.T) {
	a, err := adapter.Resolve("go:fiber")
	if err != nil {
		t.Fatalf("expected go:fiber to be registered, got: %v", err)
	}
	if a.Name() != "go:fiber" {
		t.Errorf("expected name go:fiber, got %q", a.Name())
	}
}

func TestRegistry_ResolveUnknown(t *testing.T) {
	_, err := adapter.Resolve("notaruntime:notaframework")
	if err == nil {
		t.Error("expected error resolving unknown adapter, got nil")
	}
}

func TestRegistry_ByCategory_Backend(t *testing.T) {
	backends := adapter.ByCategory(adapter.CategoryBackend)
	if len(backends) == 0 {
		t.Error("expected at least one backend adapter registered")
	}
	for _, b := range backends {
		if b.Category() != adapter.CategoryBackend {
			t.Errorf("adapter %q has wrong category %q", b.Name(), b.Category())
		}
	}
}

func TestRegistry_Names_NotEmpty(t *testing.T) {
	names := adapter.Names()
	if len(names) == 0 {
		t.Error("expected registered adapter names, got empty list")
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate registration, got none")
		}
	}()

	// Try to register go:fiber again — should panic
	a, _ := adapter.Resolve("go:fiber")
	adapter.Register(a)
}

// ---------------------------------------------------------------------------
// go:fiber adapter tests
// ---------------------------------------------------------------------------

func TestGoFiber_Name(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	if a.Name() != "go:fiber" {
		t.Errorf("expected go:fiber, got %q", a.Name())
	}
}

func TestGoFiber_Category(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	if a.Category() != adapter.CategoryBackend {
		t.Errorf("expected CategoryBackend, got %q", a.Category())
	}
}

func TestGoFiber_Detect_True(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/app

go 1.22

require github.com/gofiber/fiber/v2 v2.52.4
`)
	a := mustResolve(t, "go:fiber")
	if !a.Detect(dir) {
		t.Error("expected Detect to return true for go.mod with fiber")
	}
}

func TestGoFiber_Detect_False(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/app

go 1.22

require github.com/gin-gonic/gin v1.9.1
`)
	a := mustResolve(t, "go:fiber")
	if a.Detect(dir) {
		t.Error("expected Detect to return false for go.mod without fiber")
	}
}

func TestGoFiber_Detect_NoGoMod(t *testing.T) {
	dir := t.TempDir()
	a := mustResolve(t, "go:fiber")
	if a.Detect(dir) {
		t.Error("expected Detect to return false when no go.mod exists")
	}
}

func TestGoFiber_DevCommand(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	cmd := a.DevCommand(nil)
	if cmd.Bin != "air" {
		t.Errorf("expected dev command bin=air, got %q", cmd.Bin)
	}
	if len(cmd.Args) == 0 {
		t.Error("expected air to have arguments (-c .air.toml)")
	}
}

func TestGoFiber_BuildCommand(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	cmd := a.BuildCommand(nil)
	if cmd.Bin != "go" {
		t.Errorf("expected build command bin=go, got %q", cmd.Bin)
	}
}

func TestGoFiber_TestCommand(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	cmd := a.TestCommand(nil)
	if cmd.Bin != "go" {
		t.Errorf("expected test command bin=go, got %q", cmd.Bin)
	}
}

func TestGoFiber_GeneratorTargets_NotEmpty(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	targets := a.GeneratorTargets()
	if len(targets) == 0 {
		t.Error("expected generator targets, got empty list")
	}
}

func TestGoFiber_GeneratorTargets_ContainsCore(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	targets := a.GeneratorTargets()
	required := []string{"handler", "service", "repository", "migration"}
	for _, req := range required {
		found := false
		for _, t := range targets {
			if t == req {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in generator targets", req)
		}
	}
}

func TestGoFiber_EnvVars_NotEmpty(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	envVars := a.EnvVars()
	if len(envVars) == 0 {
		t.Error("expected env vars, got empty list")
	}
}

func TestGoFiber_EnvVars_ContainsDatabaseURL(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	for _, e := range a.EnvVars() {
		if e.Key == "DATABASE_URL" {
			return
		}
	}
	t.Error("expected DATABASE_URL in env vars")
}

func TestGoFiber_Scaffold_ProducesFiles(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	files, err := a.Scaffold(adapter.ScaffoldContext{
		ProjectName: "testproject",
		NodeID:      "api",
		RootDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("unexpected scaffold error: %v", err)
	}
	if len(files) == 0 {
		t.Error("expected scaffold to produce files")
	}
}

func TestGoFiber_Scaffold_ContainsMainGo(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	files, err := a.Scaffold(adapter.ScaffoldContext{
		ProjectName: "testproject",
		NodeID:      "api",
	})
	if err != nil {
		t.Fatalf("unexpected scaffold error: %v", err)
	}
	for _, f := range files {
		if f.Path == "main.go" {
			return
		}
	}
	t.Error("expected main.go in scaffolded files")
}

func TestGoFiber_Scaffold_ContainsDockerfile(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	files, err := a.Scaffold(adapter.ScaffoldContext{
		ProjectName: "testproject",
		NodeID:      "api",
	})
	if err != nil {
		t.Fatalf("unexpected scaffold error: %v", err)
	}
	for _, f := range files {
		if f.Path == "Dockerfile" {
			return
		}
	}
	t.Error("expected Dockerfile in scaffolded files")
}

func TestGoFiber_Scaffold_FilesHaveContent(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	files, err := a.Scaffold(adapter.ScaffoldContext{
		ProjectName: "testproject",
		NodeID:      "api",
	})
	if err != nil {
		t.Fatalf("unexpected scaffold error: %v", err)
	}
	for _, f := range files {
		if len(f.Content) == 0 {
			t.Errorf("file %q has empty content", f.Path)
		}
	}
}

func TestGoFiber_Dockerfile_ContainsPort(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	df := a.Dockerfile(adapter.BuildConfig{
		ProjectName: "testproject",
		NodeID:      "api",
		Port:        8080,
	})
	if !containsStr(df, "8080") {
		t.Error("expected Dockerfile to contain port 8080")
	}
}

func TestGoFiber_Dockerfile_IsMultiStage(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	df := a.Dockerfile(adapter.BuildConfig{ProjectName: "test", NodeID: "api", Port: 8080})
	if !containsStr(df, "FROM golang:") {
		t.Error("expected Dockerfile to have Go build stage")
	}
	if !containsStr(df, "FROM gcr.io/distroless") {
		t.Error("expected Dockerfile to have distroless run stage")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustResolve(t *testing.T, name string) adapter.Adapter {
	t.Helper()
	a, err := adapter.Resolve(name)
	if err != nil {
		t.Fatalf("failed to resolve adapter %q: %v", name, err)
	}
	return a
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func containsStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
