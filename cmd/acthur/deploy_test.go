package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/secrets"
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

	// Deploy success fixtures must be semantically connected production
	// graphs. The shared project fixture intentionally contains only a node.
	configPath := filepath.Join(dir, "acthur.yml")
	yml, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	yml = append(yml, []byte("  edges:\n    - from: api\n      to: proxy\n      type: proxied_through\n")...)
	if err := os.WriteFile(configPath, yml, 0o644); err != nil {
		t.Fatal(err)
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
	_ = os.MkdirAll(root, 0o755)
	_ = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/api\n\ngo 1.22\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { undefinedSymbol() }\n"), 0o644)

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

// TestRunDeploy_MigrationFilesWireLiveStatusCheck proves command assembly
// does not silently skip migration state. A project with an up migration but
// no delivered DATABASE_URL cannot establish live production status and is
// blocked before artifacts or target execution.
func TestRunDeploy_MigrationFilesWireLiveStatusCheck(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)
	writeBuildableAPINode(t, dir)
	migrationsDir := filepath.Join(dir, "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(migrationsDir, "0001_init.up.sql"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Unsetenv("DATABASE_URL")

	var calls int
	run := func(args ...string) (string, error) { calls++; return "", nil }
	_, err := runDeploy(dir, "production", "", false, run)
	if err == nil || !strings.Contains(err.Error(), "migrations:") || !strings.Contains(err.Error(), "DATABASE_URL is not set") {
		t.Fatalf("expected live migration status preflight to block pointedly, got: %v", err)
	}
	if calls != 0 {
		t.Fatalf("migration gate failure must block target execution, got %d calls", calls)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "deploy")); !os.IsNotExist(statErr) {
		t.Fatalf("migration gate failure must block artifact writes, stat error: %v", statErr)
	}
}

// TestRunDeploy_SecurityPluginWiresProductionValidation proves an unsafe
// configured security plugin blocks before artifacts or the target are
// touched; command assembly must not omit the builtin policy callback.
func TestRunDeploy_SecurityPluginWiresProductionValidation(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)
	writeBuildableAPINode(t, dir)
	configPath := filepath.Join(dir, "acthur.yml")
	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("plugins:\n  - name: security\n    config:\n      allowed_origins: ['*']\n"); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"APP_ENV", "APP_PORT", "APP_SECRET", "DATABASE_URL", "REDIS_URL"} {
		t.Setenv(v, "test-value")
	}

	var calls int
	run := func(args ...string) (string, error) {
		calls++
		if strings.Contains(strings.Join(args, " "), "ps") {
			return `{"Service":"api","State":"running","Health":"healthy"}`, nil
		}
		return "", nil
	}
	_, err = runDeploy(dir, "production", "", false, run)
	if err == nil || !strings.Contains(err.Error(), "security:") || !strings.Contains(err.Error(), "wildcard CORS origin") {
		t.Fatalf("expected production security gate to block pointedly, got: %v", err)
	}
	if calls != 0 {
		t.Fatalf("security gate failure must block target execution, got %d calls", calls)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "deploy")); !os.IsNotExist(statErr) {
		t.Fatalf("security gate failure must block artifact writes, stat error: %v", statErr)
	}
}

// TestRunDeploy_LocalSecretDoesNotSatisfyProductionEnv proves that the
// preflight checks the environment which will actually be delivered to the
// target. The project-local development store is not target configuration,
// so merely storing a required value there must not allow production deploy.
func TestRunDeploy_LocalSecretDoesNotSatisfyProductionEnv(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)
	writeBuildableAPINode(t, dir)
	for _, v := range []string{"APP_ENV", "APP_PORT", "DATABASE_URL", "REDIS_URL"} {
		t.Setenv(v, "test-value")
	}
	_ = os.Unsetenv("APP_SECRET")
	if err := secrets.New(dir).Set("APP_SECRET", "local-only-value"); err != nil {
		t.Fatalf("seed local secret: %v", err)
	}

	var calls int
	run := func(args ...string) (string, error) { calls++; return "", nil }
	_, err := runDeploy(dir, "production", "", false, run)
	if err == nil || !strings.Contains(err.Error(), "APP_SECRET") {
		t.Fatalf("expected missing delivered APP_SECRET to block deploy, got: %v", err)
	}
	if calls != 0 {
		t.Fatalf("gate failure must block the target, got %d docker calls", calls)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "deploy")); !os.IsNotExist(statErr) {
		t.Fatalf("gate failure must block artifact writes, stat error: %v", statErr)
	}
}

// TestRunDeploy_RemoteTargets_RequireTokenEnvVars: fly/railway/render are
// gated by the same pre-deploy gate as compose/coolify — a missing
// provider token blocks before any docker/API call is made.
func TestRunDeploy_RemoteTargets_RequireTokenEnvVars(t *testing.T) {
	cases := []struct {
		target  string
		wantVar string
	}{
		{"fly", "FLY_API_TOKEN"},
		{"railway", "RAILWAY_TOKEN"},
		{"render", "RENDER_API_KEY"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			dir := t.TempDir()
			writeTestProject(t, dir)
			writeBuildableAPINode(t, dir)
			for _, v := range []string{"APP_ENV", "APP_PORT", "APP_SECRET", "DATABASE_URL", "REDIS_URL"} {
				t.Setenv(v, "test-value")
			}

			var calls int
			run := func(args ...string) (string, error) { calls++; return "", nil }

			_, err := runDeploy(dir, "production", tc.target, false, run)
			if err == nil {
				t.Fatalf("expected the gate to block on a missing %s", tc.wantVar)
			}
			if !strings.Contains(err.Error(), tc.wantVar) {
				t.Errorf("expected error to mention %s, got: %v", tc.wantVar, err)
			}
			if calls != 0 {
				t.Errorf("gate failure must block the target, got %d docker/API calls", calls)
			}
		})
	}
}

// TestRunDeploy_DryRun_RemoteTargets_PlanMentionsTarget: --dry-run for each
// remote target reports the plan (including the FLY/RAILWAY/RENDER token
// var it will require) without touching docker or the network.
func TestRunDeploy_DryRun_RemoteTargets_PlanMentionsTarget(t *testing.T) {
	for _, target := range []string{"fly", "railway", "render"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			writeTestProject(t, dir)
			writeBuildableAPINode(t, dir)

			var calls int
			run := func(args ...string) (string, error) { calls++; return "", nil }

			plan, err := runDeploy(dir, "production", target, true, run)
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			if !strings.Contains(strings.Join(plan, "\n"), "target: "+target) {
				t.Errorf("expected plan to mention target %q, got: %v", target, plan)
			}
			if calls != 0 {
				t.Errorf("dry run must not invoke docker, got %d calls", calls)
			}
		})
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
	defer func() { _ = os.Chdir(wd) }()
	_ = os.Chdir(dir)

	_, err := runDeploy(dir, "production", "", false, func(args ...string) (string, error) { return "", nil })
	if err == nil || !strings.Contains(err.Error(), "production") || !strings.Contains(err.Error(), "staging") {
		t.Fatalf("expected pointed unknown-env error listing defined envs, got: %v", err)
	}
}
