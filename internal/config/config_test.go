package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acthur/acthur/internal/config"
)

// ---------------------------------------------------------------------------
// Load tests
// ---------------------------------------------------------------------------

func TestLoad_ValidConfig(t *testing.T) {
	cfg, err := config.LoadFile(fixture("vetangle/acthur.yml"))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Project != "vetangle" {
		t.Errorf("expected project=vetangle, got %q", cfg.Project)
	}
	if cfg.Version != "1" {
		t.Errorf("expected version=1, got %q", cfg.Version)
	}
	if cfg.Identifiers.Strategy != config.StrategyULID {
		t.Errorf("expected strategy=ulid, got %q", cfg.Identifiers.Strategy)
	}
	if cfg.Dev.Domain != "vetangle.test" {
		t.Errorf("expected domain=vetangle.test, got %q", cfg.Dev.Domain)
	}
	if cfg.Dev.Port != 4000 {
		t.Errorf("expected port=4000, got %d", cfg.Dev.Port)
	}
}

func TestLoad_NodeCount(t *testing.T) {
	cfg, err := config.LoadFile(fixture("vetangle/acthur.yml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 8 // api, web, backoffice, worker, db, cache, storage, queue
	if got := len(cfg.Graph.Nodes); got != want {
		t.Errorf("expected %d nodes, got %d", want, got)
	}
}

func TestLoad_EdgeCount(t *testing.T) {
	cfg, err := config.LoadFile(fixture("vetangle/acthur.yml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Graph.Edges) == 0 {
		t.Error("expected edges to be defined")
	}
}

func TestLoad_PluginCount(t *testing.T) {
	cfg, err := config.LoadFile(fixture("vetangle/acthur.yml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 4 // migrations, auth, rbac, multitenancy
	if got := len(cfg.Plugins); got != want {
		t.Errorf("expected %d plugins, got %d", want, got)
	}
}

func TestLoad_DefaultPort(t *testing.T) {
	cfg := writeConfigAndLoad(t, `
project: testapp
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
  edges: []
`)
	if cfg.Dev.Port != 4000 {
		t.Errorf("expected default port 4000, got %d", cfg.Dev.Port)
	}
}

func TestLoad_DefaultDomain(t *testing.T) {
	cfg := writeConfigAndLoad(t, `
project: myproject
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
  edges: []
`)
	if cfg.Dev.Domain != "myproject.test" {
		t.Errorf("expected domain myproject.test, got %q", cfg.Dev.Domain)
	}
}

func TestLoad_DefaultIdentifierStrategy(t *testing.T) {
	cfg := writeConfigAndLoad(t, `
project: testapp
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
  edges: []
`)
	if cfg.Identifiers.Strategy != config.StrategyULID {
		t.Errorf("expected default strategy ulid, got %q", cfg.Identifiers.Strategy)
	}
}

func TestLoad_DefaultPoolSettings(t *testing.T) {
	cfg := writeConfigAndLoad(t, `
project: testapp
version: "1"
graph:
  nodes:
    db:
      type: infra
      adapter: db:postgres
  edges: []
`)
	pool := cfg.Graph.Nodes["db"].Pool
	if pool.MaxConns != "auto" {
		t.Errorf("expected max_conns=auto, got %q", pool.MaxConns)
	}
	if pool.BeforeAcquire != "ping" {
		t.Errorf("expected before_acquire=ping, got %q", pool.BeforeAcquire)
	}
	if pool.HealthCheckPeriod != "30s" {
		t.Errorf("expected health_check_period=30s, got %q", pool.HealthCheckPeriod)
	}
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestValidate_MissingProject(t *testing.T) {
	_, err := config.LoadFile(writeTempConfig(t, `
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
  edges: []
`))
	if err == nil {
		t.Error("expected error for missing project, got nil")
	}
}

func TestValidate_UnknownNodeRef(t *testing.T) {
	_, err := config.LoadFile(writeTempConfig(t, `
project: test
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
  edges:
    - from: api
      to: nonexistent
      type: depends_on
`))
	if err == nil {
		t.Error("expected error for unknown node reference, got nil")
	}
}

func TestValidate_SelfReferencingEdge(t *testing.T) {
	_, err := config.LoadFile(writeTempConfig(t, `
project: test
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
  edges:
    - from: api
      to: api
      type: depends_on
`))
	if err == nil {
		t.Error("expected error for self-referencing edge, got nil")
	}
}

func TestValidate_DataFlowMissingContract(t *testing.T) {
	_, err := config.LoadFile(writeTempConfig(t, `
project: test
version: "1"
graph:
  nodes:
    web:
      type: service
      adapter: ui:astro
    api:
      type: service
      adapter: go:fiber
  edges:
    - from: web
      to: api
      type: data_flow
`))
	if err == nil {
		t.Error("expected error for data_flow edge without contract, got nil")
	}
}

func TestValidate_EmitsEdgeMissingEvents(t *testing.T) {
	_, err := config.LoadFile(writeTempConfig(t, `
project: test
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
    worker:
      type: service
      adapter: go:fiber
  edges:
    - from: api
      to: worker
      type: emits
`))
	if err == nil {
		t.Error("expected error for emits edge without events, got nil")
	}
}

func TestValidate_ValidMinimalConfig(t *testing.T) {
	cfg := writeConfigAndLoad(t, `
project: testapp
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
    db:
      type: infra
      adapter: db:postgres
  edges:
    - from: api
      to: db
      type: depends_on
`)
	if len(cfg.Graph.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(cfg.Graph.Nodes))
	}
}

// ---------------------------------------------------------------------------
// Find tests
// ---------------------------------------------------------------------------

func TestFind_WalksUp(t *testing.T) {
	// Create acthur.yml in a temp dir, then call Find from a subdirectory
	root := t.TempDir()
	sub := filepath.Join(root, "services", "api", "internal")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(root, "acthur.yml")
	writeFile(t, configPath, minimalConfig("walktest"))

	found, err := config.Find(sub)
	if err != nil {
		t.Fatalf("expected to find acthur.yml, got error: %v", err)
	}
	if found != configPath {
		t.Errorf("expected %q, got %q", configPath, found)
	}
}

func TestFind_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := config.Find(dir)
	if err != config.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestExists_True(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "acthur.yml"), minimalConfig("test"))
	if !config.Exists(dir) {
		t.Error("expected Exists to return true")
	}
}

func TestExists_False(t *testing.T) {
	dir := t.TempDir()
	if config.Exists(dir) {
		t.Error("expected Exists to return false")
	}
}

// ---------------------------------------------------------------------------
// Node helpers
// ---------------------------------------------------------------------------

func TestNodeConfig_IsHotReloadEnabled_DefaultService(t *testing.T) {
	cfg := writeConfigAndLoad(t, `
project: test
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
    db:
      type: infra
      adapter: db:postgres
  edges:
    - from: api
      to: db
      type: depends_on
`)
	apiNode := cfg.Graph.Nodes["api"]
	if !apiNode.IsHotReloadEnabled() {
		t.Error("expected service node to have hot_reload enabled by default")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func fixture(rel string) string {
	return filepath.Join("..", "..", "testdata", rel)
}

func writeConfigAndLoad(t *testing.T, content string) *config.Config {
	t.Helper()
	path := writeTempConfig(t, content)
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
	return cfg
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "acthur.yml")
	writeFile(t, path, content)
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func minimalConfig(project string) string {
	return `project: ` + project + `
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
    db:
      type: infra
      adapter: db:postgres
  edges:
    - from: api
      to: db
      type: depends_on
`
}
