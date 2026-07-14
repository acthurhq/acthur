package deploy_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthurhq/acthur/internal/config"
	"github.com/acthurhq/acthur/internal/deploy"
	"github.com/acthurhq/acthur/internal/graph"
)

// writeGoNode writes a minimal buildable (and optionally test-failing) Go
// module under dir/<node>.
func writeGoNode(t *testing.T, dir, node string, brokenBuild, failingTest bool) {
	t.Helper()
	root := filepath.Join(dir, node)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gomod := "module example.com/" + node + "\n\ngo 1.22\n"
	main := "package main\n\nfunc main() {}\n"
	if brokenBuild {
		main = "package main\n\nfunc main() { undefinedSymbol() }\n"
	}
	test := "package main\n\nimport \"testing\"\n\nfunc TestOK(t *testing.T) {}\n"
	if failingTest {
		test = "package main\n\nimport \"testing\"\n\nfunc TestFail(t *testing.T) { t.Fatal(\"gate must catch this\") }\n"
	}
	for name, content := range map[string]string{
		"go.mod":       gomod,
		"main.go":      main,
		"main_test.go": test,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func passingChecks(t *testing.T, dir string) deploy.GateInput {
	t.Helper()
	writeGoNode(t, dir, "api", false, false)
	return deploy.GateInput{
		Root:            dir,
		ServiceNodes:    []string{"api"},
		RequiredEnv:     nil,
		Graph:           buildProductionGraph(t, "go:fiber"),
		AdapterResolver: productionResolver{},
	}
}

// TestGate_AllChecksPass: a healthy project passes every check and reports
// each one as run.
func TestGate_AllChecksPass(t *testing.T) {
	dir := t.TempDir()
	report, err := deploy.RunGate(passingChecks(t, dir))
	if err != nil {
		t.Fatalf("expected gate to pass, got: %v", err)
	}
	if len(report.Checks) == 0 {
		t.Fatal("expected the report to list executed checks")
	}
	for _, c := range report.Checks {
		if !c.OK {
			t.Errorf("check %s failed: %s", c.Name, c.Detail)
		}
	}
}

// TestGate_ContractsCheckPassesWhenProjectHasNoContracts: contracts are
// optional, but the deploy report must still prove that the gate checked them.
func TestGate_ContractsCheckPassesWhenProjectHasNoContracts(t *testing.T) {
	dir := t.TempDir()
	report, err := deploy.RunGate(passingChecks(t, dir))
	if err != nil {
		t.Fatalf("expected a project without contracts to pass, got: %v", err)
	}
	assertGateCheck(t, report, "contracts", true)
}

// TestGate_GeneratedArtifacts proves deploy validates the on-disk artifacts
// users will actually ship against generated.lock. Projects created before
// the lock existed (and empty locks) remain valid, while a deleted or edited
// tracked file blocks deploy and is named in the aggregate failure.
func TestGate_GeneratedArtifacts(t *testing.T) {
	t.Run("absent lock passes named check", func(t *testing.T) {
		dir := t.TempDir()
		report, err := deploy.RunGate(passingChecks(t, dir))
		if err != nil {
			t.Fatalf("expected absent generated.lock to pass, got: %v", err)
		}
		assertGateCheck(t, report, "generated artifacts", true)
	})

	t.Run("empty lock passes named check", func(t *testing.T) {
		dir := t.TempDir()
		in := passingChecks(t, dir)
		if err := os.WriteFile(filepath.Join(dir, "generated.lock"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		report, err := deploy.RunGate(in)
		if err != nil {
			t.Fatalf("expected empty generated.lock to pass, got: %v", err)
		}
		assertGateCheck(t, report, "generated artifacts", true)
	})

	for _, tt := range []struct {
		name    string
		content *string
	}{
		{name: "missing tracked file", content: nil},
		{name: "edited tracked file", content: stringPtr("user edit\n")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			in := passingChecks(t, dir)
			const tracked = "api/internal/generated.go"
			if tt.content != nil {
				target := filepath.Join(dir, filepath.FromSlash(tracked))
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(*tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			// SHA-256 of the content acthur last generated ("generated\n").
			lock := tracked + ": 9f5936ff15d3a2ba7d3d8f21858338a6c1e2adc9fe34c685c7de5b4a00caa29a\n"
			if err := os.WriteFile(filepath.Join(dir, "generated.lock"), []byte(lock), 0o644); err != nil {
				t.Fatal(err)
			}

			report, err := deploy.RunGate(in)
			if err == nil {
				t.Fatal("expected stale generated artifact to block deploy")
			}
			if msg := err.Error(); !strings.Contains(msg, "generated artifacts:") || !strings.Contains(msg, tracked) {
				t.Fatalf("expected pointed generated artifact failure, got: %v", err)
			}
			assertGateCheck(t, report, "generated artifacts", false)
		})
	}
}

// TestGate_Migrations proves the production gate distinguishes projects that
// do not use migrations from projects whose database schema is deployment
// state. The status callback is the command-layer seam: deploy owns the
// policy, while the migrations plugin owns live database inspection.
func TestGate_Migrations(t *testing.T) {
	t.Run("project without migrations passes named check", func(t *testing.T) {
		dir := t.TempDir()
		in := passingChecks(t, dir)

		report, err := deploy.RunGate(in)
		if err != nil {
			t.Fatalf("expected project without migrations to pass, got: %v", err)
		}
		assertGateCheck(t, report, "migrations", true)
	})

	for _, tt := range []struct {
		name  string
		state deploy.MigrationState
		want  string
	}{
		{
			name:  "dirty database blocks pointedly",
			state: deploy.MigrationState{Configured: true, Applied: 4, Latest: 5, Dirty: true},
			want:  "database is dirty at migration 4",
		},
		{
			name:  "database behind latest up migration blocks pointedly",
			state: deploy.MigrationState{Configured: true, Applied: 4, Latest: 5},
			want:  "database migration version 4 is behind latest migration 5",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			in := passingChecks(t, dir)
			in.MigrationStatus = func() (deploy.MigrationState, error) { return tt.state, nil }

			report, err := deploy.RunGate(in)
			if err == nil {
				t.Fatal("expected migration state to block deploy")
			}
			if msg := err.Error(); !strings.Contains(msg, "migrations:") || !strings.Contains(msg, tt.want) {
				t.Fatalf("expected pointed migrations failure %q, got: %v", tt.want, err)
			}
			assertGateCheck(t, report, "migrations", false)
		})
	}

	t.Run("fully applied clean database passes", func(t *testing.T) {
		dir := t.TempDir()
		in := passingChecks(t, dir)
		in.MigrationStatus = func() (deploy.MigrationState, error) {
			return deploy.MigrationState{Configured: true, Applied: 5, Latest: 5}, nil
		}

		report, err := deploy.RunGate(in)
		if err != nil {
			t.Fatalf("expected clean fully applied migrations to pass, got: %v", err)
		}
		assertGateCheck(t, report, "migrations", true)
	})
}

// TestGate_Security proves the deploy layer runs the command-supplied
// production security policy without depending on a concrete plugin.
func TestGate_Security(t *testing.T) {
	t.Run("project without security policy passes named check", func(t *testing.T) {
		dir := t.TempDir()
		report, err := deploy.RunGate(passingChecks(t, dir))
		if err != nil {
			t.Fatalf("expected project without security policy to pass, got: %v", err)
		}
		assertGateCheck(t, report, "security", true)
	})

	t.Run("unsafe production policy blocks pointedly", func(t *testing.T) {
		dir := t.TempDir()
		in := passingChecks(t, dir)
		in.SecurityCheck = func() error { return fmt.Errorf("wildcard CORS origin is not allowed") }

		report, err := deploy.RunGate(in)
		if err == nil {
			t.Fatal("expected unsafe security policy to block deploy")
		}
		if !strings.Contains(err.Error(), "security: wildcard CORS origin is not allowed") {
			t.Fatalf("expected pointed security failure, got: %v", err)
		}
		assertGateCheck(t, report, "security", false)
	})
}

func stringPtr(value string) *string { return &value }

// TestGate_InvalidContractsBlockDeploy: contract files are production inputs,
// so both unreadable syntax and structurally invalid contracts must prevent a
// deploy even when build, test, and environment checks pass.
func TestGate_InvalidContractsBlockDeploy(t *testing.T) {
	tests := []struct {
		name     string
		contract string
	}{
		{
			name:     "malformed YAML",
			contract: "contract: [unterminated\n",
		},
		{
			name: "semantically invalid",
			contract: `contract: users
version: "1"
transport: http
endpoints:
  - path: /users
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			in := passingChecks(t, dir)
			contractsDir := filepath.Join(dir, "contracts")
			if err := os.MkdirAll(contractsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(contractsDir, "users.contract.yml"), []byte(tt.contract), 0o644); err != nil {
				t.Fatal(err)
			}

			report, err := deploy.RunGate(in)
			if err == nil {
				t.Fatal("expected invalid contract to block deploy")
			}
			if !strings.Contains(err.Error(), "contracts") {
				t.Fatalf("expected failure to name contracts check, got: %v", err)
			}
			assertGateCheck(t, report, "contracts", false)
		})
	}
}

func assertGateCheck(t *testing.T, report *deploy.GateReport, name string, wantOK bool) {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			if check.OK != wantOK {
				t.Fatalf("check %q OK = %v, want %v (detail: %s)", name, check.OK, wantOK, check.Detail)
			}
			return
		}
	}
	t.Fatalf("report did not contain check %q: %#v", name, report.Checks)
}

type productionResolver struct{}

func (productionResolver) Resolve(key string) (graph.ResolvedAdapter, bool) {
	if key == "go:fiber" {
		return graph.ResolvedAdapter{Name: key, Category: "backend"}, true
	}
	return graph.ResolvedAdapter{}, false
}

func (productionResolver) Names() []string { return []string{"go:fiber"} }

func buildProductionGraph(t *testing.T, adapter string) *graph.Graph {
	t.Helper()
	g, err := graph.Build(&config.Config{Graph: config.GraphConfig{
		Nodes: map[string]config.NodeConfig{
			"api": {Type: config.NodeTypeService, Adapter: adapter},
		},
		Edges: []config.EdgeConfig{{From: "api", To: "proxy", Type: config.EdgeProxiedThrough}},
	}})
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	return g
}

// TestGate_ValidatesProductionGraph proves the deploy gate owns semantic
// graph validation: callers cannot accidentally ship a graph merely because
// they built it without running graph.Validate themselves.
func TestGate_ValidatesProductionGraph(t *testing.T) {
	t.Run("valid graph passes named check", func(t *testing.T) {
		dir := t.TempDir()
		in := passingChecks(t, dir)
		in.Graph = buildProductionGraph(t, "go:fiber")
		in.AdapterResolver = productionResolver{}

		report, err := deploy.RunGate(in)
		if err != nil {
			t.Fatalf("expected valid production graph to pass, got: %v", err)
		}
		assertGateCheck(t, report, "graph", true)
	})

	t.Run("unresolved adapter blocks deploy with pointed aggregate failure", func(t *testing.T) {
		dir := t.TempDir()
		in := passingChecks(t, dir)
		in.Graph = buildProductionGraph(t, "go:unknown")
		in.AdapterResolver = productionResolver{}

		report, err := deploy.RunGate(in)
		if err == nil {
			t.Fatal("expected semantic graph failure to block deploy")
		}
		if msg := err.Error(); !strings.Contains(msg, "graph:") || !strings.Contains(msg, "unresolved-adapter") || !strings.Contains(msg, "go:unknown") {
			t.Fatalf("expected pointed aggregate graph failure, got: %v", err)
		}
		assertGateCheck(t, report, "graph", false)
	})
}

// TestGate_BrokenBuild_FailsWithPointedMessage: the gate refuses to ship
// code that does not compile, naming the node.
func TestGate_BrokenBuild_FailsWithPointedMessage(t *testing.T) {
	dir := t.TempDir()
	writeGoNode(t, dir, "api", true, false)
	_, err := deploy.RunGate(deploy.GateInput{Root: dir, ServiceNodes: []string{"api"}})
	if err == nil {
		t.Fatal("expected gate failure for broken build")
	}
	if !strings.Contains(err.Error(), "api") || !strings.Contains(err.Error(), "build") {
		t.Errorf("expected pointed build failure naming the node, got: %v", err)
	}
}

// TestGate_FailingTest_Fails: failing tests block the deploy.
func TestGate_FailingTest_Fails(t *testing.T) {
	dir := t.TempDir()
	writeGoNode(t, dir, "api", false, true)
	_, err := deploy.RunGate(deploy.GateInput{Root: dir, ServiceNodes: []string{"api"}})
	if err == nil {
		t.Fatal("expected gate failure for failing test")
	}
	if !strings.Contains(err.Error(), "test") {
		t.Errorf("expected test failure in error, got: %v", err)
	}
}

// TestGate_MissingEnvVar_Fails: required env vars for the target env must
// resolve before shipping.
func TestGate_MissingEnvVar_Fails(t *testing.T) {
	dir := t.TempDir()
	in := passingChecks(t, dir)
	in.RequiredEnv = []string{"ACTHUR_GATE_TEST_MISSING_VAR"}
	_ = os.Unsetenv("ACTHUR_GATE_TEST_MISSING_VAR")
	_, err := deploy.RunGate(in)
	if err == nil {
		t.Fatal("expected gate failure for missing env var")
	}
	if !strings.Contains(err.Error(), "ACTHUR_GATE_TEST_MISSING_VAR") {
		t.Errorf("expected the missing var named, got: %v", err)
	}
}

// TestGate_RequiredEnvVarPresent_Passes: an exported env var is available to
// the deploy process and therefore satisfies the delivery preflight.
func TestGate_RequiredEnvVarPresent_Passes(t *testing.T) {
	dir := t.TempDir()
	in := passingChecks(t, dir)
	in.RequiredEnv = []string{"ACTHUR_GATE_TEST_ENV_WINS"}
	t.Setenv("ACTHUR_GATE_TEST_ENV_WINS", "from-shell")
	if _, err := deploy.RunGate(in); err != nil {
		t.Fatalf("expected gate to pass via process env, got: %v", err)
	}
}

// TestGate_CollectsAllFailures: the gate reports every failure at once, not
// just the first — a deploy attempt should tell the whole story.
func TestGate_CollectsAllFailures(t *testing.T) {
	dir := t.TempDir()
	writeGoNode(t, dir, "api", true, false)
	in := deploy.GateInput{
		Root:         dir,
		ServiceNodes: []string{"api"},
		RequiredEnv:  []string{"ACTHUR_GATE_TEST_MISSING_VAR"},
	}
	_ = os.Unsetenv("ACTHUR_GATE_TEST_MISSING_VAR")
	_, err := deploy.RunGate(in)
	if err == nil {
		t.Fatal("expected failures")
	}
	msg := err.Error()
	if !strings.Contains(msg, "build") || !strings.Contains(msg, "ACTHUR_GATE_TEST_MISSING_VAR") {
		t.Errorf("expected both failures reported together, got: %v", err)
	}
}

// TestGoStream_StreamsOutputAndSucceeds: a passing `go test` streams its
// output live to the given writer and returns no error.
func TestGoStream_StreamsOutputAndSucceeds(t *testing.T) {
	dir := t.TempDir()
	writeGoNode(t, dir, "api", false, false)

	var buf bytes.Buffer
	if err := deploy.GoStream(dir, "api", &buf, nil, "test", "./..."); err != nil {
		t.Fatalf("expected GoStream to succeed, got: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected go test output to be streamed to the writer")
	}
}

// TestGoStream_FailingTest_ReturnsPointedError: a failing `go test` names
// the node in the returned error.
func TestGoStream_FailingTest_ReturnsPointedError(t *testing.T) {
	dir := t.TempDir()
	writeGoNode(t, dir, "api", false, true)

	var buf bytes.Buffer
	err := deploy.GoStream(dir, "api", &buf, nil, "test", "./...")
	if err == nil {
		t.Fatal("expected GoStream to fail for a failing test")
	}
	if !strings.Contains(err.Error(), "api") {
		t.Errorf("expected node name in error, got: %v", err)
	}
}

// TestGoStream_PassesEnv_CGODisabledForBuild: env vars passed through are
// visible to the invoked go command (used by acthur build to force
// CGO_ENABLED=0 for static production binaries).
func TestGoStream_PassesEnv_CGODisabledForBuild(t *testing.T) {
	dir := t.TempDir()
	writeGoNode(t, dir, "api", false, false)
	outPath := filepath.Join(dir, "api-bin")

	var buf bytes.Buffer
	err := deploy.GoStream(dir, "api", &buf, []string{"CGO_ENABLED=0"}, "build", "-o", outPath, ".")
	if err != nil {
		t.Fatalf("expected build to succeed, got: %v (output: %s)", err, buf.String())
	}
	if _, statErr := os.Stat(outPath); statErr != nil {
		t.Errorf("expected built binary at %s: %v", outPath, statErr)
	}
}
