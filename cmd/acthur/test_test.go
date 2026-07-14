package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestRunTest_CIModeUsesRaceAndUncachedTests specifies the externally
// observable contract of `acthur test --ci`: every selected Go service is
// exercised with the race detector and a cold test cache. The runner seam
// keeps this command test focused on argument projection rather than spawning
// nested Go toolchains.
func TestRunTest_CIModeUsesRaceAndUncachedTests(t *testing.T) {
	dir := t.TempDir()
	twoServiceProject(t, dir)
	writeGoNodeWithTest(t, dir, "api", false)
	writeGoNodeWithTest(t, dir, "worker", false)

	_, g, err := loadProjectGraph(dir)
	if err != nil {
		t.Fatalf("loadProjectGraph: %v", err)
	}

	type call struct {
		node string
		args []string
	}
	var calls []call
	runner := func(_ string, node string, _ io.Writer, _ []string, args ...string) error {
		calls = append(calls, call{node: node, args: append([]string(nil), args...)})
		return nil
	}

	err = runTestMode(dir, g, "", io.Discard, testModeCI, runner)
	if err != nil {
		t.Fatalf("runTestMode(--ci): %v", err)
	}
	want := []call{
		{node: "api", args: []string{"test", "./...", "-race", "-count=1"}},
		{node: "worker", args: []string{"test", "./...", "-race", "-count=1"}},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("CI go invocations = %#v, want %#v", calls, want)
	}
}

func TestRunTest_CIModeAggregatesFailuresAndRunsEveryNode(t *testing.T) {
	dir := t.TempDir()
	twoServiceProject(t, dir)
	writeGoNodeWithTest(t, dir, "api", false)
	writeGoNodeWithTest(t, dir, "worker", false)

	_, g, err := loadProjectGraph(dir)
	if err != nil {
		t.Fatalf("loadProjectGraph: %v", err)
	}

	var ran []string
	runner := func(_ string, node string, _ io.Writer, _ []string, _ ...string) error {
		ran = append(ran, node)
		return os.ErrInvalid
	}
	err = runTestMode(dir, g, "", io.Discard, testModeCI, runner)
	if err == nil {
		t.Fatal("runTestMode(--ci) succeeded despite failing service tests")
	}
	if !reflect.DeepEqual(ran, []string{"api", "worker"}) {
		t.Fatalf("CI ran nodes %v, want all selected nodes [api worker]", ran)
	}
	for _, node := range ran {
		if !strings.Contains(err.Error(), node) {
			t.Errorf("aggregate error %q does not name failing node %q", err, node)
		}
	}
}

func TestTestCommand_ExposesCIMode(t *testing.T) {
	flag := testCmd.Flags().Lookup("ci")
	if flag == nil {
		t.Fatal("acthur test does not expose --ci")
	}
	if flag.DefValue != "false" {
		t.Fatalf("--ci default = %q, want false", flag.DefValue)
	}
}

// writeGoNodeWithTest drops a minimal Go module at dir/<name> with a single
// test that passes (or fails, per fail) — for exercising runTest without
// needing a full project's worth of real service code.
func writeGoNodeWithTest(t *testing.T, dir, name string, fail bool) {
	t.Helper()
	root := filepath.Join(dir, name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "func TestOK(t *testing.T) {}\n"
	if fail {
		body = "func TestOK(t *testing.T) { t.Fatal(\"boom\") }\n"
	}
	files := map[string]string{
		"go.mod":       "module example.com/" + name + "\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\n" + body,
	}
	for fname, content := range files {
		if err := os.WriteFile(filepath.Join(root, fname), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// twoServiceProject writes an acthur.yml with two go:fiber service nodes
// (api, worker) and chdir's into dir for the test's duration.
func twoServiceProject(t *testing.T, dir string) {
	t.Helper()
	yml := `project: p
version: "1"
graph:
  nodes:
    api:
      type: service
      adapter: go:fiber
      port: 8080
    worker:
      type: service
      adapter: go:fiber
      port: 8081
`
	if err := os.WriteFile(filepath.Join(dir, "acthur.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

// TestRunTest_RunsAllBuildableNodes: with no service named, every buildable
// service node's test suite runs.
func TestRunTest_RunsAllBuildableNodes(t *testing.T) {
	dir := t.TempDir()
	twoServiceProject(t, dir)
	writeGoNodeWithTest(t, dir, "api", false)
	writeGoNodeWithTest(t, dir, "worker", false)

	_, g, err := loadProjectGraph(dir)
	if err != nil {
		t.Fatalf("loadProjectGraph: %v", err)
	}

	var out bytes.Buffer
	if err := runTest(dir, g, "", &out); err != nil {
		t.Fatalf("runTest: %v (output: %s)", err, out.String())
	}
	for _, want := range []string{"api", "worker"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected test output to mention node %q, got: %s", want, out.String())
		}
	}
}

// TestRunTest_SingleService_RunsOnlyNamed: naming one service only runs
// that node's suite, leaving the other untouched.
func TestRunTest_SingleService_RunsOnlyNamed(t *testing.T) {
	dir := t.TempDir()
	twoServiceProject(t, dir)
	writeGoNodeWithTest(t, dir, "api", false)
	writeGoNodeWithTest(t, dir, "worker", true) // would fail if run

	_, g, err := loadProjectGraph(dir)
	if err != nil {
		t.Fatalf("loadProjectGraph: %v", err)
	}

	var out bytes.Buffer
	if err := runTest(dir, g, "api", &out); err != nil {
		t.Fatalf("runTest(api): %v (output: %s)", err, out.String())
	}
	if strings.Contains(out.String(), "worker") {
		t.Errorf("expected worker's suite not to run, got: %s", out.String())
	}
}

// TestRunTest_UnknownService_Errors: naming a service that isn't a
// buildable node fails pointedly instead of silently running nothing.
func TestRunTest_UnknownService_Errors(t *testing.T) {
	dir := t.TempDir()
	writeTestProject(t, dir)
	writeBuildableAPINode(t, dir)

	_, g, err := loadProjectGraph(dir)
	if err != nil {
		t.Fatalf("loadProjectGraph: %v", err)
	}

	var out bytes.Buffer
	err = runTest(dir, g, "nonexistent", &out)
	if err == nil {
		t.Fatal("expected an error for an unknown service")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("expected service name in error, got: %v", err)
	}
}

// TestRunTest_FailingTests_ReturnsFailedList: a node whose test suite fails
// is named in the aggregate error, and the other node's suite still runs.
func TestRunTest_FailingTests_ReturnsFailedList(t *testing.T) {
	dir := t.TempDir()
	twoServiceProject(t, dir)
	writeGoNodeWithTest(t, dir, "api", false)
	writeGoNodeWithTest(t, dir, "worker", true)

	_, g, err := loadProjectGraph(dir)
	if err != nil {
		t.Fatalf("loadProjectGraph: %v", err)
	}

	var out bytes.Buffer
	err = runTest(dir, g, "", &out)
	if err == nil {
		t.Fatal("expected an error since worker's suite fails")
	}
	if !strings.Contains(err.Error(), "worker") {
		t.Errorf("expected failing node named in error, got: %v", err)
	}
	if !strings.Contains(out.String(), "api") {
		t.Errorf("expected api's (passing) suite to still have run, got: %s", out.String())
	}
}
