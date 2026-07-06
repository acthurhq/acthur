package deploy_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acthur/acthur/internal/deploy"
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
		Root:         dir,
		ServiceNodes: []string{"api"},
		RequiredEnv:  nil,
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
	os.Unsetenv("ACTHUR_GATE_TEST_MISSING_VAR")
	_, err := deploy.RunGate(in)
	if err == nil {
		t.Fatal("expected gate failure for missing env var")
	}
	if !strings.Contains(err.Error(), "ACTHUR_GATE_TEST_MISSING_VAR") {
		t.Errorf("expected the missing var named, got: %v", err)
	}
}

// TestGate_MissingEnvVar_ResolvedBySecretFallback: a required var absent from
// the process environment is satisfied by the ResolveSecret fallback (the
// project's local secret store in production).
func TestGate_MissingEnvVar_ResolvedBySecretFallback(t *testing.T) {
	dir := t.TempDir()
	in := passingChecks(t, dir)
	in.RequiredEnv = []string{"ACTHUR_GATE_TEST_MISSING_VAR"}
	os.Unsetenv("ACTHUR_GATE_TEST_MISSING_VAR")
	in.ResolveSecret = func(key string) (string, bool) {
		if key == "ACTHUR_GATE_TEST_MISSING_VAR" {
			return "resolved-from-secret-store", true
		}
		return "", false
	}
	if _, err := deploy.RunGate(in); err != nil {
		t.Fatalf("expected gate to pass via secret fallback, got: %v", err)
	}
}

// TestGate_MissingEnvVar_ProcessEnvWinsOverSecretFallback: an exported env
// var is checked before the fallback and satisfies the gate even if the
// fallback would report ok=false.
func TestGate_MissingEnvVar_ProcessEnvWinsOverSecretFallback(t *testing.T) {
	dir := t.TempDir()
	in := passingChecks(t, dir)
	in.RequiredEnv = []string{"ACTHUR_GATE_TEST_ENV_WINS"}
	t.Setenv("ACTHUR_GATE_TEST_ENV_WINS", "from-shell")
	in.ResolveSecret = func(key string) (string, bool) { return "", false }
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
	os.Unsetenv("ACTHUR_GATE_TEST_MISSING_VAR")
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
