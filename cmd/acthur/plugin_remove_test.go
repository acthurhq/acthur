package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/config"
)

func writePluginTestProject(t *testing.T, dir, yml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunPluginRemove_RemovesSimpleEntry(t *testing.T) {
	dir := t.TempDir()
	writePluginTestProject(t, dir, `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
plugins:
  - name: migrations

  - name: auth
    config:
      strategy: jwt
`)

	summary, err := runPluginRemove(dir, "migrations")
	if err != nil {
		t.Fatalf("runPluginRemove: %v", err)
	}
	if summary.Plugin != "migrations" {
		t.Errorf("expected summary.Plugin=migrations, got %q", summary.Plugin)
	}

	cfg, err := config.LoadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatalf("reloading acthur.yml: %v", err)
	}
	if len(cfg.Plugins) != 1 || cfg.Plugins[0].Name != "auth" {
		t.Fatalf("expected only 'auth' to remain, got %+v", cfg.Plugins)
	}
}

func TestRunPluginRemove_RemovesEntryWithNestedConfig(t *testing.T) {
	dir := t.TempDir()
	writePluginTestProject(t, dir, `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
plugins:
  - name: migrations

  - name: auth
    config:
      strategy: jwt
      algorithm: RS256
      oauth2:
        providers:
          - google
          - github

  - name: rbac
    config:
      roles:
        - admin
        - user
`)

	if _, err := runPluginRemove(dir, "auth"); err != nil {
		t.Fatalf("runPluginRemove: %v", err)
	}

	cfg, err := config.LoadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatalf("reloading acthur.yml: %v", err)
	}
	names := make([]string, len(cfg.Plugins))
	for i, p := range cfg.Plugins {
		names[i] = p.Name
	}
	if len(names) != 2 || names[0] != "migrations" || names[1] != "rbac" {
		t.Fatalf("expected [migrations rbac] to remain, got %v", names)
	}
	// rbac's own nested config must survive untouched.
	if len(cfg.Plugins[1].Config) == 0 {
		t.Error("expected rbac's config map to survive the auth removal")
	}
}

func TestRunPluginRemove_RemovesLastRemainingEntry(t *testing.T) {
	dir := t.TempDir()
	writePluginTestProject(t, dir, `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
plugins:
  - name: migrations
`)

	if _, err := runPluginRemove(dir, "migrations"); err != nil {
		t.Fatalf("runPluginRemove: %v", err)
	}

	cfg, err := config.LoadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatalf("reloading acthur.yml: %v", err)
	}
	if len(cfg.Plugins) != 0 {
		t.Fatalf("expected no plugins to remain, got %+v", cfg.Plugins)
	}
}

func TestRunPluginRemove_NotInstalledErrorsPointedly(t *testing.T) {
	dir := t.TempDir()
	writePluginTestProject(t, dir, `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
plugins:
  - name: migrations
`)

	before, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = runPluginRemove(dir, "does-not-exist")
	if err == nil {
		t.Fatal("expected an error for a plugin that isn't installed")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("expected error to name the plugin, got: %v", err)
	}
	if !strings.Contains(err.Error(), "migrations") {
		t.Errorf("expected error to list installed plugins, got: %v", err)
	}

	after, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("expected acthur.yml to be unchanged when the plugin isn't installed")
	}
}

func TestRunPluginRemove_PreservesTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writePluginTestProject(t, dir, `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
plugins:
  - name: migrations
`)

	if _, err := runPluginRemove(dir, "migrations"); err != nil {
		t.Fatalf("runPluginRemove: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "acthur.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Errorf("expected acthur.yml to remain newline-terminated, got %q", string(data))
	}
}

func TestRunPluginRemove_NoPluginsListErrorsPointedly(t *testing.T) {
	dir := t.TempDir()
	writePluginTestProject(t, dir, `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
`)

	_, err := runPluginRemove(dir, "migrations")
	if err == nil {
		t.Fatal("expected an error when acthur.yml has no plugins listed")
	}
	if !strings.Contains(err.Error(), "no plugins") {
		t.Errorf("expected 'no plugins' in error, got: %v", err)
	}
}
