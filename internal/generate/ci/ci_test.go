package ci_test

import (
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/config"
	"github.com/acthur/acthur/internal/generate/ci"
	"gopkg.in/yaml.v3"
)

func TestGenerate_GitHubActions_ValidYAMLWorkflow(t *testing.T) {
	cfg := &config.Config{Project: "vetangle"}

	file, err := ci.Generate(cfg, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if file.Path != ".github/workflows/acthur.yml" {
		t.Errorf("expected .github/workflows/acthur.yml, got %s", file.Path)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(file.Content, &parsed); err != nil {
		t.Fatalf("generated workflow is not valid YAML: %v\n%s", err, file.Content)
	}

	if _, ok := parsed["on"]; !ok {
		t.Error("expected top-level 'on' trigger key")
	}
	jobs, ok := parsed["jobs"].(map[string]any)
	if !ok {
		t.Fatalf("expected jobs to be a map, got %T", parsed["jobs"])
	}
	job, ok := jobs["build-and-test"].(map[string]any)
	if !ok {
		t.Fatalf("expected build-and-test job, got %+v", jobs)
	}
	if job["runs-on"] != "ubuntu-latest" {
		t.Errorf("expected runs-on ubuntu-latest, got %v", job["runs-on"])
	}

	content := string(file.Content)
	for _, want := range []string{
		"actions/checkout@v4", "actions/setup-go@v5", "go-version",
		"acthur build", "acthur test",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected workflow to contain %q, got:\n%s", want, content)
		}
	}
}

func TestGenerate_ExplicitGitHubActionsTarget(t *testing.T) {
	if _, err := ci.Generate(&config.Config{Project: "p"}, "github-actions"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
}

func TestGenerate_UnsupportedTarget_ErrorsClearly(t *testing.T) {
	_, err := ci.Generate(&config.Config{Project: "p"}, "gitlab-ci")
	if err == nil {
		t.Fatal("expected error for unsupported target")
	}
	if !strings.Contains(err.Error(), "gitlab-ci") || !strings.Contains(err.Error(), "github-actions") {
		t.Errorf("expected error to name the target and the supported list, got: %v", err)
	}
}

func TestGenerate_Deterministic(t *testing.T) {
	cfg := &config.Config{Project: "p"}
	f1, err := ci.Generate(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := ci.Generate(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(f1.Content) != string(f2.Content) {
		t.Error("expected deterministic output")
	}
}
