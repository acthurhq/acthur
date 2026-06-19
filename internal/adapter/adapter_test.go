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
// go:fiber core interface tests
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

// ---------------------------------------------------------------------------
// go:fiber Runnable capability tests
// ---------------------------------------------------------------------------

func TestGoFiber_DevCommand(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	r, ok := a.(adapter.Runnable)
	if !ok {
		t.Fatal("go:fiber does not implement Runnable")
	}
	cmd := r.DevCommand(nil)
	if cmd.Bin != "air" {
		t.Errorf("expected dev command bin=air, got %q", cmd.Bin)
	}
	if len(cmd.Args) == 0 {
		t.Error("expected air to have arguments (-c .air.toml)")
	}
}

func TestGoFiber_BuildCommand(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	r, ok := a.(adapter.Runnable)
	if !ok {
		t.Fatal("go:fiber does not implement Runnable")
	}
	cmd := r.BuildCommand(nil)
	if cmd.Bin != "go" {
		t.Errorf("expected build command bin=go, got %q", cmd.Bin)
	}
}

func TestGoFiber_TestCommand(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	r, ok := a.(adapter.Runnable)
	if !ok {
		t.Fatal("go:fiber does not implement Runnable")
	}
	cmd := r.TestCommand(nil)
	if cmd.Bin != "go" {
		t.Errorf("expected test command bin=go, got %q", cmd.Bin)
	}
}

// ---------------------------------------------------------------------------
// go:fiber Scaffolder capability tests
// ---------------------------------------------------------------------------

func TestGoFiber_Scaffold_ProducesFiles(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	s, ok := a.(adapter.Scaffolder)
	if !ok {
		t.Fatal("go:fiber does not implement Scaffolder")
	}
	files, err := s.Scaffold(adapter.ScaffoldContext{
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
	s, ok := a.(adapter.Scaffolder)
	if !ok {
		t.Fatal("go:fiber does not implement Scaffolder")
	}
	files, err := s.Scaffold(adapter.ScaffoldContext{
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
	s, ok := a.(adapter.Scaffolder)
	if !ok {
		t.Fatal("go:fiber does not implement Scaffolder")
	}
	files, err := s.Scaffold(adapter.ScaffoldContext{
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
	s, ok := a.(adapter.Scaffolder)
	if !ok {
		t.Fatal("go:fiber does not implement Scaffolder")
	}
	files, err := s.Scaffold(adapter.ScaffoldContext{
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

// ---------------------------------------------------------------------------
// Capability model tests
// ---------------------------------------------------------------------------

// Behavior 1: CapabilitiesOf(go:fiber) returns exactly {Scaffold, Run}.
func TestCapabilitiesOf_GoFiber_ExactlyScaffoldAndRun(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	caps := adapter.CapabilitiesOf(a)

	want := map[adapter.Capability]bool{
		adapter.CapabilityScaffold: true,
		adapter.CapabilityRun:      true,
	}
	notWant := []adapter.Capability{
		adapter.CapabilityContainer,
		adapter.CapabilityMigrate,
		adapter.CapabilityDeploy,
	}

	if len(caps) != 2 {
		t.Errorf("expected exactly 2 capabilities, got %d: %v", len(caps), caps)
	}
	for _, c := range caps {
		if !want[c] {
			t.Errorf("unexpected capability %q in CapabilitiesOf(go:fiber)", c)
		}
	}
	capSet := make(map[adapter.Capability]bool)
	for _, c := range caps {
		capSet[c] = true
	}
	for _, c := range notWant {
		if capSet[c] {
			t.Errorf("capability %q should not be present for go:fiber", c)
		}
	}
}

// Behavior 2: go:fiber satisfies Scaffolder.
func TestGoFiber_SatisfiesScaffolder(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	if _, ok := a.(adapter.Scaffolder); !ok {
		t.Error("go:fiber does not satisfy Scaffolder interface")
	}
}

// Behavior 3: go:fiber satisfies Runnable.
func TestGoFiber_SatisfiesRunnable(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	if _, ok := a.(adapter.Runnable); !ok {
		t.Error("go:fiber does not satisfy Runnable interface")
	}
}

// Behavior 4: go:fiber does NOT satisfy Containerized.
func TestGoFiber_DoesNotSatisfyContainerized(t *testing.T) {
	a := mustResolve(t, "go:fiber")
	if _, ok := a.(adapter.Containerized); ok {
		t.Error("go:fiber should not satisfy Containerized — it has no Container() method")
	}
}

// Behavior 5: CapabilitiesOf returns empty slice for a minimal adapter with no capabilities.
func TestCapabilitiesOf_MinimalAdapter_ReturnsEmpty(t *testing.T) {
	caps := adapter.CapabilitiesOf(&minimalAdapter{})
	if len(caps) != 0 {
		t.Errorf("expected empty capability slice for minimal adapter, got %v", caps)
	}
}

// minimalAdapter is a stub that satisfies only the core Adapter interface.
type minimalAdapter struct{}

func (m *minimalAdapter) Name() string             { return "test:minimal" }
func (m *minimalAdapter) Category() adapter.Category { return adapter.CategoryBackend }
func (m *minimalAdapter) Detect(dir string) bool   { return false }
func (m *minimalAdapter) EnvVars() []adapter.EnvVar { return nil }

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
