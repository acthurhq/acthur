package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBuildableAPINode drops a minimal compiling Go module at dir/api so
// the pre-deploy gate's build+test checks pass.
func writeBuildableAPINode(t *testing.T, dir string) {
	t.Helper()
	root := filepath.Join(dir, "api")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":  "module example.com/api\n\ngo 1.22\n",
		"main.go": "package main\n\nfunc main() {}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestRunDeploy_DryRun_PlansWithoutSideEffects: --dry-run reports the plan
// and touches nothing — no artifacts on disk, no docker invocations.
func TestRunDeploy_DryRun_PlansWithoutSideEffects(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)
	writeBuildableAPINode(t, dir)

	var calls int
	run := func(args ...string) (string, error) { calls++; return "", nil }

	plan, err := runDeploy(dir, "production", "", true, run)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(plan) == 0 || !strings.Contains(strings.Join(plan, "\n"), "compose") {
		t.Errorf("expected a plan mentioning the compose target, got: %v", plan)
	}
	if calls != 0 {
		t.Errorf("dry run must not invoke docker, got %d calls", calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "deploy")); !os.IsNotExist(err) {
		t.Error("dry run must not write deploy artifacts")
	}
}

// TestRunDeploy_ComposeTarget_EndToEnd: full run writes artifacts, passes
// the gate, and drives docker compose up + health wait.
func TestRunDeploy_ComposeTarget_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)
	writeBuildableAPINode(t, dir)
	// The compose projection references ${VAR}s; the gate requires them
	// resolvable at deploy time (secrets never live in artifacts).
	for _, v := range []string{"APP_ENV", "APP_PORT", "APP_SECRET", "DATABASE_URL", "REDIS_URL"} {
		t.Setenv(v, "test-value")
	}

	var joined strings.Builder
	run := func(args ...string) (string, error) {
		joined.WriteString(strings.Join(args, " ") + "\n")
		if strings.Contains(strings.Join(args, " "), "ps") {
			return `{"Service":"api","State":"running","Health":"healthy"}`, nil
		}
		return "", nil
	}

	if _, err := runDeploy(dir, "production", "", false, run); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "deploy", "docker-compose.prod.yml")); err != nil {
		t.Errorf("expected compose artifact written: %v", err)
	}
	for _, want := range []string{"up -d --build", "ps"} {
		if !strings.Contains(joined.String(), want) {
			t.Errorf("expected docker invocation containing %q, got:\n%s", want, joined.String())
		}
	}
}

// TestRunDeploy_GateFailure_BlocksDeploy: a broken build never reaches the
// target.
func TestRunDeploy_GateFailure_BlocksDeploy(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)
	root := filepath.Join(dir, "api")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/api\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { undefinedSymbol() }\n"), 0o644)

	var calls int
	run := func(args ...string) (string, error) { calls++; return "", nil }

	_, err := runDeploy(dir, "production", "", false, run)
	if err == nil {
		t.Fatal("expected gate failure")
	}
	if !strings.Contains(err.Error(), "build") {
		t.Errorf("expected build failure surfaced, got: %v", err)
	}
	if calls != 0 {
		t.Errorf("gate failure must block the target, got %d docker calls", calls)
	}
}

// TestRunDeploy_UnknownEnv_Errors: naming an environment that acthur.yml
// doesn't define fails pointedly.
func TestRunDeploy_UnknownEnv_Errors(t *testing.T) {
	dir := t.TempDir()
	yml := `project: p
version: "1"
environments:
  staging:
    context: docker
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
`
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(dir)

	_, err := runDeploy(dir, "production", "", false, func(args ...string) (string, error) { return "", nil })
	if err == nil || !strings.Contains(err.Error(), "production") || !strings.Contains(err.Error(), "staging") {
		t.Fatalf("expected pointed unknown-env error listing defined envs, got: %v", err)
	}
}
